package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"csgclaw/internal/auth"
	"csgclaw/internal/config"
)

// remoteHubRegistryForRequest resolves the official Hub after applying the
// current OpenCSG environment. Remote resource endpoints share this selection
// policy even when their Hub protocol and authentication requirements differ.
func (h *Handler) remoteHubRegistryForRequest(r *http.Request) (config.HubRegistryConfig, error) {
	cfg, _, err := h.loadBootstrapConfig()
	if err != nil {
		return config.HubRegistryConfig{}, err
	}
	return h.officialHubRegistryForRequest(r, cfg), nil
}

func (h *Handler) officialHubRegistryForRequest(r *http.Request, cfg config.Config) config.HubRegistryConfig {
	hubCfg := applyOpenCSGEnvironmentToHubConfig(cfg.Hub, h.currentOpenCSGEnvironment(r))
	for _, registry := range hubCfg.Resolved().Registries {
		if strings.TrimSpace(registry.Name) == config.DefaultOfficialHubRegistryName &&
			strings.TrimSpace(registry.Kind) == config.HubRegistryKindRemote {
			return registry
		}
	}
	return config.HubRegistryConfig{}
}

// remoteMCPHubConnection resolves the authenticated connection used by the
// remote MCP protocol. Other remote resources can reuse the registry without
// inheriting MCP's credential requirement.
func (h *Handler) remoteMCPHubConnection(r *http.Request) (string, string, error) {
	registry, err := h.remoteHubRegistryForRequest(r)
	if err != nil {
		return "", "", err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(registry.URL), "/")
	if baseURL == "" {
		return "", "", errRemoteMCPHubNotConfigured
	}
	token := strings.TrimSpace(registry.Token)
	if token != "" {
		return baseURL, token, nil
	}
	token, err = remoteMCPHubAccessToken()
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(token) == "" {
		return "", "", errRemoteMCPHubSignInRequired
	}
	return baseURL, token, nil
}

func currentOpenCSGAccessToken() (string, error) {
	record, found, err := auth.Default().Store.Load()
	if err != nil {
		return "", fmt.Errorf("read OpenCSG authentication: %w", err)
	}
	if !found {
		return "", nil
	}
	return strings.TrimSpace(record.Tokens.AccessToken), nil
}

func remoteHubPageOptions(r *http.Request, defaultPage, defaultPer, maxPer int) (int, int, error) {
	page, err := remoteHubQueryInt(r, "page", defaultPage, 0)
	if err != nil {
		return 0, 0, err
	}
	per, err := remoteHubQueryInt(r, "per", defaultPer, maxPer)
	if err != nil {
		return 0, 0, err
	}
	return page, per, nil
}

func remoteHubQueryInt(r *http.Request, key string, fallback, maximum int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || (maximum > 0 && value > maximum) {
		if maximum > 0 {
			return 0, fmt.Errorf("%s must be an integer between 1 and %d", key, maximum)
		}
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}
