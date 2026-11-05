package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/relay"
)

func TestRelayChatAPIHandlesUnconfirmedRequestAndScopesPolling(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/agents":
			_ = json.NewEncoder(w).Encode(relay.Publication{Agent: relay.Agent{ID: "agt_test"}})
		case r.URL.Path == "/v1/responses" && r.Method == http.MethodPost:
			w.WriteHeader(409)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": relay.ProtocolError{Code: "response_in_progress", ResponseID: "resp_existing"}})
		case r.URL.Path == "/v1/responses/resp_other":
			_ = json.NewEncoder(w).Encode(relay.Response{ID: "resp_other", Model: "another_agent", Status: "completed"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer remote.Close()
	m, err := relay.NewManager(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err = m.Configure(remote.URL, "server-only-test-token"); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Publish(context.Background(), relay.PublishRequest{SourceAgentID: "local", Visibility: "public"}); err != nil {
		t.Fatal(err)
	}
	ref := m.Catalog(context.Background())[0].ConnectionID
	h := &Handler{relayManager: m}
	request := func(method, body, responseID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/relay/agents/agt_test/responses?connection_id="+ref, strings.NewReader(body))
		req.SetPathValue("id", "agt_test")
		req.SetPathValue("response_id", responseID)
		recorder := httptest.NewRecorder()
		h.handleRelayChat(recorder, req)
		return recorder
	}
	body := `{"connection_id":"` + ref + `","input":"hello","key":"stable-key"}`
	recorder := request(http.MethodPost, body, "")
	if recorder.Code != 202 || !strings.Contains(recorder.Body.String(), "resp_existing") {
		t.Fatal("lost unconfirmed request identity")
	}
	if request(http.MethodGet, "", "resp_other").Code != 409 {
		t.Fatal("polling accepted another agent's response")
	}
	if request(http.MethodPost, `{"connection_id":"wrong","input":"hello","key":"key"}`, "").Code != 409 {
		t.Fatal("accepted wrong connection")
	}
	if request(http.MethodPost, `{"input":"hello","key":"key","relay_url":"http://untrusted"}`, "").Code != 400 {
		t.Fatal("accepted browser-supplied relay URL")
	}
	if strings.Contains(recorder.Body.String(), "server-only-test-token") {
		t.Fatal("credential leaked")
	}
}
