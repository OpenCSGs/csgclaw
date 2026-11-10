package api

import (
	"context"
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

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"csgclaw/internal/llm"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/codex"
)

func TestNativeCodexOversizedModelRequestStopsWithoutRetry(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY")
	}
	for _, tc := range []struct {
		name   string
		reject http.HandlerFunc
	}{
		{"http-413", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			fmt.Fprint(w, `{"code":"BadRequest.TooLarge","message":"Exceeded limit on max bytes to request body : 67108864"}`)
		}},
		{"truncated-413", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "512")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			fmt.Fprint(w, `{"code":"BadRequest.TooLarge"`)
		}},
		{"initial-sse-error", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, ": keepalive\n\nevent: error\ndata: {\"type\":\"error\",\"code\":\"request_too_large\",\"message\":\"Provider details\"}\n\n")
		}},
		{"sse-error-after-created", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_rejected\"}}\n\n")
			fmt.Fprint(w, "data: {\"type\":\"error\",\"code\":\"request_too_large\"}\n\n")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testNativeCodexOversizedModelRequest(t, binary, tc.reject)
		})
	}
}

func testNativeCodexOversizedModelRequest(t *testing.T, binary string, reject http.HandlerFunc) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, "host"))

	var calls atomic.Int64
	var oversized atomic.Bool
	oversized.Store(true)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n > 1 && oversized.Load() {
			reject(w, r)
			return
		}
		if n == 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(event map[string]any) {
			raw, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", raw)
		}
		id := fmt.Sprintf("resp_%d", n)
		emit(map[string]any{"type": "response.created", "response": map[string]any{"id": id}})
		if n == 1 {
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "id": "tool_1", "call_id": "call_1", "name": "exec_command", "arguments": `{"cmd":"true"}`}})
		} else {
			emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": "msg_done", "content": []any{map[string]any{"type": "output_text", "text": "Done."}}}})
		}
		emit(map[string]any{"type": "response.completed", "response": map[string]any{"id": id, "status": "completed"}})
	}))
	defer upstream.Close()

	h := &Handler{
		llm: llm.NewService(config.ModelConfig{}, usageProfile{agent.AgentProfile{
			Provider: agent.ProviderAPI, ModelID: "fixture-model", BaseURL: upstream.URL + "/v1", APIKey: "fixture",
		}}),
		serverAccessToken: "fixture-token",
	}
	var ingressCalls atomic.Int64
	var rejectAtIngress atomic.Bool
	routes := h.Routes()
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ingressCalls.Add(1)
		if rejectAtIngress.Load() {
			// Exercise the ingress Content-Length check without sending a 128 MiB fixture.
			r.ContentLength = maxLLMRequestBodyBytes + 1
		}
		routes.ServeHTTP(w, r)
	}))
	defer bridge.Close()
	profile := agentruntime.Profile{
		Provider: "api", BaseURL: bridge.URL + "/api/v1/agents/dev/llm", APIKey: "fixture-token", ModelID: "fixture-model",
	}
	rt := codex.New(codex.Dependencies{
		BinaryProvider: usageNativeBinary(binary), EventSink: codex.NewEventSink(),
		AgentHome: func(string) (string, error) { return filepath.Join(home, "agent"), nil },
		ResolveAgent: func(agentruntime.Handle) (codex.AgentRef, error) {
			return codex.AgentRef{ID: "dev", Name: "dev", Profile: profile, RuntimeOptions: map[string]any{"memory_mode": "disabled"}}, nil
		},
	})
	defer rt.Close()
	h.svc = usageRuntimeOwner{runtime: rt}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-dev", AgentID: "dev", AgentName: "dev", Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	conversation := rt.Conversation(handle.RuntimeID)
	request := agentengine.TurnRequest{ID: "oversized", ConversationKey: "size-test", Input: []agentengine.InputPart{{Kind: agentengine.InputPartText, Text: "Reply done."}}}
	sink := agentengine.EventSinkFunc(func(context.Context, agentengine.TurnEvent) error { return nil })
	failed := conversation.Run(ctx, request, sink)
	if calls.Load() != 2 || ingressCalls.Load() != 2 || failed.Status != agentengine.TurnFailed || failed.Error == nil ||
		failed.Error.Code != "request_too_large" || !strings.Contains(failed.Error.Message, "start a new conversation") {
		t.Fatalf("oversized upstream request: ingress calls=%d, upstream calls=%d, status=%s, error=%+v", ingressCalls.Load(), calls.Load(), failed.Status, failed.Error)
	}

	rejectAtIngress.Store(true)
	request.ID = "oversized-at-ingress"
	failed = conversation.Run(ctx, request, sink)
	if calls.Load() != 2 || ingressCalls.Load() != 3 || failed.Status != agentengine.TurnFailed || failed.Error == nil || failed.Error.Code != "request_too_large" {
		t.Fatalf("oversized ingress request: ingress calls=%d, upstream calls=%d, status=%s, error=%+v", ingressCalls.Load(), calls.Load(), failed.Status, failed.Error)
	}

	rejectAtIngress.Store(false)
	oversized.Store(false)
	request.ID = "next"
	completed := conversation.Run(ctx, request, sink)
	if calls.Load() != 4 || completed.Status != agentengine.TurnSucceeded {
		t.Fatalf("next turn should recover from a transient error: upstream calls=%d, status=%s, error=%+v", calls.Load(), completed.Status, completed.Error)
	}
}
