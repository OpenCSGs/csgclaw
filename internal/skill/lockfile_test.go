package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteLockRecordUsesAtomicReplace(t *testing.T) {
	t.Parallel()

	skillsRoot := t.TempDir()
	record := newInstallRecord(RegistryOpenCSG, "demo-skill", "1.0.0", "abc123")
	if err := writeLockRecord(skillsRoot, record); err != nil {
		t.Fatalf("writeLockRecord() error = %v", err)
	}

	path := lockFilePath(skillsRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var payload lockFile
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := payload.Skills["demo-skill"].Version; got != "1.0.0" {
		t.Fatalf("Version = %q, want 1.0.0", got)
	}

	record.Version = "2.0.0"
	if err := writeLockRecord(skillsRoot, record); err != nil {
		t.Fatalf("writeLockRecord() second error = %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() second error = %v", err)
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal() second error = %v", err)
	}
	if got := payload.Skills["demo-skill"].Version; got != "2.0.0" {
		t.Fatalf("Version = %q, want 2.0.0", got)
	}
	matches, err := filepath.Glob(filepath.Join(skillsRoot, ".skillhub-lock-*.tmp"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary lock files left behind: %v", matches)
	}
}
