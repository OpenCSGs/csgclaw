package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/skill/discovery"
	skilllocal "csgclaw/internal/skill/local"
	skillsystem "csgclaw/internal/skill/system"
)

func TestDiscoveredSkillLibraryLifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, key := range []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR", "PI_CODING_AGENT_DIR", "XDG_CONFIG_HOME"} {
		t.Setenv(key, "")
	}
	source := filepath.Join(home, ".claude", "skills", "source-folder")
	if err := os.MkdirAll(filepath.Join(source, "references"), 0755); err != nil {
		t.Fatal(err)
	}
	write := func(version, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: discovered-demo\ndescription: "+text+"\nversion: \""+version+"\"\n---\n"+text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("1.0.0", "first")
	os.WriteFile(filepath.Join(source, "references", "guide.md"), []byte("skill resource"), 0644)
	os.WriteFile(filepath.Join(home, ".claude", "credentials.json"), []byte("PRIVATE"), 0600)
	os.Symlink(filepath.Join(home, ".claude", "credentials.json"), filepath.Join(source, "secret.md"))
	h := &Handler{}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		h.Routes().ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}
	list := func() []skillsystem.SkillSummary {
		t.Helper()
		rec := request("GET", "/api/v1/skills", "")
		if rec.Code != 200 {
			t.Fatalf("list %d: %s", rec.Code, rec.Body.String())
		}
		var items []skillsystem.SkillSummary
		if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
			t.Fatal(err)
		}
		return items
	}
	find := func() skillsystem.SkillSummary {
		t.Helper()
		for _, item := range list() {
			if item.Name == "discovered-demo" {
				return item
			}
		}
		t.Fatal("discovered skill absent from library")
		return skillsystem.SkillSummary{}
	}
	first := find()
	if first.Discovery == nil || !first.Discovery.New || first.Source != "discovered" || !filepath.IsAbs(first.Directory) || first.Directory != first.Discovery.Path {
		t.Fatalf("first=%+v", first)
	}
	for _, test := range []struct{ path, content string }{
		{"/api/v1/skills/tree", "discovered-demo"},
		{"/api/v1/skills/tree?path=discovered-demo", "references/guide.md"},
		{"/api/v1/skills/file?path=discovered-demo/SKILL.md", "first"},
		{"/api/v1/skills/file?path=discovered-demo/references/guide.md", "skill resource"},
	} {
		rec := request("GET", test.path, "")
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), test.content) {
			t.Fatalf("%s: %d %s", test.path, rec.Code, rec.Body.String())
		}
	}
	for _, path := range []string{"discovered-demo/secret.md", "discovered-demo/../credentials.json", "discovered-demo/../../.claude/credentials.json"} {
		rec := request("GET", "/api/v1/skills/file?path="+path, "")
		if rec.Code == 200 || strings.Contains(rec.Body.String(), "PRIVATE") {
			t.Fatalf("private file exposed: %s", rec.Body.String())
		}
	}
	raw, _ := json.Marshal(map[string]any{"keys": []string{first.Discovery.Key}})
	rec := request("POST", "/api/v1/skills/discovery/ack", string(raw))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("ack: %s", rec.Body.String())
	}
	if find().Discovery.New {
		t.Fatal("acknowledgement did not survive fresh request")
	}
	write("1.1.0", "updated")
	if current := find(); current.Description != "updated" || !current.Discovery.New {
		t.Fatalf("update=%+v", current)
	}
	rec = request("DELETE", "/api/v1/skills/discovered-demo", "")
	if rec.Code != 204 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	for _, item := range list() {
		if item.Name == "discovered-demo" {
			t.Fatal("unregistered skill reappeared")
		}
	}
	if _, err := os.Stat(filepath.Join(source, "SKILL.md")); err != nil {
		t.Fatal("source deleted")
	}
	root, _ := skilllocal.SkillsRoot()
	if _, err := discovery.Sync(root); err != nil {
		t.Fatal(err)
	}
	// A fresh HTTP handler sees the persisted suppression, without touching source data.
	h = &Handler{}
	rec = request("GET", "/api/v1/skills/tree", "")
	var listing apitypes.WorkspaceListing
	if err := json.Unmarshal(rec.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	for _, entry := range listing.Entries {
		if entry.Name == "discovered-demo" {
			t.Fatal("suppression lost after restart")
		}
	}
	// Explicit rescanning restores removed registrations without touching source files.
	rec = request("POST", "/api/v1/skills/discovery/rescan", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("rescan %d: %s", rec.Code, rec.Body.String())
	}
	restored := find()
	if restored.Discovery == nil || !restored.Discovery.New || !filepath.IsAbs(restored.Directory) {
		t.Fatalf("restored=%+v", restored)
	}
	if _, err := os.Stat(filepath.Join(source, "SKILL.md")); err != nil {
		t.Fatal("rescan changed source")
	}

}
