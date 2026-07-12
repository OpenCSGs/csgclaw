package codexcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocatorLocateUsesExplicitPathFirst(t *testing.T) {
	dir := t.TempDir()
	explicit := writeExecutable(t, filepath.Join(dir, "custom-codex"), "#!/bin/sh\n")
	pathBinary := writeExecutable(t, filepath.Join(dir, "bin", BinaryName), "#!/bin/sh\n")

	locator := Locator{
		ExplicitPath: explicit,
		LookPath: func(string) (string, error) {
			t.Fatal("LookPath should not be called when explicit path exists")
			return "", nil
		},
	}

	got, err := locator.Locate()
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	if got != explicit {
		t.Fatalf("Locate() = %q, want %q; pathBinary=%q", got, explicit, pathBinary)
	}
}

func TestLocatorLocateUsesEnvOverride(t *testing.T) {
	dir := t.TempDir()
	envBinary := writeExecutable(t, filepath.Join(dir, "env-codex"), "#!/bin/sh\n")
	legacyBinary := writeExecutable(t, filepath.Join(dir, "legacy-codex"), "#!/bin/sh\n")
	t.Setenv(EnvBinaryPath, envBinary)
	t.Setenv(EnvLegacyACPBinaryPath, legacyBinary)

	got, err := (Locator{}).Locate()
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	if got != envBinary {
		t.Fatalf("Locate() = %q, want %q", got, envBinary)
	}
}

func TestLocatorLocateUsesLegacyACPEnvFallback(t *testing.T) {
	dir := t.TempDir()
	legacyBinary := writeExecutable(t, filepath.Join(dir, "legacy-codex"), "#!/bin/sh\n")
	t.Setenv(EnvBinaryPath, "")
	t.Setenv(EnvLegacyACPBinaryPath, legacyBinary)

	got, err := (Locator{}).Locate()
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	if got != legacyBinary {
		t.Fatalf("Locate() = %q, want %q", got, legacyBinary)
	}
}

func TestLocatorLocateUsesPathLookup(t *testing.T) {
	dir := t.TempDir()
	pathBinary := writeExecutable(t, filepath.Join(dir, "bin", BinaryName), "#!/bin/sh\n")
	var names []string

	locator := Locator{
		LookPath: func(name string) (string, error) {
			names = append(names, name)
			return pathBinary, nil
		},
	}

	got, err := locator.Locate()
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	if got != pathBinary {
		t.Fatalf("Locate() = %q, want %q", got, pathBinary)
	}
	if len(names) != 1 || names[0] != BinaryName {
		t.Fatalf("LookPath names = %+v, want [%q]", names, BinaryName)
	}
}

func TestLocatorLocateUsesManagedFallback(t *testing.T) {
	dir := t.TempDir()
	managedBinary := writeExecutable(t, filepath.Join(dir, "managed", BinaryName), "#!/bin/sh\n")

	got, err := (Locator{
		ManagedPath: managedBinary,
		LookPath: func(string) (string, error) {
			return "", os.ErrNotExist
		},
	}).Locate()
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	if got != managedBinary {
		t.Fatalf("Locate() = %q, want %q", got, managedBinary)
	}
}

func TestDefaultManagedPathUsesCSGClawBinDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, err := DefaultManagedPath("windows")
	if err != nil {
		t.Fatalf("DefaultManagedPath() error = %v", err)
	}
	want := filepath.Join(home, ".csgclaw", "bin", "codex.exe")
	if got != want {
		t.Fatalf("DefaultManagedPath() = %q, want %q", got, want)
	}
}

func TestLocatorLocateMissingBinary(t *testing.T) {
	locator := Locator{
		LookPath: func(string) (string, error) {
			return "", os.ErrNotExist
		},
	}

	_, err := locator.Locate()
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Locate() error = %v, want os.ErrNotExist", err)
	}
}

func TestLocatorWindowsPathLookupUsesNativeExecutable(t *testing.T) {
	dir := t.TempDir()
	exeBinary := writeExecutable(t, filepath.Join(dir, "bin", "codex.exe"), "")
	var names []string

	locator := Locator{
		GOOS: "windows",
		LookPath: func(name string) (string, error) {
			names = append(names, name)
			if name == "codex.exe" {
				return exeBinary, nil
			}
			return "", os.ErrNotExist
		},
	}

	got, err := locator.Locate()
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	if got != exeBinary {
		t.Fatalf("Locate() = %q, want %q", got, exeBinary)
	}
	if len(names) != 1 || names[0] != "codex.exe" {
		t.Fatalf("LookPath names = %+v, want [codex.exe]", names)
	}
}

func TestLocatorWindowsPathLookupPrefersNativeExecutable(t *testing.T) {
	dir := t.TempDir()
	_ = writeExecutable(t, filepath.Join(dir, "bin", "codex.cmd"), "@echo off\n")
	exeBinary := writeExecutable(t, filepath.Join(dir, "bin", "codex.exe"), "")
	var names []string

	locator := Locator{
		GOOS: "windows",
		LookPath: func(name string) (string, error) {
			names = append(names, name)
			if name == "codex.exe" {
				return exeBinary, nil
			}
			return "", os.ErrNotExist
		},
	}

	got, err := locator.Locate()
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	if got != exeBinary {
		t.Fatalf("Locate() = %q, want %q", got, exeBinary)
	}
	if len(names) != 1 || names[0] != "codex.exe" {
		t.Fatalf("LookPath names = %+v, want [codex.exe]", names)
	}
}

func TestLocatorWindowsPathLookupFallsBackToCommandShim(t *testing.T) {
	dir := t.TempDir()
	cmdBinary := writeExecutable(t, filepath.Join(dir, "bin", "codex.cmd"), "@echo off\n")
	var names []string

	locator := Locator{
		GOOS: "windows",
		LookPath: func(name string) (string, error) {
			names = append(names, name)
			if name == "codex.cmd" {
				return cmdBinary, nil
			}
			return "", os.ErrNotExist
		},
	}

	got, err := locator.Locate()
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	if got != cmdBinary {
		t.Fatalf("Locate() = %q, want %q", got, cmdBinary)
	}
	if want := []string{"codex.exe", "codex.cmd"}; fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatalf("LookPath names = %+v, want %+v", names, want)
	}
}

func TestLocatorWindowsExplicitPowerShellShimUsesNativeSibling(t *testing.T) {
	dir := t.TempDir()
	ps1Path := filepath.Join(dir, "codex.ps1")
	exePath := writeExecutable(t, filepath.Join(dir, "codex.exe"), "")

	locator := Locator{
		GOOS:         "windows",
		ExplicitPath: ps1Path,
		LookPath: func(string) (string, error) {
			t.Fatal("LookPath should not be called when explicit PowerShell shim has native sibling")
			return "", nil
		},
	}

	got, err := locator.Locate()
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	if got != exePath {
		t.Fatalf("Locate() = %q, want %q", got, exePath)
	}
}

func TestLocatorWindowsExplicitPowerShellShimReportsUnsupportedWhenNoSibling(t *testing.T) {
	dir := t.TempDir()
	ps1Path := writeExecutable(t, filepath.Join(dir, "codex.ps1"), "codex\n")

	locator := Locator{
		GOOS:         "windows",
		ExplicitPath: ps1Path,
	}

	_, err := locator.Locate()
	if err == nil {
		t.Fatal("Locate() error = nil, want unsupported PowerShell shim error")
	}
	if !strings.Contains(err.Error(), "PowerShell shim") || !strings.Contains(err.Error(), "codex.cmd") {
		t.Fatalf("Locate() error = %v, want command shim guidance", err)
	}
}

func TestLocatorWindowsExplicitCommandShimIsSupported(t *testing.T) {
	dir := t.TempDir()
	cmdPath := writeExecutable(t, filepath.Join(dir, "codex.cmd"), "@echo off\n")
	managedPath := writeExecutable(t, filepath.Join(dir, "managed", "codex.exe"), "")

	got, err := (Locator{
		GOOS:         "windows",
		ExplicitPath: cmdPath,
		ManagedPath:  managedPath,
		LookPath: func(string) (string, error) {
			t.Fatal("LookPath should not be called for an existing explicit command shim")
			return "", nil
		},
	}).Locate()
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	if got != cmdPath {
		t.Fatalf("Locate() = %q, want %q", got, cmdPath)
	}
}

func TestLocatorWindowsMissingExplicitCommandShimFallsBackToManagedNativeExecutable(t *testing.T) {
	dir := t.TempDir()
	cmdPath := filepath.Join(dir, "missing", "codex.cmd")
	managedPath := writeExecutable(t, filepath.Join(dir, "managed", "codex.exe"), "")

	got, err := (Locator{
		GOOS:         "windows",
		ExplicitPath: cmdPath,
		ManagedPath:  managedPath,
		LookPath: func(string) (string, error) {
			return "", os.ErrNotExist
		},
	}).Locate()
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	if got != managedPath {
		t.Fatalf("Locate() = %q, want %q", got, managedPath)
	}
}

func TestAppServerArgsAreFixed(t *testing.T) {
	got := AppServerArgs()
	want := []string{"app-server", "--listen", "stdio://"}
	if len(got) != len(want) {
		t.Fatalf("AppServerArgs() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AppServerArgs() = %+v, want %+v", got, want)
		}
	}
}

func writeExecutable(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}
