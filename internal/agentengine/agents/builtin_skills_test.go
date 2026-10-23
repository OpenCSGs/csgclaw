package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/skill/builtin"
	skillinstall "csgclaw/internal/skill/installstate"
	"csgclaw/internal/skill/local"
	skillstate "csgclaw/internal/skill/state"
)

func TestManagerInstallsBundledSkillsWithoutRuntime(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "", filepath.Join(t.TempDir(), "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	svc.deferManagerRuntimeStart = true
	if _, err := svc.EnsureManager(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	managerRoot, err := svc.agentSkillsRoot(ManagerUserID, RuntimeKindCodex)
	if err != nil {
		t.Fatal(err)
	}
	localRoot, _ := local.SkillsRoot()
	for _, name := range builtin.Names() {
		for _, root := range []string{managerRoot, localRoot} {
			if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Workers receive only the existing default system skills until manually added.
	item := Agent{ID: "builtin-worker", Name: "worker", Role: RoleWorker, RuntimeKind: RuntimeKindCodex}
	svc.agents[item.ID] = item
	if err := svc.installDefaultSystemSkills(item.ID, item.RuntimeKind); err != nil {
		t.Fatal(err)
	}
	workerRoot, _ := svc.agentSkillsRoot(item.ID, item.RuntimeKind)
	if _, err := os.Stat(filepath.Join(workerRoot, "pdf")); !os.IsNotExist(err) {
		t.Fatalf("worker auto-installed pdf: %v", err)
	}
	if err := svc.BatchAddSkills(item.ID, []string{"pdf"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workerRoot, "pdf", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestManagerBootstrapSurvivesBundledSkillFailures(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, err := local.SkillsRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	// A broken catalog previously prevented manager bootstrap entirely.
	if err := os.WriteFile(filepath.Join(root, ".skillhub-lock.json"), []byte("{"), 0644); err != nil {
		t.Fatal(err)
	}
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "", filepath.Join(t.TempDir(), "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	svc.deferManagerRuntimeStart = true
	workspace, err := svc.agentSkillsRoot(ManagerUserID, RuntimeKindCodex)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspace, 0755); err != nil {
		t.Fatal(err)
	}
	// A broken optional activation must not prevent existing system skills loading.
	if err := os.WriteFile(filepath.Join(workspace, ".builtin-deepseek-search.json"), []byte("{"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnsureManager(context.Background(), false); err != nil {
		t.Fatalf("optional skill failure blocked manager: %v", err)
	}
	if err := svc.installDefaultSystemSkills(ManagerUserID, RuntimeKindCodex); err != nil {
		t.Fatalf("optional skill failure blocked system skills: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "skill-creator", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	// Correcting the catalog allows the next sync to retry normally.
	if err := os.WriteFile(filepath.Join(root, ".skillhub-lock.json"), []byte(`{"skills":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := builtin.SyncLocal(); err != nil {
		t.Fatal(err)
	}
}

func TestManagerRecreationPreservesUserOwnedBuiltinName(t *testing.T) {
	for _, source := range []string{skillinstall.SourceLocal, skillinstall.SourceMarket, ""} {
		for _, failCreate := range []bool{false, true} {
			t.Run(fmt.Sprintf("source=%s/fail=%t", source, failCreate), func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				var skillsRoot string
				createFailure := errors.New("runtime creation failed")
				svc, err := NewController(testModelConfig(), config.ServerConfig{ListenAddr: ":18080"}, "", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(fakeAgentRuntime{
					kind: RuntimeKindCodex,
					stop: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
						return agentruntime.StateStopped, nil
					},
					start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
						return agentruntime.StateRunning, nil
					},
					del: func(context.Context, agentruntime.Handle) error { return os.RemoveAll(skillsRoot) },
					new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
						if failCreate {
							return agentruntime.Handle{}, createFailure
						}
						return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "manager-new"}, nil
					},
					info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
						return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
					},
				}))
				if err != nil {
					t.Fatal(err)
				}
				defer svc.Close()
				svc.agents[ManagerUserID] = Agent{ID: ManagerUserID, Name: ManagerName, Role: RoleManager, RuntimeID: runtimeIDForAgentID(ManagerUserID), RuntimeKind: RuntimeKindCodex, BoxID: "manager-old", Status: string(agentruntime.StateRunning), ProfileComplete: true,
					AgentProfile: AgentProfile{Name: ManagerName, Provider: ProviderAPI, BaseURL: "https://api.example/v1", APIKey: "test-key", ModelID: "test", ProfileComplete: true},
					SkillStates:  map[string]skillstate.State{"pdf": {Enabled: false}},
				}
				skillsRoot, err = svc.agentSkillsRoot(ManagerUserID, RuntimeKindCodex)
				if err != nil {
					t.Fatal(err)
				}
				writeWorkspaceFileAt(t, skillsRoot, "pdf/SKILL.md", "# User PDF\n", 0644)
				writeWorkspaceFileAt(t, skillsRoot, "pdf/custom.txt", "user resource", 0644)
				if source != "" {
					origin := skillinstall.Record{Source: source, Version: "user-version", SHA256: "user-hash"}
					if err := skillinstall.Update(skillsRoot, "pdf", &origin, nil); err != nil {
						t.Fatal(err)
					}
				}
				_, err = svc.RecreateRecord(context.Background(), ManagerUserID)
				if failCreate {
					if !errors.Is(err, createFailure) {
						t.Fatalf("recreate error=%v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				records, err := skillinstall.Read(skillsRoot)
				if err != nil {
					t.Fatal(err)
				}
				if records["pdf"].Source != source {
					t.Fatalf("user origin changed: %+v", records["pdf"])
				}
				if source != "" && records["pdf"].Version != "user-version" {
					t.Fatal("user version changed")
				}
				entries, err := os.ReadDir(filepath.Join(skillsRoot, "pdf"))
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 2 {
					t.Fatalf("bundled files merged into user directory: %v", entries)
				}
				data, err := os.ReadFile(filepath.Join(skillsRoot, "pdf", "SKILL.md"))
				if err != nil || string(data) != "# User PDF\n" {
					t.Fatalf("user contents changed: %v", err)
				}
				manager, _ := svc.Agent(ManagerUserID)
				state, present := manager.SkillStates["pdf"]
				if !present || state.Enabled {
					t.Fatal("disabled user skill state lost or re-enabled")
				}
				if !failCreate {
					for _, name := range builtin.Names() {
						if name != "pdf" && records[name].Source != skillinstall.SourceBuiltin {
							t.Fatalf("missing default %s", name)
						}
					}
				}
			})
		}
	}
}
