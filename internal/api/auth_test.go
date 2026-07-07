package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/auth"
)

func TestHandleAuthStatus(t *testing.T) {
	loggedInAt := time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)
	restore := stubAuthStatus(func(*http.Request) (auth.Status, error) {
		return auth.Status{
			Authenticated: true,
			UserID:        "alice",
			UserUUID:      "user-1",
			Avatar:        "https://example.test/avatar.png",
			BaseURL:       "https://hub.example.test",
			PortalURL:     "https://hub.example.test/portal",
			LoggedInAt:    &loggedInAt,
		}, nil
	})
	defer restore()

	rec := httptest.NewRecorder()
	(&Handler{}).Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got auth.Status
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.Authenticated || got.UserID != "alice" || got.BaseURL != "https://hub.example.test" {
		t.Fatalf("response = %+v, want authenticated alice", got)
	}
}

func TestHandleAuthLogin(t *testing.T) {
	var gotReturnURL string
	var gotCallbackURL string
	var gotOpenCSGBaseURL string
	var gotCSGHubBaseURL string
	var gotAIGatewayBaseURL string
	restore := stubAuthLogin(func(_ *http.Request, req authLoginRequest) (auth.LoginResponse, error) {
		gotReturnURL = req.ReturnURL
		gotCallbackURL = req.CallbackURL
		gotOpenCSGBaseURL = req.OpenCSGBaseURL
		gotCSGHubBaseURL = req.CSGHubBaseURL
		gotAIGatewayBaseURL = req.AIGatewayBaseURL
		return auth.LoginResponse{LoginURL: "https://iam.example.test/login"}, nil
	})
	defer restore()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{
		"return_url":"http://127.0.0.1:18080/#/dms/room-1",
		"opencsg_base_url":"https://opencsg-stg.com",
		"csghub_base_url":"https://opencsg-stg.com",
		"ai_gateway_base_url":"https://aigateway.opencsg-stg.com/v1"
	}`))
	req.Host = "127.0.0.1:18080"
	(&Handler{}).Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got auth.LoginResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.LoginURL != "https://iam.example.test/login" {
		t.Fatalf("LoginURL = %q", got.LoginURL)
	}
	if gotReturnURL != "http://127.0.0.1:18080/#/dms/room-1" {
		t.Fatalf("return_url = %q", gotReturnURL)
	}
	if gotCallbackURL != "http://127.0.0.1:18080/api/v1/auth/callback" {
		t.Fatalf("callback_url = %q", gotCallbackURL)
	}
	if gotOpenCSGBaseURL != "https://opencsg-stg.com" || gotCSGHubBaseURL != "https://opencsg-stg.com" {
		t.Fatalf("login base urls = %q/%q", gotOpenCSGBaseURL, gotCSGHubBaseURL)
	}
	if gotAIGatewayBaseURL != "https://aigateway.opencsg-stg.com/v1" {
		t.Fatalf("ai_gateway_base_url = %q", gotAIGatewayBaseURL)
	}
}

func TestHandleAuthCallback(t *testing.T) {
	var gotToken string
	restore := stubAuthCallback(func(r *http.Request) (string, error) {
		gotToken = r.URL.Query().Get("jwt_token")
		return "http://127.0.0.1:18080/#/workspace", nil
	})
	defer restore()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?jwt_token=jwt-value", nil)
	(&Handler{}).Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusFound, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "http://127.0.0.1:18080/#/workspace" {
		t.Fatalf("Location = %q", got)
	}
	if gotToken != "jwt-value" {
		t.Fatalf("jwt_token = %q", gotToken)
	}
}

func TestHandleAuthLogout(t *testing.T) {
	restore := stubAuthLogout(func(*http.Request) (auth.Status, error) {
		return auth.Status{}, nil
	})
	defer restore()

	rec := httptest.NewRecorder()
	(&Handler{}).Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got auth.Status
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Authenticated {
		t.Fatalf("response = %+v, want unauthenticated", got)
	}
}

func stubAuthStatus(fn func(*http.Request) (auth.Status, error)) func() {
	previous := appAuthStatus
	appAuthStatus = fn
	return func() { appAuthStatus = previous }
}

func stubAuthLogin(fn func(*http.Request, authLoginRequest) (auth.LoginResponse, error)) func() {
	previous := appAuthLogin
	appAuthLogin = fn
	return func() { appAuthLogin = previous }
}

func stubAuthCallback(fn func(*http.Request) (string, error)) func() {
	previous := appAuthCallback
	appAuthCallback = fn
	return func() { appAuthCallback = previous }
}

func stubAuthLogout(fn func(*http.Request) (auth.Status, error)) func() {
	previous := appAuthLogout
	appAuthLogout = fn
	return func() { appAuthLogout = previous }
}
