package agents

import (
	"context"
	"strings"
	"testing"
	"time"

	agentruntime "csgclaw/internal/runtime"

	"csgclaw/internal/config"
)

type fakeMemoryAgentRuntime struct {
	fakeAgentRuntime
	read func(context.Context, string, map[string]any) (agentruntime.MemoryDocument, error)
}

func (f fakeMemoryAgentRuntime) ReadMemoryDocument(ctx context.Context, agentHome string, options map[string]any) (agentruntime.MemoryDocument, error) {
	if f.read != nil {
		return f.read(ctx, agentHome, options)
	}
	enabled := options["memory_mode"] != "disabled"
	return agentruntime.MemoryDocument{Enabled: enabled, Ready: true, Name: "MEMORY.md", Location: "$RUNTIME_HOME/MEMORY.md", Content: "# Memory\n"}, nil
}

func (f fakeMemoryAgentRuntime) ConfigureMemory(options map[string]any, enabled bool) (map[string]any, error) {
	next := make(map[string]any, len(options)+1)
	for key, value := range options {
		next[key] = value
	}
	if enabled {
		next["memory_mode"] = "enabled"
	} else {
		next["memory_mode"] = "disabled"
	}
	return next, nil
}

func TestMemoryDocumentUsesRuntimeCapability(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var readHome string
	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "",
		WithRuntime(fakeMemoryAgentRuntime{
			fakeAgentRuntime: fakeAgentRuntime{kind: RuntimeKindCodex},
			read: func(_ context.Context, agentHome string, options map[string]any) (agentruntime.MemoryDocument, error) {
				readHome = agentHome
				return agentruntime.MemoryDocument{Enabled: options["memory_mode"] != "disabled", Ready: true, Name: "MEMORY.md", Location: "$RUNTIME_HOME/MEMORY.md", Content: "# Agent memory\n"}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["agent-alice"] = Agent{
		ID:             "agent-alice",
		Name:           "alice",
		Role:           RoleWorker,
		RuntimeKind:    RuntimeKindCodex,
		RuntimeOptions: map[string]any{"memory_mode": "disabled"},
	}

	document, err := svc.MemoryDocument(context.Background(), "agent-alice")
	if err != nil {
		t.Fatalf("MemoryDocument() error = %v", err)
	}
	if document.Enabled || !document.Ready || document.Name != "MEMORY.md" || document.Location != "$RUNTIME_HOME/MEMORY.md" || document.Content != "# Agent memory\n" {
		t.Fatalf("MemoryDocument() = %#v", document)
	}
	if !strings.Contains(readHome, "agent-alice") {
		t.Fatalf("runtime agent home = %q, want canonical agent home", readHome)
	}
	if !svc.SupportsMemory(RuntimeKindCodex) {
		t.Fatal("SupportsMemory(codex) = false")
	}
}

func TestUpdateMemoryEnabledAllowsManagedManagerSetting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "",
		WithRuntime(fakeMemoryAgentRuntime{fakeAgentRuntime: fakeAgentRuntime{kind: RuntimeKindCodex}}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:             ManagerUserID,
		Name:           ManagerName,
		Role:           RoleManager,
		RuntimeKind:    RuntimeKindCodex,
		RuntimeOptions: map[string]any{"execution_mode": "standard", "memory_mode": "enabled"},
	}

	document, err := svc.UpdateMemoryEnabled(context.Background(), ManagerUserID, false)
	if err != nil {
		t.Fatalf("UpdateMemoryEnabled() error = %v", err)
	}
	if document.Enabled {
		t.Fatalf("UpdateMemoryEnabled() = %#v, want disabled", document)
	}
	updated, ok := svc.Agent(ManagerUserID)
	if !ok || updated.RuntimeOptions["memory_mode"] != "disabled" || updated.RuntimeOptions["execution_mode"] != "standard" {
		t.Fatalf("updated manager runtime options = %#v", updated.RuntimeOptions)
	}
}

func TestPersistManagerAgentPreservesManagedMemoryMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "",
		WithRuntime(fakeMemoryAgentRuntime{fakeAgentRuntime: fakeAgentRuntime{kind: RuntimeKindCodex}}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:             ManagerUserID,
		Name:           ManagerName,
		Role:           RoleManager,
		RuntimeKind:    RuntimeKindCodex,
		RuntimeOptions: map[string]any{"memory_mode": "disabled"},
	}

	manager := svc.newCodexManagerAgent(ManagerName, "", "", "", time.Time{}, "", agentruntime.StateRunning, string(agentruntime.StateRunning), AgentProfile{}, nil)
	persisted, err := svc.persistManagerAgent(context.Background(), manager, false)
	if err != nil {
		t.Fatalf("persistManagerAgent() error = %v", err)
	}
	if persisted.RuntimeOptions["memory_mode"] != "disabled" {
		t.Fatalf("persisted manager runtime options = %#v, want disabled memory mode", persisted.RuntimeOptions)
	}
}
