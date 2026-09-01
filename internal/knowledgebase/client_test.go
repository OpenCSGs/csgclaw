package knowledgebase

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestClientListUsesCurrentUserBearerAndServerSideLLMWikiFilter(t *testing.T) {
	client := Client{
		BaseURL: "https://hub.example.test",
		Token:   "current-user-token",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if got, want := req.URL.Path, "/api/v1/agent/knowledge-bases"; got != want {
				t.Fatalf("path = %q, want %q", got, want)
			}
			if got, want := req.URL.Query().Get("type"), TypeLLMWiki; got != want {
				t.Fatalf("type = %q, want %q", got, want)
			}
			if got, want := req.URL.Query().Get("search"), "handbook"; got != want {
				t.Fatalf("search = %q, want %q", got, want)
			}
			if got, want := req.Header.Get("Authorization"), "Bearer current-user-token"; got != want {
				t.Fatalf("authorization = %q, want %q", got, want)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{
					"data":[
						{"id":42,"name":" Handbook ","content_id":"kb-42","metadata":{"mcp_endpoint_url":" https://gateway.example.test/v1/llmwikis/kb-42/mcp ","resource_state":{"readiness":"ready","mcp_status":"ready"}},"remote_only":{"status":"kept"}}
					],
					"total":1,
					"request_id":"remote-request-123"
				}`)),
			}, nil
		})},
	}

	result, err := client.List(context.Background(), ListOptions{Page: 2, Per: 12, Search: " handbook "})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Name != "Handbook" {
		t.Fatalf("List() items = %#v", result.Items)
	}
	if got, want := result.Items[0].Metadata.MCPEndpoint, "https://gateway.example.test/v1/llmwikis/kb-42/mcp"; got != want {
		t.Fatalf("mcp_endpoint_url = %q, want %q", got, want)
	}
	if got, want := len(result.RawItems), 1; got != want {
		t.Fatalf("raw item count = %d, want %d", got, want)
	}
	var rawItem map[string]any
	if err := json.Unmarshal(result.RawItems[0], &rawItem); err != nil {
		t.Fatalf("decode raw item: %v", err)
	}
	remoteOnly := rawItem["remote_only"].(map[string]any)
	if got, want := remoteOnly["status"], "kept"; got != want {
		t.Fatalf("raw item remote_only.status = %#v, want %q", got, want)
	}
}
