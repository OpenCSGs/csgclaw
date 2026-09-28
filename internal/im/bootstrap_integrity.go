package im

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const bootstrapRecoveryGuidance = "local IM state was not reset; do not delete state.json or reinitialize. Preserve the entire data directory before recovery, then restore a known-good backup or contact support"

// A missing index is only a fresh installation when there is no conversation
// data to lose. Bootstrapping otherwise lets stale-file cleanup erase that data.
func checkMissingBootstrap(path string) error {
	for _, name := range []string{sessionsDirName, threadsDirName, assetsDirName} {
		dir := filepath.Join(filepath.Dir(path), name)
		err := filepath.WalkDir(dir, func(entryPath string, entry fs.DirEntry, err error) error {
			if errors.Is(err, os.ErrNotExist) && entryPath == dir {
				return nil
			}
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				return fmt.Errorf("IM state %q is missing but conversation data remains at %q; %s", path, entryPath, bootstrapRecoveryGuidance)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("check missing im bootstrap: %w", err)
		}
	}
	return nil
}
