package delivery

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/channel"
	"csgclaw/internal/im"
	"csgclaw/internal/taskconfig"
)

type titleTestProfiles struct{}

func (titleTestProfiles) ResolvedAgentProfile(string) (agent.AgentProfile, error) {
	return agent.AgentProfile{}, errors.New("expected task model override")
}

func (titleTestProfiles) ResolvedTaskProfile(_ string, cfg taskconfig.Config) (agent.AgentProfile, error) {
	return agent.AgentProfile{ModelID: cfg.ModelID}, nil
}

type titleTestModel func(context.Context, string, agent.AgentProfile, string, string) (string, error)

func (fn titleTestModel) GenerateTaskTitle(ctx context.Context, agentID string, profile agent.AgentProfile, question, answer string) (string, error) {
	return fn(ctx, agentID, profile, question, answer)
}

func TestTaskTitleRetriesAttachmentOnlyExchangeAfterTimeout(t *testing.T) {
	bus := im.NewBus()
	rooms, err := im.NewServiceFromPathWithBus(filepath.Join(t.TempDir(), "state.json"), bus)
	if err != nil {
		t.Fatal(err)
	}
	worker, _, err := rooms.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "user-worker", Name: "Worker", Role: "worker", SkipDirectRoom: true})
	if err != nil {
		t.Fatal(err)
	}
	room, err := rooms.CreateRoom(im.CreateRoomRequest{
		Title: "First question", CreatorID: im.AdminUserID, MemberIDs: []string{worker.ID},
		IsDirect: true, TaskSession: true, AutoTitle: true,
		Configuration: &taskconfig.Config{ModelProviderID: "local", ModelID: "selected-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := rooms.CreateMessage(im.CreateMessageRequest{
		RoomID: room.ID, SenderID: im.AdminUserID, ClientMessageID: "first", Content: "First question",
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := rooms.CreateMessage(im.CreateMessageRequest{
		RoomID: room.ID, SenderID: im.AdminUserID, ClientMessageID: "second",
		Attachments: []im.MessageAttachmentUpload{{Name: "login.txt", Data: []byte("request deadline exceeded")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	followUp, err := rooms.CreateMessage(im.CreateMessageRequest{
		RoomID: room.ID, SenderID: im.AdminUserID, ClientMessageID: "third", Content: "What is the deadline?",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started := make(chan struct{})
	unblock := make(chan struct{})
	var attempts atomic.Int32
	model := titleTestModel(func(ctx context.Context, agentID string, profile agent.AgentProfile, question, answer string) (string, error) {
		if agentID != "worker" || profile.ModelID != "selected-model" || question != "" || answer != "The login request exceeds the deadline." {
			t.Error("unexpected title request")
		}
		if attempts.Add(1) == 1 {
			close(started)
			select {
			case <-unblock:
				return "", context.DeadlineExceeded
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		return "Investigate login timeout", nil
	})
	renderer := NewTranscriptRenderer(&recordingStore{}, WithTaskTitleCallback(NewTaskTitleCallback(ctx, rooms, titleTestProfiles{}, model)))
	turn := channel.TurnContext{AgentTurn: true, RoomID: room.ID, ChatType: "direct", SourceMessageID: first.ID, AgentID: "worker", TurnID: "first"}
	if err := renderer.Complete(ctx, turn, agentengine.TurnResult{Status: agentengine.TurnFailed}); err != nil {
		t.Fatal(err)
	}
	if rooms.TaskTitlePending(room.ID, first.ID) {
		t.Fatal("failed reply claimed the title")
	}
	events, unsubscribe := bus.Subscribe()
	defer unsubscribe()
	turn.SourceMessageID, turn.TurnID = source.ID, "second"
	completed := make(chan error, 1)
	go func() {
		completed <- renderer.Complete(ctx, turn, agentengine.TurnResult{
			Status: agentengine.TurnSucceeded, Output: "The login request exceeds the deadline.",
		})
	}()
	select {
	case err := <-completed:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("completion waited for the title model")
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("title model did not start")
	}
	if current, _ := rooms.Room(room.ID); current.Title != "First question" {
		t.Fatalf("title while model is running = %q", current.Title)
	}
	if err := rooms.Reload(); err != nil {
		t.Fatal(err)
	}
	if !rooms.TaskTitlePending(room.ID, source.ID) {
		t.Fatal("reload lost the pending title")
	}
	turn.SourceMessageID, turn.TurnID = followUp.ID, "third"
	if err := renderer.Complete(ctx, turn, agentengine.TurnResult{
		Status: agentengine.TurnSucceeded, Output: "Ten seconds.",
	}); err != nil {
		t.Fatal(err)
	}
	close(unblock)
	select {
	case event := <-events:
		if event.Type != im.EventTypeRoomUpdated || event.Room == nil || event.Room.Title != "Investigate login timeout" {
			t.Fatalf("unexpected title event: %+v", event)
		}
		if got := attempts.Load(); got != 2 {
			t.Fatalf("model attempts = %d, want 2", got)
		}
	case <-ctx.Done():
		t.Fatal("missing title update")
	}
}
