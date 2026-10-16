package api

import (
	"context"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateFeishuRegistrationStoresSafeState(t *testing.T) {
	accounts := newFakeFeishuAccountsServer(t, nil)
	defer accounts.Close()
	withFeishuRegistrationAccountsBaseURL(t, accounts.URL)

	agentSvc, _ := mustNewSeededServiceWithPath(t, []agent.Agent{completeWorkerAgent("u-dev", "dev")})
	participantSvc := participant.NewService(participant.NewMemoryStore(nil), participant.WithAgentEngine(agentengine.New(agentSvc)))
	srv := &Handler{
		svc:                        agentSvc,
		participant:                participantSvc,
		channelBindings:            noOpChannelBindingReconciler{},
		feishuRegistrationStateDir: filepath.Join(t.TempDir(), "registrations"), agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/registrations", strings.NewReader(`{"agent_id":"u-dev"}`))
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "device-1") {
		t.Fatalf("registration response leaked device_code: %s", rec.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created["participant_id"] != "pt-dev" || created["agent_id"] != "agent-dev" {
		t.Fatalf("registration response = %#v, want canonical dev participant for agent-dev", created)
	}
	connectURL := strings.TrimSpace(created["connect_url"].(string))
	if !strings.Contains(connectURL, "from=csgclaw") || !strings.Contains(connectURL, "tp=csgclaw") {
		t.Fatalf("connect_url = %q, want CSGClaw launcher params", connectURL)
	}

	registrationID := strings.TrimSpace(created["registration_id"].(string))
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/registrations/"+url.PathEscape(registrationID), nil)
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "device-1") {
		t.Fatalf("status response leaked device_code: %s", rec.Body.String())
	}
}

func TestFinalizeFeishuRegistrationBindsWorkerParticipant(t *testing.T) {
	accounts := newFakeFeishuAccountsServer(t, map[string]any{
		"client_id":     "cli_dev",
		"client_secret": "dev-secret",
		"user_info": map[string]any{
			"open_id": "ou_admin",
		},
	})
	defer accounts.Close()
	withFeishuRegistrationAccountsBaseURL(t, accounts.URL)

	agentSvc, _ := mustNewSeededServiceWithPathAndOptions(t, []agent.Agent{completeWorkerAgent("u-dev", "dev")},
		agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindPicoClawSandbox}),
	)
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "admin",
		Channel:         participant.ChannelFeishu,
		Type:            participant.TypeHuman,
		Name:            "admin",
		ChannelUserRef:  "ou_old_admin",
		ChannelUserKind: participant.ChannelUserKindOpenID,
	}}), participant.WithAgentEngine(agentengine.New(agentSvc)))
	srv := &Handler{
		svc:                        agentSvc,
		participant:                participantSvc,
		channelBindings:            noOpChannelBindingReconciler{},
		feishuRegistrationStateDir: filepath.Join(t.TempDir(), "registrations"), agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}
	registrationID := startFeishuRegistrationForTest(t, srv, "u-dev")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/registrations/"+url.PathEscape(registrationID)+":finalize", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "dev-secret") {
		t.Fatalf("finalize response leaked app secret: %s", rec.Body.String())
	}
	var result map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["participant_id"] != "pt-dev" || result["config_saved"] != true || result["restart_status"] != "restart_skipped" || result["activation_status"] != "runtime_recreated" {
		t.Fatalf("finalize result = %#v, want saved dev config and activated worker runtime", result)
	}
	stored, ok := participantSvc.Get(participant.ChannelFeishu, "pt-dev")
	if !ok {
		t.Fatal("feishu:dev participant was not stored")
	}
	if stored.AgentID != "agent-dev" || stored.ChannelUserKind != participant.ChannelUserKindAppID {
		t.Fatalf("stored participant = %+v, want app-backed agent-dev Feishu participant", stored)
	}
	if got := stored.ChannelAppConfig[participant.ChannelAppConfigAppSecretKey]; got != "dev-secret" {
		t.Fatalf("stored app_secret = %#v, want real secret", got)
	}
	admin, ok := participantSvc.Get(participant.ChannelFeishu, "admin")
	if !ok {
		t.Fatal("existing feishu:admin participant was removed")
	}
	if admin.Type != participant.TypeHuman || admin.ChannelUserKind != participant.ChannelUserKindOpenID || admin.ChannelUserRef != "ou_old_admin" {
		t.Fatalf("admin participant = %+v, want worker finalize to leave existing Feishu admin unchanged", admin)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/registrations/"+url.PathEscape(registrationID), nil)
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("registration state status after successful finalize = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestFinalizeFeishuRegistrationOnlyConfiguresChannel(t *testing.T) {
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

	accounts := newFakeFeishuAccountsServer(t, map[string]any{
		"client_id":     "cli_dev",
		"client_secret": "dev-secret",
	})
	defer accounts.Close()
	withFeishuRegistrationAccountsBaseURL(t, accounts.URL)

	worker := completeWorkerAgent("u-dev", "dev")
	worker.RuntimeKind = agent.RuntimeKindCodex
	bridge := &fakeCodexBridgeController{}
	var runtimeStarts, runtimeStops int
	agentSvc, _ := mustNewSeededServiceWithPathAndOptions(t, []agent.Agent{worker},
		agent.WithRuntime(fakeCompatRuntime{
			kind: agent.RuntimeKindCodex,
			start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
				runtimeStarts++
				return agentruntime.StateRunning, nil
			},
			stop: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
				runtimeStops++
				return agentruntime.StateStopped, nil
			},
		}),
	)
	participantSvc := participant.NewService(participant.NewMemoryStore(nil), participant.WithAgentEngine(agentengine.New(agentSvc)))
	srv := &Handler{
		svc:                        agentSvc,
		participant:                participantSvc,
		channelBindings:            bridge,
		serverAccessToken:          "server-secret",
		internalBaseURL:            "http://csgclaw.test",
		feishuRegistrationStateDir: filepath.Join(t.TempDir(), "registrations"), agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}
	registrationID := startFeishuRegistrationForTest(t, srv, "u-dev")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/registrations/"+url.PathEscape(registrationID)+":finalize", nil)
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var finalizeResult feishuRegistrationFinalizeResponse
	if err := json.NewDecoder(rec.Body).Decode(&finalizeResult); err != nil {
		t.Fatalf("decode finalize response: %v", err)
	}
	if len(bridge.refreshCalls) != 1 || bridge.refreshCalls[0].agent.ID != "agent-dev" {
		t.Fatalf("channel refresh=%v", bridge.refreshCalls)
	}
	if runtimeStarts != 0 || runtimeStops != 0 {
		t.Fatalf("runtime changed: starts=%d stops=%d", runtimeStarts, runtimeStops)
	}
	if _, err := os.Stat(recordPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("CLI was invoked: %v", err)
	}
}

func TestFinalizeFeishuRegistrationRejectsBotAppIDUsedByAnotherWorker(t *testing.T) {
	accounts := newFakeFeishuAccountsServer(t, map[string]any{
		"client_id":     "cli_shared",
		"client_secret": "qa-secret",
	})
	defer accounts.Close()
	withFeishuRegistrationAccountsBaseURL(t, accounts.URL)

	agentSvc, _ := mustNewSeededServiceWithPathAndOptions(t, []agent.Agent{
		completeWorkerAgent("u-dev", "dev"),
		completeWorkerAgent("u-qa", "qa"),
	}, agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindPicoClawSandbox}))
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
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
	}}), participant.WithAgentEngine(agentengine.New(agentSvc)))
	srv := &Handler{
		svc:                        agentSvc,
		participant:                participantSvc,
		channelBindings:            noOpChannelBindingReconciler{},
		feishuRegistrationStateDir: filepath.Join(t.TempDir(), "registrations"), agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}
	registrationID := startFeishuRegistrationForTest(t, srv, "u-qa")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/registrations/"+url.PathEscape(registrationID)+":finalize", nil)
	srv.Routes().ServeHTTP(rec, req)

	assertAPIErrorCode(t, rec, http.StatusConflict, feishuBotAppIDConflictCode)
	if _, ok := participantSvc.Get(participant.ChannelFeishu, "pt-qa"); ok {
		t.Fatal("conflicting Feishu participant was written before AppID validation")
	}
	stored, ok := participantSvc.Get(participant.ChannelFeishu, "pt-dev")
	if !ok || stored.ChannelAppConfig[participant.ChannelAppConfigAppSecretKey] != "dev-secret" {
		t.Fatalf("existing participant changed after conflict: %+v", stored)
	}
}

func TestFinalizeFeishuRegistrationDoesNotUpdateCanonicalAdminForWorker(t *testing.T) {
	accounts := newFakeFeishuAccountsServer(t, map[string]any{
		"client_id":     "cli_dev",
		"client_secret": "dev-secret",
		"user_info": map[string]any{
			"open_id": "ou_admin",
		},
	})
	defer accounts.Close()
	withFeishuRegistrationAccountsBaseURL(t, accounts.URL)

	agentSvc, _ := mustNewSeededServiceWithPathAndOptions(t, []agent.Agent{completeWorkerAgent("u-dev", "dev")},
		agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindPicoClawSandbox}),
	)
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              participant.BootstrapAdminParticipantID,
		Channel:         participant.ChannelFeishu,
		Type:            participant.TypeHuman,
		Name:            "admin",
		ChannelUserRef:  "ou_old_admin",
		ChannelUserKind: participant.ChannelUserKindOpenID,
	}}), participant.WithAgentEngine(agentengine.New(agentSvc)))
	srv := &Handler{
		svc:                        agentSvc,
		participant:                participantSvc,
		channelBindings:            noOpChannelBindingReconciler{},
		feishuRegistrationStateDir: filepath.Join(t.TempDir(), "registrations"), agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}
	registrationID := startFeishuRegistrationForTest(t, srv, "u-dev")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/registrations/"+url.PathEscape(registrationID)+":finalize", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	admin, ok := participantSvc.Get(participant.ChannelFeishu, participant.BootstrapAdminParticipantID)
	if !ok {
		t.Fatal("existing canonical feishu admin participant was removed")
	}
	if admin.ChannelUserRef != "ou_old_admin" {
		t.Fatalf("admin channel_user_ref = %q, want worker finalize to leave existing admin unchanged", admin.ChannelUserRef)
	}
	if _, ok := participantSvc.Get(participant.ChannelFeishu, "pt-dev"); !ok {
		t.Fatal("feishu:dev participant was not stored")
	}
}

func TestFinalizeFeishuRegistrationResolvesAdminNameFromFeishuOpenAPI(t *testing.T) {
	accounts := newFakeFeishuAccountsServerWithOpenAPI(t, map[string]any{
		"client_id":     "cli_dev",
		"client_secret": "dev-secret",
		"user_info": map[string]any{
			"open_id": "ou_admin",
		},
	}, map[string]any{
		"code":                0,
		"msg":                 "ok",
		"tenant_access_token": "tenant-token",
		"expire":              7200,
	}, map[string]any{
		"code": 0,
		"msg":  "ok",
		"data": map[string]any{
			"user": map[string]any{
				"name":    "龙韵",
				"open_id": "ou_admin",
			},
		},
	})
	defer accounts.Close()
	withFeishuRegistrationAccountsBaseURL(t, accounts.URL)
	withFeishuOpenAPIBaseURL(t, accounts.URL)

	manager := completeWorkerAgent(agent.ManagerUserID, "manager")
	manager.Role = agent.RoleManager
	manager.RuntimeKind = agent.RuntimeKindCodex
	bridge := &fakeCodexBridgeController{}
	agentSvc, _ := mustNewSeededServiceWithPathAndOptions(t, []agent.Agent{manager})
	participantSvc := participant.NewService(participant.NewMemoryStore(nil), participant.WithAgentEngine(agentengine.New(agentSvc)))
	srv := &Handler{
		svc:                        agentSvc,
		participant:                participantSvc,
		channelBindings:            bridge,
		feishuRegistrationStateDir: filepath.Join(t.TempDir(), "registrations"), agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}
	registrationID := startFeishuRegistrationForTest(t, srv, agent.ManagerUserID)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/registrations/"+url.PathEscape(registrationID)+":finalize", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	admin, ok := participantSvc.Get(participant.ChannelFeishu, "admin")
	if !ok {
		t.Fatal("feishu:admin participant was not stored")
	}
	if admin.Name != "龙韵" {
		t.Fatalf("admin participant name = %q, want Feishu user name", admin.Name)
	}
}

func TestFinalizeFeishuRegistrationPendingDoesNotBind(t *testing.T) {
	accounts := newFakeFeishuAccountsServer(t, map[string]any{"error": "authorization_pending"})
	defer accounts.Close()
	withFeishuRegistrationAccountsBaseURL(t, accounts.URL)

	agentSvc, _ := mustNewSeededServiceWithPath(t, []agent.Agent{completeWorkerAgent("u-dev", "dev")})
	participantSvc := participant.NewService(participant.NewMemoryStore(nil), participant.WithAgentEngine(agentengine.New(agentSvc)))
	srv := &Handler{
		svc:                        agentSvc,
		participant:                participantSvc,
		feishuRegistrationStateDir: filepath.Join(t.TempDir(), "registrations"), agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}
	registrationID := startFeishuRegistrationForTest(t, srv, "u-dev")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/registrations/"+url.PathEscape(registrationID)+":finalize", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if _, ok := participantSvc.Get(participant.ChannelFeishu, "pt-dev"); ok {
		t.Fatal("feishu:dev participant was stored while registration is still pending")
	}
}

func TestCreateFeishuRegistrationConflictsWithActiveRegistration(t *testing.T) {
	accounts := newFakeFeishuAccountsServer(t, nil)
	defer accounts.Close()
	withFeishuRegistrationAccountsBaseURL(t, accounts.URL)

	agentSvc, _ := mustNewSeededServiceWithPath(t, []agent.Agent{completeWorkerAgent("u-dev", "dev")})
	participantSvc := participant.NewService(participant.NewMemoryStore(nil), participant.WithAgentEngine(agentengine.New(agentSvc)))
	srv := &Handler{
		svc:                        agentSvc,
		participant:                participantSvc,
		feishuRegistrationStateDir: filepath.Join(t.TempDir(), "registrations"), agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}
	firstID := startFeishuRegistrationForTest(t, srv, "u-dev")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/registrations", strings.NewReader(`{"agent_id":"u-dev"}`))
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "device-1") {
		t.Fatalf("conflict response leaked device_code: %s", rec.Body.String())
	}
	var conflict map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&conflict); err != nil {
		t.Fatalf("decode conflict response: %v", err)
	}
	if conflict["registration_id"] != firstID || conflict["status"] != "pending" {
		t.Fatalf("conflict response = %#v, want existing pending registration %q", conflict, firstID)
	}
}

func TestFinalizeFeishuRegistrationDeniedAndExpiredDoNotBind(t *testing.T) {
	for _, tc := range []struct {
		name       string
		poll       map[string]any
		expire     bool
		wantStatus int
	}{
		{name: "denied", poll: map[string]any{"error": "access_denied"}, wantStatus: http.StatusBadRequest},
		{name: "expired", poll: map[string]any{"client_id": "cli_dev", "client_secret": "dev-secret"}, expire: true, wantStatus: http.StatusGone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accounts := newFakeFeishuAccountsServer(t, tc.poll)
			defer accounts.Close()
			withFeishuRegistrationAccountsBaseURL(t, accounts.URL)
			withFeishuRegistrationNow(t, time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC))

			agentSvc, _ := mustNewSeededServiceWithPath(t, []agent.Agent{completeWorkerAgent("u-dev", "dev")})
			participantSvc := participant.NewService(participant.NewMemoryStore(nil), participant.WithAgentEngine(agentengine.New(agentSvc)))
			srv := &Handler{
				svc:                        agentSvc,
				participant:                participantSvc,
				feishuRegistrationStateDir: filepath.Join(t.TempDir(), "registrations"), agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
			}
			registrationID := startFeishuRegistrationForTest(t, srv, "u-dev")
			if tc.expire {
				withFeishuRegistrationNow(t, time.Date(2026, 6, 16, 8, 11, 0, 0, time.UTC))
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/registrations/"+url.PathEscape(registrationID)+":finalize", nil)
			srv.Routes().ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if _, ok := participantSvc.Get(participant.ChannelFeishu, "pt-dev"); ok {
				t.Fatal("feishu:dev participant was stored after failed finalize")
			}
		})
	}
}

func TestFinalizeFeishuRegistrationBindsManagerAdminHuman(t *testing.T) {
	accounts := newFakeFeishuAccountsServer(t, map[string]any{
		"client_id":     "cli_manager",
		"client_secret": "manager-secret",
		"user_info": map[string]any{
			"open_id": "ou_admin",
		},
	})
	defer accounts.Close()
	withFeishuRegistrationAccountsBaseURL(t, accounts.URL)

	manager := completeWorkerAgent(agent.ManagerUserID, "manager")
	manager.Role = agent.RoleManager
	manager.RuntimeKind = agent.RuntimeKindCodex
	bridge := &fakeCodexBridgeController{}
	agentSvc, _ := mustNewSeededServiceWithPathAndOptions(t, []agent.Agent{manager})
	participantSvc := participant.NewService(participant.NewMemoryStore(nil), participant.WithAgentEngine(agentengine.New(agentSvc)))
	srv := &Handler{
		svc:                        agentSvc,
		participant:                participantSvc,
		channelBindings:            bridge,
		feishuRegistrationStateDir: filepath.Join(t.TempDir(), "registrations"), agentEngine: agentengine.New(agentSvc), workspace: agentSvc.Workspace(), agentModels: agentSvc.Models(), agentRuntime: agentSvc,
	}
	registrationID := startFeishuRegistrationForTest(t, srv, agent.ManagerUserID)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/registrations/"+url.PathEscape(registrationID)+":finalize", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var result map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["restart_status"] != "restart_skipped" || result["activation_status"] != "channel_refreshed" || result["participant_id"] != "pt-manager" {
		t.Fatalf("finalize result = %#v, want manager Feishu channel refresh", result)
	}
	if len(bridge.ensureCalls) != 0 {
		t.Fatalf("EnsureAgent() calls = %+v, want none", bridge.ensureCalls)
	}
	if len(bridge.refreshCalls) != 1 || bridge.refreshCalls[0].agent.ID != agent.ManagerUserID || bridge.refreshCalls[0].channel != participant.ChannelFeishu {
		t.Fatalf("RefreshAgentChannel() calls = %+v, want manager Feishu once", bridge.refreshCalls)
	}
	admin, ok := participantSvc.Get(participant.ChannelFeishu, "admin")
	if !ok {
		t.Fatal("feishu:admin human participant was not stored")
	}
	if admin.Type != participant.TypeHuman || admin.ChannelUserRef != "ou_admin" || admin.ChannelUserKind != participant.ChannelUserKindOpenID {
		t.Fatalf("admin participant = %+v, want Feishu open_id human", admin)
	}
}

func completeWorkerAgent(id, name string) agent.Agent {
	return agent.Agent{
		ID:          id,
		Name:        name,
		Role:        agent.RoleWorker,
		RuntimeKind: agent.RuntimeKindPicoClawSandbox,
		RuntimeID:   "rt-" + id,
		Image:       "agent-image:test",
		Status:      string(agentruntime.StateRunning),
		AgentProfile: agent.AgentProfile{
			Provider:        agent.ProviderAPI,
			BaseURL:         "http://127.0.0.1:4000",
			APIKey:          "sk-test",
			ModelID:         "model-1",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}
}

func startFeishuRegistrationForTest(t *testing.T, srv *Handler, agentID string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/registrations", strings.NewReader(`{"agent_id":"`+agentID+`"}`))
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	return strings.TrimSpace(created["registration_id"].(string))
}

func newFakeFeishuAccountsServer(t *testing.T, pollResponse map[string]any) *httptest.Server {
	t.Helper()
	return newFakeFeishuAccountsServerWithOpenAPI(t, pollResponse, nil, nil)
}

func newFakeFeishuAccountsServerWithOpenAPI(t *testing.T, pollResponse, tokenResponse, userResponse map[string]any) *httptest.Server {
	t.Helper()
	if pollResponse == nil {
		pollResponse = map[string]any{"error": "authorization_pending"}
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/oauth/v1/app/registration":
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			switch r.Form.Get("action") {
			case "init":
				writeJSON(w, http.StatusOK, map[string]any{"supported_auth_methods": []string{"client_secret"}})
			case "begin":
				writeJSON(w, http.StatusOK, map[string]any{
					"device_code":               "device-1",
					"verification_uri_complete": "https://feishu.example/verify?existing=1",
					"user_code":                 "ABCD-EFGH",
					"interval":                  3,
					"expire_in":                 600,
				})
			case "poll":
				writeJSON(w, http.StatusOK, pollResponse)
			default:
				http.Error(w, "unexpected action "+r.Form.Get("action"), http.StatusBadRequest)
			}
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			if tokenResponse == nil {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, http.StatusOK, tokenResponse)
		case r.URL.Path == "/open-apis/contact/v3/users/ou_admin":
			if userResponse == nil {
				http.NotFound(w, r)
				return
			}
			if got := r.URL.Query().Get("user_id_type"); got != "open_id" {
				http.Error(w, fmt.Sprintf("user_id_type = %q, want open_id", got), http.StatusBadRequest)
				return
			}
			if got := strings.TrimSpace(r.Header.Get("Authorization")); got != "Bearer tenant-token" {
				http.Error(w, fmt.Sprintf("Authorization = %q, want bearer tenant token", got), http.StatusUnauthorized)
				return
			}
			writeJSON(w, http.StatusOK, userResponse)
		default:
			http.NotFound(w, r)
		}
	}))
}

func withFeishuRegistrationAccountsBaseURL(t *testing.T, baseURL string) {
	t.Helper()
	old := feishuRegistrationAccountsBaseURL
	feishuRegistrationAccountsBaseURL = baseURL
	t.Cleanup(func() {
		feishuRegistrationAccountsBaseURL = old
	})
}

func withFeishuOpenAPIBaseURL(t *testing.T, baseURL string) {
	t.Helper()
	old := feishuOpenAPIBaseURL
	feishuOpenAPIBaseURL = baseURL
	t.Cleanup(func() {
		feishuOpenAPIBaseURL = old
	})
}

func withFeishuRegistrationNow(t *testing.T, now time.Time) {
	t.Helper()
	old := feishuRegistrationNow
	feishuRegistrationNow = func() time.Time { return now }
	t.Cleanup(func() {
		feishuRegistrationNow = old
	})
}
