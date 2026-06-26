package team

import (
	"context"
	"testing"

	"csgclaw/internal/agent"
	"csgclaw/internal/im"
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
