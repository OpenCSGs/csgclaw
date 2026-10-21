package agents

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
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
							if scenario == "start_failure" {
								return state, startErr
							}
							state = agentruntime.StateRunning
							return state, nil
						},
						info: func(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
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
