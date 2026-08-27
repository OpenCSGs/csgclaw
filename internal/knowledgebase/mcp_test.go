package knowledgebase

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func availableKnowledgeBase() KnowledgeBase {
	return KnowledgeBase{
		ID:        42,
		Name:      "Engineering Handbook",
		ContentID: "wiki-content-42",
		Type:      TypeLLMWiki,
		Metadata: Metadata{
			MCPEndpoint: "https://gateway.example.test/v1/llmwikis/wiki-content-42/mcp",
			ResourceState: &ResourceState{
				Readiness: "ready",
				MCPStatus: "ready",
			},
		},
	}
}

func installKnowledgeBaseResponse(t *testing.T, item KnowledgeBase, wantToken string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"data": item})
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got, want := r.URL.Path, "/api/v1/agent/knowledge-bases/42"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer "+wantToken; got != want {
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

func TestServerConfigPersistsCSGHubAccessTokenAndManagedMeta(t *testing.T) {
	name, config, err := ServerConfig(availableKnowledgeBase(), "current-csghub-token")
	if err != nil {
		t.Fatalf("ServerConfig() error = %v", err)
	}
	if got, want := name, "agentichub-kb-42"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
	headers := config["headers"].(map[string]any)
	if got, want := headers["Authorization"], "Bearer current-csghub-token"; got != want {
		t.Fatalf("Authorization = %#v, want %q", got, want)
	}
	meta := config[ManagedMetaKey].(map[string]any)[ManagedMetaNamespace].(map[string]any)
	if got, want := meta["type"], ManagedMCPType; got != want {
		t.Fatalf("_meta type = %#v, want %q", got, want)
	}
	if got, want := meta["auth_type"], ManagedAuthType; got != want {
		t.Fatalf("_meta auth_type = %#v, want %q", got, want)
	}
	metadata, ok := ManagedMetadataFromServer(config)
	if !ok || metadata.KnowledgeBaseID != 42 || metadata.ContentID != "wiki-content-42" {
		t.Fatalf("ManagedMetadataFromServer() = %#v, %v", metadata, ok)
	}
}

func TestServerConfigRequiresCSGHubAccessToken(t *testing.T) {
	if _, _, err := ServerConfig(availableKnowledgeBase(), " "); err == nil {
		t.Fatal("ServerConfig() error = nil, want missing token error")
	}
}

func TestServerConfigAcceptsLegacyResourceStateEndpoint(t *testing.T) {
	item := availableKnowledgeBase()
	item.Metadata.ResourceState.MCPEndpoint = item.Metadata.MCPEndpoint
	item.Metadata.MCPEndpoint = ""
	_, config, err := ServerConfig(item, "current-csghub-token")
	if err != nil {
		t.Fatalf("ServerConfig() error = %v", err)
	}
	if got, want := config["url"], "https://gateway.example.test/v1/llmwikis/wiki-content-42/mcp"; got != want {
		t.Fatalf("url = %#v, want %q", got, want)
	}
}

func TestHydrateManagedServerRefreshesEndpointAndCSGHubAccessToken(t *testing.T) {
	_, config, err := ServerConfig(availableKnowledgeBase(), "stored-csghub-token")
	if err != nil {
		t.Fatalf("ServerConfig() error = %v", err)
	}
	current := availableKnowledgeBase()
	current.Metadata.MCPEndpoint = "https://current-gateway.example.test/v1/llmwikis/wiki-content-42/mcp"
	hubURL := installKnowledgeBaseResponse(t, current, "current-csghub-token")
	prepared, err := HydrateManagedServer(context.Background(), config, Connection{
		CSGHubBaseURL:     hubURL,
		AIGatewayBaseURL:  "https://must-not-be-used.example.test/v1",
		CSGHubAccessToken: "current-csghub-token",
	})
	if err != nil {
		t.Fatalf("HydrateManagedServer() error = %v", err)
	}
	if got, want := prepared["url"], "https://current-gateway.example.test/v1/llmwikis/wiki-content-42/mcp"; got != want {
		t.Fatalf("url = %#v, want %q", got, want)
	}
	headers := prepared["headers"].(map[string]any)
	if got, want := headers["Authorization"], "Bearer current-csghub-token"; got != want {
		t.Fatalf("Authorization = %#v, want %q", got, want)
	}
	originalHeaders := config["headers"].(map[string]any)
	if got, want := originalHeaders["Authorization"], "Bearer stored-csghub-token"; got != want {
		t.Fatalf("original Authorization = %#v, want %q", got, want)
	}
}

func TestHydrateTemplateServersInjectsRunnerTokenIntoTrustedDirectMCP(t *testing.T) {
	originalLoader := loadInteractiveConnection
	defer func() { loadInteractiveConnection = originalLoader }()
	loadInteractiveConnection = func(context.Context) (Connection, bool, error) {
		return Connection{}, false, nil
	}
	current := availableKnowledgeBase()
	current.Metadata.MCPEndpoint = "https://runner-gateway.example.test/v1/llmwikis/wiki-content-42/mcp"
	hubURL := installKnowledgeBaseResponse(t, current, "template-runner-token")
	t.Setenv("CSGHUB_API_BASE_URL", hubURL)
	t.Setenv("CSGHUB_USER_TOKEN", "template-runner-token")
	t.Setenv("CSGHUB_AIGATEWAY_BASE_URL", "https://must-not-be-used.example.test/v1")

	_, installed, err := ServerConfig(availableKnowledgeBase(), "publisher-token")
	if err != nil {
		t.Fatalf("ServerConfig() error = %v", err)
	}
	delete(installed, "headers")
	installed["url"] = "https://untrusted-template.example.test/steal-token"

	hydrated, err := HydrateTemplateServers(context.Background(), map[string]any{"handbook": installed})
	if err != nil {
		t.Fatalf("HydrateTemplateServers() error = %v", err)
	}
	entry := hydrated["handbook"].(map[string]any)
	if got, want := entry["url"], "https://runner-gateway.example.test/v1/llmwikis/wiki-content-42/mcp"; got != want {
		t.Fatalf("url = %#v, want %q", got, want)
	}
	headers := entry["headers"].(map[string]any)
	if got, want := headers["Authorization"], "Bearer template-runner-token"; got != want {
		t.Fatalf("Authorization = %#v, want %q", got, want)
	}
	if _, managed := ManagedMetadataFromServer(entry); !managed {
		t.Fatalf("hydrated config lost managed _meta: %#v", entry)
	}
}

func TestRuntimeServersKeepsDirectAuthenticationAndRemovesOnlyManagedMeta(t *testing.T) {
	name, config, err := ServerConfig(availableKnowledgeBase(), "runner-token")
	if err != nil {
		t.Fatalf("ServerConfig() error = %v", err)
	}
	config[ManagedMetaKey].(map[string]any)["third-party"] = map[string]any{"trace": "keep"}
	runtimeServers, err := RuntimeServers(map[string]any{name: config})
	if err != nil {
		t.Fatalf("RuntimeServers() error = %v", err)
	}
	entry := runtimeServers[name].(map[string]any)
	if got, want := entry["url"], "https://gateway.example.test/v1/llmwikis/wiki-content-42/mcp"; got != want {
		t.Fatalf("url = %#v, want %q", got, want)
	}
	headers := entry["headers"].(map[string]any)
	if got, want := headers["Authorization"], "Bearer runner-token"; got != want {
		t.Fatalf("Authorization = %#v, want %q", got, want)
	}
	if _, managed := ManagedMetadataFromServer(entry); managed {
		t.Fatalf("runtime config retained managed metadata: %#v", entry)
	}
	meta := entry[ManagedMetaKey].(map[string]any)
	if _, ok := meta["third-party"]; !ok {
		t.Fatalf("runtime config removed unrelated _meta: %#v", entry)
	}
}

func TestManagedMetadataFromServerSupportsLegacyMarker(t *testing.T) {
	legacy := map[string]any{
		ManagedConfigKey: map[string]any{
			"kind":              ManagedKind,
			"knowledge_base_id": "42",
			"content_id":        "content-42",
		},
	}
	metadata, ok := ManagedMetadataFromServer(legacy)
	if !ok || metadata.KnowledgeBaseID != 42 || metadata.ContentID != "content-42" {
		t.Fatalf("ManagedMetadataFromServer() = %#v, %v", metadata, ok)
	}
}
