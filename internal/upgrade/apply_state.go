package upgrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"csgclaw/internal/config"
)

const (
	applyStatusEnvVar   = "CSGCLAW_UPGRADE_STATUS_PATH"
	applyLogEnvVar      = "CSGCLAW_UPGRADE_LOG_PATH"
	applyStatusFileName = "upgrade-helper-status.json"
	applyLogFileName    = "upgrade-helper.log"
	applyLogsDirName    = "logs"
)

type ApplyArtifacts struct {
	StatusPath string
	LogPath    string
}

type applyFailureRecord struct {
	Message   string    `json:"message"`
	LogPath   string    `json:"log_path,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

func ResolveApplyArtifacts(configPath string) (ApplyArtifacts, error) {
	dir, err := applyArtifactsDir(configPath)
	if err != nil {
		return ApplyArtifacts{}, err
	}
	return ApplyArtifacts{
		StatusPath: filepath.Join(dir, applyLogsDirName, applyStatusFileName),
		LogPath:    filepath.Join(dir, applyLogsDirName, applyLogFileName),
	}, nil
}

func PrepareApplyArtifacts(configPath string) (ApplyArtifacts, error) {
	artifacts, err := ResolveApplyArtifacts(configPath)
	if err != nil {
		return ApplyArtifacts{}, err
	}
	if err := os.MkdirAll(filepath.Dir(artifacts.StatusPath), 0o755); err != nil {
		return ApplyArtifacts{}, fmt.Errorf("create upgrade helper state dir: %w", err)
	}
	if err := os.Remove(artifacts.StatusPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ApplyArtifacts{}, fmt.Errorf("remove stale upgrade helper status: %w", err)
	}
	return artifacts, nil
}

func ApplyArtifactsFromEnv() ApplyArtifacts {
	return ApplyArtifacts{
		StatusPath: strings.TrimSpace(os.Getenv(applyStatusEnvVar)),
		LogPath:    strings.TrimSpace(os.Getenv(applyLogEnvVar)),
	}
}

func (a ApplyArtifacts) Enabled() bool {
	return strings.TrimSpace(a.StatusPath) != ""
}

func (a ApplyArtifacts) Env() []string {
	if !a.Enabled() {
		return nil
	}
	env := []string{fmt.Sprintf("%s=%s", applyStatusEnvVar, a.StatusPath)}
	if strings.TrimSpace(a.LogPath) != "" {
		env = append(env, fmt.Sprintf("%s=%s", applyLogEnvVar, a.LogPath))
	}
	return env
}

func (a ApplyArtifacts) ClearStatus() error {
	if !a.Enabled() {
		return nil
	}
	if err := os.Remove(a.StatusPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove upgrade helper status: %w", err)
	}
	return nil
}

func (a ApplyArtifacts) RecordFailure(err error) error {
	if err == nil || !a.Enabled() {
		return nil
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(a.StatusPath), 0o755); mkdirErr != nil {
		return fmt.Errorf("create upgrade helper state dir: %w", mkdirErr)
	}
	record := applyFailureRecord{
		Message:   err.Error(),
		LogPath:   strings.TrimSpace(a.LogPath),
		UpdatedAt: time.Now().UTC(),
	}
	data, marshalErr := json.MarshalIndent(record, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("encode upgrade helper status: %w", marshalErr)
	}
	if writeErr := os.WriteFile(a.StatusPath, append(data, '\n'), 0o600); writeErr != nil {
		return fmt.Errorf("write upgrade helper status: %w", writeErr)
	}
	return nil
}

func ConsumeApplyFailure(configPath string) (string, error) {
	artifacts, err := ResolveApplyArtifacts(configPath)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(artifacts.StatusPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read upgrade helper status: %w", err)
	}

	var record applyFailureRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return "", fmt.Errorf("decode upgrade helper status: %w", err)
	}
	if err := os.Remove(artifacts.StatusPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("remove consumed upgrade helper status: %w", err)
	}

	message := strings.TrimSpace(record.Message)
	if message == "" {
		return "", nil
	}
	if logPath := strings.TrimSpace(record.LogPath); logPath != "" {
		message = fmt.Sprintf("%s\nLog: %s", message, logPath)
	}
	return message, nil
}

func applyArtifactsDir(configPath string) (string, error) {
	if path := strings.TrimSpace(configPath); path != "" {
		return filepath.Dir(path), nil
	}
	return config.DefaultDir()
}
