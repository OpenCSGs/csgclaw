package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"csgclaw/internal/apitypes"
	csgclawchannel "csgclaw/internal/channel/csgclaw"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	webui "csgclaw/web"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestConnectorsBrowserE2EFixture serves the real HTTP handlers and built Web UI with
// isolated Agents and an authenticated local MCP service for headless UI QA.
func TestConnectorsBrowserE2EFixture(t *testing.T) {
	readyPath := os.Getenv("CSGCLAW_CONNECTORS_BROWSER_E2E_READY")
	if readyPath == "" {
		t.Skip("set CSGCLAW_CONNECTORS_BROWSER_E2E_READY for interactive headless verification")
	}
	h, alice, bob, _, _ := newAppPlatformAuthFixture(t)
	h.serverNoAuth = false
	h.im = im.NewService()
	h.imBus = im.NewBus()
	h.csgclaw = csgclawchannel.NewService(h.im)
	h.participantBridge = im.NewParticipantBridge("test-admin-secret")
	participants := participant.NewService(participant.NewMemoryStore(nil), participant.WithAgentEngine(h.agentEngine), participant.WithIMService(h.im))
	h.SetParticipantService(participants)
	if _, err := participants.EnsureBootstrapAdmin(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, a := range []struct{ ID, Name string }{{alice.ID, alice.Name}, {bob.ID, bob.Name}} {
		if _, err := participants.Create(context.Background(), participant.CreateRequest{ID: a.Name, Channel: participant.ChannelCSGClaw, Type: participant.TypeAgent, Name: a.Name, AgentBinding: participant.AgentBindingSpec{Mode: participant.BindingModeReuse, AgentID: a.ID}}); err != nil {
			t.Fatal(err)
		}
	}
	upstream := mcp.NewServer(&mcp.Implementation{Name: "app-e2e-upstream", Version: "1"}, nil)
	upstream.AddTool(&mcp.Tool{Name: "echo", Description: "Echo a message from the App E2E service.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"message": map[string]any{"type": "string"}}}, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "app-e2e-ok"}}}, nil
	})
	upstreamHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return upstream }, nil)
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer app-e2e-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		upstreamHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(mcpServer.Close)
	done := make(chan struct{}, 1)
	router := h.Routes()
	router.Post("/_e2e/stop", func(w http.ResponseWriter, r *http.Request) {
		select {
		case done <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	})
	router.Handle("/*", webui.Handler())
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	h.SetAdvertiseBaseURL(server.URL)
	ready, _ := json.Marshal(map[string]any{"url": server.URL, "agent_id": alice.ID, "other_agent_id": bob.ID, "upstream_url": mcpServer.URL, "participants": []apitypes.Participant{}})
	if err := os.WriteFile(readyPath, ready, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Log("isolated Apps browser fixture is ready")
	select {
	case <-done:
	case <-time.After(10 * time.Minute):
		t.Fatal("browser fixture timed out")
	}
}
