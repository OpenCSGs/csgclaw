package upgrade

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStartRestartHelperIncludesConfigPath(t *testing.T) {
	origExecutable := startRestartExecutable
	origCommand := startRestartCommand
	t.Cleanup(func() {
		startRestartExecutable = origExecutable
		startRestartCommand = origCommand
	})

	startRestartExecutable = func() (string, error) {
		return "/tmp/csgclaw", nil
	}

	var gotName string
	var gotArgs []string
	var startedCmd *exec.Cmd
	startRestartCommand = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		startedCmd = exec.Command("sh", "-c", "exit 0")
		return startedCmd
	}

	configPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(configPath, []byte("[server]\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := StartRestartHelper(RestartHelperOptions{ConfigPath: configPath}); err != nil {
		t.Fatalf("StartRestartHelper() error = %v", err)
	}

	if gotName != "/tmp/csgclaw" {
		t.Fatalf("command name = %q, want %q", gotName, "/tmp/csgclaw")
	}
	wantArgs := []string{"--config", configPath, "_restart"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", gotArgs, wantArgs)
	}
	artifacts, err := ResolveRestartArtifacts(configPath)
	if err != nil {
		t.Fatalf("ResolveRestartArtifacts() error = %v", err)
	}
	if startedCmd == nil {
		t.Fatal("startRestartCommand was not called")
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

func TestConsumeRestartStatusManualRestartRequired(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	artifacts, err := ResolveRestartArtifacts(configPath)
	if err != nil {
		t.Fatalf("ResolveRestartArtifacts() error = %v", err)
	}
	if err := artifacts.RecordManualRestartRequired("manual restart required"); err != nil {
		t.Fatalf("RecordManualRestartRequired() error = %v", err)
	}

	record, err := ConsumeRestartStatus(configPath)
	if err != nil {
		t.Fatalf("ConsumeRestartStatus() error = %v", err)
	}
	if record.Status != ApplyStatusManualRestartRequired {
		t.Fatalf("status = %q, want %q", record.Status, ApplyStatusManualRestartRequired)
	}
}
