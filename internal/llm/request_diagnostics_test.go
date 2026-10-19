package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
	"csgclaw/internal/diagnostics"
)

type selectedModelFixture struct{ base, model string }

func (p *selectedModelFixture) ResolvedAgentProfile(string) (agent.AgentProfile, error) {
	return agent.AgentProfile{Provider: agent.ProviderAPI, ModelProviderID: "opencsg", BaseURL: p.base, APIKey: "fixture-key", ModelID: p.model, ProfileComplete: true}, nil
}

func TestSelectedModelReachesUpstreamAndDiagnosticsAfterSwitch(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	selected := &selectedModelFixture{model: "glm-5.3-flash"}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["model"] != selected.model {
			t.Errorf("model=%v want %s", payload["model"], selected.model)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "upstream-fixture")
		w.WriteHeader(400)
		io.WriteString(w, `{"error":{"code":"invalid_request_error","message":"unsupported tools schema","api_key":"fixture-secret"}}`)
	}))
	defer upstream.Close()
	selected.base = upstream.URL
	svc := NewService(config.ModelConfig{}, selected)
	store := diagnostics.New("")
	for _, model := range []string{"glm-5.3-flash", "glm-next"} {
		selected.model = model
		record := store.Begin("room", model, "", "agent-a", model)
		record.Running()
		record.RuntimeInfo("codex", "stale-runtime-model")
		bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response, err := svc.ChatCompletionsStream(diagnostics.WithRecord(r.Context(), record), "agent-a", []byte(`{"model":"stale-runtime-model","messages":[{"role":"user","content":"private-prompt"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]}`))
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			defer response.Body.Close()
			w.WriteHeader(response.StatusCode)
			io.Copy(w, response.Body)
		}))
		response, err := http.Post(bridge.URL, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		bridge.Close()
		if got := record.Snapshot().Model; got != model {
			t.Errorf("diagnostic model=%q want current selection %q", got, model)
		}
	}
	text := logs.String()
	for _, field := range []string{`"agent_id":"agent-a"`, `"model_provider":"opencsg"`, `"model":"glm-next"`, `"runtime_model":"stale-runtime-model"`, `"tool_count":1`, `"upstream_request_id":"upstream-fixture"`, `"request_id":`} {
		if !strings.Contains(text, field) {
			t.Errorf("missing diagnostic field %s", field)
		}
	}
	if strings.Contains(text, "private-prompt") || strings.Contains(text, "fixture-secret") || strings.Contains(text, "fixture-key") {
		t.Fatalf("sensitive data in logs: %s", text)
	}
}

type failingModelTransport struct{}

func (failingModelTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("connection failed with credential fixture-transport-secret")
}
func TestModelTransportFailureRetainsSafeRequestIdentity(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	selected := &selectedModelFixture{base: "http://fixture.invalid", model: "glm-5.3-flash"}
	service := NewService(config.ModelConfig{}, selected)
	service.client = &http.Client{Transport: failingModelTransport{}}
	_, err := service.Responses(WithNativeRequestMetadata(context.Background(), "session-fixture", "turn-fixture"), "agent-a", []byte(`{"model":"stale-model","input":"private-prompt"}`))
	if err == nil {
		t.Fatal("transport failure missing")
	}
	text := logs.String()
	for _, field := range []string{`"code":"connection_failed"`, `"agent_id":"agent-a"`, `"model":"glm-5.3-flash"`, `"runtime_session_id":"session-fixture"`, `"runtime_turn_id":"turn-fixture"`} {
		if !strings.Contains(text, field) {
			t.Errorf("missing %s in %s", field, text)
		}
	}
	if strings.Contains(text, "fixture-transport-secret") || strings.Contains(text, "private-prompt") {
		t.Fatal("sensitive transport data was logged")
	}
}
