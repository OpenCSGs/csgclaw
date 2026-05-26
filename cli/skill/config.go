package skill

import (
	"fmt"
	"os"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/config"
)

func resolveClawHubConfig(globals command.GlobalOptions) (config.ClawHubConfig, error) {
	path := strings.TrimSpace(globals.Config)
	explicit := path != ""
	if !explicit {
		if p, err := config.DefaultPath(); err == nil {
			path = p
		}
	}
	if path == "" {
		return config.ClawHubConfig{NonSuspiciousOnly: true}.Resolved(), nil
	}
	if _, err := os.Stat(path); err != nil {
		if explicit {
			return config.ClawHubConfig{}, fmt.Errorf("load config %q: %w", path, err)
		}
		if os.IsNotExist(err) {
			return config.ClawHubConfig{NonSuspiciousOnly: true}.Resolved(), nil
		}
		return config.ClawHubConfig{}, fmt.Errorf("stat config %q: %w", path, err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.ClawHubConfig{}, fmt.Errorf("load config %q: %w", path, err)
	}
	return cfg.ClawHub, nil
}
