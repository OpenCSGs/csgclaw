package filebrowse

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveDirectoryStructureBoundsAndCleanup(t *testing.T) {
	root := t.TempDir()
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	if err := os.MkdirAll(filepath.Join(root, "docs", "空目录"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "报告.md"), []byte("snapshot content"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ArchiveDirectory(root, "docs", 1024)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := snapshot.Stat()
	reader, err := zip.NewReader(snapshot, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, entry := range reader.File {
		names[entry.Name] = true
		if entry.Name == "docs/报告.md" {
			r, _ := entry.Open()
			data, _ := io.ReadAll(r)
			r.Close()
			if string(data) != "snapshot content" {
				t.Fatal(string(data))
			}
		}
	}
	for _, name := range []string{"docs/", "docs/空目录/", "docs/报告.md"} {
		if !names[name] {
			t.Fatalf("missing %q", name)
		}
	}
	snapshot.Close()
	os.Remove(snapshot.Name())
	for _, limit := range []int64{5, 100} {
		if f, err := ArchiveDirectory(root, "docs", limit); err == nil {
			f.Close()
			os.Remove(f.Name())
			t.Fatal("oversized directory accepted")
		}
	}
	if err := os.Symlink(filepath.Join(root, "docs", "报告.md"), filepath.Join(root, "docs", "link")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"docs", "../docs", "/etc", "docs/link"} {
		if f, err := ArchiveDirectory(root, path, 1024); err == nil {
			f.Close()
			os.Remove(f.Name())
			t.Fatalf("unsafe directory accepted: %s", path)
		}
	}
	remaining, err := os.ReadDir(temp)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("leaked snapshots: %v %v", remaining, err)
	}
}
