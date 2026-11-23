package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"csgclaw/internal/skill/installstate"
	"csgclaw/internal/skill/system"
)

// ImportDirectory installs a downloaded skill as local, never as a Hub market
// record. Its permissions and origin move together under the installation lock.
func ImportDirectory(root, source string, force bool) (InstallResult, error) {
	source, err := filepath.Abs(source)
	if err != nil {
		return InstallResult{}, err
	}
	name := filepath.Base(source)
	if err := validateSkillSlug(name); err != nil {
		return InstallResult{}, err
	}
	if system.IsName(name) {
		return InstallResult{}, fmt.Errorf("cannot replace system skill %q", name)
	}
	src, err := os.OpenRoot(source)
	if err != nil {
		return InstallResult{}, err
	}
	defer src.Close()
	if err := os.MkdirAll(root, 0755); err != nil {
		return InstallResult{}, err
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return InstallResult{}, err
	}
	realRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return InstallResult{}, err
	}
	realSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return InstallResult{}, err
	}
	if rel, err := filepath.Rel(realSource, realRoot); err == nil && filepath.IsLocal(rel) {
		return InstallResult{}, fmt.Errorf("skills root cannot be inside the import source")
	}
	err = installPrepared(root, name, force, func(stage string) (InstallRecord, error) {
		if err := os.CopyFS(stage, src.FS()); err != nil {
			return InstallRecord{}, err
		}
		err := filepath.WalkDir(stage, func(_ string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && !d.Type().IsRegular() {
				return ErrWorkspacePathUnsafe
			}
			return err
		})
		if err != nil {
			return InstallRecord{}, err
		}
		digest, err := installstate.BundleSHA256(stage)
		return InstallRecord{Source: installstate.SourceLocal, Slug: name, SHA256: digest}, err
	})
	if err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Slug: name, SkillsDir: filepath.Join(root, name)}, nil
}

// Prepare on the destination filesystem before replacing anything. Recheck
// conflicts under the same lock as UI installs and origin record updates.
func installPrepared(root, name string, force bool, prepare func(string) (InstallRecord, error)) error {
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(root, ".skill-install-")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmp)
		}
	}()
	stage := filepath.Join(tmp, "new")
	record, err := prepare(stage)
	if err != nil {
		return err
	}
	if fi, err := os.Lstat(filepath.Join(stage, "SKILL.md")); err != nil || !fi.Mode().IsRegular() {
		return ErrSKILLMDMissing
	}
	dest := filepath.Join(root, name)
	return installstate.Update(root, name, &record, func() error {
		if _, err := os.Lstat(dest); errors.Is(err, os.ErrNotExist) {
			return os.Rename(stage, dest)
		} else if err != nil {
			return err
		}
		if !force {
			return fmt.Errorf("%w: %s", ErrSkillDirExists, dest)
		}
		backup := filepath.Join(tmp, "previous")
		if err := os.Rename(dest, backup); err != nil {
			return err
		}
		if err := os.Rename(stage, dest); err != nil {
			if restoreErr := os.Rename(backup, dest); restoreErr != nil {
				cleanup = false // preserve the only remaining copy
				return errors.Join(err, restoreErr)
			}
			return err
		}
		return nil
	})
}
