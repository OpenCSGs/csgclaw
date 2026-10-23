package builtin

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"csgclaw/internal/skill/installstate"
	"csgclaw/internal/skill/local"
)

func TestSyncCatalog(t *testing.T) {
	root := t.TempDir()
	start := time.Now()
	if err := Sync(root); err != nil {
		t.Fatal(err)
	}
	t.Logf("first release: %v", time.Since(start))
	items, err := local.List(root)
	if err != nil || len(items) != 10 {
		t.Fatalf("catalog count=%d err=%v", len(items), err)
	}
	records, err := installstate.Read(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range Names() {
		hash, err := installstate.BundleSHA256(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if records[name].SHA256 != hash {
			t.Fatalf("manifest mismatch: %s", name)
		}
	}
	path := filepath.Join(root, "pdf", "SKILL.md")
	before, _ := os.Stat(path)
	start = time.Now()
	if err := Sync(root); err != nil {
		t.Fatal(err)
	}
	t.Logf("unchanged release: %v", time.Since(start))
	after, _ := os.Stat(path)
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("unchanged skill rewritten")
	}
}

func TestSyncPreservesEditsAndConflicts(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "pdf")
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dst, "SKILL.md")
	if err := os.WriteFile(path, []byte("user skill"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Sync(root); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(path)
	if string(content) != "user skill" {
		t.Fatal("overwrote local conflict")
	}
	// Simulate a previous bundled release and a user modification.
	records, _ := installstate.Read(root)
	record := records["xlsx"]
	record.Version = "old-version"
	if err := installstate.Update(root, "xlsx", &record, nil); err != nil {
		t.Fatal(err)
	}
	edited := filepath.Join(root, "xlsx", "SKILL.md")
	if err := os.WriteFile(edited, []byte("edited"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Sync(root); err != nil {
		t.Fatal(err)
	}
	content, _ = os.ReadFile(edited)
	if string(content) != "edited" {
		t.Fatal("overwrote modified bundle")
	}
	// An untouched old version is updated and gets current provenance.
	record = records["docx"]
	record.Version = "old-version"
	if err := installstate.Update(root, "docx", &record, nil); err != nil {
		t.Fatal(err)
	}
	if err := Sync(root); err != nil {
		t.Fatal(err)
	}
	next, _ := installstate.Read(root)
	if next["docx"].Version == "old-version" {
		t.Fatal("untouched bundle not updated")
	}
}
