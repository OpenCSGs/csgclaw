package codex

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"csgclaw/internal/modelpreset"
	"csgclaw/internal/modelprovider"
	agentruntime "csgclaw/internal/runtime"
)

func TestReconcileConfigRotatesConversationsWhenModelChanges(t *testing.T) {
	root := t.TempDir()
	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "agent-manager", Name: "manager", RuntimeID: h.RuntimeID}, nil
	})
	runtimeID := "rt-agent-manager"
	if err := rt.mkdirAll(filepath.Join(root, "agent-manager", hostStateDirName), 0o755); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}
	if err := rt.writeSessionMetadata(sessionMetadata{
		DynamicToolsVersion:         engineDynamicToolsVersion,
		RuntimeID:                   runtimeID,
		ConversationSessions:        map[string]string{"room-1": "gpt-thread"},
		FilePublishingConversations: map[string]bool{"room-1": true},
	}); err != nil {
		t.Fatalf("writeSessionMetadata() error = %v", err)
	}

	err := rt.ReconcileConfig(context.Background(), agentruntime.Handle{RuntimeID: runtimeID}, agentruntime.RuntimeConfigChange{
		Previous: agentruntime.RuntimeConfigSnapshot{Profile: agentruntime.RuntimeProfileConfig{Provider: "codex", BaseURL: "https://old.example/v1", ModelID: "gpt-5"}},
		Current:  agentruntime.RuntimeConfigSnapshot{Profile: agentruntime.RuntimeProfileConfig{Provider: "csghub", BaseURL: "https://new.example/v1/", ModelID: "glm-5.1"}},
	})
	if err != nil {
		t.Fatalf("ReconcileConfig() error = %v", err)
	}
	meta, err := rt.readSessionMetadata(runtimeID)
	if err != nil {
		t.Fatalf("readSessionMetadata() error = %v", err)
	}
	if len(meta.ConversationSessions) != 0 || len(meta.FilePublishingConversations) != 0 {
		t.Fatalf("persisted conversations = %#v, publishing = %#v; want both cleared", meta.ConversationSessions, meta.FilePublishingConversations)
	}
}

func TestReconcileConfigPreservesConversationsWhenOnlyReasoningChanges(t *testing.T) {
	root := t.TempDir()
	rt := newTestCodexRuntime(root, func(h agentruntime.Handle) (AgentRef, error) {
		return AgentRef{ID: "agent-manager", Name: "manager", RuntimeID: h.RuntimeID}, nil
	})
	runtimeID := "rt-agent-manager"
	if err := rt.mkdirAll(filepath.Join(root, "agent-manager", hostStateDirName), 0o755); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}
	if err := rt.writeSessionMetadata(sessionMetadata{
		DynamicToolsVersion:         engineDynamicToolsVersion,
		RuntimeID:                   runtimeID,
		ConversationSessions:        map[string]string{"room-1": "existing-thread"},
		FilePublishingConversations: map[string]bool{"room-1": true},
	}); err != nil {
		t.Fatalf("writeSessionMetadata() error = %v", err)
	}

	err := rt.ReconcileConfig(context.Background(), agentruntime.Handle{RuntimeID: runtimeID}, agentruntime.RuntimeConfigChange{
		Previous: agentruntime.RuntimeConfigSnapshot{Profile: agentruntime.RuntimeProfileConfig{Provider: "api", BaseURL: "https://example.test/v1", ModelID: "gpt-5", ReasoningEffort: "low"}},
		Current:  agentruntime.RuntimeConfigSnapshot{Profile: agentruntime.RuntimeProfileConfig{Provider: "api", BaseURL: "https://example.test/v1/", ModelID: "gpt-5", ReasoningEffort: "high"}},
	})
	if err != nil {
		t.Fatalf("ReconcileConfig() error = %v", err)
	}
	meta, err := rt.readSessionMetadata(runtimeID)
	if err != nil {
		t.Fatalf("readSessionMetadata() error = %v", err)
	}
	if got := meta.ConversationSessions["room-1"]; got != "existing-thread" {
		t.Fatalf("conversation thread = %q, want existing-thread", got)
	}
	if !meta.FilePublishingConversations["room-1"] {
		t.Fatal("file publishing conversation was cleared for a reasoning-only change")
	}
}

func TestValidateConfigResolvesOpenCSGCredentialsForResponsesProbe(t *testing.T) {
	restoreProbe := TestOnlySetResponsesAPIProbe(func(_ context.Context, baseURL, apiKey, modelID string, headers map[string]string) error {
		if baseURL != "https://ai.space.opencsg.com/v1" {
			t.Fatalf("probe baseURL = %q, want OpenCSG AIGateway", baseURL)
		}
		if apiKey != "gateway-key" {
			t.Fatalf("probe apiKey = %q, want gateway-key", apiKey)
		}
		if modelID != "glm-5.1" {
			t.Fatalf("probe modelID = %q, want glm-5.1", modelID)
		}
		if len(headers) != 0 {
			t.Fatalf("probe headers = %#v, want empty", headers)
		}
		return nil
	})
	defer restoreProbe()

	previousCreds := openCSGCredentialsForResponsesProbe
	openCSGCredentialsForResponsesProbe = func(context.Context) (string, string, bool, error) {
		return "https://ai.space.opencsg.com/v1", "gateway-key", true, nil
	}
	defer func() {
		openCSGCredentialsForResponsesProbe = previousCreds
	}()

	rt := &Runtime{}
	err := rt.ValidateConfig(context.Background(), agentruntime.RuntimeConfigSnapshot{
		Profile: agentruntime.RuntimeProfileConfig{
			Provider: "csghub",
			ModelID:  "glm-5.1",
		},
	})
	if err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}

func TestValidateConfigRejectsProviderWithoutUsableResponsesOrChatEndpoint(t *testing.T) {
	restoreProbe := TestOnlySetResponsesAPIProbe(func(context.Context, string, string, string, map[string]string) error {
		return modelprovider.ErrResponsesAPIUnsupported
	})
	defer restoreProbe()

	rt := &Runtime{}
	err := rt.ValidateConfig(context.Background(), agentruntime.RuntimeConfigSnapshot{
		Profile: agentruntime.RuntimeProfileConfig{
			Provider: "api",
			BaseURL:  "https://wrong.example/v1",
			APIKey:   "test-key",
			ModelID:  "qwen-test",
		},
	})
	if !errors.Is(err, modelprovider.ErrResponsesAPIUnsupported) {
		t.Fatalf("ValidateConfig() error = %v, want unsupported provider rejection", err)
	}
}

func TestValidateConfigChecksSelectedModelProtocol(t *testing.T) {
	var messagesCalls, chatCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil || body.Model != "qwen3.8-flash" {
			t.Errorf("unexpected probe request: %s %s, model %q", r.Method, r.URL.Path, body.Model)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/v1/responses":
			_, _ = w.Write([]byte(`{"object":"response","status":"completed"}`))
		case "/v1/chat/completions":
			if r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("X-Test-Header") != "custom" {
				t.Error("missing Chat Completions probe headers")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			chatCalls.Add(1)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"OK"}}]}`))
		case "/v1/messages":
			if r.Header.Get("X-Api-Key") != "test-key" || r.Header.Get("Anthropic-Version") != "2023-06-01" || r.Header.Get("X-Test-Header") != "custom" {
				t.Error("missing Messages probe headers")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if messagesCalls.Add(1) == 1 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"type":"message","content":[{"type":"text","text":"OK"}]}`))
		default:
			t.Errorf("unexpected probe path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	rt := &Runtime{}
	config := agentruntime.RuntimeConfigSnapshot{Profile: agentruntime.RuntimeProfileConfig{
		Provider:    "api",
		BaseURL:     upstream.URL + "/v1",
		APIKey:      "test-key",
		ModelID:     "qwen3.8-flash",
		APIProtocol: modelpreset.Responses,
		Headers:     map[string]string{"X-Test-Header": "custom"},
	}}
	ctx := context.Background()
	if err := rt.ValidateConfig(ctx, config); err != nil {
		t.Fatalf("validate Responses: %v", err)
	}
	config.Profile.APIProtocol = modelpreset.AnthropicMessages
	err := rt.ValidateConfig(ctx, config)
	status, code, _, ok := modelprovider.UserFacingUpstreamError(err)
	if !ok || status != http.StatusNotFound || code != "model_not_found" {
		t.Fatalf("validate unavailable Messages model: %v (status %d, code %q)", err, status, code)
	}
	for i := 0; i < 2; i++ {
		if err := rt.ValidateConfig(ctx, config); err != nil {
			t.Fatalf("validate Messages: %v", err)
		}
	}
	if calls := messagesCalls.Load(); calls != 2 {
		t.Fatalf("Messages requests = %d, want a failed probe and one successful retry", calls)
	}
	config.Profile.APIProtocol = modelpreset.ChatCompletions
	if err := rt.ValidateConfig(ctx, config); err != nil {
		t.Fatalf("validate Chat Completions: %v", err)
	}
	if calls := chatCalls.Load(); calls != 1 {
		t.Fatalf("Chat Completions requests = %d, want 1", calls)
	}
}

func TestRestartRequiredWhenMemoryModeChanges(t *testing.T) {
	rt := &Runtime{}
	restart, err := rt.RestartRequired(agentruntime.RuntimeConfigChange{
		Previous: agentruntime.RuntimeConfigSnapshot{Options: map[string]any{memoryModeOptionKey: MemoryModeEnabled}},
		Current:  agentruntime.RuntimeConfigSnapshot{Options: map[string]any{memoryModeOptionKey: MemoryModeDisabled}},
	})
	if err != nil {
		t.Fatalf("RestartRequired() error = %v", err)
	}
	if !restart {
		t.Fatal("RestartRequired() = false, want true for memory mode change")
	}
}

func TestValidateConfigDefersModelCheck(t *testing.T) {
	t.Cleanup(TestOnlySetResponsesAPIProbe(func(context.Context, string, string, string, map[string]string) error {
		t.Fatal("model probe called during platform template creation")
		return nil
	}))
	rt := &Runtime{}
	if err := rt.ValidateConfig(context.Background(), agentruntime.RuntimeConfigSnapshot{SkipModelAvailabilityCheck: true}); err != nil {
		t.Fatal(err)
	}
}

func TestDeferredModelCheckStillValidatesRuntimeOptions(t *testing.T) {
	t.Setenv("CSGCLAW_EXECUTION_MODE", ExecutionModeStandard)
	rt := &Runtime{}
	err := rt.ValidateConfig(context.Background(), agentruntime.RuntimeConfigSnapshot{
		SkipModelAvailabilityCheck: true,
		Options:                    map[string]any{"memory_mode": "invalid"},
	})
	if err == nil {
		t.Fatal("invalid runtime options accepted")
	}
}
