package agentengine

import (
	"context"
	agent "csgclaw/internal/agentengine/agents"
)

var WithLocalTemplateService = agent.WithLocalTemplateService

type unavailableAgents struct{}

func (unavailableAgents) Create(context.Context, AgentCreateRequest) (Agent, error) {
	return Agent{}, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
}
func (unavailableAgents) Get(context.Context, string, AgentGetOptions) (Agent, error) {
	return Agent{}, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
}
func (unavailableAgents) List(context.Context, AgentListOptions) ([]Agent, error) {
	return nil, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
}
func (unavailableAgents) Update(context.Context, string, AgentUpdateRequest) (Agent, error) {
	return Agent{}, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
}
func (unavailableAgents) Delete(context.Context, string) error {
	return &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
}
func (unavailableAgents) Recreate(context.Context, string, AgentRecreateOptions) (Agent, error) {
	return Agent{}, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
}
