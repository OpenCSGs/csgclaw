package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agentruntime "csgclaw/internal/runtime"
)

func TestExtensionExecutableBundledCodexE2E(t *testing.T) {
	for _, snapshot := range []bool{false, true} {
		t.Run(fmt.Sprintf("shell_snapshot_%t", snapshot), func(t *testing.T) { testExtensionExecutableBundledCodex(t, snapshot) })
	}
}

func testExtensionExecutableBundledCodex(t *testing.T, snapshot bool) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY")
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is required for the login-shell fixture")
	}
	binary, _ = filepath.Abs(binary)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, "host"))
	t.Setenv("SHELL", "/bin/zsh")
	if err := os.MkdirAll(filepath.Join(home, "host"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "host", "config.toml"), []byte(fmt.Sprintf("[features]\nshell_snapshot = %t\n", snapshot)), 0600); err != nil {
		t.Fatal(err)
	}
	managed := filepath.Join(home, "managed", "bin")
	competing := filepath.Join(home, "global", "bin")
	for _, dir := range []string{managed, competing} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for dir, output := range map[string]string{managed: "managed-cli", competing: "global-cli"} {
		if err := os.WriteFile(filepath.Join(dir, "lark-cli"), []byte("#!/bin/sh\nprintf '"+output+"\\n'\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".zprofile"), []byte("export PATH="+tomlQuotedKey(competing)+":$PATH\n"), 0600); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(home, "connector-auth")
	var calls atomic.Int64
	var output atomic.Value
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		n := calls.Add(1)
		if n > 1 {
			for _, item := range body["input"].([]any) {
				value := item.(map[string]any)
				if value["type"] == "function_call_output" {
					output.Store(value["output"].(string))
				}
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(v any) { raw, _ := json.Marshal(v); fmt.Fprintf(w, "data: %s\n\n", raw) }
		emit(map[string]any{"type": "response.created", "response": map[string]any{"id": "r1"}})
		if n == 1 {
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "id": "tool1", "call_id": "tool1", "name": "exec_command", "arguments": `{"cmd":"command -v lark-cli; lark-cli -v; printenv LARKSUITE_CLI_CONFIG_DIR","yield_time_ms":1000}`}})
		} else {
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "id": "m1", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "done"}}}})
		}
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": "r1", "usage": map[string]any{"input_tokens": 100, "output_tokens": 20, "total_tokens": 120}}})
	}))
	defer endpoint.Close()
	profile := testAgentMCPProfile()
	profile.Provider = "api"
	profile.ModelID = "fixture-model"
	profile.BaseURL = endpoint.URL + "/v1"
	delete(profile.Env, "CSGCLAW_BASE_URL")
	agentHome := filepath.Join(home, "agent")
	rt := New(Dependencies{EventSink: NewEventSink(), BinaryProvider: fakeBinaryProvider{path: binary}, AgentHome: func(string) (string, error) { return agentHome, nil }, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "alice", Name: "alice", Profile: profile, RuntimeOptions: map[string]any{"memory_mode": "disabled"}}, nil
	}})
	defer rt.Close()
	store, err := extensionStore(filepath.Dir(rt.Layout(agentHome).SkillsRoot))
	if err != nil {
		t.Fatal(err)
	}
	change, err := store.Stage("feishu-lark-cli")
	if err != nil {
		t.Fatal(err)
	}
	change.SetProjection(agentruntime.ExtensionProjection{Name: "feishu-lark-cli", Kind: "lark-cli", Executable: filepath.Join(managed, "lark-cli"), Environment: map[string]string{"LARKSUITE_CLI_CONFIG_DIR": configDir}})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := change.Activate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := change.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	h, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-alice", AgentID: "alice", AgentName: "alice", Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := rt.EnsureEngineSession(ctx, h.RuntimeID, "room:test")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Prompt(ctx, h.RuntimeID, thread, "Check the connector executable."); err != nil {
		t.Fatal(err)
	}
	got, _ := output.Load().(string)
	for _, want := range []string{filepath.Join(managed, "lark-cli"), "managed-cli", configDir} {
		if !strings.Contains(got, want) {
			t.Errorf("shell output missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "global-cli") {
		t.Errorf("shell selected global executable: %s", got)
	}
}
