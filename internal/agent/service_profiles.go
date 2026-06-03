package agent

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/utils"
)

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
	current, ok := s.agents[id]
	if !ok {
		s.mu.Unlock()
		return AgentProfileView{}, fmt.Errorf("agent %q not found", id)
	}
	if strings.TrimSpace(profile.APIKey) == "" {
		profile.APIKey = current.AgentProfile.APIKey
	}
	normalized := normalizeProfileForAgentRuntime(profile, current.RuntimeOptions, current.Name, current.Description, current.RuntimeKind, nil)
	restartRequired := !profilesEqualEnv(current.AgentProfile, normalized) || codexProfileRuntimeRestartRequired(current, normalized)
	runtimeKind := strings.TrimSpace(current.RuntimeKind)
	runningCodex := runtimeKind == RuntimeKindCodex && isRuntimeRunning(current)
	s.mu.Unlock()

	if normalized.ProfileComplete {
		if err := s.ensureCodexResponsesAPI(context.Background(), runtimeKind, normalized); err != nil {
			return AgentProfileView{}, err
		}
	}

	s.mu.Lock()
	current, ok = s.agents[id]
	if !ok {
		s.mu.Unlock()
		return AgentProfileView{}, fmt.Errorf("agent %q not found", id)
	}
	normalized.EnvRestartRequired = restartRequired
	current.AgentProfile = normalized
	current.ProfileComplete = normalized.ProfileComplete
	current.Profile = profileSelector(normalized)
	current.DetectionResults = nil
	s.agents[id] = current
	if normalized.ProfileComplete {
		s.profileDefaults = cloneProfile(normalized)
		s.detectionResults = nil
	}
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		return AgentProfileView{}, err
	}
	s.mu.Unlock()
	if restartRequired && runningCodex {
		s.stopLifecycleAgent(id)
	}
	return profileViewWithAgentRuntimeOptions(normalized, current.RuntimeOptions, current.RuntimeKind, current.DetectionResults), nil
}

func codexProfileRuntimeRestartRequired(current Agent, next AgentProfile) bool {
	if strings.TrimSpace(current.RuntimeKind) != RuntimeKindCodex {
		return false
	}
	if !isRuntimeRunning(current) {
		return false
	}
	previous := normalizeProfileForAgentRuntime(current.AgentProfile, current.RuntimeOptions, current.Name, current.Description, current.RuntimeKind, nil)
	return !codexProfileRuntimeInputsEqual(previous, next)
}

func codexProfileRuntimeInputsEqual(a, b AgentProfile) bool {
	return strings.TrimSpace(a.Provider) == strings.TrimSpace(b.Provider) &&
		strings.TrimRight(strings.TrimSpace(a.BaseURL), "/") == strings.TrimRight(strings.TrimSpace(b.BaseURL), "/") &&
		strings.TrimSpace(a.APIKey) == strings.TrimSpace(b.APIKey) &&
		strings.TrimSpace(a.ModelID) == strings.TrimSpace(b.ModelID) &&
		strings.TrimSpace(a.ReasoningEffort) == strings.TrimSpace(b.ReasoningEffort) &&
		reflect.DeepEqual(a.Headers, b.Headers) &&
		reflect.DeepEqual(a.RequestOptions, b.RequestOptions)
}

func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, fmt.Errorf("agent id is required")
	}
	if s == nil {
		return Agent{}, fmt.Errorf("agent service is required")
	}

	s.mu.Lock()
	current, ok := s.agents[id]
	if !ok {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	previous := current
	runtimeKind := strings.TrimSpace(current.RuntimeKind)
	runningCodex := runtimeKind == RuntimeKindCodex && isRuntimeRunning(current)
	restartRequired := false
	ensureProfile := AgentProfile{}
	shouldEnsureProfile := false
	profileUpdated := false
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			s.mu.Unlock()
			return Agent{}, fmt.Errorf("name is required")
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
	if req.Description != nil {
		current.Description = strings.TrimSpace(*req.Description)
	}
	if req.Image != nil {
		current.Image = strings.TrimSpace(*req.Image)
	}
	if req.AgentProfile != nil || req.RuntimeOptions != nil {
		profileUpdated = true
		profile := current.AgentProfile
		if req.AgentProfile != nil {
			profile = *req.AgentProfile
			if strings.TrimSpace(profile.APIKey) == "" {
				profile.APIKey = current.AgentProfile.APIKey
			}
		}
		var patch map[string]any
		if req.RuntimeOptions != nil {
			patch = *req.RuntimeOptions
		}
		mergedFlat := runtimeOptionsAfterPatch(current.RuntimeKind, current.RuntimeOptions, patch)
		current.RuntimeOptions = nextAgentRuntimeOptions(current.RuntimeKind, current.RuntimeOptions, mergedFlat)
		normalized := normalizeProfileForAgentRuntime(profile, current.RuntimeOptions, current.Name, current.Description, current.RuntimeKind, mergedFlat)
		restartRequired = !profilesEqualEnv(previous.AgentProfile, normalized) || codexProfileRuntimeRestartRequired(previous, normalized)
		normalized.EnvRestartRequired = restartRequired
		current.AgentProfile = normalized
		current.ProfileComplete = normalized.ProfileComplete
		current.Profile = profileSelector(normalized)
		current.DetectionResults = nil
		if normalized.ProfileComplete && runtimeKind == RuntimeKindCodex {
			ensureProfile = normalized
			shouldEnsureProfile = true
		}
	}

	if shouldEnsureProfile {
		s.mu.Unlock()
		if err := s.ensureCodexResponsesAPI(ctx, runtimeKind, ensureProfile); err != nil {
			return Agent{}, err
		}
		s.mu.Lock()
		if _, ok := s.agents[id]; !ok {
			s.mu.Unlock()
			return Agent{}, fmt.Errorf("agent %q not found", id)
		}
	}
	if profileUpdated && current.ProfileComplete {
		s.profileDefaults = cloneProfile(current.AgentProfile)
		s.detectionResults = nil
	}
	s.agents[id] = current
	s.syncRuntimeRecordLocked(current)
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		return Agent{}, err
	}
	s.mu.Unlock()
	if restartRequired && runningCodex {
		s.stopLifecycleAgent(id)
	}

	updated, ok := s.Agent(id)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	return updated, nil
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
		latest, ok := s.currentDefaultImageForAgent(ctx, got)
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
	profile := normalizeProfileForAgentRuntime(got.AgentProfile, got.RuntimeOptions, got.Name, got.Description, got.RuntimeKind, nil)
	if !profile.ProfileComplete {
		return Agent{}, fmt.Errorf("agent %q profile is incomplete", id)
	}
	if err := s.ensureCodexResponsesAPI(ctx, strings.TrimSpace(got.RuntimeKind), profile); err != nil {
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

	if testCreateGatewayBoxHook != nil {
		rt, err := s.ensureRuntime(got.Name)
		if err != nil {
			return Agent{}, err
		}
		runtimeHome, err := s.sandboxRuntimeHome(got.Name)
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
	if err := refreshGatewayTemplateSkills(got.Name, runtimeKind, recreateTemplateRole(got)); err != nil {
		return Agent{}, fmt.Errorf("refresh gateway template skills: %w", err)
	}
	if err := s.provisionRuntime(ctx, runtimeImpl, runtimeKind, agentruntime.ProvisionRequest{
		RuntimeID: createSpec.RuntimeID,
		AgentID:   createSpec.AgentID,
		AgentName: createSpec.AgentName,
		Profile:   runtimeProfile,
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
	current, ok := s.agents[id]
	if !ok {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	current.RuntimeID = normalizeRuntimeID(current.RuntimeID, current.ID)
	if image = strings.TrimSpace(image); image != "" {
		current.Image = image
	}
	current.BoxID = info.HandleID
	current.Status = string(info.State)
	if !info.CreatedAt.IsZero() {
		current.CreatedAt = info.CreatedAt.UTC()
	} else if current.CreatedAt.IsZero() {
		current.CreatedAt = time.Now().UTC()
	}
	current.AgentProfile.EnvRestartRequired = false
	current.AgentProfile.ImageUpgradeRequired = false
	current.ProfileComplete = true
	s.agents[id] = current
	s.syncRuntimeRecordLocked(current)
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return Agent{}, err
	}
	recreated, ok := s.Agent(id)
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
	profile = normalizeProfileForAgentRuntime(profile, nil, spec.Name, spec.Description, spec.RuntimeKind, runtimeOptionsAfterPatch)
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

func nextAgentRuntimeOptions(runtimeKind string, currentRuntimeOptions, mergedRuntimeOptions map[string]any) map[string]any {
	if len(mergedRuntimeOptions) == 0 {
		return currentRuntimeOptions
	}
	return utils.CloneAnyMap(mergedRuntimeOptions)
}
