package agents

import (
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/taskconfig"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func (s *ModelConfiguration) ResolvedTaskProfile(agentID string, cfg taskconfig.Config) (AgentProfile, error) {
	got, ok := s.agentSnapshot(agentID)
	if !ok {
		return AgentProfile{}, fmt.Errorf("agent not found")
	}
	profile := got.AgentProfile
	if cfg.ModelProviderID != "" || cfg.ModelID != "" {
		if cfg.ModelProviderID == "" || cfg.ModelID == "" {
			return AgentProfile{}, fmt.Errorf("model and provider must be selected together")
		}
		s.mu.RLock()
		catalog := ModelProviderCatalogFromLLM(s.llm)
		selected, ok := CatalogProviderModelConfig(s.llm, cfg.ModelProviderID+"."+cfg.ModelID)
		s.mu.RUnlock()
		available := false
		for _, provider := range catalog.Providers {
			if provider.ID == cfg.ModelProviderID && containsModelID(provider.Models, cfg.ModelID) && !containsModelID(provider.ImageModels, cfg.ModelID) && !containsModelID(provider.VideoModels, cfg.ModelID) {
				available = true
				break
			}
		}
		ok = ok && available
		if !ok {
			return AgentProfile{}, fmt.Errorf("selected model is unavailable")
		}
		selected.Env = cloneStringMap(got.AgentProfile.Env)
		selected.EnableFastMode = got.AgentProfile.EnableFastMode
		profile = selected
	}
	if cfg.ReasoningEffort != "" {
		profile.ReasoningEffort = cfg.ReasoningEffort
	}
	profile = s.hydrateProfileFromCatalog(normalizeProfile(profile, got.Name, got.Description))
	if !profile.ProfileComplete {
		return AgentProfile{}, fmt.Errorf("model profile is incomplete")
	}
	return profile, nil
}

func (s *Controller) TaskRuntimeProfile(agentID, roomID string, cfg taskconfig.Config) (agentruntime.Profile, error) {
	got, ok := s.agentSnapshot(agentID)
	if !ok {
		return agentruntime.Profile{}, fmt.Errorf("agent not found")
	}
	profile, err := s.Models().ResolvedTaskProfile(agentID, cfg)
	if err != nil {
		return agentruntime.Profile{}, err
	}
	resolved := s.runtimeProfileForAgentWithProfile(got, profile)
	resolved.BaseURL = strings.TrimSuffix(resolved.BaseURL, "/llm") + "/tasks/" + url.PathEscape(roomID) + "/llm"
	return resolved, nil
}

func (s *ModelConfiguration) ValidateTaskConfiguration(agentID string, cfg taskconfig.Config) (taskconfig.Config, error) {
	got, ok := s.agentSnapshot(agentID)
	if !ok {
		return cfg, fmt.Errorf("agent not found")
	}
	if got.RuntimeKind != RuntimeKindCodex && got.RuntimeKind != RuntimeKindDSH {
		return cfg, fmt.Errorf("this runtime does not support independent session configuration")
	}
	if _, err := s.ResolvedTaskProfile(agentID, cfg); err != nil {
		return cfg, err
	}
	switch cfg.ReasoningEffort {
	case "", "auto", "none", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return cfg, fmt.Errorf("invalid reasoning effort")
	}
	cfg.WorkspaceDir = strings.TrimSpace(cfg.WorkspaceDir)
	if cfg.WorkspaceDir != "" {
		if !filepath.IsAbs(cfg.WorkspaceDir) {
			return cfg, fmt.Errorf("working directory must be absolute")
		}
		stat, err := os.Stat(cfg.WorkspaceDir)
		if err != nil || !stat.IsDir() {
			return cfg, fmt.Errorf("working directory is unavailable")
		}
		cfg.WorkspaceDir = filepath.Clean(cfg.WorkspaceDir)
	}
	if got.RuntimeKind == RuntimeKindCodex {
		if cfg.ExecutionMode != "" && cfg.ExecutionMode != "standard" && cfg.ExecutionMode != "read_only" {
			return cfg, fmt.Errorf("invalid execution mode")
		}
		if strings.EqualFold(os.Getenv("CSGCLAW_EXECUTION_MODE"), "read_only") && cfg.ExecutionMode == "standard" {
			return cfg, fmt.Errorf("the server requires read-only execution")
		}
	} else {
		switch cfg.ExecutionMode {
		case "", "read-only", "workspace-write", "danger-full-access":
		default:
			return cfg, fmt.Errorf("invalid permission mode")
		}
	}
	return cfg, nil
}
