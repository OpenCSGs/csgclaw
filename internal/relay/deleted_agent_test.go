package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"csgclaw/internal/apiclient"
)

func TestDeletedAgentRejectsWaitingPublicationAndSurvivesRestart(t *testing.T) {
	var registrations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/agents" {
			registrations.Add(1)
			var req PublishRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			writeTestJSON(w, 201, Publication{Agent: Agent{ID: "agt_" + req.SourceAgentID}})
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()
	root := t.TempDir()
	create := func() *Manager {
		m, err := NewManager(context.Background(), root, apiclient.New(server.URL, "", server.Client()))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	m := create()
	if err := m.Configure(server.URL, "fake-token"); err != nil {
		t.Fatal(err)
	}
	req := PublishRequest{Name: "same-name", SourceAgentID: "deleted-local", Visibility: "public"}
	release, err := m.GuardAgentDeletion(req.SourceAgentID)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 2)
	results := make(chan error, 2)
	go func() { started <- struct{}{}; results <- m.QueuePublication(req) }()
	go func() { started <- struct{}{}; _, err := m.Publish(context.Background(), req); results <- err }()
	<-started
	<-started
	if err = m.CompleteAgentDeletion(req.SourceAgentID); err != nil {
		release()
		t.Fatal(err)
	}
	release()
	for range 2 {
		if err = <-results; !errors.Is(err, ErrLocalAgentDeleted) {
			t.Fatalf("accepted a request waiting behind deletion: %v", err)
		}
	}
	if registrations.Load() != 0 || len(m.manifest.AutoPublish) != 0 {
		t.Fatal("deleted agent produced a remote registration or durable queue")
	}
	m.Close()
	m = create()
	defer m.Close()
	if err = m.QueuePublication(req); !errors.Is(err, ErrLocalAgentDeleted) {
		t.Fatalf("restart forgot deleted ID: %v", err)
	}
	if _, err = m.Publish(context.Background(), req); !errors.Is(err, ErrLocalAgentDeleted) {
		t.Fatalf("restart allowed manual publication: %v", err)
	}
	req.SourceAgentID = "new-local"
	if err = m.QueuePublication(req); err != nil {
		t.Fatalf("new ID with the same name was blocked: %v", err)
	}
	req.SourceAgentID = "new-manual"
	if _, err = m.Publish(context.Background(), req); err != nil {
		t.Fatalf("new manual publication was blocked: %v", err)
	}
}

func TestAbortedDeletionDoesNotRetireAgent(t *testing.T) {
	local := httptest.NewServer(http.NotFoundHandler())
	defer local.Close()
	m, err := NewManager(context.Background(), t.TempDir(), apiclient.New(local.URL, "", local.Client()))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	release, err := m.GuardAgentDeletion("existing")
	if err != nil {
		t.Fatal(err)
	}
	// Local deletion failed, so completion must not be called.
	release()
	if err = m.QueuePublication(PublishRequest{Name: "existing", SourceAgentID: "existing", Visibility: "public"}); err != nil {
		t.Fatalf("failed deletion retired a live ID: %v", err)
	}
}

func TestDeletionSaveFailureStillRejectsWaitingRequests(t *testing.T) {
	m, err := NewManager(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	release, err := m.GuardAgentDeletion("deleted")
	if err != nil {
		t.Fatal(err)
	}
	invalidRoot := filepath.Join(t.TempDir(), "file")
	if err = os.WriteFile(invalidRoot, []byte("not a directory"), 0600); err != nil {
		release()
		t.Fatal(err)
	}
	m.root = invalidRoot
	if err = m.CompleteAgentDeletion("deleted"); err == nil {
		release()
		t.Fatal("expected persistence failure")
	}
	release()
	req := PublishRequest{Name: "deleted", SourceAgentID: "deleted", Visibility: "public"}
	if err = m.QueuePublication(req); !errors.Is(err, ErrLocalAgentDeleted) {
		t.Fatalf("disk failure reopened queueing: %v", err)
	}
	if _, err = m.Publish(context.Background(), req); !errors.Is(err, ErrLocalAgentDeleted) {
		t.Fatalf("disk failure reopened publication: %v", err)
	}
}
