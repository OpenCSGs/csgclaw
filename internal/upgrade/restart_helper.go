package upgrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	restartStatusEnvVar   = "CSGCLAW_RESTART_STATUS_PATH"
	restartLogEnvVar      = "CSGCLAW_RESTART_LOG_PATH"
	restartStatusFileName = "config-restart-status.json"
	restartLogFileName    = "config-restart-helper.log"
)

var (
	startRestartExecutable = os.Executable
	startRestartCommand    = exec.Command
)

type RestartHelperOptions struct {
	ConfigPath string
}

type RestartArtifacts struct {
	StatusPath string
	LogPath    string
}

func ResolveRestartArtifacts(configPath string) (RestartArtifacts, error) {
	artifacts, err := ResolveApplyArtifacts(configPath)
	if err != nil {
		return RestartArtifacts{}, err
	}
	logDir := filepath.Dir(artifacts.LogPath)
	return RestartArtifacts{
		StatusPath: filepath.Join(logDir, restartStatusFileName),
		LogPath:    filepath.Join(logDir, restartLogFileName),
	}, nil
}

func PrepareRestartArtifacts(configPath string) (RestartArtifacts, error) {
	artifacts, err := ResolveRestartArtifacts(configPath)
	if err != nil {
		return RestartArtifacts{}, err
	}
	if err := os.MkdirAll(filepath.Dir(artifacts.StatusPath), 0o755); err != nil {
		return RestartArtifacts{}, fmt.Errorf("create restart helper state dir: %w", err)
	}
	if err := os.Remove(artifacts.StatusPath); err != nil && !os.IsNotExist(err) {
		return RestartArtifacts{}, fmt.Errorf("remove stale restart helper status: %w", err)
	}
	return artifacts, nil
}

func RestartArtifactsFromEnv() RestartArtifacts {
	return RestartArtifacts{
		StatusPath: os.Getenv(restartStatusEnvVar),
		LogPath:    os.Getenv(restartLogEnvVar),
	}
}

func (a RestartArtifacts) Enabled() bool {
	return a.StatusPath != ""
}

func (a RestartArtifacts) Env() []string {
	if !a.Enabled() {
		return nil
	}
	env := []string{fmt.Sprintf("%s=%s", restartStatusEnvVar, a.StatusPath)}
	if a.LogPath != "" {
		env = append(env, fmt.Sprintf("%s=%s", restartLogEnvVar, a.LogPath))
	}
	return env
}

func (a RestartArtifacts) ClearStatus() error {
	if !a.Enabled() {
		return nil
	}
	if err := os.Remove(a.StatusPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove restart helper status: %w", err)
	}
	return nil
}

func (a RestartArtifacts) RecordFailure(err error) error {
	if err == nil || !a.Enabled() {
		return nil
	}
	record := applyFailureRecord{
		Status:    ApplyStatusFailed,
		Message:   err.Error(),
		LogPath:   a.LogPath,
		UpdatedAt: time.Now().UTC(),
	}
	return writeRestartStatus(a.StatusPath, record)
}

func (a RestartArtifacts) RecordManualRestartRequired(message string) error {
	if !a.Enabled() {
		return nil
	}
	record := applyFailureRecord{
		Status:    ApplyStatusManualRestartRequired,
		Message:   message,
		LogPath:   a.LogPath,
		UpdatedAt: time.Now().UTC(),
	}
	return writeRestartStatus(a.StatusPath, record)
}

func writeRestartStatus(path string, record applyFailureRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create restart helper state dir: %w", err)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode restart helper status: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write restart helper status: %w", err)
	}
	return nil
}

func ConsumeRestartStatus(configPath string) (ApplyStatusRecord, error) {
	artifacts, err := ResolveRestartArtifacts(configPath)
	if err != nil {
		return ApplyStatusRecord{}, err
	}
	data, err := os.ReadFile(artifacts.StatusPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ApplyStatusRecord{}, nil
		}
		return ApplyStatusRecord{}, fmt.Errorf("read restart helper status: %w", err)
	}

	var record applyFailureRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return ApplyStatusRecord{}, fmt.Errorf("decode restart helper status: %w", err)
	}
	if err := os.Remove(artifacts.StatusPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ApplyStatusRecord{}, fmt.Errorf("remove consumed restart helper status: %w", err)
	}

	message := strings.TrimSpace(record.Message)
	status := strings.TrimSpace(record.Status)
	if status == "" {
		status = ApplyStatusFailed
	}
	if status == ApplyStatusFailed {
		if message == "" {
			return ApplyStatusRecord{}, nil
		}
		if logPath := strings.TrimSpace(record.LogPath); logPath != "" {
			message = fmt.Sprintf("%s\nLog: %s", message, logPath)
		}
	}
	if message == "" && status != ApplyStatusManualRestartRequired {
		return ApplyStatusRecord{}, nil
	}
	return ApplyStatusRecord{
		Status:  status,
		Message: message,
	}, nil
}

func StartRestartHelper(opts RestartHelperOptions) error {
	exe, err := startRestartExecutable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	artifacts, err := PrepareRestartArtifacts(opts.ConfigPath)
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
		return fmt.Errorf("open restart helper log %s: %w", artifacts.LogPath, err)
	}

	cmd := startRestartCommand(exe, commandArgsWithConfig(opts.ConfigPath, "_restart")...)
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(), artifacts.Env()...)

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		_ = devNull.Close()
		return fmt.Errorf("start restart helper: %w", err)
	}

	go func() {
		_ = cmd.Wait()
		_ = logFile.Close()
		_ = devNull.Close()
	}()

	return nil
}
