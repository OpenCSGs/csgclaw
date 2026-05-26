package skill

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"csgclaw/cli/command"
	skillapi "csgclaw/internal/skill"
)

type cmd struct{}

func NewCmd() command.Command {
	return cmd{}
}

func (cmd) Name() string {
	return "skill"
}

func (cmd) Summary() string {
	return "Discover and install ClawHub skills."
}

func (c cmd) Run(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	if len(args) == 0 {
		c.usage(run)
		return flag.ErrHelp
	}
	if command.IsHelpArg(args[0]) {
		c.usage(run)
		return flag.ErrHelp
	}

	switch args[0] {
	case "search":
		return c.runSearch(ctx, run, args[1:], globals)
	case "get":
		return c.runGet(ctx, run, args[1:], globals)
	case "versions":
		return c.runVersions(ctx, run, args[1:], globals)
	case "install":
		return c.runInstall(ctx, run, args[1:], globals)
	default:
		c.usage(run)
		return fmt.Errorf("unknown skill subcommand %q", args[0])
	}
}

func (c cmd) usage(run *command.Context) {
	run.UsageCommandGroup(c, run.Program+" skill <subcommand> [flags]", []string{
		"search <query>     Search opencsg first; clawhub.ai if no hits",
		"get <slug>         Show one skill (--registry opencsg|clawhub)",
		"versions <slug>    List published versions (--registry, --limit)",
		"install <slug>     Install into local workspace/skills (--registry, --version)",
	})
}

func newService(globals command.GlobalOptions, run *command.Context) (*skillapi.Service, error) {
	cfg, err := resolveSkillConfig(globals)
	if err != nil {
		return nil, err
	}
	return skillapi.NewService(cfg, run.HTTPClient), nil
}

func renderSearchResults(output string, w io.Writer, items []skillapi.SearchResult) error {
	switch output {
	case "", "table":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "REGISTRY\tSLUG\tNAME\tVERSION\tSCORE\tSUMMARY")
		for _, item := range items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%.3f\t%s\n",
				displayField(string(item.Registry)),
				displayField(item.Slug),
				displayField(item.DisplayName),
				displayField(item.Version),
				item.Score,
				displayField(item.Summary),
			)
		}
		return tw.Flush()
	case "json":
		return command.WriteJSON(w, items)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func renderSkillDetail(output string, w io.Writer, detail skillapi.SkillDetail) error {
	switch output {
	case "", "table":
		version := ""
		if detail.LatestVersion != nil {
			version = detail.LatestVersion.Version
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "FIELD\tVALUE")
		fmt.Fprintf(tw, "registry\t%s\n", displayField(string(detail.Registry)))
		fmt.Fprintf(tw, "slug\t%s\n", displayField(detail.Skill.Slug))
		fmt.Fprintf(tw, "name\t%s\n", displayField(detail.Skill.DisplayName))
		fmt.Fprintf(tw, "version\t%s\n", displayField(version))
		fmt.Fprintf(tw, "summary\t%s\n", displayField(detail.Skill.Summary))
		if detail.Moderation != nil {
			fmt.Fprintf(tw, "suspicious\t%t\n", detail.Moderation.IsSuspicious)
			fmt.Fprintf(tw, "malware_blocked\t%t\n", detail.Moderation.IsMalwareBlocked)
			fmt.Fprintf(tw, "verdict\t%s\n", displayField(detail.Moderation.Verdict))
		}
		return tw.Flush()
	case "json":
		return command.WriteJSON(w, detail)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func renderSkillVersion(output string, w io.Writer, detail skillapi.SkillVersionDetail) error {
	switch output {
	case "", "table":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "FIELD\tVALUE")
		fmt.Fprintf(tw, "registry\t%s\n", displayField(string(detail.Registry)))
		fmt.Fprintf(tw, "slug\t%s\n", displayField(detail.Skill.Slug))
		fmt.Fprintf(tw, "name\t%s\n", displayField(detail.Skill.DisplayName))
		fmt.Fprintf(tw, "version\t%s\n", displayField(detail.Version.Version))
		fmt.Fprintf(tw, "changelog\t%s\n", displayField(detail.Version.Changelog))
		return tw.Flush()
	case "json":
		return command.WriteJSON(w, detail)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func renderInstallResult(output string, w io.Writer, result skillapi.InstallResult) error {
	switch output {
	case "", "table":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "REGISTRY\tSLUG\tVERSION\tPATH")
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", displayField(string(result.Registry)), result.Slug, result.Version, result.SkillsDir)
		return tw.Flush()
	case "json":
		return command.WriteJSON(w, result)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func displayField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}
