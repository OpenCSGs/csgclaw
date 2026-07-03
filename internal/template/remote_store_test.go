package template

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/config"
)

const remoteTestManifest = `name = "gitlab-assistant"
role = "worker"
description = "GitLab assistant"
runtime_kind = "openclaw"
version = "2026.6.16.0"
updated_at = "2026-05-19T07:25:31Z"

[image]
ref = "registry.example.com/openclaw-glab:2026.6.16.0"

[[image.env]]
name = "GITLAB_TOKEN"
required = true
secret = true
description = "GitLab personal access token"
`

func TestRemoteStoreListGetAndFetchWorkspace(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/organization/Agentic/codes":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{
					"name":           "gitlab-assistant",
					"nickname":       "gitlab-assistant",
					"description":    "repository description",
					"path":           "Agentic/gitlab-assistant",
					"default_branch": "",
					"updated_at":     "2026-06-25T02:00:02Z",
				}},
				"total": 1,
			})
		case r.URL.Path == "/api/v1/codes/Agentic/gitlab-assistant":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"name":           "gitlab-assistant",
					"path":           "Agentic/gitlab-assistant",
					"default_branch": "main",
				},
			})
		case r.URL.Path == "/api/v1/codes/Agentic/gitlab-assistant/blob/agent.toml":
			assertQueryValue(t, r.URL, "ref", "main")
			writeRemoteBlob(t, w, "agent.toml", []byte(remoteTestManifest))
		case r.URL.Path == "/api/v1/codes/Agentic/gitlab-assistant/refs/main/tree/workspace":
			assertQueryValue(t, r.URL, "limit", "500")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"Files": []map[string]any{
						{"name": "AGENTS.md", "type": "file", "path": "workspace/AGENTS.md"},
						{"name": "skills", "type": "dir", "path": "workspace/skills"},
					},
					"Cursor": "",
				},
			})
		case r.URL.Path == "/api/v1/codes/Agentic/gitlab-assistant/refs/main/tree/workspace/skills":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"Files": []map[string]any{
						{"name": "review.md", "type": "file", "path": "workspace/skills/review.md"},
					},
					"Cursor": "",
				},
			})
		case r.URL.Path == "/api/v1/codes/Agentic/gitlab-assistant/blob/workspace/AGENTS.md":
			writeRemoteBlob(t, w, "workspace/AGENTS.md", []byte("hello"))
		case r.URL.Path == "/api/v1/codes/Agentic/gitlab-assistant/blob/workspace/skills/review.md":
			writeRemoteBlob(t, w, "workspace/skills/review.md", []byte("review"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	store := NewRemoteStore(srv.URL, "")
	items, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := len(items), 1; got != want {
		t.Fatalf("len(List()) = %d, want %d", got, want)
	}
	if got, want := items[0].ID, "gitlab-assistant"; got != want {
		t.Fatalf("List()[0].ID = %q, want %q", got, want)
	}
	if got, want := items[0].RuntimeKind, "openclaw"; got != want {
		t.Fatalf("List()[0].RuntimeKind = %q, want %q", got, want)
	}

	item, err := store.Get(context.Background(), "gitlab-assistant")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got, want := item.Description, "GitLab assistant"; got != want {
		t.Fatalf("Get().Description = %q, want %q", got, want)
	}
	if got, want := item.Role, TemplateRoleWorker; got != want {
		t.Fatalf("Get().Role = %q, want %q", got, want)
	}
	if got, want := item.ImageEnv[0].Name, "GITLAB_TOKEN"; got != want {
		t.Fatalf("Get().ImageEnv[0].Name = %q, want %q", got, want)
	}

	listing, err := store.ListWorkspace(context.Background(), "gitlab-assistant", "")
	if err != nil {
		t.Fatalf("ListWorkspace() error = %v", err)
	}
	if got, want := len(listing.Entries), 2; got != want {
		t.Fatalf("len(ListWorkspace().Entries) = %d, want %d", got, want)
	}
	if got, want := listing.Entries[0].Path, "AGENTS.md"; got != want {
		t.Fatalf("ListWorkspace().Entries[0].Path = %q, want %q", got, want)
	}

	file, err := store.ReadWorkspaceFile(context.Background(), "gitlab-assistant", "AGENTS.md")
	if err != nil {
		t.Fatalf("ReadWorkspaceFile() error = %v", err)
	}
	if got, want := file.Content, "hello"; got != want {
		t.Fatalf("ReadWorkspaceFile().Content = %q, want %q", got, want)
	}

	workspace, err := store.FetchWorkspace(context.Background(), "Agentic/gitlab-assistant")
	if err != nil {
		t.Fatalf("FetchWorkspace() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workspace.Path) })

	if got, want := workspace.Kind, WorkspaceKindDir; got != want {
		t.Fatalf("FetchWorkspace().Kind = %q, want %q", got, want)
	}
	for name, want := range map[string]string{
		"AGENTS.md":        "hello",
		"skills/review.md": "review",
	} {
		data, err := os.ReadFile(filepath.Join(workspace.Path, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read extracted %s: %v", name, err)
		}
		if got := string(data); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestRemoteStoreListSkipsInvalidRepositories(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/organization/Agentic/codes":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"name":           "gitlab-assistant",
						"path":           "Agentic/gitlab-assistant",
						"default_branch": "main",
					},
					{
						"name":           "broken-manifest",
						"path":           "Agentic/broken-manifest",
						"default_branch": "main",
					},
					{
						"name":           "other-namespace",
						"path":           "Other/other-namespace",
						"default_branch": "main",
					},
				},
				"total": 3,
			})
		case r.URL.Path == "/api/v1/codes/Agentic/gitlab-assistant/blob/agent.toml":
			writeRemoteBlob(t, w, "agent.toml", []byte(remoteTestManifest))
		case r.URL.Path == "/api/v1/codes/Agentic/broken-manifest/blob/agent.toml":
			writeRemoteBlob(t, w, "agent.toml", []byte("name = \"broken-manifest\"\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	store := NewRemoteStore(srv.URL, "")
	items, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := len(items), 1; got != want {
		t.Fatalf("len(List()) = %d, want %d; items=%#v", got, want, items)
	}
	if got, want := items[0].ID, "gitlab-assistant"; got != want {
		t.Fatalf("List()[0].ID = %q, want %q", got, want)
	}
}

func TestDefaultStoreFactoryCreatesRemoteStore(t *testing.T) {
	t.Parallel()

	store, err := DefaultStoreFactory(config.HubRegistryConfig{
		Kind:  RegistryKindRemote,
		URL:   "https://hub.opencsg.com",
		Token: "secret",
	})
	if err != nil {
		t.Fatalf("DefaultStoreFactory() error = %v", err)
	}
	remote, ok := store.(*RemoteStore)
	if !ok {
		t.Fatalf("store type = %T, want *RemoteStore", store)
	}
	if got, want := remote.contentBaseURL, "https://hub.opencsg.com"; got != want {
		t.Fatalf("content base URL = %q, want %q", got, want)
	}
}

func TestRemoteStoreGetJSONRejectsOversizedResponse(t *testing.T) {
	t.Parallel()

	oversized := make([]byte, defaultRemoteMaxJSONBytes+1)
	for i := range oversized {
		oversized[i] = ' '
	}
	oversized[0] = '{'
	oversized[len(oversized)-1] = '}'

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/organization/Agentic/codes" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(oversized)
	}))
	t.Cleanup(srv.Close)

	store := NewRemoteStore(srv.URL, "")
	_, err := store.List(context.Background())
	if err == nil {
		t.Fatal("List() error = nil, want oversized response error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("List() error = %v, want response size limit message", err)
	}
}

func TestDefaultStoreFactoryRemoteRequiresURL(t *testing.T) {
	t.Parallel()

	_, err := DefaultStoreFactory(config.HubRegistryConfig{Kind: RegistryKindRemote})
	if !errors.Is(err, ErrRegistryURLRequired) {
		t.Fatalf("DefaultStoreFactory() error = %v, want ErrRegistryURLRequired", err)
	}
}

func writeRemoteBlob(t *testing.T, w http.ResponseWriter, name string, data []byte) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]any{
			"path":    name,
			"content": base64.StdEncoding.EncodeToString(data),
		},
	}); err != nil {
		t.Fatalf("encode blob response: %v", err)
	}
}

func assertQueryValue(t *testing.T, values *url.URL, key, want string) {
	t.Helper()
	if got := values.Query().Get(key); got != want {
		t.Fatalf("query %s = %q, want %q", key, got, want)
	}
}
