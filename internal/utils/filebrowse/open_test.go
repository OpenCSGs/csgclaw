package filebrowse

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenWorkspaceFileBoundsAndStableHandle(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.md"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../outside", "/etc/passwd", "link/a.md", "docs", "missing"} {
		file, err := OpenFile(root, path)
		if err == nil {
			file.Close()
			t.Fatalf("accepted unsafe/non-file path %q", path)
		}
	}
	if _, err := ListDirectory(root, "link"); err == nil {
		t.Fatal("listed symlink directory")
	}
	file, err := OpenFile(root, "docs/a.md")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	// Renaming permits path replacement while the original handle remains open on Windows.
	if err := os.Rename(filepath.Join(root, "docs", "a.md"), filepath.Join(root, "docs", "original.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.md"), []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	if err != nil || string(data) != "original" {
		t.Fatalf("source changed: %q %v", data, err)
	}
}
