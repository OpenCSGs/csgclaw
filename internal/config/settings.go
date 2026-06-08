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
	DefaultManagerTemplate   string
	DefaultWorkerTemplate    string
	SupportedSandboxProvider []string
}

func UserSettingsFromConfig(cfg Config) UserSettings {
	resolved := cfg.Sandbox.Resolved()
	return UserSettings{
		ListenAddr:               cfg.Server.ListenAddr,
		AdvertiseBaseURL:         cfg.Server.AdvertiseBaseURL,
		AccessToken:              cfg.Server.AccessToken,
		ShowUpgrade:              cfg.Server.ShowUpgrade,
		SandboxProvider:          resolved.Provider,
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
	cfg.Bootstrap.DefaultManagerTemplate = normalizeBootstrapTemplateRef(settings.DefaultManagerTemplate)
	cfg.Bootstrap.DefaultWorkerTemplate = normalizeBootstrapTemplateRef(settings.DefaultWorkerTemplate)

	if err := validateUserSettings(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
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
