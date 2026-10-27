package agents

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/config"
	"csgclaw/internal/resourcequery"
	skillinstall "csgclaw/internal/skill/installstate"
	skilllocal "csgclaw/internal/skill/local"
	skillsystem "csgclaw/internal/skill/system"
	templateembed "csgclaw/internal/template/embed"
)

func TestWorkspaceSkillSummariesRecognizeEmbeddedSources(t *testing.T) {
	for _, kind := range []string{RuntimeKindCodex, RuntimeKindDSH, RuntimeKindDSH} {
		t.Run(kind, func(t *testing.T) {
			workspace, agent, root := newSkillSourceWorkspace(t, kind)
			templateRoot, err := resolveRuntimeTemplateRoot(kind, RoleWorker)
			if err != nil {
				t.Fatal(err)
			}
			copySkillSourceFixture(t, root, "agent-teams", skillsystem.SkillSource{FS: templateembed.FS(), RootPath: path.Join(templateRoot, "skills", "agent-teams")})
			copySkillSourceFixture(t, root, "csgclaw-self-diagnosis", systemSkillSourceFixture(t, "csgclaw-self-diagnosis"))
			writeWorkspaceFileAt(t, root, "custom/SKILL.md", "# Custom skill\n", 0600)
			before, err := skillinstall.BundleSHA256(filepath.Join(root, "agent-teams"))
			if err != nil {
				t.Fatal(err)
			}
			assertSkillSourceSummaries(t, workspace, agent.ID, map[string]string{
				"agent-teams": skillinstall.SourceBuiltin, "csgclaw-self-diagnosis": skillinstall.SourceBuiltin, "custom": skillinstall.SourceUnknown,
			})
			after, err := skillinstall.BundleSHA256(filepath.Join(root, "agent-teams"))
			if err != nil || before != after {
				t.Fatalf("summary changed template files: before=%s after=%s error=%v", before, after, err)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if skillinstall.IsRecordsFile(entry.Name()) {
					t.Fatal("summary created installation records")
				}
			}
		})
	}
}

func TestWorkspaceSkillSummariesPreserveRecordedSources(t *testing.T) {
	for _, source := range []string{skillinstall.SourceMarket, skillinstall.SourceLocal, skillinstall.SourceBuiltin} {
		t.Run(source, func(t *testing.T) {
			workspace, agent, root := newSkillSourceWorkspace(t, RuntimeKindCodex)
			copySkillSourceFixture(t, root, "csgclaw-self-diagnosis", systemSkillSourceFixture(t, "csgclaw-self-diagnosis"))
			if err := skillinstall.Update(root, "csgclaw-self-diagnosis", &skillinstall.Record{Source: source}, nil); err != nil {
				t.Fatal(err)
			}
			if source == skillinstall.SourceBuiltin {
				writeWorkspaceFileAt(t, root, "csgclaw-self-diagnosis/SKILL.md", "# Edited builtin\n", 0600)
			}
			before, err := os.ReadFile(filepath.Join(root, ".skillhub-lock.json"))
			if err != nil {
				t.Fatal(err)
			}
			assertSkillSourceSummaries(t, workspace, agent.ID, map[string]string{"csgclaw-self-diagnosis": source})
			after, err := os.ReadFile(filepath.Join(root, ".skillhub-lock.json"))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("summary changed installation records: %v", err)
			}
		})
	}
}

func TestWorkspaceSkillSummariesRequireCompleteBuiltinContent(t *testing.T) {
	for _, change := range []string{"content", "extra", "missing", "symlink", "nonregular", "unreadable"} {
		t.Run(change, func(t *testing.T) {
			workspace, agent, root := newSkillSourceWorkspace(t, RuntimeKindDSH)
			name := "csgclaw-self-diagnosis"
			copySkillSourceFixture(t, root, name, systemSkillSourceFixture(t, name))
			auxiliary := filepath.Join(root, name, "agents", "openai.yaml")
			switch change {
			case "content":
				data, err := os.ReadFile(auxiliary)
				if err != nil {
					t.Fatal(err)
				}
				data[0] ^= 1
				if err := os.WriteFile(auxiliary, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "extra":
				writeWorkspaceFileAt(t, root, name+"/extra.txt", "extra", 0600)
			case "missing", "symlink", "nonregular":
				data, err := os.ReadFile(auxiliary)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(auxiliary); err != nil {
					t.Fatal(err)
				}
				if change == "symlink" {
					target := filepath.Join(t.TempDir(), "openai.yaml")
					if err := os.WriteFile(target, data, 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(target, auxiliary); err != nil {
						t.Skipf("symlinks unavailable: %v", err)
					}
				}
				if change == "nonregular" {
					if err := os.Mkdir(auxiliary, 0700); err != nil {
						t.Fatal(err)
					}
				}
			case "unreadable":
				if err := os.Chmod(auxiliary, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(auxiliary, 0600) })
				if _, err := os.ReadFile(auxiliary); err == nil {
					t.Skip("current user can read files without permission bits")
				}
			}
			assertSkillSourceSummaries(t, workspace, agent.ID, map[string]string{name: skillinstall.SourceUnknown})
		})
	}
}

func TestWorkspaceSkillSummariesUseCurrentRuntimeTemplate(t *testing.T) {
	workspace, agent, root := newSkillSourceWorkspace(t, RuntimeKindDSH)
	copySkillSourceFixture(t, root, "agent-teams", skillsystem.SkillSource{
		FS: templateembed.FS(), RootPath: path.Join(templateembed.CodexWorkerRoot, "skills", "agent-teams"),
	})
	assertSkillSourceSummaries(t, workspace, agent.ID, map[string]string{"agent-teams": skillinstall.SourceUnknown})
}

func TestWorkspaceSkillSummariesHandleDamagedRecordsAndCancellation(t *testing.T) {
	workspace, agent, root := newSkillSourceWorkspace(t, RuntimeKindCodex)
	copySkillSourceFixture(t, root, "csgclaw-self-diagnosis", systemSkillSourceFixture(t, "csgclaw-self-diagnosis"))
	writeWorkspaceFileAt(t, root, ".skillhub-lock.json", "{broken", 0600)
	assertSkillSourceSummaries(t, workspace, agent.ID, map[string]string{"csgclaw-self-diagnosis": skillinstall.SourceBuiltin})
	data, err := os.ReadFile(filepath.Join(root, ".skillhub-lock.json"))
	if err != nil || string(data) != "{broken" {
		t.Fatalf("summary changed damaged installation records: %q, %v", data, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := workspace.SkillSummaries(ctx, agent.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("full summary cancellation = %v", err)
	}
	if _, _, err := workspace.SkillSummariesPage(ctx, agent.ID, resourcequery.Query{Mode: "page", Page: 1, Size: 1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("paged summary cancellation = %v", err)
	}
}

func TestBuiltinSkillMatchCancellationDuringRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	content := []byte("# Test\n")
	root := t.TempDir()
	writeWorkspaceFileAt(t, root, "SKILL.md", string(content), 0600)
	source := skillsystem.SkillSource{FS: cancelSkillReadFS{FS: fstest.MapFS{"SKILL.md": {Data: content}}, cancel: cancel}, RootPath: "."}
	if matched, err := matchesBuiltinSkill(ctx, root, source); matched || !errors.Is(err, context.Canceled) {
		t.Fatalf("match after cancellation = %v, %v", matched, err)
	}
}

type cancelSkillReadFS struct {
	fs.FS
	cancel context.CancelFunc
}

func (f cancelSkillReadFS) ReadFile(name string) ([]byte, error) {
	data, err := fs.ReadFile(f.FS, name)
	f.cancel()
	return data, err
}

func newSkillSourceWorkspace(t *testing.T, kind string) (*WorkspaceService, Agent, string) {
	t.Helper()
	controller, err := NewController(testModelConfig(), config.ServerConfig{}, "", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(fakeAgentRuntime{kind: kind}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = controller.Close() })
	agent := Agent{ID: "agent-source", Name: "source", Role: RoleWorker, RuntimeKind: kind}
	controller.agents[agent.ID] = agent
	layout, err := controller.AgentLayout(agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	return controller.Workspace(), agent, layout.SkillsRoot
}

func systemSkillSourceFixture(t *testing.T, name string) skillsystem.SkillSource {
	t.Helper()
	source, err := skillsystem.ResolveSource(name)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func copySkillSourceFixture(t *testing.T, root, name string, source skillsystem.SkillSource) {
	t.Helper()
	if err := copyWorkspaceFS(source.FS, source.RootPath, filepath.Join(root, name), "test skill", true); err != nil {
		t.Fatal(err)
	}
}

func assertSkillSourceSummaries(t *testing.T, workspace *WorkspaceService, agentID string, want map[string]string) {
	t.Helper()
	items, err := workspace.SkillSummaries(context.Background(), agentID)
	if err != nil {
		t.Fatal(err)
	}
	assertSources := func(items []contract.SkillSummary) {
		t.Helper()
		got := make(map[string]string)
		for _, item := range items {
			got[item.Name] = item.Source
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("skill sources = %v, want %v", got, want)
		}
	}
	assertSources(items)
	var paged []contract.SkillSummary
	for page := 1; page <= len(want); page++ {
		items, meta, err := workspace.SkillSummariesPage(context.Background(), agentID, resourcequery.Query{Mode: "page", Page: page, Size: 1})
		if err != nil || meta == nil || meta.Total != len(want) {
			t.Fatalf("paged summaries: metadata=%+v error=%v", meta, err)
		}
		paged = append(paged, items...)
	}
	assertSources(paged)
}

func TestWorkspaceSkillSourceUnavailableFilesRemainUnknown(t *testing.T) {
	root := t.TempDir()
	agent := Agent{RuntimeKind: RuntimeKindDSH, Role: RoleWorker}
	items := []skilllocal.SkillSummary{{Name: "agent-teams", Source: skillinstall.SourceUnknown}}
	summaries, err := workspaceSkillSummaries(context.Background(), root, agent, items)
	if err != nil || len(summaries) != 1 || summaries[0].Source != skillinstall.SourceUnknown {
		t.Fatalf("missing bundle summary = %+v, %v", summaries, err)
	}
}
