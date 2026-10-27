package template

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	agentruntime "csgclaw/internal/runtime"
)

var skillActivationArtifactPaths = []string{
	".builtin-pdf.json",
	".builtin-docx.json.cancelled",
	".builtin-journal-interrupted",
	".builtin-staging/.builtin-stage-current/SKILL.md",
	".builtin-staging/.builtin-stage-old.previous/SKILL.md",
	".builtin-stage-legacy.previous/SKILL.md",
}

func TestLocalStorePublishOmitsSkillInstallLock(t *testing.T) {
	for _, runtimeKind := range []string{agentruntime.KindCodex, agentruntime.KindDSH, agentruntime.NameDSH} {
		for _, separateSkills := range []bool{false, true} {
			name := runtimeKind + "/workspace-skills"
			if separateSkills {
				name = runtimeKind + "/separate-skills"
			}
			t.Run(name, func(t *testing.T) {
				workspaceRoot := t.TempDir()
				skillsRoot := filepath.Join(workspaceRoot, "skills")
				workspace := WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot}
				if separateSkills {
					skillsRoot = t.TempDir()
					workspace.SkillsPath = skillsRoot
				}
				files := map[string]string{
					".skillhub-lock.json.lock": "",
					".skillhub-lock.json":      `{"skills":{"custom":{"source":"market","slug":"custom","version":"1.0.0"}}}`,
					"custom/SKILL.md":          "# Custom skill\n",
					"custom/package-lock.json": "{}\n",
					"custom/data.lock":         "skill resource\n",
					"custom/empty.txt":         "",
				}
				// The same names inside a skill are resources, not local transactions.
				files["custom/.builtin-pdf.json"] = "skill resource"
				files["custom/.builtin-staging/data.txt"] = "skill resource"
				omitted := map[string]bool{".skillhub-lock.json.lock": true}
				for _, path := range skillActivationArtifactPaths {
					files[path] = "local transaction must not be exported"
					omitted[path] = true
				}
				for path, content := range files {
					source := filepath.Join(skillsRoot, filepath.FromSlash(path))
					if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(source, []byte(content), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				store := NewLocalStore(t.TempDir())
				created, err := store.Publish(context.Background(), PublishSpec{
					Name: "custom", RuntimeKind: runtimeKind, Image: "worker:test", WorkspaceRef: workspace,
				})
				if err != nil {
					t.Fatalf("Publish() error = %v", err)
				}
				restored, err := store.FetchWorkspace(context.Background(), created.ID)
				if err != nil {
					t.Fatalf("FetchWorkspace() error = %v", err)
				}
				defer os.RemoveAll(restored.Path)
				for path, want := range files {
					source := filepath.Join(skillsRoot, filepath.FromSlash(path))
					if data, err := os.ReadFile(source); err != nil || string(data) != want {
						t.Errorf("source %s = %q, error = %v, want %q", path, data, err, want)
					}
					for _, root := range []string{store.templateRoot(created.ID), restored.Path} {
						target := filepath.Join(root, "skills", filepath.FromSlash(path))
						if omitted[path] {
							if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
								t.Errorf("template contains local skill state %s: %v", target, err)
							}
						} else if data, err := os.ReadFile(target); err != nil || string(data) != want {
							t.Errorf("template file %s = %q, error = %v, want %q", target, data, err, want)
						}
					}
				}
			})
		}
	}
}

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
		{name: "dsh default off", runtimeKind: agentruntime.NameDSH, wantMemory: false},
		{name: "dsh explicit opt in", runtimeKind: agentruntime.NameDSH, includeMemory: true, wantMemory: false},
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
	if err := writeTemplateLayout(WorkspaceRef{Kind: WorkspaceKindDir, Path: workspaceRoot}, templateRoot, agentruntime.NameDSH, nil, false); err != nil {
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
		{name: "dsh", runtimeKind: agentruntime.KindDSH, wantMemory: false},
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

	dshTarget := t.TempDir()
	if err := copyTemplateSkills(source, dshTarget, agentruntime.NameDSH); err != nil {
		t.Fatalf("copyTemplateSkills(dsh) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dshTarget, ".system", "builtin", "SKILL.md")); err != nil {
		t.Fatalf("DSH top-level .system missing: %v", err)
	}
}
