package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/auth"
	"csgclaw/internal/knowledgebase"
	"csgclaw/internal/mcp"
)

type knowledgeBaseRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn knowledgeBaseRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type knowledgeBaseServerStore struct {
	servers map[string]any
}

func (s *knowledgeBaseServerStore) ReadServers(context.Context) (map[string]any, error) {
	return s.servers, nil
}

func (s *knowledgeBaseServerStore) WriteServers(_ context.Context, servers map[string]any) error {
	s.servers = servers
	return nil
}

func TestHandleRemoteKnowledgeBasesRequiresSignInBeforeAgenticHubRequest(t *testing.T) {
	originalLoader := loadKnowledgeBaseConnection
	defer func() { loadKnowledgeBaseConnection = originalLoader }()
	loadKnowledgeBaseConnection = func(context.Context) (knowledgeBaseConnection, error) {
		return knowledgeBaseConnection{}, errKnowledgeBaseSignInRequired
	}

	recorder := httptest.NewRecorder()
	(&Handler{}).handleRemoteKnowledgeBases(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases/remote", nil))
	if got, want := recorder.Code, http.StatusUnauthorized; got != want {
		t.Fatalf("status = %d, want %d; body=%s", got, want, recorder.Body.String())
	}
}

func TestLoadKnowledgeBaseConnectionUsesManagedCommunityRunnerIdentity(t *testing.T) {
	originalInteractiveLoader := loadInteractiveKnowledgeBaseConnection
	originalLoader := loadKnowledgeBaseConnection
	defer func() {
		loadInteractiveKnowledgeBaseConnection = originalInteractiveLoader
		loadKnowledgeBaseConnection = originalLoader
	}()
	loadInteractiveKnowledgeBaseConnection = func(context.Context) (knowledgeBaseConnection, bool, error) {
		return knowledgeBaseConnection{}, false, nil
	}
	t.Setenv("CSGHUB_API_BASE_URL", "https://hub.example.test")
	t.Setenv("CSGHUB_USER_TOKEN", "community-runner-token")
	t.Setenv("CSGHUB_AIGATEWAY_BASE_URL", "")
	t.Setenv("CSGHUB_AIGATEWAY_URL", "https://gateway.example.test")
	t.Setenv("CSGCLAW_LLM_BASE_URL", "")

	connection, err := loadKnowledgeBaseConnection(context.Background())
	if err != nil {
		t.Fatalf("loadKnowledgeBaseConnection() error = %v", err)
	}
	if got, want := connection.CSGHubBaseURL, "https://hub.example.test"; got != want {
		t.Fatalf("CSGHubBaseURL = %q, want %q", got, want)
	}
	if got, want := connection.AIGatewayBaseURL, "https://gateway.example.test/v1"; got != want {
		t.Fatalf("AIGatewayBaseURL = %q, want %q", got, want)
	}
	if got, want := connection.CSGHubAccessToken, "community-runner-token"; got != want {
		t.Fatalf("CSGHubAccessToken = %q, want %q", got, want)
	}
}

func TestLoadKnowledgeBaseConnectionPrefersInteractiveIdentity(t *testing.T) {
	originalInteractiveLoader := loadInteractiveKnowledgeBaseConnection
	originalLoader := loadKnowledgeBaseConnection
	defer func() {
		loadInteractiveKnowledgeBaseConnection = originalInteractiveLoader
		loadKnowledgeBaseConnection = originalLoader
	}()
	loadInteractiveKnowledgeBaseConnection = func(context.Context) (knowledgeBaseConnection, bool, error) {
		return knowledgeBaseConnection{
			CSGHubBaseURL:     "https://interactive-hub.example.test",
			AIGatewayBaseURL:  "https://interactive-gateway.example.test/v1",
			CSGHubAccessToken: "interactive-token",
		}, true, nil
	}
	t.Setenv("CSGHUB_API_BASE_URL", "https://managed-hub.example.test")
	t.Setenv("CSGHUB_USER_TOKEN", "managed-token")

	connection, err := loadKnowledgeBaseConnection(context.Background())
	if err != nil {
		t.Fatalf("loadKnowledgeBaseConnection() error = %v", err)
	}
	if got, want := connection.CSGHubAccessToken, "interactive-token"; got != want {
		t.Fatalf("CSGHubAccessToken = %q, want %q", got, want)
	}
}

func TestManagedKnowledgeBaseConnectionMapsStagingAIGateway(t *testing.T) {
	t.Setenv("CSGHUB_API_BASE_URL", auth.StageCSGHubBaseURL)
	t.Setenv("CSGHUB_USER_TOKEN", "managed-token")
	t.Setenv("CSGHUB_AIGATEWAY_BASE_URL", "")
	t.Setenv("CSGHUB_AIGATEWAY_URL", "")
	t.Setenv("CSGCLAW_LLM_BASE_URL", "")

	connection, ok := managedKnowledgeBaseConnection()
	if !ok {
		t.Fatal("managedKnowledgeBaseConnection() = false")
	}
	if got, want := connection.AIGatewayBaseURL, auth.StageAIGatewayBaseURL; got != want {
		t.Fatalf("AIGatewayBaseURL = %q, want %q", got, want)
	}
}

func TestHandleRemoteKnowledgeBasesReportsAvailabilityAndConfiguredMCP(t *testing.T) {
	originalLoader := loadKnowledgeBaseConnection
	originalTransport := http.DefaultTransport
	defer func() {
		loadKnowledgeBaseConnection = originalLoader
		http.DefaultTransport = originalTransport
	}()
	loadKnowledgeBaseConnection = func(context.Context) (knowledgeBaseConnection, error) {
		return knowledgeBaseConnection{CSGHubBaseURL: "https://hub.example.test", CSGHubAccessToken: "user-token"}, nil
	}
	http.DefaultTransport = knowledgeBaseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got, want := req.Header.Get("Authorization"), "Bearer user-token"; got != want {
			t.Fatalf("authorization = %q, want %q", got, want)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":42,"name":"Handbook","description":"Runbooks","content_id":"content-42","type":"llmwiki","metadata":{"mcp_endpoint_url":"https://gateway.example.test/v1/llmwikis/content-42/mcp","resource_state":{"readiness":"ready","mcp_status":"ready"}},"remote_only":{"status":"kept"}}],"total":1,"request_id":"remote-request-123"}`)),
		}, nil
	})
	store := &knowledgeBaseServerStore{servers: map[string]any{
		"content-42": map[string]any{
			"type": "remote",
			"url":  "https://example.test/mcp",
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
	recorder := httptest.NewRecorder()
	handler.handleRemoteKnowledgeBases(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases/remote", nil))
	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d; body=%s", got, want, recorder.Body.String())
	}
	var response remoteKnowledgeBasesListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Items) != 1 || response.Items[0].ConfiguredMCP != "content-42" {
		t.Fatalf("items = %#v", response.Items)
	}
	var csgHubResponse map[string]any
	if err := json.Unmarshal(response.Items[0].CSGHubResponse, &csgHubResponse); err != nil {
		t.Fatalf("decode csghub_response: %v", err)
	}
	if got, want := csgHubResponse["id"], float64(42); got != want {
		t.Fatalf("csghub_response id = %#v, want %#v", got, want)
	}
	remoteOnly := csgHubResponse["remote_only"].(map[string]any)
	if got, want := remoteOnly["status"], "kept"; got != want {
		t.Fatalf("csghub_response remote_only.status = %#v, want %q", got, want)
	}
}

func TestKnowledgeBaseMCPProxyUsesCurrentUserTokenWithoutForwardingCookies(t *testing.T) {
	originalLoader := loadKnowledgeBaseConnection
	originalClient := knowledgeBaseProxyHTTPClient
	defer func() {
		loadKnowledgeBaseConnection = originalLoader
		knowledgeBaseProxyHTTPClient = originalClient
	}()
	loadKnowledgeBaseConnection = func(context.Context) (knowledgeBaseConnection, error) {
		return knowledgeBaseConnection{
			AIGatewayBaseURL:  "https://gateway.example.test/v1",
			CSGHubAccessToken: "current-user-token",
		}, nil
	}
	knowledgeBaseProxyHTTPClient = &http.Client{Transport: knowledgeBaseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got, want := req.URL.String(), "https://gateway.example.test/v1/llmwikis/content-42/mcp"; got != want {
			t.Fatalf("url = %q, want %q", got, want)
		}
		if got, want := req.Header.Get("Authorization"), "Bearer current-user-token"; got != want {
			t.Fatalf("authorization = %q, want %q", got, want)
		}
		if cookie := req.Header.Get("Cookie"); cookie != "" {
			t.Fatalf("cookie was forwarded: %q", cookie)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"},
				"Location":     []string{"https://gateway.example.test/private"},
				"Set-Cookie":   []string{"secret=1"},
			},
			Body: io.NopCloser(strings.NewReader("event: ready\n\ndata: ok\n\n")),
		}, nil
	})}

	handler := &Handler{serverAccessToken: "local-server-token"}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge-bases/content-42/mcp", strings.NewReader("{}"))
	request.SetPathValue("content_id", "content-42")
	request.Header.Set("Authorization", "Bearer local-server-token")
	request.Header.Set("Cookie", "browser-secret=1")
	recorder := httptest.NewRecorder()
	handler.handleKnowledgeBaseMCPProxy(recorder, request)
	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d; body=%s", got, want, recorder.Body.String())
	}
	if cookie := recorder.Header().Get("Set-Cookie"); cookie != "" {
		t.Fatalf("Set-Cookie was forwarded: %q", cookie)
	}
	if location := recorder.Header().Get("Location"); location != "" {
		t.Fatalf("Location was forwarded: %q", location)
	}
}

func TestKnowledgeBaseMCPProxyUsesCommunityTemplateRunnerToken(t *testing.T) {
	originalInteractiveLoader := loadInteractiveKnowledgeBaseConnection
	originalLoader := loadKnowledgeBaseConnection
	originalClient := knowledgeBaseProxyHTTPClient
	defer func() {
		loadInteractiveKnowledgeBaseConnection = originalInteractiveLoader
		loadKnowledgeBaseConnection = originalLoader
		knowledgeBaseProxyHTTPClient = originalClient
	}()
	loadInteractiveKnowledgeBaseConnection = func(context.Context) (knowledgeBaseConnection, bool, error) {
		return knowledgeBaseConnection{}, false, nil
	}
	t.Setenv("CSGHUB_API_BASE_URL", "https://hub.example.test")
	t.Setenv("CSGHUB_USER_TOKEN", "community-runner-token")
	t.Setenv("CSGHUB_AIGATEWAY_BASE_URL", "https://gateway.example.test/v1")

	knowledgeBaseProxyHTTPClient = &http.Client{Transport: knowledgeBaseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got, want := req.URL.String(), "https://gateway.example.test/v1/llmwikis/content-42/mcp"; got != want {
			t.Fatalf("url = %q, want %q", got, want)
		}
		if got, want := req.Header.Get("Authorization"), "Bearer community-runner-token"; got != want {
			t.Fatalf("authorization = %q, want %q", got, want)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":{}}`)),
		}, nil
	})}

	handler := &Handler{serverAccessToken: "local-server-token"}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge-bases/content-42/mcp", strings.NewReader(`{}`))
	request.SetPathValue("content_id", "content-42")
	request.Header.Set("Authorization", "Bearer local-server-token")
	recorder := httptest.NewRecorder()
	handler.handleKnowledgeBaseMCPProxy(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d; body=%s", got, want, recorder.Body.String())
	}
}
