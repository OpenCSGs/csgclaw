//go:build !windows

package codexcli

import (
	"context"
	"os/exec"
)

func AppServerCommandContext(ctx context.Context, binaryPath string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, binaryPath, AppServerArgs()...), nil
}
