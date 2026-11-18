package agents

import (
	"fmt"
	"slices"
	"strings"

	"csgclaw/internal/config"
	"csgclaw/internal/modelcap"
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
	for _, model := range prioritizeOpenCSGAutoModel(providerID, provider.BaseURL, provider.Models) {
		if model == profile.ModelID || isMediaGenerationModel(provider, model) {
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
	if model != "" && isMediaGenerationModel(provider, model) {
		return AgentProfile{}, &SessionModelError{Field: "model", Message: fmt.Sprintf("model %q is a media generation model and cannot be used for chat", model)}
	}
	if model != "" && model != selected.ModelID {
		selected.ModelID = model
		selected.ModelProviderID = providerID
		if providerID == "" {
			selected.InputModalities = resolvedInputModalities(selected.Provider, model, false)
		}
		selected = s.hydrateProfileFromCatalogLocked(selected)
	}
	if effort := config.NormalizeReasoningEffort(reasoningEffort); effort != "" {
		switch effort {
		case config.ReasoningEffortAuto, config.ReasoningEffortNone, config.ReasoningEffortMinimal, config.ReasoningEffortLow, config.ReasoningEffortMedium, config.ReasoningEffortHigh, config.ReasoningEffortXHigh, config.ReasoningEffortMax:
		default:
			return AgentProfile{}, &SessionModelError{Field: "reasoning_effort", Message: "invalid reasoning effort"}
		}
		if effort != config.ReasoningEffortAuto {
			levels := modelcap.ForProviderModel(selected.Provider, selected.ModelID).SupportedReasoningEfforts
			// Unverified provider/model combinations accept all recognized effort names.
			if len(levels) > 0 && !slices.Contains(levels, effort) {
				return AgentProfile{}, &SessionModelError{Field: "reasoning_effort", Message: fmt.Sprintf("reasoning effort %q is unsupported by model %q", effort, selected.ModelID)}
			}
		}
		selected.ReasoningEffort = effort
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
