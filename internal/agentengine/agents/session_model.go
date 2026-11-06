package agents

import (
	"fmt"
	"slices"
	"strings"

	"csgclaw/internal/config"
)

// SessionModelError identifies the request option that could not be resolved.
type SessionModelError struct {
	Field   string
	Message string
}

func (e *SessionModelError) Error() string { return e.Message }

// SessionModelProfiles returns the current provider's chat models with the Agent's defaults.
func (s *ModelConfiguration) SessionModelProfiles(agentID string) ([]AgentProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	profile, providerID, provider, err := s.sessionModelProviderLocked(agentID)
	if err != nil {
		return nil, err
	}
	profiles := []AgentProfile{profile}
	images := imageModels(providerID, provider.Models, provider.ImageModels)
	videos := videoModels(providerID, provider.Models, provider.VideoModels)
	for _, model := range prioritizeOpenCSGAutoModel(providerID, provider.BaseURL, provider.Models) {
		if model == profile.ModelID || containsModelID(images, model) || containsModelID(videos, model) {
			continue
		}
		selected := profile
		selected.Provider = ProfileProviderForModelProviderID(providerID)
		selected.ModelProviderID = providerID
		selected.ModelID = model
		profiles = append(profiles, hydrateProfileFromProvider(selected, provider))
	}
	return profiles, nil
}

// ResolvedSessionProfile applies request options without changing the Agent's Profile.
func (s *ModelConfiguration) ResolvedSessionProfile(agentID, model, reasoningEffort string) (AgentProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	selected, providerID, provider, err := s.sessionModelProviderLocked(agentID)
	if err != nil {
		return AgentProfile{}, err
	}
	model = strings.TrimSpace(model)
	if model != "" && model != selected.ModelID {
		if !slices.Contains(provider.Models, model) || containsModelID(imageModels(providerID, []string{model}, provider.ImageModels), model) || containsModelID(videoModels(providerID, nil, provider.VideoModels), model) {
			return AgentProfile{}, &SessionModelError{Field: "model", Message: "selected model is unavailable in the Agent's provider"}
		}
		selected.Provider = ProfileProviderForModelProviderID(providerID)
		selected.ModelProviderID = providerID
		selected.ModelID = model
		selected = hydrateProfileFromProvider(selected, provider)
	}
	if effort := config.NormalizeReasoningEffort(reasoningEffort); effort != "" {
		switch effort {
		case config.ReasoningEffortAuto, config.ReasoningEffortNone, config.ReasoningEffortMinimal, config.ReasoningEffortLow, config.ReasoningEffortMedium, config.ReasoningEffortHigh, config.ReasoningEffortXHigh, config.ReasoningEffortMax:
			selected.ReasoningEffort = effort
		default:
			return AgentProfile{}, &SessionModelError{Field: "reasoning_effort", Message: "invalid reasoning effort"}
		}
	}
	return selected, nil
}

// Read the Agent and provider together so each selection uses one configuration.
func (s *ModelConfiguration) sessionModelProviderLocked(agentID string) (AgentProfile, string, config.ProviderConfig, error) {
	got, _, ok := s.agentByIDLocked(agentID)
	if !ok {
		return AgentProfile{}, "", config.ProviderConfig{}, fmt.Errorf("agent %q not found", strings.TrimSpace(agentID))
	}
	profile := normalizeProfileForAgentRuntime(got.AgentProfile, got.RuntimeOptions, got.Name, got.Description, got.RuntimeKind, nil)
	providerID := profile.ModelProviderID
	provider, found := ModelProviderConfigForProfile(s.llm, profile)
	if found {
		profile = hydrateProfileFromProvider(profile, provider)
	} else {
		profile = s.hydrateProfileFromCatalogLocked(profile)
	}
	if !profile.ProfileComplete {
		return AgentProfile{}, "", config.ProviderConfig{}, fmt.Errorf("agent %q profile is incomplete", strings.TrimSpace(agentID))
	}
	if providerID == "" {
		providerID, _ = catalogProviderIDForProfile(s.llm, profile)
		provider, _ = ModelProviderConfigForProfile(s.llm, AgentProfile{ModelProviderID: providerID})
	}
	return profile, providerID, provider, nil
}
