package apitypes

import (
	"encoding/json"
	"testing"
)

func TestAgentUnmarshalJSONSupportsLegacyRuntimeKind(t *testing.T) {
	var got Agent
	if err := json.Unmarshal([]byte(`{"id":"u-alice","name":"alice","role":"worker","runtime_kind":"codex","status":"running"}`), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.RuntimeKind != "codex" {
		t.Fatalf("RuntimeKind = %q, want %q", got.RuntimeKind, "codex")
	}
	if got.RuntimeName != "codex" {
		t.Fatalf("RuntimeName = %q, want %q", got.RuntimeName, "codex")
	}
	if got.SandboxEnabled {
		t.Fatal("SandboxEnabled = true, want false")
	}
}

func TestCreateAgentRequestUnmarshalJSONReadsRuntimeConfiguration(t *testing.T) {
	var got CreateAgentRequest
	if err := json.Unmarshal([]byte(`{"name":"alice","role":"worker","runtime":{"name":"dsh","sandbox_enabled":false,"options":{"cwd":"/tmp"}}}`), &got); err != nil {
		t.Fatal(err)
	}
	if got.Runtime == nil || got.Runtime.Name == nil || *got.Runtime.Name != "dsh" || got.Runtime.SandboxEnabled == nil || *got.Runtime.SandboxEnabled {
		t.Fatalf("Runtime = %+v", got.Runtime)
	}
	if got.Runtime.Options == nil || (*got.Runtime.Options)["cwd"] != "/tmp" {
		t.Fatalf("Runtime.Options = %+v", got.Runtime.Options)
	}
}

func TestCreateAgentRequestMarshalJSONPreservesExplicitEmptyMCPServers(t *testing.T) {
	data, err := json.Marshal(CreateAgentRequest{
		Name:          "alice",
		MCPServers:    map[string]any{},
		MCPServersSet: true,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := string(fields["mcp_servers"]); got != "{}" {
		t.Fatalf("mcpServers = %s, want {}", got)
	}
}
