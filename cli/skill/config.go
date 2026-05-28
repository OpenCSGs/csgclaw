package skill

import (
	"fmt"
	"os"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/config"
)

func resolveSkillConfig(globals command.GlobalOptions) (config.SkillConfig, error) {
	path := strings.TrimSpace(globals.Config)
	explicit := path != ""
	if !explicit {
		if p, err := config.DefaultPath(); err == nil {
			path = p
		}
	}
	if path == "" {
		return config.SkillConfig{NonSuspiciousOnly: true}.Resolved(), nil
	}
	if _, err := os.Stat(path); err != nil {
		if explicit {
			return config.SkillConfig{}, fmt.Errorf("load config %q: %w", path, err)
		}
		if os.IsNotExist(err) {
			return config.SkillConfig{NonSuspiciousOnly: true}.Resolved(), nil
		}
		return config.SkillConfig{}, fmt.Errorf("stat config %q: %w", path, err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.SkillConfig{}, fmt.Errorf("load config %q: %w", path, err)
	}
	return cfg.Skill, nil
}
