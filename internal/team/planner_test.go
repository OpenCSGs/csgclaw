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
		ID:             "team-1",
		LeadAgentID:    agent.ManagerUserID,
		MemberAgentIDs: []string{"u-worker"},
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

func TestNormalizeManagerPlanInfersValidationDependency(t *testing.T) {
	got, err := normalizeManagerPlan(managerPlanContext{
		TeamID:              "team-1",
		LeadAgentID:         agent.ManagerUserID,
		LeadParticipantID:   "manager",
		AssignableMemberIDs: []string{"frontend-dev", "qa-eng"},
		Task: managerPlanTaskContext{
			ID:    "task-1",
			Title: "Build tank game",
		},
	}, managerPlanLLMResponse{
		PlanSummary: "plan",
		Tasks: []managerPlanLLMTask{
			{
				IDRef:    "frontend",
				Title:    "开发坦克大战游戏前端界面和核心逻辑",
				AssignTo: "frontend-dev",
			},
			{
				IDRef:    "qa",
				Title:    "测试坦克大战游戏功能和质量",
				AssignTo: "qa-eng",
			},
		},
	})
	if err != nil {
		t.Fatalf("normalizeManagerPlan() error = %v", err)
	}
	if len(got.Tasks) != 2 {
		t.Fatalf("tasks len = %d, want 2", len(got.Tasks))
	}
	if len(got.Tasks[1].DependsOnRefs) != 1 || got.Tasks[1].DependsOnRefs[0] != "frontend" {
		t.Fatalf("qa depends_on_refs = %+v, want [frontend]", got.Tasks[1].DependsOnRefs)
	}
}

type plannerAliasDirectory struct{}

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

func (plannerAliasDirectory) ParticipantIDForAgentID(id string) string {
	switch id {
	case "u-manager":
		return "manager"
	case "u-worker":
		return "worker"
	default:
		return ""
	}
}
