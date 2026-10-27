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
