package installstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteLockRecordUsesAtomicReplace(t *testing.T) {
	t.Parallel()

	skillsRoot := t.TempDir()
	record := Record{Registry: "opencsg", Slug: "demo-skill", Version: "1.0.0", SHA256: "abc123"}
	if err := Update(skillsRoot, record.Slug, &record, nil); err != nil {
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
	if err := Update(skillsRoot, record.Slug, &record, nil); err != nil {
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
	matches, err := filepath.Glob(filepath.Join(skillsRoot, ".skill-lock-*.tmp"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary lock files left behind: %v", matches)
	}
}

func TestFailedFileOperationRestoresInstallationRecord(t *testing.T) {
	for _, operation := range []string{"install", "replace", "delete"} {
		t.Run(operation, func(t *testing.T) {
			root := t.TempDir()
			previous := Record{Source: SourceMarket, Slug: "demo", RemotePath: "owner/demo"}
			if operation != "install" {
				if err := Update(root, "demo", &previous, nil); err != nil {
					t.Fatal(err)
				}
			}
			before, err := Read(root)
			if err != nil {
				t.Fatal(err)
			}
			next := &Record{Source: SourceLocal}
			if operation == "delete" {
				next = nil
			}
			failure := errors.New("file operation failed")
			if err := Update(root, "demo", next, func() error { return failure }); !errors.Is(err, failure) {
				t.Fatalf("Update() error = %v, want file operation failure", err)
			}
			after, err := Read(root)
			if err != nil || len(after) != len(before) || after["demo"] != before["demo"] {
				t.Fatalf("records after failure = %+v, err = %v; want %+v", after, err, before)
			}
		})
	}
}

func TestConcurrentUpdatesKeepAllInstallationRecords(t *testing.T) {
	root := t.TempDir()
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Go(func() {
			name := fmt.Sprintf("skill-%d", i)
			if err := Update(root, name, &Record{Source: SourceLocal}, nil); err != nil {
				t.Errorf("Update(%q) error = %v", name, err)
			}
		})
	}
	wg.Wait()
	records, err := Read(root)
	if err != nil || len(records) != 12 {
		t.Fatalf("records = %+v, err = %v; want 12 independent installations", records, err)
	}
	previous := records["skill-0"]
	if err := Update(root, "skill-0", &Record{Source: SourceMarket}, nil); err != nil {
		t.Fatal(err)
	}
	records, err = Read(root)
	if err != nil || !records["skill-0"].InstalledAt.Equal(previous.InstalledAt) || records["skill-0"].UpdatedAt.Before(previous.UpdatedAt) {
		t.Fatalf("replacement record = %+v, err = %v; want original installation time", records["skill-0"], err)
	}
}

func TestUnreadableRecordsDoNotApplyFileOperation(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(lockFilePath(root), 0o755); err != nil {
		t.Fatal(err)
	}
	applied := false
	err := Update(root, "demo", &Record{Source: SourceLocal}, func() error {
		applied = true
		return nil
	})
	if err == nil || applied {
		t.Fatalf("Update() error = %v, applied = %v; files must stay untouched", err, applied)
	}
}
