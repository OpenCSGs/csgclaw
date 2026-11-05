package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"csgclaw/internal/apiclient"
	"csgclaw/internal/apitypes"
)

func TestSiteCredentialsPreferLoginThenManagedEnvironment(t *testing.T) {
	t.Setenv("CSGHUB_API_BASE_URL", "https://env.example.test")
	t.Setenv("CSGHUB_USER_TOKEN", "env-token")
	t.Setenv("CSGHUB_ACCESS_TOKEN", "")
	login := SiteCredentials{Site: "https://login.example.test", Token: "login-token", User: "uuid-one"}
	got, err := loadSiteCredentials(func() (SiteCredentials, error) { return login, nil })
	if err != nil || got != login {
		t.Fatal("environment displaced interactive credentials")
	}
	got, err = loadSiteCredentials(func() (SiteCredentials, error) { return SiteCredentials{}, ErrSiteLoginRequired })
	if err != nil || got.Site != "https://env.example.test" || got.Token != "env-token" || got.User == "" || strings.Contains(got.User, "env-token") {
		t.Fatal("invalid environment fallback")
	}
	first := got
	got, err = loadSiteCredentials(func() (SiteCredentials, error) { return SiteCredentials{}, ErrSiteLoginRequired })
	if err != nil || got.User != first.User {
		t.Fatal("unstable environment identity")
	}
	broken := errors.New("unreadable login store")
	if _, err = loadSiteCredentials(func() (SiteCredentials, error) { return SiteCredentials{}, broken }); !errors.Is(err, broken) {
		t.Fatal("silently switched accounts after store error")
	}
	t.Setenv("CSGHUB_USER_TOKEN", "")
	if _, err = loadSiteCredentials(func() (SiteCredentials, error) { return SiteCredentials{}, ErrSiteLoginRequired }); !errors.Is(err, ErrSiteLoginRequired) {
		t.Fatal("accepted missing credentials")
	}
}

func TestAutomaticPublicationWaitsForReadinessAndRestoresIntent(t *testing.T) {
	var ready atomic.Bool
	var registrations atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/agents/"):
			state := "starting"
			if ready.Load() {
				state = "running"
			}
			writeTestJSON(w, 200, apitypes.Agent{ID: "local", Status: state, Runtime: apitypes.AgentRuntime{Name: "codex", State: state}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/agents":
			registrations.Add(1)
			writeTestJSON(w, 201, Publication{Agent: Agent{ID: "agt_local", SourceAgentID: "local", Visibility: "public"}})
		case strings.HasSuffix(r.URL.Path, "/connect"):
			w.WriteHeader(404)
		case r.Method == http.MethodDelete:
			w.WriteHeader(204)
		default:
			writeTestJSON(w, 200, Agent{ID: "agt_local"})
		}
	}))
	defer server.Close()
	root := t.TempDir()
	makeManager := func() *Manager {
		m, err := NewManager(context.Background(), root, apiclient.New(server.URL, "", server.Client()))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	m := makeManager()
	if err := m.Configure(server.URL, "fixture-token"); err != nil {
		t.Fatal(err)
	}
	req := PublishRequest{SourceAgentID: "local", Name: "Startup", Visibility: "public", Executor: "csgclaw"}
	if err := m.QueuePublication(req); err != nil {
		t.Fatal(err)
	}
	if !m.Status(context.Background(), "local").Publishing {
		t.Fatal("missing pending status")
	}
	m.Close()
	if registrations.Load() != 0 {
		t.Fatal("registered before ready")
	}
	data, err := os.ReadFile(filepath.Join(root, "publications.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state relayManifest
	if json.Unmarshal(data, &state) != nil || state.AutoPublish["local"].Name != "Startup" {
		t.Fatal("intent not persisted")
	}
	ready.Store(true)
	m = makeManager()
	defer m.Close()
	waitFor(t, func() bool { return m.Status(context.Background(), "local").Published })
	waitFor(t, func() bool { return !m.Status(context.Background(), "local").Publishing })
	if err = m.QueuePublication(req); err != nil {
		t.Fatal(err)
	}
	if m.Status(context.Background(), "local").Publishing {
		t.Fatal("completed publication queued twice")
	}
	if err = m.Unpublish(context.Background(), "local"); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	count := len(m.manifest.AutoPublish)
	m.mu.Unlock()
	if count != 0 {
		t.Fatal("unpublish retained queued intent")
	}
}

func TestQueuedPublicationCancellationSurvivesRestart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("cancelled queue reached remote publication")
		}
		writeTestJSON(w, 200, apitypes.Agent{ID: "local", Status: "stopped", Runtime: apitypes.AgentRuntime{Name: "codex", State: "stopped"}})
	}))
	defer server.Close()
	root := t.TempDir()
	local := apiclient.New(server.URL, "", server.Client())
	m, err := NewManager(context.Background(), root, local)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.QueuePublication(PublishRequest{Name: "Startup", SourceAgentID: "local", Visibility: "public"}); err != nil {
		t.Fatal(err)
	}
	if err = m.Unpublish(context.Background(), "local"); err != nil {
		t.Fatal(err)
	}
	m.Close()
	m, err = NewManager(context.Background(), root, local)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if m.Status(context.Background(), "local").Publishing || len(m.manifest.AutoPublish) != 0 {
		t.Fatal("cancelled queue restored")
	}
}
