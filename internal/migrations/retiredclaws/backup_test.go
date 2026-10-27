package retiredclaws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetirementBackupReusedAcrossStepsAndRetries(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".csgclaw")
	path := filepath.Join(root, "state.json")
	writeFile(t, path, "original state")
	if err := backupOnce(root); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "state after another migration step")
	if err := backupOnce(root); err != nil {
		t.Fatal(err)
	}
	backups, _ := filepath.Glob(root + "_backup_*")
	if len(backups) != 1 {
		t.Fatalf("duplicate snapshots: %v", backups)
	}
	data, err := os.ReadFile(filepath.Join(backups[0], "state.json"))
	if err != nil || string(data) != "original state" {
		t.Fatalf("original snapshot lost: %s, %v", data, err)
	}
}

func TestRetirementBackupDoesNotReuseUnmarkedCopy(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".csgclaw")
	writeFile(t, filepath.Join(root, "state.json"), "original")
	old := root + "_backup_20261001_001"
	writeFile(t, filepath.Join(old, "state.json"), "incomplete old copy")
	if err := backupOnce(root); err != nil {
		t.Fatal(err)
	}
	backups, _ := filepath.Glob(root + "_backup_*")
	if len(backups) != 2 {
		t.Fatalf("unmarked copy reused: %v", backups)
	}
	data, _ := os.ReadFile(filepath.Join(old, "state.json"))
	if string(data) != "incomplete old copy" {
		t.Fatal("preexisting backup modified")
	}
}

func TestFailedAgentRewriteReusesCompletedBackupOnRetry(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".csgclaw")
	path := filepath.Join(root, "state.json")
	original := `{"version":1,"agents":{"items":[{"id":"agent-old","runtime":{"kind":"openclaw_sandbox","state":"stopped"}}]}}`
	writeFile(t, path, original)
	if err := os.Chmod(root, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(root, 0755) })
	if err := os.WriteFile(filepath.Join(root, "probe"), nil, 0600); err == nil {
		t.Skip("filesystem does not enforce directory permissions")
	}
	for i := 0; i < 2; i++ {
		if err := migrateAgentsWithMissingSandbox(t, root); err == nil {
			t.Fatal("read-only state unexpectedly rewritten")
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != original {
			t.Fatalf("failed migration changed record: %s, %v", data, err)
		}
		backups, _ := filepath.Glob(root + "_backup_*")
		if len(backups) != 1 {
			t.Fatalf("failed migration duplicated backup: %v", backups)
		}
	}
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := migrateAgentsWithMissingSandbox(t, root); err != nil {
		t.Fatal(err)
	}
	backups, _ := filepath.Glob(root + "_backup_*")
	if len(backups) != 1 {
		t.Fatalf("successful retry duplicated backup: %v", backups)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"codex"`) {
		t.Fatalf("retry did not migrate: %s", data)
	}
}
