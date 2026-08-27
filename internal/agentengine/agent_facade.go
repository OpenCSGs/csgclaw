package agentengine

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"csgclaw/internal/agent"
	agentruntime "csgclaw/internal/runtime"
	hub "csgclaw/internal/template"
	"csgclaw/internal/utils"
)

type localTemplateServiceContextKey struct{}

// WithLocalTemplateService preserves the HTTP adapter's request-scoped Hub
// selection without adding a concrete template client to public Engine request
// values. Remote Engine transports can perform the same resolution at their
// own authenticated boundary.
func WithLocalTemplateService(ctx context.Context, service *hub.Service) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, localTemplateServiceContextKey{}, service)
}

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

type agentFacade struct {
	service *agent.Service
	files   *FileStore
}

func (f agentFacade) Create(ctx context.Context, request AgentCreateRequest) (Agent, error) {
	requested := request.Spec
	spec := requested
	spec = normalizeAgentSpec(spec)
	if spec.Role == AgentRoleManager && strings.TrimSpace(spec.Runtime.Adapter) == "" {
		spec.Runtime.Adapter = agent.RuntimeNameCodex
	}
	if err := validateCreateAgentSpec(spec); err != nil {
		return Agent{}, err
	}
	createdByCall := true
	if spec.Role == AgentRoleManager {
		_, existed := f.service.Agent(agent.ManagerUserID)
		createdByCall = !existed
	}
	serviceRequest := agent.CreateRequest{Spec: createAgentSpec(spec)}
	serviceRequest.Spec.ID = strings.TrimSpace(request.ID)
	serviceRequest.Spec.FromTemplate = strings.TrimSpace(request.FromTemplate)
	if service, ok := ctx.Value(localTemplateServiceContextKey{}).(*hub.Service); ok {
		serviceRequest.HubService = service
	}
	created, err := f.service.Create(ctx, serviceRequest)
	if err != nil {
		return Agent{}, err
	}
	if spec.Role == AgentRoleManager {
		managerNeedsUpdate := strings.TrimSpace(requested.Description) != "" || strings.TrimSpace(requested.Instructions) != "" ||
			strings.TrimSpace(requested.Runtime.Adapter) != "" || modelSpecConfigured(requested.Model) || requested.MCPServers != nil
		if managerNeedsUpdate {
			managerSpec, specErr := f.specFromService(created, nil, true)
			if specErr != nil {
				return Agent{}, specErr
			}
			managerSpec.Name = spec.Name
			if strings.TrimSpace(requested.Description) != "" {
				managerSpec.Description = spec.Description
			}
			if strings.TrimSpace(requested.Instructions) != "" {
				managerSpec.Instructions = spec.Instructions
			}
			if strings.TrimSpace(requested.Runtime.Adapter) != "" {
				managerSpec.Runtime = spec.Runtime
			}
			if modelSpecConfigured(requested.Model) {
				managerSpec.Model = spec.Model
			}
			if requested.MCPServers != nil {
				managerSpec.MCPServers = spec.MCPServers
			}
			managerSpec.DesiredState = spec.DesiredState
			created, err = f.service.Update(ctx, created.ID, updateAgentRequestForRole(managerSpec, spec.Role))
			if err != nil {
				return Agent{}, err
			}
		}
	}
	if spec.Skills != nil {
		if err := f.service.ReplaceSkills(ctx, created.ID, spec.Skills); err != nil {
			if createdByCall {
				_ = f.service.Delete(context.Background(), created.ID)
			}
			return Agent{}, err
		}
	}
	if spec.DesiredState == AgentDesiredStateStopped {
		created, err = f.service.Stop(ctx, created.ID)
		if err != nil {
			return Agent{}, err
		}
	}
	created, err = f.service.SetDesiredState(created.ID, string(spec.DesiredState), false)
	if err != nil {
		return Agent{}, err
	}
	return f.convert(created)
}

func (f agentFacade) Get(ctx context.Context, agentID string, options AgentGetOptions) (Agent, error) {
	if f.service == nil {
		return Agent{}, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
	}
	if options.Reload {
		if err := f.service.Reload(); err != nil {
			return Agent{}, err
		}
	}
	agentID = strings.TrimSpace(agentID)
	if options.ProbeRuntime {
		selected, ok := f.service.Inspect(ctx, agentID)
		if !ok {
			return Agent{}, &TurnError{Code: ErrorAgentUnavailable, Message: fmt.Sprintf("agent %q not found", agentID)}
		}
		if options.IncludeDocuments {
			return f.convertWithDocuments(ctx, selected)
		}
		return f.convert(selected)
	}
	selected, ok := f.service.Agent(agentID)
	if !ok {
		selected, ok = f.service.AgentByName(agentID)
	}
	if !ok {
		return Agent{}, &TurnError{Code: ErrorAgentUnavailable, Message: fmt.Sprintf("agent %q not found", agentID)}
	}
	if options.IncludeDocuments {
		return f.convertWithDocuments(ctx, selected)
	}
	return f.convert(selected)
}

func (f agentFacade) List(ctx context.Context, options AgentListOptions) ([]Agent, error) {
	if f.service == nil {
		return nil, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is unavailable"}
	}
	if options.Reload {
		if err := f.service.Reload(); err != nil {
			return nil, err
		}
	}
	var items []agent.Agent
	if options.ProbeRuntime {
		items = f.service.ListContext(ctx)
	} else {
		items = f.service.List()
	}
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

func (f agentFacade) Update(ctx context.Context, agentID string, request AgentUpdateRequest) (Agent, error) {
	loadSkills := len(request.FieldMask) == 0 || fieldMaskContains(request.FieldMask, "skills")
	loadMemory := fieldMaskContains(request.FieldMask, "memory") || request.Spec.Memory != nil
	reconcileDesiredState := fieldMaskContains(request.FieldMask, "desired_state")
	return f.updateDesired(ctx, agentID, request.ResourceVersion, loadSkills, loadMemory, reconcileDesiredState, func(current AgentSpec) (AgentSpec, error) {
		return mergeAgentUpdate(current, request), nil
	})
}

func fieldMaskContains(fields []string, target string) bool {
	for _, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field), target) {
			return true
		}
	}
	return false
}

func mergeAgentUpdate(current AgentSpec, request AgentUpdateRequest) AgentSpec {
	if len(request.FieldMask) == 0 {
		return request.Spec
	}
	next := cloneAgentSpec(current)
	for _, rawField := range request.FieldMask {
		switch strings.ToLower(strings.TrimSpace(rawField)) {
		case "name":
			next.Name = request.Spec.Name
		case "description":
			next.Description = request.Spec.Description
		case "instructions":
			next.Instructions = request.Spec.Instructions
		case "role":
			next.Role = request.Spec.Role
		case "runtime":
			next.Runtime = request.Spec.Runtime
		case "runtime.adapter":
			next.Runtime.Adapter = request.Spec.Runtime.Adapter
		case "runtime.sandboxed":
			next.Runtime.Sandboxed = request.Spec.Runtime.Sandboxed
		case "runtime.image":
			next.Runtime.Image = request.Spec.Runtime.Image
		case "runtime.options":
			next.Runtime.Options = utils.CloneAnyMap(request.Spec.Runtime.Options)
		case "runtime.credentials":
			next.Runtime.Credentials = maps.Clone(request.Spec.Runtime.Credentials)
		case "runtime.init_shell":
			next.Runtime.InitShell = request.Spec.Runtime.InitShell
		case "model":
			next.Model = request.Spec.Model
		case "skills":
			next.Skills = append([]string(nil), request.Spec.Skills...)
		case "mcp_servers":
			next.MCPServers = cloneAgentSpec(AgentSpec{MCPServers: request.Spec.MCPServers}).MCPServers
		case "memory":
			if request.Spec.Memory == nil {
				next.Memory = nil
			} else {
				memory := *request.Spec.Memory
				next.Memory = &memory
			}
		case "desired_state":
			next.DesiredState = request.Spec.DesiredState
		}
	}
	return next
}

type agentSpecChange struct {
	name, description, instructions, image bool
	runtimeSelection, runtimeOptions       bool
	model, skills, mcpServers              bool
	memory                                 bool
	runtimeCredentials, runtimeInitShell   bool
	desiredState                           bool
}

func (c agentSpecChange) any() bool {
	return c.name || c.description || c.instructions || c.image ||
		c.runtimeSelection || c.runtimeOptions || c.model || c.skills ||
		c.mcpServers || c.memory || c.runtimeCredentials || c.runtimeInitShell || c.desiredState
}

func (c agentSpecChange) serviceUpdate() bool {
	return c.name || c.description || c.instructions || c.image ||
		c.runtimeOptions || c.model || c.mcpServers ||
		c.runtimeCredentials || c.runtimeInitShell
}

func diffAgentSpec(previous, desired AgentSpec) agentSpecChange {
	return agentSpecChange{
		name:               previous.Name != desired.Name,
		description:        previous.Description != desired.Description,
		instructions:       previous.Instructions != desired.Instructions,
		image:              previous.Runtime.Image != desired.Runtime.Image,
		runtimeSelection:   previous.Runtime.Adapter != desired.Runtime.Adapter || previous.Runtime.Sandboxed != desired.Runtime.Sandboxed,
		runtimeOptions:     !reflect.DeepEqual(previous.Runtime.Options, desired.Runtime.Options),
		model:              !reflect.DeepEqual(previous.Model, desired.Model),
		skills:             !slices.Equal(previous.Skills, desired.Skills),
		mcpServers:         !reflect.DeepEqual(previous.MCPServers, desired.MCPServers),
		memory:             !reflect.DeepEqual(previous.Memory, desired.Memory),
		runtimeCredentials: !maps.Equal(previous.Runtime.Credentials, desired.Runtime.Credentials),
		runtimeInitShell:   previous.Runtime.InitShell != desired.Runtime.InitShell,
		desiredState:       previous.DesiredState != desired.DesiredState,
	}
}

func diffSkillNames(previous, desired []string) (added, removed []string) {
	for _, name := range desired {
		if !slices.Contains(previous, name) {
			added = append(added, name)
		}
	}
	for _, name := range previous {
		if !slices.Contains(desired, name) {
			removed = append(removed, name)
		}
	}
	return added, removed
}

func preserveWriteOnlyFields(current, desired AgentSpec) AgentSpec {
	if desired.Runtime.Credentials == nil {
		desired.Runtime.Credentials = maps.Clone(current.Runtime.Credentials)
	}
	if strings.TrimSpace(desired.Model.APIKey) == "" {
		desired.Model.APIKey = current.Model.APIKey
	}
	return desired
}

func (f agentFacade) updateDesired(ctx context.Context, agentID, resourceVersion string, loadSkills, loadMemory, reconcileDesiredState bool, mutate func(AgentSpec) (AgentSpec, error)) (Agent, error) {
	var updated agent.Agent
	err := f.service.WithAgentLifecycle(ctx, agentID, func(lifecycleCtx context.Context) error {
		previous, ok := f.service.Agent(agentID)
		if !ok {
			return fmt.Errorf("agent %q not found", agentID)
		}
		if resourceVersion != "" && resourceVersion != resourceVersionForAgent(previous) {
			return &TurnError{Code: ErrorInvalidRequest, Message: "agent resource version is stale"}
		}
		var previousSkills []string
		var err error
		if loadSkills {
			previousSkills, err = f.service.Skills(agentID)
			if err != nil {
				return err
			}
		}
		current, err := f.specFromService(previous, previousSkills, true)
		if err != nil {
			return err
		}
		if loadMemory {
			document, memoryErr := f.service.MemoryDocument(lifecycleCtx, agentID)
			if memoryErr != nil {
				return memoryErr
			}
			current.Memory = &MemorySpec{Enabled: document.Enabled}
		}
		desired, err := mutate(cloneAgentSpec(current))
		if err != nil {
			return err
		}
		desired = normalizeAgentSpec(desired)
		desired = preserveWriteOnlyFields(current, desired)
		if err := validateAgentSpec(desired); err != nil {
			return err
		}
		if desired.Role != current.Role {
			return &TurnError{Code: ErrorInvalidRequest, Message: "agent role changes are not supported"}
		}
		change := diffAgentSpec(current, desired)
		if !change.any() && !reconcileDesiredState {
			updated = previous
			return nil
		}
		currentRuntime := previous.RuntimeConfig()
		desiredRuntime := createAgentSpec(desired).RuntimeConfig()
		replacesRuntime := change.runtimeSelection || currentRuntime != desiredRuntime
		if replacesRuntime {
			if err := f.service.ReplaceSkills(lifecycleCtx, agentID, desired.Skills); err != nil {
				return err
			}
		}
		if replacesRuntime {
			replacement := createAgentSpec(desired)
			replacement.ID = previous.ID
			updated, err = f.service.Create(lifecycleCtx, agent.CreateRequest{Spec: replacement, Replace: true})
		} else if change.serviceUpdate() {
			updated, err = f.service.Update(lifecycleCtx, agentID, updateAgentRequestForChanges(desired, change, current.Role))
		} else {
			updated = previous
		}
		if err != nil {
			if replacesRuntime {
				_ = f.service.ReplaceSkills(lifecycleCtx, agentID, previousSkills)
			}
			return err
		}
		if replacesRuntime {
			if err := f.service.ReplaceSkills(lifecycleCtx, agentID, desired.Skills); err != nil {
				return err
			}
		} else if change.skills {
			added, removed := diffSkillNames(previousSkills, desired.Skills)
			if len(added) > 0 {
				if err := f.service.BatchAddSkills(agentID, added); err != nil {
					return err
				}
			}
			for _, name := range removed {
				if err := f.service.DeleteSkill(agentID, name); err != nil {
					for _, addedName := range added {
						_ = f.service.DeleteSkill(agentID, addedName)
					}
					return err
				}
			}
		}
		if change.desiredState || reconcileDesiredState {
			switch desired.DesiredState {
			case AgentDesiredStateRunning:
				updated, err = f.service.Start(lifecycleCtx, agentID)
			case AgentDesiredStateStopped:
				updated, err = f.service.Stop(lifecycleCtx, agentID)
			}
			if err != nil {
				return err
			}
		}
		if change.memory {
			if desired.Memory == nil {
				return &TurnError{Code: ErrorInvalidRequest, Message: "memory configuration is required"}
			}
			if _, err = f.service.UpdateMemoryEnabled(lifecycleCtx, agentID, desired.Memory.Enabled); err != nil {
				return err
			}
			updated, _ = f.service.Agent(agentID)
		}
		skillOnly := change.skills && !replacesRuntime && !change.serviceUpdate() && !change.desiredState && !reconcileDesiredState && !change.memory
		updated, err = f.service.SetDesiredState(agentID, string(desired.DesiredState), skillOnly)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return Agent{}, err
	}
	return f.convert(updated)
}

func (f agentFacade) Delete(ctx context.Context, agentID string) error {
	canonicalID := strings.TrimSpace(agentID)
	if selected, ok := f.service.Agent(canonicalID); ok {
		canonicalID = selected.ID
	}
	if err := f.service.Delete(ctx, agentID); err != nil {
		return err
	}
	if f.files != nil {
		f.files.DeleteAgent(canonicalID)
	}
	return nil
}

func (f agentFacade) Recreate(ctx context.Context, agentID string, options AgentRecreateOptions) (Agent, error) {
	var (
		selected agent.Agent
		err      error
	)
	if options.UpgradeImage {
		selected, err = f.service.Upgrade(ctx, agentID)
	} else {
		selected, err = f.service.Recreate(ctx, agentID)
	}
	if err != nil {
		return Agent{}, err
	}
	selected, err = f.service.SetDesiredState(selected.ID, string(AgentDesiredStateRunning), false)
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
	if spec.Role == AgentRoleManager && runtimeOptionsContainMCP(spec.Runtime.Options) {
		return &TurnError{Code: ErrorInvalidRequest, Message: "runtime_options.mcp is not supported; use the MCP servers endpoint"}
	}
	return nil
}

func validateCreateAgentSpec(spec AgentSpec) error {
	if strings.TrimSpace(spec.Name) == "" {
		return &TurnError{Code: ErrorInvalidRequest, Message: "agent name is required"}
	}
	if strings.TrimSpace(spec.Runtime.Adapter) != "" &&
		(len(spec.Runtime.Credentials) > 0 || strings.TrimSpace(spec.Runtime.InitShell) != "") &&
		!strings.EqualFold(strings.TrimSpace(spec.Runtime.Adapter), agent.RuntimeNameCodex) {
		return &TurnError{Code: ErrorUnsupportedRuntimeProvision, Message: "Runtime credentials and initShell are supported only by the Codex Runtime Adapter"}
	}
	if spec.Role != AgentRoleWorker && spec.Role != AgentRoleManager {
		return &TurnError{Code: ErrorInvalidRequest, Message: "agent role must be worker or manager"}
	}
	if spec.Role == AgentRoleManager && runtimeOptionsContainMCP(spec.Runtime.Options) {
		return &TurnError{Code: ErrorInvalidRequest, Message: "runtime_options.mcp is not supported; use the MCP servers endpoint"}
	}
	return nil
}

func runtimeOptionsContainMCP(options map[string]any) bool {
	for key := range options {
		if strings.EqualFold(strings.TrimSpace(key), "mcp") {
			return true
		}
	}
	return false
}

func normalizeAgentSpec(spec AgentSpec) AgentSpec {
	if strings.TrimSpace(string(spec.Role)) == "" {
		spec.Role = AgentRoleWorker
	}
	spec.Role = AgentRole(strings.ToLower(strings.TrimSpace(string(spec.Role))))
	spec.Runtime.Adapter = strings.ToLower(strings.TrimSpace(spec.Runtime.Adapter))
	if spec.DesiredState == "" {
		spec.DesiredState = AgentDesiredStateRunning
	}
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
		MCPServersSet:      spec.MCPServers != nil,
		RuntimeCredentials: maps.Clone(spec.Runtime.Credentials),
		RuntimeInitShell:   spec.Runtime.InitShell,
		Profile:            strings.TrimSpace(spec.Model.Selector),
		AgentProfile:       modelToService(spec.Model),
	}
	result.SetRuntimeConfig(agentruntime.RuntimeConfig{Name: strings.TrimSpace(spec.Runtime.Adapter), Sandboxed: spec.Runtime.Sandboxed})
	return result
}

func modelSpecConfigured(spec ModelSpec) bool {
	return strings.TrimSpace(spec.Selector) != "" || strings.TrimSpace(spec.Provider) != "" ||
		strings.TrimSpace(spec.ProviderID) != "" || strings.TrimSpace(spec.BaseURL) != "" ||
		strings.TrimSpace(spec.APIKey) != "" || strings.TrimSpace(spec.ModelID) != "" ||
		strings.TrimSpace(spec.ReasoningEffort) != "" || spec.FastMode || len(spec.Headers) > 0 ||
		len(spec.Env) > 0 || len(spec.Options) > 0
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
	profileSelector := strings.TrimSpace(spec.Model.Selector)
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
		Profile:                   &profileSelector,
		AgentProfile:              &profile,
		FieldMask: []string{
			"name", "description", "instructions", "image", "runtime", "runtime_options", "mcpservers", "runtime_credentials", "runtime_init_shell", "profile", "agent_profile",
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

func updateAgentRequestForChanges(spec AgentSpec, change agentSpecChange, _ AgentRole) agent.UpdateRequest {
	request := agent.UpdateRequest{}
	addField := func(field string) {
		request.FieldMask = append(request.FieldMask, field)
	}
	if change.name {
		value := strings.TrimSpace(spec.Name)
		request.Name = &value
		addField("name")
	}
	if change.description {
		value := strings.TrimSpace(spec.Description)
		request.Description = &value
		addField("description")
	}
	if change.instructions {
		value := strings.TrimSpace(spec.Instructions)
		request.Instructions = &value
		addField("instructions")
	}
	if change.image {
		value := strings.TrimSpace(spec.Runtime.Image)
		request.Image = &value
		addField("image")
	}
	if change.runtimeOptions {
		options := utils.CloneAnyMap(spec.Runtime.Options)
		if spec.Runtime.Options != nil && options == nil {
			options = map[string]any{}
		}
		request.RuntimeOptions = &options
		addField("runtime_options")
	}
	if change.model {
		selector := strings.TrimSpace(spec.Model.Selector)
		profile := modelToService(spec.Model)
		request.Profile = &selector
		request.AgentProfile = &profile
		addField("profile")
		addField("agent_profile")
	}
	if change.mcpServers {
		servers := mcpToService(spec.MCPServers)
		request.MCPServers = &servers
		request.MCPServersSet = true
		addField("mcpservers")
	}
	if change.runtimeCredentials {
		credentials := maps.Clone(spec.Runtime.Credentials)
		request.RuntimeCredentials = &credentials
		addField("runtime_credentials")
	}
	if change.runtimeInitShell {
		initShell := spec.Runtime.InitShell
		request.RuntimeInitShell = &initShell
		addField("runtime_init_shell")
	}
	return request
}

func modelToService(spec ModelSpec) agent.AgentProfile {
	return agent.AgentProfile{
		Name:            strings.TrimSpace(spec.Name),
		Description:     strings.TrimSpace(spec.Description),
		Provider:        strings.TrimSpace(spec.Provider),
		ModelProviderID: strings.TrimSpace(spec.ProviderID),
		BaseURL:         strings.TrimSpace(spec.BaseURL),
		APIKey:          spec.APIKey,
		Headers:         maps.Clone(spec.Headers),
		ModelID:         strings.TrimSpace(spec.ModelID),
		ReasoningEffort: strings.TrimSpace(spec.ReasoningEffort),
		EnableFastMode:  spec.FastMode,
		RequestOptions:  utils.CloneAnyMap(spec.Options),
		Env:             maps.Clone(spec.Env),
		ProfileComplete: strings.TrimSpace(spec.ProviderID) != "" && strings.TrimSpace(spec.ModelID) != "",
	}
}

func modelFromService(profile agent.AgentProfile) ModelSpec {
	return ModelSpec{
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

func modelViewFromService(view agent.AgentProfileView) ModelView {
	results := make([]ProfileDetectionResult, 0, len(view.DetectionResults))
	for _, item := range view.DetectionResults {
		results = append(results, ProfileDetectionResult{
			Provider: item.Provider,
			Status:   item.Status,
			ModelID:  item.ModelID,
			Error:    item.Error,
		})
	}
	return ModelView{
		ModelSpec: ModelSpec{
			Name:            view.Name,
			Description:     view.Description,
			Provider:        view.Provider,
			ProviderID:      view.ModelProviderID,
			BaseURL:         view.BaseURL,
			Headers:         maps.Clone(view.Headers),
			ModelID:         view.ModelID,
			ReasoningEffort: view.ReasoningEffort,
			FastMode:        view.EnableFastMode,
			Env:             maps.Clone(view.Env),
			Options:         utils.CloneAnyMap(view.RequestOptions),
		},
		APIKeySet:            view.APIKeySet,
		APIKeyPreview:        view.APIKeyPreview,
		ProfileComplete:      view.ProfileComplete,
		EnvRestartRequired:   view.EnvRestartRequired,
		ImageUpgradeRequired: view.ImageUpgradeRequired,
		DetectionResults:     results,
	}
}

func mcpToService(input map[string]MCPServerConfig) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for name, config := range input {
		out[name] = utils.CloneAnyMap(map[string]any(config))
	}
	return out
}

func mcpFromService(input map[string]any) map[string]MCPServerConfig {
	if input == nil {
		return nil
	}
	out := make(map[string]MCPServerConfig, len(input))
	for name, raw := range input {
		config, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		out[name] = MCPServerConfig(utils.CloneAnyMap(config))
	}
	return out
}

func cloneAgentSpec(input AgentSpec) AgentSpec {
	input.Skills = append([]string(nil), input.Skills...)
	input.Runtime.Credentials = maps.Clone(input.Runtime.Credentials)
	input.Runtime.Options = utils.CloneAnyMap(input.Runtime.Options)
	input.Model.Headers = maps.Clone(input.Model.Headers)
	input.Model.Env = maps.Clone(input.Model.Env)
	input.Model.Options = utils.CloneAnyMap(input.Model.Options)
	input.MCPServers = mcpFromService(mcpToService(input.MCPServers))
	if input.Memory != nil {
		memory := *input.Memory
		input.Memory = &memory
	}
	return input
}

func resourceVersionForAgent(selected agent.Agent) string {
	if selected.UpdatedAt.IsZero() {
		return "0"
	}
	return selected.UpdatedAt.UTC().Format(time.RFC3339Nano)
}

func (f agentFacade) specFromService(selected agent.Agent, skills []string, includeSecrets bool) (AgentSpec, error) {
	_, runtimeInitShell := selected.RuntimeProvision()
	role := AgentRoleWorker
	if strings.EqualFold(strings.TrimSpace(selected.Role), agent.RoleManager) {
		role = AgentRoleManager
	}
	runtimeAdapter := strings.TrimSpace(selected.RuntimeName)
	if runtimeAdapter == "" {
		runtimeAdapter = selected.RuntimeConfig().Name
	}
	spec := AgentSpec{
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
			Selector:        selected.Profile,
			Name:            selected.AgentProfile.Name,
			Description:     selected.AgentProfile.Description,
			Provider:        selected.AgentProfile.Provider,
			ProviderID:      selected.AgentProfile.ModelProviderID,
			BaseURL:         selected.AgentProfile.BaseURL,
			Headers:         maps.Clone(selected.AgentProfile.Headers),
			ModelID:         selected.AgentProfile.ModelID,
			ReasoningEffort: selected.AgentProfile.ReasoningEffort,
			FastMode:        selected.AgentProfile.EnableFastMode,
			Env:             maps.Clone(selected.AgentProfile.Env),
			Options:         utils.CloneAnyMap(selected.AgentProfile.RequestOptions),
		},
		Skills:     append([]string(nil), skills...),
		MCPServers: mcpFromService(selected.MCPServers),
	}
	switch AgentDesiredState(strings.ToLower(strings.TrimSpace(selected.DesiredState))) {
	case AgentDesiredStateRunning:
		spec.DesiredState = AgentDesiredStateRunning
	case AgentDesiredStateStopped:
		spec.DesiredState = AgentDesiredStateStopped
	default:
		switch AgentState(strings.ToLower(strings.TrimSpace(selected.Status))) {
		case AgentStateStopped, AgentStateStopping, AgentStateFailed:
			spec.DesiredState = AgentDesiredStateStopped
		default:
			spec.DesiredState = AgentDesiredStateRunning
		}
	}
	if includeSecrets {
		credentials, _ := selected.RuntimeProvision()
		spec.Runtime.Credentials = credentials
		spec.Model.APIKey = selected.AgentProfile.APIKey
	}
	return spec, nil
}

func (f agentFacade) convert(selected agent.Agent) (Agent, error) {
	// Agent resource reads must remain available even when the configured
	// Runtime is temporarily missing. Skills are a Runtime-workspace projection,
	// so an unavailable projection is represented as empty instead of hiding the
	// persisted Agent resource.
	skills, _ := f.service.Skills(selected.ID)
	spec, err := f.specFromService(selected, skills, false)
	if err != nil {
		return Agent{}, err
	}
	state := AgentState(strings.ToLower(strings.TrimSpace(selected.Status)))
	ready := state == AgentStateRunning &&
		(selected.Availability == nil || selected.Availability.State == agent.RuntimeAvailabilityReady || selected.Availability.State == agent.RuntimeAvailabilityNotApplicable)
	modelView := modelViewFromService(agent.RedactedProfileViewForAgent(selected))
	modelView.ProfileComplete = selected.ProfileComplete || selected.AgentProfile.ProfileComplete
	var availability *RuntimeAvailability
	if selected.Availability != nil {
		availability = &RuntimeAvailability{
			State:     string(selected.Availability.State),
			CheckedAt: selected.Availability.CheckedAt,
			ExpiresAt: selected.Availability.ExpiresAt,
			Reason:    selected.Availability.Reason,
		}
	}
	return Agent{
		ID:              selected.ID,
		ResourceVersion: resourceVersionForAgent(selected),
		Spec:            spec,
		Status: AgentStatus{
			State:          state,
			RuntimeID:      selected.RuntimeID,
			RuntimeKind:    selected.RuntimeKind,
			SandboxID:      selected.BoxID,
			Ready:          ready,
			Availability:   availability,
			StartupPending: selected.StartupPending,
			Model:          modelView,
			Capabilities:   AgentCapabilities{Memory: f.service.SupportsMemory(selected.RuntimeKind)},
		},
		CreatedAt: selected.CreatedAt,
		UpdatedAt: selected.UpdatedAt,
	}, nil
}

func (f agentFacade) convertWithDocuments(ctx context.Context, selected agent.Agent) (Agent, error) {
	converted, err := f.convert(selected)
	if err != nil {
		return Agent{}, err
	}
	instructions, instructionsErr := f.service.InstructionsDocument(selected.ID)
	converted.Status.Instructions = &InstructionsStatus{}
	if instructionsErr != nil {
		converted.Status.Instructions.Error = instructionsErr.Error()
	} else {
		converted.Status.Instructions.Effective = instructions.Effective
	}
	memory, memoryErr := f.service.MemoryDocument(ctx, selected.ID)
	converted.Status.Memory = &MemoryStatus{}
	if memoryErr != nil {
		converted.Status.Memory.Error = memoryErr.Error()
	} else {
		converted.Spec.Memory = &MemorySpec{Enabled: memory.Enabled}
		converted.Status.Memory = &MemoryStatus{
			Enabled: memory.Enabled, Ready: memory.Ready, Name: memory.Name,
			Location: memory.Location, Content: memory.Content,
		}
	}
	return converted, nil
}
