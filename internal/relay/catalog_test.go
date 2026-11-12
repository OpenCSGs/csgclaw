package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublicationCatalogPersistsReferencesAndRedactsCredentials(t *testing.T) {
	var available atomic.Bool
	available.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/connect") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var a Agent
		switch r.Method {
		case http.MethodPost:
			var req PublishRequest
			if json.NewDecoder(r.Body).Decode(&req) != nil {
				w.WriteHeader(400)
				return
			}
			a = Agent{ID: "agt_" + req.SourceAgentID, Name: req.Name, Description: req.Description, SourceAgentID: req.SourceAgentID, Visibility: req.Visibility, OwnerID: "owner", PublisherID: "publisher", Executor: "csgclaw"}
			writeTestJSON(w, 201, Publication{Agent: a})
			return
		case http.MethodGet:
			if !available.Load() {
				w.WriteHeader(503)
				return
			}
			a = Agent{ID: "agt_public", Name: "Published agent", Description: "Published description", SourceAgentID: "public", Visibility: "public", OwnerID: "owner", PublisherID: "publisher", Status: "online"}
			writeTestJSON(w, 200, a)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	m, err := NewManager(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err = m.Configure(server.URL, "catalog-test-token"); err != nil {
		t.Fatal(err)
	}
	for _, visibility := range []string{"public", "private"} {
		_, err = m.Publish(context.Background(), PublishRequest{Name: "Published agent", Description: "Published description", Visibility: visibility, SourceAgentID: visibility}, "avatar/pic-2.png")
		if err != nil {
			t.Fatal(err)
		}
	}
	entries := m.Catalog(context.Background())
	if len(entries) != 1 {
		t.Fatalf("catalog count=%d", len(entries))
	}
	entry := entries[0]
	if entry.Agent.ID != "agt_public" || entry.RelayURL != server.URL || entry.ConnectionID == "" || entry.Avatar != "avatar/pic-2.png" || entry.Type != "csgclaw" {
		t.Fatal("missing invocation reference metadata")
	}
	raw, _ := json.Marshal(entries)
	if strings.Contains(string(raw), "catalog-test-token") {
		t.Fatal("catalog leaked credentials")
	}
	connectionID := entry.ConnectionID
	m.Close()
	available.Store(false)
	restored, err := NewManager(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	entries = restored.Catalog(context.Background())
	if len(entries) != 1 || entries[0].ConnectionID != connectionID || entries[0].Agent.Name != "Published agent" || entries[0].Agent.Status != "offline" {
		t.Fatal("restart/outage lost cached reference metadata")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = restored.Unpublish(ctx, "public"); err != nil {
		t.Fatal(err)
	}
	if len(restored.Catalog(ctx)) != 0 {
		t.Fatal("unpublished agent remained in public catalog")
	}
}

func TestLegacyPublicationManifestGetsStableConnectionReference(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "publications.json")
	if err := os.WriteFile(path, []byte(`{"endpoint":"http://127.0.0.1:28080","token":"legacy-test","agents":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := NewManager(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	connectionID := first.manifest.ConnectionID
	first.Close()
	second, err := NewManager(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if connectionID == "" || second.manifest.ConnectionID != connectionID {
		t.Fatal("legacy manifest connection identity was not durably migrated")
	}
}
