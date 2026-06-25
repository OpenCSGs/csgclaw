package team

import (
	"strings"
	"testing"

	"csgclaw/internal/agent"
)

func TestNormalizeManagerPlanRejectsNonParticipantWorkerID(t *testing.T) {
	_, err := normalizeManagerPlan(managerPlanContext{
		TeamID:              "team-1",
		LeadAgentID:         agent.ManagerUserID,
		AssignableMemberIDs: []string{"p-w-0604"},
		Task: managerPlanTaskContext{
			ID:    "task-1",
			Title: "Ship release",
		},
	}, managerPlanLLMResponse{
		PlanSummary: "plan",
		Tasks: []managerPlanLLMTask{{
			IDRef:    "draft",
			Title:    "Draft release note",
			AssignTo: "u-p-w-0604",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "assignable_member_ids") {
		t.Fatalf("normalizeManagerPlan() error = %v, want non-participant assign_to rejection", err)
	}
}

func TestManagerPlanContextResolvesLocalManagerParticipantToLead(t *testing.T) {
	planner := &ManagerPlanner{directory: plannerAliasDirectory{}}
	planCtx := planner.managerPlanContext(TeamMeta{
		ID:          "team-1",
		RoomID:      "room-1",
		LeadAgentID: agent.ManagerUserID,
	}, TeamTask{
		ID:    "task-1",
		Title: "Research",
	})

	if len(planCtx.AssignableMemberIDs) != 1 || planCtx.AssignableMemberIDs[0] != "pt-worker" {
		t.Fatalf("assignable_member_ids = %v, want [pt-worker]", planCtx.AssignableMemberIDs)
	}
	managerCount := 0
	for _, member := range planCtx.Members {
		if member.ID == "pt-manager" {
			managerCount++
		}
	}
	if managerCount != 1 {
		t.Fatalf("manager member count = %d, want 1; members=%+v", managerCount, planCtx.Members)
	}
}

type plannerAliasDirectory struct{}

func (plannerAliasDirectory) TeamRoomMemberIDs(string) []string {
	return []string{"pt-manager", "pt-worker"}
}

func (plannerAliasDirectory) UserProfile(id string) (MemberProfile, bool) {
	switch id {
	case "pt-manager":
		return MemberProfile{ID: "pt-manager", Name: "manager", Role: "manager"}, true
	case "pt-worker":
		return MemberProfile{ID: "pt-worker", Name: "worker", Role: "worker"}, true
	default:
		return MemberProfile{}, false
	}
}

func (plannerAliasDirectory) AgentProfile(id string) (MemberProfile, bool) {
	if id == "agent-manager" {
		return MemberProfile{ID: "agent-manager", Name: "manager", Role: "manager"}, true
	}
	return MemberProfile{}, false
}

func (plannerAliasDirectory) ResolveAgentID(id string) string {
	if id == "pt-manager" || id == "manager" {
		return "agent-manager"
	}
	return "agent-" + strings.TrimPrefix(cleanParticipantID(id), "pt-")
}
