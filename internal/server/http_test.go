package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"csgclaw/internal/api"
	"csgclaw/internal/mcp"
)

func TestNewHandlerWiresMCPService(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	handler := newHandler(Options{MCP: mcp.NewService()})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mcp-servers", nil)
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRunDesktopServesRendererAndSandboxOnSeparateListeners(t *testing.T) {
	rendererListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen renderer: %v", err)
	}
	sandboxListener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		_ = rendererListener.Close()
		t.Fatalf("listen sandbox: %v", err)
	}

	const (
		sessionToken = "sssssssssssssssssssssssssssssssssssssssssss"
		serverToken  = "server-access-token"
	)
	rendererHost := rendererListener.Addr().String()
	_, sandboxPort, err := net.SplitHostPort(sandboxListener.Addr().String())
	if err != nil {
		t.Fatalf("split sandbox address: %v", err)
	}
	sandboxHost := net.JoinHostPort("127.0.0.1", sandboxPort)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	beforeShutdown := make(chan struct{})
	runErr := make(chan error, 1)
	go func() {
		runErr <- Run(Options{
			ListenAddr:      sandboxListener.Addr().String(),
			Listener:        rendererListener,
			SandboxListener: sandboxListener,
			AccessToken:     serverToken,
			Desktop: &DesktopOptions{
				BaseURL:           "http://" + rendererHost,
				SessionToken:      sessionToken,
				ServerAccessToken: serverToken,
				ServerAccessHosts: []string{sandboxHost},
			},
			Context: ctx,
			OnReady: func(_ *api.Handler, _ chi.Router) {
				close(ready)
			},
			BeforeShutdown: func(context.Context) error {
				close(beforeShutdown)
				return nil
			},
		})
	}()

	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("desktop server did not become ready")
	}

	tests := []struct {
		name          string
		url           string
		host          string
		authorization string
		wantStatus    int
	}{
		{
			name:       "renderer UI accepts external browser",
			url:        "http://" + rendererHost + "/",
			wantStatus: http.StatusOK,
		},
		{
			name:       "renderer health is loopback accessible",
			url:        "http://" + rendererHost + "/healthz",
			wantStatus: http.StatusOK,
		},
		{
			name:       "sandbox health requires server token",
			url:        "http://" + sandboxHost + "/healthz",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "sandbox health accepts server token",
			url:           "http://" + sandboxHost + "/healthz",
			authorization: "Bearer " + serverToken,
			wantStatus:    http.StatusOK,
		},
		{
			name:          "sandbox listener rejects renderer host spoofing",
			url:           "http://" + sandboxHost + "/healthz",
			host:          rendererHost,
			authorization: "Bearer " + sessionToken,
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "renderer listener rejects sandbox host spoofing",
			url:           "http://" + rendererHost + "/healthz",
			host:          sandboxHost,
			authorization: "Bearer " + serverToken,
			wantStatus:    http.StatusBadRequest,
		},
	}

	client := &http.Client{Timeout: 5 * time.Second}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, tt.url, nil)
			if err != nil {
				t.Fatalf("NewRequestWithContext() error = %v", err)
			}
			if tt.host != "" {
				req.Host = tt.host
			}
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("client.Do() error = %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("desktop server did not stop")
	}
	select {
	case <-beforeShutdown:
	default:
		t.Fatal("BeforeShutdown was not called")
	}
}
