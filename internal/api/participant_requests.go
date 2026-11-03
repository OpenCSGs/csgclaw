package api

import (
	"encoding/json"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/participant"
)

type participantCreateRequest struct {
	participant.CreateRequest
	AgentBinding struct {
		Mode    string                            `json:"mode,omitempty"`
		AgentID string                            `json:"agent_id,omitempty"`
		Agent   *apitypes.ParticipantAgentRequest `json:"agent,omitempty"`
	} `json:"agent_binding,omitempty"`
}

func (r participantCreateRequest) serviceRequest() (participant.CreateRequest, error) {
	request := r.CreateRequest
	request.AgentBinding = participant.AgentBindingSpec{Mode: r.AgentBinding.Mode, AgentID: r.AgentBinding.AgentID}
	input := r.AgentBinding.Agent
	if input == nil {
		return request, nil
	}
	spec := &agent.CreateAgentSpec{
		ID:           input.ID,
		Name:         input.Name,
		Description:  input.Description,
		Instructions: input.Instructions,
		FromTemplate: input.FromTemplate,
		Role:         input.Role,
		Status:       input.Status,
		CreatedAt:    input.CreatedAt,
		UpdatedAt:    input.UpdatedAt,
	}
	if input.Skills != nil {
		spec.Skills = *input.Skills
	}
	if input.Runtime != nil {
		if input.Runtime.Name != nil {
			spec.RuntimeName = *input.Runtime.Name
		}
		if input.Runtime.SandboxEnabled != nil {
			spec.SandboxEnabled = *input.Runtime.SandboxEnabled
		}
		if input.Runtime.Image != nil {
			spec.Image = *input.Runtime.Image
		}
		if input.Runtime.Options != nil {
			spec.RuntimeOptions = *input.Runtime.Options
		}
	}
	if input.ModelConfig != nil {
		spec.AgentProfile = agentModelConfigProfile(*input.ModelConfig)
		spec.Profile = input.ModelConfig.Selector
	}
	if len(input.MCPServers) > 0 {
		spec.MCPServersSet = true
		if err := json.Unmarshal(input.MCPServers, &spec.MCPServers); err != nil {
			return request, err
		}
	}
	request.AgentBinding.Agent = spec
	return request, nil
}
