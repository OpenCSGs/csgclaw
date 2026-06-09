package team

import (
	"context"
	"strings"
	"testing"

	"csgclaw/internal/agent"
	"csgclaw/internal/im"
)

func TestCSGClawAdapterEnsureParticipantUserReusesManagerParticipantIdentity(t *testing.T) {
	imSvc := im.NewService()
	if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID: agent.ManagerParticipantID, Name: "manager", Handle: "manager", Role: agent.RoleManager,
	}); err != nil {
		t.Fatalf("EnsureAgentUser(manager participant) error = %v", err)
	}

	adapter := NewCSGClawAdapter(imSvc)
	user, err := adapter.ensureParticipantUser(agent.ManagerParticipantID, agent.RoleManager)
	if err != nil {
		t.Fatalf("ensureParticipantUser(manager) error = %v", err)
	}
	if user.ID != agent.ManagerParticipantID {
		t.Fatalf("ensureParticipantUser() ID = %q, want existing manager participant %q", user.ID, agent.ManagerParticipantID)
	}
}

func TestCSGClawAdapterEnsureParticipantUserCreatesDefaultChannelUser(t *testing.T) {
	imSvc := im.NewService()
	adapter := NewCSGClawAdapter(imSvc)
	user, err := adapter.ensureParticipantUser("p-w-0604", agent.RoleWorker)
	if err != nil {
		t.Fatalf("ensureParticipantUser(worker) error = %v", err)
	}
	if user.ID != "u-p-w-0604" || user.Handle != "p-w-0604" {
		t.Fatalf("ensureParticipantUser() = %+v, want CSGClaw user u-p-w-0604 with participant handle", user)
	}
}

func TestCSGClawAdapterRejectsLegacyUserIDAsParticipantID(t *testing.T) {
	imSvc := im.NewService()
	if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID: "u-p-w-0604", Name: "worker", Handle: "p-w-0604", Role: "worker",
	}); err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}

	adapter := NewCSGClawAdapter(imSvc)
	_, err := adapter.EnsureRoom(context.Background(), EnsureRoomRequest{
		Title:                "team",
		LeadParticipantID:    agent.ManagerParticipantID,
		MemberParticipantIDs: []string{"u-p-w-0604"},
	})
	if err == nil || !strings.Contains(err.Error(), "not CSGClaw user/agent id") {
		t.Fatalf("EnsureRoom() error = %v, want legacy user id rejection", err)
	}
}
