package im

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestEnsureAgentSessionRoomPersistsAuditContextAndRepairsNotifyAll(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "im", "state.json")
	bus := NewBus()
	svc, err := NewServiceFromPathWithBus(statePath, bus)
	if err != nil {
		t.Fatalf("NewServiceFromPathWithBus() error = %v", err)
	}
	if _, _, err := svc.EnsureAgentUser(EnsureAgentUserRequest{ID: "user-reviewer", Name: "reviewer", Role: "worker"}); err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}

	created, err := svc.EnsureAgentSessionRoom(EnsureAgentSessionRoomRequest{
		SessionID:   "audit-session-01",
		AgentID:     "agent-reviewer",
		AgentName:   "Reviewer",
		AgentUserID: "user-reviewer",
	})
	if err != nil {
		t.Fatalf("EnsureAgentSessionRoom() error = %v", err)
	}
	wantTitle := "Anonymous Session: audit-session-01 | Agent: Reviewer (agent-reviewer)"
	if created.Title != wantTitle {
		t.Fatalf("Title = %q, want %q", created.Title, wantTitle)
	}
	if created.SessionID != "audit-session-01" || created.SessionAgentID != "agent-reviewer" {
		t.Fatalf("session metadata = %q/%q", created.SessionID, created.SessionAgentID)
	}
	if created.IsDirect || !created.NotifyAllAgents {
		t.Fatalf("room flags = direct:%t notify-all:%t", created.IsDirect, created.NotifyAllAgents)
	}
	if len(created.Members) != 2 || !slices.Contains(created.Members, AdminUserID) || !slices.Contains(created.Members, "user-reviewer") {
		t.Fatalf("Members = %#v, want admin and reviewer", created.Members)
	}
	if len(created.Messages) != 1 || created.Messages[0].SenderID != AdminUserID || created.Messages[0].Event == nil || created.Messages[0].Event.ActorID != AdminUserID {
		t.Fatalf("room creation message = %#v, want admin event", created.Messages)
	}

	disabled := false
	if _, err := svc.UpdateRoom(created.ID, UpdateRoomRequest{NotifyAllAgents: &disabled}); err != nil {
		t.Fatalf("UpdateRoom() error = %v", err)
	}
	reused, err := svc.EnsureAgentSessionRoom(EnsureAgentSessionRoomRequest{
		SessionID:   "audit-session-01",
		AgentID:     "agent-reviewer",
		AgentName:   "Renamed Reviewer",
		AgentUserID: "user-reviewer",
	})
	if err != nil {
		t.Fatalf("reuse EnsureAgentSessionRoom() error = %v", err)
	}
	if reused.ID != created.ID || reused.Title != wantTitle || !reused.NotifyAllAgents {
		t.Fatalf("reused room = %+v, want same room, immutable title, notify-all", reused)
	}

	reloaded, err := NewServiceFromPath(statePath)
	if err != nil {
		t.Fatalf("NewServiceFromPath() error = %v", err)
	}
	got, err := reloaded.EnsureAgentSessionRoom(EnsureAgentSessionRoomRequest{
		SessionID:   "audit-session-01",
		AgentID:     "agent-reviewer",
		AgentName:   "Reviewer",
		AgentUserID: "user-reviewer",
	})
	if err != nil {
		t.Fatalf("reloaded EnsureAgentSessionRoom() error = %v", err)
	}
	if got.ID != created.ID || got.Title != wantTitle || !got.NotifyAllAgents {
		t.Fatalf("reloaded room = %+v", got)
	}
}

func TestEnsureAgentSessionRoomRejectsAgentReuseAndMemberChanges(t *testing.T) {
	svc := NewService()
	for _, user := range []EnsureAgentUserRequest{
		{ID: "user-alpha", Name: "alpha", Role: "worker"},
		{ID: "user-beta", Name: "beta", Role: "worker"},
	} {
		if _, _, err := svc.EnsureAgentUser(user); err != nil {
			t.Fatalf("EnsureAgentUser(%s) error = %v", user.ID, err)
		}
	}
	created, err := svc.EnsureAgentSessionRoom(EnsureAgentSessionRoomRequest{
		SessionID: "shared", AgentID: "agent-alpha", AgentName: "Alpha", AgentUserID: "user-alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnsureAgentSessionRoom(EnsureAgentSessionRoomRequest{
		SessionID: "shared", AgentID: "agent-beta", AgentName: "Beta", AgentUserID: "user-beta",
	}); !errors.Is(err, ErrSessionAgentConflict) {
		t.Fatalf("agent mismatch error = %v, want ErrSessionAgentConflict", err)
	}
	if _, err := svc.AddRoomMembers(AddRoomMembersRequest{
		RoomID: created.ID, InviterID: AdminUserID, UserIDs: []string{"user-beta"},
	}); err == nil || !strings.Contains(err.Error(), "agent session room") {
		t.Fatalf("AddRoomMembers() error = %v, want immutable session membership", err)
	}
	if _, err := svc.RemoveRoomMembers(AddRoomMembersRequest{
		RoomID: created.ID, InviterID: AdminUserID, UserIDs: []string{"user-alpha"},
	}); err == nil || !strings.Contains(err.Error(), "agent session room") {
		t.Fatalf("RemoveRoomMembers() error = %v, want immutable session membership", err)
	}
}

func TestEnsureAgentSessionRoomRejectsPersistedMembershipDrift(t *testing.T) {
	svc := NewServiceFromBootstrap(Bootstrap{
		Users: []User{
			{ID: "user-alpha", Name: "alpha", Role: "worker"},
			{ID: "user-beta", Name: "beta", Role: "worker"},
		},
		Rooms: []Room{{
			ID: "room-drifted", SessionID: "drifted", SessionAgentID: "agent-alpha",
			Members: []string{AdminUserID, "user-alpha", "user-beta"},
		}},
	})
	_, err := svc.EnsureAgentSessionRoom(EnsureAgentSessionRoomRequest{
		SessionID: "drifted", AgentID: "agent-alpha", AgentName: "Alpha", AgentUserID: "user-alpha",
	})
	if !errors.Is(err, ErrSessionRoomMembersConflict) {
		t.Fatalf("error = %v, want ErrSessionRoomMembersConflict", err)
	}
}

func TestEnsureAgentSessionRoomRejectsAmbiguousPersistedMapping(t *testing.T) {
	svc := NewServiceFromBootstrap(Bootstrap{
		Users: []User{{ID: "user-alpha", Name: "alpha", Role: "worker"}},
		Rooms: []Room{
			{ID: "room-a", SessionID: "duplicate", SessionAgentID: "agent-alpha", Members: []string{AdminUserID, "user-alpha"}},
			{ID: "room-b", SessionID: "duplicate", SessionAgentID: "agent-alpha", Members: []string{AdminUserID, "user-alpha"}},
		},
	})
	_, err := svc.EnsureAgentSessionRoom(EnsureAgentSessionRoomRequest{
		SessionID: "duplicate", AgentID: "agent-alpha", AgentName: "Alpha", AgentUserID: "user-alpha",
	})
	if !errors.Is(err, ErrSessionRoomConflict) {
		t.Fatalf("error = %v, want ErrSessionRoomConflict", err)
	}
}
