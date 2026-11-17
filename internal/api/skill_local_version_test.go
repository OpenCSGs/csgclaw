package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"csgclaw/internal/skill/installstate"
	skilllocal "csgclaw/internal/skill/local"
)

func TestLocalZIPVersionSurvivesUploadAndListing(t *testing.T) {
	for _, test := range []struct{ name, metadata, version string }{
		{"top-level", "version: 2.7.3\nmetadata:\n  version: 1.0.0\n", "2.7.3"},
		{"nested", "metadata:\n  version: ' 3.2.1 '\n", "3.2.1"},
		{"unversioned", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			for _, key := range []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR", "PI_CODING_AGENT_DIR", "XDG_CONFIG_HOME"} {
				t.Setenv(key, "")
			}
			h := &Handler{}
			upload := httptest.NewRecorder()
			h.Routes().ServeHTTP(upload, newSkillUploadRequest(t, "version-probe.zip", map[string]string{
				"SKILL.md": "---\nname: version-probe\ndescription: Local version probe\n" + test.metadata + "---\n# Probe\n",
			}))
			if upload.Code != http.StatusCreated {
				t.Fatalf("upload: %d %s", upload.Code, upload.Body.String())
			}
			var item skilllocal.SkillSummary
			if err := json.Unmarshal(upload.Body.Bytes(), &item); err != nil {
				t.Fatal(err)
			}
			check := func(item skilllocal.SkillSummary) {
				t.Helper()
				if item.Source != installstate.SourceLocal || item.Install == nil || item.Install.Version != test.version {
					t.Fatalf("local version: %+v, install=%+v, want %q", item, item.Install, test.version)
				}
			}
			check(item)
			root := filepath.Join(home, ".csgclaw", "skills")
			records, err := installstate.Read(root)
			if err != nil || records[item.Name].Version != test.version {
				t.Fatalf("persisted record: %+v, %v", records, err)
			}
			list := func() {
				t.Helper()
				rec := httptest.NewRecorder()
				(&Handler{}).Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/skills", nil))
				var items []skilllocal.SkillSummary
				if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &items) != nil {
					t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
				}
				for _, found := range items {
					if found.Name == item.Name {
						check(found)
						return
					}
				}
				t.Fatal("uploaded skill missing")
			}
			list()
			// Previously uploaded ZIPs have an empty version in the lock file.
			record := records[item.Name]
			record.Version = ""
			if err := installstate.Update(root, item.Name, &record, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			list()
		})
	}
}
