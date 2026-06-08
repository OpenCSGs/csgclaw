package upgrade

import (
	"fmt"
	"os"
	"os/exec"
)

var (
	startHelperExecutable = os.Executable
	startHelperCommand    = exec.Command
)

type ApplyHelperOptions struct {
	ConfigPath string
}

func StartApplyHelper(opts ApplyHelperOptions) error {
	exe, err := startHelperExecutable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
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
