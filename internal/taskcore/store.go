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
)

const (
	indexFileName     = "index.json"
	rootFileName      = "root.json"
	childrenFileName  = "children.json"
	eventsFileName    = "events.jsonl"
	approvalsFileName = "approvals.json"
	presenceFileName  = "presence.json"
)

type Store struct {
	root    string
	taskIDs *TaskIDAllocator
}

type IndexEntry struct {
	ID             string `json:"id"`
	AssignmentType string `json:"assignment_type"`
	AssignmentID   string `json:"assignment_id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
}

type taskIndexState struct {
	Counters taskCounterState `json:"counters"`
	Tasks    []IndexEntry     `json:"tasks"`
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
	index, ok, err := readTaskIndex(filepath.Join(s.root, indexFileName))
	if err != nil {
		return nil, err
	}
	if !ok {
		index.Tasks, err = buildTaskIndex(s.root)
		if err != nil {
			return nil, err
		}
	}
	out := make([]Snapshot, 0, len(index.Tasks))
	for _, entry := range index.Tasks {
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
	var root Task
	if err := readJSONFile(filepath.Join(dir, rootFileName), &root); err != nil {
		return Snapshot{}, err
	}
	var children []Task
	if err := readOptionalJSONFile(filepath.Join(dir, childrenFileName), &children); err != nil {
		return Snapshot{}, err
	}
	var approvals []TaskApproval
	if err := readOptionalJSONFile(filepath.Join(dir, approvalsFileName), &approvals); err != nil {
		return Snapshot{}, err
	}
	var presence []TaskPresence
	if err := readOptionalJSONFile(filepath.Join(dir, presenceFileName), &presence); err != nil {
		return Snapshot{}, err
	}
	events, err := s.readEvents(taskID)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Root:      root,
		Children:  children,
		Approvals: approvals,
		Presence:  presence,
		Events:    events,
	}, nil
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
	dir := s.taskDir(snapshot.Root.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := s.appendEvents(snapshot.Root.ID, newEvents); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(dir, rootFileName), snapshot.Root); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(dir, childrenFileName), snapshot.Children); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(dir, approvalsFileName), snapshot.Approvals); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(dir, presenceFileName), snapshot.Presence); err != nil {
		return err
	}
	return s.writeIndex()
}

func (s *Store) DeleteRoot(taskID string) error {
	if s == nil {
		return fmt.Errorf("task store is required")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("task id is required")
	}
	if err := os.RemoveAll(s.taskDir(taskID)); err != nil {
		return err
	}
	return s.writeIndex()
}

func (s *Store) DeleteAssignment(assignmentType, assignmentID string) error {
	assignmentType = strings.TrimSpace(assignmentType)
	assignmentID = strings.TrimSpace(assignmentID)
	snapshots, err := s.LoadByAssignment(assignmentType, assignmentID)
	if err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		if err := os.RemoveAll(s.taskDir(snapshot.Root.ID)); err != nil {
			return err
		}
	}
	return s.writeIndex()
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

func (s *Store) appendEvents(taskID string, events []TaskEvent) error {
	if len(events) == 0 {
		return nil
	}
	path := filepath.Join(s.taskDir(taskID), eventsFileName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	for _, event := range events {
		if err := enc.Encode(event); err != nil {
			return err
		}
	}
	if _, err := file.Write(buf.Bytes()); err != nil {
		return err
	}
	return file.Sync()
}

func (s *Store) readEvents(taskID string) ([]TaskEvent, error) {
	path := filepath.Join(s.taskDir(taskID), eventsFileName)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var (
		events     []TaskEvent
		validBytes int64
	)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			var event TaskEvent
			if unmarshalErr := json.Unmarshal(bytes.TrimSpace(line), &event); unmarshalErr != nil {
				if errors.Is(err, io.EOF) {
					if truncateErr := file.Truncate(validBytes); truncateErr != nil {
						return nil, truncateErr
					}
					if _, seekErr := file.Seek(validBytes, io.SeekStart); seekErr != nil {
						return nil, seekErr
					}
					if syncErr := file.Sync(); syncErr != nil {
						return nil, syncErr
					}
					return events, nil
				}
				return nil, fmt.Errorf("decode %s: %w", path, unmarshalErr)
			}
			events = append(events, event)
			validBytes += int64(len(line))
		}
		if errors.Is(err, io.EOF) {
			return events, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func (s *Store) writeIndex() error {
	index, err := s.buildIndex()
	if err != nil {
		return err
	}
	if s.taskIDs != nil {
		return s.taskIDs.writeIndex(index)
	}
	return writeTaskIndex(filepath.Join(s.root, indexFileName), taskIndexState{Tasks: index})
}

func (s *Store) buildIndex() ([]IndexEntry, error) {
	return buildTaskIndex(s.root)
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
		rootPath := filepath.Join(root, entry.Name(), rootFileName)
		if _, err := os.Stat(rootPath); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		var task Task
		if err := readJSONFile(rootPath, &task); err != nil {
			return nil, err
		}
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

func readTaskIndex(path string) (taskIndexState, bool, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return taskIndexState{}, false, nil
	} else if err != nil {
		return taskIndexState{}, false, err
	}
	var state taskIndexState
	if err := readJSONFile(path, &state); err != nil {
		return taskIndexState{}, false, err
	}
	if state.Counters.Task < 0 {
		return taskIndexState{}, false, fmt.Errorf("task id counter is invalid: %d", state.Counters.Task)
	}
	state.Tasks = cloneIndexEntries(state.Tasks)
	return state, true, nil
}

func writeTaskIndex(path string, state taskIndexState) error {
	if state.Counters.Task < 0 {
		return fmt.Errorf("task id counter is invalid: %d", state.Counters.Task)
	}
	state.Tasks = cloneIndexEntries(state.Tasks)
	return writeJSONFile(path, state)
}

func cloneIndexEntries(in []IndexEntry) []IndexEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]IndexEntry, len(in))
	copy(out, in)
	return out
}

func readOptionalJSONFile(path string, target any) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return readJSONFile(path, target)
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
