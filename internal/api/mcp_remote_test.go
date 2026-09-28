package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/auth"
	"csgclaw/internal/mcp"
)

type remoteMCPTestProber struct {
	delay time.Duration
	err   error
}

func (p remoteMCPTestProber) Probe(context.Context, string, map[string]any) (mcp.ProbeResult, error) {
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	if p.err != nil {
		return mcp.ProbeResult{}, p.err
	}
	return mcp.ProbeResult{Connected: true}, nil
}

func TestHandleRemoteMCPServersUsesConfiguredOfficialHub(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/v1/agent/mcp-servers"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("page"), "2"; got != want {
			t.Fatalf("page = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("per"), "12"; got != want {
			t.Fatalf("per = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("search"), "calendar"; got != want {
			t.Fatalf("search = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer hub-token"; got != want {
			t.Fatal("unexpected fixture authorization")
		}
		_, _ = fmt.Fprint(w, `{"msg":"OK","data":{"data":[{"id":"builtin:calendar","name":"calendar","description":"Calendar tools"}],"total":25}}`)
	}))
	t.Cleanup(remote.Close)
	t.Setenv("CSGHUB_API_BASE_URL", remote.URL)
	t.Setenv("CSGHUB_USER_TOKEN", "hub-token")
	previousToken := remoteMCPHubAccessToken
	remoteMCPHubAccessToken = func() (string, error) { return "hub-token", nil }
	t.Cleanup(func() { remoteMCPHubAccessToken = previousToken })
	t.Cleanup(stubAuthStatus(func(*http.Request) (auth.Status, error) { return auth.Status{}, nil }))

	configPath := filepath.Join(t.TempDir(), "config.toml")
	configText := `[server]
listen_addr = "127.0.0.1:18080"

[[hub.registries]]
name = "official"
kind = "remote"
url = "` + remote.URL + `"
token = "hub-token"
enabled = true
`
	if err := os.WriteFile(configPath, []byte(configText), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	handler := &Handler{}
	handler.SetConfigPath(configPath)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/mcp-servers/remote?page=2&per=12&search=calendar", nil)
	handler.Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response remoteMCPServersListResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Total == nil || *response.Total != 25 || response.NextPage == nil || *response.NextPage != 3 {
		t.Fatalf("page response = %#v, want total 25 and next page 3", response)
	}
	if got, want := len(response.Items), 1; got != want {
		t.Fatalf("len(items) = %d, want %d", got, want)
	}
	item := response.Items[0]
	if item.ID != "builtin:calendar" || item.Name != "calendar" || item.Description != "Calendar tools" || item.URL != "" {
		t.Fatalf("item = %#v, want remote summary without configuration", item)
	}
}

func TestHandleRemoteMCPServersKeepsPagingAfterFilteringMalformedSummaries(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Query().Get("page"), "1"; got != want {
			t.Fatalf("page = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("per"), "2"; got != want {
			t.Fatalf("per = %q, want %q", got, want)
		}
		_, _ = fmt.Fprint(w, `{"data":[{"id":"builtin:calendar","name":"calendar"},{"name":"missing-id"}]}`)
	}))
	t.Cleanup(remote.Close)
	t.Setenv("CSGHUB_API_BASE_URL", remote.URL)
	t.Setenv("CSGHUB_USER_TOKEN", "hub-token")
	previousToken := remoteMCPHubAccessToken
	remoteMCPHubAccessToken = func() (string, error) { return "hub-token", nil }
	t.Cleanup(func() { remoteMCPHubAccessToken = previousToken })
	t.Cleanup(stubAuthStatus(func(*http.Request) (auth.Status, error) { return auth.Status{}, nil }))

	configPath := filepath.Join(t.TempDir(), "config.toml")
	configText := `[server]
listen_addr = "127.0.0.1:18080"

[[hub.registries]]
name = "official"
kind = "remote"
url = "` + remote.URL + `"
token = "hub-token"
enabled = true
`
	if err := os.WriteFile(configPath, []byte(configText), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	handler := &Handler{}
	handler.SetConfigPath(configPath)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/mcp-servers/remote?per=2", nil)
	handler.Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response remoteMCPServersListResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got, want := len(response.Items), 1; got != want {
		t.Fatalf("len(items) = %d, want %d", got, want)
	}
	if response.NextPage == nil || *response.NextPage != 2 {
		t.Fatalf("NextPage = %#v, want 2", response.NextPage)
	}
}

func TestHandleRemoteMCPServersMapsUpstreamUnauthorizedToLoginRequired(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "expired token", http.StatusUnauthorized)
	}))
	t.Cleanup(remote.Close)
	t.Setenv("CSGHUB_API_BASE_URL", remote.URL)
	previousToken := remoteMCPHubAccessToken
	remoteMCPHubAccessToken = func() (string, error) { return "expired-token", nil }
	t.Cleanup(func() { remoteMCPHubAccessToken = previousToken })
	t.Cleanup(stubAuthStatus(func(*http.Request) (auth.Status, error) { return auth.Status{}, nil }))

	handler := &Handler{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/mcp-servers/remote", nil)
	handler.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}
	if got, want := strings.TrimSpace(recorder.Body.String()), errRemoteMCPHubSignInRequired.Error(); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestHandleInstallRemoteMCPServerResolvesDetailsServerSide(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/v1/agent/mcp-servers/builtin:calendar"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer hub-token"; got != want {
			t.Fatalf("Authorization = %q, want %q", got, want)
		}
		_, _ = fmt.Fprint(w, `{"data":{"id":"builtin:calendar","name":"calendar-mcp","description":"Calendar tools","protocol":"sse","url":"https://mcp.example.test/calendar/sse","headers":{"Authorization":"test-secret"}}}`)
	}))
	t.Cleanup(remote.Close)
	t.Setenv("CSGHUB_API_BASE_URL", remote.URL)
	t.Setenv("CSGHUB_USER_TOKEN", "hub-token")
	previousToken := remoteMCPHubAccessToken
	remoteMCPHubAccessToken = func() (string, error) { return "hub-token", nil }
	t.Cleanup(func() { remoteMCPHubAccessToken = previousToken })
	t.Cleanup(stubAuthStatus(func(*http.Request) (auth.Status, error) { return auth.Status{}, nil }))

	configPath := filepath.Join(t.TempDir(), "config.toml")
	configText := `[server]
listen_addr = "127.0.0.1:18080"

[[hub.registries]]
name = "official"
kind = "remote"
url = "` + remote.URL + `"
token = "hub-token"
enabled = true
`
	if err := os.WriteFile(configPath, []byte(configText), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	handler := &Handler{mcp: mcp.NewService(mcp.WithServerProber(remoteMCPTestProber{}))}
	handler.SetConfigPath(configPath)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/mcp-servers/remote/builtin:calendar/install", nil)
	handler.Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response remoteMCPServerInstallResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Name != "calendar-mcp" {
		t.Fatalf("name = %q, want resolved server name", response.Name)
	}

	state, err := handler.mcp.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers() error = %v", err)
	}
	stored, ok := state["calendar-mcp"].(map[string]any)
	if !ok {
		t.Fatalf("stored MCP servers = %#v, want calendar-mcp", state)
	}
	if got, want := stored["description"], "Calendar tools"; got != want {
		t.Fatalf("stored.description = %#v, want %q", got, want)
	}
	headers, ok := stored["headers"].(map[string]any)
	if !ok || headers["Authorization"] != "test-secret" {
		t.Fatalf("stored.headers = %#v, want server-side header", stored["headers"])
	}
}

func TestHandleInstallRemoteMCPServerTimesOutEntireInstallation(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(remote.Close)
	t.Setenv("CSGHUB_API_BASE_URL", remote.URL)
	previousToken := remoteMCPHubAccessToken
	remoteMCPHubAccessToken = func() (string, error) { return "hub-token", nil }
	t.Cleanup(func() { remoteMCPHubAccessToken = previousToken })
	previousTimeout := remoteMCPInstallTimeout
	remoteMCPInstallTimeout = 50 * time.Millisecond
	t.Cleanup(func() { remoteMCPInstallTimeout = previousTimeout })
	t.Cleanup(stubAuthStatus(func(*http.Request) (auth.Status, error) { return auth.Status{}, nil }))

	handler := &Handler{mcp: mcp.NewService(mcp.WithServerProber(remoteMCPTestProber{}))}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/mcp-servers/remote/slow/install", nil)
	started := time.Now()
	handler.Routes().ServeHTTP(recorder, request)

	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("installation elapsed = %v, want bounded by request timeout", elapsed)
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusGatewayTimeout, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"remote_mcp_install_failed"`) {
		t.Fatalf("body = %s, want remote_mcp_install_failed", recorder.Body.String())
	}
}

func TestHandleInstallRemoteMCPServerHardDeadlineRejectsLateProbe(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"data":{"id":"slow","name":"slow-mcp","protocol":"streamable-http","url":"https://mcp.example.test/slow"}}`)
	}))
	t.Cleanup(remote.Close)
	t.Setenv("CSGHUB_API_BASE_URL", remote.URL)
	previousToken := remoteMCPHubAccessToken
	remoteMCPHubAccessToken = func() (string, error) { return "hub-token", nil }
	t.Cleanup(func() { remoteMCPHubAccessToken = previousToken })
	previousTimeout := remoteMCPInstallTimeout
	remoteMCPInstallTimeout = 50 * time.Millisecond
	t.Cleanup(func() { remoteMCPInstallTimeout = previousTimeout })
	t.Cleanup(stubAuthStatus(func(*http.Request) (auth.Status, error) { return auth.Status{}, nil }))

	service := mcp.NewService(mcp.WithServerProber(remoteMCPTestProber{delay: 250 * time.Millisecond}))
	handler := &Handler{mcp: service}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/mcp-servers/remote/slow/install", nil)
	started := time.Now()
	handler.Routes().ServeHTTP(recorder, request)

	if elapsed := time.Since(started); elapsed >= 200*time.Millisecond {
		t.Fatalf("installation elapsed = %v, want hard response deadline", elapsed)
	}
	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusGatewayTimeout, recorder.Body.String())
	}
	time.Sleep(300 * time.Millisecond)
	servers, err := service.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers() error = %v", err)
	}
	if len(servers) != 0 {
		t.Fatalf("late probe persisted MCP servers after timeout: %#v", servers)
	}
}
