package template

import (
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/runtime"
)

func TestLoadManifestImageEnv(t *testing.T) {
	registryRoot := t.TempDir()
	manifestPath := filepath.Join(registryRoot, "templates", "gitlab-assistant", "agent.toml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	manifest := `
name = "gitlab-assistant"
description = "GitLab assistant"
role = "worker"
runtime_kind = "picoclaw"

[image]
ref = "picoclaw:test"

[[image.env]]
name = "GITLAB_TOKEN"
required = true
secret = true

[[image.env]]
name = "GITLAB_URL"
default = "https://gitlab.example.com"
`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := NewLocalStore(registryRoot)
	got, err := store.Get(t.Context(), "gitlab-assistant")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.RuntimeKind != runtime.NamePicoClaw {
		t.Fatalf("RuntimeKind = %q, want %q", got.RuntimeKind, runtime.NamePicoClaw)
	}
	if got.Image != "picoclaw:test" {
		t.Fatalf("Image = %q, want picoclaw:test", got.Image)
	}
	if len(got.ImageEnv) != 2 {
		t.Fatalf("ImageEnv = %#v, want 2 entries", got.ImageEnv)
	}
	if got.ImageEnv[0].Name != "GITLAB_TOKEN" || !got.ImageEnv[0].Required || !got.ImageEnv[0].Secret {
		t.Fatalf("ImageEnv[0] = %#v, want GITLAB_TOKEN required secret", got.ImageEnv[0])
	}
	if got.ImageEnv[1].Name != "GITLAB_URL" || got.ImageEnv[1].Default != "https://gitlab.example.com" {
		t.Fatalf("ImageEnv[1] = %#v, want GITLAB_URL default url", got.ImageEnv[1])
	}
}

func TestValidateImageEnvRejectsSecretDefault(t *testing.T) {
	err := validateImageEnvContracts([]templateImageEnvItem{
		{Name: "API_KEY", Secret: true, Default: "secret"},
	})
	if err == nil {
		t.Fatal("validateImageEnvContracts() error = nil, want secret default rejection")
	}
}

func TestValidatePublishTemplateName(t *testing.T) {
	for _, name := range []string{"ReviewBot", "review_bot_2", "review-bot", "A1", "a23456789012345678901234"} {
		if err := ValidatePublishTemplateName(name); err != nil {
			t.Errorf("ValidatePublishTemplateName(%q) error = %v", name, err)
		}
	}
	for _, name := range []string{"", "2review", "中文模板", "review bot", "a234567890123456789012345"} {
		if err := ValidatePublishTemplateName(name); err == nil {
			t.Errorf("ValidatePublishTemplateName(%q) error = nil, want rejection", name)
		}
	}
}

func TestNormalizeTemplateRuntimeOptions(t *testing.T) {
	got, err := normalizeTemplateRuntimeOptions(runtime.KindCodex, TemplateRoleWorker, map[string]any{
		"execution_mode": " READ_ONLY ",
	})
	if err != nil {
		t.Fatalf("normalizeTemplateRuntimeOptions() error = %v", err)
	}
	if got["execution_mode"] != "read_only" {
		t.Fatalf("execution_mode = %v, want read_only", got["execution_mode"])
	}

	for name, test := range map[string]struct {
		runtimeKind string
		role        string
		options     map[string]any
	}{
		"manager":        {runtimeKind: runtime.KindCodex, role: TemplateRoleManager, options: map[string]any{"execution_mode": "read_only"}},
		"non codex":      {runtimeKind: runtime.NameOpenClaw, role: TemplateRoleWorker, options: map[string]any{"execution_mode": "read_only"}},
		"unknown option": {runtimeKind: runtime.KindCodex, role: TemplateRoleWorker, options: map[string]any{"local_workspace_dir": "/tmp/project"}},
		"invalid mode":   {runtimeKind: runtime.KindCodex, role: TemplateRoleWorker, options: map[string]any{"execution_mode": "unsafe"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeTemplateRuntimeOptions(test.runtimeKind, test.role, test.options); err == nil {
				t.Fatal("normalizeTemplateRuntimeOptions() error = nil, want rejection")
			}
		})
	}
}
