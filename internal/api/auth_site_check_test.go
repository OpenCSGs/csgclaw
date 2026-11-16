package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestAuthSiteCheckProbesLocalLoginWithoutFollowingRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tenant/sso/login" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected probe request: %s", r.URL)
		}
		http.Redirect(w, r, "/do-not-follow", http.StatusFound)
	}))
	t.Cleanup(target.Close)

	for _, tc := range []struct {
		name, token string
		h           *Handler
		status      int
	}{
		{"authenticated local management probe succeeds", "Bearer test-session", &Handler{desktopSessionToken: "test-session"}, http.StatusOK},
		{"no-auth local management probe succeeds", "", &Handler{serverNoAuth: true}, http.StatusOK},
		{"anonymous private probe is denied when server authentication is enabled", "", &Handler{desktopSessionToken: "test-session"}, http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/auth/site-check", strings.NewReader(
				`{"base_url":"`+target.URL+`/tenant/?discard=1"}`,
			))
			r.RemoteAddr = "127.0.0.1:12345"
			r.Header.Set("Authorization", tc.token)
			w := httptest.NewRecorder()
			tc.h.handleAuthSiteCheck(w, r)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d", w.Code, tc.status)
			}
		})
	}
}

func TestProbeAuthSiteRejectsMissingLoginEndpoint(t *testing.T) {
	target := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(target.Close)
	if _, err := probeAuthSite(context.Background(), target.Client(), target.URL); err == nil {
		t.Fatal("404 should not report a reachable login endpoint")
	}
}

func TestAuthSiteProbeNeverAllowsMetadataAddresses(t *testing.T) {
	for _, address := range []string{"169.254.169.254", "100.100.100.200", "fe80::1", "64:ff9b::a9fe:a9fe"} {
		if authSiteAddressAllowed(netip.MustParseAddr(address), true) {
			t.Errorf("allowed special-use address %s", address)
		}
	}
}
