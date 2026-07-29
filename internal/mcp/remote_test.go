package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListRemoteServersUsesAgentMCPHubEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/v1/agent/mcp-servers"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("page"), "2"; got != want {
			t.Fatalf("page = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("per"), "12"; got != want {
			t.Fatalf("per = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("search"), "github"; got != want {
			t.Fatalf("search = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer user-token"; got != want {
			t.Fatalf("Authorization = %q, want %q", got, want)
		}
		_, _ = fmt.Fprint(w, `{"msg":"OK","data":{"data":[
			{"id":"builtin:github","name":"github","description":"GitHub tools"},
			{"id":"broken","name":"broken"}
		],"total":14}}`)
	}))
	t.Cleanup(server.Close)

	page, err := ListRemoteServers(context.Background(), server.URL+"/api", "user-token", RemoteServerListOptions{
		Page:   2,
		Per:    12,
		Search: "github",
	})
	if err != nil {
		t.Fatalf("ListRemoteServers() error = %v", err)
	}
	if got, want := len(page.Items), 2; got != want {
		t.Fatalf("len(Items) = %d, want %d", got, want)
	}
	if page.Total == nil || *page.Total != 14 {
		t.Fatalf("Total = %#v, want 14", page.Total)
	}
	item := page.Items[0]
	if item.Name != "github" || item.Description != "GitHub tools" || item.URL != "" || item.Protocol != "streamable-http" {
		t.Fatalf("item = %#v, want summary without resolved URL", item)
	}
}

func TestListRemoteServersPreservesExactNumericIDs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"data":[
			{"id":9007199254740993,"name":"events"},
			{"id":1.5,"name":"fractional"},
			{"id":18446744073709551616,"name":"out-of-range"}
		],"total":3}`)
	}))
	t.Cleanup(server.Close)

	page, err := ListRemoteServers(context.Background(), server.URL, "", RemoteServerListOptions{})
	if err != nil {
		t.Fatalf("ListRemoteServers() error = %v", err)
	}
	if got, want := len(page.Items), 1; got != want {
		t.Fatalf("len(Items) = %d, want %d", got, want)
	}
	item := page.Items[0]
	if item.ID != "9007199254740993" || item.Name != "events" {
		t.Fatalf("item = %#v, want exact numeric id and name", item)
	}
	if got, want := page.RecordCount, 3; got != want {
		t.Fatalf("RecordCount = %d, want %d", got, want)
	}
}

func TestGetRemoteServerResolvesCompleteConfiguration(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/v1/agent/mcp-servers/builtin:11"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer user-token"; got != want {
			t.Fatalf("Authorization = %q, want %q", got, want)
		}
		_, _ = fmt.Fprint(w, `{"data":{"data":{"id":"builtin:11","name":"gitlab","description":"GitLab project tools","protocol":"sse","url":"https://mcp.example.test/gitlab/sse","headers":{"Gitlab-Access-Token":"test-token","Gitlab-Url":"https://gitlab.example.test"}}}}`)
	}))
	t.Cleanup(server.Close)

	item, err := GetRemoteServer(context.Background(), server.URL, "user-token", "builtin:11")
	if err != nil {
		t.Fatalf("GetRemoteServer() error = %v", err)
	}
	if item.ID != "builtin:11" || item.Name != "gitlab" || item.Protocol != "sse" {
		t.Fatalf("item = %#v, want resolved gitlab server", item)
	}
	config := item.Config()
	if got, want := config["url"], "https://mcp.example.test/gitlab/sse"; got != want {
		t.Fatalf("config.url = %#v, want %q", got, want)
	}
	if got, want := config["transport"], "sse"; got != want {
		t.Fatalf("config.transport = %#v, want %q", got, want)
	}
	if got, want := config["description"], "GitLab project tools"; got != want {
		t.Fatalf("config.description = %#v, want %q", got, want)
	}
	if got, want := config["enabled"], true; got != want {
		t.Fatalf("config.enabled = %#v, want %#v", got, want)
	}
	if got, want := config["startup_timeout_sec"], remoteServerStartupTimeout; got != want {
		t.Fatalf("config.startup_timeout_sec = %#v, want %#v", got, want)
	}
	if got, want := config["tool_timeout_sec"], remoteServerToolTimeout; got != want {
		t.Fatalf("config.tool_timeout_sec = %#v, want %#v", got, want)
	}
	headers, ok := config["headers"].(map[string]any)
	if !ok || headers["Gitlab-Access-Token"] != "test-token" {
		t.Fatalf("config.headers = %#v, want resolved headers", config["headers"])
	}
}
