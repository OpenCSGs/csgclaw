package api

import (
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	"csgclaw/internal/participant"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestLegacyLarkCLIActionsReturnConnectorManagement(t *testing.T) {
	controller := mustNewSeededService(t, []agent.Agent{{ID: "agent-dev", Name: "dev", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex}})
	h := &Handler{svc: controller, agentEngine: agentengine.New(controller), agentRuntime: controller}
	for _, action := range []string{"init", "cleanup"} {
		response := httptest.NewRecorder()
		h.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-dev/lark-cli:"+action, strings.NewReader("{}")))
		if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), "connector_management_required") {
			t.Fatalf("%s: %d %s", action, response.Code, response.Body)
		}
	}
}
