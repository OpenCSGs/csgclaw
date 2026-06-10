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

	meta, err := svc.CreateTeamWithRoom(context.Background(), adapter, CreateTeamWithRoomInput{
		Title:          "release",
		LeadAgentID:    agent.ManagerUserID,
		MemberAgentIDs: []string{"u-worker"},
	})
	if err != nil {
		t.Fatalf("CreateTeamWithRoom() error = %v", err)
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
	if parent.RoomID == "" || parent.RoomID == meta.RoomID {
		t.Fatalf("parent room = %q, want dedicated execution room distinct from team room %q", parent.RoomID, meta.RoomID)
	}
	if child.RoomID != parent.RoomID {
		t.Fatalf("child room = %q, want parent execution room %q", child.RoomID, parent.RoomID)
	}
	if _, ok := imSvc.Room(parent.RoomID); !ok {
		t.Fatalf("Room(%q) ok = false, want true", parent.RoomID)
	}

	events := svc.ListEvents(meta.ID)
	foundExecutionRoomEvent := false
	for _, event := range events {
		if event.Type == EventTaskExecutionRoom && event.TaskID == parent.ID {
			foundExecutionRoomEvent = true
			break
		}
	}
	if !foundExecutionRoomEvent {
		t.Fatal("missing task.execution_room event after create")
	}
}

func TestCreateTaskWithExecutionRoomSkipsChildTasks(t *testing.T) {
	imSvc := im.NewService()
	adapter := NewCSGClawAdapter(imSvc)
	svc := NewService()

	meta, err := svc.CreateTeamWithRoom(context.Background(), adapter, CreateTeamWithRoomInput{
		Title:       "release",
		LeadAgentID: agent.ManagerUserID,
	})
	if err != nil {
		t.Fatalf("CreateTeamWithRoom() error = %v", err)
	}

	parent, err := svc.CreateTaskWithExecutionRoom(context.Background(), CreateTaskInput{
		TeamID:    meta.ID,
		Title:     "Ship release",
		CreatedBy: "manager",
	}, adapter, NewCSGClawTeamDirectory(imSvc, nil))
	if err != nil {
		t.Fatalf("CreateTaskWithExecutionRoom(parent) error = %v", err)
	}
	if parent.RoomID == "" || parent.RoomID == meta.RoomID {
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
