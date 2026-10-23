package installstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const lockFileName = ".skillhub-lock.json"

func IsRecordsFile(name string) bool { return name == lockFileName }

// Serialize callers in this process; the OS file lock also covers other processes.
var installRecordsMu sync.Mutex

type Record struct {
	Source      string    `json:"source,omitempty"`
	SourceURL   string    `json:"source_url,omitempty"`
	Registry    string    `json:"registry,omitempty"` // Older CLI records; new installs use SourceURL.
	Ref         string    `json:"ref,omitempty"`
	Slug        string    `json:"slug"`
	Version     string    `json:"version"`
	InstalledAt time.Time `json:"installed_at"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	SHA256      string    `json:"sha256,omitempty"`
}

const (
	SourceBuiltin = "builtin"
	SourceLocal   = "local"
	SourceMarket  = "market"
	SourceUnknown = "unknown"
)

func (r Record) SourceKind() string {
	switch r.Source {
	case SourceBuiltin, SourceLocal, SourceMarket:
		return r.Source
	case "":
		// Recognize full source URLs and older CLI records without an explicit source.
		if r.Registry != "" || r.SourceURL != "" {
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
	var records map[string]Record
	err := readSnapshot(skillsRoot, false, func(snapshot map[string]Record) error {
		records = snapshot
		return nil
	})
	return records, err
}

// ReadSnapshot holds the installation lock while consuming origins and files.
// It creates a missing root so concurrent installations share the same lock.
// The callback must not call other installstate functions.
func ReadSnapshot(skillsRoot string, consume func(map[string]Record) error) error {
	return readSnapshot(skillsRoot, true, consume)
}

func readSnapshot(skillsRoot string, createRoot bool, consume func(map[string]Record) error) error {
	installRecordsMu.Lock()
	defer installRecordsMu.Unlock()
	lock, err := acquireRecordsLock(skillsRoot, createRoot)
	if err != nil {
		return err
	}
	if lock == nil {
		return consume(map[string]Record{})
	}
	defer lock.Close()
	payload, err := readLockFile(skillsRoot)
	if err != nil {
		return err
	}
	return consume(payload.Skills)
}

// RestoreRecords merges preserved origins with regenerated origins without modifying the recovery inputs.
func RestoreRecords(preservedRoot, generatedRoot, targetRoot string) error {
	installRecordsMu.Lock()
	defer installRecordsMu.Unlock()
	// Unreadable origins do not prevent skill recovery; original files remain available for rollback.
	preserved, _ := readLockFile(preservedRoot)
	generated, _ := readLockFile(generatedRoot)
	if preserved.Skills == nil {
		preserved.Skills = make(map[string]Record)
	}
	for name, record := range generated.Skills {
		if _, exists := preserved.Skills[name]; !exists || record.SourceKind() == SourceBuiltin {
			preserved.Skills[name] = record
		}
	}
	return writeLockFile(targetRoot, preserved)
}

// Update also coordinates a local install or removal. Persist first
// so a lock-file write failure leaves skill files untouched; restore the previous
// record if the file operation fails. A nil record removes the installation entry.
func Update(skillsRoot, name string, record *Record, apply func() error) error {
	return UpdateMany(skillsRoot, map[string]*Record{strings.TrimSpace(name): record}, apply)
}

// UpdateMany persists related installation records together and restores them
// when the prepared file operation fails.
func UpdateMany(skillsRoot string, records map[string]*Record, apply func() error) error {
	installRecordsMu.Lock()
	defer installRecordsMu.Unlock()
	lock, err := acquireRecordsLock(skillsRoot, true)
	if err != nil {
		return err
	}
	defer lock.Close()
	payload, err := readLockFile(skillsRoot)
	if err != nil {
		return err
	}
	rollback, finish, err := cancelActivations(skillsRoot, records)
	if err != nil {
		return err
	}
	if err := updateManyLocked(skillsRoot, payload, records, apply); err != nil {
		return errors.Join(err, rollback())
	}
	if err := finish(); err != nil {
		// Cancellation is already durable. Retry artifact cleanup on the next sync.
		slog.Warn("cleanup cancelled skill activation", "error", err)
	}
	return nil
}

// Reconcile computes file and origin changes while holding the installation lock.
// Files are activated before origin records are committed. Callers must provide
// crash recovery for file changes; the callback must not call installstate functions.
func Reconcile(root string, prepare func(map[string]Record) (map[string]*Record, func() error, error)) error {
	installRecordsMu.Lock()
	defer installRecordsMu.Unlock()
	lock, err := acquireRecordsLock(root, true)
	if err != nil {
		return err
	}
	defer lock.Close()
	payload, err := readLockFile(root)
	if err != nil {
		return err
	}
	records, apply, err := prepare(maps.Clone(payload.Skills))
	if err != nil {
		return err
	}
	// Reconcile callers must journal file changes so an interrupted record commit
	// can be recovered. Never advertise a new version before activating its files.
	if apply != nil {
		if err := apply(); err != nil {
			return err
		}
	}
	return updateManyLocked(root, payload, records, nil)
}

func updateManyLocked(skillsRoot string, payload lockFile, records map[string]*Record, apply func() error) error {
	previous := maps.Clone(payload.Skills)
	changed := false
	for name, record := range records {
		old, existed := payload.Skills[name]
		if record != nil {
			next := *record
			next.Slug = name
			next.UpdatedAt = time.Now().UTC()
			next.InstalledAt = next.UpdatedAt
			if existed && !old.InstalledAt.IsZero() {
				next.InstalledAt = old.InstalledAt
			}
			payload.Skills[name] = next
		} else {
			delete(payload.Skills, name)
		}
		changed = changed || record != nil || existed
	}
	if changed {
		if err := writeLockFile(skillsRoot, payload); err != nil {
			return err
		}
	}
	if apply != nil {
		if err := apply(); err != nil {
			payload.Skills = previous
			if changed {
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
