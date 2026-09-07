package api

import (
	"context"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGetAgentFeishuAppInfoReturnsStoredSecret(t *testing.T) {
	svc := mustNewSeededServiceWithOptions(t, []agent.Agent{{
		ID:              "u-dev",
		Name:            "dev",
		Role:            agent.RoleWorker,
		RuntimeKind:     agent.RuntimeKindCodex,
		ProfileComplete: true,
		CreatedAt:       time.Now().UTC(),
	}}, agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}))
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "pt-dev",
		Channel:         participant.ChannelFeishu,
		Type:            participant.TypeAgent,
		Name:            "dev",
		ChannelUserKind: participant.ChannelUserKindAppID,
		ChannelAppConfig: map[string]any{
			"app_id":     "cli_dev",
			"app_secret": "dev-secret",
		},
		AgentID:         "agent-dev",
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}), participant.WithAgentEngine(agentengine.New(svc)))
	srv := &Handler{svc: svc, participant: participantSvc, serverAccessToken: "server-secret", agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc}
	token, err := srv.larkCLISourceAccessToken("agent-dev")
	if err != nil {
		t.Fatalf("larkCLISourceAccessToken() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-dev/feishu/app-info", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got apitypes.FeishuBotAppInfo
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.AgentID != "agent-dev" || got.ParticipantID != "pt-dev" || got.AppID != "cli_dev" || got.AppSecret != "dev-secret" {
		t.Fatalf("app info = %#v", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestGetAgentFeishuAppInfoRejectsGlobalAndOtherAgentSourceToken(t *testing.T) {
	svc := mustNewSeededServiceWithOptions(t, []agent.Agent{
		{
			ID:              "u-dev",
			Name:            "dev",
			Role:            agent.RoleWorker,
			RuntimeKind:     agent.RuntimeKindCodex,
			ProfileComplete: true,
			CreatedAt:       time.Now().UTC(),
		},
		{
			ID:              "u-qa",
			Name:            "qa",
			Role:            agent.RoleWorker,
			RuntimeKind:     agent.RuntimeKindCodex,
			ProfileComplete: true,
			CreatedAt:       time.Now().UTC(),
		},
	}, agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}))
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{
		{
			ID:              "pt-dev",
			Channel:         participant.ChannelFeishu,
			Type:            participant.TypeAgent,
			Name:            "dev",
			ChannelUserKind: participant.ChannelUserKindAppID,
			ChannelAppConfig: map[string]any{
				"app_id":     "cli_dev",
				"app_secret": "dev-secret",
			},
			AgentID:         "agent-dev",
			LifecycleStatus: participant.LifecycleStatusActive,
			Mentionable:     true,
		},
		{
			ID:              "pt-qa",
			Channel:         participant.ChannelFeishu,
			Type:            participant.TypeAgent,
			Name:            "qa",
			ChannelUserKind: participant.ChannelUserKindAppID,
			ChannelAppConfig: map[string]any{
				"app_id":     "cli_qa",
				"app_secret": "qa-secret",
			},
			AgentID:         "agent-qa",
			LifecycleStatus: participant.LifecycleStatusActive,
			Mentionable:     true,
		},
	}), participant.WithAgentEngine(agentengine.New(svc)))
	srv := &Handler{svc: svc, participant: participantSvc, serverAccessToken: "server-secret", agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc}
	qaToken, err := srv.larkCLISourceAccessToken("agent-qa")
	if err != nil {
		t.Fatalf("larkCLISourceAccessToken() error = %v", err)
	}

	for _, auth := range []string{"Bearer server-secret", "Bearer " + qaToken} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-dev/feishu/app-info", nil)
		req.Header.Set("Authorization", auth)
		rec := httptest.NewRecorder()

		srv.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("auth %q status = %d, want %d; body=%s", auth, rec.Code, http.StatusUnauthorized, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "dev-secret") {
			t.Fatalf("unauthorized response leaked secret: %s", rec.Body.String())
		}
	}
}

func TestInitAgentLarkCLIReturnsConflictWhenFeishuBotMissing(t *testing.T) {
	svc := mustNewSeededServiceWithOptions(t, []agent.Agent{{
		ID:              "u-dev",
		Name:            "dev",
		Role:            agent.RoleWorker,
		RuntimeKind:     agent.RuntimeKindCodex,
		RuntimeID:       "rt-dev",
		BoxID:           "codex-session-dev",
		Status:          string(agentruntime.StateRunning),
		ProfileComplete: true,
		CreatedAt:       time.Now().UTC(),
	}}, agent.WithRuntime(fakeCompatRuntime{
		kind: agent.RuntimeKindCodex,
		stop: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
			return agentruntime.StateStopped, nil
		},
		start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
			return agentruntime.StateRunning, nil
		},
	}))
	srv := &Handler{
		svc:               svc,
		participant:       participant.NewService(participant.NewMemoryStore(nil), participant.WithAgentEngine(agentengine.New(svc))),
		serverAccessToken: "server-secret", agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/u-dev/lark-cli:init", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	assertAPIErrorCode(t, rec, http.StatusConflict, feishuBotNotConfiguredCode)
}

func TestInternalSourceBaseURLUsesOnlyConfiguredOrDefaultAddress(t *testing.T) {
	configured := (&Handler{internalBaseURL: "http://127.0.0.1:19090/"}).internalSourceBaseURL()
	if configured != "http://127.0.0.1:19090" {
		t.Fatalf("configured source base URL = %q, want %q", configured, "http://127.0.0.1:19090")
	}

	configured = (&Handler{advertiseBaseURL: "https://gateway.example.test/sandbox"}).internalSourceBaseURL()
	if want := strings.TrimRight(config.DefaultAPIBaseURL(), "/"); configured != want {
		t.Fatalf("source base URL with only advertise URL = %q, want local default %q", configured, want)
	}

	if got, want := (&Handler{}).internalSourceBaseURL(), strings.TrimRight(config.DefaultAPIBaseURL(), "/"); got != want {
		t.Fatalf("default source base URL = %q, want %q", got, want)
	}
}

func TestClassifyLarkCLIConfigureErrorReturnsActionableCodes(t *testing.T) {
	for _, test := range []struct {
		message string
		status  int
		code    string
	}{
		{"source_unavailable", http.StatusServiceUnavailable, "lark_cli_source_unavailable"},
		{"bind_failed", http.StatusBadGateway, "lark_cli_bind_failed"},
		{"extension_unsupported", http.StatusBadRequest, "unsupported_runtime"},
		{"activate_failed", http.StatusInternalServerError, "lark_cli_config_failed"},
	} {
		status, code := classifyLarkCLIConfigureError(test.message)
		if status != test.status || code != test.code {
			t.Fatalf("classify(%q) = (%d, %q), want (%d, %q)", test.message, status, code, test.status, test.code)
		}
	}
}

func TestInitAgentLarkCLIReturnsUnavailableWhenLarkCLIMissing(t *testing.T) {
	originalLookPath := larkCLILookPath
	t.Cleanup(func() {
		larkCLILookPath = originalLookPath
	})
	larkCLILookPath = func(string) (string, error) {
		return "", os.ErrNotExist
	}

	svc := mustNewSeededServiceWithOptions(t, []agent.Agent{{
		ID:              "u-dev",
		Name:            "dev",
		Role:            agent.RoleWorker,
		RuntimeKind:     agent.RuntimeKindCodex,
		ProfileComplete: true,
		CreatedAt:       time.Now().UTC(),
	}}, agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}))
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "pt-dev",
		Channel:         participant.ChannelFeishu,
		Type:            participant.TypeAgent,
		Name:            "dev",
		ChannelUserKind: participant.ChannelUserKindAppID,
		ChannelAppConfig: map[string]any{
			"app_id":     "cli_dev",
			"app_secret": "dev-secret",
		},
		AgentID:         "agent-dev",
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}), participant.WithAgentEngine(agentengine.New(svc)))
	srv := &Handler{
		svc:               svc,
		participant:       participantSvc,
		serverAccessToken: "server-secret", agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/u-dev/lark-cli:init", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	body := rec.Body.String()
	assertAPIErrorCode(t, rec, http.StatusServiceUnavailable, "lark_cli_unavailable")
	if !strings.Contains(body, "Install lark-cli") {
		t.Fatalf("missing install guidance in response: %s", body)
	}

	agentReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-dev", nil)
	agentRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(agentRec, agentReq)
	if agentRec.Code != http.StatusOK {
		t.Fatalf("agent status = %d, want %d; body=%s", agentRec.Code, http.StatusOK, agentRec.Body.String())
	}
	var agentGot agentResponse
	if err := json.NewDecoder(agentRec.Body).Decode(&agentGot); err != nil {
		t.Fatalf("decode agent response: %v", err)
	}
	if agentGot.LarkCLI == nil || agentGot.LarkCLI.Available || agentGot.LarkCLI.State != larkCLIStatusUnavailable {
		t.Fatalf("agent lark_cli status = %#v, want unavailable", agentGot.LarkCLI)
	}
}

func TestInitAgentLarkCLIRejectsSharedFeishuAppID(t *testing.T) {
	svc := mustNewSeededServiceWithOptions(t, []agent.Agent{
		{
			ID:              "u-dev",
			Name:            "dev",
			Role:            agent.RoleWorker,
			RuntimeKind:     agent.RuntimeKindCodex,
			ProfileComplete: true,
			CreatedAt:       time.Now().UTC(),
		},
		{
			ID:              "u-qa",
			Name:            "qa",
			Role:            agent.RoleWorker,
			RuntimeKind:     agent.RuntimeKindCodex,
			ProfileComplete: true,
			CreatedAt:       time.Now().UTC(),
		},
	}, agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}))
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{
		{
			ID:              "pt-dev",
			Channel:         participant.ChannelFeishu,
			Type:            participant.TypeAgent,
			Name:            "dev",
			ChannelUserKind: participant.ChannelUserKindAppID,
			ChannelAppConfig: map[string]any{
				"app_id":     "cli_shared",
				"app_secret": "dev-secret",
			},
			AgentID:         "agent-dev",
			LifecycleStatus: participant.LifecycleStatusActive,
			Mentionable:     true,
		},
		{
			ID:              "pt-qa",
			Channel:         participant.ChannelFeishu,
			Type:            participant.TypeAgent,
			Name:            "qa",
			ChannelUserKind: participant.ChannelUserKindAppID,
			ChannelAppConfig: map[string]any{
				"app_id":     "cli_shared",
				"app_secret": "qa-secret",
			},
			AgentID:         "agent-qa",
			LifecycleStatus: participant.LifecycleStatusActive,
			Mentionable:     true,
		},
	}), participant.WithAgentEngine(agentengine.New(svc)))
	srv := &Handler{svc: svc, participant: participantSvc, serverAccessToken: "server-secret", agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/u-dev/lark-cli:init", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	assertAPIErrorCode(t, rec, http.StatusConflict, feishuBotAppIDConflictCode)
}

func TestInitAgentLarkCLIConfiguresWorkerScopedSource(t *testing.T) {
	originalLookPath := larkCLILookPath
	originalCommandContext := larkCLICommandContext
	originalCurrentExe := larkCLICurrentExe
	t.Cleanup(func() {
		larkCLILookPath = originalLookPath
		larkCLICommandContext = originalCommandContext
		larkCLICurrentExe = originalCurrentExe
	})

	recordPath := filepath.Join(t.TempDir(), "bind.json")
	t.Setenv("CSGCLAW_FAKE_LARK_CLI_COMMAND", "1")
	t.Setenv("CSGCLAW_FAKE_LARK_RECORD_PATH", recordPath)
	larkCLILookPath = func(name string) (string, error) {
		if name == "lark-cli" {
			return "/opt/lark/bin/lark-cli", nil
		}
		return "", os.ErrNotExist
	}
	larkCLICommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		helperArgs := append([]string{"-test.run=^TestLarkCLIFakeCommand$", "--"}, args...)
		return exec.CommandContext(ctx, os.Args[0], helperArgs...)
	}
	larkCLICurrentExe = func() (string, error) {
		return "/opt/csgclaw/bin/csgclaw", nil
	}

	svc := mustNewSeededServiceWithOptions(t, []agent.Agent{{
		ID:              "u-dev",
		Name:            "dev",
		Role:            agent.RoleWorker,
		RuntimeKind:     agent.RuntimeKindCodex,
		RuntimeID:       "rt-dev",
		BoxID:           "codex-session-dev",
		Status:          string(agentruntime.StateRunning),
		ProfileComplete: true,
		CreatedAt:       time.Now().UTC(),
	}}, agent.WithRuntime(fakeCompatRuntime{
		kind: agent.RuntimeKindCodex,
		stop: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
			return agentruntime.StateStopped, nil
		},
		start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
			return agentruntime.StateRunning, nil
		},
	}))
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "pt-dev",
		Channel:         participant.ChannelFeishu,
		Type:            participant.TypeAgent,
		Name:            "dev",
		ChannelUserKind: participant.ChannelUserKindAppID,
		ChannelAppConfig: map[string]any{
			"app_id":     "cli_dev",
			"app_secret": "dev-secret",
		},
		AgentID:         "agent-dev",
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}), participant.WithAgentEngine(agentengine.New(svc)))
	srv := &Handler{
		svc:               svc,
		participant:       participantSvc,
		serverAccessToken: "server-secret",
		internalBaseURL:   "http://csgclaw.test", agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/u-dev/lark-cli:init", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got apitypes.AgentLarkCLIInitResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.AgentID != "agent-dev" || got.AppID != "cli_dev" || got.RestartStatus != "runtime_loaded" {
		t.Fatalf("init response = %#v", got)
	}
	if got.Generation == 0 || !got.RuntimeLoaded {
		t.Fatalf("init response did not confirm the current generation: %#v", got)
	}
	var bind struct {
		Args []string          `json:"args"`
		Env  map[string]string `json:"env"`
	}
	recordRaw, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read fake bind record: %v", err)
	}
	if err := json.Unmarshal(recordRaw, &bind); err != nil {
		t.Fatalf("decode fake bind record: %v", err)
	}
	if got, want := strings.Join(bind.Args, " "), "config bind --source lark-channel --identity bot-only --force --lang zh"; got != want {
		t.Fatalf("bind args = %q, want %q", got, want)
	}
	if strings.TrimSpace(bind.Env["LARKSUITE_CLI_CONFIG_DIR"]) == "" ||
		bind.Env["LARK_CHANNEL"] != "1" ||
		strings.TrimSpace(bind.Env["LARK_CHANNEL_HOME"]) == "" ||
		bind.Env["LARK_CHANNEL_PROFILE"] != "agent-dev" ||
		strings.TrimSpace(bind.Env["LARK_CHANNEL_CONFIG"]) == "" {
		t.Fatalf("bind env = %#v, want worker-scoped lark-cli env", bind.Env)
	}

	agentReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-dev", nil)
	agentRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(agentRec, agentReq)
	if agentRec.Code != http.StatusOK {
		t.Fatalf("agent status = %d, want %d; body=%s", agentRec.Code, http.StatusOK, agentRec.Body.String())
	}
	var agentGot agentResponse
	if err := json.NewDecoder(agentRec.Body).Decode(&agentGot); err != nil {
		t.Fatalf("decode agent response: %v", err)
	}
	if agentGot.LarkCLI == nil ||
		!agentGot.LarkCLI.Bound ||
		agentGot.LarkCLI.State != larkCLIStatusBound ||
		agentGot.LarkCLI.AppID != "cli_dev" ||
		agentGot.LarkCLI.Generation == 0 ||
		!agentGot.LarkCLI.RuntimeLoaded {
		t.Fatalf("agent lark_cli status = %#v", agentGot.LarkCLI)
	}
}

func TestInitAgentLarkCLIPreservesConfiguredResultWhenRuntimeRestartFails(t *testing.T) {
	originalLookPath := larkCLILookPath
	originalCommandContext := larkCLICommandContext
	originalCurrentExe := larkCLICurrentExe
	t.Cleanup(func() {
		larkCLILookPath = originalLookPath
		larkCLICommandContext = originalCommandContext
		larkCLICurrentExe = originalCurrentExe
	})
	t.Setenv("CSGCLAW_FAKE_LARK_CLI_COMMAND", "1")
	t.Setenv("CSGCLAW_FAKE_LARK_RECORD_PATH", filepath.Join(t.TempDir(), "bind.json"))
	larkCLILookPath = func(string) (string, error) { return "/opt/lark/bin/lark-cli", nil }
	larkCLICommandContext = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestLarkCLIFakeCommand$", "--"}, args...)...)
	}
	larkCLICurrentExe = func() (string, error) { return "/opt/csgclaw/bin/csgclaw", nil }

	target := agent.Agent{
		ID: "agent-dev", Name: "dev", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex,
		RuntimeID: "rt-agent-dev", BoxID: "codex-dev", Status: string(agentruntime.StateRunning),
		ProfileComplete: true, CreatedAt: time.Now().UTC(),
	}
	svc := mustNewSeededServiceWithOptions(t, []agent.Agent{target}, agent.WithRuntime(fakeCompatRuntime{
		kind: agent.RuntimeKindCodex,
		stop: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
			return agentruntime.StateStopped, nil
		},
		start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
			return "", errors.New("restart failed")
		},
	}))
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID: "pt-dev", Channel: participant.ChannelFeishu, Type: participant.TypeAgent, Name: "dev",
		ChannelUserKind:  participant.ChannelUserKindAppID,
		ChannelAppConfig: map[string]any{"app_id": "cli_dev", "app_secret": "dev-secret"},
		AgentID:          "agent-dev", LifecycleStatus: participant.LifecycleStatusActive, Mentionable: true,
	}}), participant.WithAgentEngine(agentengine.New(svc)))
	srv := &Handler{svc: svc, participant: participantSvc, serverAccessToken: "server-secret", agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc}
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-dev/lark-cli:init", strings.NewReader(`{}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got apitypes.AgentLarkCLIInitResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "configured" || got.RestartStatus != "restart_failed" || got.RestartError == "" || got.RuntimeLoaded {
		t.Fatalf("response = %+v", got)
	}
}

func TestAgentLarkCLIStatusWithoutExtensionIsUnbound(t *testing.T) {
	originalLookPath := larkCLILookPath
	originalCommandContext := larkCLICommandContext
	t.Cleanup(func() {
		larkCLILookPath = originalLookPath
		larkCLICommandContext = originalCommandContext
	})
	t.Setenv("CSGCLAW_FAKE_LARK_CLI_COMMAND", "1")
	t.Setenv("CSGCLAW_FAKE_LARK_RECORD_PATH", filepath.Join(t.TempDir(), "probe.json"))
	t.Setenv("CSGCLAW_FAKE_LARK_EXIT_CODE", "1")
	larkCLILookPath = func(string) (string, error) {
		return "/opt/lark/bin/lark-cli", nil
	}
	larkCLICommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		helperArgs := append([]string{"-test.run=^TestLarkCLIFakeCommand$", "--"}, args...)
		return exec.CommandContext(ctx, os.Args[0], helperArgs...)
	}

	target := agent.Agent{
		ID:              "agent-dev",
		Name:            "dev",
		Role:            agent.RoleWorker,
		RuntimeKind:     agent.RuntimeKindCodex,
		ProfileComplete: true,
		CreatedAt:       time.Now().UTC(),
	}
	svc := mustNewSeededServiceWithOptions(t, []agent.Agent{target}, agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}))
	srv := &Handler{svc: svc, agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc}
	target, _ = svc.Agent("agent-dev")
	status := srv.agentLarkCLIStatus(target)
	if status == nil || status.Available || status.State != larkCLIStatusUnbound || status.Error != "" {
		t.Fatalf("status = %+v, want unbound resource state", status)
	}
}

func TestAgentLarkCLIStatusDoesNotImplicitlyApplyConnectedBinding(t *testing.T) {
	originalLookPath := larkCLILookPath
	originalCommandContext := larkCLICommandContext
	originalCurrentExe := larkCLICurrentExe
	t.Cleanup(func() {
		larkCLILookPath = originalLookPath
		larkCLICommandContext = originalCommandContext
		larkCLICurrentExe = originalCurrentExe
	})
	t.Setenv("CSGCLAW_FAKE_LARK_CLI_COMMAND", "1")
	t.Setenv("CSGCLAW_FAKE_LARK_RECORD_PATH", filepath.Join(t.TempDir(), "bind.json"))
	larkCLILookPath = func(string) (string, error) { return "/opt/lark/bin/lark-cli", nil }
	larkCLICommandContext = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestLarkCLIFakeCommand$", "--"}, args...)...)
	}
	larkCLICurrentExe = func() (string, error) { return "/opt/csgclaw/bin/csgclaw", nil }

	target := agent.Agent{ID: "agent-dev", Name: "dev", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex, ProfileComplete: true, CreatedAt: time.Now().UTC()}
	svc := mustNewSeededServiceWithOptions(t, []agent.Agent{target}, agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}))
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID: "pt-dev", Channel: participant.ChannelFeishu, Type: participant.TypeAgent, AgentID: "agent-dev",
		ChannelUserKind:  participant.ChannelUserKindAppID,
		ChannelAppConfig: map[string]any{"app_id": "cli_dev", "app_secret": "dev-secret"},
	}}), participant.WithAgentEngine(agentengine.New(svc)))
	srv := &Handler{svc: svc, participant: participantSvc, serverAccessToken: "server-secret", agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc}
	target, _ = svc.Agent("agent-dev")
	status := srv.agentLarkCLIStatus(target)
	if status == nil || status.Bound || status.State != larkCLIStatusUnbound {
		t.Fatalf("status = %+v", status)
	}
	if _, err := srv.agentEngine.RuntimeExtensions("agent-dev").Get(context.Background(), "feishu-lark-cli"); agentengine.ErrorCodeOf(err) != agentengine.ErrorRuntimeExtensionNotFound {
		t.Fatalf("status read created a RuntimeExtension: %v", err)
	}
}

func TestDisconnectWaitsForInFlightLarkCLIBindAndClearsResult(t *testing.T) {
	originalLookPath := larkCLILookPath
	originalCommandContext := larkCLICommandContext
	originalCurrentExe := larkCLICurrentExe
	t.Cleanup(func() {
		larkCLILookPath = originalLookPath
		larkCLICommandContext = originalCommandContext
		larkCLICurrentExe = originalCurrentExe
	})

	tempDir := t.TempDir()
	recordPath := filepath.Join(tempDir, "bind.json")
	readyPath := filepath.Join(tempDir, "ready")
	releasePath := filepath.Join(tempDir, "release")
	t.Setenv("CSGCLAW_FAKE_LARK_CLI_COMMAND", "1")
	t.Setenv("CSGCLAW_FAKE_LARK_RECORD_PATH", recordPath)
	t.Setenv("CSGCLAW_FAKE_LARK_READY_PATH", readyPath)
	t.Setenv("CSGCLAW_FAKE_LARK_RELEASE_PATH", releasePath)
	larkCLILookPath = func(name string) (string, error) {
		if name == "lark-cli" {
			return "/opt/lark/bin/lark-cli", nil
		}
		return "", os.ErrNotExist
	}
	larkCLICommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		helperArgs := append([]string{"-test.run=^TestLarkCLIFakeCommand$", "--"}, args...)
		return exec.CommandContext(ctx, os.Args[0], helperArgs...)
	}
	larkCLICurrentExe = func() (string, error) {
		return "/opt/csgclaw/bin/csgclaw", nil
	}

	target := agent.Agent{
		ID:              "agent-dev",
		Name:            "dev",
		Role:            agent.RoleWorker,
		RuntimeKind:     agent.RuntimeKindCodex,
		ProfileComplete: true,
		CreatedAt:       time.Now().UTC(),
	}
	bridge := &fakeCodexBridgeController{}
	svc := mustNewSeededServiceWithOptions(t, []agent.Agent{target},
		agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}),
	)
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "pt-dev",
		Channel:         participant.ChannelFeishu,
		Type:            participant.TypeAgent,
		Name:            "dev",
		ChannelUserKind: participant.ChannelUserKindAppID,
		ChannelAppConfig: map[string]any{
			"app_id":     "cli_dev",
			"app_secret": "dev-secret",
		},
		AgentID:         "agent-dev",
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}), participant.WithAgentEngine(agentengine.New(svc)))
	srv := &Handler{
		svc:               svc,
		participant:       participantSvc,
		channelBindings:   bridge,
		serverAccessToken: "server-secret", agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc,
	}
	router := srv.Routes()

	initDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-dev/lark-cli:init", strings.NewReader(`{}`))
		router.ServeHTTP(rec, req)
		initDone <- rec
	}()
	waitForTestPath(t, readyPath)

	disconnectDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/feishu/participants/pt-dev", nil)
		router.ServeHTTP(rec, req)
		disconnectDone <- rec
	}()
	select {
	case rec := <-disconnectDone:
		t.Fatalf("disconnect completed before bind released: status=%d body=%s", rec.Code, rec.Body.String())
	case <-time.After(50 * time.Millisecond):
	}
	if _, ok := participantSvc.Get(participant.ChannelFeishu, "pt-dev"); !ok {
		t.Fatal("Participant changed before the in-flight Apply released its Agent lease")
	}
	if err := os.WriteFile(releasePath, []byte("release\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	initRec := waitForTestResponse(t, initDone)
	disconnectRec := waitForTestResponse(t, disconnectDone)
	if initRec.Code != http.StatusOK {
		t.Fatalf("init status = %d, want %d; body=%s", initRec.Code, http.StatusOK, initRec.Body.String())
	}
	if disconnectRec.Code != http.StatusNoContent {
		t.Fatalf("disconnect status = %d, want %d; body=%s", disconnectRec.Code, http.StatusNoContent, disconnectRec.Body.String())
	}
}

func waitForTestPath(t *testing.T, path string) {
	t.Helper()
	waitForTestCondition(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	})
}

func waitForTestCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for test condition")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForTestResponse(t *testing.T, responses <-chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case rec := <-responses:
		return rec
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for HTTP response")
		return nil
	}
}

func TestLarkCLIFakeCommand(t *testing.T) {
	if os.Getenv("CSGCLAW_FAKE_LARK_CLI_COMMAND") != "1" {
		return
	}
	recordPath := strings.TrimSpace(os.Getenv("CSGCLAW_FAKE_LARK_RECORD_PATH"))
	if recordPath == "" {
		t.Fatal("CSGCLAW_FAKE_LARK_RECORD_PATH is required")
	}
	var args []string
	for idx, arg := range os.Args {
		if arg == "--" {
			args = append(args, os.Args[idx+1:]...)
			break
		}
	}
	if len(args) == 1 && args[0] == "-v" {
		if os.Getenv("CSGCLAW_FAKE_LARK_EXIT_CODE") == "1" {
			os.Exit(1)
		}
		os.Exit(0)
	}
	payload := struct {
		Args []string          `json:"args"`
		Env  map[string]string `json:"env"`
	}{
		Args: args,
		Env: map[string]string{
			"LARKSUITE_CLI_CONFIG_DIR": os.Getenv("LARKSUITE_CLI_CONFIG_DIR"),
			"LARK_CHANNEL":             os.Getenv("LARK_CHANNEL"),
			"LARK_CHANNEL_HOME":        os.Getenv("LARK_CHANNEL_HOME"),
			"LARK_CHANNEL_PROFILE":     os.Getenv("LARK_CHANNEL_PROFILE"),
			"LARK_CHANNEL_CONFIG":      os.Getenv("LARK_CHANNEL_CONFIG"),
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode fake lark-cli record: %v", err)
	}
	if err := os.WriteFile(recordPath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write fake lark-cli record: %v", err)
	}
	isBind := len(args) >= 2 && args[0] == "config" && args[1] == "bind"
	if isBind {
		if readyPath := strings.TrimSpace(os.Getenv("CSGCLAW_FAKE_LARK_READY_PATH")); readyPath != "" {
			if err := os.WriteFile(readyPath, []byte("ready\n"), 0o600); err != nil {
				t.Fatalf("write fake lark-cli ready marker: %v", err)
			}
		}
		if releasePath := strings.TrimSpace(os.Getenv("CSGCLAW_FAKE_LARK_RELEASE_PATH")); releasePath != "" {
			deadline := time.Now().Add(10 * time.Second)
			for {
				if _, err := os.Stat(releasePath); err == nil {
					break
				} else if !os.IsNotExist(err) {
					t.Fatalf("stat fake lark-cli release marker: %v", err)
				}
				if time.Now().After(deadline) {
					t.Fatal("timed out waiting for fake lark-cli release marker")
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
	}
	if os.Getenv("CSGCLAW_FAKE_LARK_EXIT_CODE") == "1" ||
		(isBind && os.Getenv("CSGCLAW_FAKE_LARK_BIND_EXIT_CODE") == "1") {
		os.Exit(1)
	}
	if isBind {
		configPath := filepath.Join(os.Getenv("LARKSUITE_CLI_CONFIG_DIR"), "lark-channel", "config.json")
		if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
			t.Fatalf("create fake lark-cli config dir: %v", err)
		}
		sourceRaw, err := os.ReadFile(os.Getenv("LARK_CHANNEL_CONFIG"))
		var source struct {
			Accounts struct {
				App struct {
					ID string `json:"id"`
				} `json:"app"`
			} `json:"accounts"`
		}
		if err != nil || json.Unmarshal(sourceRaw, &source) != nil || strings.TrimSpace(source.Accounts.App.ID) == "" {
			t.Fatal("read app ID from fake lark-cli source config")
		}
		config := fmt.Sprintf("{\"apps\":[{\"appId\":%q}]}\n", source.Accounts.App.ID)
		if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
			t.Fatalf("write fake lark-cli config: %v", err)
		}
	}
}

func assertAPIErrorCode(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, wantStatus, rec.Body.String())
	}
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error.Code != wantCode {
		t.Fatalf("error.code = %q, want %q; body=%s", payload.Error.Code, wantCode, rec.Body.String())
	}
}
