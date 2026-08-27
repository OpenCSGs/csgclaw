package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
		"name":"agentichub-kb-42",
		"config":{
			"type":"remote",
			"url":"https://gateway.example.test/v1/gateway/mcp",
			"transport":"streamable-http",
			"headers":{"Authorization":"Bearer draft-csghub-token"},
			"csgclaw":{
				"kind":"agentichub_knowledge_base",
				"knowledge_base_id":"42",
				"content_id":"content-42"
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
