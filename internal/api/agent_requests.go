package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
)

type updateAgentRequest struct {
	Name         *string                     `json:"name,omitempty"`
	Description  *string                     `json:"description,omitempty"`
	Instructions *string                     `json:"instructions,omitempty"`
	Runtime      *apitypes.AgentRuntimeInput `json:"runtime,omitempty"`
	ModelConfig  json.RawMessage             `json:"model_config,omitempty"`
	MCPServers   json.RawMessage             `json:"mcp_servers,omitempty"`
}

type recreateAgentRequest apitypes.RecreateAgentRequest

func decodeAgentRequest(reader io.Reader, target any, optional bool) error {
	decoder := json.NewDecoder(reader)
	var payload json.RawMessage
	if err := decoder.Decode(&payload); err != nil {
		if optional && err == io.EOF {
			return nil
		}
		return err
	}
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 || payload[0] != '{' {
		return fmt.Errorf("request must contain one JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("request must contain one JSON object")
	}
	fields := json.NewDecoder(bytes.NewReader(payload))
	fields.DisallowUnknownFields()
	return fields.Decode(target)
}

func (r updateAgentRequest) updateRequest() (agent.UpdateRequest, error) {
	result := agent.UpdateRequest{Name: r.Name, Description: r.Description, Instructions: r.Instructions}
	if r.Runtime != nil {
		result.Image = r.Runtime.Image
		result.RuntimeOptions = r.Runtime.Options
		result.RuntimeSelectionRequested = r.Runtime.Name != nil || r.Runtime.SandboxEnabled != nil
		if r.Runtime.Name != nil {
			result.RuntimeName = *r.Runtime.Name
		}
		result.SandboxEnabled = r.Runtime.SandboxEnabled
	}
	if len(r.MCPServers) > 0 {
		result.MCPServersSet = true
		var config map[string]any
		if err := json.Unmarshal(r.MCPServers, &config); err != nil {
			return result, err
		}
		result.MCPServers = &config
	}
	if len(r.ModelConfig) > 0 && !bytes.Equal(bytes.TrimSpace(r.ModelConfig), []byte("null")) {
		var config apitypes.AgentModelConfigRequest
		if err := decodeAgentRequest(bytes.NewReader(r.ModelConfig), &config, false); err != nil {
			return result, err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(r.ModelConfig, &fields); err != nil {
			return result, err
		}
		if raw, exists := fields["selector"]; exists && string(raw) != "null" {
			selector := strings.TrimSpace(config.Selector)
			result.Profile = &selector
		}
		if _, exists := fields["selector"]; !exists || len(fields) > 1 {
			profile := agentModelConfigProfile(config)
			result.AgentProfile = &profile
		}
	}
	return result, nil
}

func agentModelConfigProfile(r apitypes.AgentModelConfigRequest) agent.AgentProfile {
	profile := agentProfileFromAPI(&r.CreateAgentProfile)
	profile.APIProtocol = r.APIProtocol
	profile.Name = r.Name
	profile.Description = r.Description
	profile.Provider = r.Provider
	return profile
}

func (r recreateAgentRequest) updateRequest(current agentengine.Agent) (*agentengine.AgentUpdateRequest, error) {
	if r.Configuration == nil {
		if len(r.FieldMask) > 0 {
			return nil, fmt.Errorf("field_mask requires configuration")
		}
		return nil, nil
	}
	maskNames := map[string]string{"name": "name", "description": "description", "instructions": "instructions", "runtime": "runtime", "runtime.image": "image", "runtime.options": "runtime_options", "model_config": "agent_profile", "model_config.selector": "profile", "mcp_servers": "mcpServers"}
	mask := make([]string, 0, len(r.FieldMask))
	seen := make(map[string]bool, len(r.FieldMask))
	for _, field := range r.FieldMask {
		mapped, ok := maskNames[field]
		if !ok {
			return nil, fmt.Errorf("unsupported field_mask path %q", field)
		}
		for other := range seen {
			if strings.HasPrefix(field, other+".") || strings.HasPrefix(other, field+".") {
				return nil, fmt.Errorf("field_mask cannot combine %q and %q", field, other)
			}
		}
		if !seen[field] {
			mask = append(mask, mapped)
			seen[field] = true
		}
	}
	config := r.Configuration
	req := apitypes.CreateAgentRequest{Name: config.Name, Description: config.Description, Instructions: config.Instructions, Runtime: config.Runtime, ModelConfig: config.ModelConfig, MCPServers: config.MCPServers, MCPServersSet: config.MCPServers != nil}
	create := agentCreateRequestFromAPI(req)
	create.FieldMask = mask
	update := engineReplaceRequest(create, current)
	return &update, nil
}
