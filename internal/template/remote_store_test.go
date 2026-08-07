package template

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func TestRemoteStorePublishUploadsArchiveAndCreatesTemplateCode(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("publish me\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var uploadedArchive []byte
	var created remoteCreateCodeRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/codes/upload_url":
			if got, want := r.URL.Query().Get("current_user"), "alice"; got != want {
				t.Errorf("upload current_user = %q, want %q", got, want)
			}
			if got, want := r.Header.Get("Authorization"), "Bearer access-token"; got != want {
				t.Errorf("upload authorization = %q, want %q", got, want)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"url":      "http://" + r.Host + "/object-storage",
				"uuid":     "package-uuid",
				"formData": map[string]string{"key": "codes/package-uuid", "policy": "policy-value"},
			}})
		case "/object-storage":
			if err := r.ParseMultipartForm(4 << 20); err != nil {
				t.Errorf("ParseMultipartForm() error = %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if got, want := r.FormValue("key"), "codes/package-uuid"; got != want {
				t.Errorf("upload key = %q, want %q", got, want)
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Errorf("FormFile() error = %v", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			defer file.Close()
			uploadedArchive, err = io.ReadAll(file)
			if err != nil {
				t.Errorf("ReadAll(upload) error = %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		case "/api/v1/codes":
			if got, want := r.URL.Query().Get("current_user"), "alice"; got != want {
				t.Errorf("create current_user = %q, want %q", got, want)
			}
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Errorf("Decode(create) error = %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"path": "alice/ReviewBot_2",
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	store := NewRemoteStore(srv.URL, "access-token")
	store.username = "alice"
	item, err := store.Publish(context.Background(), PublishSpec{
		ID:          "legacy-internal-id",
		Name:        "ReviewBot_2",
		Description: "Reviews changes",
		Role:        TemplateRoleWorker,
		RuntimeKind: "codex",
		WorkspaceRef: WorkspaceRef{
			Kind:             WorkspaceKindDir,
			Path:             workspace,
			InstructionsPath: filepath.Join(workspace, "AGENTS.md"),
		},
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if got, want := item.ID, "alice/ReviewBot_2"; got != want {
		t.Fatalf("Publish().ID = %q, want %q", got, want)
	}
	if got, want := created.CodeFile, "package-uuid"; got != want {
		t.Fatalf("create code_file = %q, want %q", got, want)
	}
	if got, want := created.Namespace, "alice"; got != want {
		t.Fatalf("create namespace = %q, want %q", got, want)
	}
	if got, want := created.Type, "template"; got != want {
		t.Fatalf("create type = %q, want %q", got, want)
	}
	if got, want := created.Name, "ReviewBot_2"; got != want {
		t.Fatalf("create name = %q, want agent.toml name %q", got, want)
	}
	if got, want := created.Nickname, "ReviewBot_2"; got != want {
		t.Fatalf("create nickname = %q, want agent.toml name %q", got, want)
	}
	if !created.Private {
		t.Fatal("create private = false, want true")
	}
	if got, want := created.Description, "Reviews changes"; got != want {
		t.Fatalf("create description = %q, want agent.toml description %q", got, want)
	}

	archive, err := zip.NewReader(bytes.NewReader(uploadedArchive), int64(len(uploadedArchive)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}
	names := make(map[string]bool, len(archive.File))
	for _, file := range archive.File {
		names[file.Name] = true
		if file.Name == "agent.toml" {
			reader, err := file.Open()
			if err != nil {
				t.Fatalf("Open(agent.toml) error = %v", err)
			}
			manifest, err := io.ReadAll(reader)
			_ = reader.Close()
			if err != nil {
				t.Fatalf("ReadAll(agent.toml) error = %v", err)
			}
			if !strings.Contains(string(manifest), `schema_version = "agentfile/v1"`) {
				t.Errorf("agent.toml missing schema_version: %s", manifest)
			}
			if !strings.Contains(string(manifest), `name = 'ReviewBot_2'`) {
				t.Errorf("agent.toml missing template name: %s", manifest)
			}
			if !strings.Contains(string(manifest), `description = 'Reviews changes'`) {
				t.Errorf("agent.toml missing template description: %s", manifest)
			}
		}
	}
	for _, want := range []string{"agent.toml", "instructions/AGENTS.md"} {
		if !names[want] {
			t.Errorf("uploaded archive missing %q; names = %#v", want, names)
		}
	}
}

func TestRemoteStoreListMergesOrganizationAndAgentTemplatesByNamespacePath(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/organization/Agentic/codes":
			if got, want := r.Header.Get("Authorization"), "Bearer access-token"; got != want {
				t.Errorf("organization templates authorization = %q, want %q", got, want)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{
				"name": "official-bot", "path": "Agentic/official-bot", "default_branch": "main",
			}}})
		case r.URL.Path == "/api/v1/agent/templates":
			if got, want := r.URL.Query().Get("type"), "csgclaw"; got != want {
				t.Errorf("agent template type = %q, want %q", got, want)
			}
			if got, want := r.Header.Get("Authorization"), "Bearer access-token"; got != want {
				t.Errorf("agent templates authorization = %q, want %q", got, want)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{
					"id": 1, "type": "csgclaw", "name": "alice/personal-bot",
					"metadata": map[string]any{
						"repo_path":    "alice/personal-bot",
						"runtime_kind": "codex",
						"agent_file": map[string]any{
							"name": "personal-bot", "role": "worker", "runtime_kind": "codex",
						},
					},
				},
				{"id": 2, "type": "langflow", "name": "Ignored", "metadata": map[string]any{"repo_path": "alice/ignored"}},
			}})
		case r.URL.Path == "/api/v1/codes/alice/personal-bot":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"name": "personal-bot", "path": "alice/personal-bot", "default_branch": "main",
			}})
		case strings.HasSuffix(r.URL.Path, "/blob/agent.toml"):
			name := "Official Bot"
			if strings.Contains(r.URL.Path, "personal-bot") {
				name = "Personal Bot"
			}
			manifest := strings.Replace(remoteTestManifest, `name = "gitlab-assistant"`, `name = "`+name+`"`, 1)
			writeRemoteBlob(t, w, "agent.toml", []byte(manifest))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	items, err := NewRemoteStore(srv.URL, "access-token").List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := len(items), 2; got != want {
		t.Fatalf("len(List()) = %d, want %d", got, want)
	}
	if got, want := items[0].ID, "Agentic/official-bot"; got != want {
		t.Fatalf("List()[0].ID = %q, want %q", got, want)
	}
	if got, want := items[1].ID, "alice/personal-bot"; got != want {
		t.Fatalf("List()[1].ID = %q, want %q", got, want)
	}
}

func TestRemoteStoreListPaginatesAgentTemplates(t *testing.T) {
	t.Parallel()

	const total = remoteAgentTemplatesPerPage + 1
	requestedPages := make([]string, 0, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/organization/Agentic/codes":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
		case r.URL.Path == "/api/v1/agent/templates":
			if got, want := r.URL.Query().Get("per"), "20"; got != want {
				t.Errorf("agent templates per = %q, want %q", got, want)
			}
			page := r.URL.Query().Get("page")
			requestedPages = append(requestedPages, page)
			start, end := 0, remoteAgentTemplatesPerPage
			if page == "2" {
				start, end = remoteAgentTemplatesPerPage, total
			}
			data := make([]map[string]any, 0, end-start)
			for i := start; i < end; i++ {
				path := fmt.Sprintf("alice/template-%02d", i)
				data = append(data, map[string]any{
					"id": i + 1, "type": "csgclaw", "name": path,
					"metadata": map[string]any{"repo_path": path},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "total": total})
		case strings.HasSuffix(r.URL.Path, "/blob/agent.toml"):
			writeRemoteBlob(t, w, "agent.toml", []byte(remoteTestManifest))
		case strings.HasPrefix(r.URL.Path, "/api/v1/codes/alice/template-"):
			path := strings.TrimPrefix(r.URL.Path, "/api/v1/codes/")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"name": path[strings.LastIndex(path, "/")+1:], "path": path, "default_branch": "main",
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	items, err := NewRemoteStore(srv.URL, "access-token").List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := len(items), total; got != want {
		t.Fatalf("len(List()) = %d, want %d", got, want)
	}
	if got, want := strings.Join(requestedPages, ","), "1,2"; got != want {
		t.Fatalf("agent template pages = %q, want %q", got, want)
	}
}

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
		case r.URL.Path == "/api/v1/codes/Agentic/gitlab-assistant/refs/main/tree/":
			assertQueryValue(t, r.URL, "limit", "500")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"Files": []map[string]any{
						{"name": "instructions", "type": "dir", "path": "instructions"},
						{"name": "skills", "type": "dir", "path": "skills"},
					},
					"Cursor": "",
				},
			})
		case r.URL.Path == "/api/v1/codes/Agentic/gitlab-assistant/refs/main/tree/instructions":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"Files": []map[string]any{{"name": "AGENTS.md", "type": "file", "path": "instructions/AGENTS.md"}}, "Cursor": ""}})
		case r.URL.Path == "/api/v1/codes/Agentic/gitlab-assistant/refs/main/tree/skills":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"Files": []map[string]any{
						{"name": "review.md", "type": "file", "path": "skills/review.md"},
					},
					"Cursor": "",
				},
			})
		case r.URL.Path == "/api/v1/codes/Agentic/gitlab-assistant/refs/main/tree/mcps":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"Files": []map[string]any{{"name": "mcp.json", "type": "file", "path": "mcps/mcp.json"}}, "Cursor": ""}})
		case r.URL.Path == "/api/v1/codes/Agentic/gitlab-assistant/refs/main/tree/memories":
			http.NotFound(w, r)
		case r.URL.Path == "/api/v1/codes/Agentic/gitlab-assistant/blob/instructions/AGENTS.md":
			writeRemoteBlob(t, w, "instructions/AGENTS.md", []byte("hello"))
		case r.URL.Path == "/api/v1/codes/Agentic/gitlab-assistant/blob/skills/review.md":
			writeRemoteBlob(t, w, "skills/review.md", []byte("review"))
		case r.URL.Path == "/api/v1/codes/Agentic/gitlab-assistant/blob/mcps/mcp.json":
			writeRemoteBlob(t, w, "mcps/mcp.json", []byte("{}"))
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
	if got, want := items[0].ID, "Agentic/gitlab-assistant"; got != want {
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
	if got, want := listing.Entries[0].Path, "instructions"; got != want {
		t.Fatalf("ListWorkspace().Entries[0].Path = %q, want %q", got, want)
	}

	file, err := store.ReadWorkspaceFile(context.Background(), "gitlab-assistant", "instructions/AGENTS.md")
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

func TestRemoteStoreFetchWorkspaceSupportsLegacyLayoutAndEmptyMissingTree(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/codes/Agentic/feishu-assistant":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"default_branch": "main"}})
		case "/api/v1/codes/Agentic/feishu-assistant/refs/main/tree/instructions":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/codes/Agentic/feishu-assistant/refs/main/tree/workspace":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"Files": []map[string]any{
					{"name": "skills", "type": "dir", "path": "workspace/skills"},
					{"name": "AGENTS.md", "type": "file", "path": "workspace/AGENTS.md"},
					{"name": "USER.md", "type": "file", "path": "workspace/USER.md"},
				},
				"Cursor": "",
			}})
		case "/api/v1/codes/Agentic/feishu-assistant/refs/main/tree/workspace/skills":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"Files":  []map[string]any{{"name": "feishu", "type": "dir", "path": "workspace/skills/feishu"}},
				"Cursor": "",
			}})
		case "/api/v1/codes/Agentic/feishu-assistant/refs/main/tree/workspace/skills/feishu":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"Files":  []map[string]any{{"name": "SKILL.md", "type": "file", "path": "workspace/skills/feishu/SKILL.md"}},
				"Cursor": "",
			}})
		case "/api/v1/codes/Agentic/feishu-assistant/blob/workspace/AGENTS.md":
			writeRemoteBlob(t, w, "workspace/AGENTS.md", []byte("legacy instructions\n"))
		case "/api/v1/codes/Agentic/feishu-assistant/blob/workspace/USER.md":
			writeRemoteBlob(t, w, "workspace/USER.md", []byte("legacy user\n"))
		case "/api/v1/codes/Agentic/feishu-assistant/blob/workspace/skills/feishu/SKILL.md":
			writeRemoteBlob(t, w, "workspace/skills/feishu/SKILL.md", []byte("legacy skill\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	store := NewRemoteStore(srv.URL, "")
	file, err := store.ReadWorkspaceFile(context.Background(), "feishu-assistant", "instructions/AGENTS.md")
	if err != nil {
		t.Fatalf("ReadWorkspaceFile() error = %v", err)
	}
	if got, want := file.Content, "legacy instructions\n"; got != want {
		t.Fatalf("ReadWorkspaceFile().Content = %q, want %q", got, want)
	}
	workspace, err := store.FetchWorkspace(context.Background(), "feishu-assistant")
	if err != nil {
		t.Fatalf("FetchWorkspace() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workspace.Path) })
	if !workspace.Temporary {
		t.Fatal("FetchWorkspace().Temporary = false, want true")
	}
	for name, want := range map[string]string{
		"AGENTS.md":              "legacy instructions\n",
		"USER.md":                "legacy user\n",
		"skills/feishu/SKILL.md": "legacy skill\n",
	} {
		data, err := os.ReadFile(filepath.Join(workspace.Path, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", name, err)
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
	if got, want := items[0].ID, "Agentic/gitlab-assistant"; got != want {
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
