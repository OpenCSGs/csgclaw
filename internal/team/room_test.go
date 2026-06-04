package team

import (
	"strings"
	"testing"
)

func TestTaskExecutionRoomTitleIncludesTaskID(t *testing.T) {
	title := TaskExecutionRoomTitle(TeamTask{ID: "task-11", Title: "调研下周上海天气"})
	if !strings.Contains(title, "task-11") {
		t.Fatalf("title = %q, want task id", title)
	}
	if !strings.Contains(title, "调研") {
		t.Fatalf("title = %q, want task title fragment", title)
	}
}

func TestFindTeamByRoomMatchesExecutionRoom(t *testing.T) {
	svc := newTestService()
	meta, err := svc.CreateTeam(CreateTeamInput{
		ID:        "team-ops",
		RoomID:    "room-team-home",
		Channel:   "csgclaw",
		LeadBotID: "u-manager",
	})
	if err != nil {
		t.Fatalf("CreateTeam() error = %v", err)
	}
	parent, err := svc.CreateTask(CreateTaskInput{
		TeamID:    meta.ID,
		Title:     "Ship",
		CreatedBy: "u-manager",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := svc.PlanTask(PlanTaskInput{
		TeamID:  meta.ID,
		TaskID:  parent.ID,
		ActorID: "u-manager",
		Tasks: []PlanTaskItem{
			{Title: "Build", AssignTo: "u-alice"},
		},
	}); err != nil {
		t.Fatalf("PlanTask() error = %v", err)
	}
	if _, err := svc.StartTask(StartTaskInput{
		TeamID:     meta.ID,
		TaskID:     parent.ID,
		ActorID:    "web",
		TaskRoomID: "room-task-exec",
	}); err != nil {
		t.Fatalf("StartTask() error = %v", err)
	}

	if _, ok := svc.FindTeamByRoom("room-team-home"); !ok {
		t.Fatal("FindTeamByRoom(team home) = false, want true")
	}
	found, ok := svc.FindTeamByRoom("room-task-exec")
	if !ok {
		t.Fatal("FindTeamByRoom(task room) = false, want true")
	}
	if found.ID != meta.ID {
		t.Fatalf("FindTeamByRoom(task room).ID = %q, want %q", found.ID, meta.ID)
	}
}

func TestPlanTaskProjectsPlanningCompleteToBoundExecutionRoom(t *testing.T) {
	svc := newTestService()
	meta, err := svc.CreateTeam(CreateTeamInput{
		ID:        "team-ops",
		RoomID:    "room-team-home",
		Channel:   "csgclaw",
		LeadBotID: "u-manager",
	})
	if err != nil {
		t.Fatalf("CreateTeam() error = %v", err)
	}
	parent, err := svc.CreateTask(CreateTaskInput{
		TeamID:    meta.ID,
		Title:     "Ship",
		CreatedBy: "u-manager",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := svc.BindTaskExecutionRoom(BindTaskExecutionRoomInput{
		TeamID:     meta.ID,
		TaskID:     parent.ID,
		ActorID:    "web",
		TaskRoomID: "room-task-exec",
	}); err != nil {
		t.Fatalf("BindTaskExecutionRoom() error = %v", err)
	}
	if _, err := svc.PlanTask(PlanTaskInput{
		TeamID:      meta.ID,
		TaskID:      parent.ID,
		ActorID:     "u-manager",
		PlanSummary: "Split backend and frontend work.",
		Tasks: []PlanTaskItem{
			{Title: "Build", AssignTo: "u-alice"},
		},
	}); err != nil {
		t.Fatalf("PlanTask() error = %v", err)
	}

	events := svc.ListEvents(meta.ID)
	var planningRoomID string
	for _, event := range events {
		if event.Type == "task.planned" {
			planningRoomID = event.RoomID
			break
		}
	}
	if planningRoomID != "room-task-exec" {
		t.Fatalf("task.planned room = %q, want execution room", planningRoomID)
	}
	var childRoomID string
	for _, task := range svc.ListTasks(meta.ID) {
		if task.ParentID == parent.ID {
			childRoomID = task.RoomID
			break
		}
	}
	if childRoomID != "room-task-exec" {
		t.Fatalf("planned child room = %q, want execution room", childRoomID)
	}
}
