package apps

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFeishuPassthroughPreservesBothAuthenticationLayers(t *testing.T) {
	for _, tokenType := range []string{"user_access_token", "tenant_access_token"} {
		t.Run(tokenType, func(t *testing.T) {
			var calls atomic.Int32
			upstream := mcp.NewServer(&mcp.Implementation{Name: "lark-passthrough", Version: "1"}, nil)
			upstream.AddTool(&mcp.Tool{Name: "read", InputSchema: map[string]any{"type": "object"}, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(_ context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				for key, want := range map[string]string{"Authorization": "Bearer fake-opencsg-token", "lark-access-token": "fake-lark-token", "X-Lark-Token-Type": tokenType} {
					if got := r.Extra.Header.Get(key); got != want {
						t.Errorf("header %s not preserved", key)
					}
				}
				calls.Add(1)
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "read succeeded"}}}, nil
			})
			handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return upstream }, nil)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fake-opencsg-token" {
					http.Error(w, "platform authorization failed", 401)
					return
				}
				if r.Header.Get("X-Feishu-App-Secret") != "" {
					t.Error("App Secret must not be sent to passthrough server")
				}
				handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			service := newTestService(t, Options{})
			item, err := service.Create(context.Background(), "agent", CreateRequest{AppID: "feishu", Name: "Passthrough", Config: Config{URL: server.URL, AuthMode: "bearer", Headers: map[string]string{"X-Lark-Token-Type": tokenType}}, Credentials: Credentials{Token: "fake-opencsg-token", Headers: map[string]string{"lark-access-token": "fake-lark-token"}}, Connect: true})
			if err != nil || item.Status != "connected" {
				t.Fatalf("connect: %v", err)
			}
			client := gatewayClient(t, service, "agent")
			result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: toolName(item.InstallationID, "read"), Arguments: map[string]any{}})
			if err != nil || result.IsError || calls.Load() != 1 {
				t.Fatalf("passthrough call failed: %v", err)
			}
			data, _ := json.Marshal(item)
			for _, secret := range []string{"fake-opencsg-token", "fake-lark-token"} {
				if strings.Contains(string(data), secret) {
					t.Fatal("credential exposed in App response")
				}
			}
		})
	}
}
