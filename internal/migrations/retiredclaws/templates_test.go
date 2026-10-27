package retiredclaws_test

import (
	"context"
	"csgclaw/internal/migrations/retiredclaws"
	"csgclaw/internal/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/config"
)

func TestLocalRetiredRuntimeTemplatesMigrateBeforeBootstrapResolution(t *testing.T) {
	for _, kind := range []string{"openclaw", "picoclaw_sandbox"} {
		t.Run(kind, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "registry")
			templateRoot := filepath.Join(root, "templates", "reviewer")
			if err := os.MkdirAll(filepath.Join(templateRoot, "instructions"), 0755); err != nil {
				t.Fatal(err)
			}
			original := "name = \"reviewer\"\nruntime_kind = \"" + kind + "\"\nversion = \"1.0.0\"\n[image]\nref = \"retired-image\"\n[[image.env]]\nname = \"USER_TOKEN\"\nsecret = true\n[runtime_options]\nnative_option = \"old\"\n"
			if err := os.MkdirAll(filepath.Join(templateRoot, "mcps"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(templateRoot, "mcps", "mcp.json"), []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
			manifestPath := filepath.Join(templateRoot, "agent.toml")
			if err := os.WriteFile(manifestPath, []byte(original), 0640); err != nil {
				t.Fatal(err)
			}
			instruction := "AGENTS.md"
			if strings.HasPrefix(kind, "picoclaw") {
				instruction = "AGENT.md"
			}
			if err := os.WriteFile(filepath.Join(templateRoot, "instructions", instruction), []byte("# Reviewer"), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := config.HubConfig{Registries: []config.HubRegistryConfig{{Name: "local", Kind: template.RegistryKindLocal, Path: root, Enabled: true}}}
			service, err := template.NewService(cfg, template.DefaultStoreFactory)
			if err != nil {
				t.Fatal(err)
			}
			defaults, err := template.ResolveBootstrapDefaults(context.Background(), config.BootstrapConfig{DefaultWorkerTemplate: "local.reviewer"}, service)
			if err != nil {
				t.Fatal(err)
			}
			if defaults.WorkerRuntimeKind != "codex" || defaults.WorkerImage != "" {
				t.Fatalf("default runtime: %+v", defaults)
			}
			item, err := service.Get(context.Background(), "local.reviewer")
			if err != nil {
				t.Fatal(err)
			}
			if len(item.ImageEnv) != 1 || item.ImageEnv[0].Name != "USER_TOKEN" || len(item.RuntimeOptions) != 0 {
				t.Fatalf("template credentials/options: %+v", item)
			}
			workspace, err := service.FetchWorkspace(context.Background(), "local.reviewer")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(workspace.Path) })
			if data, err := os.ReadFile(filepath.Join(workspace.Path, "AGENTS.md")); err != nil || string(data) != "# Reviewer" {
				t.Fatal("instructions not preserved")
			}
			info, err := os.Stat(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0640 {
				t.Fatalf("manifest mode: %o", info.Mode().Perm())
			}
			backups, err := filepath.Glob(root + "_backup_*")
			if err != nil || len(backups) != 1 {
				t.Fatalf("backups: %v %v", backups, err)
			}
			data, err := os.ReadFile(filepath.Join(backups[0], "templates", "reviewer", "agent.toml"))
			if err != nil || string(data) != original {
				t.Fatal("backup does not contain original template")
			}
			if _, err := template.NewService(cfg, template.DefaultStoreFactory); err != nil {
				t.Fatal(err)
			}
			again, err := os.Stat(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			if !info.ModTime().Equal(again.ModTime()) {
				t.Fatal("template migration repeats")
			}
			if _, err := template.NewLocalStore(root).Publish(context.Background(), template.PublishSpec{Name: "new-retired", RuntimeKind: kind, Image: "old"}); err == nil {
				t.Fatal("new publication accepted retired runtime")
			}
		})
	}
}

func TestLocalRuntimeMigrationLeavesActiveAndUnrelatedTemplatesUntouched(t *testing.T) {
	root := t.TempDir()
	for _, kind := range []string{"codex", "dsh", "unsupported"} {
		path := filepath.Join(root, "templates", kind, "agent.toml")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		data := []byte("# preserve this formatting\nname = \"reviewer\"\nruntime_kind = \"" + kind + "\"\n")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
		if err := retiredclaws.LocalTemplates(root); err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(data) {
			t.Fatalf("unrelated template changed: %s", kind)
		}
	}
}

func TestReadOnlyRetiredTemplateRegistryRemainsUsable(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "registry")
	templateRoot := filepath.Join(root, "templates", "reviewer")
	if err := os.MkdirAll(filepath.Join(templateRoot, "instructions"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(templateRoot, "mcps"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(templateRoot, "agent.toml")
	for path, data := range map[string]string{manifest: "name=\"reviewer\"\nruntime_kind=\"picoclaw\"\n", filepath.Join(templateRoot, "instructions", "AGENT.md"): "# Preserved", filepath.Join(templateRoot, "mcps", "mcp.json"): "{}"} {
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(parent, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0755) })
	probe := filepath.Join(parent, "probe")
	if err := os.WriteFile(probe, nil, 0600); err == nil {
		os.Remove(probe)
		t.Skip("filesystem does not enforce directory permissions")
	}
	store, err := template.DefaultStoreFactory(config.HubRegistryConfig{Kind: template.RegistryKindLocal, Path: root})
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.Get(context.Background(), "reviewer")
	if err != nil || item.RuntimeKind != "codex" {
		t.Fatalf("read-only template: %+v %v", item, err)
	}
	workspace, err := store.FetchWorkspace(context.Background(), "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace.Path)
	if data, err := os.ReadFile(filepath.Join(workspace.Path, "AGENTS.md")); err != nil || string(data) != "# Preserved" {
		t.Fatal("read-only template instructions lost")
	}
}
