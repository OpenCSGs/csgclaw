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

func TestCreateAgentRequestMarshalUsesSplitRuntimeFields(t *testing.T) {
	data, err := json.Marshal(CreateAgentRequest{
		Name:        "alice",
		RuntimeKind: "codex_sandbox",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode marshaled request: %v", err)
	}
	if _, ok := got["runtime_kind"]; ok {
		t.Fatalf("runtime_kind = %#v, want omitted", got["runtime_kind"])
	}
	if got["runtime_name"] != "codex" {
		t.Fatalf("runtime_name = %#v, want %q", got["runtime_name"], "codex")
	}
	if got["sandbox_enabled"] != true {
		t.Fatalf("sandbox_enabled = %#v, want true", got["sandbox_enabled"])
	}
	runtime, ok := got["runtime"].(map[string]any)
	if !ok {
		t.Fatalf("runtime = %#v, want object", got["runtime"])
	}
	if runtime["name"] != "codex" || runtime["sandbox_enabled"] != true {
		t.Fatalf("runtime = %#v, want codex sandbox selection", runtime)
	}
}

func TestCreateAgentRequestUnmarshalProfileSelector(t *testing.T) {
	var got CreateAgentRequest
	if err := json.Unmarshal([]byte(`{"name":"alice","profile":" codex.gpt-5.5 "}`), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.Profile != "codex.gpt-5.5" {
		t.Fatalf("Profile = %q, want %q", got.Profile, "codex.gpt-5.5")
	}
	if got.ProfileConfig != nil {
		t.Fatalf("ProfileConfig = %+v, want nil for selector profile", got.ProfileConfig)
	}
}

func TestCreateAgentRequestUnmarshalProfileObject(t *testing.T) {
	var got CreateAgentRequest
	if err := json.Unmarshal([]byte(`{"name":"alice","profile":{"model_provider_id":"codex","model_id":"gpt-5.5","env":{"OPENAI_API_KEY":"from-env"}}}`), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.Profile != "" {
		t.Fatalf("Profile = %q, want empty selector for object profile", got.Profile)
	}
	if got.ProfileConfig == nil {
		t.Fatalf("ProfileConfig = nil, want decoded object profile")
	}
	if got.ProfileConfig.ModelProviderID != "codex" || got.ProfileConfig.ModelID != "gpt-5.5" {
		t.Fatalf("ProfileConfig = %+v, want codex/gpt-5.5", got.ProfileConfig)
	}
	if got.ProfileConfig.Env["OPENAI_API_KEY"] != "from-env" {
		t.Fatalf("ProfileConfig.Env = %+v, want OPENAI_API_KEY", got.ProfileConfig.Env)
	}
}
