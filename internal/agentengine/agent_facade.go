package agentengine

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"csgclaw/internal/agent"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/utils"
)

type unavailableAgents struct{}

func (unavailableAgents) Create(context.Context, AgentSpec) (Agent, error) {
	return Agent{}, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
}
func (unavailableAgents) Get(context.Context, string) (Agent, error) {
	return Agent{}, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
}
func (unavailableAgents) List(context.Context) ([]Agent, error) {
	return nil, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
}
func (unavailableAgents) Update(context.Context, string, AgentSpec) (Agent, error) {
	return Agent{}, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
}
func (unavailableAgents) Delete(context.Context, string) error {
	return &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
}
func (unavailableAgents) Start(context.Context, string) (Agent, error) {
	return Agent{}, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
}
func (unavailableAgents) Stop(context.Context, string) (Agent, error) {
	return Agent{}, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
}
func (unavailableAgents) Recreate(context.Context, string) (Agent, error) {
	return Agent{}, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
}

type agentFacade struct {
	service *agent.Service
}

func (f agentFacade) Create(ctx context.Context, spec AgentSpec) (Agent, error) {
	spec = normalizeAgentSpec(spec)
	if err := validateAgentSpec(spec); err != nil {
		return Agent{}, err
	}
	createdByCall := true
	if spec.Role == AgentRoleManager {
		_, existed := f.service.Agent(agent.ManagerUserID)
		createdByCall = !existed
	}
	created, err := f.service.Create(ctx, agent.CreateRequest{Spec: createAgentSpec(spec)})
	if err != nil {
		return Agent{}, err
	}
	if spec.Role == AgentRoleManager {
		created, err = f.service.Update(ctx, created.ID, updateAgentRequestForRole(spec, spec.Role))
		if err != nil {
			return Agent{}, err
		}
	}
	if err := f.service.ReplaceSkills(ctx, created.ID, spec.Skills); err != nil {
		if createdByCall {
			_ = f.service.Delete(context.Background(), created.ID)
		}
		return Agent{}, err
	}
	return f.convert(created)
}

func (f agentFacade) Get(ctx context.Context, agentID string) (Agent, error) {
	if f.service == nil {
		return Agent{}, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
	}
	agentID = strings.TrimSpace(agentID)
	selected, ok := f.service.Agent(agentID)
	if !ok {
		selected, ok = f.service.AgentByName(agentID)
	}
	if !ok {
		return Agent{}, &TurnError{Code: ErrorAgentUnavailable, Message: fmt.Sprintf("agent %q not found", agentID)}
	}
	return f.convert(selected)
}

func (f agentFacade) List(ctx context.Context) ([]Agent, error) {
	if f.service == nil {
		return nil, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
	}
	items := f.service.ListContext(ctx)
	out := make([]Agent, 0, len(items))
	for _, item := range items {
		converted, err := f.convert(item)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}
	return out, nil
}

func (f agentFacade) Update(ctx context.Context, agentID string, spec AgentSpec) (Agent, error) {
	spec = normalizeAgentSpec(spec)
	if err := validateAgentSpec(spec); err != nil {
		return Agent{}, err
	}
	var updated agent.Agent
	err := f.service.WithAgentLifecycle(ctx, agentID, func(lifecycleCtx context.Context) error {
		previous, ok := f.service.Agent(agentID)
		if !ok {
			return fmt.Errorf("agent %q not found", agentID)
		}
		previousRole := AgentRoleWorker
		if strings.EqualFold(strings.TrimSpace(previous.Role), agent.RoleManager) {
			previousRole = AgentRoleManager
		}
		if spec.Role != previousRole {
			return &TurnError{Code: ErrorInvalidRequest, Message: "agent role changes are not supported"}
		}
		previousSkills, err := f.service.Skills(agentID)
		if err != nil {
			return err
		}
		currentRuntime := previous.RuntimeConfig()
		desiredRuntime := createAgentSpec(spec).RuntimeConfig()
		replacesRuntime := currentRuntime != desiredRuntime
		if err := f.service.ReplaceSkills(lifecycleCtx, agentID, spec.Skills); err != nil {
			return err
		}
		if replacesRuntime {
			replacement := createAgentSpec(spec)
			replacement.ID = previous.ID
			updated, err = f.service.Create(lifecycleCtx, agent.CreateRequest{Spec: replacement, Replace: true})
		} else {
			updated, err = f.service.Update(lifecycleCtx, agentID, updateAgentRequestForRole(spec, AgentRole(previous.Role)))
		}
		if err != nil {
			_ = f.service.ReplaceSkills(lifecycleCtx, agentID, previousSkills)
			return err
		}
		if replacesRuntime {
			if err := f.service.ReplaceSkills(lifecycleCtx, agentID, spec.Skills); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Agent{}, err
	}
	return f.convert(updated)
}

func (f agentFacade) Delete(ctx context.Context, agentID string) error {
	return f.service.Delete(ctx, agentID)
}

func (f agentFacade) Start(ctx context.Context, agentID string) (Agent, error) {
	selected, err := f.service.Start(ctx, agentID)
	if err != nil {
		return Agent{}, err
	}
	return f.convert(selected)
}

func (f agentFacade) Stop(ctx context.Context, agentID string) (Agent, error) {
	selected, err := f.service.Stop(ctx, agentID)
	if err != nil {
		return Agent{}, err
	}
	return f.convert(selected)
}

func (f agentFacade) Recreate(ctx context.Context, agentID string) (Agent, error) {
	selected, err := f.service.Recreate(ctx, agentID)
	if err != nil {
		return Agent{}, err
	}
	return f.convert(selected)
}

func validateAgentSpec(spec AgentSpec) error {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Runtime.Adapter) == "" {
		return &TurnError{Code: ErrorInvalidRequest, Message: "agent name and Runtime adapter are required"}
	}
	if (len(spec.Runtime.Credentials) > 0 || strings.TrimSpace(spec.Runtime.InitShell) != "") && !strings.EqualFold(strings.TrimSpace(spec.Runtime.Adapter), agent.RuntimeNameCodex) {
		return &TurnError{Code: ErrorUnsupportedRuntimeProvision, Message: "Runtime credentials and initShell are supported only by the Codex Runtime Adapter"}
	}
	if spec.Role != AgentRoleWorker && spec.Role != AgentRoleManager {
		return &TurnError{Code: ErrorInvalidRequest, Message: "agent role must be worker or manager"}
	}
	return nil
}

func normalizeAgentSpec(spec AgentSpec) AgentSpec {
	if strings.TrimSpace(string(spec.Role)) == "" {
		spec.Role = AgentRoleWorker
	}
	spec.Role = AgentRole(strings.ToLower(strings.TrimSpace(string(spec.Role))))
	spec.Runtime.Adapter = strings.ToLower(strings.TrimSpace(spec.Runtime.Adapter))
	return spec
}

func createAgentSpec(spec AgentSpec) agent.CreateAgentSpec {
	result := agent.CreateAgentSpec{
		Name:               strings.TrimSpace(spec.Name),
		Description:        strings.TrimSpace(spec.Description),
		Instructions:       strings.TrimSpace(spec.Instructions),
		Image:              strings.TrimSpace(spec.Runtime.Image),
		Role:               string(spec.Role),
		RuntimeOptions:     utils.CloneAnyMap(spec.Runtime.Options),
		MCPServers:         mcpToService(spec.MCPServers),
		MCPServersSet:      true,
		RuntimeCredentials: maps.Clone(spec.Runtime.Credentials),
		RuntimeInitShell:   spec.Runtime.InitShell,
		AgentProfile:       modelToService(spec.Model),
	}
	result.SetRuntimeConfig(agentruntime.RuntimeConfig{Name: strings.TrimSpace(spec.Runtime.Adapter), Sandboxed: spec.Runtime.Sandboxed})
	return result
}

func updateAgentRequest(spec AgentSpec) agent.UpdateRequest {
	name := strings.TrimSpace(spec.Name)
	description := strings.TrimSpace(spec.Description)
	instructions := strings.TrimSpace(spec.Instructions)
	image := strings.TrimSpace(spec.Runtime.Image)
	sandboxed := spec.Runtime.Sandboxed
	runtimeOptions := utils.CloneAnyMap(spec.Runtime.Options)
	mcp := mcpToService(spec.MCPServers)
	credentials := maps.Clone(spec.Runtime.Credentials)
	initShell := spec.Runtime.InitShell
	profile := modelToService(spec.Model)
	return agent.UpdateRequest{
		Name:                      &name,
		Description:               &description,
		Instructions:              &instructions,
		Image:                     &image,
		RuntimeName:               strings.TrimSpace(spec.Runtime.Adapter),
		SandboxEnabled:            &sandboxed,
		RuntimeSelectionRequested: true,
		RuntimeOptions:            &runtimeOptions,
		MCPServers:                &mcp,
		MCPServersSet:             true,
		RuntimeCredentials:        &credentials,
		RuntimeInitShell:          &initShell,
		AgentProfile:              &profile,
		FieldMask: []string{
			"name", "description", "instructions", "image", "runtime", "runtime_options", "mcpservers", "runtime_credentials", "runtime_init_shell", "agent_profile",
		},
	}
}

func updateAgentRequestForRole(spec AgentSpec, role AgentRole) agent.UpdateRequest {
	request := updateAgentRequest(spec)
	if role != AgentRoleManager {
		return request
	}
	request.RuntimeOptions = nil
	fields := request.FieldMask[:0]
	for _, field := range request.FieldMask {
		if field != "runtime_options" {
			fields = append(fields, field)
		}
	}
	request.FieldMask = fields
	return request
}

func modelToService(spec ModelSpec) agent.AgentProfile {
	return agent.AgentProfile{
		ModelProviderID: strings.TrimSpace(spec.ProviderID),
		ModelID:         strings.TrimSpace(spec.ModelID),
		ReasoningEffort: strings.TrimSpace(spec.ReasoningEffort),
		EnableFastMode:  spec.FastMode,
		RequestOptions:  utils.CloneAnyMap(spec.Options),
		ProfileComplete: strings.TrimSpace(spec.ProviderID) != "" && strings.TrimSpace(spec.ModelID) != "",
	}
}

func mcpToService(input map[string]MCPServerConfig) map[string]any {
	out := make(map[string]any, len(input))
	for name, config := range input {
		out[name] = utils.CloneAnyMap(map[string]any(config))
	}
	return out
}

func (f agentFacade) convert(selected agent.Agent) (Agent, error) {
	skills, err := f.service.Skills(selected.ID)
	if err != nil {
		return Agent{}, err
	}
	mcp := make(map[string]MCPServerConfig, len(selected.MCPServers))
	for name, raw := range selected.MCPServers {
		config, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		mcp[name] = MCPServerConfig(utils.CloneAnyMap(config))
	}
	state := AgentState(strings.ToLower(strings.TrimSpace(selected.Status)))
	_, runtimeInitShell := selected.RuntimeProvision()
	role := AgentRoleWorker
	if strings.EqualFold(strings.TrimSpace(selected.Role), agent.RoleManager) {
		role = AgentRoleManager
	}
	runtimeAdapter := strings.TrimSpace(selected.RuntimeName)
	if runtimeAdapter == "" {
		runtimeAdapter = selected.RuntimeConfig().Name
	}
	ready := state == AgentStateRunning &&
		(selected.Availability == nil || selected.Availability.State == agent.RuntimeAvailabilityReady || selected.Availability.State == agent.RuntimeAvailabilityNotApplicable)
	return Agent{
		ID: selected.ID,
		Spec: AgentSpec{
			Name:         selected.Name,
			Description:  selected.Description,
			Instructions: selected.Instructions,
			Role:         role,
			Runtime: RuntimeSpec{
				Adapter:   runtimeAdapter,
				Sandboxed: selected.SandboxEnabled,
				Image:     selected.Image,
				Options:   utils.CloneAnyMap(selected.RuntimeOptions),
				InitShell: runtimeInitShell,
			},
			Model: ModelSpec{
				ProviderID:      selected.AgentProfile.ModelProviderID,
				ModelID:         selected.AgentProfile.ModelID,
				ReasoningEffort: selected.AgentProfile.ReasoningEffort,
				FastMode:        selected.AgentProfile.EnableFastMode,
				Options:         utils.CloneAnyMap(selected.AgentProfile.RequestOptions),
			},
			Skills:     append([]string(nil), skills...),
			MCPServers: mcp,
		},
		Status: AgentStatus{
			State:     state,
			RuntimeID: selected.RuntimeID,
			Ready:     ready,
		},
		CreatedAt: selected.CreatedAt,
		UpdatedAt: selected.UpdatedAt,
	}, nil
}
