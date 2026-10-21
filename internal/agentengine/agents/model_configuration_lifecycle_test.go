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
	runtimecodex "csgclaw/internal/runtime/codex"
	"csgclaw/internal/sandbox"
)

func TestModelConfigurationSaveAppliesRuntimeIntent(t *testing.T) {
	for _, kind := range []string{RuntimeKindCodex, RuntimeKindDSH} {
		for _, entrypoint := range []string{"profile", "record"} {
			for _, scenario := range []string{"first_configuration", "running", "failed", "user_stopped", "incomplete", "start_failure"} {
				t.Run(kind+"/"+entrypoint+"/"+scenario, func(t *testing.T) {
					t.Setenv("HOME", t.TempDir())
					state := agentruntime.StateRunning
					desired := DesiredStateRunning
					profile := AgentProfile{Provider: ProviderAPI, BaseURL: "https://model.example/v1", APIKey: "test-key", ModelID: "old-model", ProfileComplete: true}
					handleID := "persisted-runtime"
					switch scenario {
					case "first_configuration":
						state, profile, handleID = "profile_incomplete", AgentProfile{}, ""
					case "failed":
						state = agentruntime.StateFailed
					case "user_stopped":
						state, desired = agentruntime.StateStopped, DesiredStateStopped
					case "incomplete":
						state = agentruntime.StateUnknown
					}
					starts, stops := 0, 0
					var skillsRoot string
					startErr := errors.New("runtime start failed")
					svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager:test", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(fakeAgentRuntime{
						kind:    kind,
						restart: func(agentruntime.RuntimeConfigChange) (bool, error) { return true, nil },
						stop: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
							stops++
							state = agentruntime.StateStopped
							if scenario == "first_configuration" {
								return state, sandbox.ErrNotFound
							}
							return state, nil
						},
						start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
							starts++
							if scenario == "first_configuration" {
								assertDefaultSystemSkills(t, skillsRoot)
							}
							if scenario == "start_failure" {
								return state, startErr
							}
							state = agentruntime.StateRunning
							return state, nil
						},
						info: func(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
							if scenario == "first_configuration" && starts == 0 {
								return agentruntime.Info{}, sandbox.ErrNotFound
							}
							return agentruntime.Info{HandleID: "persisted-runtime", State: state}, nil
						},
						del: func(context.Context, agentruntime.Handle) error {
							t.Fatal("saving model configuration deleted the runtime and its history")
							return nil
						},
					}))
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = svc.Close() })
					skillsRoot, err = svc.agentSkillsRoot(ManagerUserID, kind)
					if err != nil {
						t.Fatal(err)
					}
					svc.agents[ManagerUserID] = Agent{ID: ManagerUserID, Name: ManagerName, Role: RoleManager, RuntimeID: "rt-agent-manager", RuntimeKind: kind, BoxID: handleID, Status: string(state), DesiredState: desired, AgentProfile: profile, ProfileComplete: profile.ProfileComplete}
					next := AgentProfile{Provider: ProviderAPI, BaseURL: "https://model.example/v1", APIKey: "test-key", ModelID: "new-model"}
					if scenario == "incomplete" {
						next.ModelID = ""
					}
					if entrypoint == "profile" {
						_, err = svc.UpdateAgentProfile(ManagerUserID, next)
					} else {
						_, err = svc.UpdateRecord(context.Background(), ManagerUserID, UpdateRequest{AgentProfile: &next, FieldMask: []string{"agent_profile"}})
					}
					got, ok := svc.agentSnapshot(ManagerUserID)
					if !ok {
						t.Fatal("configured Manager disappeared")
					}
					if scenario == "start_failure" {
						if !errors.Is(err, startErr) || got.Status != string(agentruntime.StateStopped) || !got.AgentProfile.EnvRestartRequired {
							t.Fatalf("failed startup: error=%v state=%s pending=%v", err, got.Status, got.AgentProfile.EnvRestartRequired)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if scenario == "user_stopped" || scenario == "incomplete" {
						if starts != 0 || stops != 0 || !got.AgentProfile.EnvRestartRequired {
							t.Fatalf("unexpected startup: starts=%d stops=%d pending=%v", starts, stops, got.AgentProfile.EnvRestartRequired)
						}
						return
					}
					if starts != 1 || stops != 1 || got.Status != string(agentruntime.StateRunning) || got.AgentProfile.EnvRestartRequired {
						t.Fatalf("configuration not applied: starts=%d stops=%d state=%s pending=%v", starts, stops, got.Status, got.AgentProfile.EnvRestartRequired)
					}
					if got.DesiredState != DesiredStateRunning || got.BoxID != "persisted-runtime" {
						t.Fatalf("runtime intent or identity changed: desired=%s handle=%s", got.DesiredState, got.BoxID)
					}
				})
			}
		}
	}
}

func TestManagerModelSaveInitializesSkillsOnlyOnFirstCodexStart(t *testing.T) {
	t.Cleanup(runtimecodex.TestOnlySetResponsesAPIProbe(func(context.Context, string, string, string, map[string]string) error {
		return nil
	}))
	for _, entrypoint := range []string{"profile", "record"} {
		t.Run(entrypoint, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("CODEX_HOME", t.TempDir())
			var svc *Controller
			var skillsRoot string
			starts := 0
			edited := "---\nname: skill-creator\ndescription: edited\n---\nuser edits\n"
			custom := "---\nname: custom\ndescription: custom\n---\nuser skill\n"
			manager := &creationCodexSessionManager{check: func(spec runtimecodex.SessionSpec) {
				starts++
				if starts == 1 {
					assertDefaultSystemSkills(t, filepath.Join(spec.CodexHomeDir, "skills"))
					return
				}
				if _, err := os.Stat(filepath.Join(skillsRoot, "skill-installer")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("model save restored a deleted system skill: %v", err)
				}
				for name, want := range map[string]string{"skill-creator": edited, "custom": custom} {
					got, err := os.ReadFile(filepath.Join(skillsRoot, name, "SKILL.md"))
					if err != nil || string(got) != want {
						t.Fatalf("model save changed skill %q: error=%v content=%q", name, err, got)
					}
				}
			}}
			rt := runtimecodex.New(runtimecodex.Dependencies{
				BinaryProvider: recreateCodexBinaryProvider{},
				Manager:        manager,
				AgentHome:      func(id string) (string, error) { return svc.agentHomeDir(id) },
				ResolveAgent: func(h agentruntime.Handle) (runtimecodex.AgentRef, error) {
					item, ok := svc.agentSnapshot(ManagerUserID)
					if !ok {
						return runtimecodex.AgentRef{}, fmt.Errorf("Manager not persisted before startup")
					}
					return runtimecodex.AgentRef{ID: item.ID, Name: item.Name, RuntimeID: h.RuntimeID, HandleID: item.BoxID, Profile: svc.runtimeProfileForAgent(item), RuntimeOptions: item.RuntimeOptions}, nil
				},
			})
			var err error
			svc, err = NewController(config.ModelConfig{}, config.ServerConfig{}, "", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(rt), WithStartupProfileDetectionDisabled())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = svc.Close() })
			initial, err := svc.EnsureManager(context.Background(), false)
			if err != nil {
				t.Fatal(err)
			}
			if initial.ProfileComplete || initial.BoxID != "" || starts != 0 {
				t.Fatalf("Manager unexpectedly initialized: complete=%v handle=%s starts=%d", initial.ProfileComplete, initial.BoxID, starts)
			}
			skillsRoot, err = svc.agentSkillsRoot(ManagerUserID, RuntimeKindCodex)
			if err != nil {
				t.Fatal(err)
			}
			saveModel := func(model string) {
				t.Helper()
				next := AgentProfile{Provider: ProviderAPI, BaseURL: "https://model.example/v1", APIKey: "test-key", ModelID: model}
				if entrypoint == "profile" {
					_, err = svc.UpdateAgentProfile(ManagerUserID, next)
				} else {
					_, err = svc.UpdateRecord(context.Background(), ManagerUserID, UpdateRequest{AgentProfile: &next, FieldMask: []string{"agent_profile"}})
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			saveModel("first-model")
			if starts != 1 {
				t.Fatalf("first model save started %d sessions, want 1", starts)
			}
			if err := svc.DeleteSkill(ManagerUserID, "skill-installer"); err != nil {
				t.Fatal(err)
			}
			for name, content := range map[string]string{"skill-creator": edited, "custom": custom} {
				file := filepath.Join(skillsRoot, name, "SKILL.md")
				if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			saveModel("second-model")
			if starts != 2 {
				t.Fatalf("second model save started %d sessions, want 2", starts)
			}
		})
	}
}
