package agents

import (
	"context"
	"path/filepath"
	"testing"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
)

func TestBootstrapDefersConfiguredManagerUntilServing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	statePath := filepath.Join(t.TempDir(), "agents.json")
	serving, starts := false, 0
	rt := fakeAgentRuntime{
		kind: RuntimeKindCodex,
		new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
			if !serving {
				t.Fatal("Manager started before its required platform MCP endpoint was available")
			}
			starts++
			return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "manager-runtime"}, nil
		},
		info: func(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
			state := agentruntime.StateStopped
			if starts > 0 {
				state = agentruntime.StateRunning
			}
			return agentruntime.Info{State: state}, nil
		},
	}
	bootstrap, err := NewController(testModelConfig(), config.ServerConfig{}, "manager:test", statePath,
		WithRuntime(rt), WithDeferredManagerRuntimeStart())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := bootstrap.EnsureManager(context.Background(), false)
	if err != nil {
		_ = bootstrap.Close()
		t.Fatal(err)
	}
	if starts != 0 || !manager.ProfileComplete || manager.DesiredState != DesiredStateRunning || manager.BoxID != "" {
		t.Fatalf("unexpected bootstrap state: starts=%d complete=%v desired=%s handle=%s", starts, manager.ProfileComplete, manager.DesiredState, manager.BoxID)
	}
	if err := bootstrap.Close(); err != nil {
		t.Fatal(err)
	}
	serving = true
	service, err := NewController(testModelConfig(), config.ServerConfig{}, "manager:test", statePath, WithRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = service.Close() }()
	manager, err = service.EnsureManager(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if starts != 1 || manager.Status != string(agentruntime.StateRunning) || manager.AgentProfile.ModelID != testModelConfig().ModelID {
		t.Fatalf("Manager not started after serving: starts=%d status=%s model=%s", starts, manager.Status, manager.AgentProfile.ModelID)
	}
}
