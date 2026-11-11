package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentmemory"
	"csgclaw/internal/config"
	"csgclaw/internal/diagnostics"
)

const tlsGatewayFailure = `{"error":{"code":"internal_server_error","message":"Post https://provider.invalid/responses: utls: TLS handshake: EOF"}}`

func TestChatAndMemoryShareStreamingConnectionRecovery(t *testing.T) {
	for _, operation := range []string{"responses", "chat", "memory"} {
		t.Run(operation, func(t *testing.T) {
			var calls atomic.Int32
			var original string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				count := calls.Add(1)
				if count == 1 {
					original = string(body)
				} else if original != string(body) {
					t.Error("retry changed the model request")
				}
				if !strings.Contains(string(body), `"stream":true`) || r.Header.Get("Authorization") != "Bearer fixture-key" {
					t.Error("shared route changed streaming or authentication")
				}
				if count == 1 {
					w.WriteHeader(500)
					_, _ = io.WriteString(w, tlsGatewayFailure)
					return
				}
				if count == 2 {
					w.Header().Set("Retry-After", "0")
					w.WriteHeader(429)
					_, _ = io.WriteString(w, `{"error":{"code":"model_cooldown","reset_seconds":0}}`)
					return
				}
				writeMemoryTestStream(w, "chat_completions", `{"changes":[{"entry_id":"","kind":"fact","text":"Stable fact","source_message_id":"m1"}]}`)
			}))
			t.Cleanup(upstream.Close)
			profile := agent.AgentProfile{Provider: agent.ProviderAPI, APIProtocol: "chat_completions", BaseURL: upstream.URL + "/v1", APIKey: "fixture-key", ModelID: "fixture-model", ProfileComplete: true}
			ctx := WithTaskProfile(t.Context(), profile)
			record := diagnostics.New("").Begin("room", "message", "", "agent", "turn")
			record.Running()
			record.RuntimeStart()
			ctx = diagnostics.WithRecord(ctx, record)
			svc := NewService(config.ModelConfig{}, nil)
			if operation == "memory" {
				changes, err := svc.ExtractAgentMemory(ctx, "agent", agentmemory.Extraction{Messages: []agentmemory.Message{{Source: agentmemory.Source{RoomID: "private", MessageID: "original-source"}, Text: "Remember this reusable fact"}}})
				if err != nil || len(changes) != 1 || changes[0].SourceMessageID != "original-source" {
					t.Fatalf("memory recovery failed: %+v, %v", changes, err)
				}
			} else {
				var response *UpstreamResponse
				var err error
				if operation == "responses" {
					response, err = svc.Responses(ctx, "agent", []byte(`{"input":"hello","stream":true}`))
				} else {
					response, err = svc.ChatCompletionsStream(ctx, "agent", []byte(`{"messages":[{"role":"user","content":"hello"}],"stream":true}`))
				}
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if err != nil || response.StatusCode != 200 || !strings.Contains(string(body), "Stable fact") {
					t.Fatalf("chat recovery failed: %d, %v", response.StatusCode, err)
				}
			}
			if calls.Load() != 3 {
				t.Fatalf("request attempts=%d", calls.Load())
			}
			if spans := modelSpans(record); len(spans) != 3 || spans[0].Status != "failed" || spans[1].Status != "failed" || spans[2].Status != "completed" {
				t.Fatalf("retry diagnostics lost attempts: %+v", spans)
			}
		})
	}
}

func TestModelConnectionRecoveryIsBoundedAndPreservesErrors(t *testing.T) {
	for _, test := range []struct {
		name     string
		status   int
		body     string
		attempts int
	}{
		{"tls", 500, tlsGatewayFailure, 5},
		{"auth", 401, `{"error":{"message":"expired authentication"}}`, 1},
		{"quota", 429, `{"error":{"code":"insufficient_quota"}}`, 1},
		{"ordinary server error", 500, `{"error":{"message":"generation failed"}}`, 1},
		{"large error", 502, strings.Repeat("large failure ", 4000), 1},
		{"long cooldown", 429, `{"error":{"code":"model_cooldown","last_upstream_error":"utls: TLS handshake: EOF","reset_seconds":999999999999999}}`, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			t.Cleanup(upstream.Close)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, upstream.URL, strings.NewReader(`{"input":"fixture"}`))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := NewService(config.ModelConfig{}, nil).modelRequest(req, true)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil || string(body) != test.body || resp.StatusCode != test.status || calls.Load() != int32(test.attempts) {
				t.Fatalf("final error changed: status=%d calls=%d read=%v", resp.StatusCode, calls.Load(), err)
			}
		})
	}
}

func TestModelRecoveryCancellationAndNonReplayableBody(t *testing.T) {
	for _, replayable := range []bool{true, false} {
		t.Run(map[bool]string{true: "cancel wait", false: "non-replayable body"}[replayable], func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(500)
				_, _ = io.WriteString(w, tlsGatewayFailure)
			}))
			t.Cleanup(upstream.Close)
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			var body io.Reader = strings.NewReader("fixture")
			if !replayable {
				body = io.NopCloser(body)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstream.URL, body)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := NewService(config.ModelConfig{}, nil).modelRequest(req, true)
			if replayable {
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("retry ignored cancellation: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				_ = resp.Body.Close()
			}
			if calls.Load() != 1 {
				t.Fatalf("unsafe or canceled retry sent %d requests", calls.Load())
			}
		})
	}
}

func TestModelRecoveryNeverReplaysAPartiallyReadStream(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Length", "1000")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: partial output\n\n")
	}))
	t.Cleanup(upstream.Close)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, upstream.URL, strings.NewReader("fixture"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := NewService(config.ModelConfig{}, nil).modelRequest(req, true)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err == nil || !strings.Contains(string(body), "partial output") || calls.Load() != 1 {
		t.Fatalf("partial stream replayed or hidden: calls=%d error=%v", calls.Load(), err)
	}
}
