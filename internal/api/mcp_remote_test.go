package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/mcp"
)

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
			t.Fatalf("Authorization = %q, want %q", got, want)
		}
		_, _ = fmt.Fprint(w, `{"msg":"OK","data":{"data":[{"id":"builtin:calendar","name":"calendar","description":"Calendar tools"}],"total":25}}`)
	}))
	t.Cleanup(remote.Close)

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
	handler := &Handler{mcp: mcp.NewService()}
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
