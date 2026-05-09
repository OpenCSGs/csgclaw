package upgrade

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

	startHelperExecutable = func() (string, error) {
		return "/tmp/csgclaw", nil
	}

	var gotName string
	var gotArgs []string
	var startedCmd *exec.Cmd
	startHelperCommand = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		startedCmd = exec.Command("sh", "-c", "exit 0")
		return startedCmd
	}

	configPath := filepath.Join(t.TempDir(), "csgclaw.toml")
	if err := StartApplyHelper(ApplyHelperOptions{ConfigPath: configPath}); err != nil {
		t.Fatalf("StartApplyHelper() error = %v", err)
	}

	if gotName != "/tmp/csgclaw" {
		t.Fatalf("command name = %q, want %q", gotName, "/tmp/csgclaw")
	}
	wantArgs := []string{"upgrade", "--config", configPath}
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
}

func TestStartApplyHelperOmitsEmptyConfigPath(t *testing.T) {
	origExecutable := startHelperExecutable
	origCommand := startHelperCommand
	t.Cleanup(func() {
		startHelperExecutable = origExecutable
		startHelperCommand = origCommand
	})

	startHelperExecutable = func() (string, error) {
		return "/tmp/csgclaw", nil
	}
	t.Setenv("HOME", t.TempDir())

	var gotArgs []string
	startHelperCommand = func(name string, args ...string) *exec.Cmd {
		gotArgs = append([]string(nil), args...)
		cmd := exec.Command("sh", "-c", "exit 0")
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

	startHelperExecutable = func() (string, error) {
		return "/tmp/csgclaw", nil
	}

	configPath := filepath.Join(t.TempDir(), "blocked", "config.toml")
	if err := os.WriteFile(filepath.Dir(configPath), []byte("file blocks dir"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := StartApplyHelper(ApplyHelperOptions{ConfigPath: configPath})
	if err == nil || (!errors.Is(err, os.ErrNotExist) && !strings.Contains(err.Error(), "create upgrade helper state dir")) {
		t.Fatalf("StartApplyHelper() error = %v, want artifact setup failure", err)
	}
}
