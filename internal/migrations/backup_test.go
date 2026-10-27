package migrations

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCustomConfigMigrationBacksUpOnlyTheConfigFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "custom.toml")
	original := "[server]\nlisten_addr = \"127.0.0.1:19081\"\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := BackupConfig(path); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(path + "_backup_*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("custom config backups: %v %v", backups, err)
	}
	info, err := os.Stat(backups[0])
	if err != nil || !info.Mode().IsRegular() {
		t.Fatal("custom backup is not a file")
	}
	data, err := os.ReadFile(backups[0])
	if err != nil || string(data) != original {
		t.Fatal("custom config backup lost original")
	}
}

func TestConfigBackupReusedAfterFailedRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("old config"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		RewriteConfigBestEffort(path, func() error { return os.ErrPermission })
	}
	backups, _ := filepath.Glob(path + "_backup_*")
	if len(backups) != 1 {
		t.Fatalf("retry created duplicate backups: %v", backups)
	}
	if err := os.WriteFile(path, []byte("different old config"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := BackupConfig(path); err != nil {
		t.Fatal(err)
	}
	backups, _ = filepath.Glob(path + "_backup_*")
	if len(backups) != 2 {
		t.Fatalf("changed input was not protected: %v", backups)
	}
}

func TestConfigRewriteIgnoresUnrelatedUnreadableFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".csgclaw")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte("old config"), 0600); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(root, "user-file")
	if err := os.WriteFile(blocked, []byte("keep"), 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(blocked, 0600) })
	RewriteConfigBestEffort(path, func() error { return os.WriteFile(path, []byte("clean config"), 0600) })
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "clean config" {
		t.Fatalf("config rewrite blocked: %s, %v", data, err)
	}
	dirs, _ := filepath.Glob(root + "_backup_*")
	if len(dirs) != 0 {
		t.Fatalf("config cleanup copied unrelated data: %v", dirs)
	}
}
