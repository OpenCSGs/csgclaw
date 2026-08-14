package upgrade

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"csgclaw/internal/config"
)

var (
	readPIDFileData     = os.ReadFile
	removePIDFilePath   = os.Remove
	findProcessByPID    = os.FindProcess
	processRunningByPID = processRunning
	execCommandContext  = exec.CommandContext
)

type RestartOptions struct {
	ConfigPath string
}

type RestartResult struct {
	PIDPath          string `json:"pid_path,omitempty"`
	DaemonWasRunning bool   `json:"daemon_was_running"`
	Restarted        bool   `json:"restarted"`
}

func (c Client) RestartIfRunning(ctx context.Context, installed InstalledBundle, opts RestartOptions) (RestartResult, error) {
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

	layout, err := inspectBundleDir(installed.InstallRoot)
	if err != nil {
		return RestartResult{}, err
	}
	return RestartDaemon(ctx, layout.CSGClawPath, opts)
}

func defaultUpgradePIDPath() (string, error) {
	dir, err := config.DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "server.pid"), nil
}

func daemonRunning(pidPath string) (running bool, stale bool, err error) {
	pid, err := readUpgradePID(pidPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, false, nil
		}
		return false, false, err
	}
	running, stale, err = processRunningByPID(pid)
	if err != nil {
		return false, false, fmt.Errorf("check process %d: %w", pid, err)
	}
	return running, stale, nil
}

func readUpgradePID(path string) (int, error) {
	data, err := readPIDFileData(path)
	if err != nil {
		return 0, fmt.Errorf("read pid file: %w", err)
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil {
		return 0, fmt.Errorf("parse pid file: %w", err)
	}
	if pid <= 0 {
		return 0, fmt.Errorf("parse pid file: invalid pid %d", pid)
	}
	return pid, nil
}

func runUpgradeCommand(ctx context.Context, exePath string, args ...string) error {
	cmd := execCommandContext(ctx, exePath, args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return fmt.Errorf("%s %s: %w", exePath, strings.Join(args, " "), err)
	}
	return fmt.Errorf("%s %s: %w: %s", exePath, strings.Join(args, " "), err, trimmed)
}
