package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agent"
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

func TestHandleAuthLoginUsesAdvertiseBaseURL(t *testing.T) {
	const advertiseBaseURL = "https://aigateway.opencsg-stg.com/v1/sandboxes/jared-1784118727"
	t.Setenv("CSGCLAW_ADVERTISE_BASE_URL", advertiseBaseURL)
	configPath := filepath.Join(t.TempDir(), "config.toml")
	content := `[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "${CSGCLAW_ADVERTISE_BASE_URL}"
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	var got authLoginRequest
	restore := stubAuthLogin(func(_ *http.Request, req authLoginRequest) (auth.LoginResponse, error) {
		got = req
		return auth.LoginResponse{LoginURL: "https://opencsg-stg.com/sso/login"}, nil
	})
	defer restore()

	handler := &Handler{}
	handler.SetConfigPath(configPath)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{
		"return_url":"https://aigateway.opencsg-stg.com/v1/sandboxes/jared-1784118727/#/workspace"
	}`))
	req.Host = "csgclaw.internal:18080"
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got.AdvertiseBaseURL != advertiseBaseURL {
		t.Fatalf("advertise_base_url = %q, want %q", got.AdvertiseBaseURL, advertiseBaseURL)
	}
	if want := advertiseBaseURL + authCallbackPath; got.CallbackURL != want {
		t.Fatalf("callback_url = %q, want %q", got.CallbackURL, want)
	}
}

func TestHandleAuthCallback(t *testing.T) {
	var gotToken string
	restore := stubAuthCallback(func(r *http.Request, _ string) (string, error) {
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

func TestHandleAuthCallbackRefreshesOpenCSGModels(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	writeMinimalAPIConfig(t, configPath)
	restoreCallback := stubAuthCallback(func(*http.Request, string) (string, error) {
		return "http://127.0.0.1:18080/#/settings", nil
	})
	defer restoreCallback()
	restoreCheck := stubModelProviderCheck(func(_ context.Context, input agent.ModelProviderCheckInput) agent.ModelProviderCheckResult {
		if input.ID != agent.ModelProviderIDOpenCSG {
			t.Fatalf("provider ID = %q, want OpenCSG", input.ID)
		}
		return agent.ModelProviderCheckResult{
			ID:            agent.ModelProviderIDOpenCSG,
			Status:        agent.ModelProviderStatusConnected,
			Message:       "connected",
			Models:        []string{"stage-model", "stage-model-mini"},
			LastCheckedAt: "2026-07-16T10:00:00Z",
		}
	})
	defer restoreCheck()
	handler := &Handler{}
	handler.SetConfigPath(configPath)

	callbackRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(callbackRec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?jwt_token=jwt-value", nil))
	if callbackRec.Code != http.StatusFound {
		t.Fatalf("callback status = %d, want %d; body=%s", callbackRec.Code, http.StatusFound, callbackRec.Body.String())
	}

	providersRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(providersRec, httptest.NewRequest(http.MethodGet, "/api/v1/model-providers", nil))
	if providersRec.Code != http.StatusOK {
		t.Fatalf("providers status = %d, want %d; body=%s", providersRec.Code, http.StatusOK, providersRec.Body.String())
	}
	var got modelProviderTestResponse
	if err := json.NewDecoder(providersRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	if len(got.Providers) == 0 || got.Providers[0].ID != agent.ModelProviderIDOpenCSG {
		t.Fatalf("providers = %+v, want OpenCSG first", got.Providers)
	}
	if got.Providers[0].Status != agent.ModelProviderStatusConnected || len(got.Providers[0].Models) != 2 {
		t.Fatalf("OpenCSG provider after callback = %+v, want refreshed models", got.Providers[0])
	}
}

func TestHandleAuthCallbackClearsStaleOpenCSGModels(t *testing.T) {
	tests := []struct {
		name       string
		result     agent.ModelProviderCheckResult
		wantStatus string
	}{
		{
			name: "connected with empty model list",
			result: agent.ModelProviderCheckResult{
				ID:            agent.ModelProviderIDOpenCSG,
				Status:        agent.ModelProviderStatusConnected,
				Message:       "connected",
				Models:        []string{},
				LastCheckedAt: "2026-07-16T10:00:00Z",
			},
			wantStatus: agent.ModelProviderStatusConnected,
		},
		{
			name: "check failed",
			result: agent.ModelProviderCheckResult{
				ID:            agent.ModelProviderIDOpenCSG,
				Status:        agent.ModelProviderStatusFailed,
				Message:       "stage gateway unavailable",
				LastCheckedAt: "2026-07-16T10:00:00Z",
			},
			wantStatus: agent.ModelProviderStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.toml")
			writeCachedOpenCSGModelsConfig(t, configPath)
			restoreCallback := stubAuthCallback(func(*http.Request, string) (string, error) {
				return "http://127.0.0.1:18080/#/settings", nil
			})
			defer restoreCallback()
			restoreCheck := stubModelProviderCheck(func(context.Context, agent.ModelProviderCheckInput) agent.ModelProviderCheckResult {
				return tt.result
			})
			defer restoreCheck()
			handler := &Handler{}
			handler.SetConfigPath(configPath)

			callbackRec := httptest.NewRecorder()
			handler.Routes().ServeHTTP(callbackRec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?jwt_token=jwt-value", nil))
			if callbackRec.Code != http.StatusFound {
				t.Fatalf("callback status = %d, want %d; body=%s", callbackRec.Code, http.StatusFound, callbackRec.Body.String())
			}

			provider := readOpenCSGProvider(t, handler)
			if len(provider.Models) != 0 {
				t.Fatalf("OpenCSG models after callback = %+v, want stale models cleared", provider.Models)
			}
			if provider.Status != tt.wantStatus {
				t.Fatalf("OpenCSG status after callback = %q, want %q", provider.Status, tt.wantStatus)
			}
		})
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

func TestHandleAuthLogoutClearsCachedOpenCSGModels(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	writeCachedOpenCSGModelsConfig(t, configPath)
	restore := stubAuthLogout(func(*http.Request) (auth.Status, error) {
		return auth.Status{}, nil
	})
	defer restore()
	handler := &Handler{}
	handler.SetConfigPath(configPath)

	logoutRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(logoutRec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil))
	if logoutRec.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want %d; body=%s", logoutRec.Code, http.StatusOK, logoutRec.Body.String())
	}

	providersRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(providersRec, httptest.NewRequest(http.MethodGet, "/api/v1/model-providers", nil))
	if providersRec.Code != http.StatusOK {
		t.Fatalf("providers status = %d, want %d; body=%s", providersRec.Code, http.StatusOK, providersRec.Body.String())
	}
	var got modelProviderTestResponse
	if err := json.NewDecoder(providersRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	if len(got.Providers) == 0 || got.Providers[0].ID != agent.ModelProviderIDOpenCSG {
		t.Fatalf("providers = %+v, want OpenCSG first", got.Providers)
	}
	if len(got.Providers[0].Models) != 0 || got.Providers[0].Status != agent.ModelProviderStatusUnknown {
		t.Fatalf("OpenCSG provider after logout = %+v, want cleared models and unknown status", got.Providers[0])
	}
}

func TestHandleAuthLogoutSucceedsWhenCacheCleanupFails(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("invalid = ["), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	restore := stubAuthLogout(func(*http.Request) (auth.Status, error) {
		return auth.Status{}, nil
	})
	defer restore()
	handler := &Handler{}
	handler.SetConfigPath(configPath)

	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got auth.Status
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Authenticated {
		t.Fatalf("response = %+v, want unauthenticated", got)
	}
}

func writeCachedOpenCSGModelsConfig(t *testing.T, path string) {
	t.Helper()
	content := `[server]
listen_addr = "127.0.0.1:18080"
access_token = "secret"

[models]
default = "opencsg.prod-model"

[models.providers.opencsg]
models = ["prod-model"]
status = "connected"
message = "connected"
last_checked_at = "2026-07-16T09:00:00Z"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
}

func readOpenCSGProvider(t *testing.T, handler *Handler) agent.ModelProviderSummary {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/model-providers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("providers status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got agent.ModelProviderCatalog
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	provider := findProviderSummary(got, agent.ModelProviderIDOpenCSG)
	if provider.ID != agent.ModelProviderIDOpenCSG {
		t.Fatalf("providers = %+v, want OpenCSG", got.Providers)
	}
	return provider
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

func stubAuthCallback(fn func(*http.Request, string) (string, error)) func() {
	previous := appAuthCallback
	appAuthCallback = fn
	return func() { appAuthCallback = previous }
}

func stubAuthLogout(fn func(*http.Request) (auth.Status, error)) func() {
	previous := appAuthLogout
	appAuthLogout = fn
	return func() { appAuthLogout = previous }
}

func stubModelProviderCheck(fn agent.ModelProviderCheckFunc) func() {
	previous := appCheckModelProvider
	appCheckModelProvider = fn
	return func() { appCheckModelProvider = previous }
}
