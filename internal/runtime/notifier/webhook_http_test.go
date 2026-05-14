package notifier

import (
	"net/http/httptest"
	"testing"
)

func TestBearerTokenFromRequestUsesAuthorizationHeaderOnly(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("POST", "/api/v1/agents/u-x/webhooks/notify?token=query-secret", nil)
	req.Header.Set("Authorization", "Bearer header-secret")
	if got := BearerTokenFromRequest(req); got != "header-secret" {
		t.Fatalf("BearerTokenFromRequest = %q, want header-secret", got)
	}
	req2 := httptest.NewRequest("POST", "/api/v1/agents/u-x/webhooks/notify?token=query-only", nil)
	if got := BearerTokenFromRequest(req2); got != "" {
		t.Fatalf("query token must be ignored, got %q", got)
	}
}
