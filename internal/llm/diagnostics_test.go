package llm

import (
	"context"
	"csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"csgclaw/internal/diagnostics"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type diagnosticProfiles struct{ base string }

func (p diagnosticProfiles) ResolvedAgentProfile(string) (agents.AgentProfile, error) {
	return agents.AgentProfile{Provider: agents.ProviderAPI, BaseURL: p.base, APIKey: "fixture-key", ModelID: "fixture-model", ProfileComplete: true}, nil
}
func modelSpans(record *diagnostics.Record) []diagnostics.Span {
	var out []diagnostics.Span
	for _, s := range record.Snapshot().Spans {
		if s.Owner == "llm" {
			out = append(out, s)
		}
	}
	return out
}
func TestModelTimingIncludesStreamingBodyAndExactTurn(t *testing.T) {
	store := diagnostics.New("")
	store.Source("room", "source", time.Now())
	record := store.Begin("room", "source", "", "agent", "turn")
	record.Running()
	record.RuntimeStart()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate an HTTP request arriving before turn/start's response is consumed.
		time.Sleep(15 * time.Millisecond)
		record.RuntimeRef("session", "native", "")
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		time.Sleep(40 * time.Millisecond)
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	service := NewService(config.ModelConfig{}, diagnosticProfiles{server.URL + "/v1"})
	ctx := diagnostics.WithNativeLookup(context.Background(), store, "agent", "session", "native")
	response, err := service.ChatCompletionsStream(ctx, "agent", []byte(`{"messages":[],"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	spans := modelSpans(record)
	if len(spans) != 1 {
		t.Fatalf("LLM spans=%+v", spans)
	}
	span := spans[0]
	if span.EndMS == nil || *span.EndMS-span.StartMS < 50 || span.Details == nil || span.Details.FirstResponseMS == nil || *span.Details.FirstResponseMS >= *span.EndMS-span.StartMS || span.Status != "completed" {
		t.Fatalf("stream duration/first byte=%+v, details=%+v", span, span.Details)
	}
	// The same Agent's other turn must not receive this request.
	wrong := diagnostics.WithNativeLookup(context.Background(), store, "other-agent", "session", "native")
	response, err = service.ChatCompletionsStream(wrong, "agent", []byte(`{"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if len(modelSpans(record)) != 1 {
		t.Fatal("cross-Agent request was attributed")
	}
}
func TestModelTimingPreservesFailedAttemptAndFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/responses" {
			http.Error(w, "unsupported", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"reply","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	store := diagnostics.New("")
	record := store.Begin("r", "s", "", "a", "t")
	record.Running()
	record.RuntimeStart()
	service := NewService(config.ModelConfig{}, diagnosticProfiles{server.URL + "/v1"})
	response, err := service.Responses(diagnostics.WithRecord(context.Background(), record), "a", []byte(`{"input":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	spans := modelSpans(record)
	if len(spans) != 2 || spans[0].Status != "failed" || spans[0].Details.HTTPStatus != 404 || spans[1].Status != "completed" {
		t.Fatalf("attempts=%+v", spans)
	}
}

func TestModelTimingRecognizesStreamingFailureWithHTTP200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.failed\",\"error\":{\"code\":\"rate_limit\"}}\n\n")
	}))
	defer server.Close()
	store := diagnostics.New("")
	record := store.Begin("r", "s", "", "a", "t")
	record.Running()
	service := NewService(config.ModelConfig{}, diagnosticProfiles{server.URL + "/v1"})
	response, err := service.ChatCompletionsStream(diagnostics.WithRecord(context.Background(), record), "a", []byte(`{"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	spans := modelSpans(record)
	if len(spans) != 1 || spans[0].Status != "failed" || spans[0].Details.HTTPStatus != 200 {
		t.Fatalf("stream error hidden: %+v", spans)
	}
}

func TestModelTimingEndsOnTerminalEventBeforeConnectionEOF(t *testing.T) {
	status := ""
	body := &observedModelBody{ReadCloser: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\"}\n\n")), stream: true, status: 200, started: time.Now(), update: func(int, *float64) {}, finish: func(s string) { status = s }}
	if _, err := body.Read(make([]byte, 256)); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatal("completed model response waited for HTTP EOF")
	}
	body.Close()
	if status != "completed" {
		t.Fatal("closing a finished stream changed its result")
	}
}
