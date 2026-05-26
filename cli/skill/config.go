package skill

import (
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/config"
)

func resolveClawHubConfig(globals command.GlobalOptions) config.ClawHubConfig {
	path := strings.TrimSpace(globals.Config)
	if path == "" {
		if p, err := config.DefaultPath(); err == nil {
			path = p
		}
	}
	if path != "" {
		if cfg, err := config.Load(path); err == nil {
			return cfg.ClawHub
		}
	}
	out := config.ClawHubConfig{NonSuspiciousOnly: true}.Resolved()
	return out
}
