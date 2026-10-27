package migrations

import (
	"bytes"
	"csgclaw/internal/localstore"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var configBackupMu sync.Mutex

// BackupConfig protects the configuration being rewritten. Unrelated application
// files must not prevent config cleanup. Reuse a complete copy of identical input
// when a previous rewrite failed, including across process restarts.
func BackupConfig(path string, ignoreErrors ...func(string) bool) error {
	configBackupMu.Lock()
	defer configBackupMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), filepath.Base(path)+"_backup_") {
			continue
		}
		original, err := os.ReadFile(filepath.Join(filepath.Dir(path), entry.Name()))
		if err == nil && bytes.Equal(original, data) {
			return nil
		}
	}
	_, err = localstore.CreateSiblingBackup(path, time.Now(), ignoreErrors...)
	return err
}

// RewriteConfigBestEffort keeps a readable configuration usable when an
// upgrade cannot back it up or persist its normalized representation.
func RewriteConfigBestEffort(path string, save func() error, ignoreErrors ...func(string) bool) {
	if err := BackupConfig(path, ignoreErrors...); err != nil {
		slog.Warn("config migration backup failed; continuing with loaded configuration", "path", path, "error", fmt.Errorf("config unchanged; fix backup access and restart to retry: %w", err))
		return
	}
	if err := save(); err != nil {
		slog.Warn("config migration rewrite failed; continuing with loaded configuration", "path", path, "error", err)
	}
}
