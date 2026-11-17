//go:build windows

package localstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestCreateSiblingBackupPreservesRelativeDirectorySymlink(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".csgclaw")
	target := "z-dependencies"
	link := "a-dependencies"
	writeFile(t, filepath.Join(root, target, "package.json"), "{}")
	if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
		if errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) {
			t.Skipf("Windows account cannot create symlinks: %v", err)
		}
		t.Fatalf("create source directory symlink: %v", err)
	}

	backup, err := CreateSiblingBackup(root, time.Now())
	if err != nil {
		t.Fatalf("CreateSiblingBackup() error = %v", err)
	}
	if got, err := os.Readlink(filepath.Join(backup, link)); err != nil || got != target {
		t.Fatalf("backup link target = %q, error = %v, want %q", got, err, target)
	}
	data, err := os.ReadFile(filepath.Join(backup, link, "package.json"))
	if err != nil || string(data) != "{}" {
		t.Fatalf("read through backup directory symlink = %q, error = %v", data, err)
	}
}

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

func TestCreateSiblingBackupWithUnreadableDSHSharedModule(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".csgclaw")
	writeFile(t, filepath.Join(root, "config.toml"), "version = 1\n")
	modules := filepath.Join("agents", "agent-bdcpxh", ".dsh", "home", "profiles", "node_modules")
	path := filepath.Join(root, modules, "@agentclientprotocol", "sdk")
	writeFile(t, path, "generated dependency")
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	// Deny shared reads so a backup that traverses the mirror fails on Windows.
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { windows.CloseHandle(handle) })
	if _, err := os.ReadFile(path); err == nil {
		t.Fatal("fixture dependency is readable")
	}
	backup, err := CreateSiblingBackup(root, time.Now())
	if err != nil {
		t.Fatalf("generated dependency blocked backup: %v", err)
	}
	assertMissing(t, filepath.Join(backup, modules))
	if _, err := os.Stat(filepath.Join(backup, "config.toml")); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateTypedIDsWithLockedDSHProfileDependency(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".csgclaw")
	profiles := filepath.Join("agents", "agent-worker", ".dsh", "home", "profiles")
	locked := filepath.Join(profiles, "node_modules", "@agentclientprotocol", "sdk", "package.json")
	writeJSON(t, filepath.Join(root, "agents", "state.json"), map[string]any{
		"agents": []map[string]any{{"id": "agent-worker", "name": "Worker"}},
	})
	writeFile(t, filepath.Join(root, locked), "{}")

	lockedPath, err := windows.UTF16PtrFromString(filepath.Join(root, locked))
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(lockedPath, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("open dependency without sharing: %v", err)
	}
	defer windows.CloseHandle(handle)
	if _, err := os.ReadFile(filepath.Join(root, locked)); err == nil {
		t.Fatal("locked dependency is still readable")
	}

	result, err := MigrateTypedIDs(MigrateOptions{Root: root})
	if err != nil {
		t.Fatalf("MigrateTypedIDs() error = %v", err)
	}
	assertMissing(t, filepath.Join(result.BackupPath, locked))
	assertMissing(t, filepath.Join(root, "agents", "state.json"))
}
