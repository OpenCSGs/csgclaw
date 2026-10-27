package api

import (
	"bytes"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"csgclaw/internal/llm"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Exercise the authenticated production routes over HTTP, including the SSE
// response and the complete image/tool history received by the upstream.
func TestLLMLargeRequestBodyThroughBridge(t *testing.T) {
	for _, endpoint := range []string{"responses", "chat/completions"} {
		for _, chunked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/chunked=%t", endpoint, chunked), func(t *testing.T) {
				var expected []byte
				var calls atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					var got, want map[string]json.RawMessage
					if err := json.Unmarshal(body, &got); err != nil {
						t.Errorf("upstream JSON: %v", err)
					}
					_ = json.Unmarshal(expected, &want)
					field := "input"
					if endpoint == "chat/completions" {
						field = "messages"
					}
					var gotHistory, wantHistory any
					_ = json.Unmarshal(got[field], &gotHistory)
					_ = json.Unmarshal(want[field], &wantHistory)
					if !reflect.DeepEqual(gotHistory, wantHistory) {
						t.Errorf("upstream %s differs: got %d bytes, want %d", field, len(got[field]), len(want[field]))
					}
					if r.URL.Path != "/v1/"+endpoint {
						t.Errorf("upstream path = %s", r.URL.Path)
					}
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\"}\n\n")
				}))
				defer upstream.Close()
				h := &Handler{
					llm: llm.NewService(config.ModelConfig{}, usageProfile{agent.AgentProfile{
						Provider: agent.ProviderAPI, ModelID: "fixture-model", BaseURL: upstream.URL + "/v1", APIKey: "fixture",
					}}),
					serverAccessToken: "fixture-token",
				}
				bridge := httptest.NewServer(h.Routes())
				defer bridge.Close()
				client := &http.Client{Timeout: 30 * time.Second}
				for _, size := range []int{(10 << 20) - 1, 10 << 20, (10 << 20) + 1, 12 << 20} {
					expected = largeLLMBody(t, endpoint, size)
					var source io.Reader = bytes.NewReader(expected)
					if chunked {
						// Hide the length to force Transfer-Encoding: chunked.
						source = struct{ io.Reader }{source}
					}
					req, err := http.NewRequest(http.MethodPost, bridge.URL+"/api/v1/agents/alice/llm/"+endpoint, source)
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Authorization", "Bearer fixture-token")
					req.Header.Set("Content-Type", "application/json")
					resp, err := client.Do(req)
					if err != nil {
						t.Fatal(err)
					}
					body, readErr := io.ReadAll(resp.Body)
					resp.Body.Close()
					if readErr != nil || resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "response.completed") {
						t.Fatalf("request_bytes=%d: status=%d body=%s readErr=%v upstream_calls=%d", size, resp.StatusCode, body, readErr, calls.Load())
					}
				}
				if calls.Load() != 4 {
					t.Fatalf("upstream calls = %d, want 4", calls.Load())
				}
			})
		}
	}
}

func largeLLMBody(t *testing.T, endpoint string, size int) []byte {
	t.Helper()
	items := make([]any, 0, 28)
	for i := 0; i < 14; i++ {
		var image any = map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + strings.Repeat("A", (size-8192)/14)}
		if endpoint == "chat/completions" {
			image = map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + strings.Repeat("A", (size-8192)/14)}}
		}
		items = append(items, map[string]any{"role": "user", "content": []any{image}})
	}
	tail := map[string]any{"role": "user", "content": "history-tail"}
	items = append(items, tail)
	field := "input"
	if endpoint == "chat/completions" {
		field = "messages"
	}
	body, err := json.Marshal(map[string]any{"model": "fixture-model", field: items, "stream": true})
	if err != nil {
		t.Fatal(err)
	}
	// Put the closing JSON bytes exactly at the requested boundary.
	tail["content"] = strings.Repeat("x", size-len(body)) + "history-tail"
	body, err = json.Marshal(map[string]any{"model": "fixture-model", field: items, "stream": true})
	if err != nil || len(body) != size {
		t.Fatalf("body size = %d, want %d: %v", len(body), size, err)
	}
	return body
}

type llmBodyErrorReader struct{}

func (llmBodyErrorReader) Read([]byte) (int, error) { return 0, errors.New("broken body") }

type llmBodyFillReader struct{}

func (llmBodyFillReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}

func TestReadLLMRequestBodyLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		size   int64
		known  bool
		status int
	}{
		{"at-limit", maxLLMRequestBodyBytes, false, http.StatusOK},
		{"over-limit-known-length", maxLLMRequestBodyBytes + 1, true, http.StatusRequestEntityTooLarge},
		{"over-limit-unknown-length", maxLLMRequestBodyBytes + 1, false, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", io.LimitReader(llmBodyFillReader{}, tc.size))
			if tc.known {
				r.ContentLength = tc.size
			}
			w := httptest.NewRecorder()
			body, ok := readLLMRequestBody(w, r)
			if tc.status == http.StatusOK {
				if !ok || int64(len(body)) != tc.size {
					t.Fatalf("ok=%t body bytes=%d, want %d", ok, len(body), tc.size)
				}
				return
			}
			if ok || body != nil || w.Code != tc.status {
				t.Fatalf("ok=%t status=%d body bytes=%d", ok, w.Code, len(body))
			}
			var result struct {
				Error struct{ Code, Message string }
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Error.Code != "request_too_large" || !strings.Contains(result.Error.Message, "128 MiB") {
				t.Fatalf("missing explicit size error: %s", w.Body.String())
			}
		})
	}
}

func TestLLMRequestBodyErrorsDoNotReachUpstream(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	h := &Handler{llm: llm.NewService(config.ModelConfig{}, usageProfile{agent.AgentProfile{
		Provider: agent.ProviderAPI, ModelID: "fixture-model", BaseURL: upstream.URL, APIKey: "fixture",
	}})}
	for _, endpoint := range []string{"responses", "chat/completions"} {
		for _, tc := range []struct {
			name    string
			body    io.Reader
			size    int64
			status  int
			message string
		}{
			{"oversize", strings.NewReader("{}"), maxLLMRequestBodyBytes + 1, http.StatusRequestEntityTooLarge, "request_too_large"},
			{"read-error", llmBodyErrorReader{}, -1, http.StatusBadRequest, "read request body"},
			{"malformed", strings.NewReader(`{"input":`), -1, http.StatusBadRequest, "decode request"},
			// A valid prefix must never hide invalid bytes beyond the old cap.
			{"invalid-tail", io.MultiReader(strings.NewReader(`{"input":[]}`), io.LimitReader(llmBodyFillReader{}, 10<<20), strings.NewReader("!")), -1, http.StatusBadRequest, "decode request"},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				r := httptest.NewRequest(http.MethodPost, "/", tc.body)
				r.ContentLength = tc.size
				w := httptest.NewRecorder()
				if endpoint == "responses" {
					h.handleBotLLMResponses(w, r, "alice")
				} else {
					h.handleBotLLMChatCompletions(w, r, "alice")
				}
				if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.message) {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
			})
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid requests reached upstream: %d", calls.Load())
	}
}
