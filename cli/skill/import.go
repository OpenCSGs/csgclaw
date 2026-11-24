package skill

import (
	"fmt"
	"os"
	"path/filepath"

	"csgclaw/cli/command"
	"csgclaw/internal/identity"
	skillapi "csgclaw/internal/skill"
	"csgclaw/internal/skill/installstate"
)

func workerSkillsRoot() (string, bool) {
	id := identity.CanonicalAgentID(os.Getenv("CSGCLAW_CALLER_AGENT_ID"))
	if id == "" || id == identity.ManagerAgentID {
		return "", false
	}
	root := skillapi.RuntimeSkillsRoot()
	return root, filepath.IsAbs(root)
}

func isCurrentWorkerRoot(path string) bool {
	root, ok := workerSkillsRoot()
	if !ok {
		return false
	}
	full, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	if root == full {
		return true
	}
	a, err := os.Stat(root)
	if err != nil {
		return false
	}
	b, err := os.Stat(full)
	return err == nil && os.SameFile(a, b)
}

func (c cmd) runImport(run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("skill import", run.Program+" skill import <directory> [--force]", "Install the contents of a skill directory into the current worker, without enabling it.")
	force := fs.Bool("force", false, "replace an existing skill")
	confirmation := fs.String("confirmation", "", "confirmation from the worker's source-conflict response")
	sourceURL := fs.String("source-url", "", "original skills.sh skill page")
	if err := command.ParseFlexible(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("skill import requires one skill directory")
	}
	root, ok := workerSkillsRoot()
	if !ok {
		return fmt.Errorf("skill import requires a CSGClaw worker home")
	}
	result, err := skillapi.ImportForWorker(root, fs.Arg(0), *sourceURL, installstate.WorkerInstall{Replace: *force, Confirmation: *confirmation})
	if err != nil {
		return err
	}
	return renderInstallResult(globals.Output, run.Stdout, result)
}
