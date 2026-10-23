// Package builtin releases bundled skills into the local catalog and manager workspace.
package builtin

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"csgclaw/internal/skill/installstate"
	"csgclaw/internal/skill/local"
)

//go:embed all:embed manifest.json
var bundled embed.FS

func Names() []string {
	var hashes map[string]string
	data, _ := bundled.ReadFile("manifest.json")
	if err := json.Unmarshal(data, &hashes); err != nil {
		panic(err)
	}
	names := make([]string, 0, len(hashes))
	for name := range hashes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func SyncLocal() error {
	root, err := local.SkillsRoot()
	if err != nil {
		return err
	}
	return Sync(root)
}

// Sync installs missing skills and updates untouched bundled copies. Matching
// manifest hashes skip all bundle reads; user-owned or edited copies are preserved.
func Sync(root string) error {
	var hashes map[string]string
	data, err := bundled.ReadFile("manifest.json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &hashes); err != nil {
		return err
	}
	var failures []error
	for _, name := range Names() {
		if err := syncSkill(root, name, hashes[name]); err != nil {
			failures = append(failures, fmt.Errorf("sync bundled skill %s: %w", name, err))
		}
	}
	return errors.Join(failures...)
}

func syncSkill(root, name, hash string) error {
	var stage string
	defer func() {
		if stage != "" {
			_ = os.RemoveAll(stage)
		}
	}()
	commitRecovery := false
	err := installstate.Reconcile(root, func(records map[string]installstate.Record) (map[string]*installstate.Record, func() error, error) {
		dst := filepath.Join(root, name)
		record, managed := records[name]
		recovered, needsCommit, err := recoverActivation(root, name, record)
		if err != nil {
			return nil, nil, err
		}
		if needsCommit {
			commitRecovery = true
			// Commit recovery separately before attempting another bundle upgrade.
			return map[string]*installstate.Record{name: &recovered}, nil, nil
		}
		info, err := os.Lstat(dst)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, nil, err
		}
		exists := err == nil
		if exists {
			if !info.IsDir() || !managed || record.Source != installstate.SourceBuiltin {
				return nil, nil, nil
			}
			if record.Version == hash {
				return nil, nil, nil
			}
			current, err := installstate.BundleSHA256(dst)
			if err != nil {
				return nil, nil, err
			}
			if record.SHA256 == "" || current != record.SHA256 {
				slog.Warn("preserving modified bundled skill", "skill", name)
				return nil, nil, nil
			}
		}
		stagingRoot := filepath.Join(root, ".builtin-staging")
		if err := os.MkdirAll(stagingRoot, 0755); err != nil {
			return nil, nil, err
		}
		stage, err = os.MkdirTemp(stagingRoot, ".builtin-stage-")
		if err != nil {
			return nil, nil, err
		}
		source, err := fs.Sub(bundled, "embed/"+name)
		if err != nil {
			return nil, nil, err
		}
		err = fs.WalkDir(source, ".", func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			target := filepath.Join(stage, filepath.FromSlash(path))
			if entry.IsDir() {
				return os.MkdirAll(target, 0755)
			}
			content, err := fs.ReadFile(source, path)
			if err != nil {
				return err
			}
			return os.WriteFile(target, content, 0644)
		})
		if err != nil {
			return nil, nil, err
		}
		next := installstate.Record{Source: installstate.SourceBuiltin, Version: hash, SHA256: hash}
		return map[string]*installstate.Record{name: &next}, func() error {
			// Stage and backup share the destination filesystem, so activation is a rename.
			backup := stage + ".previous"
			if err := writeActivation(root, name, stage, hash); err != nil {
				return err
			}
			if exists {
				if err := os.Rename(dst, backup); err != nil {
					return err
				}
			}
			if err := os.Rename(stage, dst); err != nil {
				if exists {
					return errors.Join(err, os.Rename(backup, dst))
				}
				return err
			}
			// Keep the backup and journal until a later sync observes the committed
			// version. This also recovers an origin-write failure after activation.
			stage = ""
			return nil
		}, nil
	})
	if err == nil && commitRecovery {
		return syncSkill(root, name, hash)
	}
	return err
}
