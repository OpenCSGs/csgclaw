package config

import (
	"fmt"
	"net"
	"strings"
)

// UserSettings are the config.toml fields exposed in the Web UI settings form.
type UserSettings struct {
	ListenAddr               string
	AdvertiseBaseURL         string
	AccessToken              string
	ShowUpgrade              bool
	SandboxProvider          string
	HubLocalPath             string
	HubOfficialURL           string
	DefaultManagerTemplate   string
	DefaultWorkerTemplate    string
	SupportedSandboxProvider []string
}

func UserSettingsFromConfig(cfg Config) UserSettings {
	resolved := cfg.Sandbox.Resolved()
	resolvedHub := cfg.Hub.Resolved()
	local := findHubRegistry(resolvedHub.Registries, DefaultHubPublishRegistry)
	official := findHubRegistry(resolvedHub.Registries, DefaultOfficialHubRegistryName)
	return UserSettings{
		ListenAddr:               cfg.Server.ListenAddr,
		AdvertiseBaseURL:         cfg.Server.AdvertiseBaseURL,
		AccessToken:              cfg.Server.AccessToken,
		ShowUpgrade:              cfg.Server.ShowUpgrade,
		SandboxProvider:          resolved.Provider,
		HubLocalPath:             local.Path,
		HubOfficialURL:           official.URL,
		DefaultManagerTemplate:   cfg.Bootstrap.ResolvedDefaultManagerTemplate(),
		DefaultWorkerTemplate:    cfg.Bootstrap.ResolvedDefaultWorkerTemplate(),
		SupportedSandboxProvider: SupportedSandboxProviders(),
	}
}

func SupportedSandboxProviders() []string {
	return []string{BoxLiteProvider, DockerProvider, CSGHubProvider}
}

func AccessTokenPreview(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	runes := []rune(token)
	if len(runes) < 9 {
		return ""
	}
	return string(runes[:4]) + "..."
}

func ApplyUserSettings(cfg Config, settings UserSettings) (Config, error) {
	cfg.Server.ListenAddr = strings.TrimSpace(settings.ListenAddr)
	cfg.Server.AdvertiseBaseURL = strings.TrimRight(strings.TrimSpace(settings.AdvertiseBaseURL), "/")
	cfg.Server.AccessToken = strings.TrimSpace(settings.AccessToken)
	cfg.Server.ShowUpgrade = settings.ShowUpgrade
	cfg.Sandbox.Provider = strings.TrimSpace(settings.SandboxProvider)
	cfg.Hub = applyHubUserSettings(cfg.Hub, settings)
	cfg.Bootstrap.DefaultManagerTemplate = normalizeBootstrapTemplateRef(settings.DefaultManagerTemplate)
	cfg.Bootstrap.DefaultWorkerTemplate = normalizeBootstrapTemplateRef(settings.DefaultWorkerTemplate)

	if err := validateUserSettings(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyHubUserSettings(hub HubConfig, settings UserSettings) HubConfig {
	hub = hub.Resolved()
	localPath := strings.TrimSpace(settings.HubLocalPath)
	officialURL := strings.TrimSpace(strings.TrimRight(settings.HubOfficialURL, "/"))
	for i, registry := range hub.Registries {
		switch strings.TrimSpace(registry.Name) {
		case DefaultHubPublishRegistry:
			if strings.TrimSpace(registry.Kind) == HubRegistryKindLocal && localPath != "" {
				registry.Path = localPath
			}
		case DefaultOfficialHubRegistryName:
			if strings.TrimSpace(registry.Kind) == HubRegistryKindRemote && officialURL != "" {
				registry.URL = officialURL
			}
		}
		hub.Registries[i] = registry
	}
	return hub
}

func findHubRegistry(registries []HubRegistryConfig, name string) HubRegistryConfig {
	name = strings.TrimSpace(name)
	for _, registry := range registries {
		if strings.TrimSpace(registry.Name) == name {
			return registry
		}
	}
	return HubRegistryConfig{}
}

func validateUserSettings(cfg Config) error {
	if strings.TrimSpace(cfg.Server.ListenAddr) == "" {
		return fmt.Errorf("server.listen_addr is required")
	}
	if _, _, err := net.SplitHostPort(cfg.Server.ListenAddr); err != nil {
		return fmt.Errorf("server.listen_addr must be a host:port address: %w", err)
	}
	if strings.TrimSpace(cfg.Server.AccessToken) == "" {
		return fmt.Errorf("server.access_token is required")
	}
	if err := cfg.Bootstrap.Validate(); err != nil {
		return err
	}
	cfg.Sandbox = cfg.Sandbox.Resolved()
	if err := cfg.Sandbox.Validate(); err != nil {
		return err
	}
	return nil
}
