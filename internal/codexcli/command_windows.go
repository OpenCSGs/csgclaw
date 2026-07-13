//go:build windows

package codexcli

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func AppServerCommandContext(ctx context.Context, binaryPath string) (*exec.Cmd, error) {
	if !isWindowsCommandShimPath(binaryPath) {
		return exec.CommandContext(ctx, binaryPath, AppServerArgs()...), nil
	}
	commandLine, err := windowsBatchAppServerCommandLine(binaryPath)
	if err != nil {
		return nil, err
	}
	commandProcessor := strings.TrimSpace(os.Getenv("ComSpec"))
	if commandProcessor == "" {
		commandProcessor = "cmd.exe"
	}
	cmd := exec.CommandContext(ctx, commandProcessor)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: commandLine}
	return cmd, nil
}
