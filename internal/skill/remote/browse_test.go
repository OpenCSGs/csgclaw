package remote

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowseRemoteSkillWithoutInstalling(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer private-token" {
			t.Error("missing user token")
		}
		switch r.URL.Path {
		case "/api/v1/skills/team/demo/refs/dev/tree/":
			if r.URL.Query().Get("cursor") == "" {
				io.WriteString(w, `{"data":{"Files":[{"path":"scripts","type":"dir"}],"Cursor":"next"}}`)
			} else {
				io.WriteString(w, `{"data":{"Files":[{"path":"SKILL.md","type":"file"}]}}`)
			}
		case "/api/v1/skills/team/demo/refs/dev/tree/scripts":
			io.WriteString(w, `{"data":{"Files":[{"path":"scripts/run.py","type":"file","size":3}]}}`)
		case "/api/v1/skills/team/demo/blob/SKILL.md":
			if r.URL.Query().Get("ref") != "dev" {
				t.Error("wrong blob ref")
			}
			io.WriteString(w, `{"data":{"content":"`+base64.StdEncoding.EncodeToString([]byte("# Private skill"))+`"}}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx := context.Background()
	tree, err := ListAgenticHubSkillFiles(ctx, server.URL, "private-token", "team/demo", "dev")
	if err != nil || len(tree.Entries) != 3 {
		t.Fatalf("tree = %+v, err = %v", tree, err)
	}
	if tree.Entries[1].Path != "scripts/run.py" || tree.Entries[1].Depth != 1 {
		t.Fatalf("nested entry = %+v", tree.Entries[1])
	}
	file, err := ReadAgenticHubSkillFile(ctx, server.URL, "private-token", "team/demo", "dev", "SKILL.md")
	if err != nil || file.Content != "# Private skill" || file.Path != "SKILL.md" {
		t.Fatalf("file = %+v, err = %v", file, err)
	}
	if requests != 4 {
		t.Fatalf("requests = %d, want 4 tree/blob requests only", requests)
	}
}

func TestRemoteSkillPreviewRejectsInvalidPaths(t *testing.T) {
	ctx := context.Background()
	if _, err := ListAgenticHubSkillFiles(ctx, "https://example.test", "", "../demo", ""); !IsInvalidAgenticHubRequest(err) {
		t.Fatalf("invalid repository error = %v", err)
	}
	for _, file := range []string{"", "../secret", "scripts/../../secret"} {
		if _, err := ReadAgenticHubSkillFile(ctx, "https://example.test", "", "team/demo", "main", file); !IsInvalidAgenticHubRequest(err) {
			t.Fatalf("invalid file %q error = %v", file, err)
		}
	}
}

func TestRemoteSkillPreviewReportsFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "repository unavailable", http.StatusNotFound)
	}))
	defer server.Close()
	if _, err := ListAgenticHubSkillFiles(context.Background(), server.URL, "", "team/missing", "main"); err == nil {
		t.Fatal("missing tree error")
	}
	if _, err := ReadAgenticHubSkillFile(context.Background(), server.URL, "", "team/missing", "main", "SKILL.md"); err == nil {
		t.Fatal("missing file error")
	}
}
