package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/cliproxy"
	"csgclaw/internal/config"
)

func TestChatCompletionsLLMAPIOverridesModelAndProxiesUpstream(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var gotModel string
	var gotReasoningEffort string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/chat/completions")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		gotModel, _ = payload["model"].(string)
		gotReasoningEffort, _ = payload["reasoning_effort"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"remote result"},"finish_reason":"stop"}]}`))
	}))
	defer upstream.Close()

	agentSvc := mustSeededAgentService(t, config.SingleProfileLLM(config.ModelConfig{
		Provider:        config.ProviderLLMAPI,
		BaseURL:         upstream.URL + "/v1",
		APIKey:          "sk-test",
		ModelID:         "gpt-5.4",
		ReasoningEffort: "medium",
	}), []agent.Agent{
		{
			ID:              agent.ManagerUserID,
			Name:            agent.ManagerName,
			Role:            agent.RoleManager,
			Profile:         config.DefaultLLMProfile,
			Provider:        config.ProviderLLMAPI,
			ModelID:         "gpt-5.4",
			ReasoningEffort: "medium",
			CreatedAt:       time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	body, status, _, err := svc.ChatCompletions(context.Background(), agent.ManagerUserID, []byte(`{"model":"client-model","messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatalf("ChatCompletions() error = %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if gotModel != "gpt-5.4" {
		t.Fatalf("upstream model = %q, want %q", gotModel, "gpt-5.4")
	}
	if gotReasoningEffort != "medium" {
		t.Fatalf("upstream reasoning_effort = %q, want %q", gotReasoningEffort, "medium")
	}
	if !strings.Contains(string(body), "remote result") {
		t.Fatalf("body = %s, want remote result", body)
	}
}

func TestChatCompletionsLLMAPIDoesNotOverrideRequestReasoningEffort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var gotReasoningEffort string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		gotReasoningEffort, _ = payload["reasoning_effort"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"remote result"},"finish_reason":"stop"}]}`))
	}))
	defer upstream.Close()

	agentSvc := mustSeededAgentService(t, config.SingleProfileLLM(config.ModelConfig{
		Provider:        config.ProviderLLMAPI,
		BaseURL:         upstream.URL + "/v1",
		APIKey:          "sk-test",
		ModelID:         "gpt-5.4",
		ReasoningEffort: "medium",
	}), []agent.Agent{
		{
			ID:              agent.ManagerUserID,
			Name:            agent.ManagerName,
			Role:            agent.RoleManager,
			Profile:         config.DefaultLLMProfile,
			Provider:        config.ProviderLLMAPI,
			ModelID:         "gpt-5.4",
			ReasoningEffort: "medium",
			CreatedAt:       time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	_, status, _, err := svc.ChatCompletions(context.Background(), agent.ManagerUserID, []byte(`{"messages":[{"role":"user","content":"hello"}],"reasoning_effort":"high"}`))
	if err != nil {
		t.Fatalf("ChatCompletions() error = %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if gotReasoningEffort != "high" {
		t.Fatalf("upstream reasoning_effort = %q, want %q", gotReasoningEffort, "high")
	}
}

func TestChatCompletionsMergesProfileRequestOptionsWithoutOverridingExplicitFields(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var payload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer upstream.Close()

	agentSvc := mustSeededAgentService(t, config.SingleProfileLLM(config.ModelConfig{
		Provider: config.ProviderLLMAPI,
		BaseURL:  upstream.URL + "/v1",
		APIKey:   "sk-test",
		ModelID:  "gpt-5.4",
	}), []agent.Agent{
		{
			ID:   agent.ManagerUserID,
			Name: agent.ManagerName,
			Role: agent.RoleManager,
			AgentProfile: agent.AgentProfile{
				Name:            agent.ManagerName,
				Provider:        agent.ProviderAPI,
				BaseURL:         upstream.URL + "/v1",
				APIKey:          "sk-test",
				ModelID:         "gpt-5.4",
				ReasoningEffort: "medium",
				EnableFastMode:  true,
				RequestOptions: map[string]any{
					"temperature": 0.2,
					"metadata":    map[string]any{"source": "profile"},
				},
				ProfileComplete: true,
			},
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	_, status, _, err := svc.ChatCompletions(context.Background(), agent.ManagerUserID, []byte(`{"model":"client","messages":[],"temperature":0.7}`))
	if err != nil {
		t.Fatalf("ChatCompletions() error = %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if payload["model"] != "gpt-5.4" {
		t.Fatalf("model = %#v, want gpt-5.4", payload["model"])
	}
	if payload["temperature"] != 0.7 {
		t.Fatalf("temperature = %#v, want explicit 0.7", payload["temperature"])
	}
	if payload["service_tier"] != "priority" {
		t.Fatalf("service_tier = %#v, want priority", payload["service_tier"])
	}
	if _, ok := payload["metadata"].(map[string]any); !ok {
		t.Fatalf("metadata = %#v, want profile metadata object", payload["metadata"])
	}
}

func TestChatCompletionsCodexRoutesThroughEmbeddedCLIProxy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	previous := embeddedCLIProxyProviderBaseURL
	previousAuthStatus := embeddedCLIProxyAuthStatus
	defer func() {
		embeddedCLIProxyProviderBaseURL = previous
		embeddedCLIProxyAuthStatus = previousAuthStatus
	}()

	var payload map[string]any
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/provider/codex/v1/chat/completions" {
			t.Fatalf("path = %q, want embedded codex provider route", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer upstream.Close()
	embeddedCLIProxyProviderBaseURL = func(_ context.Context, provider string) (string, error) {
		if provider != agent.ProviderCodex {
			t.Fatalf("provider = %q, want codex", provider)
		}
		return upstream.URL + "/api/provider/codex/v1", nil
	}
	embeddedCLIProxyAuthStatus = func(_ context.Context, provider string) (cliproxy.AuthStatus, error) {
		if provider != agent.ProviderCodex {
			t.Fatalf("auth provider = %q, want codex", provider)
		}
		return cliproxy.AuthStatus{Provider: "codex", Authenticated: true}, nil
	}

	agentSvc := mustSeededAgentService(t, config.LLMConfig{}, []agent.Agent{
		{
			ID:   agent.ManagerUserID,
			Name: agent.ManagerName,
			Role: agent.RoleManager,
			AgentProfile: agent.AgentProfile{
				Name:            agent.ManagerName,
				Provider:        agent.ProviderCodex,
				ModelID:         "gpt-5.4",
				ReasoningEffort: "medium",
				EnableFastMode:  true,
				ProfileComplete: true,
			},
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	_, status, _, err := svc.ChatCompletions(context.Background(), agent.ManagerUserID, []byte(`{"model":"client","messages":[],"store":true}`))
	if err != nil {
		t.Fatalf("ChatCompletions() error = %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if gotAuth != "Bearer local" {
		t.Fatalf("Authorization = %q, want Bearer local", gotAuth)
	}
	if payload["model"] != "gpt-5.4" {
		t.Fatalf("model = %#v, want gpt-5.4", payload["model"])
	}
	if payload["service_tier"] != "priority" {
		t.Fatalf("service_tier = %#v, want priority", payload["service_tier"])
	}
	if payload["store"] != false {
		t.Fatalf("store = %#v, want false", payload["store"])
	}
}

func TestChatCompletionsCodexRequiresAuth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	previousAuthStatus := embeddedCLIProxyAuthStatus
	defer func() { embeddedCLIProxyAuthStatus = previousAuthStatus }()
	embeddedCLIProxyAuthStatus = func(_ context.Context, provider string) (cliproxy.AuthStatus, error) {
		if provider != agent.ProviderCodex {
			t.Fatalf("auth provider = %q, want codex", provider)
		}
		return cliproxy.AuthStatus{
			Provider:      "codex",
			LoginRequired: true,
			Message:       "Auth required. Run csgclaw model auth login codex.",
		}, nil
	}

	agentSvc := mustSeededAgentService(t, config.LLMConfig{}, []agent.Agent{
		{
			ID:   agent.ManagerUserID,
			Name: agent.ManagerName,
			Role: agent.RoleManager,
			AgentProfile: agent.AgentProfile{
				Name:            agent.ManagerName,
				Provider:        agent.ProviderCodex,
				ModelID:         "gpt-5.4",
				ReasoningEffort: "medium",
				ProfileComplete: true,
			},
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	_, _, _, err := svc.ChatCompletions(context.Background(), agent.ManagerUserID, []byte(`{"messages":[]}`))
	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("error = %T %v, want *HTTPError", err, err)
	}
	if httpErr.Status != http.StatusConflict || httpErr.Code != "auth_required" || httpErr.Provider != agent.ProviderCodex {
		t.Fatalf("HTTPError = %+v, want auth_required conflict", httpErr)
	}
}

func TestModelsReturnsResolvedAgentModel(t *testing.T) {
	agentSvc := mustSeededAgentService(t, config.SingleProfileLLM(config.ModelConfig{
		Provider:        config.ProviderLLMAPI,
		BaseURL:         "https://example.test/v1",
		APIKey:          "sk-test",
		ModelID:         "gpt-5.4-mini",
		ReasoningEffort: "high",
	}), []agent.Agent{
		{
			ID:              agent.ManagerUserID,
			Name:            agent.ManagerName,
			Role:            agent.RoleManager,
			Profile:         config.DefaultLLMProfile,
			Provider:        config.ProviderLLMAPI,
			ModelID:         "gpt-5.4-mini",
			ReasoningEffort: "high",
			CreatedAt:       time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	body, status, _, err := svc.Models(context.Background(), agent.ManagerUserID)
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if !strings.Contains(string(body), `"id":"gpt-5.4-mini"`) {
		t.Fatalf("body = %s, want resolved model id", body)
	}
}

func mustSeededAgentService(t *testing.T, llmCfg config.LLMConfig, agents []agent.Agent) *agent.Service {
	t.Helper()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(map[string]any{
		"agents": agents,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc, err := agent.NewServiceWithLLM(llmCfg, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}
