package agents

import (
	"csgclaw/internal/config"
	"csgclaw/internal/taskconfig"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskModelSelectionUsesCatalogWithoutMutatingAgent(t *testing.T) {
	state := filepath.Join(t.TempDir(), "agents.json")
	data, _ := json.Marshal(map[string]any{"agents": []Agent{{ID: "agent-test", Name: "test", Role: RoleWorker, RuntimeKind: RuntimeKindCodex, AgentProfile: AgentProfile{Name: "test", ModelProviderID: "openai", Provider: ProviderAPI, ModelID: "first", Env: map[string]string{"PROJECT_OPTION": "enabled"}}}}})
	if err := os.WriteFile(state, data, 0600); err != nil {
		t.Fatal(err)
	}
	svc, err := NewControllerWithLLM(config.LLMConfig{Default: "openai.first", Providers: map[string]config.ProviderConfig{"openai": {BaseURL: "https://models.example/v1", APIKey: "test-key", Models: []string{"first", "qwen3.7-plus"}}}}, config.ServerConfig{ListenAddr: "127.0.0.1:18080"}, "manager:test", state)
	if err != nil {
		t.Fatal(err)
	}
	cfg := taskconfig.Config{ModelProviderID: "openai", ModelID: "qwen3.7-plus", ReasoningEffort: "high"}
	resolved, err := svc.Models().ResolvedTaskProfile("agent-test", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ModelID != "qwen3.7-plus" || resolved.APIKey != "test-key" || resolved.Name != "test" || resolved.ReasoningEffort != "high" || resolved.Env["PROJECT_OPTION"] != "enabled" {
		t.Fatal("task profile was not hydrated")
	}
	execution, err := svc.TaskRuntimeProfile("agent-test", "room-one", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(execution.BaseURL, "/agents/agent-test/tasks/room-one/llm") || execution.ModelID != "qwen3.7-plus" {
		t.Fatalf("wrong task bridge: %s", execution.BaseURL)
	}
	original, _ := svc.Agent("agent-test")
	if original.AgentProfile.ModelID != "first" {
		t.Fatal("modified agent default")
	}
	cfg.ModelID = "missing"
	if _, err := svc.Models().ResolvedTaskProfile("agent-test", cfg); err == nil {
		t.Fatal("accepted model outside the catalog")
	}
}
