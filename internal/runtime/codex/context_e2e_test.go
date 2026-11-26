package codex

import (
	"context"
	"csgclaw/internal/diagnostics"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelcap"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/turnjournal"
)

// Exercises the bundled process, real Responses transport and event subscription.
func TestContextUsageBundledCodexE2E(t *testing.T) {
	for _, mode := range []string{"diagnostics", "usage", "unsent_cold_resume", "missing_used_rollout", "native_message_identity", "manual_compact", "auto_compact", "overflow", "cold_resume", "default_cold_resume", "luna_history", "sol61_history", "sol61_auto_compact", "auto_compact_failure", "output_budget"} {
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
	diagnosticStore := diagnostics.New("")
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		n := requests.Add(1)
		if mode == "diagnostics" {
			var meta struct {
				Session string `json:"thread_id"`
				Turn    string `json:"turn_id"`
			}
			if json.Unmarshal([]byte(r.Header.Get("X-Codex-Turn-Metadata")), &meta) != nil || meta.Session == "" || meta.Turn == "" {
				t.Error("native model request lacks exact turn correlation")
			}
			update, finish := diagnostics.ObserveRequest(diagnostics.WithNativeLookup(r.Context(), diagnosticStore, "alice", meta.Session, meta.Turn), "responses")
			defer finish("completed")
			update(200, nil)
			time.Sleep(15 * time.Millisecond)
		}

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
		if mode == "output_budget" {
			for _, key := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
				if value, ok := body[key]; ok {
					t.Errorf("Codex request unexpectedly imposed %s=%v", key, value)
				}
			}
		}
		if mode == "diagnostics" {
			items, _ := body["input"].([]any)
			for _, item := range items {
				m, _ := item.(map[string]any)
				if m["type"] == "function_call_output" {
					t.Logf("native tool result: %v", m["output"])
				}
			}
		}
		input, _ := json.Marshal(body["input"])
		if mode == "missing_used_rollout" && n == 2 && strings.Contains(string(input), "CSG_CONTEXT_SENTINEL") {
			t.Error("replacement thread included input from the missing rollout")
		}
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
		if mode == "sol61_history" {
			tokens = 370000
		}
		if mode == "sol61_auto_compact" && n == 1 {
			tokens = 380000
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(v any) { b, _ := json.Marshal(v); fmt.Fprintf(w, "data: %s\n\n", b) }
		emit(map[string]any{"type": "response.created", "response": map[string]any{"id": "r1"}})
		if mode == "diagnostics" && n == 1 {
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "id": "tool1", "call_id": "tool1", "name": "exec_command", "arguments": `{"cmd":"printf native-diagnostic-fixture","yield_time_ms":1000}`}})
		} else {
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": "m1", "content": []any{map[string]any{"type": "output_text", "text": "done"}}}})
		}
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": "r1", "usage": map[string]any{"input_tokens": tokens, "output_tokens": 20, "total_tokens": tokens + 20}}})
	}))
	defer server.Close()
	profile := testAgentMCPProfile()
	profile.Provider = "api"
	profile.ModelMetadata = modelcap.Resolved{ContextWindow: 32768, ContextSource: "user"}
	profile.ModelID = "fixture-model"
	if mode == "output_budget" {
		profile.ModelID = "deepseek-v4.1-flash"
		profile.ModelMetadata = modelcap.Resolve("api", "", profile.ModelID, modelcap.Metadata{}, modelcap.Metadata{})
	}
	if mode == "luna_history" || mode == "sol61_auto_compact" || mode == "sol61_history" {
		profile.Provider = "codex"
		profile.ModelID = "gpt-5.6-luna"
		if mode == "sol61_auto_compact" || mode == "sol61_history" {
			profile.ModelID = "gpt-6.1-sol"
		}
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
	coldResume := mode == "cold_resume" || mode == "default_cold_resume"
	conversationBinding := "room:test"
	if mode == "manual_compact" {
		conversationBinding = "csgclaw-memory:managed-v2-fixture:room:test"
	}
	var thread string
	if mode == "default_cold_resume" {
		conversationBinding = ""
	} else {
		thread, err = rt.EnsureEngineSession(ctx, h.RuntimeID, conversationBinding)
		if err != nil {
			t.Fatal(err)
		}
	}
	if mode == "unsent_cold_resume" {
		fresh := thread
		if _, err := rt.Stop(ctx, h); err != nil {
			t.Fatal(err)
		}
		thread, err = rt.EnsureEngineSession(ctx, h.RuntimeID, conversationBinding)
		if err != nil || thread == "" {
			t.Fatalf("restart after saving unused thread %q: %q %v", fresh, thread, err)
		}
		t.Logf("unused native binding before restart %q, after first use %q", fresh, thread)
	}
	if mode == "sol61_auto_compact" || mode == "sol61_history" {
		live := rt.SessionManager().(*appServerManager).liveSession(h.RuntimeID)
		catalog, err := os.ReadFile(filepath.Join(live.spec.CodexHomeDir, modelCatalogFileName))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`"context_window": 500000`, `"auto_compact_token_limit": 375000`} {
			if !strings.Contains(string(catalog), want) {
				t.Fatalf("generated Codex catalog missing %q", want)
			}
		}
	}

	events, unsubscribe := rt.SubscribeSession(h.RuntimeID, thread)
	defer unsubscribe()
	var diagnostic *diagnostics.Record
	if mode == "diagnostics" {
		store := diagnosticStore
		store.Source("room", "source", time.Now())
		diagnostic = store.Begin("room", "source", "", "alice", "turn")
		diagnostic.Running()
		ctx = diagnostics.WithRecord(ctx, diagnostic)
	}
	firstMessage := PromptRequest{SessionID: thread, Prompt: []PromptContentBlock{TextBlock("CSG_CONTEXT_SENTINEL Reply done.")}}
	if coldResume {
		firstMessage.ConversationKey = conversationBinding
		firstMessage.ClientUserMessageID = "retry-original-message"
	}
	if mode == "native_message_identity" {
		firstMessage.ClientUserMessageID = "native-duplicate"
		firstMessage.ConversationKey = conversationBinding
		firstMessage.Prompt = []PromptContentBlock{TextBlock("Reply to the native ID test.")}
	}
	if err := rt.PromptTurn(ctx, h.RuntimeID, firstMessage); err != nil {
		t.Fatal(err)
	}
	if mode == "missing_used_rollout" {
		manager := rt.SessionManager().(*appServerManager)
		dir := manager.liveSession(h.RuntimeID).spec.CodexHomeDir
		if _, err := rt.Stop(ctx, h); err != nil {
			t.Fatal(err)
		}
		files, err := filepath.Glob(filepath.Join(dir, "sessions", "*", "*", "*", "rollout-*-"+thread+".jsonl"))
		if err != nil || len(files) != 1 {
			t.Fatalf("rollouts for %q: %v %v", thread, files, err)
		}
		if err := os.Rename(files[0], filepath.Join(home, "withheld-rollout.jsonl")); err != nil {
			t.Fatal(err)
		}
		key := contract.ConversationKey(conversationBinding)
		result := rt.Conversation(h.RuntimeID).Run(ctx, contract.TurnRequest{ID: "next-message", ConversationKey: key, Input: []contract.InputPart{{Kind: contract.InputPartText, Text: "Continue in the new thread."}}}, nil)
		if result.Status != contract.TurnSucceeded || result.Output != "done" || requests.Load() != 2 {
			t.Fatalf("send after missing rollout: %+v, calls=%d", result, requests.Load())
		}
		meta, err := rt.readSessionMetadata(h.RuntimeID)
		if id := meta.ConversationSessions[conversationBinding]; err != nil || id == "" || id == thread {
			t.Fatalf("replacement binding = %q; old=%q err=%v", id, thread, err)
		}
		return
	}
	if mode == "native_message_identity" {
		manager := rt.SessionManager().(*appServerManager)
		live := manager.liveSession(h.RuntimeID)
		original, err := turnjournal.ConfirmedTurn(live.spec.RuntimeDir, conversationBinding, firstMessage.ClientUserMessageID)
		if err != nil || original == "" {
			t.Fatalf("first native turn: %q %v", original, err)
		}
		input, err := appServerPromptInput(firstMessage)
		if err != nil {
			t.Fatal(err)
		}
		// Probe the native runtime directly with the identical user input and ID.
		raw, err := live.appClient.request(ctx, "turn/start", appServerTurnStartParamsForContext(ctx, live.spec, thread, input, firstMessage.ClientUserMessageID))
		duplicate := appServerTurnIDFromResult(raw)
		if err != nil || duplicate == "" || duplicate == original {
			t.Fatalf("native duplicate: first=%q second=%q err=%v", original, duplicate, err)
		}
		waitForRuntime(t, func() bool { return requests.Load() == 2 })
		waitForRuntime(t, func() bool {
			raw, err := live.appClient.request(ctx, "thread/read", map[string]any{"threadId": thread, "includeTurns": true})
			var history struct {
				Thread struct {
					Turns []struct {
						ID, Status string
						Items      []struct{ Type, ClientID string }
					}
				}
			}
			if err != nil || json.Unmarshal(raw, &history) != nil {
				return false
			}
			matches := map[string]bool{}
			for _, turn := range history.Thread.Turns {
				for _, item := range turn.Items {
					if turn.Status == "completed" && item.Type == "userMessage" && item.ClientID == firstMessage.ClientUserMessageID {
						matches[turn.ID] = true
					}
				}
			}
			return matches[original] && matches[duplicate]
		})
		if err := rt.PromptTurn(ctx, h.RuntimeID, firstMessage); !errors.Is(err, turnjournal.ErrUnconfirmed) {
			t.Fatalf("conflicting historical matches: %v", err)
		}
		if requests.Load() != 2 {
			t.Fatalf("reconciliation sent another request")
		}
		return
	}
	if diagnostic != nil {
		time.Sleep(20 * time.Millisecond)
		diagnostic.Finish("succeeded")
		v := diagnostic.Snapshot()
		calls := 0
		for _, span := range v.Spans {
			if span.Owner == "llm" {
				calls++
			}
		}
		if calls != 2 {
			t.Fatalf("native request did not correlate: %+v", v)
		}
		if v.RuntimeStartMS == nil || v.RuntimeEndMS == nil || v.TotalMS-*v.RuntimeEndMS < 20 || v.Runtime != "codex" {
			t.Fatalf("native Codex diagnostic boundary: %+v", v)
		}
	}
	if mode == "diagnostics" {
		deadline := time.Now().Add(8 * time.Second)
		found := map[string]bool{}
		for time.Now().Before(deadline) {
			for _, span := range diagnostic.Snapshot().Spans {
				if span.Details != nil && span.Details.Source == "codex_otel" {
					found[span.Details.Label] = true
				}
			}
			if found["session_task.turn"] && found["model_client.stream_responses_api"] && found["drain_in_flight"] {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !found["session_task.turn"] || !found["model_client.stream_responses_api"] || !found["drain_in_flight"] {
			t.Fatalf("missing native turn/model/tool phases: %v", found)
		}
		var bridgeCalls, nativeRequests, nativeStreams []diagnostics.Span
		for _, span := range diagnostic.Snapshot().Spans {
			if span.Owner == "llm" {
				bridgeCalls = append(bridgeCalls, span)
			}
			if span.Details != nil && span.Details.Source == "codex_otel" {
				if span.Details.Label == "model_client.stream_responses_api" {
					nativeRequests = append(nativeRequests, span)
				}
				if span.Details.Label == "receiving_stream" {
					nativeStreams = append(nativeStreams, span)
				}
			}
		}
		for _, spans := range [][]diagnostics.Span{bridgeCalls, nativeRequests, nativeStreams} {
			sort.Slice(spans, func(i, j int) bool { return spans[i].StartMS < spans[j].StartMS })
		}
		if len(nativeRequests) != 2 || len(nativeStreams) != 2 {
			t.Fatalf("native/bridge request count mismatch: requests=%d streams=%d", len(nativeRequests), len(nativeStreams))
		}
		for i, call := range bridgeCalls {
			// Allow millisecond wall-clock precision, but verify the separately clocked
			// bridge interval lies inside native request establishment + stream receipt.
			if nativeRequests[i].StartMS > call.StartMS+5 || *nativeStreams[i].EndMS < *call.EndMS-5 {
				t.Fatalf("native/bridge timing mismatch: bridge=%+v native=%+v stream=%+v", call, nativeRequests[i], nativeStreams[i])
			}
		}

	}
	if mode == "default_cold_resume" {
		meta, err := rt.readSessionMetadata(h.RuntimeID)
		if err != nil || meta.SessionID == "" {
			t.Fatalf("default ID after first message = %q, %v", meta.SessionID, err)
		}
		thread = meta.SessionID
	}
	if coldResume {
		manager := rt.SessionManager().(*appServerManager)
		live := manager.liveSession(h.RuntimeID)
		params := appServerThreadResumeParams(live.spec, thread)
		params["config"] = map[string]any{"model_context_window": 1000000, "model_auto_compact_token_limit": 900000}
		if _, err := live.appClient.request(ctx, "thread/resume", params); err != nil {
			t.Fatal(err)
		}
		if _, err := rt.Stop(ctx, h); err != nil {
			t.Fatal(err)
		}
		profile.ModelMetadata = modelcap.Resolved{ContextWindow: 65536, ContextSource: "user"}
		resumed, err := rt.EnsureEngineSession(ctx, h.RuntimeID, conversationBinding)
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
	if mode == "auto_compact" || mode == "luna_history" || mode == "sol61_auto_compact" || mode == "sol61_history" {
		if err := rt.Prompt(ctx, h.RuntimeID, thread, "Continue after compaction."); err != nil {
			t.Fatal(err)
		}
	}
	completed := 0
	successfulCommand := false
	target := 1
	if mode == "auto_compact" || coldResume || mode == "luna_history" || mode == "sol61_auto_compact" || mode == "sol61_history" {
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
			if mode == "diagnostics" && e.Kind == SessionEventToolCallUpdate && e.ToolKind == "exec_command" {
				t.Logf("native shell status=%s output=%s", e.ToolStatus, e.ToolOutputSummary)
			}
			if e.Kind == SessionEventToolCallUpdate && e.ToolKind == "exec_command" && e.ToolStatus == "completed" {
				successfulCommand = true
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
	if mode == "diagnostics" && !successfulCommand {
		t.Fatal("native shell command did not complete")
	}
	expectedUsage := int64(1020)
	if mode == "luna_history" {
		expectedUsage = 82614
	}
	if mode == "sol61_history" {
		expectedUsage = 370020
	}
	if latest.UsedTokens == nil || *latest.UsedTokens != expectedUsage || latest.Compacting {
		t.Fatalf("final usage=%+v", latest)
	}
	if (mode == "auto_compact" || mode == "sol61_auto_compact" || mode == "overflow") && (!sawCompaction || requests.Load() < 3) {
		t.Fatalf("compaction=%v requests=%d", sawCompaction, requests.Load())
	}
	if coldResume {
		before := requests.Load()
		firstMessage.SessionID = thread
		replay, err := rt.SessionManager().Prompt(ctx, SessionHandle{RuntimeID: h.RuntimeID}, firstMessage)
		if err != nil || len(replay.history) == 0 || replay.history[len(replay.history)-1].Text != "done" || requests.Load() != before {
			t.Fatalf("replay after restart = %+v, error %v, model calls %d -> %d", replay, err, before, requests.Load())
		}
	}
	if coldResume && latest.ContextWindow < 40000 {
		t.Fatalf("capacity not refreshed: %+v", latest)
	}
	if mode == "luna_history" && (sawCompaction || requests.Load() != 2 || latest.ContextWindow < 900000 || latest.CompactThreshold < 700000) {
		t.Fatalf("premature compaction at low usage: %+v, calls=%d", latest, requests.Load())
	}
	if (mode == "sol61_auto_compact" || mode == "sol61_history") && (latest.ContextWindow != 500000 || latest.CompactThreshold != 375000 || latest.ContextSource != "catalog") {
		t.Fatalf("GPT-6.1 Sol context settings differ from Codex client defaults: %+v", latest)
	}
	if mode == "sol61_history" && (sawCompaction || requests.Load() != 2) {
		t.Fatalf("premature GPT-6.1 Sol compaction at 370k usage: %+v, calls=%d", latest, requests.Load())
	}

	if mode == "manual_compact" {
		calls := requests.Load()
		result, err := rt.Conversation(h.RuntimeID).(contract.ConversationCompactor).Compact(ctx, contract.CompactRequest{ConversationKey: "room:test", Usage: latest})
		if err != nil || result.Status != "completed" {
			t.Fatalf("manual compaction: %+v %v", result, err)
		}
		if result.Usage == nil || result.Usage.UsedTokens == nil || !result.Usage.Estimated || result.Usage.Compacting {
			t.Fatalf("manual compaction lost native occupancy: %+v", result.Usage)
		}
		if requests.Load() != calls+1 {
			t.Fatalf("manual compaction accidentally started a normal turn: %d -> %d", calls, requests.Load())
		}
		if err := rt.Prompt(ctx, h.RuntimeID, thread, "Continue after manual compaction."); err != nil {
			t.Fatal(err)
		}
		saved, ok, err := rt.ExistingEngineSession(ctx, h.RuntimeID, conversationBinding)
		if err != nil || !ok || saved != thread {
			t.Fatalf("manual compaction changed session: %s %v", saved, err)
		}
	}
	t.Logf("requests=%d compaction=%v used=%d capacity=%d", requests.Load(), sawCompaction, *latest.UsedTokens, latest.ContextWindow)
}
