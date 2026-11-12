package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentmemory"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	agentruntime "csgclaw/internal/runtime"
)

func TestForgetMemoryHTTPPreservesOtherQueuedMessages(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "agents.json")
	if err := writeSeededAgents(statePath, []agent.Agent{{ID: "agent-alice", Name: "alice", RuntimeKind: agent.RuntimeKindCodex, Role: agent.RoleWorker, Status: string(agentruntime.StateStopped)}}); err != nil {
		t.Fatal(err)
	}
	controller, err := agent.NewController(config.ModelConfig{}, config.ServerConfig{}, "", statePath, agent.WithRuntime(fakeMemoryCompatRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: agent.RuntimeKindCodex}}))
	if err != nil {
		t.Fatal(err)
	}
	store, err := agentmemory.Open(filepath.Join(root, "agents"), filepath.Join(root, "state.json"), "local")
	if err != nil {
		t.Fatal(err)
	}
	controller.SetManagedMemory(store.DeleteAgent, store.SeedTemplate)
	engine := agentengine.New(controller)
	engine.SetMemory(store)
	chat := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{{ID: im.AdminUserID, Role: "admin"}, {ID: "agent-alice", Role: "worker"}}, Rooms: []im.Room{{ID: "private", IsDirect: true, Members: []string{im.AdminUserID, "agent-alice"}, Messages: []im.Message{{ID: "A", SenderID: im.AdminUserID, Content: "Forget A"}, {ID: "B", SenderID: im.AdminUserID, Content: "Keep B"}}}}})
	h := &Handler{svc: controller, agentEngine: engine, serverNoAuth: true, im: chat}
	h.SetAgentMemory(store, "local")
	server := httptest.NewServer(h.Routes())
	defer server.Close()
	scope := agentmemory.Scope{AgentID: "agent-alice", UserID: im.AdminUserID}
	ticket, _, _, err := store.Snapshot(scope, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"A", "B"} {
		if err := store.Enqueue(ticket, id, agentmemory.Message{Source: agentmemory.Source{RoomID: "private", MessageID: id}, Text: "Preference " + id}); err != nil {
			t.Fatal(err)
		}
	}
	req, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/agents/agent-alice/memory/sources/private/A", nil)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 {
		t.Fatalf("forget: %d %s", response.StatusCode, body)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker := agentmemory.Worker{Store: store, Extract: func(_ context.Context, _ string, input agentmemory.Extraction) ([]agentmemory.Change, error) {
		return []agentmemory.Change{{Kind: "preference", Text: input.Messages[0].Text, SourceMessageID: input.Messages[0].MessageID}}, nil
	}}
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(ctx) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		doc, err := store.Get(scope)
		if err != nil {
			t.Fatal(err)
		}
		if doc.Status == "idle" {
			if !strings.Contains(doc.Content, "Preference B") || strings.Contains(doc.Content, "Preference A") {
				t.Fatalf("forget discarded unrelated learning: %+v", doc)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("queue did not drain: %+v", doc)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
