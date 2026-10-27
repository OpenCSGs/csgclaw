package agents

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/config"
)

func TestUnavailableRuntimeRecordsSurviveOtherAgentSaves(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state.json")
	original := `{"version":1,"agents":{"items":[{"id":"agent-manager","name":"manager","role":"manager","runtime":{"kind":"unavailable-fixture","state":"running","sandbox_id":"manager-box"}},{"id":"agent-pending","name":"pending","role":"worker","runtime":{"kind":"unavailable-fixture","state":"running","sandbox_id":"worker-box"},"desired_state":"running"},{"id":"agent-supported","name":"supported","role":"worker","runtime":{"kind":"codex","state":"stopped"},"desired_state":"running"}]}}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "", path)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if err := svc.EnsureBootstrapManager(context.Background(), false); err != nil {
		t.Fatalf("unavailable manager blocked bootstrap: %v", err)
	}
	if _, err := svc.EnsureManager(context.Background(), false); err == nil {
		t.Fatal("unavailable manager started without migration")
	}
	if _, err := svc.SetDesiredState("agent-supported", DesiredStateStopped, false); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatalf("unavailable record corrupted by unrelated save: %v", err)
	}
	for id, box := range map[string]string{"agent-manager": "manager-box", "agent-pending": "worker-box"} {
		agent, ok := svc.Agent(id)
		if !ok || agent.RuntimeKind != "unavailable-fixture" || agent.BoxID != box || agent.Status != StatusRuntimeUnavailable {
			t.Fatalf("unavailable Agent lost: %+v", agent)
		}
		if !isManagerAgent(agent) && isConfiguredAgentStartupCandidate(agent) {
			t.Fatal("unavailable worker selected for auto-start")
		}
	}
	var state struct {
		Agents struct {
			Items []struct {
				ID      string
				Runtime RuntimeRecord
			}
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	for _, a := range state.Agents.Items {
		if a.ID != "agent-supported" && (a.Runtime.Kind != "unavailable-fixture" || a.Runtime.SandboxID == "") {
			t.Fatalf("retry identifiers lost: %s", data)
		}
	}
}
