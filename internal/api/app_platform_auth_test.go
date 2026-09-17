package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apps"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
)

func newAppPlatformAuthFixture(t *testing.T) (*Handler, agent.Agent, agent.Agent, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc, err := agent.NewController(config.ModelConfig{Provider: "codex", ModelID: "gpt-5.5"}, config.ServerConfig{ListenAddr: "127.0.0.1:18080", AccessToken: "test-admin-secret"}, "manager:test", filepath.Join(home, "agents.json"), agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	create := func(name string) (agent.Agent, string) {
		item, err := svc.CreateRecord(context.Background(), agent.CreateRequest{Spec: agent.CreateAgentSpec{Name: name, Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex, AgentProfile: agent.AgentProfile{ProfileComplete: true, Provider: agent.ProviderCodex, ModelID: "gpt-5.5"}}})
		if err != nil {
			t.Fatal(err)
		}
		profile, err := svc.PicoClawRuntimeHost().ResolveRuntimeProfile(agentruntime.Handle{RuntimeID: item.RuntimeID})
		if err != nil {
			t.Fatal(err)
		}
		if !svc.AuthorizesAgentAccessToken(item.ID, profile.APIKey) {
			t.Fatal("invalid fixture Agent credential")
		}
		return item, profile.APIKey
	}
	alice, aliceToken := create("alice")
	bob, bobToken := create("bob")
	h := &Handler{svc: svc, agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc, serverAccessToken: "test-admin-secret", desktopSessionToken: "test-desktop-secret", serverNoAuth: true}
	if err := h.EnableApps(filepath.Join(home, "apps.json")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.CloseApps() })
	return h, alice, bob, aliceToken, bobToken
}

func appAuthRequest(t *testing.T, h *Handler, method, path, body, token string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	return rec
}

func TestAppPlatformAuthHonorsNoAuthAndRejectsCrossAgentAccess(t *testing.T) {
	h, alice, bob, aliceToken, _ := newAppPlatformAuthFixture(t)
	for _, path := range []string{"/api/v1/apps", "/api/v1/agents", "/api/v1/agents/" + alice.ID + "/apps"} {
		if rec := appAuthRequest(t, h, http.MethodGet, path, "", "", nil); rec.Code != http.StatusOK {
			t.Errorf("personal mode %s status=%d", path, rec.Code)
		}
	}
	for _, path := range []string{"/api/v1/agents/" + bob.ID + "/apps", "/api/v1/agents/" + bob.ID + "/llm/models", "/api/v1/config", "/api/v1/agents"} {
		if rec := appAuthRequest(t, h, http.MethodGet, path, "", aliceToken, nil); rec.Code != http.StatusForbidden {
			t.Errorf("scoped %s status=%d", path, rec.Code)
		}
	}
	if rec := appAuthRequest(t, h, http.MethodGet, "/api/v1/agents/"+alice.ID+"/apps", "", aliceToken, nil); rec.Code != http.StatusOK {
		t.Fatalf("own apps status=%d body=%s", rec.Code, rec.Body)
	}
	body := `{"app_id":"gitlab","name":"Work GitLab"}`
	if rec := appAuthRequest(t, h, http.MethodPost, "/api/v1/agents/"+alice.ID+"/apps", body, aliceToken, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("Agent configured credentials through REST: status=%d", rec.Code)
	}
	if rec := appAuthRequest(t, h, http.MethodPost, "/api/v1/agents/"+alice.ID+"/apps", body, h.serverAccessToken, nil); rec.Code != http.StatusCreated {
		t.Fatalf("admin app create status=%d body=%s", rec.Code, rec.Body)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+alice.ID+"/llm/models", nil)
	request.Header.Set("Authorization", "Bearer "+aliceToken)
	request.Header.Set("X-CSGClaw-Caller-Agent", "agent-manager")
	response := httptest.NewRecorder()
	h.authorizeAppPlatformRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-CSGClaw-Caller-Agent") != h.agentPlatformParticipant(alice.ID) || appRequestAgentID(r) != alice.ID {
			t.Error("runtime caller was not replaced with authenticated identity")
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("own LLM middleware status=%d", response.Code)
	}
}

func TestAppManagementWithoutLogin(t *testing.T) {
	h, alice, _, _, _ := newAppPlatformAuthFixture(t)
	h.serverNoAuth = false
	path := "/api/v1/agents/" + alice.ID + "/apps"
	rec := appAuthRequest(t, h, http.MethodPost, path, `{"app_id":"gitlab","name":"Personal GitLab"}`, "", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create without login: %d %s", rec.Code, rec.Body)
	}
	var item apps.Installation
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	rec = appAuthRequest(t, h, http.MethodDelete, path+"/"+item.InstallationID, "", "", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("remove without login: %d %s", rec.Code, rec.Body)
	}
}

func TestAppsDoNotAddLoginToPersonalUI(t *testing.T) {
	h, alice, _, _, _ := newAppPlatformAuthFixture(t)
	// no_auth controls existing protected API operations, not Web UI login.
	for _, noAuth := range []bool{false, true} {
		h.serverNoAuth = noAuth
		for _, path := range []string{"/api/v1/apps", "/api/v1/agents", "/api/v1/agents/" + alice.ID + "/apps"} {
			rec := appAuthRequest(t, h, http.MethodGet, path, "", "", nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("personal UI no_auth=%v %s: %d", noAuth, path, rec.Code)
			}
		}
		if h.validateServerAccessToken("") != noAuth {
			t.Fatal("existing protected API authentication changed")
		}
	}
}

func TestAppNoAuthRejectsInvalidAgentAndCrossOriginRequests(t *testing.T) {
	h, _, _, _, _ := newAppPlatformAuthFixture(t)
	rec := appAuthRequest(t, h, http.MethodGet, "/api/v1/apps", "", "agent.invalid", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid Agent accepted: %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "http://localhost:18080/api/v1/apps", nil)
	req.Header.Set("Origin", "https://other.example")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin accepted: %d", rec.Code)
	}
}

func TestAgentMCPRejectsForeignTokensAndSessions(t *testing.T) {
	h, alice, bob, aliceToken, bobToken := newAppPlatformAuthFixture(t)
	invoke := func(agentID, token, sessionID, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+agentID+"/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if sessionID != "" {
			req.Header.Set("Mcp-Session-Id", sessionID)
			req.Header.Set("Mcp-Protocol-Version", "2025-11-25")
		}
		rec := httptest.NewRecorder()
		h.Routes().ServeHTTP(rec, req)
		return rec
	}
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	for _, token := range []string{"", bobToken, h.serverAccessToken} {
		rec := invoke(alice.ID, token, "", initialize)
		if rec.Code != 401 && rec.Code != 403 {
			t.Fatalf("foreign MCP token status=%d", rec.Code)
		}
	}
	first := invoke(alice.ID, aliceToken, "", initialize)
	if first.Code != 200 {
		t.Fatalf("MCP initialize status=%d body=%s", first.Code, first.Body)
	}
	sessionID := first.Header().Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("missing MCP session")
	}
	notification := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	if rec := invoke(alice.ID, aliceToken, sessionID, notification); rec.Code != 202 {
		t.Fatalf("MCP initialized status=%d", rec.Code)
	}
	list := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	listed := invoke(alice.ID, aliceToken, sessionID, list)
	if rec := listed; rec.Code != 200 {
		t.Fatalf("own MCP session status=%d", rec.Code)
	}
	var payload struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				InputSchema struct {
					Properties map[string]any `json:"properties"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	wantFields := map[string][]string{
		"csgclaw_publish_file": {"path", "name", "mimeType"},
		"csgclaw_upload_file":  {"path", "server", "installation_id", "uploadUri", "contentType"},
	}
	for _, tool := range payload.Result.Tools {
		fields, ok := wantFields[tool.Name]
		if !ok {
			continue
		}
		if len(tool.InputSchema.Properties) != len(fields) {
			t.Errorf("%s has fields inconsistent with native runtime", tool.Name)
		}
		for _, field := range fields {
			if tool.InputSchema.Properties[field] == nil {
				t.Errorf("%s missing native field %s", tool.Name, field)
			}
		}
		delete(wantFields, tool.Name)
	}
	if len(wantFields) != 0 {
		t.Fatal("platform file tools missing from Agent MCP")
	}
	if rec := invoke(bob.ID, bobToken, sessionID, list); rec.Code != 404 {
		t.Fatalf("foreign Agent session status=%d", rec.Code)
	}
}

func TestAppInstallationsSurviveStopAndAreRemovedWithAgent(t *testing.T) {
	h, alice, bob, aliceToken, _ := newAppPlatformAuthFixture(t)
	ctx := context.Background()
	for _, owner := range []agent.Agent{alice, bob} {
		if _, err := h.apps.Create(ctx, owner.ID, apps.CreateRequest{AppID: "gitlab", Name: "Work GitLab"}); err != nil {
			t.Fatal(err)
		}
	}
	controller := h.svc.(*agent.Controller)
	if _, err := controller.Stop(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	if items, err := h.apps.List(ctx, alice.ID); err != nil || len(items) != 1 {
		t.Fatal("stopping the runtime removed its App installation")
	}
	if err := controller.DeleteRecord(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	if items, err := h.apps.List(ctx, alice.ID); err != nil || len(items) != 0 {
		t.Fatal("Agent deletion did not remove its App installation")
	}
	if items, err := h.apps.List(ctx, bob.ID); err != nil || len(items) != 1 {
		t.Fatal("Agent deletion affected another Agent's App")
	}
	if controller.AuthorizesAgentAccessToken(alice.ID, aliceToken) {
		t.Fatal("deleted Agent still authorized")
	}
}
