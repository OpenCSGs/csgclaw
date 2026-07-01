package remote

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchAgenticHubSkillArchiveReadsTreeCursorPages(t *testing.T) {
	treeRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/skills/AIWizards/agent-builder/refs/main/tree/":
			treeRequests++
			if r.URL.Query().Get("cursor") == "" {
				_, _ = io.WriteString(w, `{"data":{"Files":[],"Cursor":"next-page"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"Files":[{"name":"SKILL.md","path":"SKILL.md","type":"file"}],"Cursor":""}}`)
		case "/api/v1/skills/AIWizards/agent-builder/blob/SKILL.md":
			if got := r.URL.Query().Get("ref"); got != "main" {
				t.Fatalf("blob ref = %q, want main", got)
			}
			content := base64.StdEncoding.EncodeToString([]byte("name: agent-builder\n"))
			_, _ = io.WriteString(w, `{"data":{"content":"`+content+`","path":"SKILL.md","type":"file"}}`)
		default:
			t.Fatalf("unexpected AgenticHub request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	archive, err := FetchAgenticHubSkillArchive(context.Background(), server.URL, "AIWizards/agent-builder", "")
	if err != nil {
		t.Fatalf("FetchAgenticHubSkillArchive() error = %v", err)
	}
	if treeRequests != 2 {
		t.Fatalf("tree requests = %d, want 2", treeRequests)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("open zip archive: %v", err)
	}
	file, err := reader.Open("agent-builder/SKILL.md")
	if err != nil {
		t.Fatalf("open skill file in archive: %v", err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read skill file in archive: %v", err)
	}
	if string(data) != "name: agent-builder\n" {
		t.Fatalf("skill file content = %q", string(data))
	}
}

func TestAgenticHubSkillTreeURLEscapesSlashRef(t *testing.T) {
	got, err := agenticHubSkillTreeURL(
		"https://example.test/hub",
		"AIWizards/agent-builder",
		"feature/install",
		"",
		"next-page",
	)
	if err != nil {
		t.Fatalf("agenticHubSkillTreeURL() error = %v", err)
	}
	want := "https://example.test/hub/api/v1/skills/AIWizards/agent-builder/refs/feature%2Finstall/tree/?cursor=next-page&limit=500"
	if got != want {
		t.Fatalf("tree URL = %q, want %q", got, want)
	}
}

func TestAgenticHubSkillBlobURLUsesRefQuery(t *testing.T) {
	got, err := agenticHubSkillBlobURL(
		"https://example.test",
		"AIWizards/agent-builder",
		"feature/install",
		"scripts/run.sh",
	)
	if err != nil {
		t.Fatalf("agenticHubSkillBlobURL() error = %v", err)
	}
	want := "https://example.test/api/v1/skills/AIWizards/agent-builder/blob/scripts/run.sh?ref=feature%2Finstall"
	if got != want {
		t.Fatalf("blob URL = %q, want %q", got, want)
	}
}
