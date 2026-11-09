package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/enginetest"
	"csgclaw/internal/apiclient"
	"csgclaw/internal/im"
	"csgclaw/internal/relay"
)

func TestArchivedAgentDeletionRequiresPublicationCancellation(t *testing.T) {
	engine := enginetest.NewMemoryClient(agentengine.Agent{ID: "agent-worker", Spec: agentengine.AgentSpec{Name: "worker", Role: agentengine.AgentRoleWorker, Runtime: agentengine.RuntimeSpec{Adapter: "codex"}}})
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{{ID: im.AdminUserID, Name: "admin", Role: "admin"}}})
	h := NewHandler(AgentServices{}, engine, imSvc, nil, nil, nil, nil)
	m, err := relay.NewManager(context.Background(), t.TempDir(), apiclient.New("http://127.0.0.1:1", "", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	h.relayManager = m
	routes := h.Routes()
	request := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rec.Code != want {
			t.Fatalf("%s %s = %d: %s", method, path, rec.Code, rec.Body.String())
		}
		return rec
	}
	request(http.MethodPost, "/api/v1/agents/agent-worker/archive", "", 200)
	target := &relay.CommunityTarget{Type: "csgclaw", ContentID: "sandbox-slot"}
	if err = m.QueuePublication(relay.PublishRequest{Name: "worker", SourceAgentID: "agent-worker", Visibility: "public"}, target); err != nil {
		t.Fatal(err)
	}
	rec := request(http.MethodDelete, "/api/v1/agents/agent-worker", `{"name":"worker"}`, 409)
	if !strings.Contains(rec.Body.String(), "relay_unpublish_required") {
		t.Fatal(rec.Body.String())
	}
	if _, err = engine.Agents().Get(context.Background(), "agent-worker", agentengine.AgentGetOptions{}); err != nil {
		t.Fatal("blocked deletion removed local agent")
	}
	if err = m.Unpublish(context.Background(), "agent-worker"); err != nil {
		t.Fatal(err)
	}
	request(http.MethodDelete, "/api/v1/agents/agent-worker", `{"name":"wrong"}`, 400)
	replacement := relay.PublishRequest{Name: "replacement", SourceAgentID: "agent-replacement", Visibility: "public"}
	if err = m.QueuePublication(replacement, target); err == nil {
		t.Fatal("failed deletion released the existing platform target")
	}
	request(http.MethodDelete, "/api/v1/agents/agent-worker", `{"name":"worker"}`, 204)
	if err = m.QueuePublication(relay.PublishRequest{Name: "worker", SourceAgentID: "agent-worker", Visibility: "public"}); !errors.Is(err, relay.ErrLocalAgentDeleted) {
		t.Fatalf("deleted local ID could be queued again: %v", err)
	}
	if err = m.QueuePublication(replacement, target); err != nil {
		t.Fatalf("successful deletion retained platform binding: %v", err)
	}
}
