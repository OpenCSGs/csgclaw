package taskcore

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	sequenceFileName = "sequence.json"
	tasksFileName    = "tasks.json"
	eventsFileName   = "events.jsonl"
)

type Store struct {
	root    string
	taskIDs *TaskIDAllocator
	locks   sync.Map
}

// taskRecord is the durable transaction boundary. Tasks are peers in one flat
// list; parent_id is the only persisted hierarchy.
type taskRecord struct {
	Tasks     []Task         `json:"tasks"`
	EventSeq  int64          `json:"event_seq,omitempty"`
	Approvals []TaskApproval `json:"approvals,omitempty"`
	Presence  []TaskPresence `json:"presence,omitempty"`
}

type IndexEntry struct {
	ID             string `json:"id"`
	AssignmentType string `json:"assignment_type"`
	AssignmentID   string `json:"assignment_id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
}

type taskSequenceState struct {
	LastTask int64 `json:"last_task"`
}

func NewStore(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("task store root is required")
	}
	taskIDs, err := newPersistentTaskIDAllocator(root)
	if err != nil {
		return nil, err
	}
	return &Store{root: root, taskIDs: taskIDs}, nil
}

func (s *Store) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func (s *Store) TaskIDAllocator() *TaskIDAllocator {
	if s == nil {
		return nil
	}
	return s.taskIDs
}

func (s *Store) Load() ([]Snapshot, error) {
	if s == nil {
		return nil, fmt.Errorf("task store is required")
	}
	// Root records are authoritative. The shared sequence file only allocates
	// numeric task IDs and never serves as a task index.
	entries, err := buildTaskIndex(s.root)
	if err != nil {
		return nil, err
	}
	out := make([]Snapshot, 0, len(entries))
	for _, entry := range entries {
		snapshot, err := s.LoadRoot(entry.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, snapshot)
	}
	return out, nil
}

func (s *Store) LoadByAssignment(assignmentType, assignmentID string) ([]Snapshot, error) {
	assignmentType = strings.TrimSpace(assignmentType)
	assignmentID = strings.TrimSpace(assignmentID)
	snapshots, err := s.Load()
	if err != nil {
		return nil, err
	}
	out := make([]Snapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.Root.AssignmentType == assignmentType && snapshot.Root.AssignmentID == assignmentID {
			out = append(out, snapshot)
		}
	}
	return out, nil
}

func (s *Store) LoadRoot(taskID string) (Snapshot, error) {
	if s == nil {
		return Snapshot{}, fmt.Errorf("task store is required")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Snapshot{}, fmt.Errorf("task id is required")
	}
	dir := s.taskDir(taskID)
	var record taskRecord
	if err := readJSONFile(filepath.Join(dir, tasksFileName), &record); err != nil {
		return Snapshot{}, err
	}
	snapshot, err := snapshotFromRecord(taskID, record)
	if err != nil {
		return Snapshot{}, err
	}
	events, err := readCommittedEvents(filepath.Join(dir, eventsFileName), record.EventSeq, false)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.Events = events
	return snapshot, nil
}

func (s *Store) SaveSnapshot(snapshot Snapshot, newEvents []TaskEvent) error {
	if s == nil {
		return fmt.Errorf("task store is required")
	}
	if strings.TrimSpace(snapshot.Root.ID) == "" {
		return fmt.Errorf("root task id is required")
	}
	if strings.TrimSpace(snapshot.Root.ParentID) != "" {
		return fmt.Errorf("root task cannot have parent_id")
	}
	unlock := s.lockRoot(snapshot.Root.ID)
	defer unlock()
	record, err := recordFromSnapshot(snapshot)
	if err != nil {
		return err
	}
	dir := s.taskDir(snapshot.Root.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	committedSeq, err := committedEventSeq(filepath.Join(dir, tasksFileName))
	if err != nil {
		return err
	}
	eventsPath := filepath.Join(dir, eventsFileName)
	if _, err := readCommittedEvents(eventsPath, committedSeq, true); err != nil {
		return err
	}
	if err := appendEvents(eventsPath, newEvents); err != nil {
		return err
	}
	// Task state and its event outbox commit together for every assignment type.
	if err := writeJSONFileAtomic(filepath.Join(dir, tasksFileName), record); err != nil {
		_, _ = readCommittedEvents(eventsPath, committedSeq, true)
		return err
	}
	return nil
}

func (s *Store) DeleteRoot(taskID string) error {
	if s == nil {
		return fmt.Errorf("task store is required")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("task id is required")
	}
	unlock := s.lockRoot(taskID)
	defer unlock()
	if err := os.RemoveAll(s.taskDir(taskID)); err != nil {
		return err
	}
	return nil
}

func (s *Store) DeleteAssignment(assignmentType, assignmentID string) error {
	assignmentType = strings.TrimSpace(assignmentType)
	assignmentID = strings.TrimSpace(assignmentID)
	snapshots, err := s.LoadByAssignment(assignmentType, assignmentID)
	if err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		if err := s.DeleteRoot(snapshot.Root.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ReplaceAssignment(assignmentType, assignmentID string, snapshots []Snapshot, eventsByRoot map[string][]TaskEvent) error {
	if err := s.DeleteAssignment(assignmentType, assignmentID); err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		if err := s.SaveSnapshot(snapshot, eventsByRoot[snapshot.Root.ID]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) taskDir(taskID string) string {
	return filepath.Join(s.root, taskID)
}

func (s *Store) lockRoot(taskID string) func() {
	value, _ := s.locks.LoadOrStore(strings.TrimSpace(taskID), &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock
}

func recordFromSnapshot(snapshot Snapshot) (taskRecord, error) {
	tasks := make([]Task, 0, len(snapshot.Children)+1)
	tasks = append(tasks, snapshot.Root)
	tasks = append(tasks, snapshot.Children...)
	record := taskRecord{
		Tasks:     tasks,
		EventSeq:  latestEventSeq(snapshot.Events),
		Approvals: snapshot.Approvals,
		Presence:  snapshot.Presence,
	}
	if _, err := snapshotFromRecord(snapshot.Root.ID, record); err != nil {
		return taskRecord{}, err
	}
	return record, nil
}

func snapshotFromRecord(expectedRootID string, record taskRecord) (Snapshot, error) {
	expectedRootID = strings.TrimSpace(expectedRootID)
	if len(record.Tasks) == 0 {
		return Snapshot{}, fmt.Errorf("task record %q has no tasks", expectedRootID)
	}

	byID := make(map[string]Task, len(record.Tasks))
	rootID := ""
	for _, task := range record.Tasks {
		task.ID = strings.TrimSpace(task.ID)
		if task.ID == "" {
			return Snapshot{}, fmt.Errorf("task record %q contains an empty task id", expectedRootID)
		}
		if _, exists := byID[task.ID]; exists {
			return Snapshot{}, fmt.Errorf("task record %q contains duplicate task %q", expectedRootID, task.ID)
		}
		byID[task.ID] = task
		if strings.TrimSpace(task.ParentID) == "" {
			if rootID != "" {
				return Snapshot{}, fmt.Errorf("task record %q contains multiple root tasks", expectedRootID)
			}
			rootID = task.ID
		}
	}
	if rootID == "" {
		return Snapshot{}, fmt.Errorf("task record %q has no root task", expectedRootID)
	}
	if rootID != expectedRootID {
		return Snapshot{}, fmt.Errorf("task record directory %q does not match root task %q", expectedRootID, rootID)
	}
	root := byID[rootID]
	children := make([]Task, 0, len(record.Tasks)-1)
	for _, task := range record.Tasks {
		if task.ID == rootID {
			continue
		}
		if task.AssignmentType != root.AssignmentType || task.AssignmentID != root.AssignmentID {
			return Snapshot{}, fmt.Errorf("task %q does not share root assignment", task.ID)
		}
		seen := map[string]bool{task.ID: true}
		cursor := task
		for strings.TrimSpace(cursor.ParentID) != "" {
			parentID := strings.TrimSpace(cursor.ParentID)
			if seen[parentID] {
				return Snapshot{}, fmt.Errorf("task %q has a parent cycle", task.ID)
			}
			seen[parentID] = true
			parent, ok := byID[parentID]
			if !ok {
				return Snapshot{}, fmt.Errorf("task %q references missing parent %q", task.ID, parentID)
			}
			cursor = parent
		}
		if cursor.ID != rootID {
			return Snapshot{}, fmt.Errorf("task %q does not belong to root %q", task.ID, rootID)
		}
		children = append(children, task)
	}
	sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })
	return Snapshot{
		Root:      root,
		Children:  children,
		Approvals: record.Approvals,
		Presence:  record.Presence,
	}, nil
}

func latestEventSeq(events []TaskEvent) int64 {
	var latest int64
	for _, event := range events {
		if event.Seq > latest {
			latest = event.Seq
		}
	}
	return latest
}

func committedEventSeq(tasksPath string) (int64, error) {
	if _, err := os.Stat(tasksPath); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	var record taskRecord
	if err := readJSONFile(tasksPath, &record); err != nil {
		return 0, err
	}
	return record.EventSeq, nil
}

func appendEvents(path string, events []TaskEvent) error {
	if len(events) == 0 {
		return nil
	}
	buffer := bytes.NewBuffer(nil)
	encoder := json.NewEncoder(buffer)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return err
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(buffer.Bytes()); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// readCommittedEvents ignores and optionally removes an uncommitted tail. The
// tasks.json event_seq is the commit marker for the append-only journal.
func readCommittedEvents(path string, committedSeq int64, repair bool) ([]TaskEvent, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if committedSeq > 0 {
			return nil, fmt.Errorf("task event journal is missing through seq %d", committedSeq)
		}
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	flag := os.O_RDONLY
	if repair {
		flag = os.O_RDWR
	}
	file, err := os.OpenFile(path, flag, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	events := make([]TaskEvent, 0)
	var committedBytes int64
	var latestSeq int64
	for {
		if latestSeq >= committedSeq {
			break
		}
		line, readErr := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var event TaskEvent
			if err := json.Unmarshal(bytes.TrimSpace(line), &event); err != nil {
				if !repair || !errors.Is(readErr, io.EOF) {
					return nil, fmt.Errorf("decode %s: %w", path, err)
				}
				break
			}
			if event.Seq > committedSeq {
				break
			}
			events = append(events, event)
			if event.Seq > latestSeq {
				latestSeq = event.Seq
			}
			committedBytes += int64(len(line))
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	if latestSeq < committedSeq {
		return nil, fmt.Errorf("task event journal ends at seq %d, want %d", latestSeq, committedSeq)
	}
	if repair {
		if err := file.Truncate(committedBytes); err != nil {
			return nil, err
		}
		return events, file.Sync()
	}
	return events, nil
}

func buildTaskIndex(root string) ([]IndexEntry, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	index := make([]IndexEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		tasksPath := filepath.Join(root, entry.Name(), tasksFileName)
		if _, err := os.Stat(tasksPath); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		var record taskRecord
		if err := readJSONFile(tasksPath, &record); err != nil {
			return nil, err
		}
		snapshot, err := snapshotFromRecord(entry.Name(), record)
		if err != nil {
			return nil, err
		}
		task := snapshot.Root
		index = append(index, IndexEntry{
			ID:             task.ID,
			AssignmentType: task.AssignmentType,
			AssignmentID:   task.AssignmentID,
			Title:          task.Title,
			Status:         task.Status,
		})
	}
	sort.Slice(index, func(i, j int) bool { return index[i].ID < index[j].ID })
	return index, nil
}

func readTaskSequence(path string) (taskSequenceState, bool, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return taskSequenceState{}, false, nil
	} else if err != nil {
		return taskSequenceState{}, false, err
	}
	var state taskSequenceState
	if err := readJSONFile(path, &state); err != nil {
		return taskSequenceState{}, false, err
	}
	if state.LastTask < 0 {
		return taskSequenceState{}, false, fmt.Errorf("task id sequence is invalid: %d", state.LastTask)
	}
	return state, true, nil
}

func writeTaskSequence(path string, state taskSequenceState) error {
	if state.LastTask < 0 {
		return fmt.Errorf("task id sequence is invalid: %d", state.LastTask)
	}
	return writeJSONFile(path, state)
}

func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	return json.Unmarshal(data, target)
}

func writeJSONFileAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".tasks-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
