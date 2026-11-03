package retiredclaws

import (
	"csgclaw/internal/localstore"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// GeneratedBackupEntry excludes only unreadable old container-generated plugin
// copies. Backup errors for user workspaces remain fatal.
func GeneratedBackupEntry(rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	return len(parts) >= 4 && parts[0] == "agents" && parts[2] == ".openclaw" && parts[3] == "plugin-skills"
}

var snapshotMu sync.Mutex

const snapshotReceipt = ".retired-claws-v1-complete"

// backupOnce protects the original state before retirement starts. The receipt
// is written inside a completed backup, never in the live data. All retirement
// steps and later retries reuse that snapshot, except forced sandbox conversions.
func backupOnce(root string) error {
	snapshotMu.Lock()
	defer snapshotMu.Unlock()
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Dir(root))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), filepath.Base(root)+"_backup_") {
			continue
		}
		receipt, err := os.ReadFile(filepath.Join(filepath.Dir(root), entry.Name(), snapshotReceipt))
		if err == nil && string(receipt) == root {
			return nil
		}
	}
	_, err = freshBackup(root)
	return err
}

// A forced migration always snapshots the current records, even if an older
// completed retirement backup exists. The caller holds snapshotMu.
func freshBackup(root string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	backup, err := localstore.CreateSiblingBackup(root, time.Now(), GeneratedBackupEntry)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(backup, snapshotReceipt), []byte(root), 0600); err != nil {
		return "", errors.Join(err, os.RemoveAll(backup))
	}
	return backup, nil
}
