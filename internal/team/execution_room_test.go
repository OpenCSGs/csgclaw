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
		ID: leadParticipantID, Name: "manager", Handle: "manager", Role: agent.RoleManager,
	}); err != nil {
		t.Fatalf("EnsureAgentUser(manager) error = %v", err)
	}
	if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID: "u-p-w-1315", Name: "worker", Handle: "p-w-1315", Role: agent.RoleWorker,
	}); err != nil {
		t.Fatalf("EnsureAgentUser(worker) error = %v", err)
	}
	room, err := imSvc.CreateRoom(im.CreateRoomRequest{
		Title:     "team room",
		CreatorID: leadParticipantID,
		MemberIDs: []string{leadParticipantID, "u-p-w-1315"},
	})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	teamSvc := NewService()
	meta, err := teamSvc.CreateTeam(CreateTeamInput{
		RoomID:      room.ID,
		Channel:     "csgclaw",
		Title:       "team",
		LeadAgentID: agent.ManagerUserID,
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
	got := teamSvc.taskExecutionRoomMemberParticipantIDs(directory, meta, parent)
	if len(got) != 1 || got[0] != "p-w-1315" {
		t.Fatalf("taskExecutionRoomMemberParticipantIDs() = %v, want [p-w-1315]", got)
	}
}

func TestTaskExecutionRoomMemberParticipantIDsMapsLocalManagerParticipantToLead(t *testing.T) {
	imSvc := im.NewService()
	if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID: agent.ManagerParticipantID, Name: "manager", Handle: "manager", Role: agent.RoleManager,
	}); err != nil {
		t.Fatalf("EnsureAgentUser(manager) error = %v", err)
	}
	if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID: "u-worker", Name: "worker", Handle: "worker", Role: agent.RoleWorker,
	}); err != nil {
		t.Fatalf("EnsureAgentUser(worker) error = %v", err)
	}
	room, err := imSvc.CreateRoom(im.CreateRoomRequest{
		Title:     "team room",
		CreatorID: agent.ManagerParticipantID,
		MemberIDs: []string{agent.ManagerParticipantID, "u-worker"},
	})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	teamSvc := NewService()
	meta, err := teamSvc.CreateTeam(CreateTeamInput{
		RoomID:      room.ID,
		Channel:     "csgclaw",
		Title:       "team",
		LeadAgentID: agent.ManagerUserID,
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
	got := teamSvc.taskExecutionRoomMemberParticipantIDs(directory, meta, parent)
	if len(got) != 1 || got[0] != "worker" {
		t.Fatalf("taskExecutionRoomMemberParticipantIDs() = %v, want [worker]", got)
	}
}
