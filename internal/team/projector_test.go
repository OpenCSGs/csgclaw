package team

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/im"
)

func TestProjectorProjectsBuiltInChannelMessages(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "bot-manager",
		Users: []im.User{
			{ID: "bot-manager", Name: "manager", Handle: "manager", Role: "manager"},
			{ID: "bot-alice", Name: "alice", Handle: "alice", Role: "worker"},
		},
		Rooms: []im.Room{
			{ID: "room-ops", Title: "ops", Members: []string{"bot-manager", "bot-alice"}},
		},
	})
	svc := NewService(
		WithProjector(NewProjector(NewCSGClawAdapter(imSvc), nil)),
		WithNowFunc(sequenceNow(
			time.Date(2026, 5, 29, 16, 0, 0, 0, time.UTC),
			time.Date(2026, 5, 29, 16, 0, 1, 0, time.UTC),
			time.Date(2026, 5, 29, 16, 0, 2, 0, time.UTC),
			time.Date(2026, 5, 29, 16, 0, 3, 0, time.UTC),
			time.Date(2026, 5, 29, 16, 0, 4, 0, time.UTC),
			time.Date(2026, 5, 29, 16, 0, 5, 0, time.UTC),
		)),
	)
	teamID := createTestTeam(t, svc)

	result, err := svc.CreateTasks(CreateTaskBatchInput{
		TeamID:    teamID,
		CreatedBy: "bot-manager",
		Tasks: []CreateTaskBatchItem{
			{Title: "Collect feedback"},
			{Title: "Write report"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTasks() error = %v", err)
	}
	if _, err := svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: result.Tasks[0].ID, BotID: "bot-alice"}); err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}
	if _, err := svc.CompleteTask(CompleteTaskInput{
		TeamID:  teamID,
		TaskID:  result.Tasks[0].ID,
		ActorID: "bot-alice",
		Result:  "done",
	}); err != nil {
		t.Fatalf("CompleteTask() error = %v", err)
	}

	messages, err := imSvc.ListMessages("room-ops")
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}

	var teamMessages []string
	for _, message := range messages {
		if strings.HasPrefix(message.Content, "[team]") {
			teamMessages = append(teamMessages, message.Content)
		}
	}
	if len(teamMessages) != 4 {
		t.Fatalf("team projection messages = %d, want 4", len(teamMessages))
	}
	if !strings.Contains(teamMessages[0], "Team enabled") {
		t.Fatalf("team projection = %q, want team enabled message", teamMessages[0])
	}
	if !strings.Contains(teamMessages[1], "created 2 tasks") {
		t.Fatalf("batch projection = %q, want batch summary", teamMessages[1])
	}
	if !strings.Contains(teamMessages[1], "\n- "+result.Tasks[0].ID) {
		t.Fatalf("batch projection = %q, want task list", teamMessages[1])
	}
	if !strings.Contains(teamMessages[2], "claimed "+result.Tasks[0].ID) {
		t.Fatalf("claim projection = %q, want claimed task", teamMessages[2])
	}
	if !strings.Contains(teamMessages[3], "completed "+result.Tasks[0].ID) {
		t.Fatalf("complete projection = %q, want completed task", teamMessages[3])
	}
}

func TestProjectionFailureAppendsAuditEventWithoutBreakingTaskState(t *testing.T) {
	svc := NewService(
		WithProjector(NewProjector(failingAdapter{err: errors.New("send boom")}, nil)),
		WithNowFunc(sequenceNow(
			time.Date(2026, 5, 29, 17, 0, 0, 0, time.UTC),
			time.Date(2026, 5, 29, 17, 0, 1, 0, time.UTC),
			time.Date(2026, 5, 29, 17, 0, 2, 0, time.UTC),
		)),
	)
	teamID := createTestTeam(t, svc)

	task, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Projected task",
		CreatedBy: "bot-manager",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if task.ID == "" {
		t.Fatal("CreateTask().ID = empty")
	}

	events := svc.ListEvents(teamID)
	if len(events) != 4 {
		t.Fatalf("ListEvents() len = %d, want 4 including projection.failed events", len(events))
	}
	if events[1].Type != "projection.failed" {
		t.Fatalf("events[1].Type = %s, want projection.failed for team.created", events[1].Type)
	}
	if events[2].Type != "task.created" {
		t.Fatalf("events[2].Type = %s, want task.created", events[2].Type)
	}
	if events[3].Type != "projection.failed" {
		t.Fatalf("events[3].Type = %s, want projection.failed", events[3].Type)
	}
	if got, ok := svc.GetTask(teamID, task.ID); !ok || got.Status != TaskStatusPending {
		t.Fatalf("GetTask() = %+v, %v; want pending task preserved", got, ok)
	}
}

type failingAdapter struct {
	err error
}

func (a failingAdapter) Channel() string {
	return builtinChannelName
}

func (a failingAdapter) EnsureRoom(context.Context, EnsureRoomRequest) (RoomRef, error) {
	return RoomRef{Channel: builtinChannelName, RoomID: "room-ops"}, nil
}

func (a failingAdapter) AddMembers(context.Context, AddMembersRequest) error {
	return nil
}

func (a failingAdapter) SendMessage(context.Context, SendMessageRequest) (MessageRef, error) {
	return MessageRef{}, a.err
}
