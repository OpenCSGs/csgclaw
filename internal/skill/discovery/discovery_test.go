package discovery

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func definition(t *testing.T, dir, name, version, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nname: "+name+"\ndescription: test skill\nmetadata:\n  version: \""+version+"\"\n---\n"+body), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScanDeduplicatesVersionsAndTracksSources(t *testing.T) {
	home := t.TempDir()
	old := definition(t, filepath.Join(home, "claude", "pdf"), "pdf", "1.9.0", "old")
	newest := definition(t, filepath.Join(home, "codex", "pdf"), "pdf", "1.10.0", "new")
	now := time.Now()
	os.Chtimes(old, now.Add(time.Hour), now.Add(time.Hour))
	roots := []Source{{Tool: "Claude Code", Path: filepath.Dir(filepath.Dir(old))}, {Tool: "Codex", Path: filepath.Dir(filepath.Dir(newest))}}
	items := Scan(roots)
	if len(items) != 1 || items[0].Version != "1.10.0" || len(items[0].Sources) != 2 {
		t.Fatalf("items=%+v", items)
	}
	definition(t, filepath.Dir(newest), "pdf", "1.10.0", "updated")
	after := Scan(roots)
	if after[0].Key == items[0].Key {
		t.Fatal("content update not detected")
	}
	if err := os.RemoveAll(filepath.Dir(newest)); err != nil {
		t.Fatal(err)
	}
	if after := Scan(roots); len(after) != 1 || after[0].Tool != "Claude Code" {
		t.Fatalf("fallback=%+v", after)
	}
}

func TestScanBoundaries(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	definition(t, filepath.Join(root, "good"), "good", "", "valid")
	definition(t, filepath.Join(root, "good", "assets", "nested"), "nested", "", "must not be scanned")
	definition(t, filepath.Join(outside, "private"), "private", "", "must not be discovered through arbitrary link")
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skip(err)
	}
	bad := filepath.Join(root, "bad")
	os.MkdirAll(bad, 0755)
	os.Symlink(filepath.Join(outside, "private", "SKILL.md"), filepath.Join(bad, "SKILL.md"))
	// A linked installed skill is supported, but never its unrelated siblings.
	os.Symlink(filepath.Join(root, "good"), filepath.Join(root, "linked"))
	os.MkdirAll(filepath.Join(root, "oversize"), 0755)
	os.WriteFile(filepath.Join(root, "oversize", "SKILL.md"), []byte(strings.Repeat("x", maxDefinitionSize+1)), 0644)
	os.MkdirAll(filepath.Join(root, "invalid"), 0755)
	os.WriteFile(filepath.Join(root, "invalid", "SKILL.md"), []byte("---\nname: [\n---\n"), 0644)
	items := Scan([]Source{{Tool: "test", Path: root}})
	if len(items) != 1 || items[0].Name != "good" {
		t.Fatalf("unexpected discovery: %+v", items)
	}
	os.Symlink(filepath.Join(outside, "private", "SKILL.md"), filepath.Join(root, "good", "secret"))
	bounded := BoundedFS(filepath.Join(root, "good"))
	if _, err := fs.ReadFile(bounded, "secret"); err == nil {
		t.Fatal("followed outside link")
	}
	if _, err := fs.ReadFile(bounded, "../private/SKILL.md"); err == nil {
		t.Fatal("escaped skill root")
	}
}

func TestRegistryIncrementalAcknowledgementAndRemoval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	root := filepath.Join(home, ".csgclaw", "skills")
	first := definition(t, filepath.Join(home, ".claude", "skills", "one"), "one", "1.0.0", "one")
	items, err := Sync(root)
	if err != nil || len(items) != 1 || !items[0].New {
		t.Fatalf("initial=%+v %v", items, err)
	}
	key := items[0].Key
	definition(t, filepath.Join(home, ".pi", "agent", "skills", "two"), "two", "1.0.0", "two")
	if _, err := Sync(root); err != nil {
		t.Fatal(err)
	}
	if err := Acknowledge(root, []string{key}); err != nil {
		t.Fatal(err)
	}
	items, err = Sync(root)
	if err != nil || items[0].New || !items[1].New {
		t.Fatalf("ack raced new skill: %+v %v", items, err)
	}
	// Source changes are visible without reimporting a copy.
	definition(t, filepath.Dir(first), "one", "1.1.0", "changed")
	items, err = Sync(root)
	if err != nil || !items[0].New || items[0].Version != "1.1.0" {
		t.Fatalf("updated=%+v %v", items, err)
	}
	if err := Ignore(root, "one"); err != nil {
		t.Fatal(err)
	}
	items, err = Sync(root)
	if err != nil || len(items) != 1 || items[0].Name != "two" {
		t.Fatalf("ignored=%+v %v", items, err)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal("unregister deleted external file")
	}
	os.RemoveAll(filepath.Join(home, ".pi"))
	items, err = Sync(root)
	if err != nil || len(items) != 0 {
		t.Fatalf("stale registration=%+v %v", items, err)
	}
}

func TestCompareSemverAndTimestamp(t *testing.T) {
	for _, pair := range [][2]string{{"1.10.0", "1.9.0"}, {"1.0.0", "1.0.0-rc.2"}, {"1.0.0-beta.11", "1.0.0-beta.2"}} {
		if Compare(Skill{Source: Source{Version: pair[0]}}, Skill{Source: Source{Version: pair[1]}}) <= 0 {
			t.Fatal(pair)
		}
	}
	if Compare(Skill{Source: Source{UpdatedAt: time.Now()}}, Skill{}) <= 0 {
		t.Fatal("mtime ignored")
	}
}

func TestRegistryConcurrentSyncAcknowledgementAndSuppression(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, key := range []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR", "PI_CODING_AGENT_DIR", "XDG_CONFIG_HOME"} {
		t.Setenv(key, "")
	}
	definition(t, filepath.Join(home, ".claude", "skills", "one"), "one", "1.0.0", "one")
	definition(t, filepath.Join(home, ".claude", "skills", "two"), "two", "1.0.0", "two")
	root := filepath.Join(home, ".csgclaw", "skills")
	items, err := Sync(root)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func(i int) {
			<-start
			switch i % 3 {
			case 0:
				_, err := Sync(root)
				results <- err
			case 1:
				results <- Acknowledge(root, []string{items[0].Key})
			case 2:
				results <- Ignore(root, "two")
			}
		}(i)
	}
	close(start)
	for i := 0; i < 12; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	after, err := Sync(root)
	if err != nil || len(after) != 1 || after[0].Name != "one" || after[0].New {
		t.Fatalf("concurrent updates lost: %+v %v", after, err)
	}
}

func TestManualRescanRestoresRemovedSkillsOnlyWhenRequested(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, key := range []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR", "PI_CODING_AGENT_DIR", "XDG_CONFIG_HOME"} {
		t.Setenv(key, "")
	}
	path := definition(t, filepath.Join(home, ".codex", "skills", "removed"), "removed", "1.0.0", "original source")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".csgclaw", "skills")
	initial, err := Sync(root)
	if err != nil || len(initial) != 1 {
		t.Fatalf("initial=%+v %v", initial, err)
	}
	if err := Acknowledge(root, []string{initial[0].Key}); err != nil {
		t.Fatal(err)
	}
	if err := Ignore(root, "removed"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if items, err := Sync(root); err != nil || len(items) != 0 {
			t.Fatalf("automatic scan restored deletion: %+v %v", items, err)
		}
	}
	restored, err := Rescan(root)
	if err != nil || len(restored) != 1 || !restored[0].New {
		t.Fatalf("manual rescan=%+v %v", restored, err)
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != string(original) {
		t.Fatal("source changed")
	}
	if items, err := Sync(root); err != nil || len(items) != 1 {
		t.Fatalf("restoration lost: %+v %v", items, err)
	}
}
