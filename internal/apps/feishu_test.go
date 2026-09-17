package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	feishutransport "csgclaw/internal/channel/feishu/transport"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeTenantSource struct {
	mu         sync.Mutex
	prefix     string
	generation int
}

func (s *fakeTenantSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%d", s.prefix, s.generation), nil
}
func (s *fakeTenantSource) Invalidate(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token == fmt.Sprintf("%s-%d", s.prefix, s.generation) {
		s.generation++
	}
}

func TestFeishuAppObtainsAndRefreshesTenantToken(t *testing.T) {
	for _, reject := range []string{"lark_token_invalid", "lark_token_invalid_rpc", "lark_token_invalid_always", "lark_scope_missing", "upstream_retryable"} {
		t.Run(reject, func(t *testing.T) {
			var calls atomic.Int32
			source := &fakeTenantSource{prefix: "tenant-test", generation: 1}
			upstream := mcp.NewServer(&mcp.Implementation{Name: "passthrough", Version: "1"}, nil)
			upstream.AddTool(&mcp.Tool{Name: "read", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				n := calls.Add(1)
				if r.Extra.Header.Get("Authorization") != "Bearer platform-test" || r.Extra.Header.Get("X-Lark-Token-Type") != "tenant_access_token" {
					t.Error("wrong credential layer")
				}
				want := "tenant-test-1"
				if n > 1 && strings.HasPrefix(reject, "lark_token_invalid") {
					want = "tenant-test-2"
				}
				if r.Extra.Header.Get("lark-access-token") != want {
					t.Error("TAT was not refreshed")
				}
				if n == 1 && reject == "lark_token_invalid_rpc" {
					return nil, &jsonrpc.Error{Code: -32000, Message: "rejected", Data: json.RawMessage(`{"code":"lark_token_invalid"}`)}
				}
				if n == 1 || reject == "lark_token_invalid_always" {
					code := reject
					if reject == "lark_token_invalid_always" {
						code = "lark_token_invalid"
					}
					return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: `{"error":{"code":"` + code + `"}}`}}}, nil
				}
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
			})
			handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return upstream }, nil)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-App-Secret") != "" || strings.Contains(r.Header.Get("Authorization"), "app-secret") {
					t.Error("App Secret reached MCP")
				}
				handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			service := newTestService(t, Options{ResolveFeishu: func(context.Context, string) (FeishuCredentials, error) {
				return FeishuCredentials{AppID: "cli-real-app", AppSecret: "app-secret"}, nil
			}, FeishuTokenSource: func(id, secret string) feishutransport.TenantTokenSource {
				if id != "cli-real-app" || secret != "app-secret" {
					t.Error("channel credential reference not resolved")
				}
				return source
			}})
			item, err := service.Create(context.Background(), "agent", CreateRequest{AppID: "feishu", Name: "Feishu", Config: Config{URL: server.URL, AuthMode: "feishu", CredentialSource: "feishu_channel"}, Credentials: Credentials{Token: "platform-test", Headers: map[string]string{"X-Lark-Token-Type": "user_access_token", "lark-access-token": "stale-user-token"}}, Connect: true})
			if err != nil || item.Status != "connected" {
				t.Fatalf("connection: %v", err)
			}
			client := gatewayClient(t, service, "agent")
			result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: toolName(item.InstallationID, "read"), Arguments: map[string]any{}})
			if err != nil {
				t.Fatal(err)
			}
			if reject == "lark_token_invalid_always" {
				got, _ := service.Get(context.Background(), "agent", item.InstallationID)
				if calls.Load() != 2 || !result.IsError || got.Status != "authorization_required" {
					t.Fatal("repeated invalid token was not bounded and revoked")
				}
			} else if strings.HasPrefix(reject, "lark_token_invalid") {
				if calls.Load() != 2 || result.IsError {
					t.Fatal("explicit token rejection did not refresh once")
				}
			} else if calls.Load() != 1 || !result.IsError {
				t.Fatal("non-authentication failure retried")
			}
			raw, _ := json.Marshal(item)
			for _, secret := range []string{"platform-test", "tenant-test-1", "app-secret", "stale-user-token"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("credential leaked in App view")
				}
			}
		})
	}
}

func TestFeishuTokenRejectionRequiresStructuredError(t *testing.T) {
	if feishuTokenRejected(&mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "document mentions lark_token_invalid"}}}) {
		t.Fatal("free text caused retry")
	}
	if feishuTokenRejected(&mcp.CallToolResult{StructuredContent: map[string]any{"code": "lark_token_invalid"}}) {
		t.Fatal("successful result caused retry")
	}
}

func TestFeishuChannelRotationReplacesPrivateTokenSource(t *testing.T) {
	var channel atomic.Value
	channel.Store(FeishuCredentials{AppID: "channel-one", AppSecret: "secret-one"})
	upstream := mcp.NewServer(&mcp.Implementation{Name: "rotation", Version: "1"}, nil)
	upstream.AddTool(&mcp.Tool{Name: "read", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: r.Extra.Header.Get("lark-access-token")}}}, nil
	})
	server := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return upstream }, nil))
	t.Cleanup(server.Close)
	var created atomic.Int32
	service := newTestService(t, Options{ResolveFeishu: func(context.Context, string) (FeishuCredentials, error) {
		return channel.Load().(FeishuCredentials), nil
	}, FeishuTokenSource: func(id, secret string) feishutransport.TenantTokenSource {
		created.Add(1)
		return &fakeTenantSource{prefix: id, generation: 1}
	}})
	item, err := service.Create(context.Background(), "agent", CreateRequest{AppID: "feishu", Name: "Channel", Config: Config{URL: server.URL, AuthMode: "feishu", CredentialSource: "feishu_channel"}, Connect: true})
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.Create(context.Background(), "agent", CreateRequest{AppID: "feishu", Name: "Manual", Config: Config{URL: server.URL, AuthMode: "feishu"}, Credentials: Credentials{AppID: "manual", AppSecret: "manual-secret"}, Connect: true})
	if err != nil {
		t.Fatal(err)
	}
	client := gatewayClient(t, service, "agent")
	check := func(id, want string) {
		t.Helper()
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: toolName(id, "read"), Arguments: map[string]any{}})
		if err != nil || result.IsError || result.Content[0].(*mcp.TextContent).Text != want {
			t.Fatalf("unexpected token source after rotation: %v", err)
		}
	}
	check(item.InstallationID, "channel-one-1")
	check(other.InstallationID, "manual-1")
	channel.Store(FeishuCredentials{AppID: "channel-two", AppSecret: "secret-two"})
	if err := service.RefreshCredentials(context.Background(), "agent"); err != nil {
		t.Fatal(err)
	}
	check(item.InstallationID, "channel-two-1")
	check(other.InstallationID, "manual-1")
	if created.Load() != 3 {
		t.Fatal("rotation did not isolate connection token caches")
	}
	if _, err := service.Disconnect(context.Background(), "agent", item.InstallationID); err != nil {
		t.Fatal(err)
	}
	channel.Store(FeishuCredentials{AppID: "channel-three", AppSecret: "secret-three"})
	if err := service.RefreshCredentials(context.Background(), "agent"); err != nil {
		t.Fatal(err)
	}
	if created.Load() != 3 {
		t.Fatal("channel update revived a manually disconnected App")
	}
}
