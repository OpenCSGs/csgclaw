package apps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Emulates an HTTP MCP whose idle SSE streams close without notification IDs.
// POST requests remain healthy, as on servers that only expose static tools.
type recoveryUpstream struct {
	server       *httptest.Server
	initializes  atomic.Int32
	gets         atomic.Int32
	calls        atomic.Int32
	dropFirst    atomic.Bool
	dropAll      atomic.Bool
	cut          chan struct{}
	expireCall   atomic.Bool
	expireList   atomic.Bool
	notify       chan struct{}
	reject       atomic.Bool
	unavailable  atomic.Bool
	blockInit    atomic.Bool
	initBlocked  chan struct{}
	initFinished chan struct{}
	releaseInit  chan struct{}
}

func newRecoveryUpstream(t *testing.T) *recoveryUpstream {
	t.Helper()
	u := &recoveryUpstream{cut: make(chan struct{}), notify: make(chan struct{}, 1), initBlocked: make(chan struct{}), initFinished: make(chan struct{}), releaseInit: make(chan struct{})}
	u.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u.unavailable.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if u.reject.Load() {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			u.gets.Add(1)
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, "retry: 1\n: keepalive\n\n")
			w.(http.Flusher).Flush()
			if u.dropAll.Load() || (u.dropFirst.Load() && r.Header.Get("Mcp-Session-Id") == "session-1") {
				return
			}
			var cut <-chan struct{}
			if r.Header.Get("Mcp-Session-Id") == "session-1" {
				cut = u.cut
			}
			for {
				select {
				case <-cut:
					return
				case <-r.Context().Done():
					return
				case <-u.notify:
					_, _ = fmt.Fprint(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/tools/list_changed\"}\n\n")
					w.(http.Flusher).Flush()
				}
			}
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case http.MethodPost:
			var request struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, "invalid request", 400)
				return
			}
			if len(request.ID) == 0 {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			var result any
			switch request.Method {
			case "initialize":
				n := u.initializes.Add(1)
				if n > 1 && u.blockInit.CompareAndSwap(true, false) {
					close(u.initBlocked)
					defer close(u.initFinished)
					select {
					case <-u.releaseInit:
					case <-r.Context().Done():
						return
					}
				}
				w.Header().Set("Mcp-Session-Id", fmt.Sprintf("session-%d", n))
				result = map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{"tools": map[string]any{"listChanged": true}}, "serverInfo": map[string]any{"name": "recoverable", "version": "1"}}
			case "tools/list":
				if u.expireList.CompareAndSwap(true, false) {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				result = map[string]any{"tools": []map[string]any{{"name": "read", "inputSchema": map[string]any{"type": "object"}}}}
			case "tools/call":
				u.calls.Add(1)
				if u.expireCall.CompareAndSwap(true, false) {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				result = map[string]any{"content": []map[string]any{{"type": "text", "text": "ok"}}}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
		}
	}))
	t.Cleanup(u.server.Close)
	return u
}

func waitAppCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for app recovery")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAppRecoversAfterSSEBudgetExhaustion(t *testing.T) {
	u := newRecoveryUpstream(t)
	s := newRecoveryService(t)
	item := addRecoveryApp(t, s, u)
	client := gatewayClient(t, s, "agent")
	// Disconnect the already-open listener only after Connect returned successfully.
	u.dropFirst.Store(true)
	close(u.cut)
	waitAppCondition(t, func() bool { return u.initializes.Load() >= 2 })
	waitAppCondition(t, func() bool {
		current, _ := s.Get(t.Context(), "agent", item.InstallationID)
		return current.Status == "connected"
	})
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: toolName(item.InstallationID, "read"), Arguments: map[string]any{}})
	if err != nil || result.IsError || u.calls.Load() != 1 {
		t.Fatalf("tool unavailable after recovery: %v calls=%d", err, u.calls.Load())
	}
}

func TestExpiredAppSessionRecoversWithoutReplayingToolCall(t *testing.T) {
	u := newRecoveryUpstream(t)
	s := newRecoveryService(t)
	item := addRecoveryApp(t, s, u)
	client := gatewayClient(t, s, "agent")
	u.expireCall.Store(true)
	_, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: toolName(item.InstallationID, "read"), Arguments: map[string]any{}})
	if err == nil {
		t.Fatal("expired-session call unexpectedly succeeded")
	}
	waitAppCondition(t, func() bool {
		current, _ := s.Get(t.Context(), "agent", item.InstallationID)
		return u.initializes.Load() >= 2 && current.Status == "connected"
	})
	if u.calls.Load() != 1 {
		t.Fatal("failed tool call was replayed automatically")
	}
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: toolName(item.InstallationID, "read"), Arguments: map[string]any{}})
	if err != nil || result.IsError || u.calls.Load() != 2 {
		t.Fatalf("subsequent call failed: %v calls=%d", err, u.calls.Load())
	}
}

func beginAppRecovery(t *testing.T, s *Service, u *recoveryUpstream, item Installation) *connectionRecovery {
	t.Helper()
	u.dropFirst.Store(true)
	close(u.cut)
	var recovery *connectionRecovery
	waitAppCondition(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		recovery = s.entries[item.InstallationID].recovery
		return recovery != nil
	})
	return recovery
}

func TestAppRecoveryRespectsLifecycleChanges(t *testing.T) {
	for _, action := range []string{"disconnect", "disable", "disable_resource", "delete", "delete_resource", "close", "manual_connect", "config_change", "invalidate_credentials"} {
		t.Run(action, func(t *testing.T) {
			u := newRecoveryUpstream(t)
			s := newRecoveryService(t)
			item := addRecoveryApp(t, s, u)
			recovery := beginAppRecovery(t, s, u, item)
			var err error
			off := false
			switch action {
			case "disconnect":
				_, err = s.Disconnect(t.Context(), "agent", item.InstallationID)
			case "disable":
				_, err = s.Update(t.Context(), "agent", item.InstallationID, UpdateRequest{Enabled: &off})
			case "disable_resource":
				_, err = s.Update(t.Context(), "", item.ResourceID, UpdateRequest{Enabled: &off})
			case "delete":
				err = s.Delete(t.Context(), "agent", item.InstallationID)
			case "delete_resource":
				err = s.Delete(t.Context(), "", item.ResourceID)
			case "close":
				err = s.Close()
			case "manual_connect":
				_, err = s.Connect(t.Context(), "agent", item.InstallationID)
			case "config_change":
				config := item.Config
				config.ToolTimeoutSec++
				_, err = s.Update(t.Context(), "", item.ResourceID, UpdateRequest{Config: &config})
			case "invalidate_credentials":
				s.InvalidateConnector("gitlab")
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-recovery.ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("old recovery was not canceled")
			}
			before := u.initializes.Load()
			_, err = s.connectWithRecovery(context.Background(), "agent", item.InstallationID, false, recovery)
			if !errors.Is(err, context.Canceled) && !errors.Is(err, ErrNotFound) {
				t.Fatalf("stale recovery was admitted: %v", err)
			}
			if u.initializes.Load() != before {
				t.Fatal("stale recovery replaced a newer connection")
			}
		})
	}
}

func TestAppRecoveryRetriesTransientFailureButStopsForAuthentication(t *testing.T) {
	for _, auth := range []bool{false, true} {
		t.Run(fmt.Sprint("auth=", auth), func(t *testing.T) {
			u := newRecoveryUpstream(t)
			s := newRecoveryService(t)
			item := addRecoveryApp(t, s, u)
			recovery := beginAppRecovery(t, s, u, item)
			if auth {
				u.reject.Store(true)
			} else {
				u.unavailable.Store(true)
			}
			waitAppCondition(t, func() bool {
				current, _ := s.Get(t.Context(), "agent", item.InstallationID)
				return current.LastErrorCode == "app_platform_unauthorized" || current.LastErrorCode == "app_mcp_http_error"
			})
			if auth {
				select {
				case <-recovery.ctx.Done():
				case <-time.After(time.Second):
					t.Fatal("authentication failure kept retrying")
				}
				current, _ := s.Get(t.Context(), "agent", item.InstallationID)
				if current.Status != "authorization_required" {
					t.Fatal(current.Status)
				}
			} else {
				u.unavailable.Store(false)
				waitAppCondition(t, func() bool {
					current, _ := s.Get(t.Context(), "agent", item.InstallationID)
					return current.Status == "connected"
				})
			}
		})
	}
}

func newRecoveryService(t *testing.T) *Service {
	t.Helper()
	return newTestService(t, Options{ResolveConnectorHTTP: func(_ context.Context, _ string, _ string, c Config) (ConnectorHTTPConfig, error) {
		return ConnectorHTTPConfig{Endpoint: c.URL, Token: "fixture", TokenHeader: "PRIVATE-TOKEN"}, nil
	}})
}

func addRecoveryApp(t *testing.T, s *Service, u *recoveryUpstream) Installation {
	t.Helper()
	item, err := s.Create(t.Context(), "agent", CreateRequest{AppID: "gitlab", Name: "GitLab", Connect: true, Config: Config{URL: u.server.URL, AuthMode: "connector", ConnectorID: "gitlab", GitLabBaseURL: "https://gitlab.example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func TestDisconnectCancelsPendingRecoveryHandshake(t *testing.T) {
	u := newRecoveryUpstream(t)
	s := newRecoveryService(t)
	item := addRecoveryApp(t, s, u)
	u.blockInit.Store(true)
	recovery := beginAppRecovery(t, s, u, item)
	select {
	case <-u.initBlocked:
	case <-time.After(5 * time.Second):
		t.Fatal("recovery handshake never started")
	}
	if _, err := s.Disconnect(t.Context(), "agent", item.InstallationID); err != nil {
		t.Fatal(err)
	}
	close(u.releaseInit)
	select {
	case <-u.initFinished:
	case <-time.After(time.Second):
		t.Fatal("pending handshake was not released")
	}
	select {
	case <-recovery.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("pending recovery was not canceled")
	}
	current, err := s.Get(t.Context(), "agent", item.InstallationID)
	if err != nil || !current.Disconnected || current.Status != "disconnected" || len(current.Tools) != 0 {
		t.Fatalf("late handshake revived disconnected app: %+v %v", current, err)
	}
}

func TestAppRecoveryBackoffSurvivesShortLivedSessions(t *testing.T) {
	u := newRecoveryUpstream(t)
	s := newRecoveryService(t)
	item := addRecoveryApp(t, s, u)
	u.dropAll.Store(true)
	close(u.cut)
	waitAppCondition(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		e := s.entries[item.InstallationID]
		return u.initializes.Load() >= 2 && e.recovery != nil && e.recoveryDelay >= 2*time.Second
	})
	u.dropAll.Store(false)
	waitAppCondition(t, func() bool {
		current, _ := s.Get(t.Context(), "agent", item.InstallationID)
		return u.initializes.Load() >= 3 && current.Status == "connected"
	})
	client := gatewayClient(t, s, "agent")
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: toolName(item.InstallationID, "read"), Arguments: map[string]any{}})
	if err != nil || result.IsError || u.calls.Load() != 1 {
		t.Fatalf("tool unavailable after repeated recovery: %v", err)
	}
	if _, err := s.Disconnect(t.Context(), "agent", item.InstallationID); err != nil {
		t.Fatal(err)
	}
}

func TestInitialAppConnectionRetriesTransientFailure(t *testing.T) {
	u := newRecoveryUpstream(t)
	u.unavailable.Store(true)
	s := newRecoveryService(t)
	item := addRecoveryApp(t, s, u)
	if item.Status != "error" {
		t.Fatalf("initial outage status: %s", item.Status)
	}
	u.unavailable.Store(false)
	waitAppCondition(t, func() bool {
		current, _ := s.Get(t.Context(), "agent", item.InstallationID)
		return current.Status == "connected"
	})
}

func TestAppToolRefreshRecoversExpiredSessionAndPreservesAuthenticationFailure(t *testing.T) {
	for _, auth := range []bool{false, true} {
		t.Run(fmt.Sprint("auth=", auth), func(t *testing.T) {
			u := newRecoveryUpstream(t)
			s := newRecoveryService(t)
			item := addRecoveryApp(t, s, u)
			if auth {
				u.reject.Store(true)
			} else {
				u.expireList.Store(true)
			}
			u.notify <- struct{}{}
			if auth {
				waitAppCondition(t, func() bool {
					current, _ := s.Get(t.Context(), "agent", item.InstallationID)
					return current.Status == "authorization_required"
				})
				s.mu.Lock()
				defer s.mu.Unlock()
				if s.entries[item.InstallationID].recovery != nil {
					t.Fatal("tool-list authentication error started recovery")
				}
			} else {
				waitAppCondition(t, func() bool {
					current, _ := s.Get(t.Context(), "agent", item.InstallationID)
					return u.initializes.Load() >= 2 && current.Status == "connected"
				})
			}
		})
	}
}
