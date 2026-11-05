package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/apiclient"
	"github.com/gorilla/websocket"
)

func TestRestoredPublicationWaitsForOriginalLogin(t *testing.T) {
	var mode, attempts, registrations, sockets atomic.Int32
	upgrade := websocket.Upgrader{Subprotocols: []string{Subprotocol}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/relay/v1/agents":
			registrations.Add(1)
			writeTestJSON(w, 201, Publication{Agent: Agent{ID: testRelayID}})
		case "/api/v1/agent/instances":
			writeTestJSON(w, 200, map[string]any{"data": CommunityInstance{ID: 42, Type: "csgclaw", Kind: "runtime", Source: "external", Public: true, ContentID: testRelayID, Relay: &RelayRegistration{AgentID: testRelayID}}})
		case "/api/v1/relay/v1/agents/" + testRelayID + "/connect":
			conn, err := upgrade.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			sockets.Add(1)
			var frame Frame
			if conn.ReadJSON(&frame) != nil {
				return
			}
			if conn.WriteJSON(Frame{Type: "hello.ack", HeartbeatIntervalSeconds: 1, HeartbeatTimeoutSeconds: 5}) != nil {
				return
			}
			for {
				if conn.ReadJSON(&frame) != nil {
					return
				}
			}
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	source := func() (SiteCredentials, error) {
		attempts.Add(1)
		if mode.Load() == 0 {
			return SiteCredentials{}, ErrSiteLoginRequired
		}
		user := "original"
		if mode.Load() == 1 {
			user = "another-account"
		}
		return SiteCredentials{Site: server.URL, User: user, Token: "fake-token"}, nil
	}
	root := t.TempDir()
	manifest := relayManifest{Site: server.URL, User: "original", Agents: map[string]PublishRequest{testRelayID: {Name: "worker", SourceAgentID: "local", Visibility: "public"}}}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(root, "publications.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(context.Background(), root, apiclient.New(server.URL, "", server.Client()), WithCommunity(HubCommunity{Credentials: source}, source))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	wait := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("timed out waiting for login recovery")
	}
	wait(func() bool { return attempts.Load() > 0 })
	mode.Store(1)
	before := attempts.Load()
	wait(func() bool { return attempts.Load() > before })
	if registrations.Load() != 0 || sockets.Load() != 0 {
		t.Fatal("restored publication switched accounts")
	}
	m.mu.Lock()
	run := m.running[testRelayID]
	m.mu.Unlock()
	if run == nil {
		t.Fatal("missing login discarded the desired executor")
	}
	mode.Store(2)
	wait(func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.running[testRelayID] != nil && m.running[testRelayID].connected
	})
	if registrations.Load() == 0 || sockets.Load() != 1 {
		t.Fatal("original login did not restore registration and connection")
	}
	m.Close()
}

func TestRestoredPublicationLoginWaitStopsOnShutdown(t *testing.T) {
	root := t.TempDir()
	manifest := relayManifest{Site: "http://community.test", User: "original", Agents: map[string]PublishRequest{testRelayID: {Name: "worker", SourceAgentID: "local", Visibility: "public"}}}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(root, "publications.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	source := func() (SiteCredentials, error) { return SiteCredentials{}, ErrSiteLoginRequired }
	m, err := NewManager(context.Background(), root, nil, WithCommunity(HubCommunity{Credentials: source}, source))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { m.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel credential retries")
	}
}

func TestCommunityAvailabilityPreservesQueryFailures(t *testing.T) {
	for _, test := range []struct {
		status int
		state  string
	}{
		{http.StatusUnauthorized, ""}, {http.StatusTooManyRequests, ""},
		{http.StatusServiceUnavailable, ""}, {http.StatusNotFound, ""},
		{http.StatusOK, "offline"}, {http.StatusOK, "online"}, {http.StatusOK, "unhealthy"},
	} {
		status := test.status
		t.Run(fmt.Sprintf("%d/%s", status, test.state), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/agent/instances" {
					writeTestJSON(w, 200, map[string]any{"data": []CommunityInstance{{ID: 1, Type: "csgclaw", Kind: "runtime", Source: "external", Public: true, Relay: &RelayRegistration{AgentID: testRelayID}}}})
					return
				}
				if status == http.StatusOK {
					writeTestJSON(w, 200, Agent{ID: testRelayID, Status: test.state})
					return
				}
				w.WriteHeader(status)
			}))
			defer server.Close()
			source := func() (SiteCredentials, error) {
				return SiteCredentials{Site: server.URL, Token: "fake-token", User: "caller"}, nil
			}
			m, err := NewManager(context.Background(), t.TempDir(), nil, WithCommunity(HubCommunity{Credentials: source}, source))
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			entries, err := m.Discover(context.Background())
			if status == http.StatusNotFound || status == http.StatusOK {
				expected := test.state
				if status == http.StatusNotFound {
					expected = "offline"
				}
				if err != nil || len(entries) != 1 || entries[0].Agent.Status != expected {
					t.Fatalf("confirmed offline registration lost: %v %+v", err, entries)
				}
			} else {
				var remote *HTTPError
				if !errors.As(err, &remote) || remote.Status != status || len(entries) != 0 {
					t.Fatalf("query failure became an offline listing: %v %+v", err, entries)
				}
			}
		})
	}
}

func TestCommunityAvailabilityTransportFailureIsNotOffline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/instances" {
			writeTestJSON(w, 200, map[string]any{"data": []CommunityInstance{{ID: 1, Type: "csgclaw", Kind: "runtime", Source: "external", Public: true, Relay: &RelayRegistration{AgentID: testRelayID}}}})
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	defer server.Close()
	source := func() (SiteCredentials, error) {
		return SiteCredentials{Site: server.URL, Token: "fake-token", User: "caller"}, nil
	}
	m, err := NewManager(context.Background(), t.TempDir(), nil, WithCommunity(HubCommunity{Credentials: source}, source))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if entries, err := m.Discover(context.Background()); err == nil || len(entries) != 0 {
		t.Fatalf("transport failure became offline: %v %+v", err, entries)
	}
}
