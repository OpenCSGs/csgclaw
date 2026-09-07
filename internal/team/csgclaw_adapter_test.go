package team

import (
	"context"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/im"
	"testing"
	"time"
)

func TestCSGClawAdapterEnsureParticipantUserReusesManagerParticipantIdentity(t *testing.T) {
	imSvc := im.NewService()
	if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID: agent.ManagerParticipantID, Name: "manager", Role: agent.RoleManager,
	}); err != nil {
		t.Fatalf("EnsureAgentUser(manager participant) error = %v", err)
	}

	adapter := NewCSGClawAdapter(imSvc)
	user, err := adapter.ensureParticipantUser(agent.ManagerParticipantID, agent.RoleManager)
	if err != nil {
		t.Fatalf("ensureParticipantUser(manager) error = %v", err)
	}
	if user.ID != im.ManagerUserID {
		t.Fatalf("ensureParticipantUser() ID = %q, want manager user %q", user.ID, im.ManagerUserID)
	}
}

func TestCSGClawAdapterEnsureParticipantUserCreatesDefaultChannelUser(t *testing.T) {
	imSvc := im.NewService()
	adapter := NewCSGClawAdapter(imSvc)
	user, err := adapter.ensureParticipantUser("p-w-0604", agent.RoleWorker)
	if err != nil {
		t.Fatalf("ensureParticipantUser(worker) error = %v", err)
	}
	if user.ID != "user-p-w-0604" || user.Name != "p-w-0604" {
		t.Fatalf("ensureParticipantUser() = %+v, want CSGClaw user user-p-w-0604 with participant name", user)
	}
}

func TestCSGClawAdapterAcceptsLegacyUserIDAsParticipantAlias(t *testing.T) {
	imSvc := im.NewService()
	if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID: "u-p-w-0604", Name: "worker", Role: "worker",
	}); err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}

	adapter := NewCSGClawAdapter(imSvc)
	_, err := adapter.EnsureRoom(context.Background(), EnsureRoomRequest{
		Title:                "team",
		LeadParticipantID:    agent.ManagerParticipantID,
		MemberParticipantIDs: []string{"u-p-w-0604"},
	})
	if err != nil {
		t.Fatalf("EnsureRoom() error = %v", err)
	}
}

func TestCSGClawAdapterEnsureRoomPublishesRoomCreatedEvent(t *testing.T) {
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()

	imSvc := im.NewServiceWithBus(bus)
	adapter := NewCSGClawAdapter(imSvc)
	roomRef, err := adapter.EnsureRoom(context.Background(), EnsureRoomRequest{
		Title:                "[task-1] Today and yesterday AI news",
		LeadParticipantID:    agent.ManagerParticipantID,
		MemberParticipantIDs: []string{"pt-dev"},
	})
	if err != nil {
		t.Fatalf("EnsureRoom() error = %v", err)
	}

	event := mustReceiveAdapterIMEvent(t, events)
	if event.Type != im.EventTypeRoomCreated || event.RoomID != roomRef.RoomID || event.Room == nil {
		t.Fatalf("event = %+v, want room.created for %s", event, roomRef.RoomID)
	}
	if event.Room.Title != "[task-1] Today and yesterday AI news" {
		t.Fatalf("event room title = %q, want task title", event.Room.Title)
	}
	if !roomMembersContain(event.Room.Members, im.ManagerUserID) || !roomMembersContain(event.Room.Members, "user-dev") {
		t.Fatalf("event room members = %+v, want manager and dev", event.Room.Members)
	}
}

func mustReceiveAdapterIMEvent(t *testing.T, events <-chan im.Event) im.Event {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for IM event")
		return im.Event{}
	}
}

func roomMembersContain(members []string, id string) bool {
	for _, member := range members {
		if member == id {
			return true
		}
	}
	return false
}
