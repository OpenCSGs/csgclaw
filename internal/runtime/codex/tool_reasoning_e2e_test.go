package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	agentruntime "csgclaw/internal/runtime"
)

func TestNativeSupportedReasoningAndSearchAcrossColdResume(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY to the bundled executable")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			http.Error(w, "bad JSON", 400)
			return
		}
		reasoning, _ := payload["reasoning"].(map[string]any)
		if reasoning["effort"] != "low" {
			t.Errorf("configured effort was changed: %v", reasoning)
		}
		sawSearch := false
		tools, _ := payload["tools"].([]any)
		for _, raw := range tools {
			tool, _ := raw.(map[string]any)
			if tool["type"] == "web_search" || tool["type"] == "web_search_preview" {
				sawSearch = true
			}
		}
		if !sawSearch {
			t.Error("supported native web search was disabled")
		}

		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(value any) { data, _ := json.Marshal(value); fmt.Fprintf(w, "data: %s\n\n", data) }
		emit(map[string]any{"type": "response.created", "response": map[string]any{"id": "response-minimal", "object": "response", "status": "in_progress"}})
		emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"id": "message-minimal", "type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "OK"}}}})
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": "response-minimal", "object": "response", "status": "completed", "usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}}})
	}))
	defer upstream.Close()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("CODEX_HOME", filepath.Join(root, "host-codex"))
	profile := agentruntime.Profile{Provider: "codex", ModelID: "gpt-5.6-luna", ReasoningEffort: "low", BaseURL: upstream.URL + "/v1", APIKey: "fixture-only"}
	newRuntime := func() *Runtime {
		return New(Dependencies{BinaryProvider: fakeBinaryProvider{path: binary}, AgentHome: func(string) (string, error) { return filepath.Join(root, "agent-minimal"), nil }, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: "agent-minimal", Name: "minimal", RuntimeID: "rt-agent-minimal", Profile: profile, RuntimeOptions: map[string]any{"memory_mode": "disabled"}}, nil
		}})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	runtime := newRuntime()
	defer func() { runtime.Close() }()
	handle, err := runtime.New(ctx, agentruntime.Spec{RuntimeID: "rt-agent-minimal", AgentID: "agent-minimal", AgentName: "minimal", Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := runtime.EnsureEngineSession(ctx, handle.RuntimeID, "room:minimal")
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Prompt(ctx, handle.RuntimeID, thread, "Reply only OK."); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	runtime = newRuntime()
	resumed, err := runtime.EnsureEngineSession(ctx, handle.RuntimeID, "room:minimal")
	if err != nil || resumed != thread {
		t.Fatalf("cold resume: %q %v", resumed, err)
	}
	if err := runtime.Prompt(ctx, handle.RuntimeID, resumed, "Reply only OK again."); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("successful native requests = %d", requests.Load())
	}
}
