package agent

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/apiclient"
	"csgclaw/internal/apitypes"
)

type cmd struct{}

func NewCmd() command.Command {
	return cmd{}
}

func (cmd) Name() string {
	return "agent"
}

func (cmd) Summary() string {
	return "Manage agents."
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
	case "create":
		return c.runCreate(ctx, run, args[1:], globals)
	case "start":
		return c.runStart(ctx, run, args[1:], globals)
	case "stop":
		return c.runStop(ctx, run, args[1:], globals)
	case "delete":
		return c.runDelete(ctx, run, args[1:], globals)
	case "logs":
		return c.runLogs(ctx, run, args[1:], globals)
	default:
		c.usage(run)
		return fmt.Errorf("unknown agent subcommand %q", args[0])
	}
}

func (c cmd) usage(run *command.Context) {
	run.UsageCommandGroup(c, run.Program+" agent <subcommand> [flags]", []string{
		"list               List agents",
		"create             Create an agent",
		"start <id>         Start an agent",
		"stop <id>          Stop an agent",
		"delete <id>        Delete one agent",
		"delete --all       Delete all agents",
		"logs <id>          Show agent logs",
	})
}

func (c cmd) runList(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("agent list", run.Program+" agent list [flags]", "List agents.")
	filter := fs.String("filter", "", "filter by state")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("agent list does not accept positional arguments")
	}

	agents, err := listAgents(ctx, run.APIClient(globals))
	if err != nil {
		return err
	}
	if *filter != "" {
		agents = filterAgentsByStatus(agents, *filter)
	}
	return command.RenderAgents(globals.Output, run.Stdout, agents)
}

func (c cmd) runCreate(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("agent create", run.Program+" agent create [flags]", "Create an agent.")
	replace := fs.Bool("replace", false, "replace an existing agent in place")
	fs.BoolVar(replace, "r", false, "replace an existing agent in place")
	force := fs.Bool("force", false, "replace an existing agent without confirmation")
	fs.BoolVar(force, "f", false, "replace an existing agent without confirmation")
	id := fs.String("id", "", "agent id")
	name := fs.String("name", "", "agent name")
	description := fs.String("description", "", "agent description")
	image := fs.String("image", "", "agent image")
	profile := fs.String("profile", "", "agent llm profile")
	runtimeKind := fs.String("runtime", "", "agent runtime kind (for example: picoclaw_sandbox, openclaw_sandbox, codex)")
	fs.Usage = func() {
		fmt.Fprintln(run.Stderr, "Create an agent.")
		fmt.Fprintln(run.Stderr)
		fmt.Fprintln(run.Stderr, "Usage:")
		fmt.Fprintf(run.Stderr, "  %s agent create [flags]\n", run.Program)
		fmt.Fprintf(run.Stderr, "  %s agent create --replace --id <id> [flags]\n", run.Program)
		fmt.Fprintln(run.Stderr)
		fmt.Fprintln(run.Stderr, "Flags:")
		fmt.Fprintln(run.Stderr, "  -r, --replace           replace an existing agent in place")
		fmt.Fprintln(run.Stderr, "  -f, --force             replace an existing agent without confirmation")
		fmt.Fprintln(run.Stderr, "  --id string             agent id")
		fmt.Fprintln(run.Stderr, "  --name string           agent name")
		fmt.Fprintln(run.Stderr, "  --description string    agent description")
		fmt.Fprintln(run.Stderr, "  --image string          agent image")
		fmt.Fprintln(run.Stderr, "  --profile string        agent llm profile")
		fmt.Fprintln(run.Stderr, "  --runtime string        agent runtime kind (for example: picoclaw_sandbox, openclaw_sandbox, codex)")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("agent create does not accept positional arguments")
	}

	req := apitypes.CreateAgentRequest{
		ID:          *id,
		Name:        *name,
		Description: *description,
		Image:       *image,
		RuntimeKind: *runtimeKind,
		Replace:     *replace,
		Profile:     *profile,
	}
	req.ID = normalizeAgentID(req.ID)
	client := run.APIClient(globals)
	if *replace {
		if strings.TrimSpace(req.ID) == "" {
			return fmt.Errorf("agent create --replace requires --id")
		}
		req.FieldMask = createAgentFieldMask(visitedFlags(fs))
		req.Replace = true
		if !*force {
			confirmed, err := confirmReplaceAgent(run, req.ID)
			if err != nil {
				return err
			}
			if !confirmed {
				return command.RenderAction(globals.Output, run.Stdout, command.ActionResult{
					Command: "agent",
					Action:  "create",
					Status:  "cancelled",
					ID:      req.ID,
					Message: fmt.Sprintf("cancelled replacing agent %s", req.ID),
				})
			}
		}
	}

	created, err := createAgent(ctx, client, req)
	if err != nil {
		return err
	}
	return command.RenderAgents(globals.Output, run.Stdout, []apitypes.Agent{created})
}

func (c cmd) runStart(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("agent start", run.Program+" agent start <id> [flags]", "Start an agent.")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("agent start requires exactly one id")
	}

	started, err := startAgent(ctx, run.APIClient(globals), rest[0])
	if err != nil {
		return err
	}
	return command.RenderAgents(globals.Output, run.Stdout, []apitypes.Agent{started})
}

func (c cmd) runStop(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("agent stop", run.Program+" agent stop <id> [flags]", "Stop an agent.")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("agent stop requires exactly one id")
	}

	stopped, err := stopAgent(ctx, run.APIClient(globals), rest[0])
	if err != nil {
		return err
	}
	return command.RenderAgents(globals.Output, run.Stdout, []apitypes.Agent{stopped})
}

func (c cmd) runDelete(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("agent delete", run.Program+" agent delete <id> [flags]\n  "+run.Program+" agent delete --all [flags]", "Delete one agent or all agents.")
	all := fs.Bool("all", false, "delete all agents")
	fs.BoolVar(all, "a", false, "delete all agents")
	force := fs.Bool("force", false, "delete all agents without confirmation")
	fs.BoolVar(force, "f", false, "delete all agents without confirmation")
	fs.Usage = func() {
		fmt.Fprintln(run.Stderr, "Delete one agent or all agents.")
		fmt.Fprintln(run.Stderr)
		fmt.Fprintln(run.Stderr, "Usage:")
		fmt.Fprintf(run.Stderr, "  %s agent delete <id> [flags]\n", run.Program)
		fmt.Fprintf(run.Stderr, "  %s agent delete --all [flags]\n", run.Program)
		fmt.Fprintln(run.Stderr)
		fmt.Fprintln(run.Stderr, "Flags:")
		fmt.Fprintln(run.Stderr, "  -a, --all     delete all agents")
		fmt.Fprintln(run.Stderr, "  -f, --force   delete all agents without confirmation")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if *all {
		if len(rest) != 0 {
			return fmt.Errorf("agent delete with --all does not accept an id")
		}
		return c.runDeleteAll(ctx, run, globals, *force)
	}
	if len(rest) != 1 {
		return fmt.Errorf("agent delete requires exactly one id unless --all is set")
	}

	id := normalizeAgentID(rest[0])
	if err := run.APIClient(globals).DoNoContent(ctx, http.MethodDelete, "/api/v1/agents/"+id); err != nil {
		return err
	}
	return command.RenderAction(globals.Output, run.Stdout, command.ActionResult{
		Command: "agent",
		Action:  "delete",
		Status:  "deleted",
		ID:      id,
		Message: fmt.Sprintf("deleted agent %s", id),
	})
}

func (c cmd) runDeleteAll(ctx context.Context, run *command.Context, globals command.GlobalOptions, force bool) error {
	if !force {
		confirmed, err := confirmDeleteAll(run)
		if err != nil {
			return err
		}
		if !confirmed {
			return command.RenderAction(globals.Output, run.Stdout, command.ActionResult{
				Command: "agent",
				Action:  "delete",
				Status:  "cancelled",
				Message: "cancelled deleting all agents",
			})
		}
	}
	agents, err := listAgents(ctx, run.APIClient(globals))
	if err != nil {
		return err
	}
	for _, a := range agents {
		if err := run.APIClient(globals).DoNoContent(ctx, http.MethodDelete, "/api/v1/agents/"+a.ID); err != nil {
			return err
		}
	}
	return command.RenderAction(globals.Output, run.Stdout, command.ActionResult{
		Command: "agent",
		Action:  "delete",
		Status:  "deleted",
		Message: fmt.Sprintf("deleted %d agents", len(agents)),
	})
}

func confirmDeleteAll(run *command.Context) (bool, error) {
	if _, err := fmt.Fprint(run.Stdout, "This will remove all agents. Are you sure? [y/N] "); err != nil {
		return false, err
	}
	answer, err := bufio.NewReader(run.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func confirmReplaceAgent(run *command.Context, id string) (bool, error) {
	if _, err := fmt.Fprintf(run.Stdout, "This will recreate agent %s in place. Are you sure? [y/N] ", id); err != nil {
		return false, err
	}
	answer, err := bufio.NewReader(run.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func (c cmd) runLogs(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("agent logs", run.Program+" agent logs <id> [-f] [-n lines]", "Show agent logs.")
	follow := fs.Bool("f", false, "follow log output")
	fs.BoolVar(follow, "follow", false, "follow log output")
	lines := fs.Int("n", 20, "number of lines to show")
	flagArgs, rest := splitLogsArgs(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("agent logs requires exactly one id")
	}
	if *lines <= 0 {
		return fmt.Errorf("agent logs requires -n to be greater than 0")
	}

	values := url.Values{}
	if *follow {
		values.Set("follow", "true")
	}
	apiclient.QueryInt(values, "lines", *lines)
	if globals.Output == "json" {
		if *follow {
			return fmt.Errorf("agent logs does not support --output json with --follow")
		}
		var buf bytes.Buffer
		if err := run.APIClient(globals).Stream(ctx, "/api/v1/agents/"+rest[0]+"/logs", values, &buf); err != nil {
			return err
		}
		return command.RenderAction(globals.Output, run.Stdout, command.ActionResult{
			Command: "agent",
			Action:  "logs",
			Status:  "ok",
			ID:      rest[0],
			Logs:    buf.String(),
			Lines:   *lines,
			Follow:  false,
		})
	}
	return run.APIClient(globals).Stream(ctx, "/api/v1/agents/"+rest[0]+"/logs", values, run.Stdout)
}

func listAgents(ctx context.Context, client *apiclient.Client) ([]apitypes.Agent, error) {
	var agents []apitypes.Agent
	if err := client.GetJSON(ctx, "/api/v1/agents", &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

func createAgent(ctx context.Context, client *apiclient.Client, req apitypes.CreateAgentRequest) (apitypes.Agent, error) {
	var created apitypes.Agent
	if err := client.DoJSON(ctx, http.MethodPost, "/api/v1/agents", req, &created); err != nil {
		return apitypes.Agent{}, err
	}
	return created, nil
}

func startAgent(ctx context.Context, client *apiclient.Client, id string) (apitypes.Agent, error) {
	var started apitypes.Agent
	if err := client.DoJSON(ctx, http.MethodPost, "/api/v1/agents/"+id+"/start", nil, &started); err != nil {
		return apitypes.Agent{}, err
	}
	return started, nil
}

func stopAgent(ctx context.Context, client *apiclient.Client, id string) (apitypes.Agent, error) {
	var stopped apitypes.Agent
	if err := client.DoJSON(ctx, http.MethodPost, "/api/v1/agents/"+id+"/stop", nil, &stopped); err != nil {
		return apitypes.Agent{}, err
	}
	return stopped, nil
}

func filterAgentsByStatus(agents []apitypes.Agent, status string) []apitypes.Agent {
	filtered := make([]apitypes.Agent, 0, len(agents))
	for _, a := range agents {
		if a.Status == status {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

func visitedFlags(fs interface{ Visit(func(*flag.Flag)) }) map[string]bool {
	visited := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	return visited
}

func createAgentFieldMask(visited map[string]bool) []string {
	fields := []string{"id", "name", "description", "image", "profile", "runtime_kind"}
	mask := make([]string, 0, len(fields))
	for _, field := range fields {
		if visited[field] {
			mask = append(mask, field)
		}
	}
	if visited["runtime"] && !visited["runtime_kind"] {
		mask = append(mask, "runtime_kind")
	}
	return mask
}

func normalizeAgentID(id string) string {
	if strings.EqualFold(strings.TrimSpace(id), "manager") {
		return "u-manager"
	}
	return id
}

func splitLogsArgs(args []string) ([]string, []string) {
	flagArgs := make([]string, 0, len(args))
	rest := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-f", arg == "--follow", strings.HasPrefix(arg, "--follow="):
			flagArgs = append(flagArgs, arg)
		case arg == "-n":
			flagArgs = append(flagArgs, arg)
			if i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		case strings.HasPrefix(arg, "-n="):
			flagArgs = append(flagArgs, arg)
		case strings.HasPrefix(arg, "-"):
			flagArgs = append(flagArgs, arg)
		default:
			rest = append(rest, arg)
		}
	}
	return flagArgs, rest
}
