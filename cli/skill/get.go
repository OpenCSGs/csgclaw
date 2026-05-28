package skill

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/cli/command"
	skillapi "csgclaw/internal/skill"
)

func (c cmd) runGet(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("skill get", run.Program+" skill get <slug> [flags]", "Show one ClawHub skill.")
	version := fs.String("version", "", "show a specific published version")
	registry := fs.String("registry", "", "registry: opencsg or clawhub (default: opencsg first, then clawhub)")
	if err := command.ParseFlexible(fs, args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("skill get requires exactly one skill slug")
	}
	slug := strings.TrimSpace(rest[0])
	registryID, err := skillapi.ParseRegistry(*registry)
	if err != nil {
		return err
	}
	svc, err := newService(globals, run)
	if err != nil {
		return err
	}

	if strings.TrimSpace(*version) != "" {
		detail, err := svc.GetVersion(ctx, slug, *version, registryID)
		if err != nil {
			return err
		}
		return renderSkillVersion(globals.Output, run.Stdout, detail)
	}

	detail, err := svc.Get(ctx, slug, registryID)
	if err != nil {
		return err
	}
	return renderSkillDetail(globals.Output, run.Stdout, detail)
}
