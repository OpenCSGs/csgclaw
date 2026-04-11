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
