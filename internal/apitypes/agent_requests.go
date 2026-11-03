package apitypes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// AgentModelConfigRequest is the writable configuration accepted by updates and Participant creation.
type AgentModelConfigRequest struct {
	CreateAgentProfile
	APIProtocol string `json:"api_protocol,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Provider    string `json:"provider,omitempty"`
}

type ParticipantAgentRequest struct {
	ID           string                   `json:"id,omitempty"`
	Name         string                   `json:"name"`
	Description  string                   `json:"description,omitempty"`
	Instructions string                   `json:"instructions,omitempty"`
	Skills       *[]string                `json:"skills,omitempty"`
	Runtime      *AgentRuntimeInput       `json:"runtime,omitempty"`
	ModelConfig  *AgentModelConfigRequest `json:"model_config,omitempty"`
	MCPServers   json.RawMessage          `json:"mcp_servers,omitempty"`
	FromTemplate string                   `json:"from_template,omitempty"`
	Role         string                   `json:"role,omitempty"`
	Status       string                   `json:"status,omitempty"`
	CreatedAt    time.Time                `json:"created_at,omitempty"`
	UpdatedAt    time.Time                `json:"updated_at,omitempty"`
}

type RecreateAgentConfiguration struct {
	Name         string              `json:"name,omitempty"`
	Description  string              `json:"description,omitempty"`
	Instructions string              `json:"instructions,omitempty"`
	Runtime      *AgentRuntimeInput  `json:"runtime,omitempty"`
	ModelConfig  *CreateAgentProfile `json:"model_config,omitempty"`
	MCPServers   map[string]any      `json:"mcp_servers,omitempty"`
}

type RecreateAgentRequest struct {
	Configuration *RecreateAgentConfiguration `json:"configuration,omitempty"`
	FieldMask     []string                    `json:"field_mask,omitempty"`
}

func (r *ParticipantAgentRequest) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("agent must be a JSON object")
	}
	type request ParticipantAgentRequest
	var decoded request
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*r = ParticipantAgentRequest(decoded)
	return nil
}
