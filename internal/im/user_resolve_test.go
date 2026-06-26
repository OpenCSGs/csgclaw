package im

import (
	"slices"
	"testing"
)

func TestEnsureAgentUserCanonicalizesBareID(t *testing.T) {
	svc := NewService()
	user, room, err := svc.EnsureAgentUser(EnsureAgentUserRequest{
		ID: "p-w-0604", Name: "worker", Role: "worker",
	})
	if err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}
	if user.ID != "user-p-w-0604" {
		t.Fatalf("EnsureAgentUser() ID = %q, want user-p-w-0604", user.ID)
	}
	if got := svc.ResolveUserID("p-w-0604"); got != "user-p-w-0604" {
		t.Fatalf("ResolveUserID(p-w-0604) = %q, want stored id user-p-w-0604", got)
	}
	if room == nil || !slices.Contains(room.Members, "user-p-w-0604") {
		t.Fatalf("EnsureAgentUser() room = %+v, want typed user member", room)
	}
}

func TestResolveUserIDUsesStoredIDOrName(t *testing.T) {
	svc := NewService()
	user, room, err := svc.EnsureAgentUser(EnsureAgentUserRequest{
		ID: "u-p-w-0604", Name: "worker", Role: "worker",
	})
	if err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}
	if got := svc.ResolveUserID("p-w-0604"); got != "user-p-w-0604" {
		t.Fatalf("ResolveUserID(name) = %q, want user-p-w-0604", got)
	}
	if got := svc.ResolveUserID("u-p-w-0604"); got != "user-p-w-0604" {
		t.Fatalf("ResolveUserID(id) = %q, want user-p-w-0604", got)
	}
	if user.ID != "user-p-w-0604" {
		t.Fatalf("EnsureAgentUser() ID = %q, want user-p-w-0604", user.ID)
	}
	if room == nil || !slices.Contains(room.Members, "user-p-w-0604") {
		t.Fatalf("EnsureAgentUser() room = %+v, want stored canonical worker member", room)
	}
}

func TestEnsureAgentUserPreservesExplicitWorkerID(t *testing.T) {
	svc := NewService()
	user, _, err := svc.EnsureAgentUser(EnsureAgentUserRequest{
		ID: "u-frontend-dev", Name: "frontend-dev", Role: "worker",
	})
	if err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}
	if user.ID != "user-frontend-dev" {
		t.Fatalf("EnsureAgentUser() ID = %q, want user-frontend-dev", user.ID)
	}
	if got := svc.ResolveUserID("frontend-dev"); got != "user-frontend-dev" {
		t.Fatalf("ResolveUserID(frontend-dev) = %q, want user-frontend-dev", got)
	}
}

func TestUserForParticipantResolvesStableHashAliases(t *testing.T) {
	svc := NewService()
	svc.users["user-admin"] = User{ID: "user-admin", Name: "admin"}
	svc.users["user-agent-zaha7h"] = User{ID: "user-agent-zaha7h", Name: "ux"}

	if user, ok := svc.userForParticipantLocked("pt-admin-9f6195c9"); !ok || user.Name != "admin" {
		t.Fatalf("userForParticipantLocked(pt-admin-9f6195c9) = %+v, %v; want admin", user, ok)
	}
	if user, ok := svc.userForParticipantLocked("pt-agent-zaha7h-d59735ad"); !ok || user.Name != "ux" {
		t.Fatalf("userForParticipantLocked(pt-agent-zaha7h-d59735ad) = %+v, %v; want ux", user, ok)
	}
}
