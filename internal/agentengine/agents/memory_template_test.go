package agents

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/agentmemory"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
)

func TestManagedTemplateMemoryReachesFirstPrivateTurn(t *testing.T) {
	for _, kind := range []string{RuntimeKindCodex, RuntimeKindDSH} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			store, err := agentmemory.Open(filepath.Join(root, "agents"), filepath.Join(root, "state.json"), "local")
			if err != nil {
				t.Fatal(err)
			}
			native := fakeAgentRuntime{kind: kind, provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				if req.TemplateMemorySet || req.TemplateMemory != "" {
					t.Fatal("managed template seed leaked into native memory provisioning")
				}
				return nil
			}}
			controller, err := NewController(testModelConfig(), config.ServerConfig{}, "", filepath.Join(root, "agents.json"), WithRuntime(native))
			if err != nil {
				t.Fatal(err)
			}
			controller.SetManagedMemory(store.DeleteAgent, store.SeedTemplate)
			err = controller.provisionRuntime(context.Background(), native, kind, agentruntime.ProvisionRequest{AgentID: "agent-template", TemplateMemory: "# Team knowledge\nUse the project test runner.", TemplateMemorySet: true})
			if err != nil {
				t.Fatal(err)
			}
			scope := agentmemory.Scope{AgentID: "agent-template", UserID: "user-admin"}
			_, _, text, err := store.Snapshot(scope, "")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(text, "Use the project test runner.") {
				t.Fatalf("template knowledge missing from first turn: %q", text)
			}
		})
	}
}
