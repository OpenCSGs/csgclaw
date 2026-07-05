package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadyzWaitsForGatewayReady(t *testing.T) {
	state := &gatewayState{}
	handler := newHealthHandler(state)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/health status = %d, want %d", rec.Code, http.StatusOK)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz before ready status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	state.setReady(true)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/readyz after ready status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRunReadyWithoutFeishuConfigDoesNotStartBridge(t *testing.T) {
	origCSGClaw := startCSGClawBridgeFunc
	origLark := startLarkBridgeFunc
	t.Cleanup(func() {
		startCSGClawBridgeFunc = origCSGClaw
		startLarkBridgeFunc = origLark
	})
	startCSGClawBridgeFunc = func(_ context.Context, cfg gatewayRuntimeConfig) (*runningCSGClawBridge, error) {
		if got, want := cfg.BaseURL, "http://127.0.0.1:18080"; got != want {
			t.Fatalf("CSGClaw bridge base URL = %q, want %q", got, want)
		}
		if got, want := cfg.ParticipantID, "pt-dev"; got != want {
			t.Fatalf("CSGClaw bridge participant ID = %q, want %q", got, want)
		}
		return &runningCSGClawBridge{}, nil
	}
	startLarkBridgeFunc = func(context.Context, larkBridgeConfig) (<-chan error, error) {
		t.Fatal("lark-channel-bridge should not start without Feishu config")
		return nil, nil
	}

	t.Setenv("LARK_CHANNEL_CONFIG", filepath.Join(t.TempDir(), "missing-config.json"))
	t.Setenv("LARK_CHANNEL_PROFILE", "csgclaw")
	t.Setenv(appSecretEnvKey, "")
	t.Setenv("CSGCLAW_BASE_URL", "http://127.0.0.1:18080")
	t.Setenv("CSGCLAW_ACCESS_TOKEN", "token")
	t.Setenv("CSGCLAW_PARTICIPANT_ID", "pt-dev")
	t.Setenv("CSGCLAW_AGENT_ID", "u-dev")
	t.Setenv("CSGCLAW_LLM_BASE_URL", "http://127.0.0.1:18080/api/v1/agents/u-dev/llm")
	t.Setenv("CSGCLAW_LLM_API_KEY", "token")
	t.Setenv("CSGCLAW_LLM_MODEL_ID", "gpt-5.5")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	state := &gatewayState{}
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, state)
	}()

	deadline := time.After(2 * time.Second)
	for !state.isReady() {
		select {
		case err := <-done:
			t.Fatalf("run() returned before ready: %v", err)
		case <-deadline:
			t.Fatal("gateway did not become ready without Feishu config")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if state.isBridgeStarted() {
		t.Fatal("bridge started without Feishu config")
	}
	if !state.isCSGClawStarted() {
		t.Fatal("CSGClaw bridge did not start")
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("run() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run() did not stop after context cancellation")
	}
}

func TestRunDoesNotBecomeReadyWhenCSGClawBridgeFails(t *testing.T) {
	origCSGClaw := startCSGClawBridgeFunc
	t.Cleanup(func() {
		startCSGClawBridgeFunc = origCSGClaw
	})
	startCSGClawBridgeFunc = func(context.Context, gatewayRuntimeConfig) (*runningCSGClawBridge, error) {
		return nil, fmt.Errorf("boom")
	}

	state := &gatewayState{}
	err := run(context.Background(), state)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("run() error = %v, want boom", err)
	}
	if state.isReady() {
		t.Fatal("gateway became ready after CSGClaw bridge failure")
	}
}

func TestGatewayRuntimeConfigFromEnvDefaultsLLMBaseURL(t *testing.T) {
	t.Setenv("CSGCLAW_BASE_URL", "http://127.0.0.1:18080/")
	t.Setenv("CSGCLAW_ACCESS_TOKEN", "token")
	t.Setenv("CSGCLAW_AGENT_ID", "u-dev")
	t.Setenv("CSGCLAW_PARTICIPANT_ID", "pt-dev")
	t.Setenv("CSGCLAW_LLM_MODEL_ID", "gpt-5.5")

	cfg := gatewayRuntimeConfigFromEnv("/home/codex/.codex-sandbox", "/workspace", "/usr/local/bin/codex").normalized()
	if got, want := cfg.LLMBaseURL, "http://127.0.0.1:18080/api/v1/agents/u-dev/llm"; got != want {
		t.Fatalf("LLMBaseURL = %q, want %q", got, want)
	}
	if got, want := cfg.LLMAPIKey, "token"; got != want {
		t.Fatalf("LLMAPIKey = %q, want %q", got, want)
	}
	if got, want := cfg.RuntimeID, "rt-u-dev"; got != want {
		t.Fatalf("RuntimeID = %q, want %q", got, want)
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate() error = %v", err)
	}
}

func TestGatewayRuntimeConfigAllowsEmptyLLMAPIKey(t *testing.T) {
	cfg := gatewayRuntimeConfig{
		Home:          "/home/codex/.codex-sandbox",
		Workspace:     "/home/codex/.codex-sandbox/workspace/projects",
		CodexHome:     "/home/codex/.codex-sandbox/codex-home",
		CodexBin:      "/usr/local/bin/codex",
		BaseURL:       "http://127.0.0.1:18080",
		ParticipantID: "pt-dev",
		AgentID:       "u-dev",
		RuntimeID:     "rt-u-dev",
		LLMBaseURL:    "http://127.0.0.1:18080/api/v1/agents/u-dev/llm",
		ModelID:       "gpt-5.5",
	}.normalized()
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate() error = %v", err)
	}
}

func TestCredentialsReadyRequiresEnvBackedAppSecret(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	data := `{
  "activeProfile": "csgclaw",
  "profiles": {
    "csgclaw": {
      "accounts": {
        "app": {
          "id": "cli_dev",
          "secret": "${APP_SECRET}"
        }
      }
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(data), 0o600); err != nil {
		t.Fatalf("WriteFile(config.json) error = %v", err)
	}

	t.Setenv(appSecretEnvKey, "")
	ready, reason := credentialsReady(configPath, "csgclaw")
	if ready {
		t.Fatalf("credentialsReady() ready = true, want false")
	}
	if !strings.Contains(reason, appSecretEnvKey) {
		t.Fatalf("credentialsReady() reason = %q, want %s", reason, appSecretEnvKey)
	}

	t.Setenv(appSecretEnvKey, "dev-secret")
	ready, reason = credentialsReady(configPath, "csgclaw")
	if !ready {
		t.Fatalf("credentialsReady() ready = false, reason = %q", reason)
	}
}
