package upgrade

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const upgradeHelperTempPattern = "csgclaw-upgrade-helper-*"

var (
	startHelperExecutable = os.Executable
	startHelperCommand    = exec.Command
)

type ApplyHelperOptions struct {
	ConfigPath string
}

func StartApplyHelper(opts ApplyHelperOptions) error {
	exe, originalExe, err := helperLaunchExecutable()
	if err != nil {
		return err
	}

	artifacts, err := PrepareApplyArtifacts(opts.ConfigPath)
	if err != nil {
		return err
	}

	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", os.DevNull, err)
	}

	logFile, err := os.OpenFile(artifacts.LogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		_ = devNull.Close()
		return fmt.Errorf("open upgrade helper log %s: %w", artifacts.LogPath, err)
	}

	cmd := startHelperCommand(exe, commandArgsWithConfig(opts.ConfigPath, "upgrade")...)
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(), artifacts.Env()...)
	if originalExe != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", originalExecutableEnvVar, originalExe))
	}

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		_ = devNull.Close()
		return fmt.Errorf("start upgrade helper: %w", err)
	}

	go func() {
		_ = cmd.Wait()
		_ = logFile.Close()
		_ = devNull.Close()
	}()

	return nil
}

func helperLaunchExecutable() (string, string, error) {
	exe, err := startHelperExecutable()
	if err != nil {
		return "", "", fmt.Errorf("resolve executable: %w", err)
	}
	if runtime.GOOS != "windows" {
		return exe, exe, nil
	}
	cleanupUpgradeHelperTempDirs("")
	launchExe, err := copyHelperExecutableToTemp(exe)
	if err != nil {
		return "", "", err
	}
	return launchExe, exe, nil
}

func copyHelperExecutableToTemp(exe string) (string, error) {
	src, err := os.Open(exe)
	if err != nil {
		return "", fmt.Errorf("open helper executable %s: %w", exe, err)
	}
	defer src.Close()

	helperDir, err := os.MkdirTemp("", upgradeHelperTempPattern)
	if err != nil {
		return "", fmt.Errorf("create helper temp dir: %w", err)
	}

	target := filepath.Join(helperDir, filepath.Base(exe))
	dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		return "", fmt.Errorf("create helper executable %s: %w", target, err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return "", fmt.Errorf("copy helper executable to %s: %w", target, err)
	}
	if err := dst.Close(); err != nil {
		return "", fmt.Errorf("close helper executable %s: %w", target, err)
	}
	return target, nil
}

func cleanupUpgradeHelperTempDirs(tempRoot string) {
	if tempRoot == "" {
		tempRoot = os.TempDir()
	}
	matches, err := filepath.Glob(filepath.Join(tempRoot, upgradeHelperTempPattern))
	if err != nil {
		return
	}
	for _, path := range matches {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		_ = os.RemoveAll(path)
	}
}
