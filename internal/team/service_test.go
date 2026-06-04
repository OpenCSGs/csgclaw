package team

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCreateTasksBatchIsAtomic(t *testing.T) {
	svc := newTestService()
	teamID := createTestTeam(t, svc)

	_, err := svc.CreateTasks(CreateTaskBatchInput{
		TeamID:    teamID,
		CreatedBy: "u-manager",
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
		CreatedBy: "u-manager",
		Tasks: []CreateTaskBatchItem{
			{IDRef: "collect", Title: "Collect feedback", AssignTo: "u-alice"},
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
		CreatedBy: "u-manager",
		Tasks: []CreateTaskBatchItem{
			{IDRef: "story", Title: "Release rollout"},
			{Title: "Draft note", ParentRef: "story", AssignTo: "u-alice"},
			{Title: "Smoke test", ParentRef: "story", AssignTo: "u-bob"},
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
		CreatedBy: "u-manager",
		AssignTo:  "u-bob",
		Priority:  10,
	}); err != nil {
		t.Fatalf("CreateTask() task A error = %v", err)
	}
	want, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Task B",
		CreatedBy: "u-manager",
		AssignTo:  "u-alice",
		Priority:  1,
	})
	if err != nil {
		t.Fatalf("CreateTask() task B error = %v", err)
	}

	got, err := svc.ClaimNext(teamID, "u-alice")
	if err != nil {
		t.Fatalf("ClaimNext() error = %v", err)
	}
	if got.ID != want.ID {
		t.Fatalf("ClaimNext() task = %s, want %s", got.ID, want.ID)
	}
}

func TestProvisionedWorkerClaimStoresTeamCanonicalID(t *testing.T) {
	svc := newTestService()
	teamID := createTestTeam(t, svc)

	task, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Task",
		CreatedBy: "u-manager",
		AssignTo:  "u-p-w-0604",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if task.AssignedTo != "u-p-w-0604" {
		t.Fatalf("CreateTask().AssignedTo = %q, want u-p-w-0604", task.AssignedTo)
	}

	claimed, err := svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: task.ID, BotID: "u-p-w-0604"})
	if err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}
	if claimed.ClaimedBy != "u-p-w-0604" {
		t.Fatalf("ClaimTask().ClaimedBy = %q, want u-p-w-0604", claimed.ClaimedBy)
	}
	presence, ok := svc.GetPresence(teamID, "u-p-w-0604")
	if !ok || presence.BotID != "u-p-w-0604" || presence.State != PresenceStateBusy {
		t.Fatalf("GetPresence(u-p-w-0604) = %+v, %v; want busy u-p-w-0604", presence, ok)
	}

	if _, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Next",
		CreatedBy: "u-manager",
	}); err != nil {
		t.Fatalf("CreateTask(next) error = %v", err)
	}
	if _, err := svc.ClaimNext(teamID, "u-p-w-0604"); !errors.Is(err, ErrWorkerAlreadyBusy) {
		t.Fatalf("ClaimNext() error = %v, want ErrWorkerAlreadyBusy", err)
	}
}

func TestClaimTaskRejectsIncompleteDependencies(t *testing.T) {
	svc := newTestService()
	teamID := createTestTeam(t, svc)

	dep, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Dependency",
		CreatedBy: "u-manager",
	})
	if err != nil {
		t.Fatalf("CreateTask() dependency error = %v", err)
	}
	task, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Blocked task",
		CreatedBy: "u-manager",
		DependsOn: []string{dep.ID},
	})
	if err != nil {
		t.Fatalf("CreateTask() blocked error = %v", err)
	}

	_, err = svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: task.ID, BotID: "u-alice"})
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
		CreatedBy: "u-manager",
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
				BotID:  fmt.Sprintf("u-worker-%d", i),
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
		CreatedBy: "u-manager",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := svc.CompleteTask(CompleteTaskInput{
		TeamID:  teamID,
		TaskID:  task.ID,
		ActorID: "u-manager",
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
		CreatedBy: "u-manager",
		AssignTo:  "u-alice",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: task.ID, BotID: "u-alice"}); err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}
	if _, err := svc.UpdateTaskStatus(UpdateTaskStatusInput{
		TeamID:  teamID,
		TaskID:  task.ID,
		ActorID: "u-alice",
		Status:  TaskStatusBlocked,
		Reason:  "need approval",
	}); err != nil {
		t.Fatalf("UpdateTaskStatus(blocked) error = %v", err)
	}

	approval, err := svc.RequestApproval(RequestApprovalInput{
		TeamID:      teamID,
		TaskID:      task.ID,
		RequestedBy: "u-alice",
		ApproverID:  "u-manager",
		Kind:        "command",
		Summary:     "Run integration tests",
	})
	if err != nil {
		t.Fatalf("RequestApproval() error = %v", err)
	}

	resolved, err := svc.ResolveApproval(ResolveApprovalInput{
		TeamID:     teamID,
		ApprovalID: approval.ID,
		ApproverID: "u-manager",
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
	presence, ok := svc.GetPresence(teamID, "u-alice")
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
		CreatedBy: "u-manager",
		AssignTo:  "u-alice",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: task.ID, BotID: "u-alice"}); err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}
	if _, err := svc.UpdateTaskStatus(UpdateTaskStatusInput{
		TeamID:  teamID,
		TaskID:  task.ID,
		ActorID: "u-alice",
		Status:  TaskStatusBlocked,
		Reason:  "need approval",
	}); err != nil {
		t.Fatalf("UpdateTaskStatus(blocked) error = %v", err)
	}
	approval, err := svc.RequestApproval(RequestApprovalInput{
		TeamID:      teamID,
		TaskID:      task.ID,
		RequestedBy: "u-alice",
		ApproverID:  "u-manager",
		Kind:        "command",
		Summary:     "Run risky command",
	})
	if err != nil {
		t.Fatalf("RequestApproval() error = %v", err)
	}

	if _, err := svc.ResolveApproval(ResolveApprovalInput{
		TeamID:     teamID,
		ApprovalID: approval.ID,
		ApproverID: "u-manager",
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

	first, err := svc.CreateTask(CreateTaskInput{TeamID: teamID, Title: "low", CreatedBy: "u-manager", Priority: 1})
	if err != nil {
		t.Fatalf("CreateTask() low error = %v", err)
	}
	second, err := svc.CreateTask(CreateTaskInput{TeamID: teamID, Title: "high", CreatedBy: "u-manager", Priority: 9})
	if err != nil {
		t.Fatalf("CreateTask() high error = %v", err)
	}
	third, err := svc.CreateTask(CreateTaskInput{TeamID: teamID, Title: "high later", CreatedBy: "u-manager", Priority: 9})
	if err != nil {
		t.Fatalf("CreateTask() high later error = %v", err)
	}

	got, err := svc.ClaimNext(teamID, "u-alice")
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
		LeadBotID: "u-manager",
	})
	if err != nil {
		t.Fatalf("CreateTeam(second) error = %v", err)
	}
	if _, err := svc.CreateTask(CreateTaskInput{
		TeamID:    firstTeamID,
		Title:     "lower priority",
		CreatedBy: "u-manager",
		Priority:  3,
	}); err != nil {
		t.Fatalf("CreateTask(first) error = %v", err)
	}
	want, err := svc.CreateTask(CreateTaskInput{
		TeamID:    secondTeam.ID,
		Title:     "higher priority",
		CreatedBy: "u-manager",
		Priority:  8,
	})
	if err != nil {
		t.Fatalf("CreateTask(second) error = %v", err)
	}

	got, err := svc.ClaimNext("", "u-alice")
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
		LeadBotID: "u-manager",
	})
	if err != nil {
		t.Fatalf("CreateTeam(second) error = %v", err)
	}
	if _, err := svc.CreateTask(CreateTaskInput{
		TeamID:    firstTeamID,
		Title:     "first",
		CreatedBy: "u-manager",
		Priority:  9,
	}); err != nil {
		t.Fatalf("CreateTask(first) error = %v", err)
	}
	if _, err := svc.CreateTask(CreateTaskInput{
		TeamID:    secondTeam.ID,
		Title:     "second",
		CreatedBy: "u-manager",
		Priority:  9,
	}); err != nil {
		t.Fatalf("CreateTask(second) error = %v", err)
	}

	if _, err := svc.ClaimNext("", "u-alice"); !errors.Is(err, ErrTeamSelectionRequired) {
		t.Fatalf("ClaimNext(global) error = %v, want ErrTeamSelectionRequired", err)
	}
}

func TestPlanTaskAppliesManagerPlan(t *testing.T) {
	svc := newTestService()
	teamID := createTestTeam(t, svc)
	parent, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Ship release",
		Body:      "Prepare the release package.",
		CreatedBy: "u-manager",
		AssignTo:  "u-manager",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	planned, err := svc.PlanTask(PlanTaskInput{
		TeamID:      teamID,
		TaskID:      parent.ID,
		ActorID:     "u-manager",
		PlanSummary: "Split by writing and verification responsibilities.",
		Tasks: []PlanTaskItem{
			{
				IDRef:    "draft",
				Title:    "Draft release note",
				Body:     "Goal: write the note\nAssignee reason: writer role\nDeliverable: final note",
				AssignTo: "u-alice",
				Priority: 9,
			},
			{
				IDRef:         "verify",
				Title:         "Verify checklist",
				Body:          "Goal: verify release\nAssignee reason: QA role\nDeliverable: checklist",
				AssignTo:      "u-bob",
				DependsOnRefs: []string{"draft"},
				Priority:      8,
			},
		},
	})
	if err != nil {
		t.Fatalf("PlanTask() error = %v", err)
	}
	if planned.Parent.Status != TaskStatusPending {
		t.Fatalf("planned parent status = %q, want %q", planned.Parent.Status, TaskStatusPending)
	}
	if planned.Parent.PlanSummary != "Split by writing and verification responsibilities." {
		t.Fatalf("planned summary = %q", planned.Parent.PlanSummary)
	}
	if len(planned.Tasks) != 2 {
		t.Fatalf("planned tasks len = %d, want 2", len(planned.Tasks))
	}
	if planned.Tasks[0].Status != TaskStatusPending || planned.Tasks[0].AssignedTo != "u-alice" || planned.Tasks[0].DispatchedAt != nil {
		t.Fatalf("first planned task = %+v, want pending assigned-but-not-dispatched", planned.Tasks[0])
	}
	if planned.Tasks[1].DependsOn[0] != planned.Tasks[0].ID {
		t.Fatalf("second task depends_on = %+v, want first task id %s", planned.Tasks[1].DependsOn, planned.Tasks[0].ID)
	}
}

func TestStartTaskDispatchesReadyChildrenAndCompletionDispatchesSuccessor(t *testing.T) {
	svc := newTestService()
	teamID := createTestTeam(t, svc)
	parent, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Ship release",
		CreatedBy: "u-manager",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	planned, err := svc.PlanTask(PlanTaskInput{
		TeamID:      teamID,
		TaskID:      parent.ID,
		ActorID:     "u-manager",
		PlanSummary: "Draft first, then verify.",
		Tasks: []PlanTaskItem{
			{IDRef: "draft", Title: "Draft release note", Body: "draft body", AssignTo: "u-alice", Priority: 9},
			{IDRef: "verify", Title: "Verify checklist", Body: "verify body", AssignTo: "u-bob", DependsOnRefs: []string{"draft"}, Priority: 8},
		},
	})
	if err != nil {
		t.Fatalf("PlanTask() error = %v", err)
	}

	if _, err := svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: planned.Tasks[0].ID, BotID: "u-alice"}); !errors.Is(err, ErrTaskNotClaimable) {
		t.Fatalf("ClaimTask(before start) error = %v, want ErrTaskNotClaimable", err)
	}

	started, err := svc.StartTask(StartTaskInput{
		TeamID:     teamID,
		TaskID:     parent.ID,
		ActorID:    "web",
		TaskRoomID: "room-task-exec",
	})
	if err != nil {
		t.Fatalf("StartTask() error = %v", err)
	}
	if started.Parent.Status != TaskStatusAssigned || started.ScheduledCount != 1 {
		t.Fatalf("StartTask() = %+v, want parent assigned and one scheduled child", started)
	}
	if started.Parent.RoomID != "room-task-exec" {
		t.Fatalf("parent.RoomID = %q, want dedicated execution room", started.Parent.RoomID)
	}
	draft, _ := svc.GetTask(teamID, planned.Tasks[0].ID)
	verify, _ := svc.GetTask(teamID, planned.Tasks[1].ID)
	if draft.Status != TaskStatusAssigned || draft.DispatchedAt == nil {
		t.Fatalf("draft after start = %+v, want assigned and dispatched", draft)
	}
	if draft.RoomID != "room-task-exec" {
		t.Fatalf("draft.RoomID = %q, want execution room bound from parent start", draft.RoomID)
	}
	if verify.Status != TaskStatusPending || verify.DispatchedAt != nil {
		t.Fatalf("verify after start = %+v, want pending and not dispatched", verify)
	}

	if _, err := svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: draft.ID, BotID: "u-alice"}); err != nil {
		t.Fatalf("ClaimTask(draft) error = %v", err)
	}
	if _, err := svc.CompleteTask(CompleteTaskInput{TeamID: teamID, TaskID: draft.ID, ActorID: "u-alice", Result: "draft ready"}); err != nil {
		t.Fatalf("CompleteTask(draft) error = %v", err)
	}
	verify, _ = svc.GetTask(teamID, verify.ID)
	if verify.Status != TaskStatusAssigned || verify.DispatchedAt == nil {
		t.Fatalf("verify after draft complete = %+v, want assigned and dispatched", verify)
	}
	if _, err := svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: verify.ID, BotID: "u-bob"}); err != nil {
		t.Fatalf("ClaimTask(verify) error = %v", err)
	}
	if _, err := svc.CompleteTask(CompleteTaskInput{TeamID: teamID, TaskID: verify.ID, ActorID: "u-bob", Result: "checklist passed"}); err != nil {
		t.Fatalf("CompleteTask(verify) error = %v", err)
	}
	updatedParent, _ := svc.GetTask(teamID, parent.ID)
	if updatedParent.Status != TaskStatusCompleted {
		t.Fatalf("parent status = %s, want %s", updatedParent.Status, TaskStatusCompleted)
	}
	if !strings.Contains(updatedParent.Result, "draft ready") || !strings.Contains(updatedParent.Result, "checklist passed") {
		t.Fatalf("parent result = %q, want aggregated child results", updatedParent.Result)
	}
}

func TestStartTaskDoesNotBindExecutionRoomWhenNoRunnableChildren(t *testing.T) {
	svc := newTestService()
	teamID := createTestTeam(t, svc)
	parent, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Ship release",
		CreatedBy: "u-manager",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	planned, err := svc.PlanTask(PlanTaskInput{
		TeamID:      teamID,
		TaskID:      parent.ID,
		ActorID:     "u-manager",
		PlanSummary: "Needs an assignee before dispatch.",
		Tasks: []PlanTaskItem{
			{IDRef: "draft", Title: "Draft release note"},
		},
	})
	if err != nil {
		t.Fatalf("PlanTask() error = %v", err)
	}

	if _, err := svc.StartTask(StartTaskInput{
		TeamID:     teamID,
		TaskID:     parent.ID,
		ActorID:    "web",
		TaskRoomID: "room-task-exec",
	}); !errors.Is(err, ErrTaskNotClaimable) {
		t.Fatalf("StartTask() error = %v, want ErrTaskNotClaimable", err)
	}

	storedParent, _ := svc.GetTask(teamID, parent.ID)
	storedChild, _ := svc.GetTask(teamID, planned.Tasks[0].ID)
	if storedParent.RoomID == "room-task-exec" || storedChild.RoomID == "room-task-exec" {
		t.Fatalf("execution room was bound on failed start: parent=%q child=%q", storedParent.RoomID, storedChild.RoomID)
	}
	for _, event := range svc.ListEvents(teamID) {
		if event.Type == "task.execution_room" || event.Type == "task.started" || event.Type == "task.dispatched" {
			t.Fatalf("event %s was appended on failed start", event.Type)
		}
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
		CreatedBy: "u-manager",
		AssignTo:  "u-alice",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: task.ID, BotID: "u-alice"}); err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}
	if _, err := svc.RequestApproval(RequestApprovalInput{
		TeamID:      teamID,
		TaskID:      task.ID,
		RequestedBy: "u-alice",
		ApproverID:  "u-manager",
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
		CreatedBy: "u-manager",
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
		CreatedBy: "u-manager",
		AssignTo:  "u-alice",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: task.ID, BotID: "u-alice"}); err != nil {
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
		BotID:  "u-alice",
		State:  PresenceStateIdle,
	}); err != nil {
		t.Fatalf("UpsertPresence(first) error = %v", err)
	}
	initialEvents := len(svc.ListEvents(teamID))
	if _, err := svc.UpsertPresence(UpsertPresenceInput{
		TeamID: teamID,
		BotID:  "u-alice",
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
	presence, ok := reloaded.GetPresence(teamID, "u-alice")
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
		LeadBotID: "u-manager",
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
