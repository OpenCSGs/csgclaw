package migrations

import (
	"csgclaw/internal/localstore"
	"log/slog"
	"path/filepath"
	"time"
)

// BackupConfig protects the application data directory or a standalone config.
// Optional predicates allow individual migrations to ignore errors in generated caches.
func BackupConfig(path string, ignoreErrors ...func(string) bool) error {
	root := filepath.Dir(path)
	if filepath.Base(root) != ".csgclaw" {
		root = path
	}
	_, err := localstore.CreateSiblingBackup(root, time.Now(), ignoreErrors...)
	return err
}

// RewriteConfigBestEffort keeps a readable configuration usable when an
// upgrade cannot back it up or persist its normalized representation.
func RewriteConfigBestEffort(path string, save func() error, ignoreErrors ...func(string) bool) {
	if err := BackupConfig(path, ignoreErrors...); err != nil {
		slog.Warn("config migration backup failed; continuing with loaded configuration", "path", path, "error", err)
		return
	}
	if err := save(); err != nil {
		slog.Warn("config migration rewrite failed; continuing with loaded configuration", "path", path, "error", err)
	}
}
