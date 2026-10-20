package server

import (
	"context"
	"csgclaw/internal/apps"
	"encoding/json"
	"errors"
	"fmt"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"csgclaw/internal/api"
	"csgclaw/internal/im"
	"csgclaw/internal/mcp"
)

func TestRunStopsActiveEventStreams(t *testing.T) {
	for _, tt := range []struct {
		name          string
		desktop       bool
		closeListener bool
	}{
		{name: "context cancellation"},
		{name: "listener failure", closeListener: true},
		{name: "desktop listeners", desktop: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			const accessToken = "shutdown-test-token"
			opts := Options{
				Listener:    listener,
				Context:     ctx,
				IMBus:       im.NewBus(),
				AccessToken: accessToken,
			}
			addresses := []string{listener.Addr().String()}
			hookStarted := make(chan struct{})
			releaseHook := make(chan struct{})
			defer close(releaseHook)
			if tt.desktop {
				sandboxListener, err := net.Listen("tcp4", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer sandboxListener.Close()
				opts.SandboxListener = sandboxListener
				opts.Desktop = &DesktopOptions{
					BaseURL:           "http://" + listener.Addr().String(),
					SessionToken:      "sssssssssssssssssssssssssssssssssssssssssss",
					ServerAccessToken: accessToken,
					ServerAccessHosts: []string{sandboxListener.Addr().String()},
				}
				addresses = append(addresses, sandboxListener.Addr().String())
				opts.BeforeShutdown = func(ctx context.Context) error {
					close(hookStarted)
					select {
					case <-releaseHook:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}
			runErr := make(chan error, 1)
			go func() { runErr <- Run(opts) }()
			stopped := false
			t.Cleanup(func() {
				cancel()
				if !stopped {
					select {
					case <-runErr:
					case <-time.After(6 * time.Second):
						t.Error("server did not stop during cleanup")
					}
				}
			})

			client := &http.Client{Timeout: 10 * time.Second}
			streamsDone := make(chan error, len(addresses))
			for _, address := range addresses {
				req, err := http.NewRequest(http.MethodGet, "http://"+address+"/api/v1/events", nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Authorization", "Bearer "+accessToken)
				response, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
					t.Fatalf("event stream response = %s, %q", response.Status, response.Header.Get("Content-Type"))
				}
				go func() {
					_, err := io.Copy(io.Discard, response.Body)
					streamsDone <- err
				}()
			}

			if tt.closeListener {
				if err := listener.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			if tt.desktop {
				select {
				case <-hookStarted:
				case <-time.After(time.Second):
					t.Fatal("shutdown hook did not start")
				}
				select {
				case <-streamsDone:
					t.Fatal("event stream closed before shutdown hook completed")
				case <-time.After(50 * time.Millisecond):
				}
				releaseHook <- struct{}{}
			}
			select {
			case err := <-runErr:
				stopped = true
				if tt.closeListener && !errors.Is(err, net.ErrClosed) {
					t.Fatalf("Run() error = %v, want closed listener", err)
				}
				if !tt.closeListener && err != nil {
					t.Fatalf("Run() error = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("server shutdown waited for open event streams")
			}
			for range addresses {
				select {
				case err := <-streamsDone:
					if err != nil {
						t.Fatalf("event stream did not close cleanly: %v", err)
					}
				case <-time.After(time.Second):
					t.Fatal("event stream remained open after server shutdown")
				}
			}
		})
	}
}

func TestRunDrainsOrdinaryRequestsDuringShutdown(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	started := make(chan struct{})
	shutdownStarted := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	runErr := make(chan error, 1)
	go func() {
		runErr <- Run(Options{
			Listener: listener,
			Context:  ctx,
			OnReady: func(_ *api.Handler, router chi.Router) {
				router.Get("/inflight", func(w http.ResponseWriter, r *http.Request) {
					close(started)
					select {
					case <-release:
						w.WriteHeader(http.StatusNoContent)
					case <-r.Context().Done():
						w.WriteHeader(http.StatusServiceUnavailable)
					}
				})
				close(ready)
			},
			BeforeShutdown: func(context.Context) error {
				close(shutdownStarted)
				return nil
			},
		})
	}()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not become ready")
	}
	requestResult := make(chan int, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		response, err := client.Get("http://" + listener.Addr().String() + "/inflight")
		if err != nil {
			requestResult <- 0
			return
		}
		defer response.Body.Close()
		requestResult <- response.StatusCode
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	<-shutdownStarted
	select {
	case status := <-requestResult:
		t.Fatalf("in-flight request terminated before completing: status %d", status)
	case <-runErr:
		t.Fatal("server returned before its in-flight request completed")
	case <-time.After(100 * time.Millisecond):
	}
	release <- struct{}{}
	if status := <-requestResult; status != http.StatusNoContent {
		t.Fatalf("in-flight request status = %d, want 204", status)
	}
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not finish shutdown after request completed")
	}
}

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
			OnReady: func(_ *api.Handler, router chi.Router) {
				router.Get("/shutdown-probe", func(w http.ResponseWriter, r *http.Request) {
					if r.Context().Err() != nil {
						http.Error(w, "request canceled before shutdown hook completed", http.StatusServiceUnavailable)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				})
				close(ready)
			},
			BeforeShutdown: func(ctx context.Context) error {
				close(beforeShutdown)
				for _, address := range []string{rendererHost, sandboxHost} {
					req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+"/shutdown-probe", nil)
					if err != nil {
						return err
					}
					req.Header.Set("Authorization", "Bearer "+serverToken)
					response, err := http.DefaultClient.Do(req)
					if err != nil {
						return err
					}
					_ = response.Body.Close()
					if response.StatusCode != http.StatusNoContent {
						return errors.New("HTTP requests unavailable during shutdown hook")
					}
				}
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

func TestRunCompletesConnectorMigrationBeforeOnReady(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "apps.json")
	if err := os.WriteFile(statePath, []byte(`{"feishu_cli_migration":{"v1":{"status":"pending","global_ready":true,"workers":{}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- Run(Options{ListenAddr: "127.0.0.1:0", Context: ctx, AppsStatePath: statePath, OnReady: func(_ *api.Handler, _ chi.Router) {
			data, err := os.ReadFile(statePath)
			if err != nil {
				ready <- err.Error()
				return
			}
			var state struct {
				Migration map[string]struct {
					Status string `json:"status"`
				} `json:"feishu_cli_migration"`
			}
			if err := json.Unmarshal(data, &state); err != nil {
				ready <- err.Error()
				return
			}
			ready <- state.Migration["v1"].Status
		}})
	}()
	select {
	case status := <-ready:
		if status != "completed" {
			t.Errorf("migration status at Agent restore=%q", status)
		}
	case err := <-done:
		t.Fatalf("server stopped before OnReady: %v", err)
	case <-time.After(5 * time.Second):
		t.Error("OnReady did not complete")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop")
	}
}

func TestRunRestoresConnectorsBeforeOnReady(t *testing.T) {
	for _, cancelRestore := range []bool{false, true} {
		t.Run(fmt.Sprint("cancel=", cancelRestore), func(t *testing.T) {
			var restoring atomic.Bool
			entered := make(chan struct{}, 1)
			release := make(chan struct{})
			server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "fixture", Version: "1"}, nil)
			handler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return server }, &sdkmcp.StreamableHTTPOptions{JSONResponse: true})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if restoring.Load() {
					select {
					case entered <- struct{}{}:
					default:
					}
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				}
				handler.ServeHTTP(w, r)
			}))
			defer upstream.Close()
			defer close(release)
			statePath := filepath.Join(t.TempDir(), "apps.json")
			service, err := apps.NewService(statePath, apps.Options{})
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Create(context.Background(), "worker", apps.CreateRequest{AppID: "gitlab", Name: "GitLab", Config: apps.Config{URL: upstream.URL, AuthMode: "none"}, Connect: true})
			if err != nil {
				service.Close()
				t.Fatal(err)
			}
			if err := service.Close(); err != nil {
				t.Fatal(err)
			}
			restoring.Store(true)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			listening := make(chan struct{})
			ready := make(chan struct{}, 1)
			done := make(chan error, 1)
			go func() {
				done <- Run(Options{
					Listener:      listener,
					Context:       ctx,
					AppsStatePath: statePath,
					OnListening:   func() { close(listening) },
					OnReady:       func(*api.Handler, chi.Router) { ready <- struct{}{} },
				})
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("connector restoration did not begin")
			}
			select {
			case <-listening:
			case <-time.After(time.Second):
				t.Fatal("HTTP readiness waited for connector restoration")
			}
			client := &http.Client{Timeout: time.Second}
			response, err := client.Get("http://" + listener.Addr().String() + "/healthz")
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("HTTP status during connector restoration = %d, want 200", response.StatusCode)
			}
			select {
			case <-ready:
				t.Fatal("Worker startup ran before connector restoration")
			case <-time.After(50 * time.Millisecond):
			}
			if !cancelRestore {
				restoring.Store(false)
				release <- struct{}{}
				select {
				case <-ready:
				case <-time.After(5 * time.Second):
					t.Fatal("Worker startup did not run after restoration")
				}
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("server did not stop")
			}
			if cancelRestore {
				select {
				case <-ready:
					t.Fatal("Worker startup ran after cancellation")
				default:
				}
			}
		})
	}
}

func TestRunDoesNotReportListeningAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	listening := make(chan struct{}, 1)
	if err := Run(Options{
		ListenAddr:  "127.0.0.1:0",
		Context:     ctx,
		OnListening: func() { listening <- struct{}{} },
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-listening:
		t.Fatal("HTTP readiness was reported after cancellation")
	default:
	}
}
