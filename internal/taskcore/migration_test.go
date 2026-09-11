package taskcore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreMigratesLegacyMultiFileSnapshot(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "task-40")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	rootTask := Task{ID: "task-40", AssignmentType: AssignmentTypeTeam, AssignmentID: "team-legacy", Title: "Legacy team work", Status: StatusInProgress, CreatedBy: "pt-manager", AssignedTo: "pt-dev", CreatedAt: now, UpdatedAt: now}
	child := Task{ID: "task-41", ParentID: rootTask.ID, AssignmentType: rootTask.AssignmentType, AssignmentID: rootTask.AssignmentID, Title: "Legacy child", Status: StatusPending, CreatedBy: "pt-manager", CreatedAt: now, UpdatedAt: now}
	approval := TaskApproval{ID: "approval-3", AssignmentType: rootTask.AssignmentType, AssignmentID: rootTask.AssignmentID, TaskID: rootTask.ID, RequestedBy: "pt-dev", Kind: "command", Summary: "Run checks", Status: ApprovalStatusPending, CreatedAt: now}
	presence := TaskPresence{AssignmentType: rootTask.AssignmentType, AssignmentID: rootTask.AssignmentID, ParticipantID: "pt-dev", State: "working", CurrentTaskID: rootTask.ID, UpdatedAt: now}
	events := []TaskEvent{
		{Seq: 17, AssignmentType: rootTask.AssignmentType, AssignmentID: rootTask.AssignmentID, Type: EventTaskCreated, TaskID: rootTask.ID, CreatedAt: now},
		{Seq: 18, AssignmentType: rootTask.AssignmentType, AssignmentID: rootTask.AssignmentID, Type: EventPresenceUpdated, TaskID: rootTask.ID, CreatedAt: now},
	}
	if err := writeJSONFile(filepath.Join(dir, legacyRootFileName), rootTask); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(dir, legacyChildrenFileName), []Task{child}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(dir, legacyApprovalsFileName), []TaskApproval{approval}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(dir, legacyPresenceFileName), []TaskPresence{presence}); err != nil {
		t.Fatal(err)
	}
	if err := writeTaskEventsAtomic(filepath.Join(dir, eventsFileName), events); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(root, legacyTaskIndexFileName), map[string]any{"counters": map[string]any{"task": 50}}); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore() migration error = %v", err)
	}
	assertTaskSequence(t, root, 50)
	snapshot, err := store.LoadRoot(rootTask.ID)
	if err != nil {
		t.Fatalf("LoadRoot() error = %v", err)
	}
	if snapshot.Root.ID != rootTask.ID || len(snapshot.Children) != 1 || snapshot.Children[0].ID != child.ID {
		t.Fatalf("migrated tasks = %+v %+v", snapshot.Root, snapshot.Children)
	}
	if len(snapshot.Events) != 2 || snapshot.Events[1].Seq != 18 {
		t.Fatalf("migrated events = %+v", snapshot.Events)
	}
	if len(snapshot.Approvals) != 1 || snapshot.Approvals[0].ID != approval.ID {
		t.Fatalf("migrated approvals = %+v", snapshot.Approvals)
	}
	if len(snapshot.Presence) != 1 || snapshot.Presence[0].ParticipantID != presence.ParticipantID {
		t.Fatalf("migrated presence = %+v", snapshot.Presence)
	}
	assertCurrentTaskRecord(t, filepath.Join(dir, tasksFileName), 18)
	if _, err := os.Stat(filepath.Join(dir, legacyRootFileName)); err != nil {
		t.Fatalf("legacy root was not retained: %v", err)
	}

	// A second startup observes tasks.json as the per-record migration marker
	// and the canonical sequence protects the next global task ID.
	resetTaskIDAllocatorsForTest()
	reloadedStore, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore(reloaded) error = %v", err)
	}
	created, err := NewService(WithStore(reloadedStore)).CreateRoot(CreateRootInput{AssignmentType: AssignmentTypeAgent, AssignmentID: "agent-dev", Title: "Next", CreatedBy: "user-admin"})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "task-51" {
		t.Fatalf("next task id = %q, want task-51", created.ID)
	}
}

func TestStoreMigratesLegacyAggregateV1(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "task-6")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	rootTask := Task{ID: "task-6", AssignmentType: AssignmentTypeRoom, AssignmentID: "room-a", RoomID: "room-a", Title: "Build", Status: StatusInProgress, CreatedBy: "pt-admin", CreatedAt: now, UpdatedAt: now}
	child := Task{ID: "task-7", ParentID: rootTask.ID, AssignmentType: rootTask.AssignmentType, AssignmentID: rootTask.AssignmentID, RoomID: "room-a", Title: "Implement", Status: StatusReview, CreatedBy: "pt-manager", AssignedTo: "pt-dev", CreatedAt: now, UpdatedAt: now}
	event := TaskEvent{Seq: 33, AssignmentType: rootTask.AssignmentType, AssignmentID: rootTask.AssignmentID, RoomID: "room-a", Type: EventTaskCompleted, TaskID: child.ID, CreatedAt: now}
	aggregate := legacyTaskAggregate{Task: rootTask, AggregateVersion: legacyAggregateVersion, Children: []Task{child}, Events: []TaskEvent{event}}
	if err := writeJSONFile(filepath.Join(dir, legacyRootFileName), aggregate); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore() migration error = %v", err)
	}
	snapshot, err := store.LoadRoot(rootTask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Children) != 1 || snapshot.Children[0].ParentID != rootTask.ID || len(snapshot.Events) != 1 || snapshot.Events[0].Seq != event.Seq {
		t.Fatalf("migrated aggregate = %+v", snapshot)
	}
	assertCurrentTaskRecord(t, filepath.Join(dir, tasksFileName), event.Seq)
}

func TestStoreStampsUnversionedFlatRecord(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "task-9")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	record := taskRecord{Tasks: []Task{{ID: "task-9", AssignmentType: AssignmentTypeAgent, AssignmentID: "agent-dev", Title: "Preview record", Status: StatusPending, CreatedBy: "user", CreatedAt: now, UpdatedAt: now}}}
	if err := writeJSONFileAtomic(filepath.Join(dir, tasksFileName), record); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(root); err != nil {
		t.Fatal(err)
	}
	assertCurrentTaskRecord(t, filepath.Join(dir, tasksFileName), 0)
}

func TestStoreCanonicalizesSequenceFromFlatChildTaskIDs(t *testing.T) {
	resetTaskIDAllocatorsForTest()
	root := t.TempDir()
	dir := filepath.Join(root, "task-5")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	record := taskRecord{
		SchemaVersion: currentTaskSchemaVersion,
		Tasks: []Task{
			{ID: "task-5", AssignmentType: AssignmentTypeRoom, AssignmentID: "room-a", Title: "Root", Status: StatusInProgress, CreatedBy: "manager", CreatedAt: now, UpdatedAt: now},
			{ID: "task-40", ParentID: "task-5", AssignmentType: AssignmentTypeRoom, AssignmentID: "room-a", Title: "Child", Status: StatusPending, CreatedBy: "manager", CreatedAt: now, UpdatedAt: now},
		},
	}
	if err := writeJSONFileAtomic(filepath.Join(dir, tasksFileName), record); err != nil {
		t.Fatal(err)
	}

	if _, err := NewStore(root); err != nil {
		t.Fatal(err)
	}
	assertTaskSequence(t, root, 40)
}

func TestStoreLeavesConflictingLegacyRecordUntouched(t *testing.T) {
	root := t.TempDir()
	currentDir := filepath.Join(root, "task-1")
	legacyDir := filepath.Join(root, "task-2")
	legacyScheduledDir := filepath.Join(root, "task-scheduled-run-1")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacyScheduledDir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	current := Snapshot{Root: Task{ID: "task-1", AssignmentType: AssignmentTypeAgent, AssignmentID: "new", Title: "New", Status: StatusPending, CreatedBy: "user", CreatedAt: now, UpdatedAt: now}}
	currentRecord, err := recordFromSnapshot(current)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFileAtomic(filepath.Join(currentDir, tasksFileName), currentRecord); err != nil {
		t.Fatal(err)
	}
	legacyRoot := Task{ID: "task-2", AssignmentType: AssignmentTypeTeam, AssignmentID: "old", Title: "Old", Status: StatusPending, CreatedBy: "manager", CreatedAt: now, UpdatedAt: now}
	legacyChild := Task{ID: "task-1", ParentID: legacyRoot.ID, AssignmentType: legacyRoot.AssignmentType, AssignmentID: legacyRoot.AssignmentID, Title: "Collision", Status: StatusPending, CreatedBy: "manager", CreatedAt: now, UpdatedAt: now}
	if err := writeJSONFile(filepath.Join(legacyDir, legacyRootFileName), legacyRoot); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(legacyDir, legacyChildrenFileName), []Task{legacyChild}); err != nil {
		t.Fatal(err)
	}
	legacyScheduled := Task{ID: "task-scheduled-run-1", AssignmentType: AssignmentTypeAgent, AssignmentID: "worker", Title: "Scheduled", Status: StatusInProgress, CreatedBy: "scheduler", CreatedAt: now, UpdatedAt: now}
	if err := writeJSONFile(filepath.Join(legacyScheduledDir, legacyRootFileName), legacyScheduled); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore() error = %v, want current records to remain usable", err)
	}
	if snapshot, err := store.LoadRoot("task-1"); err != nil || snapshot.Root.AssignmentID != "new" {
		t.Fatalf("current record = %+v, %v", snapshot, err)
	}
	if _, err := os.Stat(filepath.Join(legacyDir, tasksFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("conflicting legacy record was modified: %v", err)
	}
	if snapshot, err := store.LoadRoot(legacyScheduled.ID); err != nil || snapshot.Root.ID != legacyScheduled.ID {
		t.Fatalf("non-conflicting legacy record = %+v, %v", snapshot, err)
	}
}

func assertCurrentTaskRecord(t *testing.T, path string, eventSeq int64) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	var version int
	if err := json.Unmarshal(raw["schema_version"], &version); err != nil {
		t.Fatalf("schema_version missing: %v", err)
	}
	if version != currentTaskSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, currentTaskSchemaVersion)
	}
	var record taskRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.EventSeq != eventSeq {
		t.Fatalf("event seq = %d, want %d", record.EventSeq, eventSeq)
	}
}

func assertTaskSequence(t *testing.T, root string, want int64) {
	t.Helper()
	sequence, ok, err := readTaskSequence(filepath.Join(root, sequenceFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("task sequence does not exist")
	}
	if sequence.SchemaVersion != currentSequenceVersion {
		t.Fatalf("task sequence schema version = %d, want %d", sequence.SchemaVersion, currentSequenceVersion)
	}
	if sequence.LastTask != want {
		t.Fatalf("task sequence = %d, want %d", sequence.LastTask, want)
	}
}
