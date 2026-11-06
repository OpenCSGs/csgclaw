package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/cliproxy"
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

func TestSessionModelPreservesTaskProvider(t *testing.T) {
	upstream := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"upstream": name, "model": payload["model"], "authorization": r.Header.Get("Authorization")})
		}))
	}
	agentUpstream := upstream("agent")
	defer agentUpstream.Close()
	taskUpstream := upstream("task")
	defer taskUpstream.Close()
	service := NewService(config.ModelConfig{}, sessionProfileFixture{base: agentUpstream.URL})
	taskProfile := agent.AgentProfile{Provider: agent.ProviderAPI, ModelProviderID: "task", BaseURL: taskUpstream.URL, APIKey: "task-key", ModelID: "task-model", ReasoningEffort: "low"}
	for _, test := range []struct {
		name string
		call func(context.Context, string, []byte) (*UpstreamResponse, error)
		body string
	}{
		{"responses", service.Responses, `{"model":"model-b","input":"hello"}`},
		{"chat", service.ChatCompletionsStream, `{"model":"model-b","messages":[]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := WithTaskProfile(context.Background(), taskProfile)
			ctx = WithSessionModel(ctx, "model-b", "high")
			response, err := test.call(ctx, "agent-a", []byte(test.body))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var result map[string]any
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if result["upstream"] != "task" || result["model"] != taskProfile.ModelID || result["authorization"] != "Bearer task-key" {
				t.Fatalf("task provider changed: %v", result)
			}
		})
	}
}

func TestSessionModelBridgePreservesInheritedProfileEffort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	controller := mustSeededAgentService(t, config.LLMConfig{Default: "codex.gpt-5.5", Providers: map[string]config.ProviderConfig{
		agent.ModelProviderIDCodex: {Models: []string{"gpt-5.5"}},
	}}, []agent.Agent{{ID: "agent-inherited", Name: "Inherited", Role: agent.RoleWorker, AgentProfile: agent.AgentProfile{
		Provider: agent.ProviderCodex, ModelProviderID: agent.ModelProviderIDCodex, ModelID: "gpt-5.5", ReasoningEffort: "max",
	}}})
	defer controller.Close()
	models := controller.Models()
	if _, err := models.ResolvedSessionProfile("agent-inherited", "gpt-5.5", "max"); err == nil {
		t.Fatal("explicit unsupported effort was accepted")
	}
	selected, err := models.ResolvedSessionProfile("agent-inherited", "gpt-5.5", "")
	if err != nil || selected.ReasoningEffort != "max" {
		t.Fatalf("inherited selection = %+v, error = %v", selected, err)
	}
	before, err := models.ResolvedAgentProfile("agent-inherited")
	if err != nil {
		t.Fatal(err)
	}
	payloads := make(chan map[string]any, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		payloads <- payload
		w.Header().Set("Content-Type", "text/event-stream")
		if r.URL.Path == "/v1/responses" {
			io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"fixture\",\"status\":\"completed\",\"output\":[]}}\n\n")
		} else {
			io.WriteString(w, "data: {\"id\":\"fixture\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer upstream.Close()
	previousURL, previousAuth := embeddedCLIProxyProviderBaseURL, embeddedCLIProxyAuthStatus
	defer func() {
		embeddedCLIProxyProviderBaseURL, embeddedCLIProxyAuthStatus = previousURL, previousAuth
	}()
	embeddedCLIProxyProviderBaseURL = func(context.Context, string) (string, error) { return upstream.URL + "/v1", nil }
	embeddedCLIProxyAuthStatus = func(context.Context, string) (cliproxy.AuthStatus, error) {
		return cliproxy.AuthStatus{Authenticated: true}, nil
	}
	service := NewService(config.ModelConfig{}, models)
	ctx := WithSessionModel(context.Background(), selected.ModelID, selected.ReasoningEffort)
	for _, test := range []struct {
		name, body string
		call       func(context.Context, string, []byte) (*UpstreamResponse, error)
	}{
		{"responses", `{"model":"client-model","input":"hello","reasoning":{"effort":"max"},"stream":true}`, service.Responses},
		{"chat", `{"model":"client-model","messages":[],"stream":true}`, service.ChatCompletionsStream},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := test.call(ctx, "agent-inherited", []byte(test.body))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if _, err := io.Copy(io.Discard, response.Body); err != nil {
				t.Fatal(err)
			}
			payload := <-payloads
			effort := payload["reasoning_effort"]
			if reasoning, ok := payload["reasoning"].(map[string]any); ok {
				effort = reasoning["effort"]
			}
			if payload["model"] != selected.ModelID || effort != "max" || payload["stream"] != true {
				t.Fatalf("inherited upstream selection = %v", payload)
			}
		})
	}
	after, err := models.ResolvedAgentProfile("agent-inherited")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("bridge changed Profile defaults", err)
	}
}
