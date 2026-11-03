package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetiredSandboxSettingsArePrivateRetryInput(t *testing.T) {
	for _, provider := range []string{"docker", "boxlite", "boxlite-cli", "csghub", "unknown-old-provider"} {
		t.Run(provider, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			original := "[server]\nlisten_addr = \"127.0.0.1:18080\"\naccess_token = \"${RETIRED_CONFIG_TEST_TOKEN}\"\n[sandbox]\nprovider = \"" + provider + "\"\nstorage_path = \"/retained/workspace\"\ndocker_cli_path = \"/retained/docker\"\ndebian_registries_override = [\"registry.example\"]\n"
			t.Setenv("RETIRED_CONFIG_TEST_TOKEN", "fixture-only")
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("retired setting blocked startup: %v", err)
			}
			if !cfg.NeedsMigrationRewrite() {
				t.Fatal("missing rewrite")
			}
			if err := cfg.Save(path); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(path)
			if strings.Contains(string(data), "\n[sandbox]\n") || !strings.Contains(string(data), "[migrations.retired_claws.sandbox]") {
				t.Fatalf("retry input not isolated: %s", data)
			}
			cfg, err = Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Sandbox.Provider != provider || cfg.Sandbox.StoragePath != "/retained/workspace" || cfg.Sandbox.DockerCLIPath != "/retained/docker" {
				t.Fatalf("retry input lost: %+v", cfg.Sandbox)
			}
			if cfg.NeedsMigrationRewrite() {
				t.Fatal("private retry input repeatedly requests rewrite")
			}
			if cfg.IsCSGHubDeployment() != (provider == "csghub") {
				t.Fatal("deployment mode changed")
			}
			if !cfg.ClearRetiredSandboxConfig() {
				t.Fatal("pending retry input not found")
			}
			if err := cfg.Save(path); err != nil {
				t.Fatal(err)
			}
			data, _ = os.ReadFile(path)
			if strings.Contains(string(data), "[sandbox]") || strings.Contains(string(data), "migrations.retired_claws") {
				t.Fatalf("completed migration left retry input: %s", data)
			}
			if !strings.Contains(string(data), "${RETIRED_CONFIG_TEST_TOKEN}") {
				t.Fatal("unrelated environment reference changed")
			}
			cfg, err = Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.IsCSGHubDeployment() != (provider == "csghub") {
				t.Fatal("deployment mode lost after restart")
			}
			if cfg.NeedsMigrationRewrite() {
				t.Fatal("completed migration repeats")
			}
		})
	}
}

func TestMalformedRetiredRegistrySettingCannotBlockHostStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[sandbox]\nprovider=\"old\"\ndebian_registries_override=\"unused\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
}
