package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/agent"
	"csgclaw/internal/knowledgebase"
	"csgclaw/internal/mcp"
)

type stubMCPServerProber struct {
	config map[string]any
	name   string
}

func (p *stubMCPServerProber) Probe(_ context.Context, name string, config map[string]any) (mcp.ProbeResult, error) {
	p.name = name
	p.config = config
	return mcp.ProbeResult{
		Connected:       true,
		ProtocolVersion: "2025-11-25",
		ServerInfo:      &mcp.ProbeServerInfo{Name: "docs", Version: "1.0.0"},
		ToolsSupported:  true,
		Tools: []mcp.ProbeTool{{
			Name:        "search",
			Description: "Search docs",
			InputSchema: map[string]any{"type": "object"},
		}},
	}, nil
}

func TestProbeMCPServerUsesDraftConfigWithoutPersisting(t *testing.T) {
	prober := &stubMCPServerProber{}
	store := &memoryMCPServerStoreForAPI{}
	handler := &Handler{mcp: mcp.NewService(mcp.WithServerProber(prober), mcp.WithServerStore(store))}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/mcp-servers:probe", strings.NewReader(`{
		"name":"docs",
		"config":{"url":"https://mcp.example.test","type":"streamable_http"}
	}`))

	handler.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("POST probe status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if prober.name != "docs" || prober.config["url"] != "https://mcp.example.test" {
		t.Fatalf("probe input = name %q config %#v", prober.name, prober.config)
	}
	if store.writes != 0 {
		t.Fatalf("probe persisted MCP state %d times", store.writes)
	}
	var result mcp.ProbeResult
	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatalf("decode probe response: %v", err)
	}
	if !result.Connected || len(result.Tools) != 1 || result.Tools[0].Name != "search" {
		t.Fatalf("probe response = %+v", result)
	}
}

func TestProbeManagedKnowledgeBaseMCPUsesDraftConfigWithoutRefreshing(t *testing.T) {
	originalLoader := loadKnowledgeBaseConnection
	defer func() { loadKnowledgeBaseConnection = originalLoader }()
	loadKnowledgeBaseConnection = func(context.Context) (knowledgeBaseConnection, error) {
		t.Fatal("managed MCP probe unexpectedly refreshed the knowledge base connection")
		return knowledgeBaseConnection{}, nil
	}

	prober := &stubMCPServerProber{}
	store := &memoryMCPServerStoreForAPI{}
	handler := &Handler{mcp: mcp.NewService(mcp.WithServerProber(prober), mcp.WithServerStore(store))}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/mcp-servers:probe", strings.NewReader(`{
		"name":"content-42",
		"config":{
			"type":"remote",
			"url":"https://gateway.example.test/v1/gateway/mcp",
			"transport":"streamable-http",
			"headers":{"Authorization":"Bearer draft-csghub-token"},
			"_meta":{
				"com.opencsg/mcp":{
					"type":"llm_wiki",
					"resource_id":"42",
					"content_id":"content-42",
					"auth_type":"csghub_access_token"
				}
			}
		}
	}`))

	handler.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("POST managed probe status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got, want := prober.config["url"], "https://gateway.example.test/v1/gateway/mcp"; got != want {
		t.Fatalf("probe url = %#v, want %q", got, want)
	}
	headers, ok := prober.config["headers"].(map[string]any)
	if !ok {
		t.Fatalf("probe headers = %#v", prober.config["headers"])
	}
	if got, want := headers["Authorization"], "Bearer draft-csghub-token"; got != want {
		t.Fatalf("probe authorization = %#v, want %q", got, want)
	}
	if _, ok := knowledgebase.ManagedMetadataFromServer(prober.config); !ok {
		t.Fatalf("probe lost managed metadata: %#v", prober.config)
	}
	if store.writes != 0 {
		t.Fatalf("probe persisted MCP state %d times", store.writes)
	}
	if strings.Contains(recorder.Body.String(), "draft-csghub-token") {
		t.Fatalf("probe response leaked CSGHub access token: %s", recorder.Body.String())
	}
}

func TestManagedKnowledgeBaseMCPSourceStatusAndManualSync(t *testing.T) {
	originalLoader := loadKnowledgeBaseConnection
	originalTransport := http.DefaultTransport
	defer func() {
		loadKnowledgeBaseConnection = originalLoader
		http.DefaultTransport = originalTransport
	}()
	loadKnowledgeBaseConnection = func(context.Context) (knowledgeBaseConnection, error) {
		return knowledgeBaseConnection{CSGHubBaseURL: "https://hub.example.test", CSGHubAccessToken: "current-token"}, nil
	}
	responseBody := []byte(`{"data":{"id":42,"name":"Handbook","description":"Current runbooks","content_id":"content-42","type":"llmwiki","metadata":{"mcp_endpoint_url":"https://gateway.example.test/current/mcp","resource_state":{"readiness":"ready","mcp_status":"ready"}}}}`)
	http.DefaultTransport = knowledgeBaseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got, want := req.URL.Path, "/api/v1/agent/knowledge-bases/42"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := req.Header.Get("Authorization"), "Bearer current-token"; got != want {
			t.Fatalf("Authorization = %q, want %q", got, want)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(responseBody)),
		}, nil
	})

	store := &knowledgeBaseServerStore{servers: map[string]any{
		"content-42": map[string]any{
			"type":      "remote",
			"url":       "https://gateway.example.test/snapshot/mcp",
			"transport": "streamable-http",
			"headers":   map[string]any{"Authorization": "Bearer snapshot-token"},
			knowledgebase.ManagedMetaKey: map[string]any{
				knowledgebase.ManagedMetaNamespace: map[string]any{
					"type":        knowledgebase.ManagedMCPType,
					"resource_id": "42",
					"content_id":  "content-42",
					"auth_type":   knowledgebase.ManagedAuthType,
				},
			},
		},
	}}
	handler := &Handler{mcp: mcp.NewService(mcp.WithServerStore(store))}

	statusRecorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(statusRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/mcp-servers/content-42/source", nil))
	if got, want := statusRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("GET source status = %d, want %d; body=%s", got, want, statusRecorder.Body.String())
	}
	var status mcpServerSourceStatusResponse
	if err := json.NewDecoder(statusRecorder.Body).Decode(&status); err != nil {
		t.Fatalf("decode source status: %v", err)
	}
	if !status.UpdateAvailable || status.ConfiguredEndpointURL != "https://gateway.example.test/snapshot/mcp" || status.LatestEndpointURL != "https://gateway.example.test/current/mcp" {
		t.Fatalf("source status = %#v", status)
	}
	if got := store.servers["content-42"].(map[string]any)["url"]; got != "https://gateway.example.test/snapshot/mcp" {
		t.Fatalf("GET source status changed snapshot URL to %#v", got)
	}

	syncRecorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(syncRecorder, httptest.NewRequest(http.MethodPost, "/api/v1/mcp-servers/content-42/source:sync", nil))
	if got, want := syncRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("POST source sync = %d, want %d; body=%s", got, want, syncRecorder.Body.String())
	}
	var synced mcpServerSourceSyncResponse
	if err := json.NewDecoder(syncRecorder.Body).Decode(&synced); err != nil {
		t.Fatalf("decode source sync: %v", err)
	}
	if synced.Source.UpdateAvailable {
		t.Fatalf("synced source update_available = true")
	}
	config := store.servers["content-42"].(map[string]any)
	if got, want := config["url"], "https://gateway.example.test/current/mcp"; got != want {
		t.Fatalf("synced url = %#v, want %q", got, want)
	}
	headers := config["headers"].(map[string]any)
	if got, want := headers["Authorization"], "Bearer current-token"; got != want {
		t.Fatalf("synced Authorization = %#v, want %q", got, want)
	}
}

func TestAgentManagedKnowledgeBaseMCPSourceSyncUpdatesGlobalAndCurrentAgent(t *testing.T) {
	originalLoader := loadKnowledgeBaseConnection
	originalTransport := http.DefaultTransport
	defer func() {
		loadKnowledgeBaseConnection = originalLoader
		http.DefaultTransport = originalTransport
	}()
	loadKnowledgeBaseConnection = func(context.Context) (knowledgeBaseConnection, error) {
		return knowledgeBaseConnection{CSGHubBaseURL: "https://hub.example.test", CSGHubAccessToken: "current-token"}, nil
	}
	responseBody := []byte(`{"data":{"id":42,"name":"Handbook","description":"Current runbooks","content_id":"content-42","type":"llmwiki","metadata":{"mcp_endpoint_url":"https://gateway.example.test/current/mcp","resource_state":{"readiness":"ready","mcp_status":"ready"}}}}`)
	http.DefaultTransport = knowledgeBaseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(responseBody)),
		}, nil
	})

	handler, service, created := newAgentMCPManagementTestServer(t)
	agentSnapshot := map[string]any{
		"content-42": managedKnowledgeBaseMCPConfigForTest("https://gateway.example.test/agent-snapshot/mcp", "agent-snapshot-token"),
	}
	if _, err := service.Update(context.Background(), created.ID, agent.UpdateRequest{
		MCPServers:    &agentSnapshot,
		MCPServersSet: true,
		FieldMask:     []string{"mcpServers"},
	}); err != nil {
		t.Fatalf("seed Agent MCP snapshot: %v", err)
	}
	if _, err := handler.mcp.CreateServer(context.Background(), "content-42", managedKnowledgeBaseMCPConfigForTest(
		"https://gateway.example.test/global-snapshot/mcp",
		"global-snapshot-token",
	)); err != nil {
		t.Fatalf("seed global MCP snapshot: %v", err)
	}

	router := handler.Routes()
	statusRecorder := httptest.NewRecorder()
	router.ServeHTTP(statusRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+created.ID+"/mcp-servers/content-42/source", nil))
	if got, want := statusRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("GET Agent MCP source status = %d, want %d; body=%s", got, want, statusRecorder.Body.String())
	}
	var status mcpServerSourceStatusResponse
	if err := json.NewDecoder(statusRecorder.Body).Decode(&status); err != nil {
		t.Fatalf("decode Agent MCP source status: %v", err)
	}
	if !status.UpdateAvailable || !status.AgentUpdateAvailable || !status.GlobalUpdateAvailable || status.GlobalServerName != "content-42" {
		t.Fatalf("Agent MCP source status = %#v", status)
	}

	syncRecorder := httptest.NewRecorder()
	router.ServeHTTP(syncRecorder, httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+created.ID+"/mcp-servers/content-42/source:sync", nil))
	if got, want := syncRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("POST Agent MCP source sync = %d, want %d; body=%s", got, want, syncRecorder.Body.String())
	}
	var synced agentMCPServerSourceSyncResponse
	if err := json.NewDecoder(syncRecorder.Body).Decode(&synced); err != nil {
		t.Fatalf("decode Agent MCP source sync: %v", err)
	}
	if synced.Source.UpdateAvailable || synced.Source.GlobalServerName != "content-42" {
		t.Fatalf("synced source status = %#v", synced.Source)
	}

	globalServers, err := handler.mcp.ListServers(context.Background())
	if err != nil {
		t.Fatalf("list global MCP servers: %v", err)
	}
	assertManagedKnowledgeBaseMCPRuntimeSnapshot(t, globalServers["content-42"], "https://gateway.example.test/current/mcp", "current-token")
	saved, ok := service.Agent(created.ID)
	if !ok {
		t.Fatalf("Agent(%q) not found", created.ID)
	}
	assertManagedKnowledgeBaseMCPRuntimeSnapshot(t, saved.MCPServers["content-42"], "https://gateway.example.test/current/mcp", "current-token")
}

func managedKnowledgeBaseMCPConfigForTest(endpoint, token string) map[string]any {
	return map[string]any{
		"type":      "remote",
		"url":       endpoint,
		"transport": "streamable-http",
		"headers":   map[string]any{"Authorization": "Bearer " + token},
		knowledgebase.ManagedMetaKey: map[string]any{
			knowledgebase.ManagedMetaNamespace: map[string]any{
				"type":        knowledgebase.ManagedMCPType,
				"resource_id": "42",
				"content_id":  "content-42",
				"auth_type":   knowledgebase.ManagedAuthType,
			},
		},
	}
}

func assertManagedKnowledgeBaseMCPRuntimeSnapshot(t *testing.T, raw any, endpoint, token string) {
	t.Helper()
	config, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("MCP config = %#v, want object", raw)
	}
	if got := config["url"]; got != endpoint {
		t.Fatalf("MCP URL = %#v, want %q", got, endpoint)
	}
	headers, ok := config["headers"].(map[string]any)
	if !ok || headers["Authorization"] != "Bearer "+token {
		t.Fatalf("MCP headers = %#v, want current token", config["headers"])
	}
	if _, managed := knowledgebase.ManagedMetadataFromServer(config); !managed {
		t.Fatalf("MCP config lost managed metadata: %#v", config)
	}
}

type memoryMCPServerStoreForAPI struct {
	writes int
}

func (*memoryMCPServerStoreForAPI) ReadServers(context.Context) (map[string]any, error) {
	return map[string]any{}, nil
}

func (s *memoryMCPServerStoreForAPI) WriteServers(context.Context, map[string]any) error {
	s.writes++
	return nil
}
