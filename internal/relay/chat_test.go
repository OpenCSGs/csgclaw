package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
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
