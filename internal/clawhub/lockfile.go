package clawhub

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
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write skill lock file: %w", err)
	}
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
