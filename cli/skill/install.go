package skill

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"csgclaw/cli/command"
	skillapi "csgclaw/internal/skill"
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

	skillsRoot, err := skillapi.ResolveSkillsRoot(*skillsDir)
	if err != nil {
		return err
	}

	registryID, err := skillapi.ParseRegistry(*registry)
	if err != nil {
		return err
	}
	svc, err := newService(globals, run)
	if err != nil {
		return err
	}
	result, err := svc.Install(ctx, strings.TrimSpace(rest[0]), *version, registryID, skillsRoot, *force)
	if err != nil {
		if errors.Is(err, skillapi.ErrSkillDirExists) {
			return fmt.Errorf("%w; use --force to overwrite", err)
		}
		return err
	}
	return renderInstallResult(globals.Output, run.Stdout, result)
}
