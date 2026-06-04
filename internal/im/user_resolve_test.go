package im

import "testing"

func TestEnsureAgentUserDoesNotInferCanonicalID(t *testing.T) {
	svc := NewService()
	user, room, err := svc.EnsureAgentUser(EnsureAgentUserRequest{
		ID: "p-w-0604", Name: "worker", Handle: "p-w-0604", Role: "worker",
	})
	if err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}
	if user.ID != "p-w-0604" {
		t.Fatalf("EnsureAgentUser() ID = %q, want p-w-0604", user.ID)
	}
	if got := svc.ResolveUserID("p-w-0604"); got != "p-w-0604" {
		t.Fatalf("ResolveUserID(p-w-0604) = %q, want stored id p-w-0604", got)
	}
	if room == nil || !containsUserIDInRoom(*room, "p-w-0604") {
		t.Fatalf("EnsureAgentUser() room = %+v, want raw stored member", room)
	}
}

func TestResolveUserIDUsesStoredIDOrHandle(t *testing.T) {
	svc := NewService()
	user, room, err := svc.EnsureAgentUser(EnsureAgentUserRequest{
		ID: "u-p-w-0604", Name: "worker", Handle: "p-w-0604", Role: "worker",
	})
	if err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}
	if got := svc.ResolveUserID("p-w-0604"); got != "u-p-w-0604" {
		t.Fatalf("ResolveUserID(handle) = %q, want u-p-w-0604", got)
	}
	if got := svc.ResolveUserID("u-p-w-0604"); got != "u-p-w-0604" {
		t.Fatalf("ResolveUserID(id) = %q, want u-p-w-0604", got)
	}
	if user.ID != "u-p-w-0604" {
		t.Fatalf("EnsureAgentUser() ID = %q, want u-p-w-0604", user.ID)
	}
	if room == nil || !containsUserIDInRoom(*room, "u-p-w-0604") {
		t.Fatalf("EnsureAgentUser() room = %+v, want stored canonical worker member", room)
	}
}

func TestEnsureAgentUserPreservesExplicitWorkerID(t *testing.T) {
	svc := NewService()
	user, _, err := svc.EnsureAgentUser(EnsureAgentUserRequest{
		ID: "u-frontend-dev", Name: "frontend-dev", Handle: "frontend-dev", Role: "worker",
	})
	if err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}
	if user.ID != "u-frontend-dev" {
		t.Fatalf("EnsureAgentUser() ID = %q, want u-frontend-dev", user.ID)
	}
	if got := svc.ResolveUserID("frontend-dev"); got != "u-frontend-dev" {
		t.Fatalf("ResolveUserID(frontend-dev) = %q, want u-frontend-dev", got)
	}
}
