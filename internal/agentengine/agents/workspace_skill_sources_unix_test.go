//go:build !windows

package agents

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	skillinstall "csgclaw/internal/skill/installstate"
)

func TestWorkspaceSkillSummariesRejectNamedPipe(t *testing.T) {
	workspace, agent, root := newSkillSourceWorkspace(t, RuntimeKindCodex, RoleWorker)
	name := "csgclaw-self-diagnosis"
	copySkillSourceFixture(t, root, name, systemSkillSourceFixture(t, name))
	file := filepath.Join(root, name, "agents", "openai.yaml")
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(file, 0600); err != nil {
		t.Fatal(err)
	}
	assertSkillSourceSummaries(t, workspace, agent.ID, map[string]string{name: skillinstall.SourceUnknown})
}
