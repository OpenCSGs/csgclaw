package agent

import (
	"slices"
	"strings"
	"time"
)

const (
	RoleAgent   = "agent"
	RoleWorker  = "worker"
	RoleManager = "manager"
)

type Agent struct {
	ID               string                   `json:"id"`
	Name             string                   `json:"name"`
	Description      string                   `json:"description,omitempty"`
	Image            string                   `json:"image,omitempty"`
	BoxID            string                   `json:"box_id,omitempty"`
	Role             string                   `json:"role"`
	Status           string                   `json:"status"`
	CreatedAt        time.Time                `json:"created_at"`
	Profile          string                   `json:"profile,omitempty"`
	Provider         string                   `json:"provider,omitempty"`
	ModelID          string                   `json:"model_id,omitempty"`
	ReasoningEffort  string                   `json:"reasoning_effort,omitempty"`
	AgentProfile     AgentProfile             `json:"agent_profile,omitempty"`
	ProfileComplete  bool                     `json:"profile_complete"`
	DetectionResults []ProfileDetectionResult `json:"detection_results,omitempty"`
}

type CreateAgentSpec struct {
	ID           string       `json:"id,omitempty"`
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	Image        string       `json:"image,omitempty"`
	Role         string       `json:"role,omitempty"`
	Status       string       `json:"status,omitempty"`
	CreatedAt    time.Time    `json:"created_at,omitempty"`
	Profile      string       `json:"profile,omitempty"`
	ModelID      string       `json:"model_id,omitempty"`
	AgentProfile AgentProfile `json:"agent_profile,omitempty"`
}

type UpdateRequest struct {
	Name         *string       `json:"name,omitempty"`
	Description  *string       `json:"description,omitempty"`
	Image        *string       `json:"image,omitempty"`
	AgentProfile *AgentProfile `json:"agent_profile,omitempty"`
}

type CreateRequest struct {
	Spec      CreateAgentSpec
	Replace   bool
	FieldMask []string
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", RoleAgent:
		return RoleAgent
	case RoleWorker:
		return RoleWorker
	case RoleManager:
		return RoleManager
	default:
		return strings.ToLower(strings.TrimSpace(role))
	}
}

func isManagerAgent(a Agent) bool {
	return strings.EqualFold(strings.TrimSpace(a.Role), RoleManager) ||
		strings.EqualFold(strings.TrimSpace(a.Name), ManagerName) ||
		strings.EqualFold(strings.TrimSpace(a.ID), ManagerUserID)
}

func sortedAgentsFromMap(items map[string]Agent) []Agent {
	agents := make([]Agent, 0, len(items))
	for _, a := range items {
		agents = append(agents, *cloneAgent(&a))
	}
	slices.SortFunc(agents, func(a, b Agent) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			switch {
			case a.ID < b.ID:
				return -1
			case a.ID > b.ID:
				return 1
			default:
				return 0
			}
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 1
	})
	return agents
}

func persistedAgentsFromMap(items map[string]Agent) []persistedAgent {
	agents := sortedAgentsFromMap(items)
	persisted := make([]persistedAgent, 0, len(agents))
	for _, a := range agents {
		persisted = append(persisted, newPersistedAgent(a))
	}
	return persisted
}

func cloneAgent(src *Agent) *Agent {
	if src == nil {
		return nil
	}
	dst := *src
	dst.AgentProfile = cloneProfile(src.AgentProfile)
	dst.DetectionResults = append([]ProfileDetectionResult(nil), src.DetectionResults...)
	return &dst
}
