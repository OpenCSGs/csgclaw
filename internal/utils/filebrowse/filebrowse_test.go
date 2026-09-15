package filebrowse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"csgclaw/internal/apitypes"
)

func TestReadFileAcceptsTextAboveFormerPreviewLimit(t *testing.T) {
	content := strings.Repeat("preview content\n", (4*1024*1024)/16)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	source := fstest.MapFS{"workspace/large.txt": &fstest.MapFile{Data: []byte(content)}}
	readers := map[string]func() (apitypes.WorkspaceFile, error){
		"local": func() (apitypes.WorkspaceFile, error) { return ReadFile(root, "large.txt") },
		"embedded": func() (apitypes.WorkspaceFile, error) {
			return ReadFileFS(source, "workspace", "large.txt")
		},
	}
	for name, read := range readers {
		t.Run(name, func(t *testing.T) {
			file, err := read()
			if err != nil {
				t.Fatal(err)
			}
			if file.Truncated || file.Binary || file.Content != content {
				t.Fatalf("preview truncated=%t, binary=%t, bytes=%d, want full %d bytes", file.Truncated, file.Binary, len(file.Content), len(content))
			}
		})
	}
}
