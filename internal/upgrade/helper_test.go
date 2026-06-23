package upgrade

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestStartApplyHelperIncludesConfigPath(t *testing.T) {
	origExecutable := startHelperExecutable
	origCommand := startHelperCommand
	t.Cleanup(func() {
		startHelperExecutable = origExecutable
		startHelperCommand = origCommand
	})

	helperPath := writeTestHelperExecutable(t)
	startHelperExecutable = func() (string, error) { return helperPath, nil }

	var gotName string
	var gotArgs []string
	var startedCmd *exec.Cmd
	startHelperCommand = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		startedCmd = testNoopCommand()
		return startedCmd
	}

	configPath := filepath.Join(t.TempDir(), "csgclaw.toml")
	if err := StartApplyHelper(ApplyHelperOptions{ConfigPath: configPath}); err != nil {
		t.Fatalf("StartApplyHelper() error = %v", err)
	}

	if runtime.GOOS == "windows" {
		if gotName == helperPath {
			t.Fatalf("command name = %q, want temp helper copy instead of original executable", gotName)
		}
		if filepath.Base(gotName) != filepath.Base(helperPath) {
			t.Fatalf("command name base = %q, want %q", filepath.Base(gotName), filepath.Base(helperPath))
		}
	} else if gotName != helperPath {
		t.Fatalf("command name = %q, want %q", gotName, helperPath)
	}
	wantArgs := []string{"--config", configPath, "upgrade"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", gotArgs, wantArgs)
	}
	artifacts, err := ResolveApplyArtifacts(configPath)
	if err != nil {
		t.Fatalf("ResolveApplyArtifacts() error = %v", err)
	}
	if startedCmd == nil {
		t.Fatal("startHelperCommand was not called")
	}
	for _, want := range artifacts.Env() {
		found := false
		for _, got := range startedCmd.Env {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("command env missing %q in %#v", want, startedCmd.Env)
		}
	}
	wantOriginalExecutable := originalExecutableEnvVar + "=" + helperPath
	foundOriginalExecutable := false
	for _, got := range startedCmd.Env {
		if got == wantOriginalExecutable {
			foundOriginalExecutable = true
			break
		}
	}
	if !foundOriginalExecutable {
		t.Fatalf("command env missing %q in %#v", wantOriginalExecutable, startedCmd.Env)
	}
}

func TestStartApplyHelperOmitsEmptyConfigPath(t *testing.T) {
	origExecutable := startHelperExecutable
	origCommand := startHelperCommand
	t.Cleanup(func() {
		startHelperExecutable = origExecutable
		startHelperCommand = origCommand
	})

	helperPath := writeTestHelperExecutable(t)
	startHelperExecutable = func() (string, error) { return helperPath, nil }
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	var gotArgs []string
	startHelperCommand = func(name string, args ...string) *exec.Cmd {
		gotArgs = append([]string(nil), args...)
		cmd := testNoopCommand()
		return cmd
	}

	if err := StartApplyHelper(ApplyHelperOptions{}); err != nil {
		t.Fatalf("StartApplyHelper() error = %v", err)
	}

	wantArgs := []string{"upgrade"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestStartApplyHelperReturnsLogOpenError(t *testing.T) {
	origExecutable := startHelperExecutable
	t.Cleanup(func() {
		startHelperExecutable = origExecutable
	})

	helperPath := writeTestHelperExecutable(t)
	startHelperExecutable = func() (string, error) { return helperPath, nil }

	configPath := filepath.Join(t.TempDir(), "blocked", "config.toml")
	if err := os.WriteFile(filepath.Dir(configPath), []byte("file blocks dir"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := StartApplyHelper(ApplyHelperOptions{ConfigPath: configPath})
	if err == nil || (!errors.Is(err, os.ErrNotExist) && !strings.Contains(err.Error(), "create upgrade helper state dir")) {
		t.Fatalf("StartApplyHelper() error = %v, want artifact setup failure", err)
	}
}

func TestCleanupUpgradeHelperTempDirsRemovesOnlyHelperDirs(t *testing.T) {
	tempRoot := t.TempDir()
	oldHelperDir := filepath.Join(tempRoot, "csgclaw-upgrade-helper-old")
	keepDir := filepath.Join(tempRoot, "csgclaw-other-temp")
	if err := os.MkdirAll(oldHelperDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", oldHelperDir, err)
	}
	if err := os.WriteFile(filepath.Join(oldHelperDir, "csgclaw.exe"), []byte("old helper"), 0o600); err != nil {
		t.Fatalf("WriteFile(old helper) error = %v", err)
	}
	if err := os.MkdirAll(keepDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", keepDir, err)
	}

	cleanupUpgradeHelperTempDirs(tempRoot)

	if _, err := os.Stat(oldHelperDir); !os.IsNotExist(err) {
		t.Fatalf("old helper dir still exists, stat err = %v", err)
	}
	if _, err := os.Stat(keepDir); err != nil {
		t.Fatalf("keep dir stat error = %v", err)
	}
}

func writeTestHelperExecutable(t *testing.T) string {
	t.Helper()
	name := "csgclaw"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("helper"), 0o700); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}

func testNoopCommand() *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", "exit", "0")
	}
	return exec.Command("sh", "-c", "exit 0")
}
