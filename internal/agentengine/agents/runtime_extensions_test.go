package agents

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"csgclaw/internal/config"
)

func TestRuntimeExtensionsPersistWithAgentRepository(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "agents.json")
	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager:test", statePath)
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.agents["agent-a"] = Agent{ID: "agent-a", Name: "A", Role: RoleWorker, RuntimeKind: RuntimeKindCodex, CreatedAt: time.Now().UTC()}
	err = svc.saveLocked()
	svc.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"agent_id":"agent-a","spec":{"name":"docs"}}`)
	if err := svc.PutRuntimeExtension("agent-a", "docs", raw); err != nil {
		t.Fatal(err)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager:test", statePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	got, ok, err := reloaded.RuntimeExtension("agent-a", "docs")
	var gotValue, wantValue any
	_ = json.Unmarshal(got, &gotValue)
	_ = json.Unmarshal(raw, &wantValue)
	if err != nil || !ok || !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("RuntimeExtension() = %s, %v, %v", got, ok, err)
	}
	if err := reloaded.DeleteRuntimeExtension("agent-a", "docs"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := reloaded.RuntimeExtension("agent-a", "docs"); err != nil || ok {
		t.Fatalf("RuntimeExtension() after delete found=%v err=%v", ok, err)
	}
}
