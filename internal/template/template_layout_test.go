package template

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	agentruntime "csgclaw/internal/runtime"
)

func TestLocalStoreCodexTemplateDoesNotRoundTripWorkspaceMemory(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "AGENTS.md"), []byte("# Instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "MEMORY.md"), []byte("# Pinned memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	storeRoot := t.TempDir()
	store := NewLocalStore(storeRoot)
	created, err := store.Publish(context.Background(), PublishSpec{
		ID:          "codex-with-workspace-memory",
		Name:        "codex-with-workspace-memory",
		Role:        TemplateRoleWorker,
		RuntimeKind: agentruntime.KindCodex,
		WorkspaceRef: WorkspaceRef{
			Kind: WorkspaceKindDir,
			Path: workspaceRoot,
		},
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.templateRoot(created.ID), localMemoriesDirName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("published Codex template contains memories directory: %v", err)
	}

	workspace, err := store.FetchWorkspace(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("FetchWorkspace() error = %v", err)
	}
	defer os.RemoveAll(workspace.Path)
	if _, err := os.Stat(filepath.Join(workspace.Path, "MEMORY.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("materialized Codex template contains MEMORY.md: %v", err)
	}
}

func TestWriteTemplateLayoutKeepsWorkspaceMemoryOutOfCodexTemplates(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "AGENTS.md"), []byte("# Instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "MEMORY.md"), []byte("# Pinned memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "memory", "2026-08-25.md"), []byte("dated memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name        string
		runtimeKind string
		wantMemory  bool
	}{
		{name: "codex", runtimeKind: agentruntime.KindCodex, wantMemory: false},
		{name: "openclaw", runtimeKind: agentruntime.NameOpenClaw, wantMemory: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			templateRoot := t.TempDir()
			if err := writeTemplateLayout(WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot}, templateRoot, tt.runtimeKind, nil); err != nil {
				t.Fatalf("writeTemplateLayout() error = %v", err)
			}
			for _, path := range []string{
				filepath.Join(templateRoot, localMemoriesDirName, "MEMORY.md"),
				filepath.Join(templateRoot, localMemoriesDirName, "2026-08-25.md"),
			} {
				_, err := os.Stat(path)
				if tt.wantMemory && err != nil {
					t.Fatalf("template memory %q missing: %v", path, err)
				}
				if !tt.wantMemory && !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("Codex template unexpectedly contains workspace memory %q: %v", path, err)
				}
			}
		})
	}
}

func TestMaterializeTemplateFSSkipsMemoryForCodex(t *testing.T) {
	templateRoot := t.TempDir()
	for path, content := range map[string]string{
		filepath.Join(templateRoot, localInstructionsDirName, requiredInstructionsFile): "# Instructions\n",
		filepath.Join(templateRoot, localMemoriesDirName, "MEMORY.md"):                  "# Pinned memory\n",
		filepath.Join(templateRoot, localMCPsDirName, localMCPFileName):                 "{}\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, tt := range []struct {
		name        string
		runtimeKind string
		wantMemory  bool
	}{
		{name: "codex", runtimeKind: agentruntime.KindCodex, wantMemory: false},
		{name: "picoclaw", runtimeKind: agentruntime.NamePicoClaw, wantMemory: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workspace, err := materializeTemplateFS(os.DirFS(templateRoot), ".", tt.runtimeKind)
			if err != nil {
				t.Fatalf("materializeTemplateFS() error = %v", err)
			}
			defer os.RemoveAll(workspace.Path)
			_, err = os.Stat(filepath.Join(workspace.Path, "MEMORY.md"))
			if tt.wantMemory && err != nil {
				t.Fatalf("materialized memory missing: %v", err)
			}
			if !tt.wantMemory && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("Codex workspace unexpectedly contains template memory: %v", err)
			}
		})
	}
}
