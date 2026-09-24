package codex

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

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
