package agents

import (
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
	profile, err := s.ResolvedAgentProfile(agentID)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	providerID := profile.ModelProviderID
	if providerID == "" {
		providerID, _ = catalogProviderIDForProfile(s.llm, profile)
	}
	profiles := []AgentProfile{profile}
	for _, provider := range ModelProviderCatalogFromLLM(s.llm).Providers {
		if provider.ID != providerID {
			continue
		}
		for _, model := range provider.Models {
			if model == profile.ModelID || containsModelID(provider.ImageModels, model) || containsModelID(provider.VideoModels, model) {
				continue
			}
			selected := cloneProfile(profile)
			selected.ModelProviderID = providerID
			selected.ModelID = model
			profiles = append(profiles, s.hydrateProfileFromCatalogLocked(selected))
		}
		break
	}
	return profiles, nil
}

// ResolvedSessionProfile applies request options without changing the Agent's Profile.
func (s *ModelConfiguration) ResolvedSessionProfile(agentID, model, reasoningEffort string) (AgentProfile, error) {
	profiles, err := s.SessionModelProfiles(agentID)
	if err != nil {
		return AgentProfile{}, err
	}
	model = strings.TrimSpace(model)
	selected := profiles[0]
	if model != "" && model != selected.ModelID {
		found := false
		for _, profile := range profiles {
			if profile.ModelID == model {
				selected, found = profile, true
				break
			}
		}
		if !found {
			return AgentProfile{}, &SessionModelError{Field: "model", Message: "selected model is unavailable in the Agent's provider"}
		}
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
