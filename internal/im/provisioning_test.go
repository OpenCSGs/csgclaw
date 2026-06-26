package im

import (
	"context"
	"testing"
	"time"
)

func TestProvisionerEnsureAgentUserPublishesBootstrapRoom(t *testing.T) {
	bus := NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()

	provisioner := NewProvisioner(NewService(), bus)

	_, err := provisioner.EnsureAgentUser(context.Background(), AgentIdentity{
		ID:          "u-alice",
		Name:        "Alice",
		Description: "test lead",
		Role:        "Worker",
	})
	if err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}

	first := mustReceiveEvent(t, events)
	if first.Type != EventTypeUserCreated {
		t.Fatalf("first event.Type = %q, want %q", first.Type, EventTypeUserCreated)
	}
	if first.User == nil || first.User.ID != "user-alice" {
		t.Fatalf("first event.User = %+v, want user-alice", first.User)
	}

	second := mustReceiveEvent(t, events)
	if second.Type != EventTypeRoomCreated {
		t.Fatalf("second event.Type = %q, want %q", second.Type, EventTypeRoomCreated)
	}
	if second.Room == nil {
		t.Fatal("second event.Room = nil, want bootstrap room")
	}
	if second.Room.Title != "Alice" {
		t.Fatalf("second event.Room.Title = %q, want %q", second.Room.Title, "Alice")
	}
	if !containsUserIDInRoom(*second.Room, "admin") || !containsUserIDInRoom(*second.Room, "u-alice") {
		t.Fatalf("second event.Room.Members = %+v, want admin and worker", second.Room.Members)
	}

	select {
	case evt := <-events:
		t.Fatalf("unexpected third event before delay: %q", evt.Type)
	case <-time.After(150 * time.Millisecond):
	}

	third := mustReceiveEventWithin(t, events, 2*time.Second)
	if third.Type != EventTypeMessageCreated {
		t.Fatalf("third event.Type = %q, want %q", third.Type, EventTypeMessageCreated)
	}
	if third.Message == nil {
		t.Fatal("third event.Message = nil, want bootstrap message")
	}
	if third.Message.SenderID != "user-admin" {
		t.Fatalf("third event.Message.SenderID = %q, want %q", third.Message.SenderID, "user-admin")
	}
	wantContent := "Write this down in your memory: your name is Alice. Your responsibility is test lead"
	if third.Message.Content != wantContent {
		t.Fatalf("third event.Message.Content = %q, want %q", third.Message.Content, wantContent)
	}
	if third.Sender == nil || third.Sender.ID != "user-admin" {
		t.Fatalf("third event.Sender = %+v, want user-admin", third.Sender)
	}
}

func mustReceiveEvent(t *testing.T, events <-chan Event) Event {
	t.Helper()
	return mustReceiveEventWithin(t, events, time.Second)
}

func mustReceiveEventWithin(t *testing.T, events <-chan Event, timeout time.Duration) Event {
	t.Helper()
	select {
	case evt := <-events:
		return evt
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for IM event after %v", timeout)
		return Event{}
	}
}
