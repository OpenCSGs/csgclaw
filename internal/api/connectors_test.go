package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/connectors"
	agentruntime "csgclaw/internal/runtime"
)

func TestConnectorGitHubOAuthAPIFlowAndCredentialAuth(t *testing.T) {
	var sawTokenExchange bool
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := r.Form.Get("client_secret"); got != "client-secret" {
				t.Fatalf("client_secret = %q", got)
			}
			sawTokenExchange = true
			writeJSON(w, http.StatusOK, map[string]any{
				"access_token": "gh-token",
				"token_type":   "bearer",
				"scope":        "repo,read:user,user:email",
			})
		case "/user":
			writeJSON(w, http.StatusOK, map[string]any{
				"login":      "octocat",
				"id":         1,
				"avatar_url": "https://avatars.example/octocat.png",
				"html_url":   "https://github.com/octocat",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(github.Close)

	svc := connectors.NewService(connectors.NewStore(filepath.Join(t.TempDir(), "state.json")))
	svc.HTTPClient = github.Client()
	svc.Endpoints = connectors.Endpoints{
		AuthorizeURL: github.URL + "/login/oauth/authorize",
		TokenURL:     github.URL + "/login/oauth/access_token",
		APIBaseURL:   github.URL,
	}
	svc.Now = func() time.Time { return time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC) }
	svc.GenerateOAuthState = func() (connectors.OAuthState, error) {
		return connectors.OAuthState{State: "state-value", CodeVerifier: "verifier"}, nil
	}
	handler := &Handler{serverAccessToken: "server-token"}
	handler.SetConnectorService(svc)
	routes := handler.Routes()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/connectors/github/config", strings.NewReader(`{"client_id":"client-id","client_secret":"client-secret"}`))
	req.Host = "127.0.0.1:18080"
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("config status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "client-secret") {
		t.Fatalf("config response leaks client secret: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/connectors/github/oauth/start", strings.NewReader(`{"return_url":"http://127.0.0.1:18080/#/workspace"}`))
	req.Host = "127.0.0.1:18080"
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var start connectors.OAuthStartResponse
	if err := json.NewDecoder(rec.Body).Decode(&start); err != nil {
		t.Fatalf("decode start: %v", err)
	}
	authorizeURL, err := url.Parse(start.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse start url: %v", err)
	}
	if got := authorizeURL.Query().Get("state"); got != "state-value" {
		t.Fatalf("state = %q", got)
	}
	if got := authorizeURL.Query().Get("client_id"); got != "client-id" {
		t.Fatalf("client_id = %q", got)
	}
	if got := authorizeURL.Query().Get("redirect_uri"); got != "http://127.0.0.1:18080/api/v1/connectors/github/oauth/callback" {
		t.Fatalf("redirect_uri = %q", got)
	}
	if got := authorizeURL.Query().Get("response_type"); got != "code" {
		t.Fatalf("response_type = %q", got)
	}
	if got := authorizeURL.Query().Get("scope"); got != "repo read:user user:email" {
		t.Fatalf("scope = %q", got)
	}
	if got := authorizeURL.Query().Get("code_challenge"); got == "" {
		t.Fatal("code_challenge is empty")
	}
	if got := authorizeURL.Query().Get("code_challenge_method"); got != "S256" {
		t.Fatalf("code_challenge_method = %q", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/connectors/github/oauth/callback?code=oauth-code&state=state-value", nil)
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("callback status = %d, want 302: %s", rec.Code, rec.Body.String())
	}
	if !sawTokenExchange {
		t.Fatal("token exchange was not called")
	}
	installURL, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse app install redirect: %v", err)
	}
	if got := installURL.Scheme + "://" + installURL.Host + installURL.Path; got != "https://github.com/apps/csgclaw/installations/select_target" {
		t.Fatalf("callback redirect = %q", got)
	}
	if got := installURL.Query().Get("state"); got != "state-value" {
		t.Fatalf("app install state = %q", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/connectors/github", nil)
	req.Host = "127.0.0.1:18080"
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "client-secret") || strings.Contains(rec.Body.String(), "gh-token") {
		t.Fatalf("status response leaks secret material: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/connectors/github/credential", nil)
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("credential without auth status = %d, want 401", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/connectors/github/credential", nil)
	req.Header.Set("Authorization", "Bearer server-token")
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("credential status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if !strings.Contains(rec.Body.String(), "gh-token") {
		t.Fatalf("credential body = %s, want access token", rec.Body.String())
	}
}

func TestAgentConnectorCredentialAPIReturnsDynamicManagerLease(t *testing.T) {
	var sawTokenExchange bool
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			sawTokenExchange = true
			writeJSON(w, http.StatusOK, map[string]any{
				"access_token": "gh-token",
				"token_type":   "bearer",
				"scope":        "repo,read:user,user:email",
			})
		case "/user":
			if got := r.Header.Get("Authorization"); got != "Bearer gh-token" {
				t.Fatalf("user Authorization = %q", got)
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"login": "octocat",
				"id":    1,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(github.Close)

	connectorSvc := connectors.NewService(connectors.NewStore(filepath.Join(t.TempDir(), "state.json")))
	connectorSvc.HTTPClient = github.Client()
	connectorSvc.Endpoints = connectors.Endpoints{
		AuthorizeURL: github.URL + "/login/oauth/authorize",
		TokenURL:     github.URL + "/login/oauth/access_token",
		APIBaseURL:   github.URL,
	}
	connectorSvc.GenerateOAuthState = func() (connectors.OAuthState, error) {
		return connectors.OAuthState{State: "state-value", CodeVerifier: "verifier"}, nil
	}

	var specs []agentruntime.Spec
	var deleteCalls int
	agentSvc, err := agent.NewService(
		config.ModelConfig{
			Provider: config.ProviderLLMAPI,
			BaseURL:  "http://127.0.0.1:4000",
			APIKey:   "sk-test",
			ModelID:  "model-1",
		},
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "server-token"},
		"",
		filepath.Join(t.TempDir(), "agents.json"),
		agent.WithRuntime(fakeCompatRuntime{
			kind: agent.RuntimeKindCodex,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				specs = append(specs, spec)
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-manager-session"}, nil
			},
			del: func(_ context.Context, h agentruntime.Handle) error {
				deleteCalls++
				return nil
			},
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}
	if _, err := agentSvc.EnsureManager(context.Background(), false); err != nil {
		t.Fatalf("initial EnsureManager() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("initial runtime specs = %d, want 1", len(specs))
	}
	if got := specs[0].Profile.Env["GITHUB_TOKEN"]; got != "" {
		t.Fatalf("initial manager GITHUB_TOKEN = %q, want empty before OAuth", got)
	}
	worker, err := agentSvc.CreateWorker(context.Background(), agent.CreateAgentSpec{
		ID:          "agent-worker",
		Name:        "worker",
		RuntimeKind: agent.RuntimeKindCodex,
		AgentProfile: agent.AgentProfile{
			Name:            "worker",
			Provider:        agent.ProviderAPI,
			BaseURL:         "https://api.example/v1",
			APIKey:          "api-key",
			ModelID:         "gpt-4.1",
			ProfileComplete: true,
		},
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if worker.Role != agent.RoleWorker {
		t.Fatalf("worker role = %q, want worker", worker.Role)
	}

	handler := &Handler{svc: agentSvc, serverAccessToken: "server-token"}
	handler.SetConnectorService(connectorSvc)
	routes := handler.Routes()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/connectors/github/config", strings.NewReader(`{"client_id":"client-id","client_secret":"client-secret"}`))
	req.Host = "127.0.0.1:18080"
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("config status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/connectors/github/oauth/start", strings.NewReader(`{"return_url":"http://127.0.0.1:18080/#/workspace"}`))
	req.Host = "127.0.0.1:18080"
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/connectors/github/oauth/callback?code=oauth-code&state=state-value", nil)
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("callback status = %d, want 302: %s", rec.Code, rec.Body.String())
	}
	if !sawTokenExchange {
		t.Fatal("token exchange was not called")
	}
	if len(specs) != 2 {
		t.Fatalf("runtime specs = %d, want manager plus worker only", len(specs))
	}
	if deleteCalls != 0 {
		t.Fatalf("manager runtime delete calls = %d, want no refresh after OAuth", deleteCalls)
	}
	for i, spec := range specs {
		if got := spec.Profile.Env["GITHUB_TOKEN"]; got != "" {
			t.Fatalf("runtime spec %d GITHUB_TOKEN = %q, want empty", i, got)
		}
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-manager/connectors/github/credential", nil)
	req.Header.Set("Authorization", "Bearer server-token")
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("manager credential status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var lease struct {
		Provider    string              `json:"provider"`
		AccessToken string              `json:"access_token"`
		TokenType   string              `json:"token_type"`
		Account     *connectors.Account `json:"account"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&lease); err != nil {
		t.Fatalf("decode manager credential lease: %v", err)
	}
	if lease.Provider != connectors.ProviderGitHub || lease.AccessToken != "gh-token" || lease.TokenType != "bearer" {
		t.Fatalf("manager credential lease = %+v, want github bearer token", lease)
	}
	if lease.Account == nil || lease.Account.Login != "octocat" {
		t.Fatalf("manager credential account = %+v, want octocat", lease.Account)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-worker/connectors/github/credential", nil)
	req.Header.Set("Authorization", "Bearer server-token")
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("worker credential status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
}

func TestConnectorGitHubOAuthStartUsesManagedAppWithoutUserConfig(t *testing.T) {
	svc := connectors.NewService(connectors.NewStore(filepath.Join(t.TempDir(), "state.json")))
	svc.Endpoints = connectors.Endpoints{AuthorizeURL: "https://github.example/login/oauth/authorize"}
	svc.GitHubOAuthApp = connectors.Config{
		ClientID:     "managed-client-id",
		ClientSecret: "managed-client-secret",
	}
	svc.GenerateOAuthState = func() (connectors.OAuthState, error) {
		return connectors.OAuthState{State: "state-value", CodeVerifier: "verifier"}, nil
	}
	handler := &Handler{}
	handler.SetConnectorService(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/github/oauth/start", strings.NewReader(`{"return_url":"http://127.0.0.1:18080/#/workspace"}`))
	req.Host = "127.0.0.1:18080"
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var start connectors.OAuthStartResponse
	if err := json.NewDecoder(rec.Body).Decode(&start); err != nil {
		t.Fatalf("decode start: %v", err)
	}
	authorizeURL, err := url.Parse(start.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse start url: %v", err)
	}
	if got := authorizeURL.Query().Get("client_id"); got != "managed-client-id" {
		t.Fatalf("client_id = %q", got)
	}
	if got := authorizeURL.Query().Get("redirect_uri"); got != "http://127.0.0.1:18080/api/v1/connectors/github/oauth/callback" {
		t.Fatalf("redirect_uri = %q", got)
	}
}

func TestConnectorGitHubAppInstallStart(t *testing.T) {
	svc := connectors.NewService(connectors.NewStore(filepath.Join(t.TempDir(), "state.json")))
	svc.GitHubAppSlug = "csgclaw"
	svc.GenerateOAuthState = func() (connectors.OAuthState, error) {
		return connectors.OAuthState{State: "install-state", CodeVerifier: "verifier"}, nil
	}
	handler := &Handler{}
	handler.SetConnectorService(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/connectors/github/app/install/start", nil)
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("app install start status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var start connectors.AppInstallStartResponse
	if err := json.NewDecoder(rec.Body).Decode(&start); err != nil {
		t.Fatalf("decode app install start: %v", err)
	}
	installURL, err := url.Parse(start.InstallURL)
	if err != nil {
		t.Fatalf("parse install url: %v", err)
	}
	if got := installURL.Scheme + "://" + installURL.Host + installURL.Path; got != "https://github.com/apps/csgclaw/installations/select_target" {
		t.Fatalf("install url = %q", got)
	}
	if got := installURL.Query().Get("state"); got != "install-state" {
		t.Fatalf("state = %q", got)
	}
}

func TestConnectorGitHubOAuthStartRedirectsForNewTab(t *testing.T) {
	svc := connectors.NewService(connectors.NewStore(filepath.Join(t.TempDir(), "state.json")))
	svc.Endpoints = connectors.Endpoints{AuthorizeURL: "https://github.example/login/oauth/authorize"}
	svc.GitHubOAuthApp = connectors.Config{
		ClientID:     "managed-client-id",
		ClientSecret: "managed-client-secret",
	}
	svc.GenerateOAuthState = func() (connectors.OAuthState, error) {
		return connectors.OAuthState{State: "state-value", CodeVerifier: "verifier"}, nil
	}
	handler := &Handler{}
	handler.SetConnectorService(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/connectors/github/oauth/start?return_url=http%3A%2F%2F127.0.0.1%3A18080%2F%23%2Fworkspace", nil)
	req.Host = "127.0.0.1:18080"
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("start redirect status = %d, want 302: %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	authorizeURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse redirect location: %v", err)
	}
	if got := authorizeURL.Query().Get("client_id"); got != "managed-client-id" {
		t.Fatalf("client_id = %q", got)
	}
	if got := authorizeURL.Query().Get("state"); got != "state-value" {
		t.Fatalf("state = %q", got)
	}
}

func TestConnectorGitHubOAuthCallbackReportsValidationErrors(t *testing.T) {
	svc := connectors.NewService(connectors.NewStore(filepath.Join(t.TempDir(), "state.json")))
	handler := &Handler{}
	handler.SetConnectorService(svc)

	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/connectors/github/oauth/callback?state=missing-code", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want 400", rec.Code)
	}
}
