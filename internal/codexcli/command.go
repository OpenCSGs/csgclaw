package codexcli

import (
	"context"
	"os/exec"
)

// AppServerCommandContext starts the bundled native Codex executable.
func AppServerCommandContext(ctx context.Context, binaryPath string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, binaryPath, AppServerArgs()...), nil
}
