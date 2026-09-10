package taskcore

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServicePersistsRootChildAndEvents(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	svc := NewService(WithStore(store), WithNowFunc(func() time.Time { return now }))

	root, err := svc.CreateRoot(CreateRootInput{
		AssignmentType: AssignmentTypeTeam,
		AssignmentID:   "team-1",
		Title:          "Ship release",
		Body:           "Prepare launch",
		CreatedBy:      "pt-manager",
	})
	if err != nil {
		t.Fatalf("CreateRoot() error = %v", err)
	}
	child, err := svc.CreateChild(CreateChildInput{
		ParentID:   root.ID,
		Title:      "Draft note",
		CreatedBy:  "pt-manager",
		AssignedTo: "pt-writer",
	})
	if err != nil {
		t.Fatalf("CreateChild() error = %v", err)
	}
	if child.ParentID != root.ID || child.AssignmentType != AssignmentTypeTeam || child.AssignmentID != "team-1" {
		t.Fatalf("child assignment = %+v, want root assignment", child)
	}

	reloaded := NewService(WithStore(store))
	tasks := reloaded.ListByAssignment(AssignmentTypeTeam, "team-1")
	if len(tasks) != 2 {
		t.Fatalf("ListByAssignment() len = %d, want 2", len(tasks))
	}
	if events := reloaded.Events(root.ID); len(events) != 2 {
		t.Fatalf("Events() len = %d, want 2", len(events))
	}
}

func TestServiceClaimCompleteFailBlockApproval(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	svc := NewService(WithStore(store))
	task, err := svc.CreateRoot(CreateRootInput{
		AssignmentType: AssignmentTypeAgent,
		AssignmentID:   "agent-dev",
		Title:          "Fix bug",
		CreatedBy:      "user-admin",
		AssignedTo:     "pt-dev",
	})
	if err != nil {
		t.Fatalf("CreateRoot() error = %v", err)
	}

	claimed, err := svc.Claim(ClaimInput{TaskID: task.ID, ParticipantID: "pt-dev"})
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if claimed.Status != StatusInProgress || claimed.ClaimedBy != "pt-dev" {
		t.Fatalf("Claim() = %+v, want in_progress claimed by pt-dev", claimed)
	}
	approval, err := svc.RequestApproval(RequestApprovalInput{
		TaskID:      task.ID,
		RequestedBy: "pt-dev",
		ApproverID:  "pt-manager",
		Kind:        "command",
		Summary:     "run tests",
	})
	if err != nil {
		t.Fatalf("RequestApproval() error = %v", err)
	}
	resolved, err := svc.ResolveApproval(ResolveApprovalInput{
		ApprovalID: approval.ID,
		ApproverID: "pt-manager",
		Status:     ApprovalStatusApproved,
		Resolution: "ok",
	})
	if err != nil {
		t.Fatalf("ResolveApproval() error = %v", err)
	}
	if resolved.Status != ApprovalStatusApproved || resolved.ResolvedAt == nil {
		t.Fatalf("ResolveApproval() = %+v, want approved with resolved_at", resolved)
	}
	completed, err := svc.Complete(CompleteInput{TaskID: task.ID, ActorID: "pt-dev", Result: "done"})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if completed.Status != StatusCompleted || completed.Result != "done" || completed.CompletedAt == nil {
		t.Fatalf("Complete() = %+v, want completed result", completed)
	}

	failing, err := svc.CreateRoot(CreateRootInput{
		AssignmentType: AssignmentTypeAgent,
		AssignmentID:   "agent-dev",
		Title:          "Failing task",
		CreatedBy:      "user-admin",
	})
	if err != nil {
		t.Fatalf("CreateRoot(failing) error = %v", err)
	}
	if _, err := svc.Claim(ClaimInput{TaskID: failing.ID, ParticipantID: "pt-dev"}); err != nil {
		t.Fatalf("Claim(failing) error = %v", err)
	}
	blocked, err := svc.Block(BlockInput{TaskID: failing.ID, ActorID: "pt-dev", Reason: "need input"})
	if err != nil {
		t.Fatalf("Block() error = %v", err)
	}
	if blocked.Status != StatusBlocked || blocked.Error != "need input" {
		t.Fatalf("Block() = %+v, want blocked", blocked)
	}

	failedTask, err := svc.CreateRoot(CreateRootInput{
		AssignmentType: AssignmentTypeAgent,
		AssignmentID:   "agent-dev",
		Title:          "Another task",
		CreatedBy:      "user-admin",
	})
	if err != nil {
		t.Fatalf("CreateRoot(failedTask) error = %v", err)
	}
	if _, err := svc.Claim(ClaimInput{TaskID: failedTask.ID, ParticipantID: "pt-dev"}); err != nil {
		t.Fatalf("Claim(failedTask) error = %v", err)
	}
	failed, err := svc.Fail(FailInput{TaskID: failedTask.ID, ActorID: "pt-dev", Error: "boom"})
	if err != nil {
		t.Fatalf("Fail() error = %v", err)
	}
	if failed.Status != StatusFailed || failed.Error != "boom" {
		t.Fatalf("Fail() = %+v, want failed", failed)
	}
}

func TestStorePersistsFlatTaskRecords(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	svc := NewService(WithStore(store))
	task, err := svc.CreateRoot(CreateRootInput{
		AssignmentType: AssignmentTypeTeam,
		AssignmentID:   "team-1",
		Title:          "Task",
		CreatedBy:      "pt-manager",
	})
	if err != nil {
		t.Fatalf("CreateRoot() error = %v", err)
	}
	child, err := svc.CreateChild(CreateChildInput{
		ParentID:  task.ID,
		Title:     "Child",
		CreatedBy: "pt-manager",
	})
	if err != nil {
		t.Fatalf("CreateChild() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, task.ID, tasksFileName))
	if err != nil {
		t.Fatalf("read tasks record: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode tasks record: %v", err)
	}
	if _, found := raw["children"]; found {
		t.Fatal("task record contains a nested children field")
	}
	if _, found := raw["events"]; found {
		t.Fatal("task record embeds the append-only event journal")
	}
	var record taskRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("decode typed tasks record: %v", err)
	}
	if len(record.Tasks) != 2 || record.Tasks[1].ID != child.ID || record.Tasks[1].ParentID != task.ID {
		t.Fatalf("flat tasks = %+v", record.Tasks)
	}
	events, err := os.ReadFile(filepath.Join(root, task.ID, eventsFileName))
	if err != nil {
		t.Fatalf("read events journal: %v", err)
	}
	if lines := bytes.Count(events, []byte{'\n'}); lines != 2 {
		t.Fatalf("events.jsonl lines = %d, want 2", lines)
	}
}

func TestStoreIgnoresAndRepairsUncommittedEventTail(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	svc := NewService(WithStore(store))
	task, err := svc.CreateRoot(CreateRootInput{
		AssignmentType: AssignmentTypeAgent,
		AssignmentID:   "agent-dev",
		Title:          "Task",
		CreatedBy:      "user-admin",
	})
	if err != nil {
		t.Fatalf("CreateRoot() error = %v", err)
	}
	eventsPath := filepath.Join(root, task.ID, eventsFileName)
	committed, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read committed journal: %v", err)
	}
	file, err := os.OpenFile(eventsPath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open event journal: %v", err)
	}
	if _, err := file.WriteString(`{"seq":999,"type":"task.created"}` + "\n" + `{"seq":1000`); err != nil {
		file.Close()
		t.Fatalf("append uncommitted tail: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close event journal: %v", err)
	}

	snapshot, err := store.LoadRoot(task.ID)
	if err != nil {
		t.Fatalf("LoadRoot() with uncommitted tail error = %v", err)
	}
	if len(snapshot.Events) != 1 {
		t.Fatalf("LoadRoot() events = %d, want 1 committed event", len(snapshot.Events))
	}
	if err := store.SaveSnapshot(snapshot, nil); err != nil {
		t.Fatalf("SaveSnapshot() repair error = %v", err)
	}
	repaired, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read repaired journal: %v", err)
	}
	if !bytes.Equal(repaired, committed) {
		t.Fatalf("repaired journal = %q, want %q", repaired, committed)
	}
}

func TestTaskIDCounterPersistsAcrossDeleteAndReload(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	svc := NewService(WithStore(store))
	first, err := svc.CreateRoot(CreateRootInput{
		AssignmentType: AssignmentTypeAgent,
		AssignmentID:   "agent-dev",
		Title:          "First",
		CreatedBy:      "user-admin",
	})
	if err != nil {
		t.Fatalf("CreateRoot(first) error = %v", err)
	}
	second, err := svc.CreateRoot(CreateRootInput{
		AssignmentType: AssignmentTypeAgent,
		AssignmentID:   "agent-dev",
		Title:          "Second",
		CreatedBy:      "user-admin",
	})
	if err != nil {
		t.Fatalf("CreateRoot(second) error = %v", err)
	}
	if first.ID != "task-1" || second.ID != "task-2" {
		t.Fatalf("created task ids = %q, %q; want task-1, task-2", first.ID, second.ID)
	}
	if err := store.DeleteRoot(second.ID); err != nil {
		t.Fatalf("DeleteRoot(second) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "counters.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("counters.json exists after task writes: %v", err)
	}

	resetTaskIDAllocatorsForTest()
	reloadedStore, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore(reloaded) error = %v", err)
	}
	reloaded := NewService(WithStore(reloadedStore))
	third, err := reloaded.CreateRoot(CreateRootInput{
		AssignmentType: AssignmentTypeAgent,
		AssignmentID:   "agent-dev",
		Title:          "Third",
		CreatedBy:      "user-admin",
	})
	if err != nil {
		t.Fatalf("CreateRoot(third) error = %v", err)
	}
	if third.ID != "task-3" {
		t.Fatalf("CreateRoot(third).ID = %q, want task-3", third.ID)
	}
	sequence, ok, err := readTaskSequence(filepath.Join(root, sequenceFileName))
	if err != nil {
		t.Fatalf("readTaskSequence() error = %v", err)
	}
	if !ok {
		t.Fatal("readTaskSequence() ok = false, want true")
	}
	if sequence.LastTask != 3 {
		t.Fatalf("task sequence = %d, want 3", sequence.LastTask)
	}
}

func TestPersistentTaskIDsDoNotCollideWithLegacyLayout(t *testing.T) {
	resetTaskIDAllocatorsForTest()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "task-27"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(root, legacyTaskIndexFileName), map[string]any{
		"counters": map[string]any{"task": 41},
	}); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	task, err := NewService(WithStore(store)).CreateRoot(CreateRootInput{
		AssignmentType: AssignmentTypeAgent,
		AssignmentID:   "agent-dev",
		Title:          "New format task",
		CreatedBy:      "user-admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "task-42" {
		t.Fatalf("new task id = %q, want task-42", task.ID)
	}
	if _, err := os.Stat(filepath.Join(root, "task-42", tasksFileName)); err != nil {
		t.Fatalf("new-format task was not persisted separately: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "task-27", tasksFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy task directory was modified: %v", err)
	}
}

func resetTaskIDAllocatorsForTest() {
	taskIDAllocators.Lock()
	defer taskIDAllocators.Unlock()
	taskIDAllocators.byRoot = make(map[string]*TaskIDAllocator)
}

func TestServiceRejectsInvalidTransitions(t *testing.T) {
	svc := NewService()
	task, err := svc.CreateRoot(CreateRootInput{
		AssignmentType: AssignmentTypeAgent,
		AssignmentID:   "agent-dev",
		Title:          "Task",
		CreatedBy:      "user-admin",
	})
	if err != nil {
		t.Fatalf("CreateRoot() error = %v", err)
	}
	_, err = svc.Complete(CompleteInput{TaskID: task.ID, ActorID: "pt-dev", Result: "done"})
	if !errors.Is(err, ErrTransitionInvalid) {
		t.Fatalf("Complete() error = %v, want ErrTransitionInvalid", err)
	}
}
