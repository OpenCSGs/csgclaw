package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apiclient"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/relay"
)

func TestRelayCatalogAuthenticationError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"missing login", relay.ErrSiteLoginRequired, http.StatusUnauthorized, "relay_login_required"},
		{"credential store failure", fmt.Errorf("credential store unavailable"), http.StatusBadGateway, "relay_catalog_failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			credentials := func() (relay.SiteCredentials, error) {
				return relay.SiteCredentials{}, tc.err
			}
			manager, err := relay.NewManager(t.Context(), t.TempDir(), nil, relay.WithCommunity(relay.HubCommunity{}, credentials))
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			h := &Handler{relayManager: manager}
			response := httptest.NewRecorder()
			h.handleRelayCatalog(response, httptest.NewRequest(http.MethodGet, "/api/v1/relay/agents", nil))
			var body agentSessionErrorEnvelope
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.status || body.Error.Code != tc.code {
				t.Fatalf("got status %d and error code %q, want %d and %q", response.Code, body.Error.Code, tc.status, tc.code)
			}
		})
	}
}

// Set CSGCLAW_TEST_RELAY_URL/TOKEN to exercise the same flow against a running
// CSGRelay with real NATS. No credentials or response contents are logged.
func TestRelayPublicationEndToEnd(t *testing.T) {
	endpoint, token := os.Getenv("CSGCLAW_TEST_RELAY_URL"), os.Getenv("CSGCLAW_TEST_RELAY_TOKEN")
	if endpoint == "" || token == "" {
		t.Skip("live CSGRelay not configured; set CSGCLAW_TEST_RELAY_URL and CSGCLAW_TEST_RELAY_TOKEN")
	}
	var mu sync.Mutex
	calls := map[string]int{}
	engine := &fakeSessionEngine{run: func(_ context.Context, id string, req agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		mu.Lock()
		calls[id]++
		mu.Unlock()
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: id + ":" + req.Input[0].Text, Dispatched: true}
	}}
	h, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-relay-alpha", "Alpha"), sessionCodexAgent("agent-relay-beta", "Beta")}, engine, "")
	local := httptest.NewServer(h.Routes())
	defer local.Close()
	root := t.TempDir()
	newManager := func() *relay.Manager {
		m, err := relay.NewManager(context.Background(), root, apiclient.New(local.URL, "", local.Client()))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	h.relayManager = newManager()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = h.relayManager.Unpublish(ctx, "agent-relay-alpha")
		_ = h.relayManager.Unpublish(ctx, "agent-relay-beta")
	}()
	defer func() { h.relayManager.Close() }()
	request := func(method, id string, body any) relay.PublicationStatus {
		t.Helper()
		data, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, local.URL+"/api/v1/agents/"+id+"/relay", bytes.NewReader(data))
		resp, err := local.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			var detail struct {
				Error struct {
					Code    string
					Message string
				}
			}
			_ = json.NewDecoder(resp.Body).Decode(&detail)
			t.Fatalf("relay API returned %d: %s %s", resp.StatusCode, detail.Error.Code, detail.Error.Message)
		}
		var status relay.PublicationStatus
		if json.NewDecoder(resp.Body).Decode(&status) != nil {
			t.Fatal("invalid status")
		}
		return status
	}
	catalog := func() []relay.CatalogEntry {
		t.Helper()
		resp, err := local.Client().Get(local.URL + "/api/v1/relay/agents")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var result struct {
			Data []relay.CatalogEntry `json:"data"`
		}
		if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&result) != nil {
			t.Fatal("invalid local relay catalog response")
		}
		raw, _ := json.Marshal(result)
		if strings.Contains(string(raw), token) {
			t.Fatal("catalog exposed credentials")
		}
		return result.Data
	}
	waitOnline := func(id string) relay.PublicationStatus {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			s := request("GET", id, nil)
			if s.Agent != nil && s.Agent.Status == "online" {
				return s
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("published agent did not become online")
		return relay.PublicationStatus{}
	}
	request("POST", "agent-relay-alpha", map[string]string{"endpoint": endpoint, "token": token})
	request("POST", "agent-relay-beta", map[string]string{})
	alpha, beta := waitOnline("agent-relay-alpha"), waitOnline("agent-relay-beta")
	entries := catalog()
	if len(entries) != 2 || entries[0].ConnectionID == "" || entries[0].RelayURL != endpoint {
		t.Fatal("publication missing from local debug catalog")
	}
	connectionID := entries[0].ConnectionID
	if alpha.Agent.ID == beta.Agent.ID {
		t.Fatal("agents share a relay identity")
	}
	invoke := func(remote, key, input, previous string) relay.Response {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"connection_id": connectionID, "input": input, "previous_response_id": previous, "key": key})
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, local.URL+"/api/v1/relay/agents/"+remote+"/responses", bytes.NewReader(body))
		resp, err := local.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var result relay.Response
		if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&result) != nil || result.Status != "completed" {
			t.Fatalf("local cloud chat proxy failed: HTTP %d, status %s", resp.StatusCode, result.Status)
		}
		return result
	}
	key := time.Now().Format("150405.000000000")
	first := invoke(alpha.Agent.ID, key+"-1", "hello", "")
	if first.Text() != "agent-relay-alpha:hello" {
		t.Fatal("response routed to wrong agent")
	}
	replay := invoke(alpha.Agent.ID, key+"-1", "hello", "")
	if replay.ID != first.ID {
		t.Fatal("idempotency failed")
	}
	invoke(alpha.Agent.ID, key+"-2", "continue", first.ID)
	if invoke(beta.Agent.ID, key+"-3", "hello", "").Text() != "agent-relay-beta:hello" {
		t.Fatal("response routed to wrong agent")
	}
	// Restart uses private persisted credentials and desired publications.
	h.relayManager.Close()
	h.relayManager = newManager()
	alpha = waitOnline("agent-relay-alpha")
	if entries = catalog(); len(entries) != 2 || entries[0].ConnectionID != connectionID {
		t.Fatal("catalog connection reference changed after restart")
	}
	invoke(alpha.Agent.ID, key+"-4", "after restart", "")
	info, err := os.Stat(filepath.Join(root, "publications.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("manifest is not private")
	}
	public, _ := json.Marshal(request("GET", "agent-relay-alpha", nil))
	if strings.Contains(string(public), token) {
		t.Fatal("status exposed credential")
	}
	// Exercise the ordinary task API, not only the standalone cloud chat proxy.
	taskRequest := func(path string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		h.Routes().ServeHTTP(recorder, req)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("task API %s returned %d: %s", path, recorder.Code, recorder.Body.String())
		}
		return recorder
	}
	recorder := taskRequest("/api/v1/rooms", apitypes.CreateRoomRequest{Title: "Cloud task", CreatorID: im.AdminUserID, ClientRequestID: key + "-task", CloudAgent: &apitypes.CloudAgentReference{AgentID: alpha.Agent.ID, ConnectionID: connectionID}})
	var taskRoom apitypes.Room
	if json.Unmarshal(recorder.Body.Bytes(), &taskRoom) != nil || taskRoom.CloudAgent == nil || !taskRoom.TaskSession {
		t.Fatal("missing cloud task metadata")
	}
	for turn := 1; turn <= 2; turn++ {
		body := map[string]any{"room_id": taskRoom.ID, "sender_id": im.AdminUserID, "content": "task turn", "client_message_id": fmt.Sprintf("%s-turn-%d", key, turn)}
		taskRequest("/api/v1/messages", body)
		taskRequest("/api/v1/messages", body)
		deadline := time.Now().Add(15 * time.Second)
		for {
			room, _ := h.im.Room(taskRoom.ID)
			replies := 0
			for _, message := range room.Messages {
				if message.SenderID == taskRoom.CloudAgent.UserID {
					replies++
					if message.Content != "agent-relay-alpha:task turn" || message.Metadata["relay_response_id"] == nil {
						t.Fatalf("cloud task reply failed: %s", message.Content)
					}
				}
			}
			if replies == turn {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("cloud task reply timed out")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	request("DELETE", "agent-relay-alpha", nil)
	if len(catalog()) != 1 {
		t.Fatal("unpublish did not remove temporary catalog entry")
	}
	if request("GET", "agent-relay-alpha", nil).Published {
		t.Fatal("unpublish retained desired publication")
	}
	// An immediate republish must start a fresh executor after old cleanup.
	request("POST", "agent-relay-alpha", map[string]string{})
	alpha = waitOnline("agent-relay-alpha")
	invoke(alpha.Agent.ID, key+"-5", "republished", "")
	request("DELETE", "agent-relay-alpha", nil)
	request("DELETE", "agent-relay-beta", nil)
	mu.Lock()
	defer mu.Unlock()
	if calls["agent-relay-alpha"] != 6 || calls["agent-relay-beta"] != 1 {
		t.Fatalf("unexpected execution counts: %v", calls)
	}
}
