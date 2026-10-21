package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBatchAddMCPEnablementIsAgentScoped(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{"inherit", `{"names":["tools"]}`, false},
		{"enable", `{"names":["tools"],"enabled":true}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, svc, created := newAgentMCPManagementTestServer(t)
			if _, err := handler.mcp.CreateServer(context.Background(), "tools", map[string]any{
				"command": "node", "enabled": false,
			}); err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost,
				"/api/v1/agents/"+created.ID+"/mcp-servers:batchAdd", strings.NewReader(tc.body)))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
			}
			saved, ok := svc.Agent(created.ID)
			if !ok {
				t.Fatal("Agent is missing")
			}
			config := mcpServerForTest(t, mcpServersForTest(t, saved.MCPServers), "tools")
			if config["enabled"] != tc.want {
				t.Fatalf("Agent enabled = %v, want %v", config["enabled"], tc.want)
			}
			catalog, err := handler.mcp.ListServers(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if catalog["tools"].(map[string]any)["enabled"] != false {
				t.Fatal("global resource enablement changed")
			}
		})
	}
}
