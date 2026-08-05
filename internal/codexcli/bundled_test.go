package codexcli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBundleLocatorLocateUsesCodexBesideExecutable(t *testing.T) {
	root := t.TempDir()
	executable := writeBundledExecutable(t, filepath.Join(root, "bin", "csgclaw"))
	codex := writeBundledExecutable(t, filepath.Join(root, "bin", "codex"))

	got, err := (BundleLocator{ExecutablePath: func() (string, error) { return executable, nil }}).Locate()
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	assertSameFile(t, got, codex)
}

func TestBundleLocatorLocateFollowsCSGClawLauncherSymlink(t *testing.T) {
	root := t.TempDir()
	launcher := filepath.Join(root, "launcher", "csgclaw")
	realExecutable := writeBundledExecutable(t, filepath.Join(root, "bundle", "bin", "csgclaw"))
	codex := writeBundledExecutable(t, filepath.Join(root, "bundle", "bin", "codex"))
	// A host Codex beside the launcher must not override the bundled binary.
	writeBundledExecutable(t, filepath.Join(root, "launcher", "codex"))
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Symlink(realExecutable, launcher); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	got, err := (BundleLocator{ExecutablePath: func() (string, error) { return launcher, nil }}).Locate()
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	assertSameFile(t, got, codex)
}

func TestBundleLocatorLocateIgnoresLocalCodexEnvironment(t *testing.T) {
	root := t.TempDir()
	executable := writeBundledExecutable(t, filepath.Join(root, "bin", "csgclaw"))
	bundled := writeBundledExecutable(t, filepath.Join(root, "bin", "codex"))
	local := writeBundledExecutable(t, filepath.Join(root, "local", "codex"))
	t.Setenv("CSGCLAW_CODEX_PATH", local)
	t.Setenv("CSGCLAW_CODEX_ACP_PATH", local)
	t.Setenv("PATH", filepath.Dir(local))

	got, err := (BundleLocator{ExecutablePath: func() (string, error) { return executable, nil }}).Locate()
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	assertSameFile(t, got, bundled)
}

func TestBundleLocatorLocateRequiresBundledBinary(t *testing.T) {
	root := t.TempDir()
	executable := writeBundledExecutable(t, filepath.Join(root, "bin", "csgclaw"))

	_, err := (BundleLocator{ExecutablePath: func() (string, error) { return executable, nil }}).Locate()
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Locate() error = %v, want os.ErrNotExist", err)
	}
}

func TestBundleLocatorLocateUsesWindowsExecutable(t *testing.T) {
	root := t.TempDir()
	executable := writeBundledExecutable(t, filepath.Join(root, "bin", "csgclaw.exe"))
	codex := writeBundledExecutable(t, filepath.Join(root, "bin", "codex.exe"))

	got, err := (BundleLocator{
		GOOS:           "windows",
		ExecutablePath: func() (string, error) { return executable, nil },
	}).Locate()
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	assertSameFile(t, got, codex)
}

func writeBundledExecutable(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}

func assertSameFile(t *testing.T, got, want string) {
	t.Helper()

	gotInfo, err := os.Stat(got)
	if err != nil {
		t.Fatalf("Stat(Locate() result %q) error = %v", got, err)
	}
	wantInfo, err := os.Stat(want)
	if err != nil {
		t.Fatalf("Stat(want %q) error = %v", want, err)
	}
	if !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("Locate() = %q, want file %q", got, want)
	}
}
