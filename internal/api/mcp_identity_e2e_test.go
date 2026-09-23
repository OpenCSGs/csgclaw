package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/auth"
	"csgclaw/internal/im"
	"csgclaw/internal/mcp"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/codex"
	webui "csgclaw/web"
)

type identityMCPRuntime struct{ snapshotMCPServersRuntime }

func (*identityMCPRuntime) ValidateMCPServers(ctx context.Context, snapshot agentruntime.MCPServersSnapshot) error {
	return codex.New(codex.Dependencies{}).ValidateMCPServers(ctx, snapshot)
}

func TestMCPIdentityBrowserFixture(t *testing.T) {
	ready := os.Getenv("CSGCLAW_MCP_IDENTITY_BROWSER_READY")
	if ready == "" {
		t.Skip("set CSGCLAW_MCP_IDENTITY_BROWSER_READY")
	}
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(stubAuthStatus(func(*http.Request) (auth.Status, error) { return auth.Status{}, nil }))
	rt := &identityMCPRuntime{snapshotMCPServersRuntime: snapshotMCPServersRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: agent.RuntimeKindCodex}}}
	controller := mustNewSeededServiceWithOptions(t, []agent.Agent{{ID: "agent-identity", Name: "MCP验收", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex, RuntimeID: "rt-identity", Status: "running", ProfileComplete: true, MCPServers: map[string]any{}}}, agent.WithRuntime(rt))
	bus := im.NewBus()
	imService := im.NewServiceFromBootstrapWithBus(im.Bootstrap{CurrentUserID: "user-admin", Users: []im.User{{ID: "user-admin", Name: "验收用户", Role: "admin"}}}, bus)
	h := NewHandlerWithAuth(AgentServices{Records: controller, Workspace: controller.Workspace(), Models: controller.Models(), Runtime: controller}, agentengine.New(controller), imService, bus, nil, nil, nil, "", true)
	h.mcp = mcp.NewService()
	participants := participant.NewService(participant.NewMemoryStore(nil), participant.WithAgentEngine(h.agentEngine), participant.WithIMService(imService))
	h.SetParticipantService(participants)
	if _, err := participants.EnsureBootstrapAdmin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := participants.Create(context.Background(), participant.CreateRequest{ID: "identity", Channel: participant.ChannelCSGClaw, Type: participant.TypeAgent, Name: "MCP验收", AgentBinding: participant.AgentBindingSpec{Mode: participant.BindingModeReuse, AgentID: "agent-identity"}}); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{}, 1)
	router := h.Routes()
	router.Post("/_e2e/stop", func(w http.ResponseWriter, _ *http.Request) {
		select {
		case done <- struct{}{}:
		default:
		}
		w.WriteHeader(204)
	})
	router.Handle("/*", webui.Handler())
	server := httptest.NewServer(router)
	defer server.Close()
	raw, _ := json.Marshal(map[string]string{"url": server.URL, "agent_id": "agent-identity"})
	if err := os.WriteFile(ready, raw, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Minute):
		t.Fatal("browser fixture timed out")
	}
}

// Exercise the user-facing catalog and Agent endpoints with the real Codex
// validator, real Engine persistence and isolated local state.
func TestMCPIdentityHTTPFlow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rt := &identityMCPRuntime{snapshotMCPServersRuntime: snapshotMCPServersRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: agent.RuntimeKindCodex}}}
	controller := mustNewSeededServiceWithOptions(t, []agent.Agent{{ID: "agent-mcp-identity", Name: "identity", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex, RuntimeID: "rt-identity", Status: "running", ProfileComplete: true, MCPServers: map[string]any{}}}, agent.WithRuntime(rt))
	h := &Handler{svc: controller, mcp: mcp.NewService(), agentEngine: agentengine.New(controller), workspace: controller.Workspace(), agentModels: controller.Models(), agentRuntime: controller}
	server := httptest.NewServer(h.Routes())
	defer server.Close()
	request := func(method, path string, body any, status int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		responseBody, _ := io.ReadAll(resp.Body)
		var value map[string]any
		if err := json.Unmarshal(responseBody, &value); err != nil {
			t.Fatalf("%s %s: status %d: %s", method, path, resp.StatusCode, responseBody)
		}
		if resp.StatusCode != status {
			t.Fatalf("%s %s: status %d: %#v", method, path, resp.StatusCode, value)
		}
		return value
	}
	for _, label := range []string{"必应搜索中文", "Web Fetch"} {
		state := request(http.MethodPost, "/api/v1/mcp-servers", map[string]any{"name": label, "config": map[string]any{"url": "https://mcp.example.test/mcp"}}, http.StatusCreated)
		servers := state["mcpServers"].(map[string]any)
		id := ""
		for key, raw := range servers {
			config := raw.(map[string]any)
			if key == label || config["display_name"] == label {
				id = key
			}
		}
		if id == "" {
			t.Fatal("created MCP missing")
		}
		request(http.MethodPost, "/api/v1/agents/agent-mcp-identity/mcp-servers:batchAdd", map[string]any{"names": []string{id}}, http.StatusOK)
		renamed := label + " 已改名"
		state = request(http.MethodPut, "/api/v1/mcp-servers/"+id, map[string]any{"name": renamed, "config": servers[id]}, http.StatusOK)
		entry, ok := state["mcpServers"].(map[string]any)[id].(map[string]any)
		if !ok || entry["display_name"] != renamed {
			t.Fatalf("rename changed identity: %#v", state)
		}
		request(http.MethodPost, "/api/v1/agents/agent-mcp-identity/mcp-servers:batchAdd", map[string]any{"names": []string{id}}, http.StatusOK)
		saved, _ := controller.Agent("agent-mcp-identity")
		if saved.MCPServers[id].(map[string]any)["display_name"] != renamed {
			t.Fatal("Agent lost display name")
		}
		if _, ok := rt.servers[id].(map[string]any)["display_name"]; ok {
			t.Fatal("runtime received presentation metadata")
		}
		request(http.MethodPost, "/api/v1/agents/agent-mcp-identity/mcp-servers:batchDelete", map[string]any{"names": []string{id}}, http.StatusOK)
		request(http.MethodDelete, "/api/v1/mcp-servers/"+id, nil, http.StatusOK)
	}
}
