package agents

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/skill/discovery"
	"csgclaw/internal/skill/installstate"
)

func TestResolveDiscoveredSkillInsideInstallationTransaction(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, key := range []string{"CODEX_HOME", "CLAUDE_CONFIG_DIR", "PI_CODING_AGENT_DIR", "XDG_CONFIG_HOME"} {
		t.Setenv(key, "")
	}
	dir := filepath.Join(home, ".claude", "skills", "pdf")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: pdf\ndescription: linked\n---\nfirst"), 0644)
	root := filepath.Join(home, ".csgclaw", "skills")
	if _, err := discovery.Sync(root); err != nil {
		t.Fatal(err)
	}
	if err := installstate.ReadSnapshot(root, func(_ map[string]installstate.Record) error {
		source, err := resolveAgentSkillSource(root, "pdf")
		if err != nil {
			return err
		}
		data, err := fs.ReadFile(source.FS, "SKILL.md")
		if err != nil {
			return err
		}
		if string(data) != "---\nname: pdf\ndescription: linked\n---\nfirst" {
			t.Fatal("wrong source")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
