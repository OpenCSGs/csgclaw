package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apps"
	"csgclaw/internal/config"
	"csgclaw/internal/connectors"
	agentruntime "csgclaw/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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

func TestGitLabPATAppCallsConfiguredMCPByConnectorID(t *testing.T) {
	h, alice, _, _, _ := newAppPlatformAuthFixture(t)
	upstreamMCP := mcp.NewServer(&mcp.Implementation{Name: "gitlab-fixture", Version: "1"}, nil)
	upstreamMCP.AddTool(&mcp.Tool{Name: "list_projects", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
	})
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		if r.Header.Get("PRIVATE-TOKEN") != "custom-gitlab-token" || r.Header.Get("X-GitLab-Base-URL") != "https://custom.gitlab.example.com" || r.Header.Get("X-MCP-Deployment") != "fixture-secret" {
			t.Errorf("unexpected upstream headers: %v", r.Header)
		}
		return upstreamMCP
	}, nil)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/user" {
			writeJSON(w, http.StatusOK, map[string]any{"id": 1, "username": "fixture"})
			return
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(upstream.Close)

	store := connectors.NewStore(filepath.Join(t.TempDir(), "connectors.json"))
	if err := store.SaveGitLab(connectors.State{Config: connectors.Config{BaseURL: upstream.URL, AccessToken: "connector-token"}, Account: &connectors.Account{Login: "fixture"}, ConnectedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	h.SetConnectorService(connectors.NewService(store))
	body := `{"app_id":"gitlab","config":{"transport":"http","url":"` + upstream.URL + `","gitlab_base_url":"` + upstream.URL + `","connector_id":"gitlab","auth_mode":"connector"},"credentials":{"headers":{"PRIVATE-TOKEN":"custom-gitlab-token","X-GitLab-Base-URL":"https://custom.gitlab.example.com","X-MCP-Deployment":"fixture-secret"}}}`
	rec := appAuthRequest(t, h, http.MethodPost, "/api/v1/agents/"+alice.ID+"/apps:probe", body, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("probe status=%d body=%s", rec.Code, rec.Body.String())
	}
	var result apps.ProbeResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || !result.Connected || len(result.Tools) != 1 || result.Tools[0].Name != "list_projects" {
		t.Fatalf("probe result=%+v err=%v", result, err)
	}
}

func TestGitLabResourceSaveRollsBackConnectorWhenPersistenceFails(t *testing.T) {
	h, _, _, _, _ := newAppPlatformAuthFixture(t)
	gitlab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"id": 1, "username": "fixture"})
	}))
	t.Cleanup(gitlab.Close)
	store := connectors.NewStore(filepath.Join(t.TempDir(), "connectors.json"))
	svc := connectors.NewService(store)
	svc.HTTPClient = gitlab.Client()
	h.SetConnectorService(svc)
	if _, err := h.apps.Create(context.Background(), "", apps.CreateRequest{AppID: "gitlab", Name: "duplicate"}); err != nil {
		t.Fatal(err)
	}
	body := `{"app_id":"gitlab","name":"duplicate","config":{"transport":"http","url":"https://mcp.example.com/mcp","gitlab_base_url":"` + gitlab.URL + `","connector_id":"gitlab","auth_mode":"connector"},"credentials":{"token":"temporary-pat"}}`
	rec := appAuthRequest(t, h, http.MethodPost, "/api/v1/app-resources", body, "", nil)
	if rec.Code < 400 {
		t.Fatalf("invalid resource save status=%d", rec.Code)
	}
	if _, ok, err := store.LoadGitLab(); err != nil || ok {
		t.Fatalf("failed resource save retained Connector: ok=%v err=%v", ok, err)
	}
}

func TestConcurrentGitLabResourceRollbackDoesNotOverwriteSuccessfulUpdate(t *testing.T) {
	h, _, _, _, _ := newAppPlatformAuthFixture(t)
	firstValidation, releaseFirst, secondValidation := make(chan struct{}), make(chan struct{}), make(chan struct{})
	gitlab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("PRIVATE-TOKEN") {
		case "token-a":
			close(firstValidation)
			<-releaseFirst
		case "token-b":
			close(secondValidation)
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": 1, "username": "fixture"})
	}))
	t.Cleanup(gitlab.Close)
	store := connectors.NewStore(filepath.Join(t.TempDir(), "connectors.json"))
	svc := connectors.NewService(store)
	svc.HTTPClient = gitlab.Client()
	h.SetConnectorService(svc)
	if _, err := h.apps.Create(context.Background(), "", apps.CreateRequest{AppID: "gitlab", Name: "duplicate"}); err != nil {
		t.Fatal(err)
	}
	body := func(name, token string) string {
		return `{"app_id":"gitlab","name":"` + name + `","config":{"transport":"http","url":"https://mcp.example.com/mcp","gitlab_base_url":"` + gitlab.URL + `","connector_id":"gitlab","auth_mode":"connector"},"credentials":{"token":"` + token + `"}}`
	}
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstDone <- appAuthRequest(t, h, http.MethodPost, "/api/v1/app-resources", body("duplicate", "token-a"), "", nil)
	}()
	<-firstValidation
	secondDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		secondDone <- appAuthRequest(t, h, http.MethodPost, "/api/v1/app-resources", body("work", "token-b"), "", nil)
	}()
	select {
	case <-secondValidation:
		t.Fatal("second global Connector update entered before the first transaction completed")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	if rec := <-firstDone; rec.Code < 400 {
		t.Fatalf("first resource save status=%d", rec.Code)
	}
	if rec := <-secondDone; rec.Code != http.StatusCreated {
		t.Fatalf("second resource save status=%d body=%s", rec.Code, rec.Body.String())
	}
	state, ok, err := store.LoadGitLab()
	if err != nil || !ok || state.Config.AccessToken != "token-b" {
		t.Fatalf("global Connector after concurrent rollback=%+v ok=%v err=%v", state, ok, err)
	}
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
	resource, err := h.apps.Create(context.Background(), "", apps.CreateRequest{AppID: "gitlab", Name: "Work GitLab"})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"resource_id":"` + resource.InstallationID + `"}`
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
	resource, err := h.apps.Create(context.Background(), "", apps.CreateRequest{AppID: "gitlab", Name: "Personal GitLab"})
	if err != nil {
		t.Fatal(err)
	}
	rec := appAuthRequest(t, h, http.MethodPost, path, `{"resource_id":"`+resource.InstallationID+`"}`, "", nil)
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

func TestAppRequestSameOrigin(t *testing.T) {
	tests := []struct {
		name             string
		requestURL       string
		origin           string
		secFetchSite     string
		advertiseBaseURL string
		want             bool
	}{
		{
			name:       "direct HTTP origin",
			requestURL: "http://localhost:18080/api/v1/apps",
			origin:     "http://localhost:18080",
			want:       true,
		},
		{
			name:       "direct HTTPS origin",
			requestURL: "https://csgclaw.example.test/api/v1/apps",
			origin:     "https://csgclaw.example.test",
			want:       true,
		},
		{
			name:             "advertised origin through HTTP proxy",
			requestURL:       "http://csghub-runner:8082/api/v1/apps",
			origin:           "https://aigateway.opencsg-stg.com",
			secFetchSite:     "same-origin",
			advertiseBaseURL: "https://aigateway.opencsg-stg.com/v1/sandboxes/user-123?jwt=test",
			want:             true,
		},
		{
			name:             "advertised HTTPS origin with explicit default port",
			requestURL:       "http://csghub-runner:8082/api/v1/apps",
			origin:           "https://aigateway.example.test",
			secFetchSite:     "same-origin",
			advertiseBaseURL: "https://aigateway.example.test:443/v1/sandboxes/user-123",
			want:             true,
		},
		{
			name:             "advertised HTTP origin with explicit default port",
			requestURL:       "http://csghub-runner:8082/api/v1/apps",
			origin:           "http://aigateway.example.test",
			secFetchSite:     "same-origin",
			advertiseBaseURL: "http://aigateway.example.test:80/v1/sandboxes/user-123",
			want:             true,
		},
		{
			name:       "direct HTTPS origin with explicit default port",
			requestURL: "https://csgclaw.example.test:443/api/v1/apps",
			origin:     "https://csgclaw.example.test",
			want:       true,
		},
		{
			name:             "advertised non-default port",
			requestURL:       "http://csghub-runner:8082/api/v1/apps",
			origin:           "https://aigateway.example.test:8443",
			advertiseBaseURL: "https://aigateway.example.test:8443/v1/sandboxes/user-123",
			want:             true,
		},
		{
			name:             "different advertised port",
			requestURL:       "http://csghub-runner:8082/api/v1/apps",
			origin:           "https://aigateway.example.test",
			advertiseBaseURL: "https://aigateway.example.test:8443/v1/sandboxes/user-123",
			want:             false,
		},
		{
			name:             "direct origin remains available with advertised URL",
			requestURL:       "http://localhost:18080/api/v1/apps",
			origin:           "http://localhost:18080",
			advertiseBaseURL: "https://aigateway.example.test/v1/sandboxes/user-123",
			want:             true,
		},
		{
			name:             "different advertised origin",
			requestURL:       "http://csghub-runner:8082/api/v1/apps",
			origin:           "https://other.example.test",
			advertiseBaseURL: "https://aigateway.example.test/v1/sandboxes/user-123",
			want:             false,
		},
		{
			name:             "cross-site request",
			requestURL:       "http://csghub-runner:8082/api/v1/apps",
			origin:           "https://aigateway.example.test",
			secFetchSite:     "cross-site",
			advertiseBaseURL: "https://aigateway.example.test/v1/sandboxes/user-123",
			want:             false,
		},
		{
			name:             "invalid advertised URL",
			requestURL:       "http://csghub-runner:8082/api/v1/apps",
			origin:           "https://aigateway.example.test",
			advertiseBaseURL: "://invalid",
			want:             false,
		},
		{
			name:       "missing origin",
			requestURL: "http://csghub-runner:8082/api/v1/apps",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.requestURL, nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.secFetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tt.secFetchSite)
			}
			if got := appRequestSameOrigin(req, tt.advertiseBaseURL); got != tt.want {
				t.Fatalf("appRequestSameOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAppNoAuthAllowsAdvertisedOriginThroughProxy(t *testing.T) {
	h, _, _, _, _ := newAppPlatformAuthFixture(t)
	h.SetAdvertiseBaseURL("https://aigateway.opencsg-stg.com:443/v1/sandboxes/user-123?jwt=test")

	req := httptest.NewRequest(http.MethodPost, "http://csghub-runner:8082/api/v1/channels/csgclaw/participants", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://aigateway.opencsg-stg.com")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.authorizeAppPlatformRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("advertised origin status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "http://csghub-runner:8082/api/v1/missing", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://aigateway.opencsg-stg.com")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("advertised origin router status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAppNoAuthAllowsCrossSiteConnectorOAuthCallbacks(t *testing.T) {
	h, _, _, _, _ := newAppPlatformAuthFixture(t)
	req := httptest.NewRequest(http.MethodGet, "http://localhost:18080"+githubConnectorCallbackPath+"?state=missing-code", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cross-site OAuth callback status=%d, want callback validation 400: %s", rec.Code, rec.Body.String())
	}
}

func TestAppNoAuthAllowsCrossSiteOpenCSGCallbacks(t *testing.T) {
	h, _, _, _, _ := newAppPlatformAuthFixture(t)
	server := httptest.NewServer(h.Routes())
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	for _, origin := range []string{"https://opencsg.com", "https://opencsg-stg.com"} {
		t.Run(origin, func(t *testing.T) {
			send := func() *http.Response {
				t.Helper()
				req, err := http.NewRequest(http.MethodGet, server.URL+authCallbackPath+"?auth_state=invalid-repro", nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Origin", origin)
				req.Header.Set("Referer", origin+"/")
				req.Header.Set("Sec-Fetch-Site", "cross-site")
				req.Header.Set("Sec-Fetch-Mode", "navigate")
				req.Header.Set("Sec-Fetch-Dest", "document")
				resp, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = resp.Body.Close() })
				return resp
			}
			// Invalid credentials must reach the real callback validator.
			if resp := send(); resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("invalid callback status = %d, want 400", resp.StatusCode)
			}
			// Successful authentication must preserve the settings redirect.
			restore := stubAuthCallback(func(*http.Request, string) (string, error) {
				return server.URL + "/#/settings", nil
			})
			defer restore()
			resp := send()
			if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != server.URL+"/#/settings?auth_result=success" {
				t.Fatalf("successful callback status = %d, location = %q", resp.StatusCode, resp.Header.Get("Location"))
			}
		})
	}
}

func TestAppCrossSiteCallbackExemptionIsScoped(t *testing.T) {
	h, _, _, _, _ := newAppPlatformAuthFixture(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, authCallbackPath},
		{http.MethodHead, authCallbackPath},
		{http.MethodGet, authCallbackPath + "/extra"},
		{http.MethodGet, "/api/v1/auth/status"},
		{http.MethodPost, "/api/v1/auth/login"},
		{http.MethodPost, "/api/v1/auth/logout"},
		{http.MethodPost, githubConnectorCallbackPath},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			rec := httptest.NewRecorder()
			h.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "origin_denied") {
				t.Fatalf("status = %d, body = %s; want origin_denied", rec.Code, rec.Body.String())
			}
		})
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
	resource, err := h.apps.Create(ctx, "", apps.CreateRequest{AppID: "gitlab", Name: "Work GitLab"})
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []agent.Agent{alice, bob} {
		if _, err := h.apps.Bind(ctx, owner.ID, apps.BindRequest{ResourceID: resource.InstallationID}); err != nil {
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
