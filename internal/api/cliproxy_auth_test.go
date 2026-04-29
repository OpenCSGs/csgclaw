package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/cliproxy"
)

func TestHandleCLIProxyAuthStatus(t *testing.T) {
	restoreStatus := stubCLIProxyAuthStatus(func(r *http.Request, provider string) (cliproxy.AuthStatus, error) {
		if provider != "codex" {
			t.Fatalf("provider = %q, want codex", provider)
		}
		return cliproxy.AuthStatus{Provider: "codex", Authenticated: true, Source: "cli-proxy", SupportsLogin: true}, nil
	})
	defer restoreStatus()

	rec := httptest.NewRecorder()
	(&Handler{}).Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/cliproxy/auth/status?provider=codex", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got cliproxy.AuthStatus
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Authenticated || got.Source != "cli-proxy" {
		t.Fatalf("response = %+v, want authenticated cli-proxy", got)
	}
}

func TestHandleCLIProxyAuthStatusRequiresProvider(t *testing.T) {
	rec := httptest.NewRecorder()
	(&Handler{}).Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/cliproxy/auth/status", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleCLIProxyAuthLogin(t *testing.T) {
	restoreLogin := stubCLIProxyAuthLogin(func(r *http.Request, req cliproxyAuthLoginRequest) (cliproxy.AuthStatus, error) {
		if req.Provider != "claude_code" || !req.NoBrowser {
			t.Fatalf("request = %+v, want claude_code no_browser", req)
		}
		return cliproxy.AuthStatus{Provider: "claude_code", Authenticated: true, Source: "oauth", SupportsLogin: true}, nil
	})
	defer restoreLogin()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cliproxy/auth/login", strings.NewReader(`{"provider":"claude_code","no_browser":true}`))
	(&Handler{}).Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got cliproxy.AuthStatus
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Authenticated || got.Source != "oauth" {
		t.Fatalf("response = %+v, want authenticated oauth", got)
	}
}

func TestHandleCLIProxyAuthLoginRequiresProvider(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cliproxy/auth/login", strings.NewReader(`{"no_browser":true}`))
	(&Handler{}).Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func stubCLIProxyAuthStatus(fn func(*http.Request, string) (cliproxy.AuthStatus, error)) func() {
	previous := cliproxyAuthStatus
	cliproxyAuthStatus = fn
	return func() { cliproxyAuthStatus = previous }
}

func stubCLIProxyAuthLogin(fn func(*http.Request, cliproxyAuthLoginRequest) (cliproxy.AuthStatus, error)) func() {
	previous := cliproxyAuthLogin
	cliproxyAuthLogin = fn
	return func() { cliproxyAuthLogin = previous }
}
