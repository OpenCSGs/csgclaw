package team

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCreateTasksBatchIsAtomic(t *testing.T) {
	svc := newTestService()
	teamID := createTestTeam(t, svc)

	_, err := svc.CreateTasks(CreateTaskBatchInput{
		TeamID:    teamID,
		CreatedBy: "bot-manager",
		Tasks: []CreateTaskBatchItem{
			{IDRef: "a", Title: "Collect feedback"},
			{Title: "Analyze", DependsOnRefs: []string{"missing"}},
		},
	})
	if err == nil {
		t.Fatal("CreateTasks() error = nil, want invalid batch dependency")
	}

	if tasks := svc.ListTasks(teamID); len(tasks) != 0 {
		t.Fatalf("ListTasks() len = %d, want 0 after failed batch", len(tasks))
	}
}

func TestCreateTasksBatchResolvesIDRefs(t *testing.T) {
	svc := newTestService()
	teamID := createTestTeam(t, svc)

	result, err := svc.CreateTasks(CreateTaskBatchInput{
		TeamID:    teamID,
		CreatedBy: "bot-manager",
		Tasks: []CreateTaskBatchItem{
			{IDRef: "collect", Title: "Collect feedback", AssignTo: "bot-alice"},
			{IDRef: "analyze", Title: "Analyze", DependsOnRefs: []string{"collect"}},
			{Title: "Report", DependsOnRefs: []string{"analyze"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateTasks() error = %v", err)
	}
	if len(result.Tasks) != 3 {
		t.Fatalf("CreateTasks() tasks len = %d, want 3", len(result.Tasks))
	}
	if len(result.IDRefs) != 2 {
		t.Fatalf("CreateTasks() id_refs len = %d, want 2", len(result.IDRefs))
	}
	if result.Tasks[1].DependsOn[0] != result.Tasks[0].ID {
		t.Fatalf("task[1].DependsOn = %+v, want [%s]", result.Tasks[1].DependsOn, result.Tasks[0].ID)
	}
	if result.Tasks[2].DependsOn[0] != result.Tasks[1].ID {
		t.Fatalf("task[2].DependsOn = %+v, want [%s]", result.Tasks[2].DependsOn, result.Tasks[1].ID)
	}
}

func TestCreateTasksBatchResolvesParentRefs(t *testing.T) {
	svc := newTestService()
	teamID := createTestTeam(t, svc)

	result, err := svc.CreateTasks(CreateTaskBatchInput{
		TeamID:    teamID,
		CreatedBy: "bot-manager",
		Tasks: []CreateTaskBatchItem{
			{IDRef: "story", Title: "Release rollout"},
			{Title: "Draft note", ParentRef: "story", AssignTo: "bot-alice"},
			{Title: "Smoke test", ParentRef: "story", AssignTo: "bot-bob"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTasks() error = %v", err)
	}
	if len(result.Tasks) != 3 {
		t.Fatalf("CreateTasks() tasks len = %d, want 3", len(result.Tasks))
	}
	parentID := result.Tasks[0].ID
	if result.Tasks[1].ParentID != parentID {
		t.Fatalf("task[1].ParentID = %q, want %q", result.Tasks[1].ParentID, parentID)
	}
	if result.Tasks[2].ParentID != parentID {
		t.Fatalf("task[2].ParentID = %q, want %q", result.Tasks[2].ParentID, parentID)
	}
}

func TestClaimNextSkipsAssignedToOtherWorker(t *testing.T) {
	svc := newTestService()
	teamID := createTestTeam(t, svc)

	if _, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Task A",
		CreatedBy: "bot-manager",
		AssignTo:  "bot-bob",
		Priority:  10,
	}); err != nil {
		t.Fatalf("CreateTask() task A error = %v", err)
	}
	want, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Task B",
		CreatedBy: "bot-manager",
		AssignTo:  "bot-alice",
		Priority:  1,
	})
	if err != nil {
		t.Fatalf("CreateTask() task B error = %v", err)
	}

	got, err := svc.ClaimNext(teamID, "bot-alice")
	if err != nil {
		t.Fatalf("ClaimNext() error = %v", err)
	}
	if got.ID != want.ID {
		t.Fatalf("ClaimNext() task = %s, want %s", got.ID, want.ID)
	}
}

func TestClaimTaskRejectsIncompleteDependencies(t *testing.T) {
	svc := newTestService()
	teamID := createTestTeam(t, svc)

	dep, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Dependency",
		CreatedBy: "bot-manager",
	})
	if err != nil {
		t.Fatalf("CreateTask() dependency error = %v", err)
	}
	task, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Blocked task",
		CreatedBy: "bot-manager",
		DependsOn: []string{dep.ID},
	})
	if err != nil {
		t.Fatalf("CreateTask() blocked error = %v", err)
	}

	_, err = svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: task.ID, BotID: "bot-alice"})
	if !errors.Is(err, ErrTaskDependenciesOpen) {
		t.Fatalf("ClaimTask() error = %v, want ErrTaskDependenciesOpen", err)
	}
}

func TestConcurrentClaimAllowsOnlyOneWinner(t *testing.T) {
	svc := newTestService()
	teamID := createTestTeam(t, svc)

	task, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Single task",
		CreatedBy: "bot-manager",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	const workers = 8
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	successCh := make(chan TeamTask, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got, claimErr := svc.ClaimTask(ClaimTaskInput{
				TeamID: teamID,
				TaskID: task.ID,
				BotID:  fmt.Sprintf("bot-%d", i),
			})
			if claimErr != nil {
				errCh <- claimErr
				return
			}
			successCh <- got
		}(i)
	}
	wg.Wait()
	close(errCh)
	close(successCh)

	if len(successCh) != 1 {
		t.Fatalf("successful claims = %d, want 1", len(successCh))
	}
	for claimErr := range errCh {
		if !errors.Is(claimErr, ErrTaskNotClaimable) {
			t.Fatalf("claim error = %v, want ErrTaskNotClaimable", claimErr)
		}
	}

	stored, ok := svc.GetTask(teamID, task.ID)
	if !ok {
		t.Fatalf("GetTask(%s) found = false, want true", task.ID)
	}
	if stored.Status != TaskStatusInProgress {
		t.Fatalf("GetTask(%s).Status = %s, want %s", task.ID, stored.Status, TaskStatusInProgress)
	}
}

func TestIllegalTransitionsAreRejected(t *testing.T) {
	svc := newTestService()
	teamID := createTestTeam(t, svc)

	task, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Task",
		CreatedBy: "bot-manager",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := svc.CompleteTask(CompleteTaskInput{
		TeamID:  teamID,
		TaskID:  task.ID,
		ActorID: "bot-manager",
		Result:  "done",
	}); !errors.Is(err, ErrTaskTransitionInvalid) {
		t.Fatalf("CompleteTask() error = %v, want ErrTaskTransitionInvalid", err)
	}
}

func TestApprovalResolveMovesBlockedTaskBackToInProgress(t *testing.T) {
	svc := newTestService()
	teamID := createTestTeam(t, svc)

	task, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Task",
		CreatedBy: "bot-manager",
		AssignTo:  "bot-alice",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: task.ID, BotID: "bot-alice"}); err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}
	if _, err := svc.UpdateTaskStatus(UpdateTaskStatusInput{
		TeamID:  teamID,
		TaskID:  task.ID,
		ActorID: "bot-alice",
		Status:  TaskStatusBlocked,
		Reason:  "need approval",
	}); err != nil {
		t.Fatalf("UpdateTaskStatus(blocked) error = %v", err)
	}

	approval, err := svc.RequestApproval(RequestApprovalInput{
		TeamID:      teamID,
		TaskID:      task.ID,
		RequestedBy: "bot-alice",
		ApproverID:  "bot-manager",
		Kind:        "command",
		Summary:     "Run integration tests",
	})
	if err != nil {
		t.Fatalf("RequestApproval() error = %v", err)
	}

	resolved, err := svc.ResolveApproval(ResolveApprovalInput{
		TeamID:     teamID,
		ApprovalID: approval.ID,
		ApproverID: "bot-manager",
		Status:     ApprovalStatusApproved,
		Resolution: "approved",
	})
	if err != nil {
		t.Fatalf("ResolveApproval() error = %v", err)
	}
	if resolved.Status != ApprovalStatusApproved {
		t.Fatalf("ResolveApproval().Status = %s, want %s", resolved.Status, ApprovalStatusApproved)
	}

	stored, _ := svc.GetTask(teamID, task.ID)
	if stored.Status != TaskStatusInProgress {
		t.Fatalf("GetTask().Status = %s, want %s", stored.Status, TaskStatusInProgress)
	}
	presence, ok := svc.GetPresence(teamID, "bot-alice")
	if !ok {
		t.Fatal("GetPresence() found = false, want true")
	}
	if presence.State != PresenceStateBusy {
		t.Fatalf("GetPresence().State = %s, want %s", presence.State, PresenceStateBusy)
	}
}

func TestResolveRejectedApprovalKeepsTaskBlocked(t *testing.T) {
	svc := newTestService()
	teamID := createTestTeam(t, svc)

	task, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Task",
		CreatedBy: "bot-manager",
		AssignTo:  "bot-alice",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: task.ID, BotID: "bot-alice"}); err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}
	if _, err := svc.UpdateTaskStatus(UpdateTaskStatusInput{
		TeamID:  teamID,
		TaskID:  task.ID,
		ActorID: "bot-alice",
		Status:  TaskStatusBlocked,
		Reason:  "need approval",
	}); err != nil {
		t.Fatalf("UpdateTaskStatus(blocked) error = %v", err)
	}
	approval, err := svc.RequestApproval(RequestApprovalInput{
		TeamID:      teamID,
		TaskID:      task.ID,
		RequestedBy: "bot-alice",
		ApproverID:  "bot-manager",
		Kind:        "command",
		Summary:     "Run risky command",
	})
	if err != nil {
		t.Fatalf("RequestApproval() error = %v", err)
	}

	if _, err := svc.ResolveApproval(ResolveApprovalInput{
		TeamID:     teamID,
		ApprovalID: approval.ID,
		ApproverID: "bot-manager",
		Status:     ApprovalStatusRejected,
		Resolution: "do something else",
	}); err != nil {
		t.Fatalf("ResolveApproval() error = %v", err)
	}

	stored, _ := svc.GetTask(teamID, task.ID)
	if stored.Status != TaskStatusBlocked {
		t.Fatalf("GetTask().Status = %s, want %s", stored.Status, TaskStatusBlocked)
	}
}

func TestClaimNextUsesStablePriorityOrdering(t *testing.T) {
	svc := NewService(WithNowFunc(sequenceNow(
		time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 29, 12, 0, 1, 0, time.UTC),
		time.Date(2026, 5, 29, 12, 0, 2, 0, time.UTC),
		time.Date(2026, 5, 29, 12, 0, 3, 0, time.UTC),
	)))
	teamID := createTestTeam(t, svc)

	first, err := svc.CreateTask(CreateTaskInput{TeamID: teamID, Title: "low", CreatedBy: "bot-manager", Priority: 1})
	if err != nil {
		t.Fatalf("CreateTask() low error = %v", err)
	}
	second, err := svc.CreateTask(CreateTaskInput{TeamID: teamID, Title: "high", CreatedBy: "bot-manager", Priority: 9})
	if err != nil {
		t.Fatalf("CreateTask() high error = %v", err)
	}
	third, err := svc.CreateTask(CreateTaskInput{TeamID: teamID, Title: "high later", CreatedBy: "bot-manager", Priority: 9})
	if err != nil {
		t.Fatalf("CreateTask() high later error = %v", err)
	}

	got, err := svc.ClaimNext(teamID, "bot-alice")
	if err != nil {
		t.Fatalf("ClaimNext() error = %v", err)
	}
	if got.ID != second.ID {
		t.Fatalf("ClaimNext() = %s, want %s over %s and %s", got.ID, second.ID, first.ID, third.ID)
	}
}

func TestClaimNextAcrossTeamsUsesUniqueHighestPriority(t *testing.T) {
	svc := newTestService()
	firstTeamID := createTestTeam(t, svc)
	secondTeam, err := svc.CreateTeam(CreateTeamInput{
		ID:        "team-qa",
		RoomID:    "room-qa",
		Channel:   "csgclaw",
		Title:     "QA",
		LeadBotID: "bot-manager",
	})
	if err != nil {
		t.Fatalf("CreateTeam(second) error = %v", err)
	}
	if _, err := svc.CreateTask(CreateTaskInput{
		TeamID:    firstTeamID,
		Title:     "lower priority",
		CreatedBy: "bot-manager",
		Priority:  3,
	}); err != nil {
		t.Fatalf("CreateTask(first) error = %v", err)
	}
	want, err := svc.CreateTask(CreateTaskInput{
		TeamID:    secondTeam.ID,
		Title:     "higher priority",
		CreatedBy: "bot-manager",
		Priority:  8,
	})
	if err != nil {
		t.Fatalf("CreateTask(second) error = %v", err)
	}

	got, err := svc.ClaimNext("", "bot-alice")
	if err != nil {
		t.Fatalf("ClaimNext(global) error = %v", err)
	}
	if got.TeamID != secondTeam.ID || got.ID != want.ID {
		t.Fatalf("ClaimNext(global) = (%s,%s), want (%s,%s)", got.TeamID, got.ID, secondTeam.ID, want.ID)
	}
}

func TestClaimNextAcrossTeamsRequiresExplicitTeamOnPriorityTie(t *testing.T) {
	svc := newTestService()
	firstTeamID := createTestTeam(t, svc)
	secondTeam, err := svc.CreateTeam(CreateTeamInput{
		ID:        "team-qa",
		RoomID:    "room-qa",
		Channel:   "csgclaw",
		Title:     "QA",
		LeadBotID: "bot-manager",
	})
	if err != nil {
		t.Fatalf("CreateTeam(second) error = %v", err)
	}
	if _, err := svc.CreateTask(CreateTaskInput{
		TeamID:    firstTeamID,
		Title:     "first",
		CreatedBy: "bot-manager",
		Priority:  9,
	}); err != nil {
		t.Fatalf("CreateTask(first) error = %v", err)
	}
	if _, err := svc.CreateTask(CreateTaskInput{
		TeamID:    secondTeam.ID,
		Title:     "second",
		CreatedBy: "bot-manager",
		Priority:  9,
	}); err != nil {
		t.Fatalf("CreateTask(second) error = %v", err)
	}

	if _, err := svc.ClaimNext("", "bot-alice"); !errors.Is(err, ErrTeamSelectionRequired) {
		t.Fatalf("ClaimNext(global) error = %v, want ErrTeamSelectionRequired", err)
	}
}

func TestServicePersistsAndRecoversState(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	svc := NewService(
		WithStore(store),
		WithNowFunc(sequenceNow(
			time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC),
			time.Date(2026, 5, 29, 12, 0, 1, 0, time.UTC),
			time.Date(2026, 5, 29, 12, 0, 2, 0, time.UTC),
			time.Date(2026, 5, 29, 12, 0, 3, 0, time.UTC),
			time.Date(2026, 5, 29, 12, 0, 4, 0, time.UTC),
			time.Date(2026, 5, 29, 12, 0, 5, 0, time.UTC),
		)),
	)
	teamID := createTestTeam(t, svc)

	task, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Persisted task",
		CreatedBy: "bot-manager",
		AssignTo:  "bot-alice",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: task.ID, BotID: "bot-alice"}); err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}
	if _, err := svc.RequestApproval(RequestApprovalInput{
		TeamID:      teamID,
		TaskID:      task.ID,
		RequestedBy: "bot-alice",
		ApproverID:  "bot-manager",
		Kind:        "command",
		Summary:     "Need approval",
	}); err != nil {
		t.Fatalf("RequestApproval() error = %v", err)
	}

	reloaded := NewService(WithStore(store), WithNowFunc(sequenceNow(time.Date(2026, 5, 29, 12, 1, 0, 0, time.UTC))))
	gotTask, ok := reloaded.GetTask(teamID, task.ID)
	if !ok {
		t.Fatalf("GetTask(%s) found = false", task.ID)
	}
	if gotTask.Status != TaskStatusInProgress {
		t.Fatalf("GetTask().Status = %s, want %s", gotTask.Status, TaskStatusInProgress)
	}
	if approvals := reloaded.ListApprovals(teamID); len(approvals) != 1 {
		t.Fatalf("ListApprovals() len = %d, want 1", len(approvals))
	}
	if events := reloaded.ListEvents(teamID); len(events) < 4 {
		t.Fatalf("ListEvents() len = %d, want at least 4", len(events))
	}
}

func TestStoreTruncatesPartialFinalEvent(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	svc := NewService(WithStore(store), WithNowFunc(sequenceNow(time.Date(2026, 5, 29, 13, 0, 0, 0, time.UTC))))
	teamID := createTestTeam(t, svc)
	if _, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Task",
		CreatedBy: "bot-manager",
	}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	eventsPath := filepath.Join(root, teamID, eventsFileName)
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("ReadFile(events) error = %v", err)
	}
	data = append(data, []byte(`{"seq":999`)...)
	if err := os.WriteFile(eventsPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile(events) error = %v", err)
	}

	reloaded := NewService(WithStore(store), WithNowFunc(sequenceNow(time.Date(2026, 5, 29, 13, 1, 0, 0, time.UTC))))
	events := reloaded.ListEvents(teamID)
	if len(events) != 2 {
		t.Fatalf("ListEvents() len = %d, want 2", len(events))
	}
	trimmed, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("ReadFile(trimmed events) error = %v", err)
	}
	if bytes.Contains(trimmed, []byte(`{"seq":999`)) {
		t.Fatal("events.jsonl still contains truncated final line")
	}
}

func TestRecoverBlocksStaleInProgressTask(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	svc := NewService(
		WithStore(store),
		WithStaleTaskTTL(2*time.Minute),
		WithNowFunc(sequenceNow(
			time.Date(2026, 5, 29, 14, 0, 0, 0, time.UTC),
			time.Date(2026, 5, 29, 14, 0, 1, 0, time.UTC),
			time.Date(2026, 5, 29, 14, 0, 2, 0, time.UTC),
		)),
	)
	teamID := createTestTeam(t, svc)
	task, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Stale task",
		CreatedBy: "bot-manager",
		AssignTo:  "bot-alice",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: task.ID, BotID: "bot-alice"}); err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}

	reloaded := NewService(
		WithStore(store),
		WithStaleTaskTTL(2*time.Minute),
		WithNowFunc(sequenceNow(time.Date(2026, 5, 29, 14, 10, 0, 0, time.UTC))),
	)
	got, ok := reloaded.GetTask(teamID, task.ID)
	if !ok {
		t.Fatalf("GetTask(%s) found = false", task.ID)
	}
	if got.Status != TaskStatusBlocked {
		t.Fatalf("GetTask().Status = %s, want %s", got.Status, TaskStatusBlocked)
	}
	if got.Error == "" {
		t.Fatal("GetTask().Error = empty, want stale-task explanation")
	}
}

func TestPresenceHeartbeatCheckpointPersistsWithoutImmediateEvent(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	svc := NewService(
		WithStore(store),
		WithNowFunc(sequenceNow(
			time.Date(2026, 5, 29, 15, 0, 0, 0, time.UTC),
			time.Date(2026, 5, 29, 15, 0, 10, 0, time.UTC),
			time.Date(2026, 5, 29, 15, 0, 20, 0, time.UTC),
		)),
	)
	teamID := createTestTeam(t, svc)
	if _, err := svc.UpsertPresence(UpsertPresenceInput{
		TeamID: teamID,
		BotID:  "bot-alice",
		State:  PresenceStateIdle,
	}); err != nil {
		t.Fatalf("UpsertPresence(first) error = %v", err)
	}
	initialEvents := len(svc.ListEvents(teamID))
	if _, err := svc.UpsertPresence(UpsertPresenceInput{
		TeamID: teamID,
		BotID:  "bot-alice",
		State:  PresenceStateIdle,
	}); err != nil {
		t.Fatalf("UpsertPresence(heartbeat) error = %v", err)
	}
	if got := len(svc.ListEvents(teamID)); got != initialEvents {
		t.Fatalf("ListEvents() len = %d, want unchanged %d after heartbeat-only update", got, initialEvents)
	}
	if err := svc.CheckpointPresence(); err != nil {
		t.Fatalf("CheckpointPresence() error = %v", err)
	}

	reloaded := NewService(WithStore(store), WithNowFunc(sequenceNow(time.Date(2026, 5, 29, 15, 1, 0, 0, time.UTC))))
	presence, ok := reloaded.GetPresence(teamID, "bot-alice")
	if !ok {
		t.Fatal("GetPresence() found = false")
	}
	if !presence.LastHeartbeatAt.Equal(time.Date(2026, 5, 29, 15, 0, 20, 0, time.UTC)) {
		t.Fatalf("GetPresence().LastHeartbeatAt = %s, want latest checkpointed heartbeat", presence.LastHeartbeatAt)
	}
}

func newTestService() *Service {
	return NewService(WithNowFunc(sequenceNow(
		time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 29, 10, 0, 1, 0, time.UTC),
		time.Date(2026, 5, 29, 10, 0, 2, 0, time.UTC),
		time.Date(2026, 5, 29, 10, 0, 3, 0, time.UTC),
		time.Date(2026, 5, 29, 10, 0, 4, 0, time.UTC),
		time.Date(2026, 5, 29, 10, 0, 5, 0, time.UTC),
		time.Date(2026, 5, 29, 10, 0, 6, 0, time.UTC),
		time.Date(2026, 5, 29, 10, 0, 7, 0, time.UTC),
		time.Date(2026, 5, 29, 10, 0, 8, 0, time.UTC),
		time.Date(2026, 5, 29, 10, 0, 9, 0, time.UTC),
	)))
}

func createTestTeam(t *testing.T, svc *Service) string {
	t.Helper()

	team, err := svc.CreateTeam(CreateTeamInput{
		ID:        "team-ops",
		RoomID:    "room-ops",
		Channel:   "csgclaw",
		Title:     "Ops",
		LeadBotID: "bot-manager",
	})
	if err != nil {
		t.Fatalf("CreateTeam() error = %v", err)
	}
	return team.ID
}

func sequenceNow(times ...time.Time) func() time.Time {
	var mu sync.Mutex
	index := 0
	if len(times) == 0 {
		times = []time.Time{time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)}
	}
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		if index >= len(times) {
			return times[len(times)-1]
		}
		value := times[index]
		index++
		return value
	}
}
