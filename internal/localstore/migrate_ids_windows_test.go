//go:build windows

package localstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestCreateSiblingBackupWithLockedRuntimeLock(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, ".csgclaw")
	writeFile(t, filepath.Join(root, "config.toml"), "version = 1\n")
	lockPath := filepath.Join(root, runtimeLockFileName)
	writeFile(t, lockPath, "12345\n")

	lockFile, err := os.OpenFile(lockPath, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("OpenFile(runtime lock) error = %v", err)
	}
	var overlapped windows.Overlapped
	if err := windows.LockFileEx(
		windows.Handle(lockFile.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&overlapped,
	); err != nil {
		_ = lockFile.Close()
		t.Fatalf("LockFileEx() error = %v", err)
	}
	t.Cleanup(func() {
		var unlockOverlapped windows.Overlapped
		if err := windows.UnlockFileEx(windows.Handle(lockFile.Fd()), 0, 1, 0, &unlockOverlapped); err != nil {
			t.Errorf("UnlockFileEx() error = %v", err)
		}
		if err := lockFile.Close(); err != nil {
			t.Errorf("Close(runtime lock) error = %v", err)
		}
	})

	backupPath, err := CreateSiblingBackup(root, time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("CreateSiblingBackup() with locked runtime.lock error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(backupPath, "config.toml")); err != nil {
		t.Fatalf("backup missing config.toml: %v", err)
	}
	assertMissing(t, filepath.Join(backupPath, runtimeLockFileName))
}
