package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"csgclaw/internal/config"
	"csgclaw/internal/knowledgebase"
	hub "csgclaw/internal/template"
)

type agentKnowledgeBaseRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn agentKnowledgeBaseRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func installAgentKnowledgeBaseResponse(t *testing.T, endpoint, token string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"id": 42, "name": "Handbook", "content_id": "content-42", "type": "llmwiki",
			"metadata": map[string]any{
				"mcp_endpoint_url": endpoint,
				"resource_state":   map[string]any{"readiness": "ready", "mcp_status": "ready"},
			},
		},
	})
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	http.DefaultTransport = agentKnowledgeBaseRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got, want := r.URL.Path, "/api/v1/agent/knowledge-bases/42"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer "+token; got != want {
			t.Fatalf("Authorization = %q, want %q", got, want)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(encoded)),
		}, nil
	})
	return "https://hub.example.test"
}

func TestManagedKnowledgeBaseMCPPublishAndRuntimeMaterialization(t *testing.T) {
	localConfig := map[string]any{
		"type":      "remote",
		"url":       "https://publisher-gateway.example.test/v1/llmwikis/content-42/mcp",
		"transport": "streamable-http",
		"headers": map[string]any{
			"Authorization": "Bearer publisher-csghub-token",
		},
		knowledgebase.ManagedMetaKey: map[string]any{
			knowledgebase.ManagedMetaNamespace: map[string]any{
				"type":        knowledgebase.ManagedMCPType,
				"resource_id": "42",
				"content_id":  "content-42",
				"auth_type":   knowledgebase.ManagedAuthType,
			},
		},
	}

	published := templateSafeMCPServers(map[string]any{"handbook": localConfig})
	publishedConfig := published["handbook"].(map[string]any)
	if _, exists := publishedConfig["headers"]; exists {
		t.Fatalf("published config leaked publisher authentication: %#v", publishedConfig)
	}
	if _, managed := knowledgebase.ManagedMetadataFromServer(publishedConfig); !managed {
		t.Fatalf("published config lost managed knowledge-base identity: %#v", publishedConfig)
	}
	encoded, err := json.Marshal(published)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), "publisher-csghub-token") {
		t.Fatalf("published MCP document leaked publisher token: %s", encoded)
	}

	hubURL := installAgentKnowledgeBaseResponse(t, "https://runner-gateway.example.test/v1/llmwikis/content-42/mcp", "runner-csghub-token")
	hydratedConfig, err := knowledgebase.HydrateManagedServer(context.Background(), publishedConfig, knowledgebase.Connection{
		CSGHubBaseURL:     hubURL,
		AIGatewayBaseURL:  "https://must-not-be-used.example.test/v1",
		CSGHubAccessToken: "runner-csghub-token",
	})
	if err != nil {
		t.Fatalf("HydrateManagedServer() error = %v", err)
	}
	hydrated := map[string]any{"handbook": hydratedConfig}
	if _, managed := knowledgebase.ManagedMetadataFromServer(hydrated["handbook"]); !managed {
		t.Fatalf("hydrated config lost managed metadata: %#v", hydrated)
	}

	svc := &Service{}
	runtimeServers, err := svc.materializeRuntimeMCPServers(RuntimeKindCodex, hydrated)
	if err != nil {
		t.Fatalf("materializeRuntimeMCPServers() error = %v", err)
	}
	runtimeConfig := runtimeServers["handbook"].(map[string]any)
	if got, want := runtimeConfig["url"], "https://runner-gateway.example.test/v1/llmwikis/content-42/mcp"; got != want {
		t.Fatalf("runtime url = %#v, want %q", got, want)
	}
	headers := runtimeConfig["headers"].(map[string]any)
	if got, want := headers["Authorization"], "Bearer runner-csghub-token"; got != want {
		t.Fatalf("runtime Authorization = %#v, want %q", got, want)
	}
	if _, managed := knowledgebase.ManagedMetadataFromServer(runtimeConfig); managed {
		t.Fatalf("runtime config leaked managed metadata: %#v", runtimeConfig)
	}
}

func TestTemplateCreateSpecInjectsCurrentRunnerTokenIntoManagedKnowledgeBaseMCP(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	hubURL := installAgentKnowledgeBaseResponse(t, "https://runner-gateway.example.test/v1/llmwikis/content-42/mcp", "runner-csghub-token")
	t.Setenv("CSGHUB_API_BASE_URL", hubURL)
	t.Setenv("CSGHUB_ACCESS_TOKEN", "runner-csghub-token")
	t.Setenv("CSGHUB_AIGATEWAY_BASE_URL", "https://must-not-be-used.example.test/v1")

	templateConfig := map[string]any{
		"type":      "remote",
		"url":       "https://publisher-gateway.example.test/untrusted",
		"transport": "streamable-http",
		knowledgebase.ManagedMetaKey: map[string]any{
			knowledgebase.ManagedMetaNamespace: map[string]any{
				"type":        knowledgebase.ManagedMCPType,
				"resource_id": "42",
				"content_id":  "content-42",
				"auth_type":   knowledgebase.ManagedAuthType,
			},
		},
	}
	hubSvc := mustNewLocalTemplateHubServiceWithMCP(t, "knowledge-worker", hub.Template{
		ID:          "knowledge-worker",
		Name:        "knowledge-worker",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: RuntimeNameCodex,
	}, map[string]any{"knowledge": templateConfig})
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:1", "", WithHubService(hubSvc))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	resolved, cleanup, err := svc.resolveTemplateCreateSpec(context.Background(), CreateAgentSpec{
		Name:         "alice",
		FromTemplate: "local.knowledge-worker",
	})
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatalf("resolveTemplateCreateSpec() error = %v", err)
	}
	entry := resolved.MCPServers["knowledge"].(map[string]any)
	if got, want := entry["url"], "https://runner-gateway.example.test/v1/llmwikis/content-42/mcp"; got != want {
		t.Fatalf("url = %#v, want %q", got, want)
	}
	headers := entry["headers"].(map[string]any)
	if got, want := headers["Authorization"], "Bearer runner-csghub-token"; got != want {
		t.Fatalf("Authorization = %#v, want %q", got, want)
	}
	if _, managed := knowledgebase.ManagedMetadataFromServer(entry); !managed {
		t.Fatalf("resolved config lost managed _meta: %#v", entry)
	}
}
