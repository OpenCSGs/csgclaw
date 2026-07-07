package connectors

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
)

func TestServiceConfigStartCallbackCredentialAndDisconnect(t *testing.T) {
	var sawTokenExchange bool
	var sawUserFetch bool
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			if r.Method != http.MethodPost {
				t.Fatalf("token method = %s, want POST", r.Method)
			}
			if got := r.Header.Get("Accept"); got != "application/json" {
				t.Fatalf("token Accept = %q, want application/json", got)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := r.Form.Get("client_id"); got != "client-id" {
				t.Fatalf("client_id = %q", got)
			}
			if got := r.Form.Get("client_secret"); got != "client-secret" {
				t.Fatalf("client_secret = %q", got)
			}
			if got := r.Form.Get("code"); got != "oauth-code" {
				t.Fatalf("code = %q", got)
			}
			if got := r.Form.Get("redirect_uri"); got != "http://127.0.0.1:18080/api/v1/connectors/github/oauth/callback" {
				t.Fatalf("redirect_uri = %q", got)
			}
			if got := r.Form.Get("code_verifier"); got != "verifier" {
				t.Fatalf("code_verifier = %q", got)
			}
			sawTokenExchange = true
			writeJSONResponse(t, w, map[string]any{
				"access_token": "gh-token",
				"token_type":   "bearer",
				"scope":        "repo,read:user,user:email",
			})
		case "/user":
			if got := r.Header.Get("Authorization"); got != "Bearer gh-token" {
				t.Fatalf("user Authorization = %q", got)
			}
			sawUserFetch = true
			writeJSONResponse(t, w, map[string]any{
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

	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	service := NewService(NewStore(filepath.Join(t.TempDir(), "state.json")))
	service.HTTPClient = github.Client()
	service.Endpoints = Endpoints{
		AuthorizeURL: github.URL + "/login/oauth/authorize",
		TokenURL:     github.URL + "/login/oauth/access_token",
		APIBaseURL:   github.URL,
	}
	service.Now = func() time.Time { return now }
	service.GenerateOAuthState = func() (OAuthState, error) {
		return OAuthState{State: "state-value", CodeVerifier: "verifier"}, nil
	}

	status, err := service.SaveConfig(context.Background(), ProviderGitHub, Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	})
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if !status.Configured || status.Connected {
		t.Fatalf("status after config = %+v", status)
	}
	if strings.Join(status.Scopes, " ") != strings.Join(DefaultGitHubScopes, " ") {
		t.Fatalf("default scopes = %#v", status.Scopes)
	}

	start, err := service.StartOAuth(context.Background(), ProviderGitHub, OAuthStartOptions{
		CallbackURL: "http://127.0.0.1:18080/api/v1/connectors/github/oauth/callback",
		ReturnURL:   "http://127.0.0.1:18080/#/workspace",
	})
	if err != nil {
		t.Fatalf("StartOAuth() error = %v", err)
	}
	authorizeURL, err := url.Parse(start.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse authorization url: %v", err)
	}
	if authorizeURL.Path != "/login/oauth/authorize" {
		t.Fatalf("authorize path = %q", authorizeURL.Path)
	}
	if got := authorizeURL.Query().Get("scope"); got != "repo read:user user:email" {
		t.Fatalf("scope = %q", got)
	}
	if got := authorizeURL.Query().Get("state"); got != "state-value" {
		t.Fatalf("state = %q", got)
	}
	if got := authorizeURL.Query().Get("code_challenge_method"); got != "S256" {
		t.Fatalf("code_challenge_method = %q", got)
	}
	if authorizeURL.Query().Get("code_challenge") == "" {
		t.Fatal("code_challenge is empty")
	}
	pendingStatus, err := service.Status(context.Background(), ProviderGitHub, "")
	if err != nil {
		t.Fatalf("Status() after StartOAuth error = %v", err)
	}
	if !pendingStatus.OAuthPending {
		t.Fatalf("Status() OAuthPending = false after StartOAuth: %+v", pendingStatus)
	}

	callbackStatus, err := service.CompleteOAuth(context.Background(), ProviderGitHub, url.Values{
		"code":  []string{"oauth-code"},
		"state": []string{"state-value"},
	})
	if err != nil {
		t.Fatalf("CompleteOAuth() error = %v", err)
	}
	if !sawTokenExchange || !sawUserFetch {
		t.Fatalf("token exchange=%v user fetch=%v, want both", sawTokenExchange, sawUserFetch)
	}
	if !callbackStatus.Connected || callbackStatus.Account == nil || callbackStatus.Account.Login != "octocat" {
		t.Fatalf("callback status = %+v", callbackStatus)
	}
	if callbackStatus.OAuthPending {
		t.Fatalf("callback status OAuthPending = true: %+v", callbackStatus)
	}

	credential, err := service.Credential(context.Background(), ProviderGitHub)
	if err != nil {
		t.Fatalf("Credential() error = %v", err)
	}
	if credential.AccessToken != "gh-token" || credential.TokenType != "bearer" {
		t.Fatalf("credential = %+v", credential)
	}

	disconnected, err := service.Disconnect(context.Background(), ProviderGitHub)
	if err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	if !disconnected.Configured || disconnected.Connected || !disconnected.ClientSecretSet {
		t.Fatalf("disconnect status = %+v", disconnected)
	}
	if _, err := service.Credential(context.Background(), ProviderGitHub); err == nil {
		t.Fatal("Credential() error = nil after disconnect")
	}
}

func TestServiceStartOAuthUsesManagedGitHubAppWhenUserConfigIsEmpty(t *testing.T) {
	service := NewService(NewStore(filepath.Join(t.TempDir(), "state.json")))
	service.Endpoints = Endpoints{
		AuthorizeURL: "https://github.example/login/oauth/authorize",
	}
	service.GitHubOAuthApp = Config{
		ClientID:     "managed-client-id",
		ClientSecret: "managed-client-secret",
	}
	service.GenerateOAuthState = func() (OAuthState, error) {
		return OAuthState{State: "state-value", CodeVerifier: "verifier"}, nil
	}

	start, err := service.StartOAuth(context.Background(), ProviderGitHub, OAuthStartOptions{
		CallbackURL: "http://127.0.0.1:18080/api/v1/connectors/github/oauth/callback",
	})
	if err != nil {
		t.Fatalf("StartOAuth() error = %v", err)
	}
	authorizeURL, err := url.Parse(start.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse authorization url: %v", err)
	}
	if got := authorizeURL.Query().Get("client_id"); got != "managed-client-id" {
		t.Fatalf("client_id = %q", got)
	}
	if got := authorizeURL.Query().Get("redirect_uri"); got != "http://127.0.0.1:18080/api/v1/connectors/github/oauth/callback" {
		t.Fatalf("redirect_uri = %q", got)
	}
	if got := authorizeURL.Query().Get("scope"); got != "repo read:user user:email" {
		t.Fatalf("scope = %q", got)
	}

	state, ok, err := service.Store.LoadGitHub()
	if err != nil {
		t.Fatalf("LoadGitHub() error = %v", err)
	}
	if !ok || state.Pending == nil {
		t.Fatalf("pending state was not persisted: ok=%v state=%+v", ok, state)
	}
	if state.Config.ClientID != "" || state.Config.ClientSecret != "" {
		t.Fatalf("managed app credentials leaked into root state: %+v", state.Config)
	}
}

func TestServiceStartGitHubAppInstallBuildsInstallURL(t *testing.T) {
	service := NewService(NewStore(filepath.Join(t.TempDir(), "state.json")))
	service.GitHubAppSlug = "csgclaw"
	service.GenerateOAuthState = func() (OAuthState, error) {
		return OAuthState{State: "install-state", CodeVerifier: "verifier"}, nil
	}

	start, err := service.StartGitHubAppInstall(context.Background(), ProviderGitHub)
	if err != nil {
		t.Fatalf("StartGitHubAppInstall() error = %v", err)
	}
	if start.Provider != ProviderGitHub {
		t.Fatalf("provider = %q", start.Provider)
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

func TestServiceStartGitHubAppInstallRequiresSlug(t *testing.T) {
	service := NewService(NewStore(filepath.Join(t.TempDir(), "state.json")))
	service.GitHubAppSlug = " "

	if _, err := service.StartGitHubAppInstall(context.Background(), ProviderGitHub); err == nil {
		t.Fatal("StartGitHubAppInstall() error = nil")
	}
}

func TestServiceCallbackRejectsStateMismatch(t *testing.T) {
	service := NewService(NewStore(filepath.Join(t.TempDir(), "state.json")))
	service.GenerateOAuthState = func() (OAuthState, error) {
		return OAuthState{State: "state-value", CodeVerifier: "verifier"}, nil
	}
	if _, err := service.SaveConfig(context.Background(), ProviderGitHub, Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
	}); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if _, err := service.StartOAuth(context.Background(), ProviderGitHub, OAuthStartOptions{
		CallbackURL: "http://127.0.0.1:18080/api/v1/connectors/github/oauth/callback",
	}); err != nil {
		t.Fatalf("StartOAuth() error = %v", err)
	}

	_, err := service.CompleteOAuth(context.Background(), ProviderGitHub, url.Values{
		"code":  []string{"oauth-code"},
		"state": []string{"wrong-state"},
	})
	if err == nil || !IsCallbackValidationError(err) {
		t.Fatalf("CompleteOAuth() error = %v, want callback validation error", err)
	}
}

func writeJSONResponse(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
