package upgrade

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// RestartDaemon stops a running daemon (via server.pid) and starts serve --daemon again.
// exePath must point at the csgclaw binary to use for stop/start.
func RestartDaemon(ctx context.Context, exePath string, opts RestartOptions) (RestartResult, error) {
	exePath = strings.TrimSpace(exePath)
	if exePath == "" {
		return RestartResult{}, fmt.Errorf("executable path is required")
	}

	pidPath, err := defaultUpgradePIDPath()
	if err != nil {
		return RestartResult{}, err
	}

	running, stale, err := daemonRunning(pidPath)
	if err != nil {
		return RestartResult{}, err
	}
	result := RestartResult{
		PIDPath:          pidPath,
		DaemonWasRunning: running,
	}
	if stale {
		_ = removePIDFilePath(pidPath)
	}
	if !running {
		return result, nil
	}

	if err := runUpgradeCommand(ctx, exePath, "stop"); err != nil {
		return RestartResult{}, fmt.Errorf("stop running daemon: %w", err)
	}

	if err := runUpgradeCommand(ctx, exePath, commandArgsWithConfig(opts.ConfigPath, "serve", "-d")...); err != nil {
		return RestartResult{}, fmt.Errorf("restart daemon: %w", err)
	}

	result.Restarted = true
	return result, nil
}

// RestartDaemonFromExecutable stops and restarts the daemon using the current process binary.
func RestartDaemonFromExecutable(ctx context.Context, opts RestartOptions) (RestartResult, error) {
	exePath, err := os.Executable()
	if err != nil {
		return RestartResult{}, fmt.Errorf("resolve executable: %w", err)
	}
	return RestartDaemon(ctx, exePath, opts)
}
