package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/config"
)

type sessionProfileFixture struct{ base string }

func (f sessionProfileFixture) ResolvedAgentProfile(string) (agent.AgentProfile, error) {
	return agent.AgentProfile{Provider: agent.ProviderAPI, ModelID: "model-a", ReasoningEffort: "low", BaseURL: f.base, APIKey: "test-key"}, nil
}

func (f sessionProfileFixture) ResolvedSessionProfile(id, model, effort string) (agent.AgentProfile, error) {
	profile, _ := f.ResolvedAgentProfile(id)
	if model != "model-b" {
		return profile, fmt.Errorf("model unavailable")
	}
	profile.ModelID, profile.ReasoningEffort = model, effort
	return profile, nil
}

func TestSessionModelBridgeUsesOnlyScopedSelection(t *testing.T) {
	payloads := make(chan map[string]any, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		payloads <- payload
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"response","object":"response","status":"completed","output":[]}`)
	}))
	defer upstream.Close()
	service := NewService(config.ModelConfig{}, sessionProfileFixture{base: upstream.URL})
	for _, test := range []struct {
		ctx   context.Context
		model string
	}{
		{WithSessionModel(context.Background(), "model-b", "high"), "model-b"},
		{context.Background(), "model-a"},
		{context.Background(), "model-a"},
	} {
		response, err := service.Responses(test.ctx, "agent-a", []byte(`{"model":"untrusted-model","input":"hello"}`))
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if payload := <-payloads; payload["model"] != test.model {
			t.Fatalf("upstream model=%v want %s", payload["model"], test.model)
		}
	}
	if _, err := service.Responses(WithSessionModel(context.Background(), "missing", "high"), "agent-a", []byte(`{"input":"hello"}`)); err == nil {
		t.Fatal("invalid selection fell back to the Agent model")
	}
}
