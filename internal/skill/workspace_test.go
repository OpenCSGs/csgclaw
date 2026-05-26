package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSkillsRootOverride(t *testing.T) {
	t.Parallel()

	got, err := ResolveSkillsRoot("/tmp/custom-skills")
	if err != nil {
		t.Fatalf("ResolveSkillsRoot() error = %v", err)
	}
	if got != "/tmp/custom-skills" {
		t.Fatalf("got = %q", got)
	}
}

func TestResolveSkillsRootFromWorkspace(t *testing.T) {
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	skills := filepath.Join(workspace, "skills")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	t.Setenv("CSGCLAW_SKILLS_DIR", skills)

	got, err := ResolveSkillsRoot("")
	if err != nil {
		t.Fatalf("ResolveSkillsRoot() error = %v", err)
	}
	if got != skills {
		t.Fatalf("got = %q, want %q", got, skills)
	}
}
