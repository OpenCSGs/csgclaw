package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const lockFileName = ".skillhub-lock.json"

type lockFile struct {
	Skills map[string]InstallRecord `json:"skills"`
}

func readLockFile(skillsRoot string) (lockFile, error) {
	path := lockFilePath(skillsRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return lockFile{Skills: map[string]InstallRecord{}}, nil
		}
		return lockFile{}, fmt.Errorf("read skill lock file: %w", err)
	}
	var payload lockFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return lockFile{}, fmt.Errorf("decode skill lock file: %w", err)
	}
	if payload.Skills == nil {
		payload.Skills = map[string]InstallRecord{}
	}
	return payload, nil
}

func writeLockRecord(skillsRoot string, record InstallRecord) error {
	payload, err := readLockFile(skillsRoot)
	if err != nil {
		return err
	}
	payload.Skills[normalizeSlug(record.Slug)] = record
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode skill lock file: %w", err)
	}
	path := lockFilePath(skillsRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create skills root: %w", err)
	}
	return writeFileAtomic(path, append(data, '\n'), 0o644)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".skill-lock-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary skill lock file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temporary skill lock file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary skill lock file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temporary skill lock file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary skill lock file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace skill lock file: %w", err)
	}
	cleanup = false
	return nil
}

func lockFilePath(skillsRoot string) string {
	return filepath.Join(skillsRoot, lockFileName)
}

func newInstallRecord(registry RegistryID, slug, version, sha256 string) InstallRecord {
	return InstallRecord{
		Registry:    registry,
		Slug:        normalizeSlug(slug),
		Version:     strings.TrimSpace(version),
		InstalledAt: time.Now().UTC(),
		SHA256:      sha256,
	}
}
