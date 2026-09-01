package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/config"
	"csgclaw/internal/knowledgebase"
	agentruntime "csgclaw/internal/runtime"
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

	svc := &Service{}
	runtimeServers, err := svc.materializeRuntimeMCPServers(context.Background(), RuntimeKindCodex, map[string]any{"handbook": localConfig})
	if err != nil {
		t.Fatalf("materializeRuntimeMCPServers() error = %v", err)
	}
	runtimeConfig := runtimeServers["handbook"].(map[string]any)
	if got, want := runtimeConfig["url"], "https://publisher-gateway.example.test/v1/llmwikis/content-42/mcp"; got != want {
		t.Fatalf("runtime url = %#v, want %q", got, want)
	}
	headers, ok := runtimeConfig["headers"].(map[string]any)
	if !ok {
		t.Fatalf("runtime headers = %#v, want object; config=%#v", runtimeConfig["headers"], runtimeConfig)
	}
	if got, want := headers["Authorization"], "Bearer publisher-csghub-token"; got != want {
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
	}, map[string]any{"content-42": templateConfig})
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
	entry := resolved.MCPServers["content-42"].(map[string]any)
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

func TestDeleteManagedKnowledgeBaseMCPUsesPersistedSnapshotWithoutSourceAccess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var reconciled agentruntime.MCPServersChange
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			mcpRestart: func(agentruntime.MCPServersChange) (bool, error) {
				return false, nil
			},
			mcpReconcile: func(_ context.Context, _ agentruntime.Handle, change agentruntime.MCPServersChange) error {
				reconciled = change
				return nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	managed := map[string]any{
		"type":      "remote",
		"url":       "https://gateway.example.test/snapshot/mcp",
		"transport": "streamable-http",
		"headers":   map[string]any{"Authorization": "Bearer saved-token"},
		knowledgebase.ManagedMetaKey: map[string]any{
			knowledgebase.ManagedMetaNamespace: map[string]any{
				"type":        knowledgebase.ManagedMCPType,
				"resource_id": "42",
				"content_id":  "content-42",
				"auth_type":   knowledgebase.ManagedAuthType,
			},
		},
	}
	svc.agents["u-dev"] = Agent{
		ID:          "u-dev",
		Name:        "dev",
		RuntimeID:   "rt-u-dev",
		RuntimeKind: RuntimeKindCodex,
		Role:        RoleWorker,
		Status:      string(agentruntime.StateRunning),
		MCPServers:  map[string]any{"content-42": managed},
		CreatedAt:   time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
	}

	updated, err := svc.DeleteMCPServers(context.Background(), "u-dev", []string{"content-42"})
	if err != nil {
		t.Fatalf("DeleteMCPServers() error = %v", err)
	}
	if len(updated.MCPServers) != 0 {
		t.Fatalf("updated MCPServers = %#v, want empty", updated.MCPServers)
	}
	previous := reconciled.Previous.Servers["content-42"].(map[string]any)
	if got, want := previous["url"], "https://gateway.example.test/snapshot/mcp"; got != want {
		t.Fatalf("reconciled previous URL = %#v, want %q", got, want)
	}
	if _, managed := knowledgebase.ManagedMetadataFromServer(previous); managed {
		t.Fatalf("reconciled previous snapshot retained managed metadata: %#v", previous)
	}
	if len(reconciled.Current.Servers) != 0 {
		t.Fatalf("reconciled current servers = %#v, want empty", reconciled.Current.Servers)
	}
}
