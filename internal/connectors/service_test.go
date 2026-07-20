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
	var sawUserFetch int
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
			sawUserFetch++
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
	if !sawTokenExchange || sawUserFetch != 1 {
		t.Fatalf("token exchange=%v user fetch=%d, want exchange and one user fetch", sawTokenExchange, sawUserFetch)
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
	if sawUserFetch != 2 {
		t.Fatalf("Credential() validation user fetch count = %d, want 2", sawUserFetch)
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

func TestServiceGitLabPATFlow(t *testing.T) {
	gitlab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/user" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("PRIVATE-TOKEN"); got != "glpat-secret" {
			t.Fatalf("PRIVATE-TOKEN = %q", got)
		}
		writeJSONResponse(t, w, map[string]any{
			"username": "gitlab-user",
			"id":       42,
			"name":     "GitLab User",
			"web_url":  gitlabURL(r) + "/gitlab-user",
		})
	}))
	t.Cleanup(gitlab.Close)

	service := NewService(NewStore(filepath.Join(t.TempDir(), "state.json")))
	service.HTTPClient = gitlab.Client()
	status, err := service.SaveGitLabConfig(context.Background(), Config{BaseURL: gitlab.URL + "/", AccessToken: "glpat-secret"})
	if err != nil {
		t.Fatalf("SaveGitLabConfig() error = %v", err)
	}
	if !status.Configured || !status.Connected || status.BaseURL != gitlab.URL || !status.AccessTokenSet {
		t.Fatalf("status = %+v", status)
	}
	credential, err := service.Credential(context.Background(), ProviderGitLab)
	if err != nil {
		t.Fatalf("Credential() error = %v", err)
	}
	if credential.BaseURL != gitlab.URL || credential.AccessToken != "glpat-secret" || credential.TokenType != "private-token" {
		t.Fatalf("credential = %+v", credential)
	}
	disconnected, err := service.Disconnect(context.Background(), ProviderGitLab)
	if err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	if disconnected.Connected || disconnected.AccessTokenSet || disconnected.BaseURL != gitlab.URL {
		t.Fatalf("disconnected status = %+v", disconnected)
	}
}

func gitlabURL(r *http.Request) string {
	return "http://" + r.Host
}

func TestServiceCredentialRejectsInvalidGitHubTokenWithoutLeakingIt(t *testing.T) {
	const invalidToken = "gho_invalid_secret"
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			if got := r.Header.Get("Authorization"); got != "Bearer "+invalidToken {
				t.Fatalf("user Authorization = %q", got)
			}
			http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(github.Close)

	store := NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.SaveGitHub(State{
		Token: &Token{
			AccessToken: invalidToken,
			TokenType:   "bearer",
			Scopes:      []string{"repo"},
		},
		Account: &Account{Login: "octocat", ID: 1},
	}); err != nil {
		t.Fatalf("SaveGitHub() error = %v", err)
	}
	service := NewService(store)
	service.HTTPClient = github.Client()
	service.Endpoints = Endpoints{APIBaseURL: github.URL}

	_, err := service.Credential(context.Background(), ProviderGitHub)
	if err == nil {
		t.Fatal("Credential() error = nil, want invalid token error")
	}
	if !strings.Contains(err.Error(), "reconnect GitHub") {
		t.Fatalf("Credential() error = %v, want reconnect guidance", err)
	}
	if strings.Contains(err.Error(), invalidToken) {
		t.Fatalf("Credential() error leaked token: %v", err)
	}
}

func TestServiceCredentialRefreshesExpiredGitHubAppUserToken(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	var sawRefresh bool
	var sawUserFetch bool
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			if r.Method != http.MethodPost {
				t.Fatalf("refresh method = %s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := r.Form.Get("grant_type"); got != "refresh_token" {
				t.Fatalf("grant_type = %q, want refresh_token", got)
			}
			if got := r.Form.Get("client_id"); got != "client-id" {
				t.Fatalf("client_id = %q, want client-id", got)
			}
			if got := r.Form.Get("client_secret"); got != "client-secret" {
				t.Fatalf("client_secret = %q, want client-secret", got)
			}
			if got := r.Form.Get("refresh_token"); got != "old-refresh" {
				t.Fatalf("refresh_token = %q, want old-refresh", got)
			}
			sawRefresh = true
			writeJSONResponse(t, w, map[string]any{
				"access_token":             "new-access",
				"token_type":               "bearer",
				"scope":                    "repo,read:user",
				"refresh_token":            "new-refresh",
				"expires_in":               28800,
				"refresh_token_expires_in": 15897600,
			})
		case "/user":
			if got := r.Header.Get("Authorization"); got != "Bearer new-access" {
				t.Fatalf("user Authorization = %q, want refreshed token", got)
			}
			sawUserFetch = true
			writeJSONResponse(t, w, map[string]any{
				"login": "octocat",
				"id":    1,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(github.Close)

	store := NewStore(filepath.Join(t.TempDir(), "state.json"))
	if err := store.SaveGitHub(State{
		Config: Config{
			ClientID:     "client-id",
			ClientSecret: "client-secret",
		},
		Token: &Token{
			AccessToken:           "old-access",
			TokenType:             "bearer",
			Scopes:                []string{"repo"},
			RefreshToken:          "old-refresh",
			ExpiresAt:             now.Add(-time.Minute),
			RefreshTokenExpiresAt: now.Add(24 * time.Hour),
		},
		Account: &Account{Login: "octocat", ID: 1},
	}); err != nil {
		t.Fatalf("SaveGitHub() error = %v", err)
	}
	service := NewService(store)
	service.HTTPClient = github.Client()
	service.Endpoints = Endpoints{
		TokenURL:   github.URL + "/login/oauth/access_token",
		APIBaseURL: github.URL,
	}
	service.Now = func() time.Time { return now }

	credential, err := service.Credential(context.Background(), ProviderGitHub)
	if err != nil {
		t.Fatalf("Credential() error = %v", err)
	}
	if credential.AccessToken != "new-access" {
		t.Fatalf("Credential().AccessToken = %q, want refreshed token", credential.AccessToken)
	}
	if !sawRefresh || !sawUserFetch {
		t.Fatalf("refresh=%v user fetch=%v, want both", sawRefresh, sawUserFetch)
	}

	state, ok, err := store.LoadGitHub()
	if err != nil {
		t.Fatalf("LoadGitHub() error = %v", err)
	}
	if !ok || state.Token == nil {
		t.Fatalf("stored state missing token: ok=%v state=%+v", ok, state)
	}
	if state.Token.AccessToken != "new-access" || state.Token.RefreshToken != "new-refresh" {
		t.Fatalf("stored token = %+v, want refreshed access and refresh tokens", state.Token)
	}
	if state.Token.ExpiresAt.IsZero() || !state.Token.ExpiresAt.After(now) {
		t.Fatalf("stored token ExpiresAt = %v, want future expiry", state.Token.ExpiresAt)
	}
	if state.Token.RefreshTokenExpiresAt.IsZero() || !state.Token.RefreshTokenExpiresAt.After(now) {
		t.Fatalf("stored token RefreshTokenExpiresAt = %v, want future expiry", state.Token.RefreshTokenExpiresAt)
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
