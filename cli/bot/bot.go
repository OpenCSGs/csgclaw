package bot

import (
	"context"
	"flag"
	"fmt"

	"csgclaw/cli/command"
	"csgclaw/internal/apitypes"
)

type cmd struct{}

func NewCmd() command.Command {
	return cmd{}
}

func (cmd) Name() string {
	return "bot"
}

func (cmd) Summary() string {
	return "Manage bots."
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
	case "delete":
		return c.runDelete(ctx, run, args[1:], globals)
	case "config":
		return c.runConfig(ctx, run, args[1:], globals)
	default:
		c.usage(run)
		return fmt.Errorf("unknown bot subcommand %q", args[0])
	}
}

func (c cmd) usage(run *command.Context) {
	subcommands := []string{
		"list               List bots",
		"create             Create a bot",
		"delete <id>        Delete a bot",
		"config             Manage bot channel config",
	}
	run.UsageCommandGroup(c, run.Program+" bot <subcommand> [flags]", subcommands)
}

func (c cmd) runList(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("bot list", run.Program+" bot list [flags]", "List bots.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	role := fs.String("role", "", "bot role: manager or worker")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("bot list does not accept positional arguments")
	}

	bots, err := run.APIClient(globals).ListBots(ctx, *channelName, *role)
	if err != nil {
		return err
	}
	return command.RenderBots(globals.Output, run.Stdout, bots)
}

func (c cmd) runCreate(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("bot create", run.Program+" bot create [flags]", "Create a bot.")
	id := fs.String("id", "", "bot id")
	name := fs.String("name", "", "bot name")
	description := fs.String("description", "", "bot description")
	role := fs.String("role", "", "bot role: manager or worker")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	modelID := fs.String("model-id", "", "agent model identifier")
	runtimeKind := fs.String("runtime", "", "agent runtime kind for worker bots (for example: picoclaw-sandbox, codex)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("bot create does not accept positional arguments")
	}
	if *name == "" {
		return fmt.Errorf("bot create requires --name")
	}
	if *role == "" {
		return fmt.Errorf("bot create requires --role")
	}

	created, err := run.APIClient(globals).CreateBot(ctx, apitypes.CreateBotRequest{
		ID:          *id,
		Name:        *name,
		Description: *description,
		Role:        *role,
		Channel:     *channelName,
		ModelID:     *modelID,
		RuntimeKind: *runtimeKind,
	})
	if err != nil {
		return err
	}
	return command.RenderBots(globals.Output, run.Stdout, []apitypes.Bot{created})
}

func (c cmd) runDelete(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("bot delete", run.Program+" bot delete <id> [flags]", "Delete a bot.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("bot delete requires exactly one id")
	}

	if err := run.APIClient(globals).DeleteBot(ctx, *channelName, rest[0]); err != nil {
		return err
	}
	return command.RenderAction(globals.Output, run.Stdout, command.ActionResult{
		Command: "bot",
		Action:  "delete",
		Status:  "deleted",
		ID:      rest[0],
		Channel: *channelName,
		Message: fmt.Sprintf("deleted %s bot %s", *channelName, rest[0]),
	})
}
