package skill

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/skill/installstate"
)

func TestReinstallRetainsPreviousBundleUntilReady(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "design")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(source, "SKILL.md")
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(manifest, []byte(value), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("original")
	result, err := ImportDirectory(root, source, false)
	if err != nil {
		t.Fatal(err)
	}
	before, err := installstate.Read(root)
	if err != nil {
		t.Fatal(err)
	}
	write("replacement")
	if _, err := ImportDirectory(root, source, false); !errors.Is(err, ErrSkillDirExists) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportDirectory(root, source, true); !errors.Is(err, ErrSKILLMDMissing) {
		t.Fatalf("invalid reinstall: %v", err)
	}
	after, err := installstate.Read(root)
	if err != nil || before["design"] != after["design"] {
		t.Fatalf("failed reinstall changed origin: %+v, %v", after, err)
	}
	if data, err := os.ReadFile(filepath.Join(result.SkillsDir, "SKILL.md")); err != nil || string(data) != "original" {
		t.Fatalf("lost original installation: %q, %v", data, err)
	}
	write("replacement")
	if _, err := ImportDirectory(root, source, true); err != nil {
		t.Fatal(err)
	}
	after, err = installstate.Read(root)
	if err != nil || after["design"].SHA256 == before["design"].SHA256 || after["design"].SourceKind() != installstate.SourceLocal {
		t.Fatalf("replacement origin: %+v, %v", after, err)
	}
}
