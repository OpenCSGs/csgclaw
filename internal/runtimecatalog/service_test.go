package runtimecatalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/codexcli"
)

func TestServiceListReportsCodexAndClaudeCode(t *testing.T) {
	target := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv(codexcli.EnvBinaryPath, target)
	service := NewService(WithPlatform("darwin", "arm64"))

	runtimes := service.List()
	if len(runtimes) != 2 {
		t.Fatalf("List() length = %d, want 2: %+v", len(runtimes), runtimes)
	}
	if got := runtimes[0]; got.Name != RuntimeCodex || !got.Supported || !got.Installed || !got.Installable || got.Path != target {
		t.Fatalf("Codex runtime = %+v, want installed and installable", got)
	}
	if got := runtimes[1]; got.Name != RuntimeClaudeCode || got.Supported || got.Installed || got.Installable || got.Status != StatusComingSoon {
		t.Fatalf("Claude Code runtime = %+v, want coming soon", got)
	}
	for _, got := range runtimes {
		if got.OS != "darwin" || got.Arch != "arm64" {
			t.Fatalf("runtime platform = %s/%s, want darwin/arm64: %+v", got.OS, got.Arch, got)
		}
	}
}

func TestServiceListReportsMissingCodexFromEnvOverride(t *testing.T) {
	t.Setenv(codexcli.EnvBinaryPath, filepath.Join(t.TempDir(), "missing-codex"))

	got := NewService().List()[0]
	if got.Name != RuntimeCodex || got.Installed || got.Status != string(codexcli.InstallStateNotInstalled) {
		t.Fatalf("Codex runtime = %+v, want not installed", got)
	}
}

func TestServiceInstallRejectsUnsupportedAndUnknownRuntimes(t *testing.T) {
	service := NewService()
	if _, err := service.Install(context.Background(), RuntimeClaudeCode); !errors.Is(err, ErrInstallUnsupported) {
		t.Fatalf("Install(claude_code) error = %v, want ErrInstallUnsupported", err)
	}
	if _, err := service.Install(context.Background(), "unknown"); !errors.Is(err, ErrRuntimeNotFound) {
		t.Fatalf("Install(unknown) error = %v, want ErrRuntimeNotFound", err)
	}
}

func TestServiceListDisablesInstallOnUnsupportedPlatform(t *testing.T) {
	t.Setenv(codexcli.EnvBinaryPath, filepath.Join(t.TempDir(), "missing-codex"))

	got := NewService(WithPlatform("freebsd", "amd64")).List()[0]
	if got.Supported || got.Installable || got.Installed || got.Status != StatusUnsupported {
		t.Fatalf("Codex runtime = %+v, want unsupported and not installable", got)
	}
	if got.Message == "" {
		t.Fatal("Codex unsupported runtime message is empty")
	}
}
