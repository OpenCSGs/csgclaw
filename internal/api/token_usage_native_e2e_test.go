package api

import (
	"context"
	"csgclaw/internal/activity"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"csgclaw/internal/llm"
	"csgclaw/internal/modelusage"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/codex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type usageNativeBinary string

func (b usageNativeBinary) Ensure(context.Context) (string, error) { return string(b), nil }

type usageRuntimeOwner struct {
	AgentRecords
	runtime *codex.Runtime
}

func (s usageRuntimeOwner) Runtime(string) (agentruntime.Runtime, error) { return s.runtime, nil }

type usageProfile struct{ profile agent.AgentProfile }

func (s usageProfile) ResolvedAgentProfile(string) (agent.AgentProfile, error) { return s.profile, nil }

// Actual Codex tool round trips exercise the production HTTP handler, upstream
// observer, native turn identity, cumulative counters and final partial event.
func TestNativeCodexUsageCompletenessThroughBridge(t *testing.T) {
	binary := os.Getenv("CSGCLAW_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set CSGCLAW_TEST_CODEX_BINARY")
	}
	for _, tc := range []struct {
		name     string
		reported [2]bool
	}{
		{"complete", [2]bool{true, true}},
		{"missing-last", [2]bool{true, false}},
		{"missing-first", [2]bool{false, true}},
		{"all-missing", [2]bool{false, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("CODEX_HOME", filepath.Join(home, "host"))
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/responses" {
					http.NotFound(w, r)
					return
				}
				n := calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				emit := func(value any) { raw, _ := json.Marshal(value); fmt.Fprintf(w, "data: %s\n\n", raw) }
				emit(map[string]any{"type": "response.created", "response": map[string]any{"id": fmt.Sprint(n)}})
				if n == 1 {
					emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "id": "tool1", "call_id": "tool1", "name": "exec_command", "arguments": `{"cmd":"true","yield_time_ms":1000}`}})
				} else {
					emit(map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "message", "role": "assistant", "id": fmt.Sprint(n), "content": []any{map[string]any{"type": "output_text", "text": "Done."}}}})
				}
				response := map[string]any{"id": fmt.Sprint(n), "status": "completed"}
				if n > 2 || tc.reported[n-1] {
					response["usage"] = map[string]any{"input_tokens": 100, "output_tokens": 10, "total_tokens": 110}
				}
				emit(map[string]any{"type": "response.completed", "response": response})
			}))
			defer upstream.Close()
			h := &Handler{llm: llm.NewService(config.ModelConfig{}, usageProfile{agent.AgentProfile{Provider: agent.ProviderAPI, ModelID: "fixture-model", BaseURL: upstream.URL + "/v1", APIKey: "fixture"}})}
			bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.handleBotLLMResponses(w, r, "alice") }))
			defer bridge.Close()
			profile := agentruntime.Profile{Provider: "api", BaseURL: bridge.URL + "/v1", APIKey: "local", ModelID: "fixture-model"}
			rt := codex.New(codex.Dependencies{BinaryProvider: usageNativeBinary(binary), EventSink: codex.NewEventSink(), AgentHome: func(string) (string, error) { return filepath.Join(home, "agent"), nil }, ResolveAgent: func(agentruntime.Handle) (codex.AgentRef, error) {
				return codex.AgentRef{ID: "alice", Name: "alice", Profile: profile, RuntimeOptions: map[string]any{"memory_mode": "disabled"}}, nil
			}})
			defer rt.Close()
			h.svc = usageRuntimeOwner{runtime: rt}
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			handle, err := rt.New(ctx, agentruntime.Spec{RuntimeID: "rt-alice", AgentID: "alice", AgentName: "alice", Profile: profile})
			if err != nil {
				t.Fatal(err)
			}
			thread, err := rt.EnsureEngineSession(ctx, handle.RuntimeID, "usage-test")
			if err != nil {
				t.Fatal(err)
			}
			prompt := func() modelusage.Usage {
				events, unsubscribe := rt.SubscribeSession(handle.RuntimeID, thread)
				defer unsubscribe()
				if err := rt.Prompt(ctx, handle.RuntimeID, thread, "Reply done."); err != nil {
					t.Fatal(err)
				}
				var usage modelusage.Usage
				for {
					select {
					case e := <-events:
						if e.Kind == activity.RuntimeEventTokenUsage {
							usage, _ = e.Payload.(modelusage.Usage)
						}
					default:
						return usage
					}
				}
			}
			usage := prompt()
			knownCalls := int64(0)
			for _, known := range tc.reported {
				if known {
					knownCalls++
				}
			}
			if calls.Load() != 2 || usage.Total != 110*knownCalls || usage.Known != (knownCalls > 0) || usage.Partial != (knownCalls != 2) {
				t.Fatalf("calls=%d usage=%+v", calls.Load(), usage)
			}
			// An incomplete previous turn must not mark the next complete turn.
			usage = prompt()
			if calls.Load() != 3 || !usage.Known || usage.Partial || usage.Total != 110 {
				t.Fatalf("next turn calls=%d usage=%+v", calls.Load(), usage)
			}
		})
	}
}
