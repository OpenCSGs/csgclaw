package apiclient

import (
	"context"
	"encoding/json"
	"net/http"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/participant"
)

func (c *Client) CreateAgent(ctx context.Context, request apitypes.CreateAgentRequest) (apitypes.Agent, error) {
	var result apitypes.Agent
	if err := c.DoJSON(ctx, http.MethodPost, "/api/v1/agents", request, &result); err != nil {
		return apitypes.Agent{}, err
	}
	return result, nil
}

type participantCreateRequest struct {
	participant.CreateRequest
	AgentBinding struct {
		Mode    string                            `json:"mode,omitempty"`
		AgentID string                            `json:"agent_id,omitempty"`
		Agent   *apitypes.ParticipantAgentRequest `json:"agent,omitempty"`
	} `json:"agent_binding,omitempty"`
}

func participantCreatePayload(request participant.CreateRequest) (participantCreateRequest, error) {
	result := participantCreateRequest{CreateRequest: request}
	result.AgentBinding.Mode = request.AgentBinding.Mode
	result.AgentBinding.AgentID = request.AgentBinding.AgentID
	spec := request.AgentBinding.Agent
	if spec == nil {
		return result, nil
	}
	runtime := spec.RuntimeConfig()
	profile := spec.AgentProfile
	input := &apitypes.ParticipantAgentRequest{
		ID:           spec.ID,
		Name:         spec.Name,
		Description:  spec.Description,
		Instructions: spec.Instructions,
		FromTemplate: spec.FromTemplate,
		Role:         spec.Role,
		Status:       spec.Status,
		CreatedAt:    spec.CreatedAt,
		UpdatedAt:    spec.UpdatedAt,
		Runtime: &apitypes.AgentRuntimeInput{
			Name:           &runtime.Name,
			SandboxEnabled: &runtime.Sandboxed,
			Image:          &spec.Image,
			Options:        &spec.RuntimeOptions,
		},
		ModelConfig: &apitypes.AgentModelConfigRequest{
			CreateAgentProfile: apitypes.CreateAgentProfile{
				Selector:        spec.Profile,
				ImageGeneration: profile.ImageGeneration,
				VideoGeneration: profile.VideoGeneration,
				ModelProviderID: profile.ModelProviderID,
				BaseURL:         profile.BaseURL,
				APIKey:          profile.APIKey,
				Headers:         profile.Headers,
				ModelID:         profile.ModelID,
				ReasoningEffort: profile.ReasoningEffort,
				EnableFastMode:  profile.EnableFastMode,
				RequestOptions:  profile.RequestOptions,
				Env:             profile.Env,
			},
			APIProtocol: profile.APIProtocol,
			Name:        profile.Name,
			Description: profile.Description,
			Provider:    profile.Provider,
		},
	}
	if spec.Skills != nil {
		input.Skills = &spec.Skills
	}
	if spec.MCPServersSet || spec.MCPServers != nil {
		data, err := json.Marshal(spec.MCPServers)
		if err != nil {
			return result, err
		}
		input.MCPServers = data
	}
	result.AgentBinding.Agent = input
	return result, nil
}
