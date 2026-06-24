package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestCompleteCallbackStoresCredentials(t *testing.T) {
	var sawTokenAuth bool
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/user/alice/tokens":
			if got := r.URL.Query().Get("app"); got != "git" {
				t.Fatalf("app query = %q, want git", got)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer "+testJWT("alice", "user-1") {
				t.Fatalf("Authorization = %q", got)
			}
			sawTokenAuth = true
			writeJSON(t, w, map[string]any{
				"msg": "OK",
				"data": []map[string]any{{
					"token": "access-token",
				}},
			})
		case "/api/v1/user/alice":
			writeJSON(t, w, map[string]any{
				"msg": "OK",
				"data": map[string]any{
					"avatar": "https://example.test/avatar.png",
				},
			})
		case "/api/v1/namespaces/user-1/apikeys/builtin":
			if got := r.URL.Query().Get("current_user"); got != "alice" {
				t.Fatalf("builtin current_user = %q, want alice", got)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer "+testJWT("alice", "user-1") {
				t.Fatalf("builtin Authorization = %q", got)
			}
			writeJSON(t, w, map[string]any{
				"msg": "OK",
				"data": map[string]any{
					"token": "gk_aigateway-key",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)

	store := newTestStore(t)
	now := time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)
	service := &Service{
		Store:         store,
		CSGHubBaseURL: api.URL,
		HTTPClient:    api.Client(),
		Now:           func() time.Time { return now },
	}

	returnURL := "http://127.0.0.1:18080/#/dms/room-1"
	redirectURL, err := service.CompleteCallback(context.Background(), url.Values{
		"jwt_token":  []string{testJWT("alice", "user-1")},
		"return_url": []string{returnURL},
	})
	if err != nil {
		t.Fatalf("CompleteCallback() error = %v", err)
	}
	if redirectURL != returnURL {
		t.Fatalf("callback redirect = %q", redirectURL)
	}
	if !sawTokenAuth {
		t.Fatal("token endpoint was not called")
	}

	record, ok, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("auth record not saved")
	}
	if record.Tokens.AccessToken != "access-token" {
		t.Fatalf("record = %+v, want saved access token", record)
	}
	if record.Account.UserID != "alice" || record.Account.UserUUID != "user-1" {
		t.Fatalf("record user = %q/%q", record.Account.UserID, record.Account.UserUUID)
	}
	if record.Account.BaseURL != api.URL {
		t.Fatalf("record BaseURL = %q, want %q", record.Account.BaseURL, api.URL)
	}
	if !record.Account.LoggedInAt.Equal(now) {
		t.Fatalf("LoggedInAt = %s, want %s", record.Account.LoggedInAt, now)
	}
	if !record.LastRefresh.Equal(now) {
		t.Fatalf("LastRefresh = %s, want %s", record.LastRefresh, now)
	}
	credentials, ok, err := store.LoadCSGHubProviderCredentials()
	if err != nil || !ok {
		t.Fatalf("LoadCSGHubProviderCredentials() = %+v, %v, %v", credentials, ok, err)
	}
	if credentials.AIGatewayBuiltinAPIKey != "gk_aigateway-key" {
		t.Fatalf("AIGatewayBuiltinAPIKey = %q, want gk_aigateway-key", credentials.AIGatewayBuiltinAPIKey)
	}
}

func TestLoginUsesOpenCSGSSOCallbackURL(t *testing.T) {
	service := &Service{
		OpenCSGBaseURL: "https://opencsg.example.test",
	}

	returnURL := "http://127.0.0.1:18080/#/dms/room-1"
	callbackURL := "http://127.0.0.1:18080/api/v1/auth/callback"
	login, err := service.Login(context.Background(), LoginOptions{
		ReturnURL:   returnURL,
		CallbackURL: callbackURL,
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	parsedLogin, err := url.Parse(login.LoginURL)
	if err != nil {
		t.Fatalf("parse LoginURL: %v", err)
	}
	if got := parsedLogin.Scheme + "://" + parsedLogin.Host + parsedLogin.Path; got != "https://opencsg.example.test/sso/login" {
		t.Fatalf("login URL base = %q", got)
	}
	redirectURL := parsedLogin.Query().Get("redirect_url")
	if redirectURL == "" {
		t.Fatal("redirect_url is empty")
	}
	parsedRedirect, err := url.Parse(redirectURL)
	if err != nil {
		t.Fatalf("parse redirect_url: %v", err)
	}
	if got := parsedRedirect.Scheme + "://" + parsedRedirect.Host + parsedRedirect.Path; got != callbackURL {
		t.Fatalf("redirect callback = %q, want %q", got, callbackURL)
	}
	if got := parsedRedirect.Query().Get("return_url"); got != returnURL {
		t.Fatalf("return_url = %q, want %q", got, returnURL)
	}
}

func TestCallbackRejectsMissingParams(t *testing.T) {
	service := &Service{}
	_, err := service.completeCallback(context.Background(), url.Values{})
	if err == nil || !isCallbackValidationError(err) {
		t.Fatalf("completeCallback() error = %v, want validation error", err)
	}
}

func TestCallbackRejectsInvalidJWT(t *testing.T) {
	service := &Service{}
	_, err := service.completeCallback(context.Background(), url.Values{
		"jwt_token": []string{"not-a-jwt"},
	})
	if err == nil || !isCallbackValidationError(err) {
		t.Fatalf("completeCallback() error = %v, want validation error", err)
	}
}

func TestCallbackAllowsMissingBuiltinAPIKey(t *testing.T) {
	var sawBuiltin bool
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/user/alice/tokens":
			writeJSON(t, w, map[string]any{
				"msg":  "OK",
				"data": []map[string]any{{"token": "access-token"}},
			})
		case "/api/v1/user/alice":
			writeJSON(t, w, map[string]any{"msg": "OK", "data": map[string]any{}})
		case "/api/v1/namespaces/user-1/apikeys/builtin":
			if got := r.URL.Query().Get("current_user"); got != "alice" {
				t.Fatalf("builtin current_user = %q, want alice", got)
			}
			sawBuiltin = true
			http.Error(w, "not available", http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)

	store := newTestStore(t)
	service := &Service{Store: store, CSGHubBaseURL: api.URL, HTTPClient: api.Client()}
	redirect, err := service.completeCallback(context.Background(), url.Values{
		"jwt_token": []string{testJWT("alice", "user-1")},
	})
	if err != nil {
		t.Fatalf("completeCallback() error = %v", err)
	}
	if redirect != api.URL {
		t.Fatalf("redirect = %q", redirect)
	}
	if !sawBuiltin {
		t.Fatal("builtin endpoint was not attempted")
	}
	record, ok, err := store.Load()
	if err != nil || !ok {
		t.Fatalf("Load() = %+v, %v, %v", record, ok, err)
	}
	_, ok, err = store.LoadCSGHubProviderCredentials()
	if err != nil {
		t.Fatalf("LoadCSGHubProviderCredentials() error = %v", err)
	}
	if ok {
		t.Fatal("provider credentials saved when builtin fetch fails")
	}
}

func TestCallbackReturnURLAcceptsLangflowURLParam(t *testing.T) {
	returnURL := "http://127.0.0.1:18080/#/workspace"
	got := callbackReturnURL(url.Values{
		"url": []string{returnURL},
	})
	if got != returnURL {
		t.Fatalf("callbackReturnURL() = %q, want %q", got, returnURL)
	}
}

func TestCallbackReturnURLRejectsExternalURLs(t *testing.T) {
	got := callbackReturnURL(url.Values{
		"return_url": []string{"https://evil.example.test/callback"},
		"url":        []string{"https://also-evil.example.test/callback"},
	})
	if got != "" {
		t.Fatalf("callbackReturnURL() = %q, want empty for external URLs", got)
	}
}

func TestCompleteCallbackRedirectsToTrustedPortalURL(t *testing.T) {
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/user/alice/tokens":
			writeJSON(t, w, map[string]any{
				"msg":  "OK",
				"data": []map[string]any{{"token": "access-token"}},
			})
		case "/api/v1/user/alice":
			writeJSON(t, w, map[string]any{"msg": "OK", "data": map[string]any{}})
		case "/api/v1/namespaces/user-1/apikeys/builtin":
			http.Error(w, "not available", http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)

	store := newTestStore(t)
	service := &Service{Store: store, CSGHubBaseURL: api.URL, HTTPClient: api.Client()}
	jwtToken := testJWT("alice", "user-1")
	portalURL := api.URL + "/portal?next=workspace"

	redirect, err := service.completeCallback(context.Background(), url.Values{
		"jwt_token":  []string{jwtToken},
		"portal_url": []string{portalURL},
	})
	if err != nil {
		t.Fatalf("completeCallback() error = %v", err)
	}
	want := portalRedirectURL(portalURL, jwtToken)
	if redirect != want {
		t.Fatalf("redirect = %q, want %q", redirect, want)
	}
	record, ok, err := store.Load()
	if err != nil || !ok {
		t.Fatalf("Load() = %+v, %v, %v", record, ok, err)
	}
	if record.Account.PortalURL != portalURL {
		t.Fatalf("PortalURL = %q, want %q", record.Account.PortalURL, portalURL)
	}
}

func TestCallbackRejectsUntrustedPortalURL(t *testing.T) {
	service := &Service{CSGHubBaseURL: "https://hub.opencsg.com"}
	_, err := service.completeCallback(context.Background(), url.Values{
		"jwt_token":  []string{testJWT("alice", "user-1")},
		"portal_url": []string{"https://evil.example.test/callback"},
	})
	if err == nil || !isCallbackValidationError(err) {
		t.Fatalf("completeCallback() error = %v, want validation error", err)
	}
}

func TestSanitizePortalURLRequiresSameOrigin(t *testing.T) {
	baseURL := "https://hub.opencsg.com"
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "same origin",
			raw:  "https://hub.opencsg.com/portal?next=workspace",
			want: "https://hub.opencsg.com/portal?next=workspace",
		},
		{
			name: "default port",
			raw:  "https://hub.opencsg.com:443/portal",
			want: "https://hub.opencsg.com:443/portal",
		},
		{
			name: "external host",
			raw:  "https://evil.example.test/portal",
		},
		{
			name: "lookalike host",
			raw:  "https://hub.opencsg.com.evil.example/portal",
		},
		{
			name: "different scheme",
			raw:  "http://hub.opencsg.com/portal",
		},
		{
			name: "relative URL",
			raw:  "/portal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizePortalURL(tt.raw, baseURL); got != tt.want {
				t.Fatalf("sanitizePortalURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLogoutDeletesAuth(t *testing.T) {
	store := newTestStore(t)
	if err := store.Save(Record{
		Tokens:  Tokens{AccessToken: "token"},
		Account: Account{BaseURL: "https://hub.example.test"},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	service := &Service{Store: store}

	status, err := service.Logout(context.Background())
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if status.Authenticated {
		t.Fatalf("Logout() status = %+v, want unauthenticated", status)
	}
	_, ok, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if ok {
		t.Fatal("auth record still exists after logout")
	}
}

func testJWT(userID, userUUID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, err := json.Marshal(map[string]string{
		"current_user": userID,
		"uuid":         userUUID,
	})
	if err != nil {
		panic(err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func writeJSON(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
