package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestRuntimeProvisionStatePersistsWithoutEnteringAgentJSON(t *testing.T) {
	item := Agent{ID: "agent-a", Name: "alice", Role: RoleWorker, RuntimeKind: RuntimeKindCodex}
	item.SetRuntimeProvision(map[string]string{"secrets/token": "persisted-secret"}, "test -f secrets/token")

	raw, err := json.Marshal(newPersistedAgent(item))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "persisted-secret") || !strings.Contains(string(raw), "runtime_init_shell") {
		t.Fatalf("persisted Agent omitted Runtime provisioning state: %s", raw)
	}
	var stored persistedAgent
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	loaded := stored.toAgent()
	credentials, initShell := loaded.RuntimeProvision()
	if credentials["secrets/token"] != "persisted-secret" || initShell != "test -f secrets/token" {
		t.Fatalf("loaded Runtime provisioning = %#v, %q", credentials, initShell)
	}
	credentials["secrets/token"] = "mutated"
	got, _ := loaded.RuntimeProvision()
	if got["secrets/token"] != "persisted-secret" {
		t.Fatalf("RuntimeProvision returned mutable credentials: %#v", got)
	}

	public, err := json.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(public), "persisted-secret") || strings.Contains(string(public), "test -f secrets/token") {
		t.Fatalf("Agent JSON leaked Runtime provisioning state: %s", public)
	}
	if rendered := fmt.Sprintf("%+v", loaded); strings.Contains(rendered, "persisted-secret") {
		t.Fatalf("formatted Agent leaked Runtime credentials: %s", rendered)
	}
}
