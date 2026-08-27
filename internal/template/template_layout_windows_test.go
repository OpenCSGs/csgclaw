//go:build windows

package template

import (
	"io/fs"
	"testing"
)

func TestTemplateManifestFSPathUsesForwardSlashesOnWindows(t *testing.T) {
	got := templateManifestFSPath(`C:\hub\templates\codex-memory`)
	if want := "codex-memory/agent.toml"; got != want {
		t.Fatalf("templateManifestFSPath() = %q, want %q", got, want)
	}
	if !fs.ValidPath(got) {
		t.Fatalf("templateManifestFSPath() = %q, want a valid fs.FS path", got)
	}
}
