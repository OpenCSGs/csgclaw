//go:build !windows

package discovery

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestScanRejectsSpecialDefinitionsWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	definition(t, filepath.Join(root, "good"), "good", "", "good")
	os.MkdirAll(filepath.Join(root, "pipe"), 0755)
	if err := syscall.Mkfifo(filepath.Join(root, "pipe", "SKILL.md"), 0600); err != nil {
		t.Fatal(err)
	}
	if items := Scan([]Source{{Tool: "test", Path: root}}); len(items) != 1 || items[0].Name != "good" {
		t.Fatalf("items=%+v", items)
	}
}
