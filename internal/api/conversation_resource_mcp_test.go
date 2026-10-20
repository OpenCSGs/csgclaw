package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/conversationresource"
	"csgclaw/internal/knowledgebase"
)

func TestConversationResourcesUseEffectiveMCPConfiguration(t *testing.T) {
	for _, kind := range []string{agent.RuntimeKindCodex, agent.RuntimeKindDSH} {
		for _, state := range []string{"runtime", "managed", "cleared"} {
			t.Run(kind+"/"+state, func(t *testing.T) {
				item := completeWorkerAgent("runtime-mcp", "Runtime MCP")
				item.RuntimeKind, item.RuntimeName = kind, kind
				item.Status, item.DesiredState = "stopped", "stopped"
				servers := map[string]any{
					"search": map[string]any{"command": "uvx", "args": []any{"example-mcp"}, "env": map[string]any{"API_KEY": "secret-fixture"}},
					"knowledge": map[string]any{"url": "https://fixture.invalid/mcp", "_meta": map[string]any{knowledgebase.ManagedMetaNamespace: map[string]any{
						"type": knowledgebase.ManagedMCPType, "auth_type": knowledgebase.ManagedAuthType, "resource_id": "1", "content_id": "wiki",
					}}},
				}
				switch state {
				case "runtime":
					item.MCPServers = nil
				case "managed":
					item.MCPServers = servers
				case "cleared":
					item.MCPServers = map[string]any{}
				}
				rt := &snapshotMCPServersRuntime{fakeCompatRuntime: fakeCompatRuntime{kind: kind}}
				controller := mustNewSeededServiceWithOptions(t, []agent.Agent{item}, agent.WithRuntime(rt))
				if state == "cleared" {
					empty := map[string]any{}
					if _, err := controller.UpdateRecord(t.Context(), item.ID, agent.UpdateRequest{MCPServers: &empty, MCPServersSet: true, FieldMask: []string{"mcpServers"}}); err != nil {
						t.Fatal(err)
					}
				}
				rt.servers = servers
				if state == "managed" {
					rt.servers = map[string]any{"runtime-only": map[string]any{"command": "uvx"}}
				}
				h := &Handler{svc: controller, agentEngine: agentengine.New(controller), workspace: controller.Workspace(), agentRuntime: controller}
				routes := h.Routes()
				read := func(suffix string) *httptest.ResponseRecorder {
					t.Helper()
					response := httptest.NewRecorder()
					routes.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+item.ID+suffix, nil))
					if response.Code != http.StatusOK {
						t.Fatalf("GET %s: %d %s", suffix, response.Code, response.Body)
					}
					return response
				}
				profile := read("/mcp-servers")
				var view agent.MCPServersView
				if err := json.Unmarshal(profile.Body.Bytes(), &view); err != nil {
					t.Fatal(err)
				}
				for resourceType, id := range map[apitypes.ResourceType]string{apitypes.ResourceTypeConnector: "search", apitypes.ResourceTypeKnowledgeBase: "knowledge"} {
					picker := read("/resources?type=" + string(resourceType) + "&pagination=cursor")
					var page struct {
						Items []apitypes.AgentResourceSummary `json:"items"`
					}
					if err := json.Unmarshal(picker.Body.Bytes(), &page); err != nil {
						t.Fatal(err)
					}
					ref := apitypes.ResourceReference{Source: apitypes.ResourceSourceMCP, ID: id}
					resolved, err := h.resources.Resolve(t.Context(), item.ID, []apitypes.ResourceReference{ref})
					if state == "cleared" {
						var failure *conversationresource.Error
						if len(view.Servers) != 0 || len(page.Items) != 0 || !errors.As(err, &failure) || failure.Code != conversationresource.ResourceNotAttached {
							t.Fatalf("cleared MCP configuration: profile=%v picker=%v err=%v", view.Servers, page.Items, err)
						}
						continue
					}
					if view.Servers[id] == nil || len(page.Items) != 1 || page.Items[0].ResourceReference != ref || !page.Items[0].Available || err != nil || len(resolved.Resources) != 1 {
						t.Fatalf("effective MCP configuration: profile=%v picker=%v resolved=%v err=%v", view.Servers, page.Items, resolved, err)
					}
					if strings.Contains(picker.Body.String(), "secret-fixture") {
						t.Fatal("resource picker exposed MCP credentials")
					}
					if state == "runtime" {
						servers[id].(map[string]any)["enabled"] = false
						_, _, err = h.resources.PrepareInput(t.Context(), item.ID, "Review", resolved.Snapshot())
						var failure *conversationresource.Error
						if !errors.As(err, &failure) || failure.Code != conversationresource.ResourceDisabled {
							t.Fatalf("execution did not recheck runtime enablement: %v", err)
						}
						delete(servers, id)
						_, _, err = h.resources.PrepareInput(t.Context(), item.ID, "Review", resolved.Snapshot())
						if !errors.As(err, &failure) || failure.Code != conversationresource.ResourceNotAttached {
							t.Fatalf("execution accepted a removed runtime MCP server: %v", err)
						}
					}
				}
			})
		}
	}
}
