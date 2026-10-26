package retiredclaws_test

import (
	"csgclaw/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMigratesRetiredRuntimeConfig(t *testing.T) {
	for _, template := range []string{
		"builtin.openclaw-manager", "builtin.openclaw-worker",
		"builtin.picoclaw-manager", "builtin.picoclaw-worker",
		"builtin/openclaw-manager", "builtin/openclaw-worker",
		"builtin/picoclaw-manager", "builtin/picoclaw-worker",
	} {
		t.Run(template, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			original := "[bootstrap]\ndefault_manager_template = \"" + template + "\"\ndefault_worker_template = \"" + template + "\"\n"
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if !cfg.NeedsMigrationRewrite() {
				t.Fatal("retired template did not request a rewrite")
			}
			if cfg.Bootstrap.DefaultManagerTemplate != "" || cfg.Bootstrap.DefaultWorkerTemplate != "" {
				t.Fatalf("retired overrides remain: %+v", cfg.Bootstrap)
			}
			if cfg.Bootstrap.ResolvedDefaultManagerTemplate() != config.DefaultBootstrapManagerTemplate || cfg.Bootstrap.ResolvedDefaultWorkerTemplate() != config.DefaultBootstrapWorkerTemplate {
				t.Fatalf("wrong defaults: %+v", cfg.Bootstrap)
			}
			before, err := os.ReadFile(path)
			if err != nil || string(before) != original {
				t.Fatalf("config.Load modified config: %v", err)
			}
			if err := cfg.Save(path); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(after), "openclaw") || strings.Contains(string(after), "picoclaw") {
				t.Fatalf("retired reference persisted: %s", after)
			}
			cfg, err = config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.NeedsMigrationRewrite() {
				t.Fatal("migration repeats after save")
			}
		})
	}
}

func TestLoadDropsObsoleteBootstrapKeys(t *testing.T) {
	for _, key := range []string{"runtime_kind", "manager_image", "manager_image_override"} {
		t.Run(key, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte("[bootstrap]\n"+key+" = \"retired-value\"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if !cfg.NeedsMigrationRewrite() {
				t.Fatal("obsolete key did not request rewrite")
			}
			if err := cfg.Save(path); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), key+" = ") {
				t.Fatalf("obsolete key retained: %s", data)
			}
			cfg, err = config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.NeedsMigrationRewrite() {
				t.Fatal("migration repeats after save")
			}
		})
	}
}

func TestRuntimeMigrationPreservesSupportedAndCustomTemplates(t *testing.T) {
	for _, ref := range []string{"builtin.manager-codex", "builtin.codex-worker", "builtin.dsh-worker", "local.openclaw-worker", "team.picoclaw-manager", "team/custom"} {
		t.Run(ref, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte("[bootstrap]\ndefault_manager_template = \""+ref+"\"\ndefault_worker_template = \""+ref+"\"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.NeedsMigrationRewrite() && !strings.Contains(ref, "/") {
				t.Fatal("custom or supported template marked retired")
			}
			want := strings.Replace(ref, "/", ".", 1)
			if cfg.Bootstrap.DefaultWorkerTemplate != want || cfg.Bootstrap.DefaultManagerTemplate != want {
				t.Fatalf("template changed: %+v", cfg.Bootstrap)
			}
		})
	}
}

func TestRuntimeMigrationDoesNotRestoreRetiredEnvironmentReference(t *testing.T) {
	t.Setenv("RETIRED_TEMPLATE", "builtin.openclaw-worker")
	t.Setenv("ACTIVE_TEMPLATE", "builtin.dsh-worker")
	t.Setenv("MIGRATION_TOKEN", "fixture-token")
	path := filepath.Join(t.TempDir(), "config.toml")
	original := `[server]
listen_addr = "127.0.0.1:19081"
access_token = "${MIGRATION_TOKEN}"
[bootstrap]
default_manager_template = "${ACTIVE_TEMPLATE}"
default_worker_template = "${RETIRED_TEMPLATE}"
[sandbox]
provider = "docker"
storage_path = "/custom/storage"
`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"${ACTIVE_TEMPLATE}", "${MIGRATION_TOKEN}", "127.0.0.1:19081", "/custom/storage", config.DefaultBootstrapWorkerTemplate} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("saved config lost %q", want)
		}
	}
	if strings.Contains(string(data), "${RETIRED_TEMPLATE}") {
		t.Fatal("retired environment reference restored")
	}
}
