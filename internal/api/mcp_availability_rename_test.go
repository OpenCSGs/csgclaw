package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"csgclaw/internal/mcp"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Use real catalog HTTP requests and MCP handshakes to ensure a display-only
// rename cannot hide a healthy server behind another slow availability probe.
func TestMCPCatalogRenameReusesAvailability(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var handshakes atomic.Int32
	releaseProbe := make(chan struct{})
	upstream := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "search", Version: "1"}, nil)
	transport := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return upstream }, &mcpsdk.StreamableHTTPOptions{JSONResponse: true})
	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
			var request struct {
				Method string `json:"method"`
			}
			_ = json.Unmarshal(raw, &request)
			if request.Method == "initialize" && handshakes.Add(1) > 1 {
				select {
				case <-releaseProbe:
				case <-r.Context().Done():
					return
				}
			}
		}
		transport.ServeHTTP(w, r)
	}))
	defer mcpServer.Close()
	defer close(releaseProbe)
	h := &Handler{mcp: mcp.NewService()}
	id, err := h.mcp.InstallRemoteServer(context.Background(), mcp.RemoteServer{ID: "bing", HubURL: "https://hub.example", Name: "必应 搜索", URL: mcpServer.URL})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h.Routes())
	defer server.Close()
	request := func(method, path string, body any) map[string]any {
		t.Helper()
		encoded, _ := json.Marshal(body)
		req, err := http.NewRequest(method, server.URL+path, bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, raw)
		}
		var result map[string]any
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	initial := request(http.MethodGet, "/api/v1/mcp-servers", nil)
	entry, ok := initial["mcpServers"].(map[string]any)[id].(map[string]any)
	if !ok {
		t.Fatalf("healthy server missing initially: %#v", initial)
	}
	if handshakes.Load() != 1 {
		t.Fatalf("initial handshakes = %d", handshakes.Load())
	}
	entry["display_name"] = "改名后的搜索"
	entry["description"] = "Updated description"
	request(http.MethodPut, "/api/v1/mcp-servers/"+id, map[string]any{"name": "改名后的搜索", "config": entry})
	renamed := request(http.MethodGet, "/api/v1/mcp-servers", nil)
	visible, ok := renamed["mcpServers"].(map[string]any)[id].(map[string]any)
	if !ok {
		t.Fatalf("display-only rename hid a healthy server: %#v", renamed)
	}
	if visible["display_name"] != "改名后的搜索" || renamed["probes_pending"] != false || handshakes.Load() != 1 {
		t.Fatalf("rename did not reuse availability: response=%#v handshakes=%d", renamed, handshakes.Load())
	}
	entry["headers"] = map[string]any{"X-Fixture": "changed-connection"}
	request(http.MethodPut, "/api/v1/mcp-servers/"+id, map[string]any{"config": entry})
	changed := request(http.MethodGet, "/api/v1/mcp-servers", nil)
	if changed["mcpServers"].(map[string]any)[id] != nil || changed["probes_pending"] != true || handshakes.Load() != 2 {
		t.Fatalf("connection edit reused stale availability: response=%#v handshakes=%d", changed, handshakes.Load())
	}
}
