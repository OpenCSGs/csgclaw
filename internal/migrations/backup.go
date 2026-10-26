package migrations

import (
	"csgclaw/internal/localstore"
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
