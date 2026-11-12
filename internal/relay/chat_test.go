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
