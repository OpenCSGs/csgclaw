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

func TestAgenticHubSkillArchiveNameUsesRemotePathBasename(t *testing.T) {
	if got, want := AgenticHubSkillArchiveName("team/gitlab"), "gitlab.zip"; got != want {
		t.Fatalf("AgenticHubSkillArchiveName() = %q, want %q", got, want)
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

func TestListAgenticHubSkillsNormalizesCatalogRecords(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hub/api/v1/skills" {
			t.Fatalf("path = %q, want catalog endpoint", r.URL.Path)
		}
		if got := r.URL.Query().Get("page"); got != "2" {
			t.Fatalf("page = %q, want 2", got)
		}
		if got := r.URL.Query().Get("per"); got != "16" {
			t.Fatalf("per = %q, want 16", got)
		}
		if got := r.URL.Query().Get("search"); got != "agent" {
			t.Fatalf("search = %q, want agent", got)
		}
		if got := r.URL.Query().Get("sort"); got != "trending" {
			t.Fatalf("sort = %q, want trending", got)
		}
		if got := r.URL.Query().Get("source"); got != "" {
			t.Fatalf("source = %q, want empty", got)
		}
		_, _ = io.WriteString(w, `{
			"data":[
				{"name":"Skill","path":"AIWizards/agent-builder","description":"Build agents","default_branch":"dev"},
				{"name":"broken","description":"missing path"}
			],
			"total":"78"
		}`)
	}))
	defer server.Close()

	page, err := ListAgenticHubSkills(context.Background(), server.URL+"/hub", AgenticHubSkillListOptions{
		Page:   2,
		Per:    16,
		Search: " agent ",
	})
	if err != nil {
		t.Fatalf("ListAgenticHubSkills() error = %v", err)
	}
	if page.RecordCount != 2 {
		t.Fatalf("RecordCount = %d, want 2", page.RecordCount)
	}
	if page.Total == nil || *page.Total != 78 {
		t.Fatalf("Total = %v, want 78", page.Total)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %#v, want one valid record", page.Items)
	}
	if got, want := page.Items[0], (AgenticHubSkillSummary{
		Description: "Build agents",
		Name:        "agent-builder",
		Ref:         "dev",
		RemotePath:  "AIWizards/agent-builder",
	}); got != want {
		t.Fatalf("item = %#v, want %#v", got, want)
	}
}

func TestAgenticHubSkillWebURLPreservesHubBasePath(t *testing.T) {
	got, err := AgenticHubSkillWebURL("https://hub.example.test/community", "AIWizards/agent-builder")
	if err != nil {
		t.Fatalf("AgenticHubSkillWebURL() error = %v", err)
	}
	if want := "https://hub.example.test/community/skills/AIWizards/agent-builder"; got != want {
		t.Fatalf("web URL = %q, want %q", got, want)
	}
}

func TestAgenticHubTotalTreatsNullAsUnknown(t *testing.T) {
	if got := agenticHubTotal([]byte("null")); got != nil {
		t.Fatalf("agenticHubTotal(null) = %v, want nil", got)
	}
}
