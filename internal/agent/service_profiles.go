package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
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
	return profileView(got.AgentProfile, got.DetectionResults), nil
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
	defer s.mu.Unlock()

	current, ok := s.agents[id]
	if !ok {
		return AgentProfileView{}, fmt.Errorf("agent %q not found", id)
	}
	if strings.TrimSpace(profile.APIKey) == "" {
		profile.APIKey = current.AgentProfile.APIKey
	}
	normalized := normalizeProfile(profile, current.Name, current.Description)
	normalized.EnvRestartRequired = !profilesEqualEnv(current.AgentProfile, normalized)
	current.AgentProfile = normalized
	current.ProfileComplete = normalized.ProfileComplete
	current.Profile = profileSelector(normalized)
	current.Provider = normalized.Provider
	current.ModelID = normalized.ModelID
	current.ReasoningEffort = normalized.ReasoningEffort
	current.DetectionResults = nil
	s.agents[id] = current
	if normalized.ProfileComplete {
		s.profileDefaults = cloneProfile(normalized)
		s.detectionResults = nil
	}
	if err := s.saveLocked(); err != nil {
		return AgentProfileView{}, err
	}
	return profileView(normalized, current.DetectionResults), nil
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
	if req.AgentProfile != nil {
		profile := *req.AgentProfile
		if strings.TrimSpace(profile.APIKey) == "" {
			profile.APIKey = current.AgentProfile.APIKey
		}
		normalized := normalizeProfile(profile, current.Name, current.Description)
		normalized.EnvRestartRequired = !profilesEqualEnv(current.AgentProfile, normalized)
		current.AgentProfile = normalized
		current.ProfileComplete = normalized.ProfileComplete
		current.Profile = profileSelector(normalized)
		current.Provider = normalized.Provider
		current.ModelID = normalized.ModelID
		current.ReasoningEffort = normalized.ReasoningEffort
		current.DetectionResults = nil
		if normalized.ProfileComplete {
			s.profileDefaults = cloneProfile(normalized)
			s.detectionResults = nil
		}
	}
	s.agents[id] = current
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		return Agent{}, err
	}
	s.mu.Unlock()

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

func (s *Service) ResolvedAgentProfile(agentID string) (AgentProfile, error) {
	got, ok := s.Agent(agentID)
	if !ok {
		return AgentProfile{}, fmt.Errorf("agent %q not found", strings.TrimSpace(agentID))
	}
	profile := normalizeProfile(got.AgentProfile, got.Name, got.Description)
	if !profile.ProfileComplete {
		return AgentProfile{}, fmt.Errorf("agent %q profile is incomplete", strings.TrimSpace(agentID))
	}
	return profile, nil
}

func (s *Service) Recreate(ctx context.Context, id string) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, fmt.Errorf("agent id is required")
	}
	got, ok := s.Agent(id)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	profile := normalizeProfile(got.AgentProfile, got.Name, got.Description)
	if !profile.ProfileComplete {
		return Agent{}, fmt.Errorf("agent %q profile is incomplete", id)
	}

	runtimeImpl, err := s.runtimeForAgent(got)
	if err != nil {
		return Agent{}, err
	}
	image := strings.TrimSpace(got.Image)
	if image == "" {
		image = s.managerImage
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
			if err := s.forceRemoveBox(ctx, rt, got.BoxID); err != nil && !sandbox.IsNotFound(err) {
				return Agent{}, fmt.Errorf("remove existing agent box: %w", err)
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
		return s.persistRecreatedAgent(ctx, id, info)
	}
	if err := runtimeImpl.Delete(ctx, runtimeHandleForAgent(got)); err != nil && !sandbox.IsNotFound(err) {
		return Agent{}, fmt.Errorf("remove existing agent box: %w", err)
	}
	handle, err := runtimeImpl.Create(ctx, agentruntime.Spec{
		RuntimeID: normalizeRuntimeID(got.RuntimeID, got.ID),
		AgentID:   got.ID,
		AgentName: got.Name,
		Image:     image,
		Profile:   s.runtimeProfileForKind(runtimeKindForAgent(got), got.ID, got.Name, got.Description, profile),
	})
	if err != nil {
		return Agent{}, fmt.Errorf("create agent box: %w", err)
	}
	info, err := s.runtimeInfo(ctx, runtimeImpl, handle)
	if err != nil {
		return Agent{}, fmt.Errorf("read agent runtime info: %w", err)
	}
	return s.persistRecreatedAgent(ctx, id, info)
}

func (s *Service) persistRecreatedAgent(ctx context.Context, id string, info agentruntime.Info) (Agent, error) {
	s.mu.Lock()
	current, ok := s.agents[id]
	if !ok {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	current.RuntimeID = normalizeRuntimeID(current.RuntimeID, current.ID)
	current.BoxID = info.HandleID
	current.Status = string(info.State)
	if !info.CreatedAt.IsZero() {
		current.CreatedAt = info.CreatedAt.UTC()
	} else if current.CreatedAt.IsZero() {
		current.CreatedAt = time.Now().UTC()
	}
	current.AgentProfile.EnvRestartRequired = false
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

func (s *Service) profileForCreateRequest(ctx context.Context, req CreateAgentSpec) (AgentProfile, error) {
	profile := req.AgentProfile
	if strings.TrimSpace(profile.ModelID) == "" && strings.TrimSpace(req.ModelID) != "" {
		profile.ModelID = strings.TrimSpace(req.ModelID)
	}
	if strings.TrimSpace(profile.Provider) == "" && strings.TrimSpace(req.Profile) != "" {
		if _, cfg, err := s.llm.Resolve(req.Profile); err == nil {
			profile = profileFromConfigModel(req.Name, req.Description, cfg)
		} else if provider, modelID, ok := splitProfileSelector(req.Profile); ok {
			profile.Provider = provider
			if strings.TrimSpace(profile.ModelID) == "" {
				profile.ModelID = modelID
			}
		}
	}
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
	profile = normalizeProfile(profile, req.Name, req.Description)
	if !profile.ProfileComplete {
		detected, _ := s.DetectDefaultProfile(ctx)
		if detected.ProfileComplete {
			detected.Name = strings.TrimSpace(req.Name)
			detected.Description = strings.TrimSpace(req.Description)
			return normalizeProfile(detected, req.Name, req.Description), nil
		}
		return AgentProfile{}, fmt.Errorf("agent profile is incomplete")
	}
	return profile, nil
}

func splitProfileSelector(selector string) (string, string, bool) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", "", false
	}
	for _, sep := range []string{".", ":"} {
		provider, modelID, ok := strings.Cut(selector, sep)
		if ok && strings.TrimSpace(provider) != "" && strings.TrimSpace(modelID) != "" {
			return normalizeProfileProvider(provider), strings.TrimSpace(modelID), true
		}
	}
	return normalizeProfileProvider(selector), "", true
}
