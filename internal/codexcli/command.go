package codexcli

import (
	"context"
	"os/exec"
)

// AppServerCommandContext starts the bundled native Codex executable.
func AppServerCommandContext(ctx context.Context, binaryPath string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, binaryPath, AppServerArgs()...), nil
}

func AppServerCommandContextWithOverrides(ctx context.Context, binaryPath string, overrides []string) (*exec.Cmd, error) {
	args := []string{"app-server", "--disable", "plugins"}
	args = append(args, overrides...)
	args = append(args, "--listen", "stdio://")
	return exec.CommandContext(ctx, binaryPath, args...), nil
}
