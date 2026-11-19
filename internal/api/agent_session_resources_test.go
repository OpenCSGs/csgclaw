package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
)

func TestSessionResourcesAreResolvedAtExecutionAndDoNotPersistToNextTurn(t *testing.T) {
	item := sessionCodexAgent("worker", "Worker")
	item.MCPServers = map[string]any{"search": map[string]any{"url": "https://fixture.invalid/mcp", "display_name": "Runtime search"}}
	var inputs []string
	engine := &fakeSessionEngine{run: func(_ context.Context, _ string, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		inputs = append(inputs, sessionTurnText(request))
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "ok"}
	}}
	h, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{item}, engine, "")
	h.EnableConversationResources()
	refs := []apitypes.ResourceReference{{Source: "mcp", ID: "search"}}
	response := performAgentSessionRequest(t, h, "worker", "session", map[string]any{"input": "inspect", "resource_refs": refs})
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	if len(inputs) != 1 || !strings.Contains(inputs[0], "Runtime search") || !strings.Contains(inputs[0], `"id":"search"`) {
		t.Fatalf("resource missing from runtime input: %v", inputs)
	}
	response = performAgentSessionRequest(t, h, "worker", "session", map[string]any{"input": "next"})
	if response.Code != http.StatusOK || len(inputs) != 2 || inputs[1] != "next" {
		t.Fatalf("selection leaked into next turn: %v %s", inputs, response.Body.String())
	}
	for _, payload := range []map[string]any{
		{"input": "inspect", "resource_refs": []apitypes.ResourceReference{{Source: "mcp", ID: "missing"}}},
		{"input": "", "resource_refs": refs},
		{"input": "inspect", "resource_refs": []apitypes.ResourceReference{{Source: "skill", ID: "../escape"}}},
	} {
		response = performAgentSessionRequest(t, h, "worker", "session", payload)
		if response.Code < 400 || len(inputs) != 2 {
			t.Fatalf("invalid selection executed: status=%d input=%v", response.Code, inputs)
		}
	}
}
