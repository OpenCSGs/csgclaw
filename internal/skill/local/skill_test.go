package local

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/skill/installstate"
)

func TestListReturnsSkillDirectoriesWithDescriptions(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "alpha", "SKILL.md"), "---\ndescription: First skill\n---\n# Alpha\n")
	mustWriteFile(t, filepath.Join(root, "beta", "SKILL.md"), "# Beta\n")
	mustWriteFile(t, filepath.Join(root, "notes.md"), "ignore")
	if err := os.MkdirAll(filepath.Join(root, "gamma"), 0o755); err != nil {
		t.Fatalf("MkdirAll(gamma) error = %v", err)
	}

	got, err := List(root)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(List()) = %d, want 2", len(got))
	}
	if got[0].Name != "alpha" || got[0].Description != "First skill" {
		t.Fatalf("first skill = %+v, want alpha with description", got[0])
	}
	if got[1].Name != "beta" || got[1].Description != "" {
		t.Fatalf("second skill = %+v, want beta without description", got[1])
	}
}

func TestListParsesBlockScalarDescription(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "alpha", "SKILL.md"), "---\ndescription: >\n  line one\n  line two\n---\n# Alpha\n")

	got, err := List(root)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(List()) = %d, want 1", len(got))
	}
	if got[0].Description != "line one line two" {
		t.Fatalf("description = %q, want %q", got[0].Description, "line one line two")
	}
}

func TestDeleteRemovesSkillDirectory(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "alpha", "SKILL.md"), "# Alpha\n")
	mustWriteFile(t, filepath.Join(root, "alpha", "scripts", "run.sh"), "#!/bin/sh\necho hi\n")

	if err := Delete(root, "alpha"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "alpha")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("alpha still exists, err = %v", err)
	}
}

func TestDeleteRejectsUnsafeName(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "alpha", "SKILL.md"), "# Alpha\n")

	if err := Delete(root, "../alpha"); err == nil {
		t.Fatalf("Delete() error = nil, want invalid skill name")
	}
	if _, err := os.Stat(filepath.Join(root, "alpha", "SKILL.md")); err != nil {
		t.Fatalf("alpha skill unexpectedly changed: %v", err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func TestInstallArchive(t *testing.T) {
	root := t.TempDir()

	got, err := InstallArchive(root, "alpha.zip", mustZip(t, map[string]string{
		"alpha/SKILL.md":       "---\ndescription: First skill\n---\n# Alpha\n",
		"alpha/scripts/run.sh": "#!/bin/sh\necho hi\n",
	}))
	if err != nil {
		t.Fatalf("InstallArchive() error = %v", err)
	}
	if got.Name != "alpha" || got.Description != "First skill" {
		t.Fatalf("InstallArchive() = %+v, want alpha summary", got)
	}
	if _, err := os.Stat(filepath.Join(root, "alpha", "SKILL.md")); err != nil {
		t.Fatalf("installed SKILL.md missing: %v", err)
	}
}

func TestInstallOriginSurvivesListingAndReplacement(t *testing.T) {
	root := t.TempDir()
	archive := mustZip(t, map[string]string{
		"alpha/SKILL.md": "# Alpha\n",
	})
	for _, source := range []string{installstate.SourceMarket, installstate.SourceLocal, installstate.SourceMarket} {
		origin := installstate.Record{Source: source}
		if source == installstate.SourceMarket {
			origin.RegistryURL = "https://hub.example.test"
			origin.RemotePath = "owner/alpha"
			origin.Ref = "main"
		}
		item, err := InstallArchiveWithOptions(root, "alpha.zip", archive, InstallArchiveOptions{Replace: true, Origin: origin})
		if err != nil {
			t.Fatal(err)
		}
		if item.Source != source {
			t.Fatalf("installation source = %q, want %q", item.Source, source)
		}
		items, err := List(root)
		if err != nil || len(items) != 1 || items[0].Source != source {
			t.Fatalf("listed origin = %+v, error = %v; want %q", items, err, source)
		}
		records, err := installstate.Read(root)
		if err != nil {
			t.Fatal(err)
		}
		record := records["alpha"]
		if record.RemotePath != origin.RemotePath || record.RegistryURL != origin.RegistryURL || record.Ref != origin.Ref || record.SHA256 == "" {
			t.Fatalf("installation record = %+v, want current source identity and hash", record)
		}
	}
	files, err := os.ReadDir(filepath.Join(root, "alpha"))
	if err != nil || len(files) != 1 || files[0].Name() != "SKILL.md" {
		t.Fatalf("skill files = %v, err = %v; installer should not add files to the skill", files, err)
	}
	before, err := installstate.Read(root)
	if err != nil {
		t.Fatal(err)
	}
	// The same skill packaged without its enclosing directory has the same hash.
	_, err = InstallArchiveWithOptions(root, "alpha.zip", mustZip(t, map[string]string{
		"SKILL.md": "# Alpha\n",
	}), InstallArchiveOptions{Replace: true, Origin: before["alpha"]})
	if err != nil {
		t.Fatal(err)
	}
	after, err := installstate.Read(root)
	if err != nil || after["alpha"].SHA256 != before["alpha"].SHA256 {
		t.Fatalf("content hash changed with archive layout: %+v, err = %v", after, err)
	}
	if err := Delete(root, "alpha"); err != nil {
		t.Fatal(err)
	}
	records, err := installstate.Read(root)
	if err != nil || len(records) != 0 {
		t.Fatalf("records after deletion = %+v, err = %v", records, err)
	}
}

func TestListRecognizesLegacyCLIRecordsAndUntrackedSkills(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "legacy", "SKILL.md"), "# Legacy\n")
	mustWriteFile(t, filepath.Join(root, "untracked", "SKILL.md"), "# Untracked\n")
	mustWriteFile(t, filepath.Join(root, "untracked", ".csgclaw-origin.json"), `{"source":"market"}`)
	mustWriteFile(t, filepath.Join(root, ".skillhub-lock.json"), `{"skills":{"legacy":{"registry":"clawhub","slug":"legacy","version":"1.0.0"}}}`)
	items, err := List(root)
	if err != nil || len(items) != 2 || items[0].Source != installstate.SourceMarket || items[1].Source != installstate.SourceUnknown {
		t.Fatalf("skills = %+v, err = %v; want tracked market and unknown sources", items, err)
	}
}

func TestInstallDoesNotChangeFilesWhenRecordsCannotBeRead(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "alpha", "SKILL.md"), "# Original\n")
	mustWriteFile(t, filepath.Join(root, ".skillhub-lock.json"), "invalid json")
	_, err := InstallArchiveWithOptions(root, "alpha.zip", mustZip(t, map[string]string{
		"alpha/SKILL.md": "# Replacement\n",
	}), InstallArchiveOptions{Replace: true})
	if err == nil {
		t.Fatal("expected invalid installation records to prevent replacement")
	}
	content, err := os.ReadFile(filepath.Join(root, "alpha", "SKILL.md"))
	if err != nil || string(content) != "# Original\n" {
		t.Fatalf("existing skill changed: %q, err = %v", content, err)
	}
}

func TestInstallArchiveRejectsDuplicate(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "alpha", "SKILL.md"), "# Alpha\n")

	_, err := InstallArchive(root, "alpha.zip", mustZip(t, map[string]string{
		"alpha/SKILL.md": "# Alpha\n",
	}))
	if !errors.Is(err, ErrSkillAlreadyExists) {
		t.Fatalf("InstallArchive() error = %v, want ErrSkillAlreadyExists", err)
	}
}

func TestInstallArchiveWithOptionsReplacesDuplicate(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "alpha", "SKILL.md"), "# Alpha\n")
	mustWriteFile(t, filepath.Join(root, "alpha", "old.txt"), "old")

	got, err := InstallArchiveWithOptions(root, "alpha.zip", mustZip(t, map[string]string{
		"alpha/SKILL.md": "---\ndescription: Replacement skill\n---\n# Alpha v2\n",
		"alpha/new.txt":  "new",
	}), InstallArchiveOptions{Replace: true})
	if err != nil {
		t.Fatalf("InstallArchiveWithOptions() error = %v", err)
	}
	if got.Name != "alpha" || got.Description != "Replacement skill" {
		t.Fatalf("InstallArchiveWithOptions() = %+v, want replacement summary", got)
	}
	if _, err := os.Stat(filepath.Join(root, "alpha", "old.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old file still exists, err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "alpha", "new.txt")); err != nil {
		t.Fatalf("replacement file missing: %v", err)
	}
}

func TestInstallArchiveRejectsSystemSkillName(t *testing.T) {
	root := t.TempDir()

	_, err := InstallArchive(root, "skill-installer.zip", mustZip(t, map[string]string{
		"skill-installer/SKILL.md": "# Skill Installer\n",
	}))
	if !errors.Is(err, ErrSkillAlreadyExists) {
		t.Fatalf("InstallArchive() error = %v, want ErrSkillAlreadyExists", err)
	}
}

func TestInstallArchiveRejectsInvalidShape(t *testing.T) {
	root := t.TempDir()

	_, err := InstallArchive(root, "invalid.zip", mustZip(t, map[string]string{
		"alpha/SKILL.md": "# Alpha\n",
		"beta/SKILL.md":  "# Beta\n",
	}))
	if !errors.Is(err, ErrSkillArchiveInvalid) {
		t.Fatalf("InstallArchive() error = %v, want ErrSkillArchiveInvalid", err)
	}
}

func TestInstallArchiveRejectsZipSlip(t *testing.T) {
	root := t.TempDir()

	_, err := InstallArchive(root, "escape.zip", mustZip(t, map[string]string{
		"../escape/SKILL.md": "# Escape\n",
	}))
	if !errors.Is(err, ErrSkillArchiveUnsafe) {
		t.Fatalf("InstallArchive() error = %v, want ErrSkillArchiveUnsafe", err)
	}
}

func mustZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return buf.Bytes()
}
