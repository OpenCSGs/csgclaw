package installstate

import (
	"errors"
	"testing"
)

func TestReconcileCommitsOnlyAfterActivation(t *testing.T) {
	root := t.TempDir()
	old := Record{Source: SourceBuiltin, Version: "old"}
	if err := Update(root, "pdf", &old, nil); err != nil {
		t.Fatal(err)
	}
	next := Record{Source: SourceBuiltin, Version: "new"}
	interrupted := errors.New("activation interrupted")
	err := Reconcile(root, func(records map[string]Record) (map[string]*Record, func() error, error) {
		return map[string]*Record{"pdf": &next}, func() error {
			// Read directly here: public readers acquire the lock held by Reconcile.
			onDisk, err := readLockFile(root)
			if err != nil {
				return err
			}
			if onDisk.Skills["pdf"].Version != "old" {
				t.Fatal("origin committed before file activation")
			}
			return interrupted
		}, nil
	})
	if !errors.Is(err, interrupted) {
		t.Fatalf("error=%v", err)
	}
	records, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if records["pdf"].Version != "old" {
		t.Fatal("failed activation advanced the version")
	}
}
