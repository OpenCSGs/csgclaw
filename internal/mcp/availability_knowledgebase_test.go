package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"csgclaw/internal/auth"
	"csgclaw/internal/knowledgebase"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTemplateKnowledgeBaseAvailabilityUsesHostedIdentity(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			originalStore := auth.Default().Store
			auth.Default().Store = auth.NewStore(filepath.Join(t.TempDir(), "state.json"))
			t.Cleanup(func() { auth.Default().Store = originalStore })
			t.Setenv("CSGHUB_ACCESS_TOKEN", "")
			t.Setenv("CSGHUB_USER_TOKEN", "hosted-test-token")

			server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "wiki", Version: "1.0.0"}, nil)
			mcpHandler := mcpsdk.NewStreamableHTTPHandler(
				func(*http.Request) *mcpsdk.Server { return server },
				&mcpsdk.StreamableHTTPOptions{JSONResponse: true, Stateless: true},
			)
			var apiCalls, mcpCalls atomic.Int32
			var baseURL string
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer hosted-test-token" {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				switch r.URL.Path {
				case "/api/v1/agent/knowledge-bases/42":
					apiCalls.Add(1)
					if status != http.StatusOK {
						http.Error(w, "forbidden", status)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
						"id": 42, "name": "Handbook", "content_id": "wiki-42", "type": "llmwiki",
						"metadata": map[string]any{
							"mcp_endpoint_url": baseURL + "/mcp",
							"resource_state":   map[string]any{"readiness": "ready", "mcp_status": "ready"},
						},
					}})
				case "/mcp":
					mcpCalls.Add(1)
					mcpHandler.ServeHTTP(w, r)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(httpServer.Close)
			baseURL = httpServer.URL
			t.Setenv("CSGHUB_API_BASE_URL", baseURL)
			servers := map[string]any{"wiki-42": map[string]any{
				"url": baseURL + "/mcp", "transport": "streamable-http",
				"_meta": map[string]any{"com.opencsg/mcp": map[string]any{
					"type": "llm_wiki", "resource_id": "42", "content_id": "wiki-42", "auth_type": "csghub_access_token",
				}},
			}}
			// Template hydration refreshes the source, but deliberately persists no token.
			hydrated, err := knowledgebase.HydrateTemplateServers(context.Background(), servers)
			if err != nil {
				t.Fatal(err)
			}
			filtered, skipped := NewService().FilterAvailableTemplateServers(context.Background(), hydrated)
			_, retained := filtered["wiki-42"]
			if want := status == http.StatusOK; retained != want {
				t.Fatalf("knowledge base retained = %v, want %v; skipped = %#v", retained, want, skipped)
			}
			if status == http.StatusOK && (len(skipped) != 0 || mcpCalls.Load() == 0) {
				t.Fatalf("authenticated MCP was not retained: skipped=%#v MCP calls=%d", skipped, mcpCalls.Load())
			}
			if status == http.StatusForbidden && (len(skipped) != 1 || mcpCalls.Load() != 0) {
				t.Fatalf("permission denial reached MCP: skipped=%#v MCP calls=%d", skipped, mcpCalls.Load())
			}
			if apiCalls.Load() == 0 {
				t.Fatal("hosted identity did not check the current resource")
			}
			for _, snapshot := range []map[string]any{servers, hydrated, filtered} {
				if entry, ok := snapshot["wiki-42"].(map[string]any); ok {
					if _, persisted := entry["headers"]; persisted {
						t.Fatal("availability check persisted the hosted credential")
					}
				}
			}
		})
	}
}
