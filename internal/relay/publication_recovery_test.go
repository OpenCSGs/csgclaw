package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestFailedFirstPublicationDoesNotPinAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(502)
			return
		}
		writeTestJSON(w, 200, map[string]any{"data": []CommunityInstance{}})
	}))
	defer server.Close()
	cred := SiteCredentials{Site: server.URL, Token: "fake-token", User: "one"}
	source := func() (SiteCredentials, error) { return cred, nil }
	root := t.TempDir()
	m, err := NewManager(context.Background(), root, nil, WithCommunity(HubCommunity{Credentials: source}, source))
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Publish(context.Background(), PublishRequest{Name: "worker", SourceAgentID: "local", Visibility: "public"})
	if err == nil {
		t.Fatal("expected registration failure")
	}
	cred.User = "two"
	if _, err = m.Discover(context.Background()); err != nil {
		t.Fatalf("failed registration pinned login: %v", err)
	}
	m.Close()
	// Older releases persisted an account even when no publication was created.
	data, err := os.ReadFile(filepath.Join(root, "publications.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest relayManifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Site, manifest.User = server.URL, "one"
	data, _ = json.Marshal(manifest)
	if err = os.WriteFile(filepath.Join(root, "publications.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	m, err = NewManager(context.Background(), root, nil, WithCommunity(HubCommunity{Credentials: source}, source))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err = m.Discover(context.Background()); err != nil {
		t.Fatalf("old empty binding survived restart: %v", err)
	}
}

func TestUnpublishRecoversUnconfirmedCommunityCommit(t *testing.T) {
	exists, failLookup := false, true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/relay/v1/agents" && r.Method == http.MethodPost:
			writeTestJSON(w, 201, Publication{Agent: Agent{ID: testRelayID}})
		case r.URL.Path == "/api/v1/agent/instances" && r.Method == http.MethodPost:
			exists = true
			w.WriteHeader(502)
		case r.URL.Path == "/api/v1/agent/instances" && r.Method == http.MethodGet:
			if failLookup {
				w.WriteHeader(503)
				return
			}
			if r.URL.Query().Get("source") != "external" || r.URL.Query().Has("public") {
				t.Error("cleanup must include private external registrations")
			}
			records := []CommunityInstance{}
			if exists {
				records = append(records, CommunityInstance{ID: 42, Type: "csgclaw", Kind: "runtime", Source: "external", ContentID: testRelayID, Relay: &RelayRegistration{AgentID: testRelayID}})
			}
			writeTestJSON(w, 200, map[string]any{"data": records})
		case r.URL.Path == "/api/v1/agent/instances/42" && r.Method == http.MethodGet:
			writeTestJSON(w, 200, map[string]any{"data": CommunityInstance{ID: 42, Type: "csgclaw", Source: "external"}})
		case r.URL.Path == "/api/v1/agent/instances/42" && r.Method == http.MethodDelete:
			exists = false
			w.WriteHeader(200)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/relay/v1/agents/"+testRelayID:
			writeTestJSON(w, 200, Agent{ID: testRelayID})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/relay/v1/agents/"+testRelayID:
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	source := func() (SiteCredentials, error) {
		return SiteCredentials{Site: server.URL, Token: "fake-token", User: "one"}, nil
	}
	root := t.TempDir()
	create := func() *Manager {
		m, err := NewManager(context.Background(), root, nil, WithCommunity(HubCommunity{Credentials: source}, source))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	m := create()
	_, err := m.Publish(context.Background(), PublishRequest{Name: "worker", SourceAgentID: "local", Visibility: "public"})
	if err == nil {
		t.Fatal("expected lost success response")
	}
	status := m.Status(context.Background(), "local")
	if !status.Pending || !status.CanUnpublish || status.Published {
		t.Fatalf("missing recovery state: %+v", status)
	}
	if err = m.Unpublish(context.Background(), "local"); err == nil {
		t.Fatal("cleanup claimed success without resolving directory")
	}
	if !m.manifest.Removing["local"] {
		t.Fatal("cleanup intent not retained")
	}
	m.Close()
	failLookup = false
	m = create()
	defer m.Close()
	if len(m.running) != 0 {
		t.Fatal("restart resurrected cleanup")
	}
	if err = m.Unpublish(context.Background(), "local"); err != nil {
		t.Fatal(err)
	}
	if exists || len(m.manifest.Agents) != 0 || len(m.manifest.Removing) != 0 {
		t.Fatal("cleanup retained remote registration or local recovery state")
	}
}

func TestDiscoveryConcurrentProbesAndExpiredBudget(t *testing.T) {
	var probes atomic.Int32
	bothStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/instances" {
			writeTestJSON(w, 200, map[string]any{"data": []CommunityInstance{
				{ID: 1, Name: "first", Type: "csgclaw", Kind: "runtime", Source: "external", Public: true, Relay: &RelayRegistration{AgentID: testRelayID}},
				{ID: 2, Name: "second", Type: "csgclaw", Kind: "runtime", Source: "external", Public: true, Relay: &RelayRegistration{AgentID: platformRelayID}},
			}})
			return
		}
		if probes.Add(1) == 2 {
			close(bothStarted)
		}
		select {
		case <-r.Context().Done():
			return
		case <-bothStarted:
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
		id := testRelayID
		if r.URL.Path == "/api/v1/relay/v1/agents/"+platformRelayID {
			id = platformRelayID
		}
		writeTestJSON(w, 200, Agent{ID: id, Status: "online"})
	}))
	defer server.Close()
	source := func() (SiteCredentials, error) {
		return SiteCredentials{Site: server.URL, Token: "fake-token", User: "one"}, nil
	}
	m, err := NewManager(context.Background(), t.TempDir(), nil, WithCommunity(HubCommunity{Credentials: source}, source))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	entries, err := m.Discover(ctx)
	if err != nil || len(entries) != 2 {
		t.Fatalf("parallel probes failed: %v", err)
	}
	for _, entry := range entries {
		if entry.Agent.Status != "online" {
			t.Fatal("online agent was marked offline")
		}
	}
	entries, err = m.DiscoverCatalog(t.Context(), nil)
	if err != nil || len(entries) != 2 || probes.Load() != 2 {
		t.Fatalf("metadata request started probes: entries=%d probes=%d err=%v", len(entries), probes.Load(), err)
	}
	entries, err = m.DiscoverCatalog(t.Context(), []string{platformRelayID})
	if err != nil || len(entries) != 1 || entries[0].Agent.ID != platformRelayID || probes.Load() != 3 {
		t.Fatalf("selected probe returned unexpected result: entries=%+v probes=%d err=%v", entries, probes.Load(), err)
	}

}

func TestQueuedPublicationCanBeCancelledWithoutLoginAndBlocksDeletion(t *testing.T) {
	source := func() (SiteCredentials, error) { return SiteCredentials{}, ErrSiteLoginRequired }
	m, err := NewManager(context.Background(), t.TempDir(), nil, WithCommunity(HubCommunity{Credentials: source}, source))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.manifest.AutoPublish["local"] = PublishRequest{SourceAgentID: "local"}
	if release, err := m.GuardAgentDeletion("local"); err == nil {
		release()
		t.Fatal("allowed deletion of a queued publication")
	}
	status := m.Status(context.Background(), "local")
	if !status.Publishing || !status.CanUnpublish {
		t.Fatalf("queue has no cancellation state: %+v", status)
	}
	if err = m.Unpublish(context.Background(), "local"); err != nil {
		t.Fatal(err)
	}
	release, err := m.GuardAgentDeletion("local")
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestDiscoverAgentDoesNotProbeOtherRegistrations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/instances":
			writeTestJSON(w, 200, map[string]any{"data": []CommunityInstance{
				{ID: 1, Type: "csgclaw", Kind: "runtime", Source: "external", Public: true, Relay: &RelayRegistration{AgentID: testRelayID}},
				{ID: 2, Type: "csgclaw", Kind: "runtime", Source: "external", Public: true, Relay: &RelayRegistration{AgentID: platformRelayID}},
			}})
		case "/api/v1/relay/v1/agents/" + testRelayID:
			writeTestJSON(w, 200, Agent{ID: testRelayID, Status: "online"})
		default:
			t.Errorf("unrelated registration was probed: %s", r.URL.Path)
			w.WriteHeader(503)
		}
	}))
	defer server.Close()
	source := func() (SiteCredentials, error) {
		return SiteCredentials{Site: server.URL, Token: "fake-token", User: "one"}, nil
	}
	m, err := NewManager(context.Background(), t.TempDir(), nil, WithCommunity(HubCommunity{Credentials: source}, source))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	reference, err := m.connectionReference()
	if err != nil {
		t.Fatal(err)
	}
	entry, err := m.DiscoverAgent(context.Background(), testRelayID, reference)
	if err != nil || entry.Agent.Status != "online" {
		t.Fatalf("selected registration failed: %+v %v", entry, err)
	}
	if _, err = m.DiscoverAgent(context.Background(), testRelayID, "another-login"); err != ErrChatReference {
		t.Fatalf("accepted another login reference: %v", err)
	}
}
