package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillConfigResolvedDefaults(t *testing.T) {
	t.Parallel()

	got := (SkillConfig{}).Resolved()
	if got.BaseURL != DefaultSkillBaseURL {
		t.Fatalf("BaseURL = %q, want %q", got.BaseURL, DefaultSkillBaseURL)
	}
	if got.OfficialBaseURL != DefaultSkillOfficialBaseURL {
		t.Fatalf("OfficialBaseURL = %q, want %q", got.OfficialBaseURL, DefaultSkillOfficialBaseURL)
	}
}

func TestSkillConfigResolvedDisablesOfficialWhenSetEmpty(t *testing.T) {
	t.Parallel()

	got := (SkillConfig{OfficialBaseURLSet: true}).Resolved()
	if got.OfficialBaseURL != "" {
		t.Fatalf("OfficialBaseURL = %q, want empty", got.OfficialBaseURL)
	}
}

func TestLoadReadsSkillConfig(t *testing.T) {
	t.Setenv("SKILL_TOKEN", "skill-test")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[skill]
base_url = "https://claw.example.com"
token = "${SKILL_TOKEN}"
non_suspicious_only = false

[bootstrap]
default_manager_template = "builtin/picoclaw-manager"
default_worker_template = "builtin/picoclaw-worker"

[models]
default = "default.minimax-m2.7"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
models = ["minimax-m2.7"]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.Skill.BaseURL, "https://claw.example.com"; got != want {
		t.Fatalf("Skill.BaseURL = %q, want %q", got, want)
	}
	if got, want := cfg.Skill.Token, "skill-test"; got != want {
		t.Fatalf("Skill.Token = %q, want %q", got, want)
	}
	if cfg.Skill.NonSuspiciousOnly {
		t.Fatal("Skill.NonSuspiciousOnly = true, want false")
	}
}

func TestLoadReadsLegacyClawHubSection(t *testing.T) {
	t.Setenv("CLAWHUB_TOKEN", "legacy-test")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[clawhub]
base_url = "https://claw.legacy.example"
token = "${CLAWHUB_TOKEN}"

[bootstrap]
default_manager_template = "builtin/picoclaw-manager"
default_worker_template = "builtin/picoclaw-worker"

[models]
default = "default.minimax-m2.7"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
models = ["minimax-m2.7"]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.Skill.BaseURL, "https://claw.legacy.example"; got != want {
		t.Fatalf("Skill.BaseURL = %q, want %q", got, want)
	}
	if got, want := cfg.Skill.Token, "legacy-test"; got != want {
		t.Fatalf("Skill.Token = %q, want %q", got, want)
	}
}
