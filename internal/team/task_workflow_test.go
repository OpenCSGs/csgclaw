package team

import (
	"context"
	"testing"

	"csgclaw/internal/agent"
	"csgclaw/internal/im"
)

func TestCreateTasksWithExecutionRoomBindsParentOnCreate(t *testing.T) {
	imSvc := im.NewService()
	adapter := NewCSGClawAdapter(imSvc)
	svc := NewService()

	meta, err := svc.CreateTeamWithMembers(CreateTeamWithMembersInput{
		Title:          "release",
		LeadAgentID:    agent.ManagerUserID,
		MemberAgentIDs: []string{"u-worker"},
	})
	if err != nil {
		t.Fatalf("CreateTeamWithMembers() error = %v", err)
	}

	result, err := svc.CreateTasksWithExecutionRoom(context.Background(), CreateTaskBatchInput{
		TeamID:    meta.ID,
		CreatedBy: "manager",
		Tasks: []CreateTaskBatchItem{
			{IDRef: "parent", Title: "Ship release"},
			{Title: "Draft release note", ParentRef: "parent", AssignTo: "worker"},
		},
	}, adapter, NewCSGClawTeamDirectory(imSvc, nil))
	if err != nil {
		t.Fatalf("CreateTasksWithExecutionRoom() error = %v", err)
	}
	if len(result.Tasks) != 2 {
		t.Fatalf("tasks len = %d, want 2", len(result.Tasks))
	}

	parent := result.Tasks[0]
	child := result.Tasks[1]
	if parent.Status != TaskStatusInProgress {
		t.Fatalf("parent status = %q, want %q after auto-start", parent.Status, TaskStatusInProgress)
	}
	if parent.RoomID == "" {
		t.Fatalf("parent room = %q, want execution room", parent.RoomID)
	}
	if child.RoomID != parent.RoomID {
		t.Fatalf("child room = %q, want parent execution room %q", child.RoomID, parent.RoomID)
	}
	if child.Status != TaskStatusAssigned || child.DispatchedAt == nil {
		t.Fatalf("child after create batch = %+v, want assigned and dispatched", child)
	}
	if _, ok := imSvc.Room(parent.RoomID); !ok {
		t.Fatalf("Room(%q) ok = false, want true", parent.RoomID)
	}

	events := svc.ListEvents(meta.ID)
	foundExecutionRoomEvent := false
	foundDispatchEvent := false
	for _, event := range events {
		if event.Type == EventTaskExecutionRoom && event.TaskID == parent.ID {
			foundExecutionRoomEvent = true
		}
		if event.Type == EventTaskDispatched && event.TaskID == child.ID {
			foundDispatchEvent = true
		}
	}
	if !foundExecutionRoomEvent {
		t.Fatal("missing task.execution_room event after create")
	}
	if !foundDispatchEvent {
		t.Fatal("missing task.dispatched event after create auto-start")
	}
}

func TestCreateTasksWithExecutionRoomSkipsAutoStartWhenParentDependenciesOpen(t *testing.T) {
	imSvc := im.NewService()
	adapter := NewCSGClawAdapter(imSvc)
	svc := NewService()

	meta, err := svc.CreateTeamWithMembers(CreateTeamWithMembersInput{
		Title:          "release",
		LeadAgentID:    agent.ManagerUserID,
		MemberAgentIDs: []string{"u-worker"},
	})
	if err != nil {
		t.Fatalf("CreateTeamWithMembers() error = %v", err)
	}

	result, err := svc.CreateTasksWithExecutionRoom(context.Background(), CreateTaskBatchInput{
		TeamID:    meta.ID,
		CreatedBy: "manager",
		Tasks: []CreateTaskBatchItem{
			{IDRef: "a", Title: "Prepare A"},
			{Title: "Build A", ParentRef: "a", AssignTo: "worker"},
			{IDRef: "b", Title: "Prepare B", DependsOnRefs: []string{"a"}},
			{Title: "Build B", ParentRef: "b", AssignTo: "worker"},
		},
	}, adapter, NewCSGClawTeamDirectory(imSvc, nil))
	if err != nil {
		t.Fatalf("CreateTasksWithExecutionRoom() error = %v", err)
	}
	if len(result.Tasks) != 4 {
		t.Fatalf("tasks len = %d, want 4", len(result.Tasks))
	}

	parentA := result.Tasks[0]
	childA := result.Tasks[1]
	parentB := result.Tasks[2]
	childB := result.Tasks[3]
	if parentA.Status != TaskStatusInProgress || childA.DispatchedAt == nil {
		t.Fatalf("A after create batch: parent=%+v child=%+v, want auto-started and dispatched", parentA, childA)
	}
	if parentB.Status != TaskStatusPending {
		t.Fatalf("parent B status = %q, want %q while dependency is open", parentB.Status, TaskStatusPending)
	}
	if childB.DispatchedAt != nil {
		t.Fatalf("child B dispatched_at = %v, want nil while parent dependency is open", childB.DispatchedAt)
	}
	if len(parentB.DependsOn) != 1 || parentB.DependsOn[0] != parentA.ID {
		t.Fatalf("parent B depends_on = %+v, want [%s]", parentB.DependsOn, parentA.ID)
	}
}

func TestCreateTaskWithExecutionRoomSkipsChildTasks(t *testing.T) {
	imSvc := im.NewService()
	adapter := NewCSGClawAdapter(imSvc)
	svc := NewService()

	meta, err := svc.CreateTeamWithMembers(CreateTeamWithMembersInput{
		Title:       "release",
		LeadAgentID: agent.ManagerUserID,
	})
	if err != nil {
		t.Fatalf("CreateTeamWithMembers() error = %v", err)
	}

	parent, err := svc.CreateTaskWithExecutionRoom(context.Background(), CreateTaskInput{
		TeamID:    meta.ID,
		Title:     "Ship release",
		CreatedBy: "manager",
	}, adapter, NewCSGClawTeamDirectory(imSvc, nil))
	if err != nil {
		t.Fatalf("CreateTaskWithExecutionRoom(parent) error = %v", err)
	}
	if parent.RoomID == "" {
		t.Fatalf("parent room = %q, want dedicated execution room", parent.RoomID)
	}

	child, err := svc.CreateTaskWithExecutionRoom(context.Background(), CreateTaskInput{
		TeamID:    meta.ID,
		ParentID:  parent.ID,
		Title:     "Draft release note",
		CreatedBy: "manager",
		AssignTo:  "worker",
	}, adapter, NewCSGClawTeamDirectory(imSvc, nil))
	if err != nil {
		t.Fatalf("CreateTaskWithExecutionRoom(child) error = %v", err)
	}
	if child.RoomID != parent.RoomID {
		t.Fatalf("child room = %q, want inherited execution room %q", child.RoomID, parent.RoomID)
	}
}

func TestStartTaskWithExecutionRoomUsesTeamLeadActor(t *testing.T) {
	imSvc := im.NewService()
	adapter := NewCSGClawAdapter(imSvc)
	svc := NewService()

	meta, err := svc.CreateTeamWithMembers(CreateTeamWithMembersInput{
		Title:          "release",
		LeadAgentID:    agent.ManagerUserID,
		MemberAgentIDs: []string{"u-worker"},
	})
	if err != nil {
		t.Fatalf("CreateTeamWithMembers() error = %v", err)
	}
	parent, err := svc.CreateTask(CreateTaskInput{
		TeamID:    meta.ID,
		Title:     "Ship release",
		CreatedBy: "manager",
	})
	if err != nil {
		t.Fatalf("CreateTask(parent) error = %v", err)
	}
	child, err := svc.CreateTask(CreateTaskInput{
		TeamID:    meta.ID,
		ParentID:  parent.ID,
		Title:     "Draft release note",
		CreatedBy: "manager",
		AssignTo:  "worker",
	})
	if err != nil {
		t.Fatalf("CreateTask(child) error = %v", err)
	}

	if _, err := svc.StartTaskWithExecutionRoom(context.Background(), StartTaskWithExecutionRoomInput{
		TeamID:  meta.ID,
		TaskID:  parent.ID,
		ActorID: "worker",
	}, adapter, NewCSGClawTeamDirectory(imSvc, nil)); err != nil {
		t.Fatalf("StartTaskWithExecutionRoom() error = %v", err)
	}

	foundStarted := false
	foundDispatched := false
	for _, event := range svc.ListEvents(meta.ID) {
		if (event.Type == EventTaskStarted && event.TaskID == parent.ID) ||
			(event.Type == EventTaskDispatched && event.TaskID == child.ID) {
			if event.ActorID != agent.ManagerParticipantID {
				t.Fatalf("%s actor = %q, want manager", event.Type, event.ActorID)
			}
			if event.Type == EventTaskStarted {
				foundStarted = true
			}
			if event.Type == EventTaskDispatched {
				foundDispatched = true
			}
		}
	}
	if !foundStarted || !foundDispatched {
		t.Fatalf("events found: started=%v dispatched=%v, want both", foundStarted, foundDispatched)
	}
}
