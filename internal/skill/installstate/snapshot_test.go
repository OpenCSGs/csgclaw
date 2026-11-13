package installstate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestConfirmationProtectsRecordAndFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "writer")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(file, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	record := Record{Source: SourceMarket, Version: "1.0.0"}
	if err := Update(root, "writer", &record, nil); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"files", "record"} {
		t.Run(kind, func(t *testing.T) {
			_, token, err := Snapshot(root, "writer")
			if err != nil {
				t.Fatal(err)
			}
			if kind == "files" {
				err = os.WriteFile(file, []byte("local edit"), 0600)
			} else {
				record.SourceURL = "another source"
				err = Update(root, "writer", &record, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			called := false
			err = UpdateChecked(root, "writer", &record, token, func() error { called = true; return nil })
			if !errors.Is(err, ErrInstallationChanged) || called {
				t.Fatalf("stale %s confirmation: %v applied=%v", kind, err, called)
			}
		})
	}
}

func TestConfirmationRejectsSymlinkFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "writer")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "SKILL.md")); err != nil {
		t.Skip(err)
	}
	if _, _, err := Snapshot(root, "writer"); err == nil {
		t.Fatal("symbolic link was accepted")
	}
}

func TestConfirmationRejectsParentPath(t *testing.T) {
	if _, _, err := Snapshot(t.TempDir(), ".."); err == nil {
		t.Fatal("parent path was accepted")
	}
}
