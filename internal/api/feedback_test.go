package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"csgclaw/internal/auth"
)

func TestFeedbackUsesAuthenticatedSiteAndServerVersion(t *testing.T) {
	var called bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/api/v1/csgbot/user-feedback" {
			t.Error(r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing user token")
		}
		var payload struct {
			CSGClaw struct {
				SiteURL string `json:"site_url"`
				Version string `json:"version"`
			} `json:"csgclaw"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.CSGClaw.SiteURL == "https://attacker.example" || payload.CSGClaw.Version == "fake" {
			t.Error("trusted caller metadata")
		}
		io.WriteString(w, `{"success":true,"issue_id":5}`)
	}))
	defer upstream.Close()
	previousStore := auth.Default().Store
	t.Cleanup(func() { auth.Default().Store = previousStore })
	auth.Default().Store = auth.NewStore(filepath.Join(t.TempDir(), "auth.json"))
	if err := auth.Default().Store.Save(auth.Record{Tokens: auth.Tokens{AccessToken: "test-token"}, Account: auth.Account{BaseURL: upstream.URL}}); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"problem_description": "Test", "site_url": "https://attacker.example", "version": "fake"} {
		writer.WriteField(key, value)
	}
	writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	request.Header.Set("Authorization", "Bearer server-token")
	(&Handler{serverAccessToken: "server-token"}).Routes().ServeHTTP(recorder, request)
	if recorder.Code != 200 || !called {
		t.Fatalf("status=%d body=%s called=%v", recorder.Code, recorder.Body, called)
	}
}

func TestFeedbackRequiresLogin(t *testing.T) {
	previousStore := auth.Default().Store
	t.Cleanup(func() { auth.Default().Store = previousStore })
	auth.Default().Store = auth.NewStore(filepath.Join(t.TempDir(), "auth.json"))
	for _, browser := range []bool{false, true} {
		name := "server credential"
		if browser {
			name = "local browser"
		}
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "http://localhost:18080/api/v1/feedback", nil)
			request.RemoteAddr = "127.0.0.1:23456"
			if browser {
				request.Header.Set("Origin", "http://localhost:18080")
			} else {
				request.Header.Set("Authorization", "Bearer server-token")
			}
			(&Handler{serverAccessToken: "server-token"}).Routes().ServeHTTP(response, request)
			assertFeedbackAccessError(t, response, "feedback_login_required")
		})
	}
}

func TestFeedbackValidatesServerCredential(t *testing.T) {
	previousStore := auth.Default().Store
	t.Cleanup(func() { auth.Default().Store = previousStore })
	auth.Default().Store = auth.NewStore(filepath.Join(t.TempDir(), "auth.json"))
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer account-token" {
			t.Error("upstream must receive the account credential")
		}
		io.WriteString(w, `{"success":true,"issue_id":5}`)
	}))
	defer upstream.Close()
	if err := auth.Default().Store.Save(auth.Record{Tokens: auth.Tokens{AccessToken: "account-token"}, Account: auth.Account{BaseURL: upstream.URL}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name          string
		authorization string
		origin        string
		noAuth        bool
		want          int
	}{
		{name: "missing credential", want: http.StatusUnauthorized},
		{name: "invalid credential", authorization: "Bearer wrong-token", want: http.StatusUnauthorized},
		{name: "account credential is not server credential", authorization: "Bearer account-token", want: http.StatusUnauthorized},
		{name: "cross origin without credential", origin: "https://untrusted.example", want: http.StatusUnauthorized},
		{name: "server credential", authorization: "Bearer server-token", want: http.StatusOK},
		{name: "desktop credential", authorization: "Bearer desktop-token", want: http.StatusOK},
		{name: "authentication disabled", noAuth: true, want: http.StatusOK},
		{name: "authentication disabled with explicit credential", noAuth: true, authorization: "Bearer ignored-token", want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			if err := writer.WriteField("problem_description", "Test"); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", &body)
			request.Header.Set("Content-Type", writer.FormDataContentType())
			request.Header.Set("Authorization", tc.authorization)
			request.Header.Set("Origin", tc.origin)
			response := httptest.NewRecorder()
			before := calls
			handler := &Handler{serverAccessToken: "server-token", desktopSessionToken: "desktop-token", serverNoAuth: tc.noAuth}
			handler.Routes().ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			if tc.want == http.StatusUnauthorized {
				assertFeedbackAccessError(t, response, "feedback_access_denied")
			}
			wantCalls := before
			if tc.want == http.StatusOK {
				wantCalls++
			}
			if calls != wantCalls {
				t.Fatalf("upstream calls=%d, want %d", calls, wantCalls)
			}
		})
	}
}

func TestFeedbackLocalBrowserAccess(t *testing.T) {
	previousStore := auth.Default().Store
	t.Cleanup(func() { auth.Default().Store = previousStore })
	auth.Default().Store = auth.NewStore(filepath.Join(t.TempDir(), "auth.json"))
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer account-token" {
			t.Error("upstream must receive the account credential")
		}
		io.WriteString(w, `{"success":true,"issue_id":5}`)
	}))
	defer upstream.Close()
	if err := auth.Default().Store.Save(auth.Record{Tokens: auth.Tokens{AccessToken: "account-token"}, Account: auth.Account{BaseURL: upstream.URL}}); err != nil {
		t.Fatal(err)
	}
	const localOrigin = "http://localhost:18080"
	for _, tc := range []struct {
		name       string
		target     string
		host       string
		remoteAddr string
		origins    []string
		headers    http.Header
		agentID    string
		allowed    bool
	}{
		{name: "localhost", allowed: true},
		{name: "IPv4", target: "http://127.0.0.1:18080/api/v1/feedback", origins: []string{"http://127.0.0.1:18080"}, allowed: true},
		{name: "IPv6", target: "http://[::1]:18080/api/v1/feedback", remoteAddr: "[::1]:23456", origins: []string{"http://[::1]:18080"}, allowed: true},
		{name: "default HTTP port", target: "http://localhost/api/v1/feedback", origins: []string{"http://localhost"}, allowed: true},
		{name: "IPv6 default port", target: "http://[::1]/api/v1/feedback", remoteAddr: "[::1]:23456", origins: []string{"http://[::1]"}, allowed: true},
		{name: "HTTPS", target: "https://localhost:18443/api/v1/feedback", origins: []string{"https://localhost:18443"}, allowed: true},
		{name: "cross site", origins: []string{"http://untrusted.example:18080"}},
		{name: "cross port", origins: []string{"http://localhost:18081"}},
		{name: "cross scheme", origins: []string{"https://localhost:18080"}},
		{name: "HTTP origin on TLS connection", target: "https://localhost:18080/api/v1/feedback"},
		{name: "null origin", origins: []string{"null"}},
		{name: "missing origin", origins: []string{}},
		{name: "empty origin", origins: []string{""}},
		{name: "multiple origins", origins: []string{localOrigin, localOrigin}},
		{name: "origin list", origins: []string{localOrigin + " " + localOrigin}},
		{name: "origin user information", origins: []string{"http://user@localhost:18080"}},
		{name: "origin path", origins: []string{localOrigin + "/"}},
		{name: "origin query", origins: []string{localOrigin + "?ignored=true"}},
		{name: "origin empty query", origins: []string{localOrigin + "?"}},
		{name: "origin fragment", origins: []string{localOrigin + "#ignored"}},
		{name: "origin empty fragment", origins: []string{localOrigin + "#"}},
		{name: "invalid origin port", host: "localhost:invalid", origins: []string{"http://localhost:invalid"}},
		{name: "empty origin port", host: "localhost:", origins: []string{"http://localhost:"}},
		{name: "out of range origin port", host: "localhost:65536", origins: []string{"http://localhost:65536"}},
		{name: "unbracketed IPv6", host: "::1:18080", origins: []string{"http://::1:18080"}},
		{name: "remote peer", remoteAddr: "192.0.2.1:23456"},
		{name: "invalid peer", remoteAddr: "127.0.0.1"},
		{name: "nonloopback host", target: "http://untrusted.example:18080/api/v1/feedback", origins: []string{"http://untrusted.example:18080"}},
		{name: "localhost suffix", target: "http://localhost.untrusted.example:18080/api/v1/feedback", origins: []string{"http://localhost.untrusted.example:18080"}},
		{name: "invalid credential", headers: http.Header{"Authorization": {"Bearer wrong-token"}}},
		{name: "account credential", headers: http.Header{"Authorization": {"Bearer account-token"}}},
		{name: "Agent credential", headers: http.Header{"Authorization": {"Bearer agent.test-token"}}},
		{name: "empty explicit credential", headers: http.Header{"Authorization": {""}}},
		{name: "multiple credentials", headers: http.Header{"Authorization": {"", "Bearer wrong-token"}}},
		{name: "Agent caller header", headers: http.Header{"X-CSGClaw-Caller-Agent": {"agent-test"}}},
		{name: "Agent context", agentID: "agent-test"},
		{name: "forwarded local peer", remoteAddr: "192.0.2.1:23456", headers: http.Header{"X-Forwarded-For": {"127.0.0.1"}, "Forwarded": {"for=127.0.0.1;host=localhost:18080;proto=http"}}},
		{name: "forwarded local host", target: "http://untrusted.example:18080/api/v1/feedback", headers: http.Header{"X-Forwarded-Host": {"localhost:18080"}}},
		{name: "forwarded HTTPS", origins: []string{"https://localhost:18080"}, headers: http.Header{"X-Forwarded-Proto": {"https"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			if err := writer.WriteField("problem_description", "Browser feedback test"); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			target := tc.target
			if target == "" {
				target = localOrigin + "/api/v1/feedback"
			}
			request := httptest.NewRequest(http.MethodPost, target, &body)
			request.RemoteAddr = "127.0.0.1:23456"
			if tc.remoteAddr != "" {
				request.RemoteAddr = tc.remoteAddr
			}
			if tc.host != "" {
				request.Host = tc.host
			}
			request.Header.Set("Content-Type", writer.FormDataContentType())
			origins := tc.origins
			if origins == nil {
				origins = []string{localOrigin}
			}
			for _, origin := range origins {
				request.Header.Add("Origin", origin)
			}
			for name, values := range tc.headers {
				for _, value := range values {
					request.Header.Add(name, value)
				}
			}
			if tc.agentID != "" {
				request = request.WithContext(context.WithValue(request.Context(), appAgentContextKey{}, tc.agentID))
			}
			response := httptest.NewRecorder()
			before := calls.Load()
			(&Handler{serverAccessToken: "server-token"}).Routes().ServeHTTP(response, request)
			wantCalls := before
			if tc.allowed {
				if response.Code != http.StatusOK {
					t.Fatalf("status=%d body=%s", response.Code, response.Body)
				}
				wantCalls++
			} else {
				assertFeedbackAccessError(t, response, "feedback_access_denied")
			}
			if got := calls.Load(); got != wantCalls {
				t.Fatalf("upstream calls=%d, want %d", got, wantCalls)
			}
		})
	}
}

func assertFeedbackAccessError(t *testing.T, response *httptest.ResponseRecorder, code string) {
	t.Helper()
	var result struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("invalid error response: %v; body=%s", err, response.Body)
	}
	if response.Code != http.StatusUnauthorized || result.Error.Code != code || result.Error.Message == "" {
		t.Fatalf("status=%d body=%s, want HTTP 401 with code %s", response.Code, response.Body, code)
	}
}
