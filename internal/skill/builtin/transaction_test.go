package builtin

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/skill/installstate"
	"csgclaw/internal/skill/local"
)

func TestInterruptedActivationRecovery(t *testing.T) {
	for _, point := range []string{"before_activation", "after_backup", "before_origin_commit", "after_origin_commit"} {
		t.Run(point, func(t *testing.T) {
			root := t.TempDir()
			if err := Sync(root); err != nil {
				t.Fatal(err)
			}
			if err := Sync(root); err != nil {
				t.Fatal(err)
			} // complete initial journals
			records, err := installstate.Read(root)
			if err != nil {
				t.Fatal(err)
			}
			expected := records["pdf"]
			dst := filepath.Join(root, "pdf")
			data, err := os.ReadFile(filepath.Join(dst, "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			staging := filepath.Join(root, ".builtin-staging")
			stage, err := os.MkdirTemp(staging, ".builtin-stage-")
			if err != nil {
				t.Fatal(err)
			}
			// Preserve an exact new bundle as the staged release.
			if err := os.Rename(dst, stage+".new"); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(stage); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(stage+".new", stage); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(dst, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dst, "SKILL.md"), []byte("previous release"), 0644); err != nil {
				t.Fatal(err)
			}
			oldHash, err := installstate.BundleSHA256(dst)
			if err != nil {
				t.Fatal(err)
			}
			old := installstate.Record{Source: installstate.SourceBuiltin, Version: oldHash, SHA256: oldHash}
			if err := installstate.Update(root, "pdf", &old, nil); err != nil {
				t.Fatal(err)
			}
			if err := writeActivation(root, "pdf", stage, expected.SHA256); err != nil {
				t.Fatal(err)
			}
			if point != "before_activation" {
				if err := os.Rename(dst, stage+".previous"); err != nil {
					t.Fatal(err)
				}
			}
			if point == "before_origin_commit" || point == "after_origin_commit" {
				if err := os.Rename(stage, dst); err != nil {
					t.Fatal(err)
				}
			}
			if point == "after_origin_commit" {
				if err := installstate.Reconcile(root, func(map[string]installstate.Record) (map[string]*installstate.Record, func() error, error) {
					return map[string]*installstate.Record{"pdf": &expected}, nil, nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			if err := Sync(root); err != nil {
				t.Fatal(err)
			}
			actual, err := os.ReadFile(filepath.Join(dst, "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if string(actual) != string(data) {
				t.Fatal("did not activate current bundle")
			}
			records, err = installstate.Read(root)
			if err != nil {
				t.Fatal(err)
			}
			if records["pdf"].Version != expected.Version {
				t.Fatal("origin does not match activated bundle")
			}
			if err := Sync(root); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(journalPath(root, "pdf")); !os.IsNotExist(err) {
				t.Fatalf("journal not cleaned: %v", err)
			}
			items, err := local.List(root)
			if err != nil || len(items) != 10 {
				t.Fatalf("staging leaked into catalog: count=%d err=%v", len(items), err)
			}
		})
	}
}

func TestPDFReferencesExist(t *testing.T) {
	for _, name := range []string{"reference.md", "forms.md"} {
		if _, err := fs.Stat(bundled, "embed/pdf/"+name); err != nil {
			t.Fatal(err)
		}
	}
	data, err := fs.ReadFile(bundled, "embed/pdf/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range []string{"REFERENCE.md", "FORMS.md"} {
		if strings.Contains(string(data), old) {
			t.Fatalf("obsolete reference: %s", old)
		}
	}
}

func TestCommittedActivationPreservesLaterUserEdits(t *testing.T) {
	root := t.TempDir()
	if err := Sync(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "pdf", "SKILL.md")
	if err := os.WriteFile(path, []byte("user edit after successful installation"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Sync(root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "user edit after successful installation" {
		t.Fatal("user edit overwritten during journal cleanup")
	}
}

func TestUserReplacementRetainsLocalOrigin(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprint(changed), func(t *testing.T) {
			root := t.TempDir()
			if err := Sync(root); err != nil {
				t.Fatal(err)
			}
			records, err := installstate.Read(root)
			if err != nil {
				t.Fatal(err)
			}
			replacement := records["pdf"]
			replacement.Source = installstate.SourceLocal
			if err := installstate.Update(root, "pdf", &replacement, func() error {
				if changed {
					return os.WriteFile(filepath.Join(root, "pdf", "SKILL.md"), []byte("custom skill"), 0644)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := Sync(root); err != nil {
				t.Fatal(err)
			}
			records, err = installstate.Read(root)
			if err != nil {
				t.Fatal(err)
			}
			if records["pdf"].Source != installstate.SourceLocal {
				t.Fatal("user replacement was adopted as builtin")
			}
			if changed {
				data, err := os.ReadFile(filepath.Join(root, "pdf", "SKILL.md"))
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != "custom skill" {
					t.Fatal("replacement contents changed")
				}
			}
		})
	}
}

func TestUserDeleteDiscardsOldActivationBackup(t *testing.T) {
	root := t.TempDir()
	if err := Sync(root); err != nil {
		t.Fatal(err)
	}
	if err := Sync(root); err != nil {
		t.Fatal(err)
	}
	records, err := installstate.Read(root)
	if err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(root, ".builtin-staging", ".builtin-stage-delete-test")
	if err := os.Mkdir(stage+".previous", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage+".previous", "SKILL.md"), []byte("obsolete release"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeActivation(root, "pdf", stage, records["pdf"].SHA256); err != nil {
		t.Fatal(err)
	}
	if err := local.Delete(root, "pdf"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stage + ".previous"); !os.IsNotExist(err) {
		t.Fatalf("obsolete backup remains: %v", err)
	}
	if err := Sync(root); err != nil {
		t.Fatal(err)
	}
	records, err = installstate.Read(root)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := installstate.BundleSHA256(filepath.Join(root, "pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if records["pdf"].Source != installstate.SourceBuiltin || actual != records["pdf"].SHA256 {
		t.Fatal("deleted default was restored from stale backup")
	}
}

func TestSyncIsolatesFailuresAndRetries(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"deepseek-search", "docx"} {
		if err := os.WriteFile(journalPath(root, name), []byte("{"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	err := Sync(root)
	if err == nil || !strings.Contains(err.Error(), "deepseek-search") || !strings.Contains(err.Error(), "docx") {
		t.Fatalf("missing aggregated errors: %v", err)
	}
	for _, name := range Names() {
		if name == "deepseek-search" || name == "docx" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); err != nil {
			t.Fatalf("unrelated skill %s failed: %v", name, err)
		}
	}
	for _, name := range []string{"deepseek-search", "docx"} {
		if err := os.Remove(journalPath(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := Sync(root); err != nil {
		t.Fatal(err)
	}
	items, err := local.List(root)
	if err != nil || len(items) != 10 {
		t.Fatalf("retry incomplete: count=%d err=%v", len(items), err)
	}
}
