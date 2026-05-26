package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClawHubConfigResolvedDefaults(t *testing.T) {
	t.Parallel()

	got := (ClawHubConfig{}).Resolved()
	if got.BaseURL != DefaultClawHubBaseURL {
		t.Fatalf("BaseURL = %q, want %q", got.BaseURL, DefaultClawHubBaseURL)
	}
	if got.OfficialBaseURL != DefaultClawHubOfficialBaseURL {
		t.Fatalf("OfficialBaseURL = %q, want %q", got.OfficialBaseURL, DefaultClawHubOfficialBaseURL)
	}
}

func TestClawHubConfigResolvedDisablesOfficialWhenSetEmpty(t *testing.T) {
	t.Parallel()

	got := (ClawHubConfig{OfficialBaseURLSet: true}).Resolved()
	if got.OfficialBaseURL != "" {
		t.Fatalf("OfficialBaseURL = %q, want empty", got.OfficialBaseURL)
	}
}

func TestLoadReadsClawHubConfig(t *testing.T) {
	t.Setenv("CLAWHUB_TOKEN", "clh-test")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[clawhub]
base_url = "https://claw.example.com"
token = "${CLAWHUB_TOKEN}"
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
	if got, want := cfg.ClawHub.BaseURL, "https://claw.example.com"; got != want {
		t.Fatalf("ClawHub.BaseURL = %q, want %q", got, want)
	}
	if got, want := cfg.ClawHub.Token, "clh-test"; got != want {
		t.Fatalf("ClawHub.Token = %q, want %q", got, want)
	}
	if cfg.ClawHub.NonSuspiciousOnly {
		t.Fatal("ClawHub.NonSuspiciousOnly = true, want false")
	}
}
