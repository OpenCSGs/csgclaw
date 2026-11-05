package workspace

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"csgclaw/internal/agentengine/contract"
)

func TestOpenInputRejectsUnsafeWorkspacePaths(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(outside, "secret"), "../secret", "a/../secret", `C:\private`, ".", "unavailable"}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err == nil {
		paths = append(paths, "link/secret")
	} else if runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	for _, path := range paths {
		ref := &contract.WorkspaceFile{Path: path, Name: "report"}
		if handle, _, err := OpenInput(root, ref); err == nil {
			handle.Close()
			t.Fatalf("opened unsafe workspace path %q", path)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "reports"), 0700); err != nil {
		t.Fatal(err)
	}
	ref := &contract.WorkspaceFile{Path: "reports", Name: "report"}
	if handle, _, err := OpenInput(root, ref); err == nil {
		handle.Close()
		t.Fatal("opened directory as a file")
	}
}

func TestWorkspaceInputReadsTheOpenedFileAfterReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "report.pdf")
	if err := os.WriteFile(path, []byte("uploaded"), 0600); err != nil {
		t.Fatal(err)
	}
	ref := &contract.WorkspaceFile{Path: "report.pdf", Name: "report.pdf"}
	file, prompt, err := OpenInput(root, ref)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := os.Rename(path, filepath.Join(root, "moved.pdf")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("open workspace file cannot be renamed: %v", err)
		}
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(file)
	if err != nil || string(content) != "uploaded" || !strings.Contains(prompt, ref.Path) || strings.Contains(prompt, root) {
		t.Fatalf("content = %q, prompt = %q, error = %v", content, prompt, err)
	}
}
