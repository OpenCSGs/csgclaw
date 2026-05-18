package notifier

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerTokenFromRequestUsesAuthorizationHeaderOnly(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("POST", "/api/v1/notify/u-x?token=query-secret", nil)
	req.Header.Set("Authorization", "Bearer header-secret")
	if got := BearerTokenFromRequest(req); got != "header-secret" {
		t.Fatalf("BearerTokenFromRequest = %q, want header-secret", got)
	}
	req2 := httptest.NewRequest("POST", "/api/v1/notify/u-x?token=query-only", nil)
	if got := BearerTokenFromRequest(req2); got != "" {
		t.Fatalf("query token must be ignored, got %q", got)
	}
}

func TestServeNotifyHTTPRejectsNonPost(t *testing.T) {
	t.Parallel()
	deps := WebhookHTTPDeps{
		Reload: func() error { return nil },
		LookupNotifierAgent: func(string) (map[string]any, string, string, string, bool) {
			t.Fatal("lookup must not run when method is not POST")
			return nil, "", "", "", false
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, NotifyHTTPPathPrefix+"u-alice", nil)
	ServeNotifyHTTP(rec, req, deps)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestServeNotifyHTTPEmptyAgentID(t *testing.T) {
	t.Parallel()
	deps := WebhookHTTPDeps{
		Reload: func() error { return nil },
		LookupNotifierAgent: func(string) (map[string]any, string, string, string, bool) {
			t.Fatal("lookup must not run for empty agent id")
			return nil, "", "", "", false
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, NotifyHTTPPathPrefix, nil)
	ServeNotifyHTTP(rec, req, deps)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
