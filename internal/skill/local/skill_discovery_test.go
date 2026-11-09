package local

import (
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/skill/discovery"
)

func TestDiscoveryAndLocalCopiesResolveSameWinner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, key := range []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR", "PI_CODING_AGENT_DIR", "XDG_CONFIG_HOME"} {
		t.Setenv(key, "")
	}
	root, _ := SkillsRoot()
	external := filepath.Join(home, ".claude", "skills", "pdf")
	local := filepath.Join(root, "pdf")
	write := func(dir, version string) {
		t.Helper()
		mustWriteFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: pdf\ndescription: "+version+"\nversion: \""+version+"\"\n---\n")
	}
	write(external, "2.0.0")
	external, _ = filepath.EvalSymlinks(external)
	write(local, "1.0.0")
	items, err := List(root)
	if err != nil || len(items) != 1 || items[0].Discovery == nil {
		t.Fatalf("list=%+v %v", items, err)
	}
	if dir, err := ResolveDir(root, "pdf"); err != nil || dir != external {
		t.Fatalf("resolved %s %v", dir, err)
	}
	write(local, "3.0.0")
	items, err = List(root)
	if err != nil || len(items) != 1 || items[0].Discovery != nil {
		t.Fatalf("override=%+v %v", items, err)
	}
	if dir, err := ResolveDir(root, "pdf"); err != nil || dir != local {
		t.Fatalf("override resolved %s %v", dir, err)
	}
	// Ordinary local deletion must only remove the local installation, even when
	// an external definition of the same name exists in the catalog.
	if err := Delete(root, "pdf"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(external, "SKILL.md")); err != nil {
		t.Fatal("external skill deleted")
	}
	if _, err := discovery.Sync(root); err != nil {
		t.Fatal(err)
	}
	if dir, err := ResolveDir(root, "pdf"); err != nil || dir != external {
		t.Fatalf("external fallback %s %v", dir, err)
	}
	if err := Delete(root, "pdf"); err != nil {
		t.Fatal(err)
	}
	if items, err := List(root); err != nil || len(items) != 0 {
		t.Fatalf("unregister=%+v %v", items, err)
	}
	if _, err := os.Stat(filepath.Join(external, "SKILL.md")); err != nil {
		t.Fatal("unregister changed external source")
	}
}
