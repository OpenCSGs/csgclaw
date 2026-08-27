package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProbeServerConnectsAndListsTools(t *testing.T) {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "search-server",
		Title:   "Search Server",
		Version: "1.2.3",
	}, nil)
	server.AddTool(&mcpsdk.Tool{
		Name:        "search_docs",
		Title:       "Search docs",
		Description: "Search product documentation.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query"},
			},
			"required": []any{"query"},
		},
	}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{}, nil
	})
	mcpHandler := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return server },
		&mcpsdk.StreamableHTTPOptions{JSONResponse: true, Stateless: true},
	)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()

	result, err := NewService().ProbeServer(context.Background(), "docs", map[string]any{
		"type":    "streamable_http",
		"url":     httpServer.URL,
		"headers": map[string]any{"Authorization": "Bearer test-token"},
	})
	if err != nil {
		t.Fatalf("ProbeServer() error = %v", err)
	}
	if !result.Connected || !result.ToolsSupported {
		t.Fatalf("ProbeServer() result = %+v, want connected tools server", result)
	}
	if result.ProtocolVersion == "" {
		t.Fatalf("ProbeServer() protocol version is empty: %+v", result)
	}
	if result.ServerInfo == nil || result.ServerInfo.Name != "search-server" || result.ServerInfo.Version != "1.2.3" {
		t.Fatalf("ProbeServer() server info = %+v", result.ServerInfo)
	}
	if len(result.Tools) != 1 {
		t.Fatalf("ProbeServer() tools = %+v, want one tool", result.Tools)
	}
	tool := result.Tools[0]
	if tool.Name != "search_docs" || tool.Title != "Search docs" || tool.Description == "" {
		t.Fatalf("ProbeServer() tool = %+v", tool)
	}
	inputSchema, ok := tool.InputSchema.(map[string]any)
	if !ok || inputSchema["type"] != "object" {
		t.Fatalf("ProbeServer() input schema = %#v", tool.InputSchema)
	}
}

func TestProbeServerRejectsUnsupportedTransport(t *testing.T) {
	_, err := NewService().ProbeServer(context.Background(), "docs", map[string]any{
		"transport": "websocket",
		"url":       "https://mcp.example.test",
	})
	if err == nil || !errors.Is(err, ErrInvalidServerConfig) {
		t.Fatalf("ProbeServer() error = %v, want invalid config", err)
	}
}

func TestProbeHeaderTransportDoesNotForwardCredentialsAcrossOrigins(t *testing.T) {
	seen := make([]string, 0, 2)
	transport := probeHeaderTransport{
		allowedScheme: "https",
		allowedHost:   "mcp.example.test",
		headers:       http.Header{"Authorization": []string{"Bearer test-token"}},
		base: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			seen = append(seen, request.Header.Get("Authorization"))
			return &http.Response{Body: http.NoBody, Header: http.Header{}, StatusCode: http.StatusOK}, nil
		}),
	}

	for _, target := range []string{"https://mcp.example.test/tools", "https://redirect.example.test/tools"} {
		request, err := http.NewRequest(http.MethodPost, target, nil)
		if err != nil {
			t.Fatalf("NewRequest(%q): %v", target, err)
		}
		response, err := transport.RoundTrip(request)
		if err != nil {
			t.Fatalf("RoundTrip(%q): %v", target, err)
		}
		response.Body.Close()
	}

	if len(seen) != 2 || seen[0] != "Bearer test-token" || seen[1] != "" {
		t.Fatalf("Authorization headers = %#v, want same-origin only", seen)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
