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

func TestCreateAgentRequestUnmarshalJSONSupportsLegacyRuntimeKind(t *testing.T) {
	var got CreateAgentRequest
	if err := json.Unmarshal([]byte(`{"name":"alice","role":"worker","runtime_kind":"openclaw_sandbox","runtime_options":{"cwd":"/tmp"}}`), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.RuntimeKind != "openclaw_sandbox" {
		t.Fatalf("RuntimeKind = %q, want %q", got.RuntimeKind, "openclaw_sandbox")
	}
	if got.RuntimeName != "openclaw" {
		t.Fatalf("RuntimeName = %q, want %q", got.RuntimeName, "openclaw")
	}
	if !got.SandboxEnabled {
		t.Fatal("SandboxEnabled = false, want true")
	}
	if got.RuntimeOptions["cwd"] != "/tmp" {
		t.Fatalf("RuntimeOptions[cwd] = %#v, want %q", got.RuntimeOptions["cwd"], "/tmp")
	}
}

func TestCreateAgentRequestUnmarshalJSONSupportsBareSandboxRuntimeKind(t *testing.T) {
	var got CreateAgentRequest
	if err := json.Unmarshal([]byte(`{"name":"alice","role":"worker","runtime_kind":"openclaw"}`), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.RuntimeKind != "openclaw" {
		t.Fatalf("RuntimeKind = %q, want %q", got.RuntimeKind, "openclaw")
	}
	if got.RuntimeName != "openclaw" {
		t.Fatalf("RuntimeName = %q, want %q", got.RuntimeName, "openclaw")
	}
	if !got.SandboxEnabled {
		t.Fatal("SandboxEnabled = false, want true")
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
	if got := string(fields["mcpServers"]); got != "{}" {
		t.Fatalf("mcpServers = %s, want {}", got)
	}
}
