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
	if err := json.Unmarshal([]byte(`{"name":"alice","role":"worker","runtime_kind":"dsh","runtime_options":{"cwd":"/tmp"}}`), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.RuntimeKind != "dsh" {
		t.Fatalf("RuntimeKind = %q, want %q", got.RuntimeKind, "dsh")
	}
	if got.RuntimeName != "dsh" {
		t.Fatalf("RuntimeName = %q, want %q", got.RuntimeName, "dsh")
	}
	if got.SandboxEnabled {
		t.Fatal("SandboxEnabled = true, want false")
	}
	if got.RuntimeOptions["cwd"] != "/tmp" {
		t.Fatalf("RuntimeOptions[cwd] = %#v, want %q", got.RuntimeOptions["cwd"], "/tmp")
	}
}

func TestCreateAgentRequestUnmarshalJSONSupportsDSHRuntimeKind(t *testing.T) {
	var got CreateAgentRequest
	if err := json.Unmarshal([]byte(`{"name":"alice","role":"worker","runtime_kind":"dsh"}`), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.RuntimeKind != "dsh" {
		t.Fatalf("RuntimeKind = %q, want %q", got.RuntimeKind, "dsh")
	}
	if got.RuntimeName != "dsh" {
		t.Fatalf("RuntimeName = %q, want %q", got.RuntimeName, "dsh")
	}
	if got.SandboxEnabled {
		t.Fatal("SandboxEnabled = true, want false")
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
