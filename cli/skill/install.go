package skill

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/clawhub"
)

func (c cmd) runInstall(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("skill install", run.Program+" skill install <slug> [flags]", "Install a ClawHub skill into the current workspace skills directory.")
	skillsDir := fs.String("skills-dir", "", "workspace skills directory (default: auto-detect ~/.picoclaw/workspace/skills or ~/.openclaw/workspace/skills)")
	version := fs.String("version", "", "install this semver version; default is the registry latest")
	registry := fs.String("registry", "", "registry: opencsg or clawhub (default: opencsg first, then clawhub)")
	force := fs.Bool("force", false, "overwrite an existing skill directory")
	if err := command.ParseFlexible(fs, args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("skill install requires exactly one skill slug")
	}

	skillsRoot, err := clawhub.ResolveSkillsRoot(*skillsDir)
	if err != nil {
		return err
	}

	registryID, err := clawhub.ParseRegistry(*registry)
	if err != nil {
		return err
	}
	result, err := newService(globals, run).Install(ctx, strings.TrimSpace(rest[0]), *version, registryID, skillsRoot, *force)
	if err != nil {
		if errors.Is(err, clawhub.ErrSkillDirExists) {
			return fmt.Errorf("%w; use --force to overwrite", err)
		}
		return err
	}
	return renderInstallResult(globals.Output, run.Stdout, result)
}
