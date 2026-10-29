package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"csgclaw/internal/diagnostics"
	"csgclaw/internal/modelpreset"
	"github.com/gorilla/websocket"
)

func TestFullModelDiagnosticCapturesBothSidesOfProtocolConversion(t *testing.T) {
	upstreamBodies := []string{}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		upstreamBodies = append(upstreamBodies, string(raw))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "provider-request")
		io.WriteString(w, `{"id":"chat-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"full response"},"finish_reason":"stop"}]}`)
	}))
	defer model.Close()
	store := diagnostics.New(t.TempDir())
	record := store.Begin("r", "s", "", "agent", "turn")
	record.Running()
	// Explicit profile avoids unrelated provider discovery while using the real
	// Responses -> Chat Completions translator and the real HTTP client.
	profile := agent.AgentProfile{Provider: "api", APIProtocol: modelpreset.ChatCompletions, ModelID: "fixture-model", BaseURL: model.URL, APIKey: "opaque-provider-credential"}
	ctx := WithTaskProfile(diagnostics.WithRecord(context.Background(), record), profile)
	input := `{"model":"fixture-model","input":[{"role":"user","content":"` + strings.Repeat("complete history ", 1000) + `"}],"stream":false}`
	svc := NewService(config.ModelConfig{}, nil)
	response, err := svc.Responses(ctx, "agent", []byte(input))
	if err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "full response") {
		t.Fatalf("translation lost response: %s", output)
	}
	entries := []diagnostics.ContextEntry{}
	if err := store.ReadContext(context.Background(), record.Snapshot().ID, func(entry diagnostics.ContextEntry) error { entries = append(entries, entry); return nil }); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	var upstreamRequest map[string]any
	for _, entry := range entries {
		kinds[entry.Kind] = true
		if entry.Kind == "model.upstream_request" {
			_ = json.Unmarshal(entry.Data, &upstreamRequest)
		}
	}
	for _, kind := range []string{"model.runtime_request", "model.upstream_request", "model.upstream_response", "model.runtime_response"} {
		if !kinds[kind] {
			t.Fatalf("missing %s", kind)
		}
	}
	expected := map[string]any{}
	if len(upstreamBodies) != 1 {
		t.Fatalf("calls=%d", len(upstreamBodies))
	}
	_ = json.Unmarshal([]byte(upstreamBodies[0]), &expected)
	observed, _ := json.Marshal(upstreamRequest["body"])
	wanted, _ := json.Marshal(expected)
	if string(observed) != string(wanted) {
		t.Fatal("captured upstream request differs from wire")
	}
	raw, _ := json.Marshal(entries)
	if strings.Contains(string(raw), "opaque-provider-credential") {
		t.Fatal("provider credential leaked")
	}

}

func TestWebsocketDiagnosticsUsesPerMessageIdentityAcrossTurns(t *testing.T) {
	store := diagnostics.New("")
	a := store.Begin("r", "a", "", "agent", "a")
	b := store.Begin("r", "b", "", "agent", "b")
	a.Running()
	b.Running()
	a.RuntimeRef("session", "native-a", "")
	b.RuntimeRef("session", "native-b", "")
	ctx := diagnostics.WithNativeLookup(context.Background(), store, "agent", "", "")
	observer := newWebsocketDiagnostics(ctx, agent.AgentProfile{ModelID: "model", APIKey: "private-key"})
	for _, test := range []struct{ native, id, content string }{{"native-a", "response-a", "context-a"}, {"native-b", "response-b", "context-b"}} {
		metadata, _ := json.Marshal(map[string]string{"thread_id": "session", "turn_id": test.native})
		request, _ := json.Marshal(map[string]any{"type": "response.create", "input": test.content, "client_metadata": map[string]string{"x-codex-turn-metadata": string(metadata)}})
		observer.request(websocket.TextMessage, request, request)
		observer.response(websocket.TextMessage, []byte(`{"type":"response.created","response":{"id":"`+test.id+`"}}`))
		observer.response(websocket.TextMessage, []byte(`{"type":"response.output_text.delta","response_id":"`+test.id+`","delta":"`+test.content+`"}`))
		observer.response(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"`+test.id+`"}}`))
	}
	for _, test := range []struct {
		record          *diagnostics.Record
		want, forbidden string
	}{{a, "context-a", "context-b"}, {b, "context-b", "context-a"}} {
		var content strings.Builder
		if err := store.ReadContext(context.Background(), test.record.Snapshot().ID, func(entry diagnostics.ContextEntry) error { content.Write(entry.Data); return nil }); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(content.String(), test.want) || strings.Contains(content.String(), test.forbidden) {
			t.Fatalf("websocket identity mixed: %s", content.String())
		}
	}
}
