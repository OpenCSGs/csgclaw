package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestChatResolvesServerCredentialsAndPreservesContinuation(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer server-only-token" {
			t.Error("wrong relay credentials")
		}
		if r.Method == http.MethodPost {
			if r.URL.Path != "/v1/responses" || r.Header.Get("Idempotency-Key") != "turn-key" {
				t.Error("invalid invocation routing")
			}
			var req CallRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Model != "agt_test" || req.PreviousResponseID != "resp_previous" || req.Stream {
				t.Error("lost target or continuation")
			}
		}
		writeTestJSON(w, 200, Response{ID: "resp_done", Model: "agt_test", Status: "completed", Output: []Output{{Content: []Content{{Type: "output_text", Text: "reply"}}}}})
	}))
	defer server.Close()
	m := &Manager{manifest: relayManifest{Endpoint: server.URL, Token: "server-only-token", ConnectionID: "saved-connection", Agents: map[string]PublishRequest{"agt_test": {Visibility: "public"}}}}
	response, err := m.Chat(context.Background(), "agt_test", "saved-connection", "turn-key", "hello", "resp_previous")
	if err != nil || response.Text() != "reply" {
		t.Fatalf("chat failed: %v", err)
	}
	if _, err = m.ChatResponse(context.Background(), "agt_test", "saved-connection", "resp_done"); err != nil {
		t.Fatal(err)
	}
	for _, connection := range []string{"", "another-identity"} {
		if _, err = m.Chat(context.Background(), "agt_test", connection, "key", "hello", ""); !errors.Is(err, ErrChatReference) {
			t.Fatal("accepted untrusted connection reference")
		}
	}
	if _, err = m.Chat(context.Background(), "unknown", "saved-connection", "key", "hello", ""); !errors.Is(err, ErrChatReference) {
		t.Fatal("accepted unknown remote agent")
	}
	if calls != 2 {
		t.Fatal("invalid references reached relay")
	}
}

func TestAttachmentChatReplaysCompletedRequestAfterExecutorDisconnects(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			t.Errorf("replay must not depend on executor availability: %s %s", r.Method, r.URL.Path)
			writeTestJSON(w, http.StatusOK, Agent{ID: "agt_test", Status: "offline"})
			return
		}
		calls++
		if r.Header.Get("Idempotency-Key") != "original-key" {
			t.Error("replay key changed")
		}
		if calls == 1 {
			// Relay completed the request, but its response was lost in transit.
			_, _ = w.Write([]byte("{"))
			return
		}
		writeTestJSON(w, http.StatusOK, Response{ID: "resp_saved", Model: "agt_test", Status: "completed"})
	}))
	defer server.Close()
	m := &Manager{manifest: relayManifest{Endpoint: server.URL, Token: "token", ConnectionID: "connection",
		Agents: map[string]PublishRequest{"agt_test": {Visibility: "public"}}}}
	file := File{ID: "file_input"}
	if _, err := m.Chat(t.Context(), "agt_test", "connection", "original-key", "read file", "", file); err == nil {
		t.Fatal("expected the first response to be unreadable")
	}
	result, err := m.Chat(t.Context(), "agt_test", "connection", "original-key", "read file", "", file)
	if err != nil || result.ID != "resp_saved" || result.Status != "completed" || calls != 2 {
		t.Fatalf("replay = %+v, %v, calls=%d", result, err, calls)
	}
}

func TestAttachmentPreflightDeadlineWhilePublicationWaitsForRelay(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/agents" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Read the request before waiting so cancellation closes the connection.
		var req PublishRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	m, err := NewManager(t.Context(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.Configure(server.URL, "token"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := m.Publish(ctx, PublishRequest{Name: "test", SourceAgentID: "local", Visibility: "public"})
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("publication did not reach Relay")
	}
	preflightCtx, stop := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer stop()
	checked := make(chan error, 1)
	go func() { checked <- m.CheckAttachmentSupport(preflightCtx, testRelayID, "connection") }()
	select {
	case err := <-checked:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("preflight = %v, want deadline exceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("preflight blocked behind publication beyond its deadline")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("publication unexpectedly succeeded after cancellation")
	}
	// Cancellation must not leave a waiter holding or later acquiring the mutex.
	if !m.mu.TryLock() {
		t.Fatal("manager mutex leaked after canceled invocation")
	}
	m.mu.Unlock()
}

func TestAttachmentPreflightDeadlineWhileAnotherInvocationWaitsForCatalog(t *testing.T) {
	firstEntered := make(chan struct{})
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/instances" {
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if reads.Add(1) == 1 {
			close(firstEntered)
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	credentials := func() (SiteCredentials, error) {
		return SiteCredentials{Site: server.URL, User: "user", Token: "token"}, nil
	}
	manager, err := NewManager(t.Context(), t.TempDir(), nil, WithCommunity(HubCommunity{Credentials: credentials}, credentials))
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	connection, err := manager.connectionReference()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	firstDone := make(chan error, 1)
	go func() {
		_, err := manager.Chat(ctx, testRelayID, connection, "first", "hello", "")
		firstDone <- err
	}()
	select {
	case <-firstEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("first invocation did not reach Catalog")
	}
	preflightCtx, stop := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer stop()
	secondDone := make(chan error, 1)
	go func() { secondDone <- manager.CheckAttachmentSupport(preflightCtx, testRelayID, connection) }()
	select {
	case err := <-secondDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("preflight = %v, want deadline exceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("preflight waited for another invocation beyond its deadline")
	}
	if reads.Load() != 2 {
		t.Fatalf("Catalog reads = %d, want two independent requests", reads.Load())
	}
	cancel()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("first invocation = %v, want canceled", err)
	}
}
