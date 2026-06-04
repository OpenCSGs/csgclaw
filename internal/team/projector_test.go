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
		CurrentUserID: "u-manager",
		Users: []im.User{
			{ID: "u-manager", Name: "manager", Handle: "manager", Role: "manager"},
			{ID: "u-alice", Name: "alice", Handle: "alice", Role: "worker"},
		},
		Rooms: []im.Room{
			{ID: "room-ops", Title: "ops", Members: []string{"u-manager", "u-alice"}},
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
		CreatedBy: "u-manager",
		Tasks: []CreateTaskBatchItem{
			{Title: "Collect feedback", AssignTo: "u-alice"},
			{Title: "Write report"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTasks() error = %v", err)
	}
	if _, err := svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: result.Tasks[0].ID, BotID: "u-alice"}); err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}
	if _, err := svc.CompleteTask(CompleteTaskInput{
		TeamID:  teamID,
		TaskID:  result.Tasks[0].ID,
		ActorID: "u-alice",
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
	if !strings.Contains(teamMessages[1], result.Tasks[0].ID+" Collect feedback -> u-alice") {
		t.Fatalf("batch projection = %q, want plain assignee", teamMessages[1])
	}
	if strings.Contains(teamMessages[1], "@u-alice") {
		t.Fatalf("batch projection = %q, should not mention assignee before dispatch", teamMessages[1])
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
		CreatedBy: "u-manager",
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

func TestProjectionSenderBotIDMapsWebActorToLead(t *testing.T) {
	if got := projectionSenderBotID("web", "u-manager"); got != "u-manager" {
		t.Fatalf("projectionSenderBotID(web) = %q, want u-manager", got)
	}
	if got := projectionSenderBotID("u-alice", "u-manager"); got != "u-alice" {
		t.Fatalf("projectionSenderBotID(u-alice) = %q, want u-alice", got)
	}
}

func TestProjectorRenderTaskLifecycleMessages(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-manager",
		Users: []im.User{
			{ID: "u-manager", Name: "manager", Handle: "manager", Role: "manager"},
			{ID: "u-backend-dev", Name: "backend-dev", Handle: "backend-dev", Role: "worker"},
		},
		Rooms: []im.Room{
			{ID: "room-team", Title: "team", Members: []string{"u-manager", "u-backend-dev"}},
			{ID: "room-exec", Title: "[task-14] blog", Members: []string{"u-manager", "u-backend-dev"}},
		},
	})
	projector := NewProjector(NewCSGClawAdapter(imSvc), nil)
	meta := TeamMeta{
		ID:        "team-5",
		RoomID:    "room-team",
		Channel:   "csgclaw",
		LeadBotID: "u-manager",
	}
	events := []TeamEvent{
		{Seq: 1, TeamID: meta.ID, RoomID: "room-team", Type: "task.created", ActorID: "u-manager", TaskID: "task-15", TargetID: "u-backend-dev", Summary: "Implement API", CreatedAt: time.Now()},
		{Seq: 2, TeamID: meta.ID, RoomID: "room-team", Type: "task.planned", ActorID: "u-manager", TaskID: "task-14", Summary: "Split backend and frontend work.", CreatedAt: time.Now()},
		{Seq: 3, TeamID: meta.ID, RoomID: "room-team", Type: "task.execution_room", ActorID: "web", TaskID: "task-14", TargetID: "room-exec", Summary: "[task-14] blog", CreatedAt: time.Now()},
		{Seq: 4, TeamID: meta.ID, RoomID: "room-exec", Type: "task.dispatched", ActorID: "u-manager", TaskID: "task-15", TargetID: "u-backend-dev", Summary: "Implement API", CreatedAt: time.Now()},
		{Seq: 5, TeamID: meta.ID, RoomID: "room-exec", Type: "task.started", ActorID: "web", TaskID: "task-14", Summary: "Blog dev", CreatedAt: time.Now()},
	}

	if err := projector.Project(context.Background(), meta, events); err != nil {
		t.Fatalf("Project() error = %v", err)
	}

	teamMessages, err := imSvc.ListMessages("room-team")
	if err != nil {
		t.Fatalf("ListMessages(team) error = %v", err)
	}
	if len(teamMessages) != 2 {
		t.Fatalf("team room messages = %d, want 2 (plan + execution room)", len(teamMessages))
	}
	if !strings.Contains(teamMessages[0].Content, "Task planning complete") || !strings.Contains(teamMessages[0].Content, "Split backend and frontend work.") {
		t.Fatalf("plan projection = %q, want planning complete summary", teamMessages[0].Content)
	}
	if !strings.Contains(teamMessages[1].Content, "Execution room created") {
		t.Fatalf("execution room projection = %q, want execution room notice", teamMessages[1].Content)
	}

	execMessages, err := imSvc.ListMessages("room-exec")
	if err != nil {
		t.Fatalf("ListMessages(exec) error = %v", err)
	}
	if len(execMessages) != 1 {
		t.Fatalf("exec room messages = %d, want 1 dispatch message", len(execMessages))
	}
	if strings.Contains(execMessages[0].Content, "started assigning tasks") {
		t.Fatalf("exec room should not include dispatch preamble: %q", execMessages[0].Content)
	}
	if !strings.Contains(execMessages[0].Content, "Task task-15 is ready for you") || !strings.Contains(execMessages[0].Content, "claim --team team-5 --task task-15 --bot-id u-backend-dev") {
		t.Fatalf("dispatch projection = %q, want worker claim instructions", execMessages[0].Content)
	}
	if strings.Contains(execMessages[0].Content, "HTTP fallback") || strings.Contains(execMessages[0].Content, "Dispatched by") {
		t.Fatalf("dispatch projection = %q, should not include HTTP fallback or dispatched-by line", execMessages[0].Content)
	}
}

func TestProjectorSuccessorDispatchSkipsPreamble(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-manager",
		Users: []im.User{
			{ID: "u-manager", Name: "manager", Handle: "manager", Role: "manager"},
			{ID: "u-backend-dev", Name: "backend-dev", Handle: "backend-dev", Role: "worker"},
		},
		Rooms: []im.Room{
			{ID: "room-exec", Title: "[task-14] blog", Members: []string{"u-manager", "u-backend-dev"}},
		},
	})
	projector := NewProjector(NewCSGClawAdapter(imSvc), nil)
	meta := TeamMeta{ID: "team-5", RoomID: "room-exec", Channel: "csgclaw", LeadBotID: "u-manager"}
	events := []TeamEvent{
		{Seq: 1, TeamID: meta.ID, RoomID: "room-exec", Type: "task.completed", ActorID: "u-backend-dev", TaskID: "task-15", Summary: "done", CreatedAt: time.Now()},
		{Seq: 2, TeamID: meta.ID, RoomID: "room-exec", Type: "task.dispatched", ActorID: "u-manager", TaskID: "task-16", TargetID: "u-backend-dev", Summary: "Verify API", CreatedAt: time.Now()},
	}
	if err := projector.Project(context.Background(), meta, events); err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	messages, err := imSvc.ListMessages("room-exec")
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want completed + successor dispatch only", len(messages))
	}
	if strings.Contains(messages[1].Content, "started assigning tasks") {
		t.Fatalf("successor dispatch should not include preamble: %q", messages[1].Content)
	}
}

func TestProjectorResolvesWorkerAliasSender(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-manager",
		Users: []im.User{
			{ID: "u-manager", Name: "manager", Handle: "manager", Role: "manager"},
			{ID: "u-p-w-1512", Name: "p-w-1512", Handle: "p-w-1512", Role: "worker"},
		},
		Rooms: []im.Room{
			{ID: "room-task", Title: "task", Members: []string{"u-manager", "u-p-w-1512"}},
		},
	})
	projector := NewProjector(NewCSGClawAdapter(imSvc), nil)
	meta := TeamMeta{
		ID:        "team-5",
		RoomID:    "room-task",
		Channel:   "csgclaw",
		LeadBotID: "u-manager",
	}
	events := []TeamEvent{
		{
			Seq:       1,
			TeamID:    meta.ID,
			RoomID:    "room-task",
			Type:      "task.claimed",
			ActorID:   "u-p-w-1512",
			TaskID:    "task-22",
			Summary:   "work",
			CreatedAt: time.Date(2026, 5, 29, 16, 0, 0, 0, time.UTC),
		},
	}

	if err := projector.Project(context.Background(), meta, events); err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	messages, err := imSvc.ListMessages("room-task")
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("ListMessages() len = %d, want 1", len(messages))
	}
	if messages[0].SenderID != "u-p-w-1512" {
		t.Fatalf("projected sender = %q, want u-p-w-1512", messages[0].SenderID)
	}
	if !strings.Contains(messages[0].Content, "claimed task-22") {
		t.Fatalf("projected content = %q, want claim message", messages[0].Content)
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
