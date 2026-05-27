package upgrade

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveApplyArtifactsUsesConfigDir(t *testing.T) {
	configPath := filepath.Join("/tmp", "custom", "config.toml")

	artifacts, err := ResolveApplyArtifacts(configPath)
	if err != nil {
		t.Fatalf("ResolveApplyArtifacts() error = %v", err)
	}

	if got, want := artifacts.StatusPath, filepath.Join("/tmp", "custom", applyLogsDirName, applyStatusFileName); got != want {
		t.Fatalf("StatusPath = %q, want %q", got, want)
	}
	if got, want := artifacts.LogPath, filepath.Join("/tmp", "custom", applyLogsDirName, applyLogFileName); got != want {
		t.Fatalf("LogPath = %q, want %q", got, want)
	}
}

func TestConsumeApplyFailureReturnsMessageAndClearsStatus(t *testing.T) {
	dir := t.TempDir()
	artifacts, err := ResolveApplyArtifacts(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("ResolveApplyArtifacts() error = %v", err)
	}

	if err := artifacts.RecordFailure(errors.New("restart daemon: boom")); err != nil {
		t.Fatalf("RecordFailure() error = %v", err)
	}

	got, err := ConsumeApplyFailure(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("ConsumeApplyFailure() error = %v", err)
	}
	if !strings.Contains(got, "restart daemon: boom") {
		t.Fatalf("message = %q, want failure text", got)
	}
	if !strings.Contains(got, artifacts.LogPath) {
		t.Fatalf("message = %q, want log path", got)
	}
	if _, err := os.Stat(artifacts.StatusPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("status file still exists; stat err = %v", err)
	}
}

func TestConsumeApplyStatusReturnsManualRestartRequired(t *testing.T) {
	dir := t.TempDir()
	artifacts, err := ResolveApplyArtifacts(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("ResolveApplyArtifacts() error = %v", err)
	}

	if err := artifacts.RecordManualRestartRequired("manual restart required"); err != nil {
		t.Fatalf("RecordManualRestartRequired() error = %v", err)
	}

	got, err := ConsumeApplyStatus(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("ConsumeApplyStatus() error = %v", err)
	}
	if got.Status != ApplyStatusManualRestartRequired {
		t.Fatalf("Status = %q, want %q", got.Status, ApplyStatusManualRestartRequired)
	}
	if got.Message != "manual restart required" {
		t.Fatalf("Message = %q, want manual restart text", got.Message)
	}
	if _, err := os.Stat(artifacts.StatusPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("status file still exists; stat err = %v", err)
	}
}
