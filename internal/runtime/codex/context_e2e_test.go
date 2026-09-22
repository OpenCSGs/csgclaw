package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/modelcap"
	agentruntime "csgclaw/internal/runtime"
)

// Exercises the bundled process, real Responses transport and event subscription.
func TestContextUsageBundledCodexE2E(t *testing.T) {
	for _, mode := range []string{"usage", "auto_compact", "overflow", "cold_resume", "luna_history", "auto_compact_failure"} {
		t.Run(mode, func(t *testing.T) { testContextBundledCodex(t, mode) })
	}
}
func testContextBundledCodex(t *testing.T, mode string) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY")
	}
	binary, _ = filepath.Abs(binary)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, "host"))
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		n := requests.Add(1)
		if mode == "auto_compact_failure" && n >= 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":{"code":"invalid_request_error","message":"summary rejected"}}`)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		input, _ := json.Marshal(body["input"])
		if strings.Count(string(input), "CSG_CONTEXT_SENTINEL") > 1 {
			t.Errorf("user input duplicated in request: %s", input)
		}
		if mode == "overflow" && n == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":{"code":"context_length_exceeded","type":"invalid_request_error","message":"This model context window has been exceeded"}}`)
			return
		}
		tokens := 1000
		if (mode == "auto_compact" || mode == "auto_compact_failure") && n == 1 {
			tokens = 26000
		}
		if mode == "luna_history" {
			tokens = 82594
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(v any) { b, _ := json.Marshal(v); fmt.Fprintf(w, "data: %s\n\n", b) }
		emit(map[string]any{"type": "response.created", "response": map[string]any{"id": "r1"}})
		emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": "m1", "content": []any{map[string]any{"type": "output_text", "text": "done"}}}})
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": "r1", "usage": map[string]any{"input_tokens": tokens, "output_tokens": 20, "total_tokens": tokens + 20}}})
	}))
	defer server.Close()
	profile := testAgentMCPProfile()
	profile.Provider = "api"
	profile.ModelMetadata = modelcap.Resolved{ContextWindow: 32768, ContextSource: "user"}
	profile.ModelID = "fixture-model"
	if mode == "luna_history" {
		profile.Provider = "codex"
		profile.ModelID = "gpt-5.6-luna"
		profile.ModelMetadata = modelcap.Resolve("codex", "", profile.ModelID, modelcap.Metadata{}, modelcap.Metadata{})
	}
	profile.BaseURL = server.URL + "/v1"
	delete(profile.Env, "CSGCLAW_BASE_URL")
	rt := New(Dependencies{EventSink: NewEventSink(), BinaryProvider: fakeBinaryProvider{path: binary}, AgentHome: func(string) (string, error) { return filepath.Join(home, "agent"), nil }, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "alice", Name: "alice", Profile: profile, RuntimeOptions: map[string]any{"memory_mode": "disabled"}}, nil
	}})
	defer rt.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-alice", AgentID: "alice", AgentName: "alice", Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := rt.EnsureEngineSession(ctx, h.RuntimeID, "room:test")
	if err != nil {
		t.Fatal(err)
	}

	events, unsubscribe := rt.SubscribeSession(h.RuntimeID, thread)
	defer unsubscribe()
	if err := rt.Prompt(ctx, h.RuntimeID, thread, "CSG_CONTEXT_SENTINEL Reply done."); err != nil {
		t.Fatal(err)
	}
	if mode == "cold_resume" {
		manager := rt.SessionManager().(*appServerManager)
		live := manager.liveSession(h.RuntimeID)
		params := appServerThreadResumeParams(live.spec, thread)
		params["config"] = map[string]any{"model_context_window": 1000000, "model_auto_compact_token_limit": 900000}
		if _, err := live.appClient.request(ctx, "thread/resume", params); err != nil {
			t.Fatal(err)
		}
	}
	if mode == "cold_resume" {
		if _, err := rt.Stop(ctx, h); err != nil {
			t.Fatal(err)
		}
		profile.ModelMetadata = modelcap.Resolved{ContextWindow: 65536, ContextSource: "user"}
		resumed, err := rt.EnsureEngineSession(ctx, h.RuntimeID, "room:test")
		if err != nil || resumed != thread {
			t.Fatalf("cold resume %q %v", resumed, err)
		}
		if err := rt.Prompt(ctx, h.RuntimeID, thread, "Continue after restart."); err != nil {
			t.Fatal(err)
		}
	}
	if mode == "auto_compact_failure" {
		err := rt.Prompt(ctx, h.RuntimeID, thread, "Continue after compaction.")
		if !errors.Is(err, errContextCompaction) {
			t.Fatalf("expected compaction failure, got %v", err)
		}
		return
	}
	if mode == "auto_compact" || mode == "luna_history" {
		if err := rt.Prompt(ctx, h.RuntimeID, thread, "Continue after compaction."); err != nil {
			t.Fatal(err)
		}
	}
	completed := 0
	target := 1
	if mode == "auto_compact" || mode == "cold_resume" || mode == "luna_history" {
		target = 2
	}
	var latest modelcap.ContextUsage
	sawCompaction := false
	for completed < target {
		select {
		case e, ok := <-events:
			if !ok {
				t.Fatal("event stream closed")
			}
			if string(e.Kind) == "context_usage" {
				latest = e.Payload.(modelcap.ContextUsage)
				sawCompaction = sawCompaction || latest.Compacting
			}
			if string(e.Kind) == "prompt_completed" {
				completed++
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	expectedUsage := int64(1020)
	if mode == "luna_history" {
		expectedUsage = 82614
	}
	if latest.UsedTokens == nil || *latest.UsedTokens != expectedUsage || latest.Compacting {
		t.Fatalf("final usage=%+v", latest)
	}
	if (mode == "auto_compact" || mode == "overflow") && (!sawCompaction || requests.Load() < 3) {
		t.Fatalf("compaction=%v requests=%d", sawCompaction, requests.Load())
	}
	if mode == "cold_resume" && latest.ContextWindow < 40000 {
		t.Fatalf("capacity not refreshed: %+v", latest)
	}
	if mode == "luna_history" && (sawCompaction || requests.Load() != 2 || latest.ContextWindow < 900000 || latest.CompactThreshold < 700000) {
		t.Fatalf("premature compaction at low usage: %+v, calls=%d", latest, requests.Load())
	}
	t.Logf("requests=%d compaction=%v used=%d capacity=%d", requests.Load(), sawCompaction, *latest.UsedTokens, latest.ContextWindow)
}
