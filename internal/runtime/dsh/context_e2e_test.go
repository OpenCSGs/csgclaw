package dsh

import (
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/dshcli"
	"csgclaw/internal/modelcap"
	agentruntime "csgclaw/internal/runtime"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"

	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestContextUsageNativeDSHE2E(t *testing.T) {
	for _, mode := range []string{"usage", "auto_compact", "overflow", "disabled", "overflow_failure", "cold_resume"} {
		t.Run(mode, func(t *testing.T) { testContextNativeDSH(t, mode) })
	}
}
func testContextNativeDSH(t *testing.T, mode string) {
	binary := os.Getenv("CSGCLAW_TEST_DSH_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_DSH_BINARY")
	}
	root := t.TempDir()
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		n := requests.Add(1)
		if (mode == "overflow" && n == 2) || (mode == "overflow_failure" && n >= 2) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":{"type":"invalid_request_error","code":"context_length_exceeded","message":"This model maximum context length is 32768 tokens, but this request contains 40000 tokens"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(v any) { b, _ := json.Marshal(v); fmt.Fprintf(w, "data: %s\n\n", b) }
		emit(map[string]any{"id": "r1", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": "done"}, "finish_reason": nil}}})
		emit(map[string]any{"id": "r1", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 1000, "completion_tokens": 20, "total_tokens": 1020}})
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	profile := agentruntime.Profile{BaseURL: server.URL + "/v1", APIKey: "fixture-key", ModelID: "fixture-model", ModelMetadata: modelcap.Resolve("api", server.URL, "fixture-model", modelcap.Metadata{}, modelcap.Metadata{ContextWindow: 32768})}
	if mode == "disabled" {
		disabled := false
		profile.AutoCompact = &disabled
	}
	rt := New(Dependencies{ResolveBinary: func(context.Context, string) (dshcli.Info, error) {
		return dshcli.Info{Path: binary, Version: "0.1.5-rc.2"}, nil
	}, ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "alice", RuntimeID: "rt-alice", Profile: profile}, nil
	}, AgentHome: func(string) (string, error) { return filepath.Join(root, "agent"), nil }})
	defer rt.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := rt.Provision(ctx, agentruntime.ProvisionRequest{RuntimeID: "rt-alice", AgentID: "alice", Profile: profile}); err != nil {
		t.Fatal(err)
	}
	h, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-alice", AgentID: "alice", Profile: profile})
	if err != nil {
		log, _ := os.ReadFile(filepath.Join(root, "agent", hostStateDirName, stderrFileName))
		t.Fatalf("%v\n%s", err, log)
	}

	var snapshot *modelcap.ContextUsage
	sawCompaction := false
	previousSession := ""
	turns := 1
	if mode == "overflow" || mode == "overflow_failure" || mode == "cold_resume" {
		turns = 2
	}
	if mode == "auto_compact" || mode == "disabled" {
		turns = 5
	}
	for i := 0; i < turns; i++ {
		if mode == "cold_resume" && i == 1 {
			previousSession = snapshot.SessionID
			if _, err := rt.Stop(ctx, h); err != nil {
				t.Fatal(err)
			}
			profile.ModelMetadata.ContextWindow = 65536
			profile.ModelMetadata.ContextSource = "user"
			if _, err := rt.Start(ctx, h); err != nil {
				t.Fatal(err)
			}
		}
		text := "Reply done."
		if mode == "auto_compact" || mode == "disabled" {
			text += strings.Repeat("context fixture token ", 1500)
		}
		input := []contract.InputPart{{Kind: contract.InputPartText, Text: text}}
		result := rt.Conversation(h.RuntimeID).Run(ctx, contract.TurnRequest{ID: contract.TurnID(fmt.Sprintf("turn-%d", i)), ConversationKey: "room:test", Input: input}, contract.EventSinkFunc(func(_ context.Context, e contract.TurnEvent) error {
			if e.Activity != nil && e.Activity.Kind == modelcap.ContextUsageKind {
				u := e.Activity.Payload.(modelcap.ContextUsage)
				sawCompaction = sawCompaction || u.Compacting
				if u.UsedTokens != nil {
					snapshot = &u
				}
			}
			return nil
		}))
		if mode == "overflow_failure" && i == 1 {
			if result.Status != contract.TurnFailed || result.Error == nil || result.Error.Code != contract.ErrorCode("context_compaction_failed") {
				t.Fatalf("expected friendly compaction failure: %+v", result)
			}
			t.Logf("bounded recovery failed safely after %d requests", requests.Load())
			return
		}
		if result.Status != contract.TurnSucceeded || snapshot == nil || snapshot.ContextWindow != profile.ModelMetadata.ContextWindow {
			log, _ := os.ReadFile(filepath.Join(root, "agent", hostStateDirName, stderrFileName))
			t.Fatalf("result=%+v usage=%+v\n%s", result, snapshot, log)
		}
	}
	if (mode == "auto_compact" || mode == "overflow") && !sawCompaction {
		t.Fatalf("no compaction lifecycle, calls=%d used=%d", requests.Load(), *snapshot.UsedTokens)
	}
	if mode == "disabled" && (sawCompaction || requests.Load() != int64(turns)) {
		t.Fatalf("compaction not disabled: %v calls=%d", sawCompaction, requests.Load())
	}
	if mode == "cold_resume" && snapshot.SessionID != previousSession {
		t.Fatalf("cold resume changed session: %q -> %q", previousSession, snapshot.SessionID)
	}
	t.Logf("native DSH calls=%d compaction=%v used=%d", requests.Load(), sawCompaction, *snapshot.UsedTokens)
}
