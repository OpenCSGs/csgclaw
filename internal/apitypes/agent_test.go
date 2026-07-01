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
