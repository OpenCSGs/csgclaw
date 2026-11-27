package llm

import (
	"context"
	"errors"
	"io"
	"net"
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
	"csgclaw/internal/modelprovider"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestGatewayRequestFeedback(t *testing.T) {
	const traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	for _, scenario := range []struct {
		name, category, source string
		attempts               int32
	}{
		{"rate-limit", "rate_limit", "gateway", 5},
		{"connection", "connection", "client", 5},
		{"timeout", "timeout", "client", 1},
		{"stream-idle", "timeout", "gateway", 1},
		{"truncated", "stream_interrupted", "gateway", 1},
		{"model-rejected", "rejected", "model", 1},
		{"upstream-error", "server", "upstream", 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			carrier := propagation.MapCarrier{"traceparent": traceparent, "tracestate": "qa=burst"}
			parentCtx := propagation.TraceContext{}.Extract(t.Context(), carrier)
			parent := trace.SpanContextFromContext(parentCtx)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				child := trace.SpanContextFromContext(propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header)))
				if !child.IsValid() || child.TraceID() != parent.TraceID() || child.SpanID() == parent.SpanID() || child.TraceState().String() != "qa=burst" || r.Header.Get("X-Request-ID") != "" {
					t.Errorf("invalid outgoing trace context: %+v", child)
				}
				w.Header().Set("X-Request-ID", "gateway-fixture")
				switch scenario.name {
				case "rate-limit":
					w.Header().Set("Retry-After", "0")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = io.WriteString(w, `{"error":{"code":"user_rate_limit_exceeded","message":"too many requests"}}`)
				case "timeout":
					select {
					case <-r.Context().Done():
					case <-time.After(500 * time.Millisecond):
					}
				case "truncated":
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
				case "stream-idle":
					w.Header().Set("Content-Type", "text/event-stream")
					for range 20 {
						_, _ = io.WriteString(w, ": heartbeat\n\n")
						w.(http.Flusher).Flush()
						select {
						case <-r.Context().Done():
							return
						case <-time.After(15 * time.Millisecond):
						}
					}
				case "model-rejected":
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, `{"error":{"code":"unsupported_feature","message":"fixture-key rejected","param":"tools"}}`)
				case "upstream-error":
					w.WriteHeader(http.StatusBadGateway)
					_, _ = io.WriteString(w, `{"error":{"code":"upstream_response_invalid","message":"invalid response"}}`)
				}
			}))
			t.Cleanup(upstream.Close)
			svc := NewService(config.ModelConfig{}, nil)
			svc.modelHeaderTimeout = 100 * time.Millisecond
			svc.modelIdleTimeout = 100 * time.Millisecond
			if scenario.name == "connection" {
				svc.gatewayClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
					calls.Add(1)
					return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
				})
			}
			profile := agent.AgentProfile{Provider: agent.ProviderAPI, ModelProviderID: agent.ModelProviderIDOpenCSG, APIProtocol: "responses", BaseURL: upstream.URL + "/v1", APIKey: "fixture-key", ModelID: "qwen3.7-plus", ProfileComplete: true}
			record := diagnostics.New("").Begin("room", "source", "", "agent", "turn")
			record.Running()
			var statuses []diagnostics.ModelRequestStatus
			record.ObserveModelRequest(func(status diagnostics.ModelRequestStatus) { statuses = append(statuses, status) })
			ctx := WithTaskProfile(diagnostics.WithRecord(parentCtx, record), profile)
			resp, _ := svc.Responses(ctx, "agent", []byte(`{"input":"hello","stream":true}`))
			if resp != nil {
				_, _ = io.ReadAll(resp.Body)
				_ = resp.Body.Close()
			}
			if calls.Load() != scenario.attempts || len(statuses) != int(scenario.attempts) {
				t.Fatalf("attempts=%d, published=%+v", calls.Load(), statuses)
			}
			for index, status := range statuses[:len(statuses)-1] {
				if status.State != "retrying" || status.Attempt != index+2 || status.MaxAttempts != 5 || status.RetryAt == nil || status.RequestID != statuses[len(statuses)-1].RequestID {
					t.Fatalf("retry status %+v", status)
				}
			}
			failure := statuses[len(statuses)-1]
			if failure.State != "failed" || failure.Category != scenario.category || failure.Source != scenario.source || failure.RequestID == "" || strings.Contains(failure.Details, "fixture-key") {
				t.Fatalf("unexpected failure %+v", failure)
			}
		})
	}
}

func TestGatewayStreamRecoveryWaitsForTerminalEvent(t *testing.T) {
	const delta = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
	for _, test := range []struct {
		name, terminal, want string
	}{
		{"completed", "data: {\"type\":\"response.completed\"}\n\n", "recovered"},
		{"failed", "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"unsupported_feature\"}}}\n\n", "failed"},
		{"disconnected", "", "failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			finish := make(chan struct{})
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.Header().Set("X-Request-ID", "original-gateway-request")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = io.WriteString(w, `{"error":{"code":"rate_limit_exceeded"}}`)
					return
				}
				w.Header().Set("X-Request-ID", "retried-gateway-request")
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, delta)
				w.(http.Flusher).Flush()
				select {
				case <-finish:
					_, _ = io.WriteString(w, test.terminal)
				case <-r.Context().Done():
				}
			}))
			t.Cleanup(gateway.Close)
			record := diagnostics.New("").Begin("room", "source", "", "agent", "turn")
			record.Running()
			updates := make(chan diagnostics.ModelRequestStatus, 4)
			record.ObserveModelRequest(func(status diagnostics.ModelRequestStatus) { updates <- status })
			ctx := WithTaskProfile(diagnostics.WithRecord(t.Context(), record), openCSGTestProfile(gateway.URL))
			response, err := NewService(config.ModelConfig{}, nil).Responses(ctx, "agent", []byte(`{"input":"test","stream":true}`))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			select {
			case retry := <-updates:
				if retry.State != "retrying" || retry.GatewayRequestID != "original-gateway-request" {
					t.Fatalf("unexpected retry feedback: %+v", retry)
				}
			default:
				t.Fatal("expected retry feedback")
			}
			if _, err := io.ReadFull(response.Body, make([]byte, len(delta))); err != nil {
				t.Fatal(err)
			}
			select {
			case update := <-updates:
				t.Fatalf("premature feedback before the terminal event: %+v", update)
			default:
			}
			close(finish)
			_, _ = io.Copy(io.Discard, response.Body)
			select {
			case update := <-updates:
				if update.State != test.want || update.GatewayRequestID != "retried-gateway-request" {
					t.Fatalf("unexpected terminal feedback: %+v", update)
				}
			default:
				t.Fatal("stream ended without terminal feedback")
			}
		})
	}
}

func TestGatewayErrorDetailsStayInDiagnostics(t *testing.T) {
	const private = "quoted private task text"
	const providerError = `{"code":"unsupported_feature","message":"rejected ` + private + `","param":"tools"}`
	for _, test := range []struct {
		name        string
		status      int
		contentType string
		body        string
	}{
		{"HTTP", http.StatusBadRequest, "application/json", `{"error":` + providerError + `}`},
		{"event stream", http.StatusOK, "text/event-stream", "data: " + `{"type":"response.failed","response":{"error":` + providerError + "}}\n\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", test.contentType)
				w.Header().Set("X-Request-ID", "gateway-fixture")
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer gateway.Close()
			store := diagnostics.New(t.TempDir())
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := store.Flush(ctx); err != nil {
					t.Error(err)
				}
			})
			record := store.Begin("room", "source", "", "agent", "turn")
			record.Running()
			var feedback []diagnostics.ModelRequestStatus
			record.ObserveModelRequest(func(status diagnostics.ModelRequestStatus) { feedback = append(feedback, status) })
			ctx := WithTaskProfile(diagnostics.WithRecord(t.Context(), record), openCSGTestProfile(gateway.URL))
			resp, err := NewService(config.ModelConfig{}, nil).Responses(ctx, "agent", []byte(`{"input":"test","stream":true}`))
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || strings.Contains(string(body), private) {
				t.Fatalf("unexpected runtime response: %s, %v", body, err)
			}
			want := modelprovider.FriendlyUpstreamErrorMessage(http.StatusBadRequest, "unsupported_feature")
			if len(feedback) != 1 || feedback[0].Details != want || feedback[0].GatewayRequestID != "gateway-fixture" {
				t.Errorf("transcript feedback did not use the friendly error message: %+v", feedback)
			}
			captured := false
			err = store.ReadContext(t.Context(), record.Snapshot().ID, func(entry diagnostics.ContextEntry) error {
				if entry.Kind == "model.upstream_response" && strings.Contains(string(entry.Data), private) {
					captured = true
				}
				return nil
			})
			if err != nil || !captured {
				t.Fatalf("provider error was lost from diagnostics: %v", err)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

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
