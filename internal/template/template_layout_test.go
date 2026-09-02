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
	if err := os.WriteFile(filepath.Join(workspaceRoot, "memory.md"), []byte("# Pinned memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "Memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "Memory", "2026-08-25.md"), []byte("dated memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stagingDir := filepath.Join(workspaceRoot, ".CSGClaw-Template-Memory")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "memory_summary.md"), []byte("staged memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name          string
		runtimeKind   string
		includeMemory bool
		wantMemory    bool
	}{
		{name: "codex", runtimeKind: agentruntime.KindCodex, includeMemory: true, wantMemory: false},
		{name: "openclaw default off", runtimeKind: agentruntime.NameOpenClaw, wantMemory: false},
		{name: "openclaw explicit opt in", runtimeKind: agentruntime.NameOpenClaw, includeMemory: true, wantMemory: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			templateRoot := t.TempDir()
			if err := writeTemplateLayout(WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot}, templateRoot, tt.runtimeKind, nil, tt.includeMemory); err != nil {
				t.Fatalf("writeTemplateLayout() error = %v", err)
			}
			for _, path := range []string{
				filepath.Join(templateRoot, localMemoriesDirName, "MEMORY.md"),
				filepath.Join(templateRoot, localMemoriesDirName, "memory", "2026-08-25.md"),
			} {
				_, err := os.Stat(path)
				if tt.wantMemory && err != nil {
					t.Fatalf("template memory %q missing: %v", path, err)
				}
				if !tt.wantMemory && !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("Codex template unexpectedly contains workspace memory %q: %v", path, err)
				}
			}
			if _, err := os.Stat(filepath.Join(templateRoot, localInstructionsDirName, ".CSGClaw-Template-Memory")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("reserved memory staging directory leaked into instructions: %v", err)
			}
		})
	}
}

func TestWriteTemplateLayoutPublishesOnlyAllowedWorkspaceFiles(t *testing.T) {
	workspaceRoot := t.TempDir()
	for path, content := range map[string]string{
		"AGENT.md":            "# Legacy instructions\n",
		"HEARTBEAT.md":        "heartbeat\n",
		"IDENTITY.md":         "identity\n",
		"SOUL.md":             "soul\n",
		"TOOLS.md":            "tools\n",
		"USER.md":             "user\n",
		"PLAYBOOK.md":         "not an instruction file\n",
		"heartbeat.log":       "runtime log\n",
		"downloads/input.pdf": "downloaded data\n",
		"sessions/chat.jsonl": "conversation\n",
		"state/state.json":    "runtime state\n",
		".csgclaw/input":      "runtime data\n",
	} {
		path = filepath.Join(workspaceRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	templateRoot := t.TempDir()
	if err := writeTemplateLayout(WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot}, templateRoot, agentruntime.NameOpenClaw, nil, false); err != nil {
		t.Fatalf("writeTemplateLayout() error = %v", err)
	}
	instructionsRoot := filepath.Join(templateRoot, localInstructionsDirName)
	for _, name := range []string{"AGENTS.md", "HEARTBEAT.md", "IDENTITY.md", "SOUL.md", "TOOLS.md", "USER.md"} {
		if _, err := os.Stat(filepath.Join(instructionsRoot, name)); err != nil {
			t.Errorf("allowed instruction %q missing: %v", name, err)
		}
	}
	for _, name := range []string{"AGENT.md", "PLAYBOOK.md", "heartbeat.log", "downloads", "sessions", "state", ".csgclaw"} {
		if _, err := os.Stat(filepath.Join(instructionsRoot, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("non-template workspace entry %q was published: %v", name, err)
		}
	}
}

func TestOpenClawTemplateMemoryRoundTripPreservesWorkspaceLayout(t *testing.T) {
	workspaceRoot := t.TempDir()
	for path, content := range map[string]string{
		"AGENTS.md":            "# Instructions\n",
		"MEMORY.md":            "# Durable memory\n",
		"memory/2026-08-28.md": "dated memory\n",
	} {
		path = filepath.Join(workspaceRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	store := NewLocalStore(t.TempDir())
	created, err := store.Publish(context.Background(), PublishSpec{
		ID: "openclaw-memory", Name: "openclaw-memory", RuntimeKind: agentruntime.NameOpenClaw,
		Image: "openclaw:test", IncludeMemory: true,
		WorkspaceRef: WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot},
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	workspace, err := store.FetchWorkspace(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("FetchWorkspace() error = %v", err)
	}
	defer os.RemoveAll(workspace.Path)
	for path, want := range map[string]string{
		"MEMORY.md":            "# Durable memory\n",
		"memory/2026-08-28.md": "dated memory\n",
	} {
		data, readErr := os.ReadFile(filepath.Join(workspace.Path, filepath.FromSlash(path)))
		if readErr != nil || string(data) != want {
			t.Fatalf("restored %s = %q, error = %v, want %q", path, data, readErr, want)
		}
	}
}

func TestMaterializeWorkspaceMemorySupportsLegacyFlattenedEntries(t *testing.T) {
	templateRoot := t.TempDir()
	for path, content := range map[string]string{
		"instructions/AGENTS.md": "# Instructions\n",
		"memories/MEMORY.md":     "# Durable memory\n",
		"memories/2026-08-27.md": "legacy dated memory\n",
		"mcps/mcp.json":          "{}\n",
	} {
		path = filepath.Join(templateRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	workspace, err := materializeTemplateFS(os.DirFS(templateRoot), ".", agentruntime.NameOpenClaw)
	if err != nil {
		t.Fatalf("materializeTemplateFS() error = %v", err)
	}
	defer os.RemoveAll(workspace.Path)
	if data, readErr := os.ReadFile(filepath.Join(workspace.Path, "memory", "2026-08-27.md")); readErr != nil || string(data) != "legacy dated memory\n" {
		t.Fatalf("legacy dated memory = %q, error = %v", data, readErr)
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

func TestCopyTemplateSkillsSkipsOnlyTopLevelCodexSystemSkills(t *testing.T) {
	source := t.TempDir()
	for path, content := range map[string]string{
		".system/builtin/SKILL.md":       "builtin\n",
		"custom/SKILL.md":                "custom\n",
		"custom/references/.system/data": "nested\n",
	} {
		path = filepath.Join(source, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	codexTarget := t.TempDir()
	if err := copyTemplateSkills(source, codexTarget, agentruntime.KindCodex); err != nil {
		t.Fatalf("copyTemplateSkills(codex) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(codexTarget, ".system")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("top-level Codex .system stat error = %v, want not exist", err)
	}
	for _, path := range []string{"custom/SKILL.md", "custom/references/.system/data"} {
		if _, err := os.Stat(filepath.Join(codexTarget, filepath.FromSlash(path))); err != nil {
			t.Fatalf("Codex custom skill path %q missing: %v", path, err)
		}
	}

	openClawTarget := t.TempDir()
	if err := copyTemplateSkills(source, openClawTarget, agentruntime.NameOpenClaw); err != nil {
		t.Fatalf("copyTemplateSkills(openclaw) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(openClawTarget, ".system", "builtin", "SKILL.md")); err != nil {
		t.Fatalf("OpenClaw top-level .system missing: %v", err)
	}
}
