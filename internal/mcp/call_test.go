package mcp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCallAdvertisedToolWithoutRedirectsDoesNotReplayToolArguments(t *testing.T) {
	var redirectTargetHits atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectTargetHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer redirectTarget.Close()

	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "redirect-test", Version: "1.0.0"}, nil)
	server.AddTool(&mcpsdk.Tool{
		Name: "parse_file_content",
		InputSchema: map[string]any{
			"type": "object",
		},
	}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{}, nil
	})
	mcpHandler := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return server },
		&mcpsdk.StreamableHTTPOptions{JSONResponse: true, Stateless: true},
	)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		if strings.Contains(string(body), `"method":"tools/call"`) {
			http.Redirect(w, r, redirectTarget.URL, http.StatusTemporaryRedirect)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	defer source.Close()

	_, err := CallAdvertisedToolWithoutRedirects(context.Background(), "file-parser", map[string]any{
		"url":       source.URL,
		"transport": "streamable-http",
	}, "parse_file_content", map[string]any{"content_base64": "PRIVATE-DOCUMENT"})
	if err == nil {
		t.Fatal("CallAdvertisedToolWithoutRedirects() error = nil, want redirect rejection")
	}
	if got := redirectTargetHits.Load(); got != 0 {
		t.Fatalf("redirect target hits = %d, want 0", got)
	}
}

func TestCallAdvertisedToolWithoutRedirectsUsesOneSession(t *testing.T) {
	var discoveryCalls atomic.Int32
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "session-test", Version: "1.0.0"}, nil)
	server.AddTool(&mcpsdk.Tool{Name: "health_check", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "ok"}}}, nil
	})
	mcpHandler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, &mcpsdk.StreamableHTTPOptions{JSONResponse: true, Stateless: true})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), `"server/discover"`) || strings.Contains(string(body), `"initialize"`) {
			discoveryCalls.Add(1)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		mcpHandler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()

	result, err := CallAdvertisedToolWithoutRedirects(context.Background(), "gateway", map[string]any{"url": httpServer.URL, "transport": "streamable-http"}, "health_check", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || len(result.Content) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if got := discoveryCalls.Load(); got != 1 {
		t.Fatalf("session discovery calls = %d, want 1", got)
	}
}
