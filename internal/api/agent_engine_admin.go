package api

import (
	"maps"
	"slices"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/agentengine"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/utils"
)

func engineCreateRequest(request agent.CreateRequest) agentengine.AgentCreateRequest {
	return agentengine.AgentCreateRequest{
		ID:           request.Spec.ID,
		Spec:         engineSpecFromCreate(request.Spec),
		FromTemplate: request.Spec.FromTemplate,
	}
}

func engineSpecFromCreate(spec agent.CreateAgentSpec) agentengine.AgentSpec {
	runtimeConfig := spec.RuntimeConfig()
	model := engineModelFromProfile(spec.AgentProfile)
	model.Selector = spec.Profile
	return agentengine.AgentSpec{
		Name:         spec.Name,
		Description:  spec.Description,
		Instructions: spec.Instructions,
		Role:         engineRole(spec.Role),
		Runtime: agentengine.RuntimeSpec{
			Adapter:     runtimeConfig.Name,
			Sandboxed:   runtimeConfig.Sandboxed,
			Image:       spec.Image,
			Credentials: maps.Clone(spec.RuntimeCredentials),
			InitShell:   spec.RuntimeInitShell,
			Options:     utils.CloneAnyMap(spec.RuntimeOptions),
		},
		Model:        model,
		MCPServers:   engineMCPServers(spec.MCPServers),
		DesiredState: agentengine.AgentDesiredStateRunning,
	}
}

func engineUpdateRequest(request agent.UpdateRequest, resourceVersion string) agentengine.AgentUpdateRequest {
	result := agentengine.AgentUpdateRequest{ResourceVersion: resourceVersion}
	add := func(field string) {
		if !slices.Contains(result.FieldMask, field) {
			result.FieldMask = append(result.FieldMask, field)
		}
	}
	if request.Name != nil {
		result.Spec.Name = *request.Name
		add("name")
	}
	if request.Description != nil {
		result.Spec.Description = *request.Description
		add("description")
	}
	if request.Instructions != nil {
		result.Spec.Instructions = *request.Instructions
		add("instructions")
	}
	if request.Image != nil {
		result.Spec.Runtime.Image = *request.Image
		add("runtime.image")
	}
	if request.RuntimeSelectionRequested {
		result.Spec.Runtime.Adapter = request.RuntimeName
		if request.SandboxEnabled != nil {
			result.Spec.Runtime.Sandboxed = *request.SandboxEnabled
		}
		add("runtime.adapter")
		add("runtime.sandboxed")
	}
	if request.RuntimeOptions != nil {
		result.Spec.Runtime.Options = utils.CloneAnyMap(*request.RuntimeOptions)
		add("runtime.options")
	}
	if request.RuntimeCredentials != nil {
		result.Spec.Runtime.Credentials = maps.Clone(*request.RuntimeCredentials)
		add("runtime.credentials")
	}
	if request.RuntimeInitShell != nil {
		result.Spec.Runtime.InitShell = *request.RuntimeInitShell
		add("runtime.init_shell")
	}
	if request.AgentProfile != nil {
		result.Spec.Model = engineModelFromProfile(*request.AgentProfile)
		add("model")
	}
	if request.Profile != nil {
		result.Spec.Model.Selector = *request.Profile
		add("model")
	}
	if request.MCPServersSet || request.MCPServers != nil {
		if request.MCPServers != nil {
			result.Spec.MCPServers = engineMCPServers(*request.MCPServers)
		} else {
			result.Spec.MCPServers = nil
		}
		add("mcp_servers")
	}
	return result
}

func agentUpdateIncludes(request agentengine.AgentUpdateRequest, field string) bool {
	return slices.Contains(request.FieldMask, field)
}

func engineReplaceRequest(request agent.CreateRequest, current agentengine.Agent) agentengine.AgentUpdateRequest {
	result := agentengine.AgentUpdateRequest{Spec: engineSpecFromCreate(request.Spec), ResourceVersion: current.ResourceVersion}
	result.Spec.Role = current.Spec.Role
	for _, field := range request.FieldMask {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "id":
		case "name", "description", "instructions", "role":
			result.FieldMask = append(result.FieldMask, strings.ToLower(strings.TrimSpace(field)))
		case "image":
			result.FieldMask = append(result.FieldMask, "runtime.image")
		case "runtime":
			result.FieldMask = append(result.FieldMask, "runtime")
		case "runtime_options":
			result.FieldMask = append(result.FieldMask, "runtime.options")
		case "mcpservers":
			result.FieldMask = append(result.FieldMask, "mcp_servers")
		case "profile", "agent_profile":
			result.FieldMask = append(result.FieldMask, "model")
		}
	}
	if len(result.FieldMask) == 0 {
		if result.Spec.Runtime.Adapter == "" {
			result.Spec.Runtime = current.Spec.Runtime
		}
		if result.Spec.Role == "" {
			result.Spec.Role = current.Spec.Role
		}
		result.Spec.DesiredState = current.Spec.DesiredState
	}
	return result
}

func serviceAgentFromEngine(item agentengine.Agent) agent.Agent {
	profile := serviceProfileFromEngine(item.Status.Model)
	profile.ProfileComplete = item.Status.Model.ProfileComplete
	runtimeConfig := agentruntime.RuntimeConfig{Name: item.Spec.Runtime.Adapter, Sandboxed: item.Spec.Runtime.Sandboxed}.Normalized()
	out := agent.Agent{
		ID:               item.ID,
		Name:             item.Spec.Name,
		Description:      item.Spec.Description,
		Instructions:     item.Spec.Instructions,
		RuntimeID:        item.Status.RuntimeID,
		RuntimeKind:      item.Status.RuntimeKind,
		RuntimeName:      runtimeConfig.Name,
		SandboxEnabled:   runtimeConfig.Sandboxed,
		Image:            item.Spec.Runtime.Image,
		BoxID:            item.Status.SandboxID,
		RuntimeOptions:   utils.CloneAnyMap(item.Spec.Runtime.Options),
		Profile:          item.Spec.Model.Selector,
		MCPServers:       serviceMCPServers(item.Spec.MCPServers),
		Role:             serviceRole(item.Spec.Role),
		Status:           string(item.Status.State),
		CreatedAt:        item.CreatedAt,
		UpdatedAt:        item.UpdatedAt,
		AgentProfile:     profile,
		ProfileComplete:  item.Status.Model.ProfileComplete,
		DetectionResults: serviceDetectionResults(item.Status.Model.DetectionResults),
		StartupPending:   item.Status.StartupPending,
	}
	out.SetRuntimeConfig(runtimeConfig)
	if item.Status.Availability != nil {
		out.Availability = &agent.RuntimeAvailability{
			State:     agent.RuntimeAvailabilityState(item.Status.Availability.State),
			CheckedAt: item.Status.Availability.CheckedAt,
			ExpiresAt: item.Status.Availability.ExpiresAt,
			Reason:    item.Status.Availability.Reason,
		}
	}
	return out
}

func engineModelFromProfile(profile agent.AgentProfile) agentengine.ModelSpec {
	return agentengine.ModelSpec{
		Name:            profile.Name,
		Description:     profile.Description,
		Provider:        profile.Provider,
		ProviderID:      profile.ModelProviderID,
		BaseURL:         profile.BaseURL,
		APIKey:          profile.APIKey,
		Headers:         maps.Clone(profile.Headers),
		ModelID:         profile.ModelID,
		ReasoningEffort: profile.ReasoningEffort,
		FastMode:        profile.EnableFastMode,
		Options:         utils.CloneAnyMap(profile.RequestOptions),
		Env:             maps.Clone(profile.Env),
	}
}

func serviceProfileFromEngine(view agentengine.ModelView) agent.AgentProfile {
	profile := agent.AgentProfile{
		Name:                 view.Name,
		Description:          view.Description,
		Provider:             view.Provider,
		ModelProviderID:      view.ProviderID,
		BaseURL:              view.BaseURL,
		Headers:              maps.Clone(view.Headers),
		ModelID:              view.ModelID,
		ReasoningEffort:      view.ReasoningEffort,
		EnableFastMode:       view.FastMode,
		RequestOptions:       utils.CloneAnyMap(view.Options),
		Env:                  maps.Clone(view.Env),
		ProfileComplete:      view.ProfileComplete,
		EnvRestartRequired:   view.EnvRestartRequired,
		ImageUpgradeRequired: view.ImageUpgradeRequired,
	}
	if view.APIKeySet {
		profile.APIKey = "present"
		if len(view.APIKeyPreview) >= 4 {
			profile.APIKey = view.APIKeyPreview[:4] + "placeholder"
		}
	}
	return profile
}

func serviceProfileViewFromEngine(view agentengine.ModelView) agent.AgentProfileView {
	return agent.AgentProfileView{
		Name:                 view.Name,
		Description:          view.Description,
		Provider:             view.Provider,
		ModelProviderID:      view.ProviderID,
		BaseURL:              view.BaseURL,
		APIKeySet:            view.APIKeySet,
		APIKeyPreview:        view.APIKeyPreview,
		Headers:              maps.Clone(view.Headers),
		ModelID:              view.ModelID,
		ReasoningEffort:      view.ReasoningEffort,
		EnableFastMode:       view.FastMode,
		RequestOptions:       utils.CloneAnyMap(view.Options),
		Env:                  maps.Clone(view.Env),
		ProfileComplete:      view.ProfileComplete,
		EnvRestartRequired:   view.EnvRestartRequired,
		ImageUpgradeRequired: view.ImageUpgradeRequired,
		DetectionResults:     serviceDetectionResults(view.DetectionResults),
	}
}

func engineMCPServers(input map[string]any) map[string]agentengine.MCPServerConfig {
	if input == nil {
		return nil
	}
	out := make(map[string]agentengine.MCPServerConfig, len(input))
	for name, raw := range input {
		config, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		out[name] = agentengine.MCPServerConfig(utils.CloneAnyMap(config))
	}
	return out
}

func serviceMCPServers(input map[string]agentengine.MCPServerConfig) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for name, config := range input {
		out[name] = utils.CloneAnyMap(map[string]any(config))
	}
	return out
}

func engineRole(role string) agentengine.AgentRole {
	if role == agent.RoleManager {
		return agentengine.AgentRoleManager
	}
	return agentengine.AgentRoleWorker
}

func serviceRole(role agentengine.AgentRole) string {
	if role == agentengine.AgentRoleManager {
		return agent.RoleManager
	}
	return agent.RoleWorker
}

func serviceDetectionResults(input []agentengine.ProfileDetectionResult) []agent.ProfileDetectionResult {
	out := make([]agent.ProfileDetectionResult, 0, len(input))
	for _, item := range input {
		out = append(out, agent.ProfileDetectionResult{
			Provider: item.Provider,
			Status:   item.Status,
			ModelID:  item.ModelID,
			Error:    item.Error,
		})
	}
	return out
}
