package team

import (
	"testing"

	"csgclaw/internal/agent"
	"csgclaw/internal/im"
)

func TestTaskExecutionRoomMemberParticipantIDsIncludesWorkerAgents(t *testing.T) {
	imSvc := im.NewService()
	leadParticipantID := agent.ManagerParticipantID
	if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID: leadParticipantID, Name: "manager", Role: agent.RoleManager,
	}); err != nil {
		t.Fatalf("EnsureAgentUser(manager) error = %v", err)
	}
	if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID: "u-p-w-1315", Name: "worker", Role: agent.RoleWorker,
	}); err != nil {
		t.Fatalf("EnsureAgentUser(worker) error = %v", err)
	}

	teamSvc := NewService()
	meta, err := teamSvc.CreateTeam(CreateTeamInput{
		Title:          "team",
		LeadAgentID:    agent.ManagerUserID,
		MemberAgentIDs: []string{"u-p-w-1315"},
	})
	if err != nil {
		t.Fatalf("CreateTeam() error = %v", err)
	}
	parent, err := teamSvc.CreateTask(CreateTaskInput{
		TeamID:    meta.ID,
		Title:     "parent",
		CreatedBy: "web",
	})
	if err != nil {
		t.Fatalf("CreateTask(parent) error = %v", err)
	}
	if _, err := teamSvc.PlanTask(PlanTaskInput{
		TeamID:      meta.ID,
		TaskID:      parent.ID,
		ActorID:     "web",
		PlanSummary: "plan",
		Tasks: []PlanTaskItem{{
			Title:    "child",
			AssignTo: "p-w-1315",
		}},
	}); err != nil {
		t.Fatalf("PlanTask() error = %v", err)
	}

	directory := NewCSGClawTeamDirectory(imSvc, nil)
	got, err := teamSvc.taskExecutionRoomMemberParticipantIDs(NewCSGClawAdapter(imSvc), directory, meta, parent)
	if err != nil {
		t.Fatalf("taskExecutionRoomMemberParticipantIDs() error = %v", err)
	}
	if len(got) != 1 || got[0] != "pt-p-w-1315" {
		t.Fatalf("taskExecutionRoomMemberParticipantIDs() = %v, want [pt-p-w-1315]", got)
	}
}

func TestTaskExecutionRoomMemberParticipantIDsMapsLocalManagerParticipantToLead(t *testing.T) {
	imSvc := im.NewService()
	if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID: agent.ManagerParticipantID, Name: "manager", Role: agent.RoleManager,
	}); err != nil {
		t.Fatalf("EnsureAgentUser(manager) error = %v", err)
	}
	if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID: "u-worker", Name: "worker", Role: agent.RoleWorker,
	}); err != nil {
		t.Fatalf("EnsureAgentUser(worker) error = %v", err)
	}

	teamSvc := NewService()
	meta, err := teamSvc.CreateTeam(CreateTeamInput{
		Title:          "team",
		LeadAgentID:    agent.ManagerUserID,
		MemberAgentIDs: []string{"u-worker"},
	})
	if err != nil {
		t.Fatalf("CreateTeam() error = %v", err)
	}
	parent, err := teamSvc.CreateTask(CreateTaskInput{
		TeamID:    meta.ID,
		Title:     "parent",
		CreatedBy: "web",
	})
	if err != nil {
		t.Fatalf("CreateTask(parent) error = %v", err)
	}

	directory := NewCSGClawTeamDirectory(imSvc, nil)
	got, err := teamSvc.taskExecutionRoomMemberParticipantIDs(NewCSGClawAdapter(imSvc), directory, meta, parent)
	if err != nil {
		t.Fatalf("taskExecutionRoomMemberParticipantIDs() error = %v", err)
	}
	if len(got) != 1 || got[0] != "pt-worker" {
		t.Fatalf("taskExecutionRoomMemberParticipantIDs() = %v, want [pt-worker]", got)
	}
}
