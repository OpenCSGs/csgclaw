package agents

import (
	context "context"
	config "csgclaw/internal/config"
	fmt "fmt"
	strings "strings"
)

func (s *ModelConfiguration) resolveModelProfile(profile string) (string, config.ModelConfig, error) {
	if strings.TrimSpace(profile) != "" {
		name, cfg, err := s.llm.Resolve(profile)
		if err != nil {
			return "", config.ModelConfig{}, err
		}
		return name, cfg, nil
	}

	name, cfg, err := s.llm.Resolve("")
	if err != nil {
		return "", config.ModelConfig{}, err
	}
	return name, cfg, nil
}

func (s *ModelConfiguration) inferProfileForAgent(got Agent) string {
	if profile := strings.TrimSpace(got.Profile); profile != "" {
		if _, _, err := s.llm.Resolve(profile); err == nil {
			return profile
		}
	}
	if strings.TrimSpace(got.AgentProfile.Provider) != "" || strings.TrimSpace(got.AgentProfile.ModelID) != "" {
		if name, _, ok := s.llm.MatchProfile(config.ModelConfig{
			Provider:        got.AgentProfile.Provider,
			ModelID:         got.AgentProfile.ModelID,
			ReasoningEffort: got.AgentProfile.ReasoningEffort,
		}); ok {
			return name
		}
	}
	name, _, err := s.llm.Resolve("")
	if err != nil {
		return ""
	}
	return name
}

func (s *ModelConfiguration) modelConfigForAgent(got Agent) (string, config.ModelConfig, error) {
	if selector := profileSelector(got.AgentProfile); selector != "" {
		if profile := s.hydrateProfileFromCatalog(got.AgentProfile); profile.ModelProviderID != "" && profile.ProfileComplete {
			return selector, modelConfigFromProfile(profile), nil
		}
	}
	profile := s.inferProfileForAgent(got)
	if profile == "" {
		return "", config.ModelConfig{}, fmt.Errorf("no llm profile could be resolved for agent %q", strings.TrimSpace(got.ID))
	}
	name, cfg, err := s.llm.Resolve(profile)
	if err != nil {
		return "", config.ModelConfig{}, err
	}
	return name, cfg.Resolved(), nil
}

func (s *ModelConfiguration) ResolvedModelConfig(agentID string) (config.ModelConfig, error) {
	got, ok := s.agentSnapshot(agentID)
	if !ok {
		return config.ModelConfig{}, fmt.Errorf("agent %q not found", strings.TrimSpace(agentID))
	}
	_, cfg, err := s.modelConfigForAgent(got)
	if err != nil {
		return config.ModelConfig{}, err
	}
	return cfg, nil
}

func (s *ModelConfiguration) SetLLMConfig(llmCfg config.LLMConfig) {
	if s == nil {
		return
	}
	llmCfg = llmCfg.Normalized()
	defaultSelector, defaultModel, err := llmCfg.Resolve("")
	s.mu.Lock()
	s.llm = llmCfg
	s.normalizeProfileReference = profileCatalogNormalizer(llmCfg)
	if err == nil {
		s.model = defaultModel
		if strings.TrimSpace(s.profileDefaults.ModelProviderID) != "" || strings.TrimSpace(s.profileDefaults.Provider) == "" {
			s.profileDefaults = profileFromConfigModel(defaultSelector, "", defaultModel)
		}
	}
	s.mu.Unlock()
}

func (s *ModelConfiguration) AgentProfileView(id string) (AgentProfileView, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return AgentProfileView{}, fmt.Errorf("agent id is required")
	}
	got, ok := s.agentSnapshot(id)
	if !ok {
		return AgentProfileView{}, fmt.Errorf("agent %q not found", id)
	}
	return profileViewWithAgentRuntimeOptions(got.AgentProfile, got.RuntimeOptions, got.RuntimeKind, got.DetectionResults), nil
}

func (s *ModelConfiguration) ProfileDefaultsView() AgentProfileView {
	if s == nil {
		return AgentProfileView{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return profileView(s.profileDefaults, s.detectionResults)
}

func (s *ModelConfiguration) ListModelsForRequest(ctx context.Context, req ProfileModelRequest) ([]string, error) {
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
	models, err := ListModelsForProfile(ctx, profile)
	if err != nil {
		return nil, err
	}
	return sortModelIDs(models), nil
}

func (s *ModelConfiguration) hydrateProfileFromCatalog(profile AgentProfile) AgentProfile {
	if s == nil {
		return profile
	}
	s.mu.RLock()
	out := s.hydrateProfileFromCatalogLocked(profile)
	s.mu.RUnlock()
	return out
}

func (s *ModelConfiguration) hydrateProfileFromCatalogLocked(profile AgentProfile) AgentProfile {
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

func (s *ModelConfiguration) inheritModelProviderReference(profile AgentProfile, current Agent) AgentProfile {
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

func (s *ModelConfiguration) storedAPIKeyForModelRequest(req ProfileModelRequest, profile AgentProfile) string {
	agentID := strings.TrimSpace(req.AgentID)
	if s == nil || agentID == "" || profile.Provider != ProviderAPI {
		return ""
	}
	got, ok := s.agentSnapshot(agentID)
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

func (s *ModelConfiguration) withDefaultAPIKeyForMatchingProfile(profile AgentProfile) AgentProfile {
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

func (s *ModelConfiguration) ResolvedAgentProfile(agentID string) (AgentProfile, error) {
	got, ok := s.agentSnapshot(agentID)
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
