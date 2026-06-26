package team

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/im"
)

func TestProjectorProjectsBuiltInChannelMessages(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-manager",
		Users: []im.User{
			{ID: "u-manager", Name: "manager", Role: "manager"},
			{ID: "u-alice", Name: "alice", Role: "worker"},
		},
		Rooms: []im.Room{
			{ID: "room-ops", Title: "ops", Members: []string{"u-manager", "u-alice"}},
			{ID: "room-task", Title: "[task] ops", Members: []string{"u-manager", "u-alice"}},
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

	task, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Collect feedback",
		CreatedBy: "manager",
		AssignTo:  "alice",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := svc.BindTaskExecutionRoom(BindTaskExecutionRoomInput{TeamID: teamID, TaskID: task.ID, ActorID: "web", TaskRoomID: "room-task"}); err != nil {
		t.Fatalf("BindTaskExecutionRoom() error = %v", err)
	}
	if _, err := svc.ClaimTask(ClaimTaskInput{TeamID: teamID, TaskID: task.ID, ParticipantID: "alice"}); err != nil {
		t.Fatalf("ClaimTask() error = %v", err)
	}
	if _, err := svc.CompleteTask(CompleteTaskInput{
		TeamID:  teamID,
		TaskID:  task.ID,
		ActorID: "alice",
		Result:  "done",
	}); err != nil {
		t.Fatalf("CompleteTask() error = %v", err)
	}

	messages, err := imSvc.ListMessages("room-ops")
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	for _, message := range messages {
		if message.Kind == im.MessageKindEvent && message.Event != nil && message.Event.Key == "team_event" {
			t.Fatalf("team room should not receive task projection: %+v", message)
		}
	}

	var teamMessages []im.Message
	taskRoomMessages, err := imSvc.ListMessages("room-task")
	if err != nil {
		t.Fatalf("ListMessages(task room) error = %v", err)
	}
	for _, message := range taskRoomMessages {
		if message.Kind == im.MessageKindEvent && message.Event != nil && message.Event.Key == "team_event" {
			teamMessages = append(teamMessages, message)
		}
	}
	if len(teamMessages) != 2 {
		t.Fatalf("task room projection messages = %d, want 2", len(teamMessages))
	}
	for _, message := range teamMessages {
		if strings.Contains(message.Content, "[team]") || strings.Contains(message.Content, "[approval]") {
			t.Fatalf("team projection = %q, should not expose bracketed team prefixes", message.Content)
		}
	}
	if !strings.Contains(teamMessages[0].Content, "alice claimed "+task.ID) {
		t.Fatalf("claim projection = %q, want claimed task", teamMessages[0].Content)
	}
	if !strings.Contains(teamMessages[1].Content, "alice completed "+task.ID) {
		t.Fatalf("complete projection = %q, want completed task", teamMessages[1].Content)
	}
}

func TestProjectionFailureAppendsAuditEventWithoutBreakingTaskState(t *testing.T) {
	svc := NewService(
		WithProjector(NewProjector(failingAdapter{err: errors.New("send boom")}, nil)),
		WithNowFunc(sequenceNow(
			time.Date(2026, 5, 29, 17, 0, 0, 0, time.UTC),
			time.Date(2026, 5, 29, 17, 0, 1, 0, time.UTC),
			time.Date(2026, 5, 29, 17, 0, 2, 0, time.UTC),
			time.Date(2026, 5, 29, 17, 0, 3, 0, time.UTC),
		)),
	)
	teamID := createTestTeam(t, svc)

	task, err := svc.CreateTask(CreateTaskInput{
		TeamID:    teamID,
		Title:     "Projected task",
		CreatedBy: "manager",
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if task.ID == "" {
		t.Fatal("CreateTask().ID = empty")
	}
	if _, err := svc.BindTaskExecutionRoom(BindTaskExecutionRoomInput{TeamID: teamID, TaskID: task.ID, ActorID: "web", TaskRoomID: "room-task"}); err != nil {
		t.Fatalf("BindTaskExecutionRoom() error = %v", err)
	}
	if _, err := svc.RequestApproval(RequestApprovalInput{
		TeamID:      teamID,
		TaskID:      task.ID,
		RequestedBy: "manager",
		Kind:        "command",
		Summary:     "Run tests",
	}); err != nil {
		t.Fatalf("RequestApproval() error = %v", err)
	}

	events := svc.ListEvents(teamID)
	if len(events) != 5 {
		t.Fatalf("ListEvents() len = %d, want 5 including hidden main-room events and task-room projection failure", len(events))
	}
	if events[0].Type != EventTeamCreated {
		t.Fatalf("events[0].Type = %s, want team.created audit event", events[0].Type)
	}
	if events[1].Type != EventTaskCreated {
		t.Fatalf("events[1].Type = %s, want task.created", events[1].Type)
	}
	if events[2].Type != EventTaskExecutionRoom {
		t.Fatalf("events[2].Type = %s, want task.execution_room", events[2].Type)
	}
	if events[3].Type != EventApprovalRequested {
		t.Fatalf("events[3].Type = %s, want approval.requested", events[3].Type)
	}
	if events[4].Type != EventProjectionFailed {
		t.Fatalf("events[4].Type = %s, want projection.failed", events[4].Type)
	}
	if got, ok := svc.GetTask(teamID, task.ID); !ok || got.Status != TaskStatusPending {
		t.Fatalf("GetTask() = %+v, %v; want pending task preserved", got, ok)
	}
}

func TestProjectionSenderParticipantIDMapsWebActorToLead(t *testing.T) {
	if got := projectionSenderParticipantID("web", "manager"); got != "pt-manager" {
		t.Fatalf("projectionSenderParticipantID(web) = %q, want pt-manager", got)
	}
	if got := projectionSenderParticipantID("alice", "manager"); got != "pt-alice" {
		t.Fatalf("projectionSenderParticipantID(alice) = %q, want pt-alice", got)
	}
}

func TestProjectorRenderTaskLifecycleMessages(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-manager",
		Users: []im.User{
			{ID: "u-manager", Name: "manager", Role: "manager"},
			{ID: "u-backend-dev", Name: "backend-dev", Role: "worker"},
		},
		Rooms: []im.Room{
			{ID: "room-team", Title: "team", Members: []string{"u-manager", "u-backend-dev"}},
			{ID: "room-exec", Title: "[task-14] blog", Members: []string{"u-manager", "u-backend-dev"}},
		},
	})
	projector := NewProjector(NewCSGClawAdapter(imSvc), nil)
	meta := TeamMeta{
		ID:          "team-5",
		RoomID:      "room-team",
		Channel:     "csgclaw",
		LeadAgentID: agent.ManagerUserID,
	}
	events := []TeamEvent{
		{Seq: 1, TeamID: meta.ID, RoomID: "room-team", Type: EventTaskCreated, ActorID: "manager", TaskID: "task-15", TargetID: "backend-dev", Summary: "Implement API", CreatedAt: time.Now()},
		{Seq: 2, TeamID: meta.ID, RoomID: "room-exec", Type: EventTaskPlanned, ActorID: "manager", TaskID: "task-14", Summary: "Split backend and frontend work.", CreatedAt: time.Now()},
		{Seq: 3, TeamID: meta.ID, RoomID: "room-team", Type: EventTaskExecutionRoom, ActorID: "web", TaskID: "task-14", TargetID: "room-exec", Summary: "[task-14] blog", CreatedAt: time.Now()},
		{Seq: 4, TeamID: meta.ID, RoomID: "room-exec", Type: EventTaskDispatched, ActorID: "manager", TaskID: "task-15", TargetID: "backend-dev", Summary: "Implement API", CreatedAt: time.Now()},
		{Seq: 5, TeamID: meta.ID, RoomID: "room-exec", Type: EventTaskStarted, ActorID: "web", TaskID: "task-14", Summary: "Blog dev", CreatedAt: time.Now()},
	}

	if err := projector.Project(context.Background(), meta, events); err != nil {
		t.Fatalf("Project() error = %v", err)
	}

	teamMessages, err := imSvc.ListMessages("room-team")
	if err != nil {
		t.Fatalf("ListMessages(team) error = %v", err)
	}
	if len(teamMessages) != 0 {
		t.Fatalf("team room messages = %d, want 0 task projections: %+v", len(teamMessages), teamMessages)
	}

	execMessages, err := imSvc.ListMessages("room-exec")
	if err != nil {
		t.Fatalf("ListMessages(exec) error = %v", err)
	}
	if len(execMessages) != 2 {
		t.Fatalf("exec room messages = %d, want plan + dispatch messages", len(execMessages))
	}
	if execMessages[0].Kind != im.MessageKindEvent || !strings.Contains(execMessages[0].Content, "completed planning for task-14") || strings.Contains(execMessages[0].Content, "Split backend") {
		t.Fatalf("plan projection = %q, want concise planning complete message", execMessages[0].Content)
	}
	if strings.Contains(execMessages[1].Content, "started assigning tasks") {
		t.Fatalf("exec room should not include dispatch preamble: %q", execMessages[1].Content)
	}
	if execMessages[1].Kind != im.MessageKindMessage || !strings.Contains(execMessages[1].Content, "dispatched task-15") || strings.Contains(execMessages[1].Content, "dispatched task-15 to") || !strings.Contains(execMessages[1].Content, "claim --team team-5 --task task-15 --participant-id backend-dev") {
		t.Fatalf("dispatch projection = %q, want @mention task dispatch with claim command", execMessages[1].Content)
	}
	if len(execMessages[1].Mentions) != 1 || execMessages[1].Mentions[0].ID != "user-backend-dev" {
		t.Fatalf("dispatch mentions = %+v, want user-backend-dev mention", execMessages[1].Mentions)
	}
	if strings.Contains(execMessages[1].Content, "HTTP fallback") || strings.Contains(execMessages[1].Content, "Dispatched by") {
		t.Fatalf("dispatch projection = %q, should not include HTTP fallback or dispatched-by line", execMessages[1].Content)
	}
}

func TestProjectorSuccessorDispatchSkipsPreamble(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-manager",
		Users: []im.User{
			{ID: "u-manager", Name: "manager", Role: "manager"},
			{ID: "u-backend-dev", Name: "backend-dev", Role: "worker"},
		},
		Rooms: []im.Room{
			{ID: "room-team", Title: "team", Members: []string{"u-manager", "u-backend-dev"}},
			{ID: "room-exec", Title: "[task-14] blog", Members: []string{"u-manager", "u-backend-dev"}},
		},
	})
	projector := NewProjector(NewCSGClawAdapter(imSvc), nil)
	meta := TeamMeta{ID: "team-5", RoomID: "room-team", Channel: "csgclaw", LeadAgentID: agent.ManagerUserID}
	events := []TeamEvent{
		{Seq: 1, TeamID: meta.ID, RoomID: "room-exec", Type: EventTaskCompleted, ActorID: "backend-dev", TaskID: "task-15", Summary: "done", CreatedAt: time.Now()},
		{Seq: 2, TeamID: meta.ID, RoomID: "room-exec", Type: EventTaskDispatched, ActorID: "manager", TaskID: "task-16", TargetID: "backend-dev", Summary: "Verify API", CreatedAt: time.Now()},
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
			{ID: "u-manager", Name: "manager", Role: "manager"},
			{ID: "u-p-w-1512", Name: "p-w-1512", Role: "worker"},
		},
		Rooms: []im.Room{
			{ID: "room-team", Title: "team", Members: []string{"u-manager", "u-p-w-1512"}},
			{ID: "room-task", Title: "task", Members: []string{"u-manager", "u-p-w-1512"}},
		},
	})
	projector := NewProjector(NewCSGClawAdapter(imSvc), nil)
	meta := TeamMeta{
		ID:          "team-5",
		RoomID:      "room-team",
		Channel:     "csgclaw",
		LeadAgentID: agent.ManagerUserID,
	}
	events := []TeamEvent{
		{
			Seq:       1,
			TeamID:    meta.ID,
			RoomID:    "room-task",
			Type:      EventTaskClaimed,
			ActorID:   "p-w-1512",
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
	if messages[0].SenderID != "user-p-w-1512" {
		t.Fatalf("projected sender = %q, want user-p-w-1512", messages[0].SenderID)
	}
	if !strings.Contains(messages[0].Content, "p-w-1512 claimed task-22") {
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
