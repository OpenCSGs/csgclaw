package skill

import (
	"fmt"
	"os"
	"path/filepath"

	"csgclaw/cli/command"
	"csgclaw/internal/identity"
	skillapi "csgclaw/internal/skill"
)

func workerSkillsRoot() (string, bool) {
	id := identity.CanonicalAgentID(os.Getenv("CSGCLAW_CALLER_AGENT_ID"))
	if id == "" || id == identity.ManagerAgentID {
		return "", false
	}
	root := skillapi.RuntimeSkillsRoot()
	return root, filepath.IsAbs(root)
}

func (c cmd) runImport(run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("skill import", run.Program+" skill import <directory> [--force]", "Install the contents of a skill directory into the current worker, without enabling it.")
	force := fs.Bool("force", false, "replace an existing skill")
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
	result, err := skillapi.ImportDirectory(root, fs.Arg(0), *force)
	if err != nil {
		return err
	}
	return renderInstallResult(globals.Output, run.Stdout, result)
}
