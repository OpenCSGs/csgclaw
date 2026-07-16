package template

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"text/tabwriter"

	"csgclaw/cli/command"
	"csgclaw/internal/apiclient"
	"csgclaw/internal/apitypes"
)

type cmd struct{}

func NewCmd() command.Command {
	return cmd{}
}

func (cmd) Name() string {
	return "template"
}

func (cmd) Summary() string {
	return "Discover agent templates."
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
	case "list":
		return c.runList(ctx, run, args[1:], globals)
	case "get":
		return c.runGet(ctx, run, args[1:], globals)
	case "publish":
		return c.runPublish(ctx, run, args[1:], globals)
	default:
		c.usage(run)
		return fmt.Errorf("unknown template subcommand %q", args[0])
	}
}

func (c cmd) usage(run *command.Context) {
	run.UsageCommandGroup(c, run.Program+" template <subcommand> [flags]", []string{
		"list               List templates",
		"get <template>     Show one template",
		"publish            Publish an existing agent as a template",
	})
}

func (c cmd) runList(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("template list", run.Program+" template list [flags]", "List templates.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("template list does not accept positional arguments")
	}

	items, err := listTemplates(ctx, run.APIClient(globals))
	if err != nil {
		return err
	}
	return renderTemplates(globals.Output, run.Stdout, items)
}

func (c cmd) runGet(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("template get", run.Program+" template get <template> [flags]", "Show a template.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("template get requires exactly one template id")
	}

	item, err := getTemplate(ctx, run.APIClient(globals), rest[0])
	if err != nil {
		return err
	}
	return renderTemplate(globals.Output, run.Stdout, item)
}

func (c cmd) runPublish(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("template publish", run.Program+" template publish --agent <id> [flags]", "Publish an agent as a template.")
	agentID := fs.String("agent", "", "existing agent id to publish")
	registry := fs.String("registry", "", "template registry to publish into")
	tags := fs.String("tags", "", "comma-separated template compatibility tags")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("template publish does not accept positional arguments")
	}
	if strings.TrimSpace(*agentID) == "" {
		return fmt.Errorf("template publish requires --agent")
	}

	item, err := publishTemplate(ctx, run.APIClient(globals), apitypes.CreateHubTemplateRequest{
		AgentID:  *agentID,
		Registry: *registry,
		Tags:     splitTemplateTags(*tags),
	})
	if err != nil {
		return err
	}
	return renderTemplate(globals.Output, run.Stdout, item)
}

func splitTemplateTags(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if tag := strings.TrimSpace(part); tag != "" {
			out = append(out, tag)
		}
	}
	return out
}

func listTemplates(ctx context.Context, client *apiclient.Client) ([]apitypes.HubTemplate, error) {
	var items []apitypes.HubTemplate
	if err := client.GetJSON(ctx, "/api/v1/hub/templates", &items); err != nil {
		return nil, err
	}
	return items, nil
}

func getTemplate(ctx context.Context, client *apiclient.Client, id string) (apitypes.HubTemplate, error) {
	var item apitypes.HubTemplate
	if err := client.GetJSON(ctx, "/api/v1/hub/templates/"+url.PathEscape(strings.TrimSpace(id)), &item); err != nil {
		return apitypes.HubTemplate{}, err
	}
	return item, nil
}

func publishTemplate(ctx context.Context, client *apiclient.Client, req apitypes.CreateHubTemplateRequest) (apitypes.HubTemplate, error) {
	var item apitypes.HubTemplate
	if err := client.DoJSON(ctx, "POST", "/api/v1/hub/templates", req, &item); err != nil {
		return apitypes.HubTemplate{}, err
	}
	return item, nil
}

func renderTemplates(output string, w io.Writer, items []apitypes.HubTemplate) error {
	switch output {
	case "", "table":
		return renderTemplatesTable(w, items)
	case "json":
		return command.WriteJSON(w, items)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func renderTemplate(output string, w io.Writer, item apitypes.HubTemplate) error {
	switch output {
	case "", "table":
		return renderTemplatesTable(w, []apitypes.HubTemplate{item})
	case "json":
		return command.WriteJSON(w, item)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func renderTemplatesTable(w io.Writer, items []apitypes.HubTemplate) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tREGISTRY\tKIND\tRUNTIME\tIMAGE")
	for _, item := range items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			displayField(item.ID),
			displayField(item.Name),
			displayField(item.Source.Name),
			displayField(item.Source.Kind),
			displayField(item.RuntimeKind),
			displayField(item.Image),
		)
	}
	return tw.Flush()
}

func displayField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}
