package agents

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/skill/installstate"
	hub "csgclaw/internal/template"
)

func TestTemplateFileUpdateRollsBackPartialWrite(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "first")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := newTemplateFileUpdate(home)
	if err != nil {
		t.Fatal(err)
	}
	defer files.close()
	for _, target := range []string{path, filepath.Join(home, "new"), filepath.Join(home, "last")} {
		if err := files.stageFile(target, []byte("updated"), 0600, false); err != nil {
			t.Fatal(err)
		}
	}
	// A destination that became a directory cannot be replaced with a file.
	if err := os.Mkdir(filepath.Join(home, "last"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := files.apply(); err == nil {
		t.Fatal("expected commit failure")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "original" {
		t.Fatalf("original: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(home, "new")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new file survived rollback: %v", err)
	}
}

func TestTemplateFileUpdateRejectsLinkedDirectory(t *testing.T) {
	home, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(home, "link")); err != nil {
		t.Skip(err)
	}
	files, err := newTemplateFileUpdate(home)
	if err != nil {
		t.Fatal(err)
	}
	defer files.close()
	if err := files.stageFile(filepath.Join(home, "link", "file"), []byte("updated"), 0600, false); err == nil {
		t.Fatal("accepted a linked directory")
	}
}

func TestTemplateSkillUpdateWaitsForInstallation(t *testing.T) {
	home, src := t.TempDir(), t.TempDir()
	layout := agentruntime.Layout{WorkspaceRoot: filepath.Join(home, "workspace"), SkillsRoot: filepath.Join(home, "runtime", "skills")}
	source := filepath.Join(src, "skills", "review", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(source), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("template"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := newTemplateFileUpdate(home)
	if err != nil {
		t.Fatal(err)
	}
	defer files.close()
	ctx := t.Context()
	if _, err := files.prepare(ctx, hub.WorkspaceRef{Path: src}, layout, false); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	target := filepath.Join(layout.SkillsRoot, "review", "SKILL.md")
	err = installstate.Update(layout.SkillsRoot, "review", &installstate.Record{Slug: "review"}, func() error {
		started := make(chan struct{})
		go func() { close(started); _, err := files.applySkills(ctx, layout); done <- err }()
		<-started
		select {
		case <-done:
			return errors.New("template files were published during installation")
		case <-time.After(30 * time.Millisecond):
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, []byte("installed"), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{target, filepath.Join(layout.WorkspaceRoot, "skills", "review", "SKILL.md")} {
		if data, err := os.ReadFile(name); err != nil || string(data) != "template" {
			t.Fatalf("%s = %q, %v", name, data, err)
		}
	}
}
