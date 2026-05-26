package skill

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractSkillZip(t *testing.T) {
	t.Parallel()

	archive := mustZip(t, map[string]string{
		"SKILL.md":       "# Demo\n",
		"scripts/run.sh": "#!/bin/sh\n",
	})
	dir := t.TempDir()
	sha, err := extractSkillZip(archive, dir, 1024*1024)
	if err != nil {
		t.Fatalf("extractSkillZip() error = %v", err)
	}
	if sha == "" {
		t.Fatal("sha256 is empty")
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Fatalf("SKILL.md missing: %v", err)
	}
}

func TestExtractSkillZipRejectsZipSlip(t *testing.T) {
	t.Parallel()

	archive := mustZip(t, map[string]string{
		"../escape/SKILL.md": "# Demo\n",
	})
	_, err := extractSkillZip(archive, t.TempDir(), 1024*1024)
	if err == nil {
		t.Fatal("extractSkillZip() error = nil, want unsafe path error")
	}
}

func mustZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return buf.Bytes()
}
