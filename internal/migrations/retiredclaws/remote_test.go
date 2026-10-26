package retiredclaws_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/template"
)

func TestExistingRemoteRetiredTemplateAdaptsWithoutRemoteWrites(t *testing.T) {
	manifest := "name=\"reviewer\"\nruntime_kind=\"picoclaw\"\n[image]\nref=\"retired-image\"\n[runtime_options]\nnative_option=\"old\"\n"
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for path, data := range map[string]string{"reviewer/agent.toml": manifest, "reviewer/instructions/AGENT.md": "# Original instructions", "reviewer/mcps/mcp.json": "{}"} {
		entry, err := writer.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected remote mutation: %s", r.Method)
			http.Error(w, "unexpected", 500)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/blob/agent.toml"):
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"content": base64.StdEncoding.EncodeToString([]byte(manifest))}})
		case strings.HasSuffix(r.URL.Path, "/download_archive/refs/main"):
			w.Header().Set("Content-Type", "application/zip")
			w.Write(archive.Bytes())
		case r.URL.Path == "/api/v1/codes/team/reviewer":
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"name": "reviewer", "path": "team/reviewer", "default_branch": "main"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	store := template.NewRemoteStore(server.URL, "")
	item, err := store.Get(context.Background(), "team/reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if item.RuntimeKind != "codex" || item.Image != "" || len(item.RuntimeOptions) != 0 {
		t.Fatalf("remote template: %+v", item)
	}
	workspace, err := store.FetchWorkspace(context.Background(), "team/reviewer")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(workspace.Path)
	if data, err := os.ReadFile(filepath.Join(workspace.Path, "AGENTS.md")); err != nil || string(data) != "# Original instructions" {
		t.Fatal("remote instructions lost")
	}
}
