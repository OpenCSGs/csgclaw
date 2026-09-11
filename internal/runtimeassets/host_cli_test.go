package runtimeassets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHostCLIPathNeverFallsBackToPATH(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if err := os.WriteFile(filepath.Join(dir, "csgclaw-cli"), []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "bundle", "bin", "csgclaw")
	for _, goos := range []string{"darwin", "linux", "windows"} {
		name := "csgclaw-cli"
		if goos == "windows" {
			name += ".exe"
		}
		if got, want := hostCLIPath(exe, goos), filepath.Join(filepath.Dir(exe), name); got != want {
			t.Fatalf("%s: got %q, want %q", goos, got, want)
		}
	}
}

func TestHostCLIPathResolvesServerLauncher(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "bundle", "csgclaw")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("server"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "launcher")
	if err := os.Symlink(exe, link); err != nil {
		t.Skip(err)
	}
	exe, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := hostCLIPath(link, "linux"), filepath.Join(filepath.Dir(exe), "csgclaw-cli"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
