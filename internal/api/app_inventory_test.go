package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/apps"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAppInventoryReflectsCurrentUsableInstances(t *testing.T) {
	h := newAppTaskTestHandler(t)
	upstream := mcp.NewServer(&mcp.Implementation{Name: "inventory-fixture", Version: "1"}, nil)
	upstream.AddTool(&mcp.Tool{Name: "read", InputSchema: map[string]any{"type": "object"}, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	server := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return upstream }, nil))
	t.Cleanup(server.Close)
	t.Cleanup(func() { _ = h.apps.Close() })
	add := func(owner, name string) apps.Installation {
		t.Helper()
		item, err := h.apps.Create(context.Background(), owner, apps.CreateRequest{AppID: "gitlab", Name: name, Config: apps.Config{URL: server.URL, AuthMode: "none"}, Connect: true})
		if err != nil || item.Status != "connected" {
			t.Fatalf("create: %v", err)
		}
		return item
	}
	alice := add("agent-manager", "Alice")
	bob := add("agent-manager", "Bob")
	add("agent-dev", "Foreign")
	h.registerAppIdentityTools("agent-manager")
	client := taskMCPClient(t, h, "agent-manager")
	check := func(wantAvailable, wantInstalled int) {
		t.Helper()
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "apps_list", Arguments: map[string]any{}})
		if err != nil || result.IsError {
			t.Fatalf("list: %v", err)
		}
		raw := result.Content[0].(*mcp.TextContent).Text
		var inventory struct {
			Apps []struct {
				Name      string `json:"name"`
				Available bool   `json:"available"`
			} `json:"apps"`
			Counts map[string]int `json:"available_app_counts"`
		}
		if err := json.Unmarshal([]byte(raw), &inventory); err != nil {
			t.Fatal(err)
		}
		if len(inventory.Apps) != wantInstalled || inventory.Counts["gitlab"] != wantAvailable {
			t.Fatalf("stale inventory: %s", raw)
		}
		for _, forbidden := range []string{"Foreign", "inputSchema", "credentials", "headers"} {
			if strings.Contains(raw, forbidden) {
				t.Fatalf("unexpected inventory data: %s", forbidden)
			}
		}
	}
	check(2, 2)
	if _, err := h.apps.Disconnect(context.Background(), "agent-manager", bob.InstallationID); err != nil {
		t.Fatal(err)
	}
	check(1, 2)
	if err := h.apps.Delete(context.Background(), "agent-manager", bob.InstallationID); err != nil {
		t.Fatal(err)
	}
	check(1, 1)
	disabled := false
	if _, err := h.apps.Update(context.Background(), "agent-manager", alice.InstallationID, apps.UpdateRequest{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	check(0, 1)
}
