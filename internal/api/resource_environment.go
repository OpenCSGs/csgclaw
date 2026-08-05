package api

import (
	"net/http"
	"net/url"
	"os"
	"strings"

	"csgclaw/internal/auth"
	"csgclaw/internal/config"
	hub "csgclaw/internal/template"
)

func (h *Handler) currentOpenCSGEnvironment(r *http.Request) auth.Environment {
	env := auth.DefaultEnvironment()
	if r == nil {
		if baseURL := trustedCSGHubEnvironmentBaseURL(); baseURL != "" {
			env.CSGHubBaseURL = baseURL
		}
		return env
	}
	status, err := appAuthStatus(r)
	if err != nil || !status.Authenticated {
		if baseURL := trustedCSGHubEnvironmentBaseURL(); baseURL != "" {
			env.CSGHubBaseURL = baseURL
		}
		return env
	}
	if openCSGBaseURL := strings.TrimSpace(status.OpenCSGBaseURL); openCSGBaseURL != "" {
		env = auth.EnvironmentForOpenCSGBaseURL(openCSGBaseURL)
	} else if baseURL := strings.TrimRight(strings.TrimSpace(status.BaseURL), "/"); baseURL != "" {
		env.CSGHubBaseURL = baseURL
	}
	if baseURL := strings.TrimRight(strings.TrimSpace(status.AIGatewayBaseURL), "/"); baseURL != "" {
		env.AIGatewayBaseURL = baseURL
	}
	return env
}

func applyOpenCSGEnvironmentToHubConfig(cfg config.HubConfig, env auth.Environment, preserveOfficial bool) config.HubConfig {
	cfg = cfg.Resolved()
	if preserveOfficial {
		return cfg
	}
	hubURL := strings.TrimRight(strings.TrimSpace(env.CSGHubBaseURL), "/")
	if hubURL == "" {
		hubURL = config.DefaultOfficialHubRegistryURL
	}
	for i, registry := range cfg.Registries {
		if strings.TrimSpace(registry.Name) == config.DefaultOfficialHubRegistryName &&
			strings.TrimSpace(registry.Kind) == config.HubRegistryKindRemote {
			registry.URL = hubURL
			cfg.Registries[i] = registry
		}
	}
	return cfg
}

func (h *Handler) hubServiceForRequest(r *http.Request) (*hub.Service, error) {
	if strings.TrimSpace(h.configPath) == "" {
		return h.hub, nil
	}
	cfg, _, err := h.loadBootstrapConfig()
	if err != nil {
		return nil, err
	}
	hubCfg := applyOpenCSGEnvironmentToHubConfig(cfg.Hub, h.currentOpenCSGEnvironment(r), cfg.HasExplicitOfficialHubRegistry())
	username := ""
	managedBaseURL := trustedCSGHubEnvironmentBaseURL()
	managedToken := strings.TrimSpace(os.Getenv("CSGHUB_USER_TOKEN"))
	status, statusErr := appAuthStatus(r)
	if statusErr != nil {
		return nil, statusErr
	}
	if status.Authenticated {
		token, tokenErr := currentOpenCSGAccessToken()
		if tokenErr != nil {
			return nil, tokenErr
		}
		for i, registry := range hubCfg.Registries {
			if strings.TrimSpace(registry.Name) == config.DefaultOfficialHubRegistryName &&
				strings.TrimSpace(registry.Kind) == config.HubRegistryKindRemote &&
				strings.EqualFold(
					strings.TrimRight(strings.TrimSpace(registry.URL), "/"),
					strings.TrimRight(strings.TrimSpace(status.BaseURL), "/"),
				) {
				registry.Token = strings.TrimSpace(token)
				username = strings.TrimSpace(status.UserID)
				hubCfg.Registries[i] = registry
			}
		}
	} else {
		for i, registry := range hubCfg.Registries {
			if strings.TrimSpace(registry.Name) == config.DefaultOfficialHubRegistryName &&
				strings.TrimSpace(registry.Kind) == config.HubRegistryKindRemote &&
				(managedBaseURL == "" || strings.EqualFold(strings.TrimRight(strings.TrimSpace(registry.URL), "/"), managedBaseURL)) {
				registry.Token = managedToken
				username = strings.TrimSpace(os.Getenv("CSGHUB_USER_NAME"))
				hubCfg.Registries[i] = registry
			}
		}
	}
	return hub.NewService(hubCfg, func(registry config.HubRegistryConfig) (hub.Store, error) {
		if strings.TrimSpace(registry.Name) == config.DefaultOfficialHubRegistryName &&
			strings.TrimSpace(registry.Kind) == config.HubRegistryKindRemote &&
			username != "" {
			return hub.NewAuthenticatedRemoteStore(registry.URL, registry.Token, username), nil
		}
		return hub.DefaultStoreFactory(registry)
	})
}

func trustedCSGHubEnvironmentBaseURL() string {
	raw := strings.TrimRight(strings.TrimSpace(os.Getenv("CSGHUB_BASE_URL")), "/")
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	return raw
}

func (h *Handler) officialHubBaseURLForRequest(r *http.Request, cfg config.Config) string {
	return strings.TrimRight(strings.TrimSpace(h.officialHubRegistryForRequest(r, cfg).URL), "/")
}

func skillConfigForEnvironment(cfg config.SkillConfig, env auth.Environment) config.SkillConfig {
	out := cfg
	if strings.TrimSpace(out.BaseURL) == "" {
		out.BaseURL = defaultSkillRegistryBaseURL(env)
	}
	if !out.OfficialBaseURLSet && strings.TrimSpace(out.OfficialBaseURL) == "" && isStageOpenCSGEnvironment(env) {
		out.OfficialBaseURLSet = true
		out.OfficialBaseURL = ""
	}
	return out
}

func defaultSkillRegistryBaseURL(env auth.Environment) string {
	if isDefaultOpenCSGEnvironment(env) {
		return config.DefaultSkillBaseURL
	}
	return config.DefaultSkillBaseURL
}

func isStageOpenCSGEnvironment(env auth.Environment) bool {
	openCSGBaseURL := strings.TrimRight(strings.TrimSpace(env.OpenCSGBaseURL), "/")
	csgHubBaseURL := strings.TrimRight(strings.TrimSpace(env.CSGHubBaseURL), "/")
	return openCSGBaseURL == auth.StageOpenCSGBaseURL || csgHubBaseURL == auth.StageCSGHubBaseURL
}

func isDefaultOpenCSGEnvironment(env auth.Environment) bool {
	openCSGBaseURL := strings.TrimRight(strings.TrimSpace(env.OpenCSGBaseURL), "/")
	csgHubBaseURL := strings.TrimRight(strings.TrimSpace(env.CSGHubBaseURL), "/")
	return openCSGBaseURL == auth.DefaultOpenCSGBaseURL || csgHubBaseURL == auth.DefaultCSGHubBaseURL
}
