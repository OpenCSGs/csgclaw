//go:build windows

package codexcli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppServerCommandContextRunsWindowsCommandShim(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "npm shim & (test)")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	shimPath := filepath.Join(dir, "codex.cmd")
	shim := "@echo off\r\nset /p input=\r\necho %1^|%2^|%3^|%input%\r\n"
	if err := os.WriteFile(shimPath, []byte(shim), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd, err := AppServerCommandContext(context.Background(), shimPath)
	if err != nil {
		t.Fatalf("AppServerCommandContext() error = %v", err)
	}
	cmd.Stdin = strings.NewReader("ping\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error = %v; stderr=%s", err, stderr.String())
	}
	if got, want := strings.TrimSpace(stdout.String()), "app-server|--listen|stdio://|ping"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}
