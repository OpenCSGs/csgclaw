package installstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const lockFileName = ".skillhub-lock.json"

// Serialize read/modify/write operations from CLI and Web callers in this process.
var installRecordsMu sync.Mutex

type Record struct {
	Source      string    `json:"source,omitempty"`
	Registry    string    `json:"registry,omitempty"`
	RegistryURL string    `json:"registry_url,omitempty"`
	RemotePath  string    `json:"remote_path,omitempty"`
	Ref         string    `json:"ref,omitempty"`
	Slug        string    `json:"slug"`
	Version     string    `json:"version"`
	InstalledAt time.Time `json:"installed_at"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	SHA256      string    `json:"sha256,omitempty"`
}

const (
	SourceLocal   = "local"
	SourceMarket  = "market"
	SourceUnknown = "unknown"
)

func (r Record) SourceKind() string {
	switch r.Source {
	case SourceLocal, SourceMarket:
		return r.Source
	case "":
		// Older CLI records contain a registry but no explicit installation source.
		if r.Registry != "" {
			return SourceMarket
		}
	}
	return SourceUnknown
}

type lockFile struct {
	Skills map[string]Record `json:"skills"`
}

func readLockFile(skillsRoot string) (lockFile, error) {
	path := lockFilePath(skillsRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return lockFile{Skills: map[string]Record{}}, nil
		}
		return lockFile{}, fmt.Errorf("read skill lock file: %w", err)
	}
	var payload lockFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return lockFile{}, fmt.Errorf("decode skill lock file: %w", err)
	}
	if payload.Skills == nil {
		payload.Skills = map[string]Record{}
	}
	return payload, nil
}

func Read(skillsRoot string) (map[string]Record, error) {
	installRecordsMu.Lock()
	defer installRecordsMu.Unlock()
	payload, err := readLockFile(skillsRoot)
	return payload.Skills, err
}

// Update also coordinates a local install or removal. Persist first
// so a lock-file write failure leaves skill files untouched; restore the previous
// record if the file operation fails. A nil record removes the installation entry.
func Update(skillsRoot, name string, record *Record, apply func() error) error {
	installRecordsMu.Lock()
	defer installRecordsMu.Unlock()
	payload, err := readLockFile(skillsRoot)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	previous, existed := payload.Skills[name]
	if record != nil {
		next := *record
		next.Slug = name
		next.UpdatedAt = time.Now().UTC()
		next.InstalledAt = next.UpdatedAt
		if existed && !previous.InstalledAt.IsZero() {
			next.InstalledAt = previous.InstalledAt
		}
		payload.Skills[name] = next
	} else {
		delete(payload.Skills, name)
	}
	if record != nil || existed {
		if err := writeLockFile(skillsRoot, payload); err != nil {
			return err
		}
	}
	if apply != nil {
		if err := apply(); err != nil {
			if existed {
				payload.Skills[name] = previous
			} else {
				delete(payload.Skills, name)
			}
			if record != nil || existed {
				return errors.Join(err, writeLockFile(skillsRoot, payload))
			}
			return err
		}
	}
	return nil
}

func writeLockFile(skillsRoot string, payload lockFile) error {
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
