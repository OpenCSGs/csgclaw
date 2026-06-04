package api

import (
	"testing"

	"csgclaw/internal/agent"
	"csgclaw/internal/im"
	"csgclaw/internal/team"
)

func TestTaskExecutionRoomMemberBotIDsIncludesWorkerAgents(t *testing.T) {
	imSvc := im.NewService()
	leadID := "u-manager"
	if _, _, err := imSvc.EnsureAgentUser(im.EnsureAgentUserRequest{
		ID: leadID, Name: "manager", Handle: "u-manager", Role: agent.RoleManager,
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
		CreatorID: leadID,
		MemberIDs: []string{leadID, "u-p-w-1315"},
	})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	teamSvc := team.NewService()
	meta, err := teamSvc.CreateTeam(team.CreateTeamInput{
		RoomID:    room.ID,
		Channel:   "csgclaw",
		Title:     "team",
		LeadBotID: leadID,
	})
	if err != nil {
		t.Fatalf("CreateTeam() error = %v", err)
	}
	parent, err := teamSvc.CreateTask(team.CreateTaskInput{
		TeamID:    meta.ID,
		Title:     "parent",
		CreatedBy: "web",
	})
	if err != nil {
		t.Fatalf("CreateTask(parent) error = %v", err)
	}
	if _, err := teamSvc.PlanTask(team.PlanTaskInput{
		TeamID:      meta.ID,
		TaskID:      parent.ID,
		ActorID:     "web",
		PlanSummary: "plan",
		Tasks: []team.PlanTaskItem{{
			Title:    "child",
			AssignTo: "u-p-w-1315",
		}},
	}); err != nil {
		t.Fatalf("PlanTask() error = %v", err)
	}

	h := &Handler{im: imSvc, teamSvc: teamSvc}
	got := h.taskExecutionRoomMemberBotIDs(meta, parent)
	if len(got) != 1 || got[0] != "u-p-w-1315" {
		t.Fatalf("taskExecutionRoomMemberBotIDs() = %v, want [u-p-w-1315]", got)
	}
}
