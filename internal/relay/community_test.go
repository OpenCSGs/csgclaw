package relay

import (
	"context"
	"csgclaw/internal/apiclient"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const testRelayID = "agt_0123456789abcdef01234567"
const platformRelayID = "agt_abcdef0123456789abcdef01"

func TestCommunityPublicationLifecycle(t *testing.T) {
	var mu sync.Mutex
	events := []string{}
	failRegister := true
	failDelete := false
	records := map[string]int64{}
	var clientID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer site-token" {
			t.Error("missing current login")
		}
		switch {
		case r.URL.Path == "/api/v1/relay/v1/agents" && r.Method == http.MethodPost:
			events = append(events, "relay")
			clientID = r.Header.Get("X-CSG-Client-ID")
			if clientID == "" || r.Header.Get("Idempotency-Key") == "" {
				t.Error("missing relay identity/key")
			}
			writeTestJSON(w, 201, Publication{Agent: Agent{ID: testRelayID, Visibility: "public"}})
		case r.URL.Path == "/api/v1/agent/instances" && r.Method == http.MethodPost:
			events = append(events, "instance")
			var body struct {
				Type      string            `json:"type"`
				Kind      string            `json:"kind"`
				Source    string            `json:"source"`
				ContentID string            `json:"content_id"`
				Public    bool              `json:"public"`
				Relay     RelayRegistration `json:"relay"`
				Metadata  json.RawMessage   `json:"metadata"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Type != "csgclaw" || body.Kind != "runtime" || body.Source != "external" || !body.Public || body.ContentID != testRelayID || body.Relay.AgentID != testRelayID {
				t.Error("invalid external instance")
			}
			if len(body.Metadata) > 0 {
				t.Error("wrote obsolete subscription metadata")
			}
			if failRegister {
				w.WriteHeader(503)
				return
			}
			records[testRelayID] = 42
			writeTestJSON(w, 200, map[string]any{"data": map[string]any{"id": 42, "type": "csgclaw", "kind": "runtime", "source": "external", "public": true, "content_id": testRelayID, "relay": RelayRegistration{AgentID: testRelayID}}})
		case r.URL.Path == "/api/v1/agent/instances/42" && r.Method == http.MethodGet:
			writeTestJSON(w, 200, map[string]any{"data": map[string]any{"id": 42, "type": "csgclaw", "source": "external"}})
		case r.URL.Path == "/api/v1/agent/instances/42" && r.Method == http.MethodDelete:
			events = append(events, "remove-instance")
			if failDelete {
				w.WriteHeader(503)
				return
			}
			delete(records, testRelayID)
			w.WriteHeader(200)
		case r.URL.Path == "/api/v1/relay/v1/agents/"+testRelayID && r.Method == http.MethodDelete:
			events = append(events, "remove-relay")
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/connect"):
			w.WriteHeader(404)
		default:
			writeTestJSON(w, 200, Agent{ID: testRelayID})
		}
	}))
	defer server.Close()
	var authMu sync.Mutex
	cred := SiteCredentials{Site: server.URL, Token: "site-token", User: "user-one"}
	source := func() (SiteCredentials, error) { authMu.Lock(); defer authMu.Unlock(); return cred, nil }
	root := t.TempDir()
	createManager := func() *Manager {
		m, err := NewManager(context.Background(), root, nil, WithCommunity(HubCommunity{Credentials: source}, source))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	m := createManager()
	req := PublishRequest{Name: "Local", SourceAgentID: "local", Visibility: "public"}
	if _, err := m.Publish(context.Background(), req); err == nil {
		t.Fatal("accepted failed community registration")
	}
	if m.Status(context.Background(), "local").Published {
		t.Fatal("pending registration reported complete")
	}
	m.Close()
	mu.Lock()
	failRegister = false
	mu.Unlock()
	m = createManager()
	if _, err := m.Publish(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(records) != 1 || strings.Join(events[:2], ",") != "relay,instance" {
		t.Error("incorrect registration order or duplicate records")
	}
	if clientID != m.manifest.ClientID {
		t.Error("client identity changed")
	}
	mu.Unlock()
	data, err := os.ReadFile(filepath.Join(root, "publications.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "site-token") {
		t.Fatal("login token persisted")
	}
	authMu.Lock()
	cred.User = "other-user"
	authMu.Unlock()
	if _, err = m.Publish(context.Background(), req); err == nil {
		t.Fatal("accepted another account")
	}
	authMu.Lock()
	cred.User = "user-one"
	authMu.Unlock()
	mu.Lock()
	failDelete = true
	mu.Unlock()
	if err = m.Unpublish(context.Background(), "local"); err == nil {
		t.Fatal("accepted failed cleanup")
	}
	m.Close()
	// Restart during failed cleanup must not re-register/recreate the directory row.
	mu.Lock()
	before := len(events)
	mu.Unlock()
	m = createManager()
	defer m.Close()
	m.mu.Lock()
	running := len(m.running)
	m.mu.Unlock()
	if running != 0 {
		t.Fatal("restart resurrected an unpublishing agent")
	}
	mu.Lock()
	if len(events) != before {
		t.Error("restart sent a registration during cleanup")
	}
	failDelete = false
	mu.Unlock()
	if err = m.Unpublish(context.Background(), "local"); err != nil {
		t.Fatal(err)
	}
	if len(m.manifest.Agents) != 0 || len(m.manifest.Instances) != 0 {
		t.Fatal("unpublish retained desired registration")
	}
}

func TestStableClientIdentityAcrossRestartAndPorts(t *testing.T) {
	root := t.TempDir()
	first, err := StableClientID(InstanceRoot(root, "http://127.0.0.1:8000"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := StableClientID(InstanceRoot(root, "http://127.0.0.1:8000/"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := StableClientID(InstanceRoot(root, "http://127.0.0.1:8001"))
	if err != nil {
		t.Fatal(err)
	}
	if first != again || first == second {
		t.Fatal("client ID is not stable or port isolated")
	}
}

func TestCommunityDiscoveryAndInvocationOfRemotePublication(t *testing.T) {
	var public atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner := r.Header.Get("Authorization") == "Bearer owner-token"
		switch r.URL.Path {
		case "/api/v1/agent/instances":
			if r.URL.Query().Has("public") || r.URL.Query().Get("registered") != "true" || r.URL.Query().Get("kind") != "runtime" {
				t.Error("not querying instances visible to the caller")
			}
			items := []map[string]any{
				{"id": 7, "type": "csgclaw", "public": true, "name": "Remote", "kind": "runtime", "source": "external", "relay": RelayRegistration{AgentID: testRelayID}},
				{"id": 8, "type": "csgclaw", "public": true, "name": "Pending", "kind": "runtime"},
				{"id": 9, "type": "csgclaw-workspace", "public": true, "name": "Platform", "kind": "runtime", "source": "platform", "relay": RelayRegistration{AgentID: platformRelayID}, "is_running": true},
				{"id": 10, "type": "csgclaw", "public": true, "kind": "logical", "relay": RelayRegistration{AgentID: testRelayID}},
				{"id": 11, "type": "csgclaw", "public": true, "kind": "runtime", "relay": RelayRegistration{AgentID: "invalid"}},
			}
			items[0]["public"] = public.Load()
			if !owner && !public.Load() {
				items = items[1:]
			}
			writeTestJSON(w, 200, map[string]any{"data": items, "total": len(items)})
		case "/api/v1/relay/v1/agents/" + testRelayID:
			writeTestJSON(w, 200, Agent{ID: testRelayID, Name: "Relay internal name", Visibility: "private", Status: "online"})
		case "/api/v1/relay/v1/agents/" + platformRelayID:
			w.WriteHeader(404)
		case "/api/v1/relay/v1/responses":
			if !owner && !public.Load() {
				t.Error("private invocation reached Relay")
			}
			if r.Header.Get("X-CSG-Client-ID") == "" {
				t.Error("missing caller client identity")
			}
			var req CallRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Model != testRelayID {
				t.Error("did not invoke subscription ID")
			}
			writeTestJSON(w, 200, Response{ID: "resp_remote", Model: req.Model, Status: "completed"})
		default:
			t.Error(fmt.Sprint("unexpected route ", r.URL.Path))
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	login := "owner"
	source := func() (SiteCredentials, error) {
		return SiteCredentials{Site: server.URL, Token: login + "-token", User: login}, nil
	}
	m, err := NewManager(context.Background(), t.TempDir(), nil, WithCommunity(HubCommunity{Credentials: source}, source))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	entries, err := m.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Agent.ID != testRelayID || entries[0].Agent.Status != "online" || entries[1].Agent.Status != "offline" {
		t.Fatal("invalid community discovery")
	}
	if entries[0].Source != "external" || entries[1].Source != "platform" || entries[0].Agent.Name != "Remote" {
		t.Fatal("discovery lost community origin or display name")
	}
	if entries[0].Type != "csgclaw" || entries[1].Type != "csgclaw-workspace" {
		t.Fatal("discovery lost community instance types")
	}
	if entries[0].Agent.Visibility != "private" || entries[1].Agent.Visibility != "public" {
		t.Fatal("catalog visibility differs from StarHub")
	}
	if len(m.manifest.Agents) != 0 {
		t.Fatal("remote agent became a local publication")
	}
	if _, err = m.Chat(context.Background(), entries[0].Agent.ID, entries[0].ConnectionID, "call-remote", "hello", ""); err != nil {
		t.Fatal(err)
	}
	login = "caller"
	visible, err := m.Discover(t.Context())
	if err != nil || len(visible) != 1 || visible[0].Agent.ID != platformRelayID {
		t.Fatalf("other caller discovery: %v, %v", visible, err)
	}
	if _, err := m.Chat(t.Context(), testRelayID, visible[0].ConnectionID, "call-private", "hello", ""); !errors.Is(err, ErrChatReference) {
		t.Fatalf("private agent invocation by another caller: %v", err)
	}
	public.Store(true)
	visible, err = m.Discover(t.Context())
	if err != nil || len(visible) != 2 || visible[0].Agent.Visibility != "public" {
		t.Fatalf("public agent discovery: %v, %v", visible, err)
	}
	if _, err = m.Chat(t.Context(), testRelayID, visible[0].ConnectionID, "call-public", "hello", ""); err != nil {
		t.Fatal(err)
	}
}

func TestRelayHTTPAndWebSocketProxyHeaders(t *testing.T) {
	var mu sync.Mutex
	keys := map[string]bool{}
	seenSocket := false
	upgrader := websocket.Upgrader{Subprotocols: []string{Subprotocol}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if r.Header.Get("Authorization") != "Bearer access-token" || r.Header.Get("X-CSG-Client-ID") != "claw-stable" || r.Header.Get("X-CSG-Request-Id") == "" {
			t.Error("missing proxy authentication headers")
		}
		if r.Header.Get("X-CSG-User-UUID") != "" {
			t.Error("client forged trusted upstream identity")
		}
		key := r.Header.Get("Idempotency-Key")
		if key == "" || keys[key] {
			t.Error("new request did not receive independent key")
		}
		keys[key] = true
		if strings.HasSuffix(r.URL.Path, "/connect") {
			seenSocket = true
			mu.Unlock()
			conn, err := upgrader.Upgrade(w, r, nil)
			if err == nil {
				conn.Close()
			}
			return
		}
		mu.Unlock()
		w.WriteHeader(204)
	}))
	defer server.Close()
	c, err := NewClient(ResolveRelayProxy(server.URL), "access-token", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.WithClientID("claw-stable")
	for range 2 {
		if err = c.Request(context.Background(), http.MethodGet, "/v1/agents", "", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	conn, err := c.connect(context.Background(), "agt_local")
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	mu.Lock()
	defer mu.Unlock()
	if !seenSocket || len(keys) != 3 {
		t.Fatal("did not verify both transports")
	}
}

func TestPlatformWorkerUpdatesByContentIDAndPreservesLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name          string
		public        bool
		emptyResponse bool
		nullResponse  bool
	}{
		{name: "private"},
		{name: "public", public: true},
		{name: "null response", public: true, nullResponse: true},
		{name: "empty response", emptyResponse: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			public := tc.public
			t.Setenv("CSGCLAW_NAME", "platform-instance")
			t.Setenv("OPENCSG_SANDBOX_TYPE", "csgclaw")
			target, err := PlatformTargetFromEnvironment()
			if err != nil || target == nil {
				t.Fatalf("target: %v", err)
			}
			var mu sync.Mutex
			var ready bool
			var omitInstance bool
			var updates, externalCreates, instanceDeletes int
			var visibilities []string
			failOnce := true
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch {
				case r.URL.Path == "/api/v1/agents/local":
					state := "starting"
					if ready {
						state = "running"
					}
					writeTestJSON(w, 200, map[string]any{"id": "local", "status": state, "runtime": map[string]any{"name": "codex", "state": state}})
				case r.URL.Path == "/api/v1/relay/v1/agents" && r.Method == http.MethodPost:
					var req PublishRequest
					_ = json.NewDecoder(r.Body).Decode(&req)
					visibilities = append(visibilities, req.Visibility)
					writeTestJSON(w, 201, Publication{Agent: Agent{ID: testRelayID, Visibility: req.Visibility}})
				case r.URL.Path == "/api/v1/agent/instances/by-content-id/csgclaw/platform-instance" && r.Method == http.MethodPut:
					updates++
					var body map[string]json.RawMessage
					_ = json.NewDecoder(r.Body).Decode(&body)
					var registration RelayRegistration
					_ = json.Unmarshal(body["relay"], &registration)
					if len(body) != 1 || registration.AgentID != testRelayID {
						t.Error("platform patch changed more than relay.agent_id")
					}
					if failOnce {
						failOnce = false
						w.WriteHeader(404)
						return
					}
					if tc.emptyResponse || omitInstance {
						w.WriteHeader(http.StatusOK)
						return
					}
					if tc.nullResponse {
						writeTestJSON(w, http.StatusOK, map[string]any{"data": nil})
						return
					}
					writeTestJSON(w, 200, map[string]any{"data": map[string]any{"id": 7, "type": "csgclaw", "kind": "runtime", "source": "platform", "content_id": "platform-instance", "public": public, "relay": RelayRegistration{AgentID: testRelayID}}})
				case r.URL.Path == "/api/v1/agent/instances" && r.Method == http.MethodGet:
					writeTestJSON(w, 200, map[string]any{"data": []CommunityInstance{{ID: 7, Type: "csgclaw", Kind: "runtime", Source: "platform", ContentID: "platform-instance", Public: public, Relay: &RelayRegistration{AgentID: testRelayID}}}})
				case r.URL.Path == "/api/v1/agent/instances" && r.Method == http.MethodPost:
					externalCreates++
					w.WriteHeader(400)
				case strings.HasPrefix(r.URL.Path, "/api/v1/agent/instances") && r.Method == http.MethodDelete:
					instanceDeletes++
					w.WriteHeader(400)
				case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/relay/"):
					w.WriteHeader(204)
				case strings.HasSuffix(r.URL.Path, "/connect"):
					w.WriteHeader(404)
				default:
					writeTestJSON(w, 200, Agent{ID: testRelayID})
				}
			}))
			defer server.Close()
			source := func() (SiteCredentials, error) {
				return SiteCredentials{Site: server.URL, Token: "platform-token", User: "owner"}, nil
			}
			root := t.TempDir()
			create := func() *Manager {
				m, err := NewManager(context.Background(), root, apiclient.New(server.URL, "", server.Client()), WithCommunity(HubCommunity{Credentials: source}, source))
				if err != nil {
					t.Fatal(err)
				}
				return m
			}
			m := create()
			req := PublishRequest{Name: "Template worker", SourceAgentID: "local", Executor: "csgclaw", Visibility: "public"}
			if err = m.QueuePublication(req, target); err != nil {
				t.Fatal(err)
			}
			m.Close()
			// The row identity survives restart even if the environment no longer supplies it.
			t.Setenv("CSGCLAW_NAME", "")
			mu.Lock()
			ready = true
			mu.Unlock()
			m = create()
			defer m.Close()
			waitFor(t, func() bool { s := m.Status(context.Background(), "local"); return s.Published && !s.Publishing })
			mu.Lock()
			if updates < 2 || externalCreates != 0 || len(visibilities) == 0 || visibilities[0] != "private" {
				t.Error("incorrect platform startup flow")
			}
			if !public {
				for _, v := range visibilities {
					if v != "private" {
						t.Error("private platform worker was published publicly")
					}
				}
			}
			mu.Unlock()
			m.mu.Lock()
			got := m.manifest.Agents[testRelayID].Visibility
			targetSaved := m.manifest.Targets["local"]
			m.mu.Unlock()
			expected := "private"
			if public {
				expected = "public"
			}
			if got != expected || targetSaved.ContentID != "platform-instance" {
				t.Fatal("visibility or target not persisted")
			}
			if public {
				mu.Lock()
				public = false
				mu.Unlock()
				c, err := m.client("", "")
				if err != nil {
					t.Fatal(err)
				}
				m.refreshPlatformVisibility(t.Context(), c, testRelayID)
				m.mu.Lock()
				refreshed := m.manifest.Agents[testRelayID].Visibility
				m.mu.Unlock()
				if refreshed != "private" {
					t.Fatalf("running worker kept visibility %q after the catalog changed", refreshed)
				}
				m.Close()
				mu.Lock()
				omitInstance = true
				previousUpdates := updates
				mu.Unlock()
				m = create()
				defer m.Close()
				waitFor(t, func() bool {
					mu.Lock()
					updated := updates > previousUpdates
					mu.Unlock()
					status := m.Status(context.Background(), "local")
					return updated && status.Published && !status.Publishing
				})
				m.mu.Lock()
				visibility := m.manifest.Agents[testRelayID].Visibility
				m.mu.Unlock()
				mu.Lock()
				lastRegistration := visibilities[len(visibilities)-1]
				mu.Unlock()
				if visibility != "private" || lastRegistration != "private" {
					t.Fatalf("empty writeback after a visibility change left manifest %q and Relay %q", visibility, lastRegistration)
				}
			}
			if err = m.QueuePublication(PublishRequest{Name: "Other", SourceAgentID: "other-local", Visibility: "public"}, target); err == nil {
				t.Fatal("bound the platform instance to a second local agent")
			}
			if err = m.Unpublish(context.Background(), "local"); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			deletes := instanceDeletes
			mu.Unlock()
			if deletes != 0 {
				t.Fatal("unpublish attempted to delete platform instance")
			}
			// A manual republish of the same worker still patches the existing row.
			if _, err = m.Publish(context.Background(), req); err != nil {
				t.Fatal(err)
			}
			if err = m.Unpublish(context.Background(), "local"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPlatformWritebackValidatesReturnedInstance(t *testing.T) {
	valid := fmt.Sprintf(`{"id":7,"type":"csgclaw","kind":"runtime","source":"platform","content_id":"platform-instance","relay":{"agent_id":%q}}`, testRelayID)
	for _, tc := range []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "literal null", body: "null"},
		{name: "acknowledgement", body: `{"msg":"OK"}`},
		{name: "missing public", body: `{"data":` + valid + `}`},
		{name: "catalog overrides writeback public", body: `{"data":` + strings.Replace(valid, `{"id":`, `{"public":true,"id":`, 1) + `}`},
		{name: "malformed", body: `{"data":`, wantErr: true},
		{name: "unexpected envelope", body: `{"error":"write failed"}`, wantErr: true},
		{name: "wrong instance", body: `{"data":` + strings.Replace(valid, "platform-instance", "another-instance", 1) + `}`, wantErr: true},
		{name: "wrong relay", body: `{"data":` + strings.Replace(valid, testRelayID, platformRelayID, 1) + `}`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					fmt.Fprint(w, `{"data":[`+valid+`]}`)
					return
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			community := HubCommunity{Credentials: func() (SiteCredentials, error) {
				return SiteCredentials{Site: server.URL, Token: "test-token"}, nil
			}}
			instance, err := community.Register(t.Context(), testRelayID, PublishRequest{Visibility: "public"}, &CommunityTarget{Type: "csgclaw", ContentID: "platform-instance"})
			if (err != nil) != tc.wantErr {
				t.Fatalf("Register error = %v, want error: %t", err, tc.wantErr)
			}
			if !tc.wantErr && (instance.ID != 7 || instance.Public) {
				t.Fatal("registration did not use the catalog visibility")
			}
		})
	}
}

func TestCommunityRegisteredListPaginatesAcrossSources(t *testing.T) {
	var pages int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		if r.URL.Query().Get("registered") != "true" || r.URL.Query().Has("source") {
			t.Error("discovery excludes one instance source")
		}
		count := 100
		source := "external"
		if r.URL.Query().Get("page") == "2" {
			count = 1
			source = "platform"
		}
		items := make([]map[string]any, count)
		for i := range items {
			items[i] = map[string]any{"id": pages*100 + i, "type": "csgclaw", "kind": "runtime", "source": source, "public": true, "relay": RelayRegistration{AgentID: fmt.Sprintf("agt_%024x", pages*100+i)}}
		}
		writeTestJSON(w, 200, map[string]any{"data": items, "total": 101})
	}))
	defer server.Close()
	h := HubCommunity{Credentials: func() (SiteCredentials, error) { return SiteCredentials{Site: server.URL, Token: "caller"}, nil }}
	items, err := h.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 || len(items) != 101 || items[100].Source != "platform" {
		t.Fatal("did not paginate registered platform and external instances")
	}
}

func TestExternalCleanupRefusesPlatformRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("attempted to delete a platform instance")
		}
		writeTestJSON(w, 200, map[string]any{"data": map[string]any{"id": 42, "source": "platform", "type": "csgclaw"}})
	}))
	defer server.Close()
	h := HubCommunity{Credentials: func() (SiteCredentials, error) { return SiteCredentials{Site: server.URL, Token: "owner"}, nil }}
	if err := h.Delete(context.Background(), 42); err == nil {
		t.Fatal("allowed deleting an unverified instance source")
	}
}

func TestPlatformTargetUsesCSGClawName(t *testing.T) {
	t.Setenv("CSGCLAW_NAME", "community-content-id")
	t.Setenv("OPENCSG_SANDBOX_TYPE", "")
	target, err := PlatformTargetFromEnvironment()
	if err != nil || target == nil || target.ContentID != "community-content-id" || target.Type != "csgclaw" {
		t.Fatal("did not use CSGCLAW_NAME as content_id")
	}
	t.Setenv("OPENCSG_SANDBOX_TYPE", "csgclaw-workspace")
	target, err = PlatformTargetFromEnvironment()
	if err != nil || target.Type != "csgclaw-workspace" {
		t.Fatal("lost instance type")
	}
	t.Setenv("CSGCLAW_NAME", "")
	t.Setenv("CSGCLAW_AGENT_AUTO_PUBLISH", "true")
	if _, err = PlatformTargetFromEnvironment(); err == nil {
		t.Fatal("silently treated a missing platform target as external")
	}
}
