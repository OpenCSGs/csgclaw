package agent

import (
	"fmt"
	"strings"

	"csgclaw/internal/config"
)

func (s *Service) resolveModelProfile(profile string) (string, config.ModelConfig, error) {
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

func (s *Service) inferProfileForAgent(got Agent) string {
	if profile := strings.TrimSpace(got.Profile); profile != "" {
		if _, _, err := s.llm.Resolve(profile); err == nil {
			return profile
		}
	}
	if strings.TrimSpace(got.Provider) != "" || strings.TrimSpace(got.ModelID) != "" {
		if name, _, ok := s.llm.MatchProfile(config.ModelConfig{
			Provider:        got.Provider,
			ModelID:         got.ModelID,
			ReasoningEffort: got.ReasoningEffort,
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

func (s *Service) modelConfigForAgent(got Agent) (string, config.ModelConfig, error) {
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

func (s *Service) ResolvedModelConfig(agentID string) (config.ModelConfig, error) {
	got, ok := s.Agent(agentID)
	if !ok {
		return config.ModelConfig{}, fmt.Errorf("agent %q not found", strings.TrimSpace(agentID))
	}
	_, cfg, err := s.modelConfigForAgent(got)
	if err != nil {
		return config.ModelConfig{}, err
	}
	return cfg, nil
}
