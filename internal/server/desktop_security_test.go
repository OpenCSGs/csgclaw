package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
		authorization string
		origin        string
		wantStatus    int
	}{
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
			req := httptest.NewRequest(http.MethodGet, "http://"+tt.host+tt.path, nil)
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
