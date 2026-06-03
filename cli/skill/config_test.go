package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/cli/command"
	"csgclaw/internal/config"
)

func TestResolveSkillConfigUsesDefaultsWhenConfigMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := resolveSkillConfig(command.GlobalOptions{})
	if err != nil {
		t.Fatalf("resolveSkillConfig() error = %v", err)
	}
	want := config.SkillConfig{NonSuspiciousOnly: true}.Resolved()
	if got.BaseURL != want.BaseURL || got.OfficialBaseURL != want.OfficialBaseURL {
		t.Fatalf("config = %#v, want defaults %#v", got, want)
	}
}

func TestResolveSkillConfigReturnsErrorForExplicitMissingConfig(t *testing.T) {
	t.Parallel()

	_, err := resolveSkillConfig(command.GlobalOptions{Config: filepath.Join(t.TempDir(), "missing.toml")})
	if err == nil || !strings.Contains(err.Error(), "load config") {
		t.Fatalf("resolveSkillConfig() error = %v, want load config error", err)
	}
}

func TestResolveSkillConfigReturnsErrorForInvalidConfigFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("not valid toml [[[\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := resolveSkillConfig(command.GlobalOptions{Config: path})
	if err == nil || !strings.Contains(err.Error(), "load config") {
		t.Fatalf("resolveSkillConfig() error = %v, want load config error", err)
	}
}

func TestResolveSkillConfigLoadsExplicitConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"

[skill]
base_url = "https://claw.example.com"
official_base_url = ""

[bootstrap]
default_manager_template = "builtin.picoclaw-manager"
default_worker_template = "builtin.picoclaw-worker"

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

	got, err := resolveSkillConfig(command.GlobalOptions{Config: path})
	if err != nil {
		t.Fatalf("resolveSkillConfig() error = %v", err)
	}
	if got.BaseURL != "https://claw.example.com" {
		t.Fatalf("BaseURL = %q, want https://claw.example.com", got.BaseURL)
	}
	if got.OfficialBaseURL != "" {
		t.Fatalf("OfficialBaseURL = %q, want empty", got.OfficialBaseURL)
	}
}
