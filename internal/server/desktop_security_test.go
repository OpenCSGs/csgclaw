package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/opencsgmcp"
)

func TestDesktopSecurityHeadersAllowOnlyHashedInlineBootstrap(t *testing.T) {
	header := make(http.Header)
	setDesktopSecurityHeaders(header)
	csp := header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self' "+documentBootstrapCSPHash) {
		t.Fatalf("Content-Security-Policy = %q, want bootstrap script hash", csp)
	}
	if strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("Content-Security-Policy = %q, unsafe-inline must remain disabled", csp)
	}
	if !strings.Contains(csp, "frame-src 'self' blob:") {
		t.Fatalf("Content-Security-Policy = %q, want same-origin and blob attachment previews", csp)
	}
	if !strings.Contains(csp, "object-src 'none'") {
		t.Fatalf("Content-Security-Policy = %q, object embedding must remain disabled", csp)
	}
}

func TestDesktopSecuritySeparatesRendererAndSandboxAuthentication(t *testing.T) {
	const (
		rendererHost = "127.0.0.1:59842"
		sandboxHost  = "host.docker.internal:59843"
		sessionToken = "sssssssssssssssssssssssssssssssssssssssssss"
		serverToken  = "server-access-token"
	)

	opts := DesktopOptions{
		BaseURL:           "http://" + rendererHost,
		SessionToken:      sessionToken,
		ServerAccessToken: serverToken,
		ServerAccessHosts: []string{"127.0.0.1:59843", sandboxHost},
	}
	rendererHandler, err := desktopRendererSecurityHandler(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 59842},
		opts,
	)
	if err != nil {
		t.Fatalf("desktopRendererSecurityHandler() error = %v", err)
	}
	sandboxHandler, err := desktopSandboxSecurityHandler(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		&net.TCPAddr{IP: net.IPv4zero, Port: 59843},
		opts,
	)
	if err != nil {
		t.Fatalf("desktopSandboxSecurityHandler() error = %v", err)
	}

	tests := []struct {
		name          string
		sandbox       bool
		host          string
		path          string
		method        string
		authorization string
		origin        string
		wantStatus    int
	}{
		{
			name:          "sandbox accepts scoped file bridge token for matching route",
			sandbox:       true,
			host:          sandboxHost,
			path:          "/api/v1/agents/agent-a/mcp-file-bridge/file-parser",
			method:        http.MethodPost,
			authorization: "Bearer " + opencsgmcp.FileBridgeToken(serverToken, "agent-a", "file-parser"),
			wantStatus:    http.StatusNoContent,
		},
		{
			name:          "sandbox rejects scoped file bridge token for another agent",
			sandbox:       true,
			host:          sandboxHost,
			path:          "/api/v1/agents/agent-b/mcp-file-bridge/file-parser",
			method:        http.MethodPost,
			authorization: "Bearer " + opencsgmcp.FileBridgeToken(serverToken, "agent-a", "file-parser"),
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "sandbox rejects scoped file bridge token for another server",
			sandbox:       true,
			host:          sandboxHost,
			path:          "/api/v1/agents/agent-a/mcp-file-bridge/other-parser",
			method:        http.MethodPost,
			authorization: "Bearer " + opencsgmcp.FileBridgeToken(serverToken, "agent-a", "file-parser"),
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "sandbox rejects scoped file bridge token for non-post request",
			sandbox:       true,
			host:          sandboxHost,
			path:          "/api/v1/agents/agent-a/mcp-file-bridge/file-parser",
			authorization: "Bearer " + opencsgmcp.FileBridgeToken(serverToken, "agent-a", "file-parser"),
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "sandbox rejects scoped file bridge token for ordinary API",
			sandbox:       true,
			host:          sandboxHost,
			path:          "/api/v1/messages",
			method:        http.MethodPost,
			authorization: "Bearer " + opencsgmcp.FileBridgeToken(serverToken, "agent-a", "file-parser"),
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:       "renderer accepts external loopback browser",
			host:       rendererHost,
			path:       "/api/v1/messages",
			origin:     "http://" + rendererHost,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "renderer health remains unauthenticated",
			host:       rendererHost,
			path:       "/healthz",
			wantStatus: http.StatusNoContent,
		},
		{
			name:          "renderer accepts session token",
			host:          rendererHost,
			path:          "/api/v1/messages",
			authorization: "Bearer " + sessionToken,
			origin:        "http://" + rendererHost,
			wantStatus:    http.StatusNoContent,
		},
		{
			name:          "sandbox accepts server token without browser origin",
			sandbox:       true,
			host:          sandboxHost,
			path:          "/api/v1/channels/csgclaw/participants/pt-worker/events",
			authorization: "Bearer " + serverToken,
			wantStatus:    http.StatusNoContent,
		},
		{
			name:          "sandbox rejects renderer session token",
			sandbox:       true,
			host:          sandboxHost,
			path:          "/api/v1/messages",
			authorization: "Bearer " + sessionToken,
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:       "sandbox health still requires server token",
			sandbox:    true,
			host:       sandboxHost,
			path:       "/healthz",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "sandbox rejects browser origin",
			sandbox:       true,
			host:          sandboxHost,
			path:          "/api/v1/messages",
			authorization: "Bearer " + serverToken,
			origin:        "http://" + sandboxHost,
			wantStatus:    http.StatusForbidden,
		},
		{
			name:          "server token cannot authorize an unknown host",
			sandbox:       true,
			host:          "attacker.invalid:59842",
			path:          "/api/v1/messages",
			authorization: "Bearer " + serverToken,
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "renderer still rejects a foreign origin",
			host:          rendererHost,
			path:          "/api/v1/messages",
			authorization: "Bearer " + sessionToken,
			origin:        "https://attacker.invalid",
			wantStatus:    http.StatusForbidden,
		},
		{
			name:          "sandbox listener rejects renderer host",
			sandbox:       true,
			host:          rendererHost,
			path:          "/api/v1/messages",
			authorization: "Bearer " + sessionToken,
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "renderer listener rejects sandbox host",
			host:          sandboxHost,
			path:          "/api/v1/messages",
			authorization: "Bearer " + serverToken,
			wantStatus:    http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, "http://"+tt.host+tt.path, nil)
			req.Host = tt.host
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			recorder := httptest.NewRecorder()

			requestHandler := rendererHandler
			if tt.sandbox {
				requestHandler = sandboxHandler
			}
			requestHandler.ServeHTTP(recorder, req)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %q", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
		})
	}
}

func TestDesktopSecurityRejectsInvalidSandboxHosts(t *testing.T) {
	const sessionToken = "sssssssssssssssssssssssssssssssssssssssssss"
	for _, host := range []string{
		"host.docker.internal:12345",
		"host.docker.internal:59843/path",
		"user@host.docker.internal:59843",
	} {
		t.Run(strings.ReplaceAll(host, "/", "_"), func(t *testing.T) {
			_, err := desktopSandboxSecurityHandler(
				http.NotFoundHandler(),
				&net.TCPAddr{IP: net.IPv4zero, Port: 59843},
				DesktopOptions{
					BaseURL:           "http://127.0.0.1:59842",
					SessionToken:      sessionToken,
					ServerAccessToken: "server-access-token",
					ServerAccessHosts: []string{host},
				},
			)
			if err == nil {
				t.Fatalf("desktopSecurityHandler() error = nil for host %q", host)
			}
		})
	}
}

func TestDesktopSandboxAuthenticatesScopedAgentsWithoutRelaxingOrigin(t *testing.T) {
	calls := 0
	handler, err := desktopSandboxSecurityHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") == "Bearer signed-agent" && r.URL.Path == "/api/v1/admin" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}), &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 18081}, DesktopOptions{
		ServerAccessToken: "admin-token", ServerAccessHosts: []string{"127.0.0.1:18081"},
		ValidateAgentAccessToken: func(token string) bool { return token == "signed-agent" },
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, token, host, origin, path string
		status                          int
		downstream                      bool
	}{
		{"scoped Agent", "signed-agent", "127.0.0.1:18081", "", "/api/v1/agents/agent-alice/mcp", 204, true},
		{"administrator", "admin-token", "127.0.0.1:18081", "", "/api/v1/admin", 204, true},
		{"invalid Agent", "forged-agent", "127.0.0.1:18081", "", "/api/v1/agents/agent-alice/mcp", 401, false},
		{"missing credential", "", "127.0.0.1:18081", "", "/api/v1/agents/agent-alice/mcp", 401, false},
		{"foreign origin", "signed-agent", "127.0.0.1:18081", "https://evil.example", "/api/v1/agents/agent-alice/mcp", 403, false},
		{"foreign host", "signed-agent", "evil.example:18081", "", "/api/v1/agents/agent-alice/mcp", 400, false},
		{"downstream scope preserved", "signed-agent", "127.0.0.1:18081", "", "/api/v1/admin", 403, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := calls
			req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:18081"+test.path, nil)
			req.Host = test.host
			if test.token != "" {
				req.Header.Set("Authorization", "Bearer "+test.token)
			}
			if test.origin != "" {
				req.Header.Set("Origin", test.origin)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != test.status || (calls > before) != test.downstream {
				t.Fatalf("status=%d reached downstream=%v", rec.Code, calls > before)
			}
		})
	}
}
