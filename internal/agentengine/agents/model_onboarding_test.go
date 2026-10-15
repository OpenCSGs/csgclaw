package agents

import (
	"context"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
	"path/filepath"
	"testing"
)

func TestStartNewlyConfiguredCodexWithoutPreviousRuntime(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	started := 0
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager:test", filepath.Join(t.TempDir(), "agents.json"), WithRuntime(fakeAgentRuntime{
		kind: RuntimeKindCodex,
		stop: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
			return agentruntime.StateUnknown, sandbox.ErrNotFound
		},
		new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {

			return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "onboarding"}, nil
		},
		start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
			started++
			return agentruntime.StateRunning, nil
		},
		info: func(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
			return agentruntime.Info{State: agentruntime.StateRunning}, nil
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	svc.agents["u-test"] = Agent{ID: "u-test", Name: "test", Role: RoleWorker, RuntimeID: "rt-u-test", RuntimeKind: RuntimeKindCodex, Status: string(agentruntime.StateUnknown), AgentProfile: AgentProfile{Provider: ProviderAPI, BaseURL: "https://model.example/v1", APIKey: "test-key", ModelID: "test-model", ProfileComplete: true, EnvRestartRequired: true}, ProfileComplete: true}
	got, err := svc.Start(context.Background(), "u-test")
	if err != nil {
		t.Fatal(err)
	}
	if started != 1 || got.AgentProfile.EnvRestartRequired || got.Status != string(agentruntime.StateRunning) {
		t.Fatalf("started=%d status=%s restart=%v", started, got.Status, got.AgentProfile.EnvRestartRequired)
	}
}
