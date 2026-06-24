package runtimeassets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshFromBundleInstallsAndUpdatesSandboxCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	source := filepath.Join(dir, "csgclaw", "bin", "csgclaw_dir", "csgclaw-cli")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("MkdirAll(source) error = %v", err)
	}
	if err := os.WriteFile(source, []byte("v1"), 0o755); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	if _, err := RefreshFromBundle(filepath.Join(dir, "csgclaw")); err != nil {
		t.Fatalf("RefreshFromBundle(v1) error = %v", err)
	}
	if err := os.WriteFile(source, []byte("v2"), 0o755); err != nil {
		t.Fatalf("WriteFile(source v2) error = %v", err)
	}
	if _, err := RefreshFromBundle(filepath.Join(dir, "csgclaw")); err != nil {
		t.Fatalf("RefreshFromBundle(v2) error = %v", err)
	}
	target := filepath.Join(home, ".csgclaw", "sandbox-tools", "csgclaw-cli")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target) error = %v", err)
	}
	if got, want := string(data), "v2"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
	if info, err := os.Stat(target); err != nil {
		t.Fatalf("Stat(target) error = %v", err)
	} else if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("target mode = %o, want executable", info.Mode().Perm())
	}
}
