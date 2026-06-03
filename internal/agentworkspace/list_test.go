package agentworkspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileTruncatesLargeTextPreview(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("a", filePreviewMaxBytes) + "tail"
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := ReadFile(root, "large.txt")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !got.Truncated {
		t.Fatal("Truncated = false, want true")
	}
	if got.Binary {
		t.Fatal("Binary = true, want false")
	}
	if len(got.Content) != filePreviewMaxBytes {
		t.Fatalf("content length = %d, want %d", len(got.Content), filePreviewMaxBytes)
	}
}

func TestReadFileTrimsIncompleteUTF8Preview(t *testing.T) {
	root := t.TempDir()
	prefix := strings.Repeat("a", filePreviewMaxBytes-1)
	if err := os.WriteFile(filepath.Join(root, "utf8.txt"), []byte(prefix+"世界"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := ReadFile(root, "utf8.txt")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !got.Truncated {
		t.Fatal("Truncated = false, want true")
	}
	if got.Binary {
		t.Fatal("Binary = true, want false")
	}
	if got.Content != prefix {
		t.Fatalf("content = %q, want %q", got.Content, prefix)
	}
}

func TestListRejectsSymlinkPath(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, "linked")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if _, err := List(root, "linked"); err == nil {
		t.Fatal("List() error = nil, want symlink error")
	}
}
