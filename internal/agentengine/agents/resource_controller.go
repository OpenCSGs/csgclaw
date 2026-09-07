package agents

import (
	"context"
	"csgclaw/internal/agentengine/contract"
	agentruntime "csgclaw/internal/runtime"
	hub "csgclaw/internal/template"
	"csgclaw/internal/utils"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"
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

func (f *Controller) Create(ctx context.Context, request contract.AgentCreateRequest) (contract.Agent, error) {
	requested := request.Spec
	spec := requested
	spec = normalizeAgentSpec(spec)
	if spec.Role == contract.AgentRoleManager && strings.TrimSpace(spec.Runtime.Adapter) == "" {
		spec.Runtime.Adapter = RuntimeNameCodex
	}
	if err := validateCreateAgentSpec(spec); err != nil {
		return contract.Agent{}, err
	}
	createdByCall := true
	if spec.Role == contract.AgentRoleManager {
		_, existed := f.Agent(ManagerUserID)
		createdByCall = !existed
	}
	serviceRequest := CreateRequest{Spec: createAgentSpec(spec)}
	serviceRequest.Spec.ID = strings.TrimSpace(request.ID)
	serviceRequest.Spec.FromTemplate = strings.TrimSpace(request.FromTemplate)
	if service, ok := ctx.Value(localTemplateServiceContextKey{}).(*hub.Service); ok {
		serviceRequest.HubService = service
	}
	created, err := f.CreateRecord(ctx, serviceRequest)
	if err != nil {
		return contract.Agent{}, err
	}
	if spec.Role == contract.AgentRoleManager {
		managerNeedsUpdate := strings.TrimSpace(requested.Description) != "" || strings.TrimSpace(requested.Instructions) != "" ||
			strings.TrimSpace(requested.Runtime.Adapter) != "" || modelSpecConfigured(requested.Model) || requested.MCPServers != nil
		if managerNeedsUpdate {
			managerSpec, specErr := f.specFromService(created, nil, true)
			if specErr != nil {
				return contract.Agent{}, specErr
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
			created, err = f.UpdateRecord(ctx, created.ID, updateAgentRequestForRole(managerSpec, spec.Role))
			if err != nil {
				return contract.Agent{}, err
			}
		}
	}
	if spec.Skills != nil {
		if err := f.ReplaceSkills(ctx, created.ID, spec.Skills); err != nil {
			if createdByCall {
				_ = f.DeleteRecord(context.Background(), created.ID)
			}
			return contract.Agent{}, err
		}
	}
	if spec.Memory != nil {
		if _, err := f.UpdateMemoryEnabled(ctx, created.ID, spec.Memory.Enabled); err != nil {
			if createdByCall {
				if rollbackErr := f.DeleteRecord(context.Background(), created.ID); rollbackErr != nil {
					return contract.Agent{}, errors.Join(err, fmt.Errorf("rollback Agent after memory reconciliation failure: %w", rollbackErr))
				}
			}
			return contract.Agent{}, err
		}
		created, _ = f.Agent(created.ID)
	}
	if spec.DesiredState == contract.AgentDesiredStateStopped {
		created, err = f.Stop(ctx, created.ID)
		if err != nil {
			return contract.Agent{}, err
		}
	}
	created, err = f.SetDesiredState(created.ID, string(spec.DesiredState), false)
	if err != nil {
		return contract.Agent{}, err
	}
	converted, err := f.convert(created)
	if err != nil {
		return contract.Agent{}, err
	}
	if spec.Memory != nil {
		memory := *spec.Memory
		converted.Spec.Memory = &memory
	}
	return converted, nil
}

func (f *Controller) Get(ctx context.Context, agentID string, options contract.AgentGetOptions) (contract.Agent, error) {
	if f == nil {
		return contract.Agent{}, &contract.TurnError{Code: contract.ErrorAgentUnavailable, Message: "agent service is unavailable"}
	}
	if options.Reload {
		if err := f.Reload(); err != nil {
			return contract.Agent{}, err
		}
	}
	agentID = strings.TrimSpace(agentID)
	if options.ProbeRuntime {
		selected, ok := f.Inspect(ctx, agentID)
		if !ok {
			return contract.Agent{}, &contract.TurnError{Code: contract.ErrorAgentNotFound, Message: fmt.Sprintf("agent %q not found", agentID)}
		}
		var err error
		selected, err = f.withAdoptedMCPServers(ctx, selected, options.AdoptMCPServers)
		if err != nil {
			return contract.Agent{}, err
		}
		if options.IncludeDocuments {
			return f.convertWithDocuments(ctx, selected)
		}
		return f.convert(selected)
	}
	selected, ok := f.agentSnapshot(agentID)
	if !ok {
		selected, ok = f.agentSnapshotByName(agentID)
	}
	if !ok {
		return contract.Agent{}, &contract.TurnError{Code: contract.ErrorAgentNotFound, Message: fmt.Sprintf("agent %q not found", agentID)}
	}
	var err error
	selected, err = f.withAdoptedMCPServers(ctx, selected, options.AdoptMCPServers)
	if err != nil {
		return contract.Agent{}, err
	}
	if options.IncludeDocuments {
		return f.convertWithDocuments(ctx, selected)
	}
	return f.convert(selected)
}

func (f *Controller) withAdoptedMCPServers(ctx context.Context, selected Agent, adopt bool) (Agent, error) {
	if !adopt || selected.MCPServers != nil {
		return selected, nil
	}
	view, err := f.MCPServersView(ctx, selected.ID)
	if err != nil {
		return Agent{}, err
	}
	selected.MCPServers = view.Servers
	return selected, nil
}

func (f *Controller) List(ctx context.Context, options contract.AgentListOptions) ([]contract.Agent, error) {
	if f == nil {
		return nil, &contract.TurnError{Code: contract.ErrorAgentUnavailable, Message: "agent service is unavailable"}
	}
	if options.Reload {
		if err := f.Reload(); err != nil {
			return nil, err
		}
	}
	var items []Agent
	if options.ProbeRuntime {
		items = f.ListRecordsContext(ctx)
	} else {
		items = f.ListRecords()
	}
	out := make([]contract.Agent, 0, len(items))
	for _, item := range items {
		converted, err := f.convert(item)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}
	return out, nil
}

func (f *Controller) Update(ctx context.Context, agentID string, request contract.AgentUpdateRequest) (contract.Agent, error) {
	loadSkills := len(request.FieldMask) == 0 || fieldMaskContains(request.FieldMask, "skills")
	loadMemory := fieldMaskContains(request.FieldMask, "memory") || request.Spec.Memory != nil
	reconcileDesiredState := fieldMaskContains(request.FieldMask, "desired_state")
	item, err := f.updateDesired(ctx, agentID, request.ResourceVersion, loadSkills, loadMemory, reconcileDesiredState, false, func(current contract.AgentSpec) (contract.AgentSpec, error) {
		return mergeAgentUpdate(current, request), nil
	})
	if err == nil && item.Status.State == contract.AgentStateStopped && f.interactions != nil {
		f.interactions.Interrupt(agentID, "", "", true)
	}
	return item, err
}

func fieldMaskContains(fields []string, target string) bool {
	for _, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field), target) {
			return true
		}
	}
	return false
}

func mergeAgentUpdate(current contract.AgentSpec, request contract.AgentUpdateRequest) contract.AgentSpec {
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
		case "model.selector":
			next.Model.Selector = request.Spec.Model.Selector
		case "skills":
			next.Skills = append([]string(nil), request.Spec.Skills...)
		case "mcp_servers":
			next.MCPServers = cloneAgentSpec(contract.AgentSpec{MCPServers: request.Spec.MCPServers}).MCPServers
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
	modelSelector, modelConfig             bool
	skills, mcpServers                     bool
	memory                                 bool
	runtimeCredentials, runtimeInitShell   bool
	desiredState                           bool
}

func (c agentSpecChange) any() bool {
	return c.name || c.description || c.instructions || c.image ||
		c.runtimeSelection || c.runtimeOptions || c.modelSelector || c.modelConfig || c.skills ||
		c.mcpServers || c.memory || c.runtimeCredentials || c.runtimeInitShell || c.desiredState
}

func (c agentSpecChange) serviceUpdate() bool {
	return c.name || c.description || c.instructions || c.image ||
		c.runtimeOptions || c.modelSelector || c.modelConfig || c.mcpServers ||
		c.runtimeCredentials || c.runtimeInitShell
}

func diffAgentSpec(previous, desired contract.AgentSpec) agentSpecChange {
	previousModelConfig := previous.Model
	previousModelConfig.Selector = ""
	desiredModelConfig := desired.Model
	desiredModelConfig.Selector = ""
	return agentSpecChange{
		name:               previous.Name != desired.Name,
		description:        previous.Description != desired.Description,
		instructions:       previous.Instructions != desired.Instructions,
		image:              previous.Runtime.Image != desired.Runtime.Image,
		runtimeSelection:   previous.Runtime.Adapter != desired.Runtime.Adapter || previous.Runtime.Sandboxed != desired.Runtime.Sandboxed,
		runtimeOptions:     !reflect.DeepEqual(previous.Runtime.Options, desired.Runtime.Options),
		modelSelector:      previous.Model.Selector != desired.Model.Selector,
		modelConfig:        !reflect.DeepEqual(previousModelConfig, desiredModelConfig),
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

func preserveWriteOnlyFields(current, desired contract.AgentSpec) contract.AgentSpec {
	if desired.Runtime.Credentials == nil {
		desired.Runtime.Credentials = maps.Clone(current.Runtime.Credentials)
	}
	if strings.TrimSpace(desired.Model.APIKey) == "" {
		desired.Model.APIKey = current.Model.APIKey
	}
	return desired
}

func (f *Controller) updateDesired(ctx context.Context, agentID, resourceVersion string, loadSkills, loadMemory, reconcileDesiredState, forceRecreate bool, mutate func(contract.AgentSpec) (contract.AgentSpec, error)) (contract.Agent, error) {
	var updated Agent
	err := f.WithAgentLifecycle(ctx, agentID, func(lifecycleCtx context.Context) error {
		previous, ok := f.Agent(agentID)
		if !ok {
			return fmt.Errorf("agent %q not found", agentID)
		}
		if resourceVersion != "" && resourceVersion != resourceVersionForAgent(previous) {
			return &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: "agent resource version is stale"}
		}
		var previousSkills []string
		var err error
		if loadSkills {
			previousSkills, err = f.Skills(agentID)
			if err != nil {
				return err
			}
		}
		current, err := f.specFromService(previous, previousSkills, true)
		if err != nil {
			return err
		}
		if loadMemory {
			document, memoryErr := f.MemoryDocument(lifecycleCtx, agentID)
			if memoryErr != nil {
				return memoryErr
			}
			current.Memory = &contract.MemorySpec{Enabled: document.Enabled}
		}
		desired, err := mutate(cloneAgentSpec(current))
		if err != nil {
			return err
		}
		desired = normalizeAgentSpec(desired)
		if forceRecreate {
			desired.DesiredState = contract.AgentDesiredStateRunning
		}
		desired = preserveWriteOnlyFields(current, desired)
		if err := validateAgentSpec(desired); err != nil {
			return err
		}
		if desired.Role != current.Role {
			return &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: "agent role changes are not supported"}
		}
		change := diffAgentSpec(current, desired)
		if !change.any() && !reconcileDesiredState && !forceRecreate {
			updated = previous
			return nil
		}
		currentRuntime := previous.RuntimeConfig()
		desiredRuntime := createAgentSpec(desired).RuntimeConfig()
		replacesRuntime := change.runtimeSelection || currentRuntime != desiredRuntime
		if replacesRuntime {
			if err := f.ReplaceSkills(lifecycleCtx, agentID, desired.Skills); err != nil {
				return err
			}
		}
		if replacesRuntime {
			replacement := createAgentSpec(desired)
			replacement.ID = previous.ID
			updated, err = f.CreateRecord(lifecycleCtx, CreateRequest{Spec: replacement, Replace: true})
		} else if change.serviceUpdate() {
			updated, err = f.UpdateRecord(lifecycleCtx, agentID, updateAgentRequestForChanges(desired, change, current.Role))
		} else {
			updated = previous
		}
		if err != nil {
			if replacesRuntime {
				_ = f.ReplaceSkills(lifecycleCtx, agentID, previousSkills)
			}
			return err
		}
		if replacesRuntime {
			if err := f.ReplaceSkills(lifecycleCtx, agentID, desired.Skills); err != nil {
				return err
			}
		} else if change.skills {
			added, removed := diffSkillNames(previousSkills, desired.Skills)
			if len(added) > 0 {
				if err := f.BatchAddSkills(agentID, added); err != nil {
					return err
				}
			}
			for _, name := range removed {
				if err := f.DeleteSkill(agentID, name); err != nil {
					for _, addedName := range added {
						_ = f.DeleteSkill(agentID, addedName)
					}
					return err
				}
			}
		}
		if (change.desiredState || reconcileDesiredState) && !forceRecreate {
			switch desired.DesiredState {
			case contract.AgentDesiredStateRunning:
				updated, err = f.Start(lifecycleCtx, agentID)
			case contract.AgentDesiredStateStopped:
				updated, err = f.Stop(lifecycleCtx, agentID)
			}
			if err != nil {
				return err
			}
		}
		if change.memory {
			if desired.Memory == nil {
				return &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: "memory configuration is required"}
			}
			if _, err = f.UpdateMemoryEnabled(lifecycleCtx, agentID, desired.Memory.Enabled); err != nil {
				return err
			}
			updated, _ = f.Agent(agentID)
		}
		if forceRecreate && !replacesRuntime {
			updated, err = f.RecreateRecord(lifecycleCtx, agentID)
			if err != nil {
				return err
			}
		}
		skillOnly := change.skills && !replacesRuntime && !change.serviceUpdate() && !change.desiredState && !reconcileDesiredState && !change.memory && !forceRecreate
		updated, err = f.SetDesiredState(agentID, string(desired.DesiredState), skillOnly)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return contract.Agent{}, err
	}
	return f.convert(updated)
}

func (f *Controller) Delete(ctx context.Context, agentID string) error {
	ctx, release, err := f.acquireAgentLifecycle(ctx, agentID)
	if err != nil {
		return err
	}
	defer release()
	if f.extensions != nil {
		if err := f.extensions.DeleteExtensions(ctx, agentID); err != nil {
			return err
		}
	}
	if f.interactions != nil {
		f.interactions.Interrupt(agentID, "", "", true)
	}
	canonicalID := strings.TrimSpace(agentID)
	if selected, ok := f.Agent(canonicalID); ok {
		canonicalID = selected.ID
	}
	if err := f.DeleteRecord(ctx, agentID); err != nil {
		return err
	}
	if f.files != nil {
		f.files.DeleteAgent(canonicalID)
	}
	return nil
}

func (f *Controller) Recreate(ctx context.Context, agentID string, options contract.AgentRecreateOptions) (result contract.Agent, err error) {
	ctx, release, err := f.acquireAgentLifecycle(ctx, agentID)
	if err != nil {
		return result, err
	}
	defer release()
	defer func() {
		if err == nil && f.interactions != nil {
			f.interactions.Interrupt(agentID, "", "", true)
		}
	}()
	if options.Update != nil {
		if options.UpgradeImage {
			return contract.Agent{}, &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: "recreate update and image upgrade cannot be combined"}
		}
		request := *options.Update
		loadSkills := len(request.FieldMask) == 0 || fieldMaskContains(request.FieldMask, "skills")
		loadMemory := fieldMaskContains(request.FieldMask, "memory") || request.Spec.Memory != nil
		return f.updateDesired(ctx, agentID, request.ResourceVersion, loadSkills, loadMemory, false, true, func(current contract.AgentSpec) (contract.AgentSpec, error) {
			return mergeAgentUpdate(current, request), nil
		})
	}
	var selected Agent
	if options.UpgradeImage {
		selected, err = f.Upgrade(ctx, agentID)
	} else {
		selected, err = f.RecreateRecord(ctx, agentID)
	}
	if err != nil {
		return contract.Agent{}, err
	}
	selected, err = f.SetDesiredState(selected.ID, string(contract.AgentDesiredStateRunning), false)
	if err != nil {
		return contract.Agent{}, err
	}
	return f.convert(selected)
}

func validateAgentSpec(spec contract.AgentSpec) error {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Runtime.Adapter) == "" {
		return &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: "agent name and Runtime adapter are required"}
	}
	if (len(spec.Runtime.Credentials) > 0 || strings.TrimSpace(spec.Runtime.InitShell) != "") && !strings.EqualFold(strings.TrimSpace(spec.Runtime.Adapter), RuntimeNameCodex) {
		return &contract.TurnError{Code: contract.ErrorUnsupportedRuntimeProvision, Message: "Runtime credentials and initShell are supported only by the Codex Runtime Adapter"}
	}
	if spec.Role != contract.AgentRoleWorker && spec.Role != contract.AgentRoleManager {
		return &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: "agent role must be worker or manager"}
	}
	if spec.Role == contract.AgentRoleManager && runtimeOptionsContainMCP(spec.Runtime.Options) {
		return &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: "runtime_options.mcp is not supported; use the MCP servers endpoint"}
	}
	return nil
}

func validateCreateAgentSpec(spec contract.AgentSpec) error {
	if strings.TrimSpace(spec.Name) == "" {
		return &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: "agent name is required"}
	}
	if strings.TrimSpace(spec.Runtime.Adapter) != "" &&
		(len(spec.Runtime.Credentials) > 0 || strings.TrimSpace(spec.Runtime.InitShell) != "") &&
		!strings.EqualFold(strings.TrimSpace(spec.Runtime.Adapter), RuntimeNameCodex) {
		return &contract.TurnError{Code: contract.ErrorUnsupportedRuntimeProvision, Message: "Runtime credentials and initShell are supported only by the Codex Runtime Adapter"}
	}
	if spec.Role != contract.AgentRoleWorker && spec.Role != contract.AgentRoleManager {
		return &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: "agent role must be worker or manager"}
	}
	if spec.Role == contract.AgentRoleManager && runtimeOptionsContainMCP(spec.Runtime.Options) {
		return &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: "runtime_options.mcp is not supported; use the MCP servers endpoint"}
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

func normalizeAgentSpec(spec contract.AgentSpec) contract.AgentSpec {
	if strings.TrimSpace(string(spec.Role)) == "" {
		spec.Role = contract.AgentRoleWorker
	}
	spec.Role = contract.AgentRole(strings.ToLower(strings.TrimSpace(string(spec.Role))))
	spec.Runtime.Adapter = strings.ToLower(strings.TrimSpace(spec.Runtime.Adapter))
	if spec.DesiredState == "" {
		spec.DesiredState = contract.AgentDesiredStateRunning
	}
	return spec
}

func createAgentSpec(spec contract.AgentSpec) CreateAgentSpec {
	result := CreateAgentSpec{
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

func modelSpecConfigured(spec contract.ModelSpec) bool {
	return strings.TrimSpace(spec.Selector) != "" || strings.TrimSpace(spec.Provider) != "" ||
		strings.TrimSpace(spec.ProviderID) != "" || strings.TrimSpace(spec.BaseURL) != "" ||
		strings.TrimSpace(spec.APIKey) != "" || strings.TrimSpace(spec.ModelID) != "" ||
		strings.TrimSpace(spec.ReasoningEffort) != "" || spec.FastMode || len(spec.Headers) > 0 ||
		len(spec.Env) > 0 || len(spec.Options) > 0
}

func updateAgentRequest(spec contract.AgentSpec) UpdateRequest {
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
	return UpdateRequest{
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

func updateAgentRequestForRole(spec contract.AgentSpec, role contract.AgentRole) UpdateRequest {
	request := updateAgentRequest(spec)
	if role != contract.AgentRoleManager {
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

func updateAgentRequestForChanges(spec contract.AgentSpec, change agentSpecChange, _ contract.AgentRole) UpdateRequest {
	request := UpdateRequest{}
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
	if change.modelSelector {
		selector := strings.TrimSpace(spec.Model.Selector)
		request.Profile = &selector
		addField("profile")
	}
	if change.modelConfig {
		profile := modelToService(spec.Model)
		request.AgentProfile = &profile
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

func modelToService(spec contract.ModelSpec) AgentProfile {
	return AgentProfile{
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

func modelFromService(profile AgentProfile) contract.ModelSpec {
	return contract.ModelSpec{
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

func modelViewFromService(view AgentProfileView) contract.ModelView {
	results := make([]contract.ProfileDetectionResult, 0, len(view.DetectionResults))
	for _, item := range view.DetectionResults {
		results = append(results, contract.ProfileDetectionResult{
			Provider: item.Provider,
			Status:   item.Status,
			ModelID:  item.ModelID,
			Error:    item.Error,
		})
	}
	return contract.ModelView{
		ModelSpec: contract.ModelSpec{
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

func mcpToService(input map[string]contract.MCPServerConfig) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for name, config := range input {
		out[name] = utils.CloneAnyMap(map[string]any(config))
	}
	return out
}

func mcpFromService(input map[string]any) map[string]contract.MCPServerConfig {
	if input == nil {
		return nil
	}
	out := make(map[string]contract.MCPServerConfig, len(input))
	for name, raw := range input {
		config, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		out[name] = contract.MCPServerConfig(utils.CloneAnyMap(config))
	}
	return out
}

func cloneAgentSpec(input contract.AgentSpec) contract.AgentSpec {
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

func resourceVersionForAgent(selected Agent) string {
	if selected.UpdatedAt.IsZero() {
		return "0"
	}
	return selected.UpdatedAt.UTC().Format(time.RFC3339Nano)
}

func (f *Controller) specFromService(selected Agent, skills []string, includeSecrets bool) (contract.AgentSpec, error) {
	_, runtimeInitShell := selected.RuntimeProvision()
	role := contract.AgentRoleWorker
	if strings.EqualFold(strings.TrimSpace(selected.Role), RoleManager) {
		role = contract.AgentRoleManager
	}
	runtimeAdapter := strings.TrimSpace(selected.RuntimeName)
	if runtimeAdapter == "" {
		runtimeAdapter = selected.RuntimeConfig().Name
	}
	spec := contract.AgentSpec{
		Name:         selected.Name,
		Description:  selected.Description,
		Instructions: selected.Instructions,
		Role:         role,
		Runtime: contract.RuntimeSpec{
			Adapter:   runtimeAdapter,
			Sandboxed: selected.SandboxEnabled,
			Image:     selected.Image,
			Options:   utils.CloneAnyMap(selected.RuntimeOptions),
			InitShell: runtimeInitShell,
		},
		Model: contract.ModelSpec{
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
	switch contract.AgentDesiredState(strings.ToLower(strings.TrimSpace(selected.DesiredState))) {
	case contract.AgentDesiredStateRunning:
		spec.DesiredState = contract.AgentDesiredStateRunning
	case contract.AgentDesiredStateStopped:
		spec.DesiredState = contract.AgentDesiredStateStopped
	default:
		switch contract.AgentState(strings.ToLower(strings.TrimSpace(selected.Status))) {
		case contract.AgentStateStopped, contract.AgentStateStopping, contract.AgentStateFailed:
			spec.DesiredState = contract.AgentDesiredStateStopped
		default:
			spec.DesiredState = contract.AgentDesiredStateRunning
		}
	}
	if includeSecrets {
		credentials, _ := selected.RuntimeProvision()
		spec.Runtime.Credentials = credentials
		spec.Model.APIKey = selected.AgentProfile.APIKey
	}
	return spec, nil
}

func (f *Controller) convert(selected Agent) (contract.Agent, error) {
	// Agent resource reads must remain available even when the configured
	// Runtime is temporarily missing. Skills are a Runtime-workspace projection,
	// so an unavailable projection is represented as empty instead of hiding the
	// persisted Agent resource.
	skills, _ := f.Skills(selected.ID)
	spec, err := f.specFromService(selected, skills, false)
	if err != nil {
		return contract.Agent{}, err
	}
	state := contract.AgentState(strings.ToLower(strings.TrimSpace(selected.Status)))
	ready := state == contract.AgentStateRunning &&
		(selected.Availability == nil || selected.Availability.State == RuntimeAvailabilityReady || selected.Availability.State == RuntimeAvailabilityNotApplicable)
	modelView := modelViewFromService(RedactedProfileViewForAgent(selected))
	modelView.ProfileComplete = selected.ProfileComplete || selected.AgentProfile.ProfileComplete
	var availability *contract.RuntimeAvailability
	if selected.Availability != nil {
		availability = &contract.RuntimeAvailability{
			State:     string(selected.Availability.State),
			CheckedAt: selected.Availability.CheckedAt,
			ExpiresAt: selected.Availability.ExpiresAt,
			Reason:    selected.Availability.Reason,
		}
	}
	message := ""
	if f.extensions != nil {
		if err := f.extensions.RuntimeReady(selected.ID); err != nil {
			ready = false
			message = err.Error()
		}
	}
	return contract.Agent{
		ID:              selected.ID,
		ResourceVersion: resourceVersionForAgent(selected),
		Spec:            spec,
		Status: contract.AgentStatus{
			State:          state,
			RuntimeID:      selected.RuntimeID,
			RuntimeKind:    selected.RuntimeKind,
			SandboxID:      selected.BoxID,
			Ready:          ready,
			Message:        message,
			Availability:   availability,
			StartupPending: selected.StartupPending,
			Model:          modelView,
			Capabilities:   contract.AgentCapabilities{Memory: f.SupportsMemory(selected.RuntimeKind)},
		},
		CreatedAt: selected.CreatedAt,
		UpdatedAt: selected.UpdatedAt,
	}, nil
}

func (f *Controller) convertWithDocuments(ctx context.Context, selected Agent) (contract.Agent, error) {
	converted, err := f.convert(selected)
	if err != nil {
		return contract.Agent{}, err
	}
	instructions, instructionsErr := f.InstructionsDocument(selected.ID)
	converted.Status.Instructions = &contract.InstructionsStatus{}
	if instructionsErr != nil {
		converted.Status.Instructions.Error = instructionsErr.Error()
	} else {
		converted.Status.Instructions.Effective = instructions.Effective
	}
	memory, memoryErr := f.MemoryDocument(ctx, selected.ID)
	converted.Status.Memory = &contract.MemoryStatus{}
	if memoryErr != nil {
		converted.Status.Memory.Error = memoryErr.Error()
	} else {
		converted.Spec.Memory = &contract.MemorySpec{Enabled: memory.Enabled}
		converted.Status.Memory = &contract.MemoryStatus{
			Enabled: memory.Enabled, Ready: memory.Ready, Name: memory.Name,
			Location: memory.Location, Content: memory.Content,
		}
	}
	return converted, nil
}
