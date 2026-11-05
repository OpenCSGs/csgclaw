package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/apiclient"
	"github.com/gorilla/websocket"
)

func TestPlatformTargetReleasedAfterLocalDeletion(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer local.Close()
	root := t.TempDir()
	m, err := NewManager(context.Background(), root, apiclient.New(local.URL, "", local.Client()))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	target := &CommunityTarget{Type: "csgclaw", ContentID: "sandbox-one"}
	if err = m.QueuePublication(PublishRequest{Name: "old", SourceAgentID: "old", Visibility: "public"}, target); err != nil {
		t.Fatal(err)
	}
	if err = m.Unpublish(context.Background(), "old"); err != nil {
		t.Fatal(err)
	}
	release, err := m.GuardAgentDeletion("old")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.CompleteAgentDeletion("old"); err != nil {
		release()
		t.Fatal(err)
	}
	release()
	m.Close()
	m, err = NewManager(context.Background(), root, apiclient.New(local.URL, "", local.Client()))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	err = m.QueuePublication(PublishRequest{Name: "replacement", SourceAgentID: "replacement", Visibility: "public"}, target)
	if err != nil {
		t.Fatalf("replacement registration still blocked: %v", err)
	}
}

func TestCommunitySyncRetriesWithoutSocketReconnect(t *testing.T) {
	for _, platform := range []bool{false, true} {
		t.Run(fmt.Sprintf("platform=%t", platform), func(t *testing.T) {
			origin, contentID := "external", testRelayID
			if platform {
				origin, contentID = "platform", "sandbox-one"
			}
			var public atomic.Bool
			var ready atomic.Bool
			var updates atomic.Int32
			var sockets atomic.Int32
			upgrade := websocket.Upgrader{Subprotocols: []string{Subprotocol}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/relay/v1/agents":
					var request PublishRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					public.Store(request.Visibility == "public")
					writeTestJSON(w, 201, Publication{Agent: Agent{ID: testRelayID}})
				case "/api/v1/agent/instances", "/api/v1/agent/instances/by-content-id/csgclaw/sandbox-one":
					updates.Add(1)
					if !ready.Load() {
						w.WriteHeader(503)
						return
					}
					writeTestJSON(w, 200, map[string]any{"data": CommunityInstance{ID: 42, Name: "worker", Type: "csgclaw", Kind: "runtime", Source: origin, Public: true, ContentID: contentID, Relay: &RelayRegistration{AgentID: testRelayID}}})
				case "/api/v1/relay/v1/agents/" + testRelayID + "/connect":
					conn, err := upgrade.Upgrade(w, r, nil)
					if err != nil {
						return
					}
					defer conn.Close()
					sockets.Add(1)
					var hello Frame
					if conn.ReadJSON(&hello) != nil {
						return
					}
					if conn.WriteJSON(Frame{Type: "hello.ack", HeartbeatIntervalSeconds: 1, HeartbeatTimeoutSeconds: 5}) != nil {
						return
					}
					for {
						var frame Frame
						if conn.ReadJSON(&frame) != nil {
							return
						}
					}
				case "/api/v1/agents/local":
					writeTestJSON(w, 200, map[string]any{})
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			source := func() (SiteCredentials, error) {
				return SiteCredentials{Site: server.URL, Token: "fake-token", User: "one"}, nil
			}
			m, err := NewManager(context.Background(), t.TempDir(), apiclient.New(server.URL, "", server.Client()), WithCommunity(HubCommunity{Credentials: source}, source))
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			req := PublishRequest{Name: "worker", SourceAgentID: "local", Visibility: "public"}
			m.mu.Lock()
			if platform {
				m.manifest.Targets["local"] = CommunityTarget{Type: "csgclaw", ContentID: contentID}
			}
			m.manifest.Agents[testRelayID] = req
			m.manifest.Pending[testRelayID] = true
			m.startLocked(testRelayID, req)
			m.mu.Unlock()
			deadline := time.Now().Add(3 * time.Second)
			connected := false
			for time.Now().Before(deadline) {
				m.mu.Lock()
				connected = m.running[testRelayID] != nil && m.running[testRelayID].connected
				m.mu.Unlock()
				if connected {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !connected {
				t.Fatal("executor did not connect")
			}
			ready.Store(true)
			deadline = time.Now().Add(4 * time.Second)
			pending := true
			for time.Now().Before(deadline) {
				m.mu.Lock()
				pending = m.manifest.Pending[testRelayID]
				m.mu.Unlock()
				if !pending {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if pending || updates.Load() < 2 || sockets.Load() != 1 || !public.Load() {
				t.Fatalf("sync did not recover on the healthy socket: pending=%v updates=%d sockets=%d", pending, updates.Load(), sockets.Load())
			}

			if err := m.Unpublish(context.Background(), "local"); err != nil {
				t.Fatal(err)
			}
			before := updates.Load()
			time.Sleep(1100 * time.Millisecond)
			if updates.Load() != before {
				t.Fatal("retry worker registered after unpublish")
			}

		})
	}
}
