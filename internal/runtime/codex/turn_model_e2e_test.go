package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/codexmodel"
	"csgclaw/internal/modelcap"
	agentruntime "csgclaw/internal/runtime"
)

// All inference traffic stays on the local fixture, including compaction.
func TestNativeCodexTurnModelOverridesE2E(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY")
	}
	binary, _ = filepath.Abs(binary)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, "host"))
	var rt *Runtime
	type modelRequest struct {
		model, effort, input, thread string
		override                     bool
	}
	var mu sync.Mutex
	var requests []modelRequest
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Model     string          `json:"model"`
			Input     json.RawMessage `json:"input"`
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		var native struct {
			Thread string `json:"thread_id"`
			Turn   string `json:"turn_id"`
		}
		if err := json.Unmarshal([]byte(r.Header.Get("X-Codex-Turn-Metadata")), &native); err != nil {
			t.Error(err)
		}
		selection, overridden, err := rt.TurnModel("alice", native.Thread, native.Turn, payload.Model)
		if err != nil {
			t.Error(err)
		}
		if overridden && selection.ModelID != payload.Model {
			t.Errorf("bridge model = %q, native model = %q", selection.ModelID, payload.Model)
		}
		mu.Lock()
		requests = append(requests, modelRequest{payload.Model, payload.Reasoning.Effort, string(payload.Input), native.Thread, overridden})
		n := len(requests)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(v any) { raw, _ := json.Marshal(v); _, _ = fmt.Fprintf(w, "data: %s\n\n", raw) }
		emit(map[string]any{"type": "response.created", "response": map[string]any{"id": fmt.Sprint(n)}})
		emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": fmt.Sprint(n), "content": []any{map[string]any{"type": "output_text", "text": "done"}}}})
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": fmt.Sprint(n), "usage": map[string]any{"input_tokens": 100, "output_tokens": 20, "total_tokens": 120}}})
	}))
	defer endpoint.Close()
	profile := agentruntime.Profile{Provider: "api", ModelID: "fixture-a", BaseURL: endpoint.URL + "/v1", APIKey: "fixture", ReasoningEffort: "high", ModelMetadata: modelcap.Resolved{ContextWindow: 32000, ContextSource: "provider"}}
	second := contract.TurnModel{ModelID: "fixture-b", ReasoningEffort: "low", Metadata: modelcap.Resolved{ContextWindow: 64000, ContextSource: "provider"}}
	rt = New(Dependencies{
		EventSink: NewEventSink(), BinaryProvider: fakeBinaryProvider{path: binary},
		AgentHome: func(string) (string, error) { return filepath.Join(home, "agent"), nil },
		ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: "alice", Name: "alice", Profile: profile, RuntimeOptions: map[string]any{"memory_mode": "disabled"}}, nil
		},
		ModelCatalog: func(string) []codexmodel.Profile {
			return []codexmodel.Profile{{Provider: "api", ModelID: second.ModelID, Metadata: second.Metadata, ReasoningEffort: "high"}}
		},
	})
	defer rt.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-alice", AgentID: "alice", AgentName: "alice", Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	firstThread, err := rt.EnsureEngineSession(ctx, handle.RuntimeID, "session:first")
	if err != nil {
		t.Fatal(err)
	}
	otherThread, err := rt.EnsureEngineSession(ctx, handle.RuntimeID, "session:other")
	if err != nil {
		t.Fatal(err)
	}
	live := rt.SessionManager().(*appServerManager).liveSession(handle.RuntimeID)
	pid := live.cmd.Process.Pid
	auto := turnModelFromProfile(profile)
	auto.ReasoningEffort = "auto"
	steps := []struct {
		thread, text, model, effort string
		selection                   *contract.TurnModel
		window                      int64
	}{
		{firstThread, "FIRST_THREAD_SENTINEL", "fixture-a", "high", nil, 32000},
		{firstThread, "Select another model", "fixture-b", "low", &second, 64000},
		{otherThread, "OTHER_THREAD_SENTINEL", "fixture-a", "high", nil, 32000},
		{firstThread, "Select automatic effort", "fixture-a", "", &auto, 32000},
		{firstThread, "Restore defaults", "fixture-a", "high", nil, 32000},
	}
	for index, step := range steps {
		if err := rt.PromptTurn(ctx, handle.RuntimeID, PromptRequest{SessionID: step.thread, Model: step.selection, Prompt: []PromptContentBlock{TextBlock(step.text)}}); err != nil {
			t.Fatalf("step %d: %v", index, err)
		}
		mu.Lock()
		got := requests[len(requests)-1]
		count := len(requests)
		mu.Unlock()
		if count != index+1 || got.model != step.model || got.effort != step.effort || got.thread != step.thread || got.override != (step.selection != nil) {
			t.Fatalf("step %d: request %+v, count %d", index, got, count)
		}
		if step.thread == firstThread && (!strings.Contains(got.input, "FIRST_THREAD_SENTINEL") || strings.Contains(got.input, "OTHER_THREAD_SENTINEL")) {
			t.Fatalf("step %d changed conversation history", index)
		}
		if live.cmd.Process.Pid != pid || !reflect.DeepEqual(live.spec.Profile, profile.Normalized()) {
			t.Fatalf("step %d changed shared runtime", index)
		}
		live.mu.Lock()
		usage := live.contextUsage[step.thread]
		live.mu.Unlock()
		if usage.ModelID != step.model || usage.ContextWindow != step.window {
			t.Fatalf("step %d context = %+v", index, usage)
		}
	}
}
