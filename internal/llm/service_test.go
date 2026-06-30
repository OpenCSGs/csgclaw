package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/auth"
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
			ID:          agent.ManagerUserID,
			Name:        agent.ManagerName,
			Role:        agent.RoleManager,
			Profile:     config.DefaultLLMProfile,
			RuntimeKind: agent.RuntimeKindPicoClawSandbox,
			AgentProfile: agent.AgentProfile{
				Name:            agent.ManagerName,
				Provider:        agent.ProviderAPI,
				BaseURL:         upstream.URL + "/v1",
				APIKey:          "sk-test",
				ModelID:         "gpt-5.4",
				ReasoningEffort: "medium",
				ProfileComplete: true,
			},
			ProfileComplete: true,
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

func TestChatCompletionsUsesCSGHubAIGatewayAuthStore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var gotAuth string
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		gotModel, _ = payload["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"remote result"},"finish_reason":"stop"}]}`))
	}))
	defer upstream.Close()
	t.Setenv("CSGHUB_AIGATEWAY_BASE_URL", upstream.URL)

	var sawBuiltin bool
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/namespaces/user-1/apikeys/builtin" {
			t.Fatalf("hub path = %q, want builtin api key path", r.URL.Path)
		}
		if got := r.URL.Query().Get("current_user"); got != "alice" {
			t.Fatalf("current_user = %q, want alice", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("hub Authorization = %q, want access token", got)
		}
		sawBuiltin = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"msg":"OK","data":{"token":"gk_aigateway-key"}}`))
	}))
	defer hub.Close()

	store, err := auth.DefaultStore()
	if err != nil {
		t.Fatalf("DefaultStore() error = %v", err)
	}
	if err := store.Save(auth.Record{
		Tokens: auth.Tokens{AccessToken: "access-token"},
		Account: auth.Account{
			BaseURL:  hub.URL,
			UserID:   "alice",
			UserUUID: "user-1",
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	agentSvc := mustSeededAgentService(t, config.LLMConfig{}, []agent.Agent{
		{
			ID:          agent.ManagerUserID,
			Name:        agent.ManagerName,
			Role:        agent.RoleManager,
			RuntimeKind: agent.RuntimeKindPicoClawSandbox,
			AgentProfile: agent.AgentProfile{
				Name:            agent.ManagerName,
				Provider:        agent.ProviderCSGHub,
				ModelID:         "deepseek-v4-flash",
				ReasoningEffort: "medium",
				ProfileComplete: true,
			},
			ProfileComplete: true,
			CreatedAt:       time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	_, status, _, err := svc.ChatCompletions(context.Background(), agent.ManagerUserID, []byte(`{"model":"client-model","messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatalf("ChatCompletions() error = %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if gotAuth != "Bearer gk_aigateway-key" {
		t.Fatalf("Authorization = %q, want aigateway key", gotAuth)
	}
	if gotModel != "deepseek-v4-flash" {
		t.Fatalf("model = %q, want deepseek-v4-flash", gotModel)
	}
	if !sawBuiltin {
		t.Fatal("builtin api key endpoint was not called")
	}
	credentials, ok, err := store.LoadCSGHubProviderCredentials()
	if err != nil || !ok {
		t.Fatalf("LoadCSGHubProviderCredentials() = %+v, %v, %v", credentials, ok, err)
	}
	if credentials.AIGatewayBuiltinAPIKey != "gk_aigateway-key" {
		t.Fatalf("stored AIGatewayBuiltinAPIKey = %q, want cached aigateway key", credentials.AIGatewayBuiltinAPIKey)
	}
}

func TestNewServiceDoesNotImposeFixedUpstreamTimeout(t *testing.T) {
	svc := NewService(config.ModelConfig{}, nil)
	if svc.client == nil {
		t.Fatal("client is nil")
	}
	if svc.client.Timeout != 0 {
		t.Fatalf("client timeout = %s, want no fixed upstream timeout", svc.client.Timeout)
	}
	if svc.client.Transport != nil {
		t.Fatal("client transport is set; want default transport so proxy environment and no_proxy are honored")
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
			ID:          agent.ManagerUserID,
			Name:        agent.ManagerName,
			Role:        agent.RoleManager,
			Profile:     config.DefaultLLMProfile,
			RuntimeKind: agent.RuntimeKindPicoClawSandbox,
			AgentProfile: agent.AgentProfile{
				Name:            agent.ManagerName,
				Provider:        agent.ProviderAPI,
				BaseURL:         upstream.URL + "/v1",
				APIKey:          "sk-test",
				ModelID:         "gpt-5.4",
				ReasoningEffort: "medium",
				ProfileComplete: true,
			},
			ProfileComplete: true,
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

func TestResponsesLLMAPIOverridesModelAndProxiesUpstream(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var gotModel string
	var gotReasoningEffort any
	var gotContentType string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/responses")
		}
		gotContentType = r.Header.Get("Content-Type")
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		gotModel, _ = payload["model"].(string)
		gotReasoningEffort = payload["reasoning_effort"]
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Upstream-Test", "responses")
		_, _ = w.Write([]byte("event: response.completed\ndata: {}\n\n"))
	}))
	defer upstream.Close()

	agentSvc := mustSeededAgentService(t, config.LLMConfig{}, []agent.Agent{
		{
			ID:   agent.ManagerUserID,
			Name: agent.ManagerName,
			Role: agent.RoleManager,
			AgentProfile: agent.AgentProfile{
				Name:            agent.ManagerName,
				Provider:        agent.ProviderAPI,
				BaseURL:         upstream.URL + "/v1",
				APIKey:          "sk-test",
				ModelID:         "gpt-5.5",
				ReasoningEffort: "medium",
				RequestOptions: map[string]any{
					"temperature":      0.2,
					"reasoning_effort": "high",
				},
				ProfileComplete: true,
			},
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	resp, err := svc.Responses(context.Background(), agent.ManagerUserID, []byte(`{"model":"client-model","input":"hello"}`))
	if err != nil {
		t.Fatalf("Responses() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if gotContentType != "application/json" {
		t.Fatalf("request Content-Type = %q, want application/json", gotContentType)
	}
	if gotModel != "gpt-5.5" {
		t.Fatalf("upstream model = %q, want %q", gotModel, "gpt-5.5")
	}
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", resp.Header.Get("Content-Type"))
	}
	if resp.Header.Get("X-Upstream-Test") != "responses" {
		t.Fatalf("X-Upstream-Test = %q, want responses", resp.Header.Get("X-Upstream-Test"))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !strings.Contains(string(body), "response.completed") {
		t.Fatalf("body = %q, want response.completed event", string(body))
	}
	if gotReasoningEffort != nil {
		t.Fatalf("reasoning_effort = %#v, want omitted for Responses payload", gotReasoningEffort)
	}
}

func TestResponsesLLMAPICodexOmitsUnsupportedReasoningInput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	previousProviderBaseURL := embeddedCLIProxyProviderBaseURL
	previousAuthStatus := embeddedCLIProxyAuthStatus
	defer func() {
		embeddedCLIProxyProviderBaseURL = previousProviderBaseURL
		embeddedCLIProxyAuthStatus = previousAuthStatus
	}()

	var gotInput []any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/provider/codex/v1/responses" {
			t.Fatalf("path = %q, want embedded codex responses route", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		gotInput, _ = payload["input"].([]any)
		for _, item := range gotInput {
			obj, _ := item.(map[string]any)
			if strings.EqualFold(strings.TrimSpace(stringValue(obj["type"])), "reasoning") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"unsupported_feature:input.reasoning","type":"invalid_request_error","code":"unsupported_feature"}}`))
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp-1","object":"response","status":"completed","output":[],"output_text":"ok"}`))
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
				ModelID:         "gpt-5.5",
				ProfileComplete: true,
			},
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	resp, err := svc.Responses(context.Background(), agent.ManagerUserID, []byte(`{"model":"client-model","input":[{"type":"reasoning","id":"rs_1","summary":[]},{"role":"user","content":"hello"}],"stream":false}`))
	if err != nil {
		t.Fatalf("Responses() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %q, want 200 after omitting reasoning input", resp.StatusCode, string(body))
	}
	if len(gotInput) != 1 {
		t.Fatalf("input items = %#v, want only user input after dropping reasoning item", gotInput)
	}
	item, _ := gotInput[0].(map[string]any)
	if item["role"] != "user" || item["content"] != "hello" {
		t.Fatalf("remaining input = %#v, want original user message", gotInput)
	}
}

func TestResponsesLLMAPIRetriesWithoutReasoningInputAfterUnsupportedFeature(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var responsesCalls int
	var seenInputs [][]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q, want /v1/responses", r.URL.Path)
		}
		responsesCalls++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		input, _ := payload["input"].([]any)
		seenInputs = append(seenInputs, input)
		for _, item := range input {
			if responseInputItemIsReasoning(item) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"unsupported_feature:input.reasoning","type":"invalid_request_error","code":"unsupported_feature"}}`))
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp-1","object":"response","status":"completed","output":[],"output_text":"ok"}`))
	}))
	defer upstream.Close()

	agentSvc := mustSeededAgentService(t, config.LLMConfig{}, []agent.Agent{
		{
			ID:   agent.ManagerUserID,
			Name: agent.ManagerName,
			Role: agent.RoleManager,
			AgentProfile: agent.AgentProfile{
				Name:            agent.ManagerName,
				Provider:        agent.ProviderAPI,
				BaseURL:         upstream.URL + "/v1",
				APIKey:          "sk-test",
				ModelID:         "qwen3.6-plus",
				ProfileComplete: true,
			},
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	body := []byte(`{"model":"client-model","input":[{"type":"reasoning","id":"rs_1","summary":[]},{"role":"user","content":"hello"}],"stream":false}`)
	resp, err := svc.Responses(context.Background(), agent.ManagerUserID, body)
	if err != nil {
		t.Fatalf("Responses() error = %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if responsesCalls != 2 {
		t.Fatalf("/responses calls = %d, want initial failure plus retry", responsesCalls)
	}
	if len(seenInputs) != 2 {
		t.Fatalf("seen inputs = %#v, want two requests", seenInputs)
	}
	if !inputContainsReasoning(seenInputs[0]) {
		t.Fatalf("first input = %#v, want original reasoning item", seenInputs[0])
	}
	if inputContainsReasoning(seenInputs[1]) {
		t.Fatalf("retry input = %#v, want reasoning item removed", seenInputs[1])
	}

	resp, err = svc.Responses(context.Background(), agent.ManagerUserID, body)
	if err != nil {
		t.Fatalf("second Responses() error = %v", err)
	}
	_ = resp.Body.Close()
	if responsesCalls != 3 {
		t.Fatalf("/responses calls after cached retry = %d, want one additional stripped request", responsesCalls)
	}
	if inputContainsReasoning(seenInputs[2]) {
		t.Fatalf("cached input = %#v, want reasoning item removed before send", seenInputs[2])
	}
}

func TestResponsesLLMAPIFallsBackToChatCompletionsWhenUnsupported(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var sawResponses bool
	var gotChatModel string
	var gotChatMessages []map[string]any
	var gotChatStream any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			sawResponses = true
			http.Error(w, "no responses here", http.StatusNotFound)
		case "/v1/chat/completions":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode() chat payload error = %v", err)
			}
			gotChatModel, _ = payload["model"].(string)
			gotChatStream = payload["stream"]
			if messages, ok := payload["messages"].([]any); ok {
				for _, msg := range messages {
					if m, ok := msg.(map[string]any); ok {
						gotChatMessages = append(gotChatMessages, m)
					}
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"chatcmpl-fallback","object":"chat.completion","created":1710000000,"model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"fallback result"},"finish_reason":"stop"}]}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer upstream.Close()

	agentSvc := mustSeededAgentService(t, config.LLMConfig{}, []agent.Agent{
		{
			ID:   agent.ManagerUserID,
			Name: agent.ManagerName,
			Role: agent.RoleManager,
			AgentProfile: agent.AgentProfile{
				Name:            agent.ManagerName,
				Provider:        agent.ProviderAPI,
				BaseURL:         upstream.URL + "/v1",
				APIKey:          "sk-test",
				ModelID:         "deepseek-v4-pro",
				ProfileComplete: true,
			},
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	resp, err := svc.Responses(context.Background(), agent.ManagerUserID, []byte(`{"model":"client-model","input":"hello","stream":false}`))
	if err != nil {
		t.Fatalf("Responses() error = %v", err)
	}
	defer resp.Body.Close()
	if !sawResponses {
		t.Fatal("upstream /responses was not attempted before chat fallback")
	}
	if gotChatModel != "deepseek-v4-pro" {
		t.Fatalf("chat model = %q, want deepseek-v4-pro", gotChatModel)
	}
	if gotChatStream != false {
		t.Fatalf("chat stream = %#v, want false", gotChatStream)
	}
	if len(gotChatMessages) != 1 || gotChatMessages[0]["role"] != "user" || gotChatMessages[0]["content"] != "hello" {
		t.Fatalf("chat messages = %#v, want single user hello message", gotChatMessages)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("Decode fallback response error = %v body=%s", err, body)
	}
	if out["object"] != "response" || out["status"] != "completed" {
		t.Fatalf("fallback response = %#v, want completed response object", out)
	}
	if out["output_text"] != "fallback result" {
		t.Fatalf("output_text = %#v, want fallback result", out["output_text"])
	}
}

func TestResponsesLLMAPICodexReturnsCompactResponsesErrorOnTransientFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	previousProviderBaseURL := embeddedCLIProxyProviderBaseURL
	previousAuthStatus := embeddedCLIProxyAuthStatus
	defer func() {
		embeddedCLIProxyProviderBaseURL = previousProviderBaseURL
		embeddedCLIProxyAuthStatus = previousAuthStatus
	}()

	var responsesCalls int
	var chatCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/provider/codex/v1/responses":
			responsesCalls++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"Post \"https://chatgpt.com/backend-api/codex/responses\": EOF","type":"server_error","code":"internal_server_error"}}`))
		case "/api/provider/codex/v1/chat/completions":
			chatCalls++
			http.Error(w, "codex chat fallback should not be called", http.StatusTeapot)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
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
				ModelID:         "gpt-5.5",
				ProfileComplete: true,
			},
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	resp, err := svc.Responses(context.Background(), agent.ManagerUserID, []byte(`{"model":"client-model","input":"hello","stream":false}`))
	if err != nil {
		t.Fatalf("Responses() error = %v", err)
	}
	defer resp.Body.Close()

	if responsesCalls != 1 {
		t.Fatalf("/responses calls = %d, want 1", responsesCalls)
	}
	if chatCalls != 0 {
		t.Fatalf("/chat/completions calls = %d, want 0", chatCalls)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	want := `Post "https://chatgpt.com/backend-api/codex/responses": EOF (type=server_error, code=internal_server_error)`
	if string(body) != want {
		t.Fatalf("body = %q, want %q", string(body), want)
	}
}

func TestResponsesLLMAPIFallbackMapsDeveloperRoleToSystem(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var gotChatMessages []map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			http.Error(w, "no responses here", http.StatusNotFound)
		case "/v1/chat/completions":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode() chat payload error = %v", err)
			}
			messages, _ := payload["messages"].([]any)
			for _, msg := range messages {
				if m, ok := msg.(map[string]any); ok {
					gotChatMessages = append(gotChatMessages, m)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"chatcmpl-fallback","object":"chat.completion","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer upstream.Close()

	agentSvc := mustSeededAgentService(t, config.LLMConfig{}, []agent.Agent{
		{
			ID:   agent.ManagerUserID,
			Name: agent.ManagerName,
			Role: agent.RoleManager,
			AgentProfile: agent.AgentProfile{
				Name:            agent.ManagerName,
				Provider:        agent.ProviderAPI,
				BaseURL:         upstream.URL + "/v1",
				APIKey:          "sk-test",
				ModelID:         "deepseek-v4-flash",
				ProfileComplete: true,
			},
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	resp, err := svc.Responses(context.Background(), agent.ManagerUserID, []byte(`{"model":"client-model","input":[{"role":"user","content":"hello"},{"role":"developer","content":"follow repo rules"}],"stream":false}`))
	if err != nil {
		t.Fatalf("Responses() error = %v", err)
	}
	defer resp.Body.Close()

	if len(gotChatMessages) != 2 {
		t.Fatalf("chat messages = %#v, want 2 messages", gotChatMessages)
	}
	if gotChatMessages[0]["role"] != "user" {
		t.Fatalf("first chat role = %#v, want user", gotChatMessages[0]["role"])
	}
	if gotChatMessages[1]["role"] != "system" {
		t.Fatalf("developer role mapped to %#v, want system; messages=%#v", gotChatMessages[1]["role"], gotChatMessages)
	}
}

func TestResponsesLLMAPIFallbackCachesUnsupportedResponsesAPI(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var responsesCalls int
	var chatCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			responsesCalls++
			http.Error(w, "no responses here", http.StatusNotFound)
		case "/v1/chat/completions":
			chatCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"chatcmpl-fallback","object":"chat.completion","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer upstream.Close()

	agentSvc := mustSeededAgentService(t, config.LLMConfig{}, []agent.Agent{
		{
			ID:   agent.ManagerUserID,
			Name: agent.ManagerName,
			Role: agent.RoleManager,
			AgentProfile: agent.AgentProfile{
				Name:            agent.ManagerName,
				Provider:        agent.ProviderAPI,
				BaseURL:         upstream.URL + "/v1",
				APIKey:          "sk-test",
				ModelID:         "deepseek-v4-flash",
				ProfileComplete: true,
			},
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	for i := 0; i < 2; i++ {
		resp, err := svc.Responses(context.Background(), agent.ManagerUserID, []byte(`{"model":"client-model","input":"hello","stream":false}`))
		if err != nil {
			t.Fatalf("Responses() attempt %d error = %v", i+1, err)
		}
		_ = resp.Body.Close()
	}
	if responsesCalls != 1 {
		t.Fatalf("/responses calls = %d, want 1 after unsupported cache", responsesCalls)
	}
	if chatCalls != 2 {
		t.Fatalf("/chat/completions calls = %d, want 2", chatCalls)
	}
}

func TestResponsesLLMAPIFallsBackToStreamingChatCompletionsWhenUnsupported(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var gotChatStream any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			http.Error(w, "no responses here", http.StatusMethodNotAllowed)
		case "/v1/chat/completions":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode() chat payload error = %v", err)
			}
			gotChatStream = payload["stream"]
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-fallback\",\"object\":\"chat.completion.chunk\",\"created\":1710000000,\"model\":\"deepseek-v4-pro\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"fallback\"},\"finish_reason\":null}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-fallback\",\"object\":\"chat.completion.chunk\",\"created\":1710000000,\"model\":\"deepseek-v4-pro\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" stream\"},\"finish_reason\":null}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-fallback\",\"object\":\"chat.completion.chunk\",\"created\":1710000000,\"model\":\"deepseek-v4-pro\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer upstream.Close()

	agentSvc := mustSeededAgentService(t, config.LLMConfig{}, []agent.Agent{
		{
			ID:   agent.ManagerUserID,
			Name: agent.ManagerName,
			Role: agent.RoleManager,
			AgentProfile: agent.AgentProfile{
				Name:            agent.ManagerName,
				Provider:        agent.ProviderAPI,
				BaseURL:         upstream.URL + "/v1",
				APIKey:          "sk-test",
				ModelID:         "deepseek-v4-pro",
				ProfileComplete: true,
			},
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	resp, err := svc.Responses(context.Background(), agent.ManagerUserID, []byte(`{"model":"client-model","input":"hello","stream":true}`))
	if err != nil {
		t.Fatalf("Responses() error = %v", err)
	}
	defer resp.Body.Close()
	if gotChatStream != true {
		t.Fatalf("chat stream = %#v, want true", gotChatStream)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"event: response.output_text.delta",
		`"delta":"fallback"`,
		`"delta":" stream"`,
		"event: response.completed",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("stream body missing %q:\n%s", want, text)
		}
	}
}

func TestResponsesLLMAPIFallbackAllowsAdvertisedToolsForTextOnlyRequests(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var gotChatPayload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			http.Error(w, "no responses here", http.StatusNotFound)
		case "/v1/chat/completions":
			if err := json.NewDecoder(r.Body).Decode(&gotChatPayload); err != nil {
				t.Fatalf("Decode() chat payload error = %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"chatcmpl-fallback","object":"chat.completion","created":1710000000,"model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer upstream.Close()

	agentSvc := mustSeededAgentService(t, config.LLMConfig{}, []agent.Agent{
		{
			ID:   agent.ManagerUserID,
			Name: agent.ManagerName,
			Role: agent.RoleManager,
			AgentProfile: agent.AgentProfile{
				Name:            agent.ManagerName,
				Provider:        agent.ProviderAPI,
				BaseURL:         upstream.URL + "/v1",
				APIKey:          "sk-test",
				ModelID:         "deepseek-v4-pro",
				ProfileComplete: true,
			},
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	svc := NewService(config.ModelConfig{}, agentSvc)
	resp, err := svc.Responses(context.Background(), agent.ManagerUserID, []byte(`{"model":"client-model","input":"hi","tools":[{"type":"function","name":"shell","description":"run shell"}],"stream":false}`))
	if err != nil {
		t.Fatalf("Responses() error = %v", err)
	}
	defer resp.Body.Close()

	if _, ok := gotChatPayload["tools"]; ok {
		t.Fatalf("chat fallback payload includes tools: %#v", gotChatPayload["tools"])
	}
	messages, _ := gotChatPayload["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("chat messages = %#v, want single text message", messages)
	}
	if msg, _ := messages[0].(map[string]any); msg["role"] != "user" || msg["content"] != "hi" {
		t.Fatalf("chat messages = %#v, want user hi", messages)
	}
}

func TestResponsesLLMAPIFallbackRejectsActiveToolSemantics(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{
			name: "tool output",
			body: `{"model":"client-model","input":[{"type":"function_call_output","call_id":"call-1","output":"done"}],"stream":false}`,
		},
		{
			name: "required tool choice",
			body: `{"model":"client-model","input":"hello","tools":[{"type":"function","name":"shell","description":"run shell"}],"tool_choice":"required","stream":false}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			var chatCalls int
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/responses":
					http.Error(w, "no responses here", http.StatusNotFound)
				case "/v1/chat/completions":
					chatCalls++
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"id":"chatcmpl-fallback","object":"chat.completion","created":1710000000,"model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"fallback result"},"finish_reason":"stop"}]}`))
				default:
					t.Fatalf("unexpected path %q", r.URL.Path)
				}
			}))
			defer upstream.Close()

			agentSvc := mustSeededAgentService(t, config.LLMConfig{}, []agent.Agent{
				{
					ID:   agent.ManagerUserID,
					Name: agent.ManagerName,
					Role: agent.RoleManager,
					AgentProfile: agent.AgentProfile{
						Name:            agent.ManagerName,
						Provider:        agent.ProviderAPI,
						BaseURL:         upstream.URL + "/v1",
						APIKey:          "sk-test",
						ModelID:         "deepseek-v4-pro",
						ProfileComplete: true,
					},
					CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
				},
			})

			svc := NewService(config.ModelConfig{}, agentSvc)
			resp, err := svc.Responses(context.Background(), agent.ManagerUserID, []byte(tc.body))
			if resp != nil {
				_ = resp.Body.Close()
			}
			if err == nil {
				t.Fatal("Responses() error = nil, want explicit tool-bearing fallback rejection")
			}
			httpErr, ok := err.(*HTTPError)
			if !ok {
				t.Fatalf("Responses() error type = %T, want *HTTPError", err)
			}
			if httpErr.Status != http.StatusBadRequest {
				t.Fatalf("HTTP status = %d, want %d", httpErr.Status, http.StatusBadRequest)
			}
			if !strings.Contains(httpErr.Message, "Responses tool-use history") || !strings.Contains(httpErr.Message, "start a new session") {
				t.Fatalf("error message = %q, want actionable tool-use fallback rejection", httpErr.Message)
			}
			if chatCalls != 0 {
				t.Fatalf("/chat/completions calls = %d, want 0", chatCalls)
			}
		})
	}
}

func TestModelsReturnsConservativeMetadataForOpenAICompatibleProvider(t *testing.T) {
	agentSvc := mustSeededAgentService(t, config.SingleProfileLLM(config.ModelConfig{
		Provider:        config.ProviderLLMAPI,
		BaseURL:         "https://example.test/v1",
		APIKey:          "sk-test",
		ModelID:         "gpt-5.4-mini",
		ReasoningEffort: "high",
	}), []agent.Agent{
		{
			ID:          agent.ManagerUserID,
			Name:        agent.ManagerName,
			Role:        agent.RoleManager,
			Profile:     config.DefaultLLMProfile,
			RuntimeKind: agent.RuntimeKindPicoClawSandbox,
			AgentProfile: agent.AgentProfile{
				Name:            agent.ManagerName,
				Provider:        agent.ProviderAPI,
				BaseURL:         "https://example.test/v1",
				APIKey:          "sk-test",
				ModelID:         "gpt-5.4-mini",
				ReasoningEffort: "high",
				ProfileComplete: true,
			},
			ProfileComplete: true,
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
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode models body: %v", err)
	}
	models, ok := payload["models"].([]any)
	if !ok || len(models) != 1 {
		t.Fatalf("models = %#v, want one model metadata entry", payload["models"])
	}
	model, ok := models[0].(map[string]any)
	if !ok {
		t.Fatalf("models[0] = %#v, want object", models[0])
	}
	if model["slug"] != "gpt-5.4-mini" {
		t.Fatalf("models[0].slug = %#v, want gpt-5.4-mini", model["slug"])
	}
	if _, ok := model["model_messages"]; ok {
		t.Fatalf("models[0] should not expose Codex model_messages for OpenAI-compatible provider: %#v", model)
	}
	if got, want := model["default_reasoning_level"], "none"; got != want {
		t.Fatalf("default_reasoning_level = %#v, want %#v", got, want)
	}
	if levels, ok := model["supported_reasoning_levels"].([]any); !ok || len(levels) != 0 {
		t.Fatalf("supported_reasoning_levels = %#v, want empty list", model["supported_reasoning_levels"])
	}
	modalities, ok := model["input_modalities"].([]any)
	if !ok || len(modalities) != 1 || modalities[0] != "text" {
		t.Fatalf("input_modalities = %#v, want text-only", model["input_modalities"])
	}
	if got, want := model["supports_reasoning_summaries"], false; got != want {
		t.Fatalf("supports_reasoning_summaries = %#v, want %#v", got, want)
	}
}

func TestModelsReturnsCodexMetadataForCodexProvider(t *testing.T) {
	agentSvc := mustSeededAgentService(t, config.LLMConfig{}, []agent.Agent{
		{
			ID:   agent.ManagerUserID,
			Name: agent.ManagerName,
			Role: agent.RoleManager,
			AgentProfile: agent.AgentProfile{
				Name:            agent.ManagerName,
				Provider:        agent.ProviderCodex,
				ModelID:         "gpt-5.5",
				ReasoningEffort: "high",
				ProfileComplete: true,
			},
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
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
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode models body: %v", err)
	}
	models, ok := payload["models"].([]any)
	if !ok || len(models) != 1 {
		t.Fatalf("models = %#v, want one model metadata entry", payload["models"])
	}
	model, ok := models[0].(map[string]any)
	if !ok {
		t.Fatalf("models[0] = %#v, want object", models[0])
	}
	if model["slug"] != "gpt-5.5" {
		t.Fatalf("models[0].slug = %#v, want gpt-5.5", model["slug"])
	}
	if _, ok := model["model_messages"]; !ok {
		t.Fatalf("models[0] missing Codex model_messages: %#v", model)
	}
	if got, want := model["default_reasoning_level"], "high"; got != want {
		t.Fatalf("default_reasoning_level = %#v, want %#v", got, want)
	}
	if levels, ok := model["supported_reasoning_levels"].([]any); !ok || len(levels) == 0 {
		t.Fatalf("supported_reasoning_levels = %#v, want Codex reasoning levels", model["supported_reasoning_levels"])
	}
	modalities, ok := model["input_modalities"].([]any)
	if !ok || len(modalities) != 2 || modalities[0] != "text" || modalities[1] != "image" {
		t.Fatalf("input_modalities = %#v, want text and image", model["input_modalities"])
	}
}

func inputContainsReasoning(input []any) bool {
	for _, item := range input {
		if responseInputItemIsReasoning(item) {
			return true
		}
	}
	return false
}

func mustSeededAgentService(t *testing.T, llmCfg config.LLMConfig, agents []agent.Agent) *agent.Service {
	t.Helper()

	for i := range agents {
		if strings.TrimSpace(agents[i].RuntimeKind) == "" {
			agents[i].RuntimeKind = agent.RuntimeKindPicoClawSandbox
		}
	}

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

	svc, err := agent.NewServiceWithLLM(llmCfg, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}

func TestNormalizeCompletionTokenLimitsRaisesSymmetric32768Budget(t *testing.T) {
	payload := map[string]any{
		"max_tokens": float64(32768),
	}
	normalizeCompletionTokenLimits(payload)
	if got, want := payloadInt(payload, "max_tokens"), 33792; got != want {
		t.Fatalf("max_tokens = %d, want %d", got, want)
	}
	if got, want := payloadInt(payload, "max_completion_tokens"), 33792; got != want {
		t.Fatalf("max_completion_tokens = %d, want %d", got, want)
	}
}

func TestNormalizeCompletionTokenLimitsRaises16384ForReasoningModel(t *testing.T) {
	payload := map[string]any{
		"model":      "glm-5.1",
		"max_tokens": float64(16384),
	}
	normalizeCompletionTokenLimits(payload)
	if got, want := payloadInt(payload, "max_tokens"), 33792; got != want {
		t.Fatalf("max_tokens = %d, want %d", got, want)
	}
}

func TestNormalizeCompletionTokenLimitsEnsuresMaxAboveThinkingBudget(t *testing.T) {
	payload := map[string]any{
		"max_completion_tokens": float64(32768),
		"thinking_budget":       float64(32768),
	}
	normalizeCompletionTokenLimits(payload)
	if got, want := payloadInt(payload, "max_completion_tokens"), 33792; got != want {
		t.Fatalf("max_completion_tokens = %d, want %d", got, want)
	}
}

func TestNormalizeCompletionTokenLimitsLeavesSmallNonReasoningLimits(t *testing.T) {
	payload := map[string]any{
		"model":      "gpt-4.1-mini",
		"max_tokens": float64(4096),
	}
	normalizeCompletionTokenLimits(payload)
	if got, want := payloadInt(payload, "max_tokens"), 4096; got != want {
		t.Fatalf("max_tokens = %d, want %d", got, want)
	}
}
