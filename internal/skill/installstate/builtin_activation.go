package installstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// A journal is durable before activation. It remains until a later locked sync
// observes matching files AND committed origins, covering a failed record write.
type activation struct {
	Stage string `json:"stage"`
	Hash  string `json:"hash"`
}

// IsBuiltinArtifact identifies local-only transaction entries at the skills root.
// Files with the same names inside a skill are ordinary skill resources.
func IsBuiltinArtifact(name string) bool {
	return name == ".builtin-staging" || strings.HasPrefix(name, ".builtin-stage-") ||
		strings.HasPrefix(name, ".builtin-journal-") ||
		(strings.HasPrefix(name, ".builtin-") && (strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".json.cancelled")))
}

func BuiltinActivationPath(root, name string) string {
	return filepath.Join(root, ".builtin-"+name+".json")
}

func WriteBuiltinActivation(root, name, stage, hash string) error {
	data, err := json.Marshal(activation{Stage: filepath.Base(stage), Hash: hash})
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(root, ".builtin-journal-")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, BuiltinActivationPath(root, name))
}

// recoverActivation runs under the same installation lock as activation and
// origin commits. A matching new bundle is adopted; an incomplete rename is
// rolled back. User edits after interruption are never overwritten.
func RecoverBuiltinActivation(root, name string, record Record) (Record, bool, error) {
	if err := discardCancelledActivation(root, name); err != nil {
		return record, false, err
	}
	data, err := os.ReadFile(BuiltinActivationPath(root, name))
	if errors.Is(err, os.ErrNotExist) {
		return record, false, nil
	}
	if err != nil {
		return record, false, err
	}
	var pending activation
	if err := json.Unmarshal(data, &pending); err != nil {
		return record, false, err
	}
	if filepath.Base(pending.Stage) != pending.Stage || !strings.HasPrefix(pending.Stage, ".builtin-stage-") || strings.ContainsAny(pending.Stage, "/\\") || pending.Hash == "" {
		return record, false, fmt.Errorf("invalid bundled skill activation journal")
	}
	stage := filepath.Join(root, ".builtin-staging", pending.Stage)
	backup := stage + ".previous"
	// A matching committed origin proves activation completed. Subsequent edits
	// belong to the user, so clean only transaction artifacts without hashing or
	// restoring the active bundle.
	if record.Source == SourceBuiltin && record.Version == pending.Hash && record.SHA256 == pending.Hash {
		return record, false, clearActivation(root, name, stage, backup)
	}
	dst := filepath.Join(root, name)
	info, err := os.Lstat(dst)
	if errors.Is(err, os.ErrNotExist) {
		if _, backupErr := os.Lstat(backup); backupErr == nil {
			if err := os.Rename(backup, dst); err != nil {
				return record, false, err
			}
		} else if !errors.Is(backupErr, os.ErrNotExist) {
			return record, false, backupErr
		}
	} else if err != nil {
		return record, false, err
	} else if info.IsDir() {
		hash, err := BuiltinBundleSHA256(dst)
		if err != nil {
			return record, false, err
		}
		if hash == pending.Hash {
			next := Record{Source: SourceBuiltin, Version: hash, SHA256: hash}
			if record.Source != next.Source || record.Version != hash || record.SHA256 != hash {
				// Keep the journal until this reconstructed origin has been committed.
				return next, true, nil
			}
		} else if _, err := os.Lstat(backup); err == nil {
			return record, false, fmt.Errorf("bundled skill %s changed after interrupted activation; preserved backup at %s", name, backup)
		} else if !errors.Is(err, os.ErrNotExist) {
			return record, false, err
		}
	}
	// Either the origin commit completed, or activation never replaced the old
	// bundle. Removing staging data is safe in both cases.
	return record, false, clearActivation(root, name, stage, backup)
}

func clearActivation(root, name, stage, backup string) error {
	for _, path := range []string{stage, backup} {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return os.Remove(BuiltinActivationPath(root, name))
}

// cancelActivations runs under the installation lock before an explicit mutation.
// Renaming journals first prevents crash recovery from undoing a user operation.
// An ordinary failed operation restores the journals; success discards backups.
func cancelActivations(root string, records map[string]*Record) (func() error, func() error, error) {
	var cancelled []string
	restore := func() error {
		var failures []error
		for _, name := range cancelled {
			path := BuiltinActivationPath(root, name)
			if err := os.Rename(path+".cancelled", path); err != nil {
				failures = append(failures, err)
			}
		}
		return errors.Join(failures...)
	}
	finish := func() error {
		var failures []error
		for _, name := range cancelled {
			if err := discardCancelledActivation(root, name); err != nil {
				failures = append(failures, err)
			}
		}
		return errors.Join(failures...)
	}
	for name := range records {
		// Never allow an invalid resource name to address a journal outside the root.
		if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
			continue
		}
		if err := discardCancelledActivation(root, name); err != nil {
			return nil, nil, errors.Join(err, restore())
		}
		path := BuiltinActivationPath(root, name)
		if err := os.Rename(path, path+".cancelled"); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, nil, errors.Join(err, restore())
		}
		cancelled = append(cancelled, name)
	}
	return restore, finish, nil
}

func discardCancelledActivation(root, name string) error {
	path := BuiltinActivationPath(root, name) + ".cancelled"
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var pending activation
	if json.Unmarshal(data, &pending) == nil && filepath.Base(pending.Stage) == pending.Stage && strings.HasPrefix(pending.Stage, ".builtin-stage-") && !strings.ContainsAny(pending.Stage, "/\\") {
		stage := filepath.Join(root, ".builtin-staging", pending.Stage)
		for _, target := range []string{stage, stage + ".previous"} {
			if err := os.RemoveAll(target); err != nil {
				return err
			}
		}
	}
	// A corrupt journal may be discarded by an explicit user mutation too. Unknown
	// staging data is retained rather than guessing which backups belong to it.
	return os.Remove(path)
}
