package agent

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/identity"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/openclawsandbox"
	"csgclaw/internal/runtime/picoclawsandbox"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/utils"
)

func normalizeUpdateFieldMask(fieldMask []string) map[string]struct{} {
	if len(fieldMask) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(fieldMask))
	for _, field := range fieldMask {
		normalized := strings.ToLower(strings.TrimSpace(field))
		if normalized == "" {
			continue
		}
		out[normalized] = struct{}{}
	}
	return out
}

func updateIncludesMCPServers(req UpdateRequest) bool {
	fieldMask := normalizeUpdateFieldMask(req.FieldMask)
	if len(fieldMask) == 0 {
		return req.MCPServersSet
	}
	_, ok := fieldMask["mcpservers"]
	return ok
}

func updateIncludesRuntimeOptions(req UpdateRequest) bool {
	fieldMask := normalizeUpdateFieldMask(req.FieldMask)
	if len(fieldMask) == 0 {
		return req.RuntimeOptions != nil
	}
	_, ok := fieldMask["runtime_options"]
	return ok
}

func (s *Service) AgentProfileView(id string) (AgentProfileView, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return AgentProfileView{}, fmt.Errorf("agent id is required")
	}
	got, ok := s.Agent(id)
	if !ok {
		return AgentProfileView{}, fmt.Errorf("agent %q not found", id)
	}
	return profileViewWithAgentRuntimeOptions(got.AgentProfile, got.RuntimeOptions, got.RuntimeKind, got.DetectionResults), nil
}

func (s *Service) ProfileDefaultsView() AgentProfileView {
	if s == nil {
		return AgentProfileView{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return profileView(s.profileDefaults, s.detectionResults)
}

func (s *Service) UpdateAgentProfile(id string, profile AgentProfile) (AgentProfileView, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return AgentProfileView{}, fmt.Errorf("agent id is required")
	}
	if s == nil {
		return AgentProfileView{}, fmt.Errorf("agent service is required")
	}

	s.mu.Lock()
	current, key, ok := s.agentByIDLocked(id)
	if !ok {
		s.mu.Unlock()
		return AgentProfileView{}, fmt.Errorf("agent %q not found", id)
	}
	if strings.TrimSpace(profile.APIKey) == "" {
		profile.APIKey = current.AgentProfile.APIKey
	}
	profile = s.inheritModelProviderReference(profile, current)
	previous := current
	normalized := normalizeProfileForAgentRuntime(profile, current.RuntimeOptions, current.Name, current.Description, current.RuntimeKind, nil)
	runtimePrevious := s.hydrateProfileFromCatalogLocked(previous.AgentProfile)
	runtimeNormalized := s.hydrateProfileFromCatalogLocked(normalized)
	runtimeKind := strings.TrimSpace(current.RuntimeKind)
	runtimeRunning := isRuntimeRunning(current)
	change := runtimeConfigChangeForAgent(runtimePrevious, runtimeNormalized, previous.RuntimeOptions, current.RuntimeOptions)
	s.mu.Unlock()

	if runtimeNormalized.ProfileComplete {
		if err := s.validateRuntimeConfig(context.Background(), runtimeKind, change.Current); err != nil {
			return AgentProfileView{}, err
		}
	}
	restartRequired, err := s.runtimeConfigRestartRequired(runtimeKind, change)
	if err != nil {
		return AgentProfileView{}, err
	}

	s.mu.Lock()
	current, key, ok = s.agentByIDLocked(id)
	if !ok {
		s.mu.Unlock()
		return AgentProfileView{}, fmt.Errorf("agent %q not found", id)
	}
	normalized.EnvRestartRequired = restartRequired
	current.AgentProfile = normalized
	current.ProfileComplete = normalized.ProfileComplete
	current.Profile = profileSelector(normalized)
	current.DetectionResults = nil
	if current.ProfileComplete && strings.EqualFold(strings.TrimSpace(current.Status), "profile_incomplete") {
		current.Status = string(sandbox.StateStopped)
	}
	s.agents[key] = current
	if normalized.ProfileComplete {
		s.profileDefaults = cloneProfile(normalized)
		s.detectionResults = nil
	}
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		return AgentProfileView{}, err
	}
	s.mu.Unlock()
	if restartRequired && runtimeRunning && !isGatewayRuntimeKind(runtimeKind) {
		s.stopLifecycleAgent(id)
	}
	if err := s.syncGatewayAfterProfileChange(context.Background(), id, previous, normalized, restartRequired); err != nil {
		return AgentProfileView{}, err
	}
	got, ok := s.Agent(id)
	if !ok {
		return AgentProfileView{}, fmt.Errorf("agent %q not found", id)
	}
	return profileViewWithAgentRuntimeOptions(got.AgentProfile, got.RuntimeOptions, got.RuntimeKind, got.DetectionResults), nil
}

func profileRestartRequired(current Agent, next AgentProfile) bool {
	return !profilesEqualEnv(current.AgentProfile, next)
}

func gatewayProfileRuntimeRestartRequired(current Agent, next AgentProfile) bool {
	if !isGatewayRuntimeKind(strings.TrimSpace(current.RuntimeKind)) {
		return false
	}
	previous := normalizeProfileForAgentRuntime(current.AgentProfile, current.RuntimeOptions, current.Name, current.Description, current.RuntimeKind, nil)
	return !profileRuntimeInputsEqual(previous, next)
}

func profileRuntimeInputsEqual(a, b AgentProfile) bool {
	return strings.TrimSpace(a.Provider) == strings.TrimSpace(b.Provider) &&
		strings.TrimSpace(a.ModelProviderID) == strings.TrimSpace(b.ModelProviderID) &&
		strings.TrimRight(strings.TrimSpace(a.BaseURL), "/") == strings.TrimRight(strings.TrimSpace(b.BaseURL), "/") &&
		strings.TrimSpace(a.APIKey) == strings.TrimSpace(b.APIKey) &&
		strings.TrimSpace(a.ModelID) == strings.TrimSpace(b.ModelID) &&
		strings.TrimSpace(a.ReasoningEffort) == strings.TrimSpace(b.ReasoningEffort) &&
		reflect.DeepEqual(a.Headers, b.Headers) &&
		reflect.DeepEqual(a.RequestOptions, b.RequestOptions)
}

func (s *Service) syncGatewayAfterProfileChange(ctx context.Context, id string, previous Agent, normalized AgentProfile, restartRequired bool) error {
	if s == nil || !normalized.ProfileComplete {
		return nil
	}
	runtimeNormalized := s.hydrateProfileFromCatalog(normalized)
	got, ok := s.Agent(id)
	if !ok || !isGatewayRuntimeKind(strings.TrimSpace(got.RuntimeKind)) {
		return nil
	}
	profileJustCompleted := !isAgentProfileComplete(previous) && normalized.ProfileComplete
	boxMissing := strings.TrimSpace(got.BoxID) == ""
	if isManagerAgent(got) && (profileJustCompleted || boxMissing) {
		_, err := s.EnsureManager(ctx, false)
		return err
	}
	if restartRequired {
		_, err := s.Recreate(ctx, id)
		return err
	}
	if gatewayProfileRuntimeRestartRequired(previous, normalized) {
		return s.syncGatewayHostConfig(got, runtimeNormalized)
	}
	return nil
}

func (s *Service) syncGatewayHostConfig(got Agent, profile AgentProfile) error {
	if s == nil {
		return nil
	}
	modelCfg := modelConfigFromProfile(profile)
	participantID := participantIDForAgent(got.Name, got.ID)
	switch strings.TrimSpace(got.RuntimeKind) {
	case RuntimeKindPicoClawSandbox:
		feishuProvider := s.currentFeishuProviderForRuntime(RuntimeKindPicoClawSandbox)
		agentHome, err := s.agentHomeDir(got.ID)
		if err != nil {
			return err
		}
		if _, err := picoclawsandbox.EnsureConfigWithMCPServers(agentHome, participantID, got.ID, s.server, modelCfg, got.MCPServers, resolveManagerBaseURL, feishuProvider); err != nil {
			return fmt.Errorf("sync gateway picoclaw config: %w", err)
		}
	case RuntimeKindOpenClawSandbox:
		agentHome, err := s.agentHomeDir(got.ID)
		if err != nil {
			return err
		}
		feishuProvider := s.currentFeishuProviderForRuntime(RuntimeKindOpenClawSandbox)
		if _, err := openclawsandbox.EnsureConfigWithMCPServers(agentHome, participantID, got.ID, s.server, modelCfg, got.MCPServers, resolveManagerBaseURL, feishuProvider); err != nil {
			return fmt.Errorf("sync gateway openclaw config: %w", err)
		}
	default:
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.agents[got.ID]
	if !ok {
		return nil
	}
	current.AgentProfile.EnvRestartRequired = false
	s.agents[got.ID] = current
	return s.saveLocked()
}

func (s *Service) currentFeishuProviderForRuntime(runtimeKind string) feishu.AgentCredentialProvider {
	if s == nil {
		return nil
	}
	rt, err := s.runtimeForKind(runtimeKind)
	if err != nil {
		return nil
	}
	current, ok := rt.(interface {
		CurrentFeishuProvider() feishu.AgentCredentialProvider
	})
	if !ok {
		return nil
	}
	return current.CurrentFeishuProvider()
}

func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (Agent, error) {
	if s == nil {
		return Agent{}, fmt.Errorf("agent service is required")
	}
	if updateIncludesMCPServers(req) {
		s.mcpServersMu.Lock()
		defer s.mcpServersMu.Unlock()
	}
	return s.update(ctx, id, req)
}

func (s *Service) update(ctx context.Context, id string, req UpdateRequest) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, fmt.Errorf("agent id is required")
	}
	if s == nil {
		return Agent{}, fmt.Errorf("agent service is required")
	}

	s.mu.Lock()
	current, key, ok := s.agentByIDLocked(id)
	if !ok {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	if isManagerAgent(current) {
		if err := validateManagerUpdateRuntimeConfig(req); err != nil {
			s.mu.Unlock()
			return Agent{}, err
		}
	}
	previous := current
	runtimeKind := strings.TrimSpace(current.RuntimeKind)
	runtimeRunning := isRuntimeRunning(current)
	restartRequired := false
	runtimeAffectingUpdate := false
	fieldMask := normalizeUpdateFieldMask(req.FieldMask)
	hasFieldMask := len(fieldMask) > 0
	updateRequested := func(field string, legacy bool) bool {
		if !hasFieldMask {
			return legacy
		}
		_, ok := fieldMask[field]
		return ok
	}
	instructionsUpdated := updateRequested("instructions", req.Instructions != nil)
	runtimeOptionsUpdated := updateRequested("runtime_options", req.RuntimeOptions != nil)
	mcpServersUpdated := updateRequested("mcpservers", req.MCPServersSet)
	if mcpServersUpdated && !req.MCPServersSet {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("field_mask includes mcpServers but request is missing mcpServers")
	}
	if updateRequested("name", req.Name != nil) {
		if req.Name == nil {
			s.mu.Unlock()
			return Agent{}, fmt.Errorf("field_mask includes name but request is missing name")
		}
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			s.mu.Unlock()
			return Agent{}, fmt.Errorf("name is required")
		}
		if err := identity.ValidateMentionName(name); err != nil {
			s.mu.Unlock()
			return Agent{}, err
		}
		if strings.EqualFold(name, ManagerName) && !isManagerAgent(current) {
			s.mu.Unlock()
			return Agent{}, fmt.Errorf("name %q is reserved", name)
		}
		for otherID, other := range s.agents {
			if otherID != id && strings.EqualFold(strings.TrimSpace(other.Name), name) {
				s.mu.Unlock()
				return Agent{}, fmt.Errorf("agent name %q already exists", name)
			}
		}
		current.Name = name
	}
	if updateRequested("description", req.Description != nil) {
		if req.Description == nil {
			s.mu.Unlock()
			return Agent{}, fmt.Errorf("field_mask includes description but request is missing description")
		}
		current.Description = strings.TrimSpace(*req.Description)
	}
	if updateRequested("instructions", req.Instructions != nil) {
		if req.Instructions == nil {
			s.mu.Unlock()
			return Agent{}, fmt.Errorf("field_mask includes instructions but request is missing instructions")
		}
		current.Instructions = strings.TrimSpace(*req.Instructions)
	}
	if updateRequested("image", req.Image != nil) {
		if req.Image == nil {
			s.mu.Unlock()
			return Agent{}, fmt.Errorf("field_mask includes image but request is missing image")
		}
		current.Image = strings.TrimSpace(*req.Image)
	}
	if updateRequested("avatar", req.Avatar != nil) {
		if req.Avatar == nil {
			s.mu.Unlock()
			return Agent{}, fmt.Errorf("field_mask includes avatar but request is missing avatar")
		}
		current.Avatar = strings.TrimSpace(*req.Avatar)
	}
	if updateRequested("profile", req.Profile != nil) {
		if req.Profile == nil {
			s.mu.Unlock()
			return Agent{}, fmt.Errorf("field_mask includes profile but request is missing profile")
		}
		current.Profile = strings.TrimSpace(*req.Profile)
	}
	agentProfileUpdated := updateRequested("agent_profile", req.AgentProfile != nil)
	if !hasFieldMask && !agentProfileUpdated && strings.TrimSpace(current.Profile) != "" {
		if selected, ok := CatalogProviderModelConfig(s.llm, current.Profile); ok {
			selected.Name = current.AgentProfile.Name
			selected.Description = current.AgentProfile.Description
			selected.ReasoningEffort = current.AgentProfile.ReasoningEffort
			selected.EnableFastMode = current.AgentProfile.EnableFastMode
			selected.RequestOptions = current.AgentProfile.RequestOptions
			selected.Env = current.AgentProfile.Env
			req.AgentProfile = &selected
			agentProfileUpdated = true
		}
	}
	if agentProfileUpdated || runtimeOptionsUpdated || mcpServersUpdated {
		runtimeAffectingUpdate = true
		profile := current.AgentProfile
		if agentProfileUpdated {
			if req.AgentProfile == nil {
				s.mu.Unlock()
				return Agent{}, fmt.Errorf("field_mask includes agent_profile but request is missing agent_profile")
			}
			profile = *req.AgentProfile
			if strings.TrimSpace(profile.APIKey) == "" {
				profile.APIKey = current.AgentProfile.APIKey
			}
			profile = s.inheritModelProviderReference(profile, current)
		}
		var patch map[string]any
		if runtimeOptionsUpdated {
			if req.RuntimeOptions == nil {
				empty := map[string]any{}
				req.RuntimeOptions = &empty
			}
			patch = *req.RuntimeOptions
			if err := validateRuntimeOptionsWithoutMCP(patch); err != nil {
				s.mu.Unlock()
				return Agent{}, err
			}
		}
		mergedFlat := runtimeOptionsAfterPatch(current.RuntimeKind, current.RuntimeOptions, nil)
		if runtimeOptionsUpdated {
			mergedFlat = utils.CloneAnyMap(patch)
			current.RuntimeOptions = utils.CloneAnyMap(mergedFlat)
		} else {
			current.RuntimeOptions = nextAgentRuntimeOptions(current.RuntimeKind, current.RuntimeOptions, mergedFlat)
		}
		if mcpServersUpdated {
			if req.MCPServers == nil {
				current.MCPServers = nil
			} else {
				normalizedMCPServers, err := agentruntime.NormalizeMCPServers(*req.MCPServers)
				if err != nil {
					s.mu.Unlock()
					return Agent{}, err
				}
				current.MCPServers = normalizedMCPServers
			}
		}
		normalized := normalizeProfileForAgentRuntime(profile, current.RuntimeOptions, current.Name, current.Description, current.RuntimeKind, mergedFlat)
		runtimePrevious := s.hydrateProfileFromCatalogLocked(previous.AgentProfile)
		runtimeNormalized := s.hydrateProfileFromCatalogLocked(normalized)
		change := runtimeConfigChangeForAgent(runtimePrevious, runtimeNormalized, previous.RuntimeOptions, current.RuntimeOptions)
		mcpChange := mcpServersChangeForAgent(previous.MCPServers, current.MCPServers)
		runtimeConfigUpdated := agentProfileUpdated || runtimeOptionsUpdated
		restartRequired = profileRestartRequired(previous, normalized)
		if runtimeConfigUpdated {
			controllerRestartRequired, err := s.runtimeConfigRestartRequired(runtimeKind, change)
			if err != nil {
				s.mu.Unlock()
				return Agent{}, err
			}
			restartRequired = restartRequired || controllerRestartRequired
		}
		if mcpServersUpdated {
			controllerMCPRestartRequired, err := s.mcpServersRestartRequired(runtimeKind, mcpChange)
			if err != nil {
				s.mu.Unlock()
				return Agent{}, err
			}
			restartRequired = restartRequired || controllerMCPRestartRequired
		}
		normalized.EnvRestartRequired = restartRequired
		current.AgentProfile = normalized
		current.ProfileComplete = normalized.ProfileComplete
		current.Profile = profileSelector(normalized)
		current.DetectionResults = nil
		if current.ProfileComplete && strings.EqualFold(strings.TrimSpace(current.Status), "profile_incomplete") {
			current.Status = string(sandbox.StateStopped)
		}
		if (runtimeConfigUpdated && runtimeNormalized.ProfileComplete) || mcpServersUpdated {
			s.mu.Unlock()
			if runtimeConfigUpdated && runtimeNormalized.ProfileComplete {
				if err := s.validateRuntimeConfig(ctx, runtimeKind, change.Current); err != nil {
					return Agent{}, err
				}
			}
			if mcpServersUpdated {
				if err := s.validateMCPServers(ctx, runtimeKind, mcpChange.Current); err != nil {
					return Agent{}, err
				}
			}
			s.mu.Lock()
			if _, key, ok = s.agentByIDLocked(id); !ok {
				s.mu.Unlock()
				return Agent{}, fmt.Errorf("agent %q not found", id)
			}
		}
	}
	if runtimeAffectingUpdate && current.ProfileComplete {
		s.profileDefaults = cloneProfile(current.AgentProfile)
		s.detectionResults = nil
	}
	current.UpdatedAt = time.Now().UTC()
	s.agents[key] = current
	s.syncRuntimeRecordLocked(current)
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		return Agent{}, err
	}
	s.mu.Unlock()
	if instructionsUpdated || runtimeOptionsUpdated {
		if err := s.reconcileRuntimeConfig(ctx, previous, current); err != nil {
			return Agent{}, err
		}
	}
	if mcpServersUpdated {
		// OpenClaw consumes MCP settings during provisioning/recreation. Writing
		// its live config here can race the gateway process, including for an
		// idempotent save that does not require a restart.
		skipMCPReconcileForOpenClaw := strings.EqualFold(strings.TrimSpace(runtimeKind), RuntimeKindOpenClawSandbox)
		if !skipMCPReconcileForOpenClaw {
			if err := s.reconcileMCPServers(ctx, previous, current); err != nil {
				return Agent{}, err
			}
		}
	}
	if restartRequired && runtimeRunning && !isGatewayRuntimeKind(runtimeKind) {
		s.stopLifecycleAgent(id)
	}

	updated, ok := s.Agent(id)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	if runtimeAffectingUpdate {
		normalized := normalizeProfileForAgentRuntime(updated.AgentProfile, updated.RuntimeOptions, updated.Name, updated.Description, updated.RuntimeKind, nil)
		if err := s.syncGatewayAfterProfileChange(ctx, id, previous, normalized, restartRequired); err != nil {
			return Agent{}, err
		}
		updated, ok = s.Agent(id)
		if !ok {
			return Agent{}, fmt.Errorf("agent %q not found", id)
		}
	}
	return updated, nil
}

func (s *Service) AddMCPServersFromHub(ctx context.Context, id string, names []string, catalogServers map[string]any) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, fmt.Errorf("agent id is required")
	}
	if s == nil {
		return Agent{}, fmt.Errorf("agent service is required")
	}
	names = normalizeMCPServerNames(names)
	if len(names) == 0 {
		return Agent{}, fmt.Errorf("mcp server names are required")
	}
	serverConfigs := make(map[string]any, len(names))
	for _, name := range names {
		rawServer, ok := catalogServers[name]
		if !ok {
			return Agent{}, fmt.Errorf("mcp server %q not found", name)
		}
		serverConfig, err := runtimeMCPServerConfigFromCatalog(name, rawServer)
		if err != nil {
			return Agent{}, err
		}
		serverConfigs[name] = serverConfig
	}

	s.mcpServersMu.Lock()
	defer s.mcpServersMu.Unlock()

	s.mu.RLock()
	current, _, ok := s.agentByIDLocked(id)
	s.mu.RUnlock()
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	currentServers, err := s.currentMCPServersForManagement(ctx, current)
	if err != nil {
		return Agent{}, err
	}
	mergedServers := make(map[string]any, len(currentServers)+len(serverConfigs))
	for name, serverConfig := range currentServers {
		mergedServers[name] = serverConfig
	}
	for _, name := range names {
		mergedServers[name] = serverConfigs[name]
	}
	return s.update(ctx, id, UpdateRequest{
		MCPServers:    &mergedServers,
		MCPServersSet: true,
		FieldMask:     []string{"mcpServers"},
	})
}

func (s *Service) DeleteMCPServers(ctx context.Context, id string, names []string) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, fmt.Errorf("agent id is required")
	}
	if s == nil {
		return Agent{}, fmt.Errorf("agent service is required")
	}
	names = normalizeMCPServerNames(names)
	if len(names) == 0 {
		return Agent{}, fmt.Errorf("mcp server names are required")
	}

	s.mcpServersMu.Lock()
	defer s.mcpServersMu.Unlock()

	s.mu.RLock()
	current, _, ok := s.agentByIDLocked(id)
	s.mu.RUnlock()
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	servers, err := s.currentMCPServersForManagement(ctx, current)
	if err != nil {
		return Agent{}, err
	}
	for _, name := range names {
		if _, ok := servers[name]; !ok {
			return Agent{}, fmt.Errorf("mcp server %q not found", name)
		}
	}
	for _, name := range names {
		delete(servers, name)
	}
	return s.update(ctx, id, UpdateRequest{
		MCPServers:    &servers,
		MCPServersSet: true,
		FieldMask:     []string{"mcpServers"},
	})
}

// currentMCPServersForManagement uses persisted MCPServers once an agent has
// entered CSGClaw management. For an unmanaged agent, it reads the runtime
// configuration once so the first management action can adopt every server.
func (s *Service) currentMCPServersForManagement(ctx context.Context, current Agent) (map[string]any, error) {
	servers, err := agentruntime.NormalizeMCPServers(current.MCPServers)
	if err != nil || servers != nil {
		return servers, err
	}

	runtimeKind := strings.TrimSpace(current.RuntimeKind)
	if runtimeKind == "" {
		return nil, nil
	}
	rt, err := s.runtimeForKind(runtimeKind)
	if err != nil {
		return nil, err
	}
	lister, ok := rt.(agentruntime.MCPServersListController)
	if !ok {
		return nil, nil
	}

	listed, err := lister.ListMCPServers(ctx, runtimeHandleForAgent(current), agentruntime.MCPServersSnapshot{})
	if err != nil {
		return nil, fmt.Errorf("read runtime mcpServers for agent %q: %w", current.ID, err)
	}
	servers, err = agentruntime.NormalizeMCPServers(listed.Servers)
	if err != nil {
		return nil, fmt.Errorf("normalize runtime mcpServers for agent %q: %w", current.ID, err)
	}
	return servers, nil
}

func normalizeMCPServerNames(values []string) []string {
	names := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

func runtimeMCPServerConfigFromCatalog(name string, raw any) (map[string]any, error) {
	rawConfig, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mcp server %q config must be an object", name)
	}
	normalized, err := agentruntime.NormalizeMCPServers(map[string]any{name: rawConfig})
	if err != nil {
		return nil, err
	}
	serverConfig, ok := normalized[name].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mcp server %q config must be an object", name)
	}
	delete(serverConfig, "description")
	return serverConfig, nil
}

func validateManagerUpdateRuntimeConfig(req UpdateRequest) error {
	if updateIncludesRuntimeOptions(req) {
		return fmt.Errorf("manager runtime options are managed automatically")
	}
	if !req.RuntimeSelectionRequested {
		return nil
	}
	sandboxEnabled := false
	if req.SandboxEnabled != nil {
		sandboxEnabled = *req.SandboxEnabled
	}
	cfg, err := agentruntime.RuntimeConfigFromSelection(req.RuntimeKind, req.RuntimeName, sandboxEnabled)
	if err != nil {
		return err
	}
	if cfg.LegacyKind() == RuntimeKindCodex && cfg.Name == RuntimeNameCodex && !cfg.Sandboxed {
		return nil
	}
	return fmt.Errorf("manager runtime is fixed to codex")
}

func (s *Service) ListModelsForRequest(ctx context.Context, req ProfileModelRequest) ([]string, error) {
	profile := AgentProfile{
		Name:     "preview",
		Provider: req.Provider,
		BaseURL:  req.BaseURL,
		APIKey:   req.APIKey,
		Headers:  req.Headers,
	}
	profile = normalizeProfile(profile, profile.Name, profile.Description)
	if strings.TrimSpace(profile.APIKey) == "" {
		profile.APIKey = s.storedAPIKeyForModelRequest(req, profile)
	}
	if strings.TrimSpace(profile.APIKey) == "" {
		profile = s.withDefaultAPIKeyForMatchingProfile(profile)
	}
	if profile.Provider == ProviderCodex || profile.Provider == ProviderClaudeCode {
		models, err := listCLIProxyModelChoices(ctx, profile.Provider)
		if err != nil {
			return nil, err
		}
		return sortModelIDs(models), nil
	}
	return ListModelsForProfile(ctx, profile)
}

func runtimeConfigChangeForAgent(previousProfile, currentProfile AgentProfile, previousOptions, currentOptions map[string]any) agentruntime.RuntimeConfigChange {
	return agentruntime.RuntimeConfigChange{
		Previous: runtimeConfigSnapshotForAgent(previousProfile, previousOptions),
		Current:  runtimeConfigSnapshotForAgent(currentProfile, currentOptions),
	}
}

func runtimeConfigSnapshotForAgent(profile AgentProfile, options map[string]any) agentruntime.RuntimeConfigSnapshot {
	profile = normalizeProfile(profile, profile.Name, profile.Description)
	return agentruntime.RuntimeConfigSnapshot{
		Profile: agentruntime.RuntimeProfileConfig{
			Provider:        strings.TrimSpace(profile.Provider),
			BaseURL:         profileBaseURL(profile),
			APIKey:          profileAPIKey(profile),
			ModelID:         strings.TrimSpace(profile.ModelID),
			ReasoningEffort: strings.TrimSpace(profile.ReasoningEffort),
			Headers:         normalizeStringMap(profile.Headers),
			RequestOptions:  utils.CloneAnyMap(profile.RequestOptions),
		},
		Options: utils.CloneAnyMap(options),
	}
}

func mcpServersChangeForAgent(previousConfig, currentConfig map[string]any) agentruntime.MCPServersChange {
	return agentruntime.MCPServersChange{
		Previous: mcpServersSnapshotForAgent(previousConfig),
		Current:  mcpServersSnapshotForAgent(currentConfig),
	}
}

func mcpServersSnapshotForAgent(servers map[string]any) agentruntime.MCPServersSnapshot {
	return agentruntime.MCPServersSnapshot{Servers: cloneMCPServers(servers)}
}

func (s *Service) hydrateProfileFromCatalog(profile AgentProfile) AgentProfile {
	if s == nil {
		return profile
	}
	s.mu.RLock()
	out := s.hydrateProfileFromCatalogLocked(profile)
	s.mu.RUnlock()
	return out
}

func (s *Service) hydrateProfileFromCatalogLocked(profile AgentProfile) AgentProfile {
	if s == nil {
		return profile
	}
	out := cloneProfile(profile)
	providerID := NormalizeModelProviderID(out.ModelProviderID)
	if providerID == "" {
		return out
	}
	out.ModelProviderID = providerID
	out.Provider = ProfileProviderForModelProviderID(providerID)
	if provider, ok := ModelProviderConfigForProfile(s.llm, out); ok {
		out.BaseURL = provider.BaseURL
		out.APIKey = provider.APIKey
		out.Headers = cloneStringMap(provider.Headers)
		if strings.TrimSpace(out.ReasoningEffort) == "" && strings.TrimSpace(provider.ReasoningEffort) != "" {
			out.ReasoningEffort = provider.ReasoningEffort
		}
	}
	out.ProfileComplete = profileIsComplete(out)
	return out
}

func (s *Service) inheritModelProviderReference(profile AgentProfile, current Agent) AgentProfile {
	if s == nil {
		return profile
	}
	if strings.TrimSpace(profile.ModelProviderID) != "" || strings.TrimSpace(profile.ModelID) == "" {
		return profile
	}
	if strings.TrimSpace(profile.BaseURL) != "" || strings.TrimSpace(profile.APIKey) != "" || len(profile.Headers) > 0 {
		return profile
	}
	if id := NormalizeModelProviderID(current.AgentProfile.ModelProviderID); id != "" {
		profile.ModelProviderID = id
		return profile
	}
	if selector := strings.TrimSpace(current.Profile); selector != "" {
		if providerID, _, ok := splitModelProviderSelector(selector); ok {
			profile.ModelProviderID = providerID
			return profile
		}
	}
	if referenced, ok := CatalogReferenceProfile(s.llm, current.AgentProfile); ok {
		if id := NormalizeModelProviderID(referenced.ModelProviderID); id != "" {
			profile.ModelProviderID = id
		}
	}
	return profile
}

func (s *Service) validateRuntimeConfig(ctx context.Context, runtimeKind string, current agentruntime.RuntimeConfigSnapshot) error {
	if s == nil {
		return fmt.Errorf("agent service is required")
	}
	if err := validateRuntimeProfileAvailability(current); err != nil {
		return err
	}
	runtimeKind = strings.TrimSpace(runtimeKind)
	if runtimeKind == "" {
		return nil
	}
	rt, err := s.runtimeForKind(runtimeKind)
	if err != nil {
		return err
	}
	controller, ok := rt.(agentruntime.RuntimeConfigController)
	if !ok {
		return nil
	}
	return controller.ValidateConfig(ctx, current)
}

func validateRuntimeProfileAvailability(current agentruntime.RuntimeConfigSnapshot) error {
	if normalizeProfileProvider(current.Profile.Provider) != ProviderCodex {
		return nil
	}
	if _, err := locateCodexCLI(); err != nil {
		return fmt.Errorf("codex model provider requires Codex CLI: %w", err)
	}
	return nil
}

func (s *Service) runtimeConfigRestartRequired(runtimeKind string, change agentruntime.RuntimeConfigChange) (bool, error) {
	if s == nil {
		return false, fmt.Errorf("agent service is required")
	}
	runtimeKind = strings.TrimSpace(runtimeKind)
	if runtimeKind == "" {
		return false, nil
	}
	rt, err := s.runtimeForKind(runtimeKind)
	if err != nil {
		return false, err
	}
	controller, ok := rt.(agentruntime.RuntimeConfigController)
	if !ok {
		return false, nil
	}
	return controller.RestartRequired(change)
}

func (s *Service) validateMCPServers(ctx context.Context, runtimeKind string, current agentruntime.MCPServersSnapshot) error {
	if s == nil {
		return fmt.Errorf("agent service is required")
	}
	if current.Servers == nil {
		return nil
	}
	runtimeKind = strings.TrimSpace(runtimeKind)
	if runtimeKind == "" {
		return nil
	}
	rt, err := s.runtimeForKind(runtimeKind)
	if err != nil {
		return err
	}
	controller, ok := rt.(agentruntime.MCPServersController)
	if !ok {
		return fmt.Errorf("mcpServers is not supported for runtime_kind %q", runtimeKind)
	}
	return controller.ValidateMCPServers(ctx, current)
}

func (s *Service) mcpServersRestartRequired(runtimeKind string, change agentruntime.MCPServersChange) (bool, error) {
	if s == nil {
		return false, fmt.Errorf("agent service is required")
	}
	if change.Previous.Servers == nil && change.Current.Servers == nil {
		return false, nil
	}
	runtimeKind = strings.TrimSpace(runtimeKind)
	if runtimeKind == "" {
		return false, nil
	}
	rt, err := s.runtimeForKind(runtimeKind)
	if err != nil {
		return false, err
	}
	controller, ok := rt.(agentruntime.MCPServersController)
	if !ok {
		return false, fmt.Errorf("mcpServers is not supported for runtime_kind %q", runtimeKind)
	}
	return controller.MCPServersRestartRequired(change)
}

func (s *Service) reconcileRuntimeConfig(ctx context.Context, previous, current Agent) error {
	if s == nil {
		return fmt.Errorf("agent service is required")
	}
	runtimeKind := strings.TrimSpace(current.RuntimeKind)
	if runtimeKind == "" {
		return nil
	}
	rt, err := s.runtimeForKind(runtimeKind)
	if err != nil {
		return err
	}
	controller, ok := rt.(agentruntime.RuntimeConfigController)
	if !ok {
		return nil
	}
	previous.AgentProfile = s.hydrateProfileFromCatalog(previous.AgentProfile)
	current.AgentProfile = s.hydrateProfileFromCatalog(current.AgentProfile)
	return controller.ReconcileConfig(ctx, runtimeHandleForAgent(current), runtimeConfigChangeForAgent(previous.AgentProfile, current.AgentProfile, previous.RuntimeOptions, current.RuntimeOptions))
}

func (s *Service) reconcileMCPServers(ctx context.Context, previous, current Agent) error {
	if s == nil {
		return fmt.Errorf("agent service is required")
	}
	runtimeKind := strings.TrimSpace(current.RuntimeKind)
	if runtimeKind == "" {
		return nil
	}
	rt, err := s.runtimeForKind(runtimeKind)
	if err != nil {
		return err
	}
	controller, ok := rt.(agentruntime.MCPServersReconciler)
	if !ok {
		return fmt.Errorf("mcpServers live reconciliation is not supported for runtime_kind %q", runtimeKind)
	}
	return controller.ReconcileMCPServers(ctx, runtimeHandleForAgent(current), mcpServersChangeForAgent(previous.MCPServers, current.MCPServers))
}

func normalizeMCPServers(config map[string]any) (map[string]any, error) {
	if config == nil {
		return nil, nil
	}
	return agentruntime.NormalizeMCPServers(config)
}

func (s *Service) storedAPIKeyForModelRequest(req ProfileModelRequest, profile AgentProfile) string {
	agentID := strings.TrimSpace(req.AgentID)
	if s == nil || agentID == "" || profile.Provider != ProviderAPI {
		return ""
	}
	got, ok := s.Agent(agentID)
	if !ok {
		return ""
	}
	stored := normalizeProfile(got.AgentProfile, got.Name, got.Description)
	if stored.Provider != ProviderAPI || strings.TrimSpace(stored.APIKey) == "" {
		return ""
	}
	if profile.BaseURL != stored.BaseURL {
		return ""
	}
	return stored.APIKey
}

func (s *Service) withDefaultAPIKeyForMatchingProfile(profile AgentProfile) AgentProfile {
	if s == nil || strings.TrimSpace(profile.APIKey) != "" || normalizeProfileProvider(profile.Provider) != ProviderAPI {
		return profile
	}
	s.mu.RLock()
	defaultProfile := cloneProfile(s.profileDefaults)
	s.mu.RUnlock()
	defaultProfile = normalizeProfile(defaultProfile, defaultProfile.Name, defaultProfile.Description)
	if defaultProfile.Provider != ProviderAPI || strings.TrimSpace(defaultProfile.APIKey) == "" {
		return profile
	}
	baseURL := strings.TrimRight(strings.TrimSpace(profile.BaseURL), "/")
	if baseURL == "" || baseURL != defaultProfile.BaseURL {
		return profile
	}
	profile.APIKey = defaultProfile.APIKey
	return profile
}

func (s *Service) ResolvedAgentProfile(agentID string) (AgentProfile, error) {
	got, ok := s.Agent(agentID)
	if !ok {
		return AgentProfile{}, fmt.Errorf("agent %q not found", strings.TrimSpace(agentID))
	}
	profile := normalizeProfileForAgentRuntime(got.AgentProfile, got.RuntimeOptions, got.Name, got.Description, got.RuntimeKind, nil)
	profile = s.hydrateProfileFromCatalog(profile)
	if !profile.ProfileComplete {
		return AgentProfile{}, fmt.Errorf("agent %q profile is incomplete", strings.TrimSpace(agentID))
	}
	return profile, nil
}

func (s *Service) Recreate(ctx context.Context, id string) (Agent, error) {
	return s.recreate(ctx, id, func(ctx context.Context, got Agent) (string, error) {
		return s.imageForRecreate(ctx, got), nil
	})
}

func (s *Service) Upgrade(ctx context.Context, id string) (Agent, error) {
	return s.recreate(ctx, id, func(ctx context.Context, got Agent) (string, error) {
		latest, ok := s.imageForUpgrade(ctx, got)
		if !ok || strings.TrimSpace(latest) == "" {
			return "", fmt.Errorf("agent %q has no default image to upgrade", got.ID)
		}
		return latest, nil
	})
}

func (s *Service) recreate(ctx context.Context, id string, imageFor func(context.Context, Agent) (string, error)) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, fmt.Errorf("agent id is required")
	}
	got, ok := s.Agent(id)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	if isManagerAgent(got) {
		return s.EnsureManager(ctx, true)
	}
	got.ID = canonicalAgentID(got.ID)
	got.RuntimeID = normalizeRuntimeID(got.RuntimeID, got.ID)
	profile := normalizeProfileForAgentRuntime(got.AgentProfile, got.RuntimeOptions, got.Name, got.Description, got.RuntimeKind, nil)
	profile = s.hydrateProfileFromCatalog(profile)
	if !profile.ProfileComplete {
		return Agent{}, fmt.Errorf("agent %q profile is incomplete", id)
	}
	if err := s.validateRuntimeConfig(ctx, strings.TrimSpace(got.RuntimeKind), runtimeConfigSnapshotForAgent(profile, got.RuntimeOptions)); err != nil {
		return Agent{}, err
	}
	if err := s.validateMCPServers(ctx, strings.TrimSpace(got.RuntimeKind), mcpServersSnapshotForAgent(got.MCPServers)); err != nil {
		return Agent{}, err
	}

	runtimeImpl, err := s.runtimeForKind(strings.TrimSpace(got.RuntimeKind))
	if err != nil {
		return Agent{}, err
	}
	image, err := imageFor(ctx, got)
	if err != nil {
		return Agent{}, err
	}
	runtimeKind := strings.TrimSpace(got.RuntimeKind)
	if isGatewayRuntimeKind(runtimeKind) {
		if image == "" {
			image = s.managerImage
		}
	}

	s.stopLifecycleAgent(got.ID)

	if testCreateGatewayBoxHook != nil {
		rt, err := s.ensureRuntime(got.ID)
		if err != nil {
			return Agent{}, err
		}
		runtimeHome, err := s.sandboxRuntimeHome(got.ID)
		if err != nil {
			return Agent{}, err
		}
		defer func() {
			_ = s.closeRuntime(runtimeHome, rt)
		}()
		if strings.TrimSpace(got.BoxID) != "" {
			deleteErr := s.forceRemoveBox(ctx, rt, got.BoxID)
			if deleteErr != nil && !sandbox.IsNotFound(deleteErr) {
				return Agent{}, fmt.Errorf("remove existing agent box: %w", deleteErr)
			}
		}
		box, sandboxInfo, err := s.createGatewayBox(ctx, rt, image, got.Name, got.ID, profile)
		if err != nil {
			return Agent{}, fmt.Errorf("create agent box: %w", err)
		}
		defer func() {
			_ = s.closeBox(box)
		}()
		info := agentruntime.Info{
			HandleID:  strings.TrimSpace(sandboxInfo.ID),
			State:     agentruntime.State(sandboxInfo.State),
			CreatedAt: sandboxInfo.CreatedAt.UTC(),
		}
		recreated, err := s.persistRecreatedAgent(ctx, id, image, info)
		if err != nil {
			return Agent{}, err
		}
		return recreated, nil
	}

	deleteHandle := runtimeHandleForAgent(got)
	deleteErr := runtimeImpl.Delete(ctx, deleteHandle)
	if deleteErr != nil && !sandbox.IsNotFound(deleteErr) {
		return Agent{}, fmt.Errorf("remove existing agent box: %w", deleteErr)
	}

	runtimeProfile := s.runtimeProfileForKind(runtimeKind, got.ID, got.Name, got.Description, profile)
	createSpec := agentruntime.Spec{
		RuntimeID: normalizeRuntimeID(got.RuntimeID, got.ID),
		AgentID:   got.ID,
		AgentName: got.Name,
		Image:     image,
		Profile:   runtimeProfile,
	}
	if err := s.refreshGatewayTemplateSkills(got.ID, runtimeKind, recreateTemplateRole(got)); err != nil {
		return Agent{}, fmt.Errorf("refresh gateway template skills: %w", err)
	}
	if err := s.provisionRuntime(ctx, runtimeImpl, runtimeKind, agentruntime.ProvisionRequest{
		RuntimeID:      createSpec.RuntimeID,
		AgentID:        createSpec.AgentID,
		ParticipantID:  participantIDForAgent(createSpec.AgentName, createSpec.AgentID),
		AgentName:      createSpec.AgentName,
		Instructions:   strings.TrimSpace(got.Instructions),
		Profile:        runtimeProfile,
		RuntimeOptions: utils.CloneAnyMap(got.RuntimeOptions),
		MCPServers:     cloneMCPServers(got.MCPServers),
	}); err != nil {
		return Agent{}, fmt.Errorf("provision agent runtime: %w", err)
	}
	handle, err := runtimeImpl.New(ctx, createSpec)
	if err != nil {
		return Agent{}, fmt.Errorf("create agent box: %w", err)
	}

	info, err := s.runtimeInfo(ctx, runtimeImpl, handle)
	if err != nil {
		return Agent{}, fmt.Errorf("read agent runtime info: %w", err)
	}

	recreated, err := s.persistRecreatedAgent(ctx, id, image, info)
	if err != nil {
		return Agent{}, err
	}
	return recreated, nil
}

func (s *Service) persistRecreatedAgent(ctx context.Context, id, image string, info agentruntime.Info) (Agent, error) {
	s.mu.Lock()
	current, key, ok := s.agentByIDLocked(id)
	if !ok {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	current.ID = canonicalAgentID(current.ID)
	current.RuntimeID = normalizeRuntimeID(current.RuntimeID, current.ID)
	if image = strings.TrimSpace(image); image != "" {
		current.Image = image
	}
	current.BoxID = info.HandleID
	current.Status = string(info.State)
	current.UpdatedAt = time.Now().UTC()
	if !info.CreatedAt.IsZero() {
		current.CreatedAt = info.CreatedAt.UTC()
	} else if current.CreatedAt.IsZero() {
		current.CreatedAt = time.Now().UTC()
	}
	current.AgentProfile.EnvRestartRequired = false
	current.AgentProfile.ImageUpgradeRequired = false
	current.ProfileComplete = true
	delete(s.agents, key)
	s.agents[current.ID] = current
	s.syncRuntimeRecordLocked(current)
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return Agent{}, err
	}
	recreated, ok := s.Agent(current.ID)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	if err := s.syncLifecycleForAgent(ctx, recreated); err != nil {
		return Agent{}, err
	}
	return recreated, nil
}

func (s *Service) profileForCreateRequest(ctx context.Context, spec *CreateAgentSpec) (AgentProfile, error) {
	if spec == nil {
		return AgentProfile{}, fmt.Errorf("create spec is required")
	}

	profile := spec.AgentProfile
	if strings.TrimSpace(spec.Profile) != "" {
		if selected, ok := CatalogProviderModelConfig(s.llm, spec.Profile); ok {
			selected.Name = profile.Name
			selected.Description = profile.Description
			if strings.TrimSpace(selected.Name) == "" {
				selected.Name = spec.Name
			}
			if strings.TrimSpace(selected.Description) == "" {
				selected.Description = spec.Description
			}
			if strings.TrimSpace(profile.ReasoningEffort) != "" {
				selected.ReasoningEffort = profile.ReasoningEffort
			}
			selected.EnableFastMode = profile.EnableFastMode
			selected.RequestOptions = profile.RequestOptions
			selected.Env = profile.Env
			profile = selected
		}
	}
	rk := strings.TrimSpace(spec.RuntimeKind)
	if isGatewayRuntimeKind(rk) {
		if strings.TrimSpace(profile.Provider) == "" || strings.TrimSpace(profile.ModelID) == "" {
			s.mu.RLock()
			defaultProfile := cloneProfile(s.profileDefaults)
			s.mu.RUnlock()
			if strings.TrimSpace(profile.Provider) == "" {
				profile.Provider = defaultProfile.Provider
			}
			if strings.TrimSpace(profile.ModelID) == "" {
				profile.ModelID = defaultProfile.ModelID
			}
			if strings.TrimSpace(profile.BaseURL) == "" {
				profile.BaseURL = defaultProfile.BaseURL
			}
			if strings.TrimSpace(profile.APIKey) == "" {
				profile.APIKey = defaultProfile.APIKey
			}
			if len(profile.Headers) == 0 {
				profile.Headers = defaultProfile.Headers
			}
			if strings.TrimSpace(profile.ReasoningEffort) == "" {
				profile.ReasoningEffort = defaultProfile.ReasoningEffort
			}
			profile.EnableFastMode = profile.EnableFastMode || defaultProfile.EnableFastMode
			if len(profile.RequestOptions) == 0 {
				profile.RequestOptions = defaultProfile.RequestOptions
			}
			if len(profile.Env) == 0 {
				profile.Env = defaultProfile.Env
			}
		}
	}
	profile = s.withDefaultAPIKeyForMatchingProfile(profile)
	runtimeOptionsAfterPatch := runtimeOptionsAfterPatch(rk, nil, spec.RuntimeOptions)
	if err := validateRuntimeOptionsWithoutMCP(runtimeOptionsAfterPatch); err != nil {
		return AgentProfile{}, err
	}
	profile = normalizeProfileForAgentRuntime(profile, nil, spec.Name, spec.Description, spec.RuntimeKind, runtimeOptionsAfterPatch)
	runtimeProfile := s.hydrateProfileFromCatalog(profile)
	if !profile.ProfileComplete {
		detected, _ := s.DetectDefaultProfile(ctx)
		if detected.ProfileComplete {
			detected.Name = strings.TrimSpace(spec.Name)
			detected.Description = strings.TrimSpace(spec.Description)
			det := normalizeProfileForAgentRuntime(detected, nil, spec.Name, spec.Description, spec.RuntimeKind, nil)
			return det, nil
		}
		return AgentProfile{}, fmt.Errorf("agent profile is incomplete")
	}
	if !runtimeProfile.ProfileComplete {
		return AgentProfile{}, fmt.Errorf("agent profile is incomplete")
	}
	if len(runtimeOptionsAfterPatch) > 0 {
		spec.RuntimeOptions = utils.CloneAnyMap(runtimeOptionsAfterPatch)
	}
	return profile, nil
}

func runtimeOptionsAfterPatch(runtimeKind string, currentRuntimeOptions, patchRuntimeOptions map[string]any) map[string]any {
	if len(patchRuntimeOptions) == 0 {
		return utils.CloneAnyMap(currentRuntimeOptions)
	}
	if len(currentRuntimeOptions) == 0 {
		return utils.CloneAnyMap(patchRuntimeOptions)
	}
	return utils.OverlayAnyMap(utils.CloneAnyMap(currentRuntimeOptions), patchRuntimeOptions)
}

func validateRuntimeOptionsWithoutMCP(runtimeOptions map[string]any) error {
	for _, key := range []string{"mcp", "mcpServers"} {
		if _, exists := runtimeOptions[key]; exists {
			return fmt.Errorf("runtime_options.%s is not supported; use mcpServers", key)
		}
	}
	return nil
}

func nextAgentRuntimeOptions(runtimeKind string, currentRuntimeOptions, mergedRuntimeOptions map[string]any) map[string]any {
	if len(mergedRuntimeOptions) == 0 {
		return currentRuntimeOptions
	}
	return utils.CloneAnyMap(mergedRuntimeOptions)
}
