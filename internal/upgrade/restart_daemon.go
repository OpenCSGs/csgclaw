package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

	if err := stopDaemonWithExecutable(ctx, exePath); err != nil {
		return RestartResult{}, fmt.Errorf("stop running daemon: %w", err)
	}

	if err := startDaemonWithExecutable(ctx, exePath, opts); err != nil {
		return RestartResult{}, fmt.Errorf("restart daemon: %w", err)
	}

	result.Restarted = true
	return result, nil
}

// RestartDaemonFromExecutable stops and restarts the daemon using the current process binary.
func RestartDaemonFromExecutable(ctx context.Context, opts RestartOptions) (RestartResult, error) {
	exePath, err := daemonLifecycleExecutable()
	if err != nil {
		return RestartResult{}, fmt.Errorf("resolve executable: %w", err)
	}
	return RestartDaemon(ctx, exePath, opts)
}

func StopDaemonFromExecutable(ctx context.Context) (RestartResult, error) {
	exePath, err := daemonLifecycleExecutable()
	if err != nil {
		return RestartResult{}, fmt.Errorf("resolve executable: %w", err)
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
	if err := stopDaemonWithExecutable(ctx, exePath); err != nil {
		return RestartResult{}, fmt.Errorf("stop running daemon: %w", err)
	}
	return result, nil
}

func StartDaemonFromExecutable(ctx context.Context, opts RestartOptions) error {
	exePath, err := daemonLifecycleExecutable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	return startDaemonWithExecutable(ctx, exePath, opts)
}

func StartInstalledDaemon(ctx context.Context, installed InstalledBundle, opts RestartOptions) (RestartResult, error) {
	layout, err := inspectBundleDir(installed.InstallRoot)
	if err != nil {
		return RestartResult{}, err
	}
	if err := startDaemonWithExecutable(ctx, layout.CSGClawPath, opts); err != nil {
		return RestartResult{}, fmt.Errorf("restart daemon: %w", err)
	}

	pidPath, err := defaultUpgradePIDPath()
	if err != nil {
		return RestartResult{}, err
	}
	return RestartResult{
		PIDPath:          pidPath,
		DaemonWasRunning: true,
		Restarted:        true,
	}, nil
}

func stopDaemonWithExecutable(ctx context.Context, exePath string) error {
	return runUpgradeCommand(ctx, exePath, "stop")
}

func daemonLifecycleExecutable() (string, error) {
	if exe := strings.TrimSpace(os.Getenv(originalExecutableEnvVar)); exe != "" {
		return exe, nil
	}
	return os.Executable()
}

func startDaemonWithExecutable(ctx context.Context, exePath string, opts RestartOptions) error {
	exeDir := filepath.Clean(filepath.Dir(strings.TrimSpace(exePath)))
	cmd := execCommandContext(ctx, exePath, commandArgsWithConfig(opts.ConfigPath, "serve", "--daemon")...)
	if exeDir != "" {
		cmd.Dir = exeDir
	}
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return fmt.Errorf("%s serve --daemon: %w", exePath, err)
	}
	return fmt.Errorf("%s serve --daemon: %w: %s", exePath, err, trimmed)
}
