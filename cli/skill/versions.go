package skill

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"csgclaw/cli/command"
	skillapi "csgclaw/internal/skill"
)

func (c cmd) runVersions(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("skill versions", run.Program+" skill versions <slug> [flags]", "List published versions of a ClawHub skill.")
	limit := fs.Int("limit", 50, "maximum number of versions to list")
	registry := fs.String("registry", "", "registry: opencsg or clawhub (default: opencsg first, then clawhub)")
	if err := command.ParseFlexible(fs, args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("skill versions requires exactly one skill slug")
	}
	registryID, err := skillapi.ParseRegistry(*registry)
	if err != nil {
		return err
	}

	svc, err := newService(globals, run)
	if err != nil {
		return err
	}
	list, err := svc.ListVersions(ctx, strings.TrimSpace(rest[0]), registryID, *limit)
	if err != nil {
		return err
	}
	return renderSkillVersions(globals.Output, run.Stdout, list)
}

func renderSkillVersions(output string, w io.Writer, list skillapi.SkillVersionsList) error {
	switch output {
	case "", "table":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "REGISTRY\tSLUG\tVERSION\tCREATED\tCHANGELOG\n")
		for _, item := range list.Versions {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				displayField(string(list.Registry)),
				displayField(list.Slug),
				displayField(item.Version),
				displayField(formatVersionCreatedAt(item.CreatedAt)),
				displayField(item.Changelog),
			)
		}
		return tw.Flush()
	case "json":
		return command.WriteJSON(w, list)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func formatVersionCreatedAt(createdAt int64) string {
	if createdAt <= 0 {
		return "-"
	}
	// OpenCSG/clawhub use epoch milliseconds.
	if createdAt > 1_000_000_000_000 {
		return time.UnixMilli(createdAt).UTC().Format(time.RFC3339)
	}
	return time.Unix(createdAt, 0).UTC().Format(time.RFC3339)
}
