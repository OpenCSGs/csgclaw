package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadRawFile returns the on-disk config.toml bytes.
func ReadRawFile(path string) ([]byte, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return nil, fmt.Errorf("config path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config not found at %s; run `csgclaw serve` to initialize local state first", path)
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	return data, nil
}

// ValidateRaw parses config content without writing it to disk.
func ValidateRaw(content []byte) error {
	tmp, err := os.CreateTemp("", "csgclaw-config-validate-*.toml")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if _, err := Load(tmpPath); err != nil {
		return err
	}
	return nil
}

// WriteRawFile validates and writes config content to path.
func WriteRawFile(path string, content []byte) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return fmt.Errorf("config path is required")
	}
	if err := ValidateRaw(content); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
