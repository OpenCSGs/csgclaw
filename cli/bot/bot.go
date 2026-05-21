package bot

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/apitypes"
	botdomain "csgclaw/internal/bot"
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
		"list               List bots (--type normal|notification optional; csgclaw default includes notification)",
		"create             Create a bot",
		"delete <id>        Delete a bot",
		"config             Manage bot channel config",
	}
	run.UsageCommandGroup(c, run.Program+" bot <subcommand> [flags]", subcommands)
}

func (c cmd) runList(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("bot list", run.Program+" bot list [flags]", "List bots (csgclaw includes notification bots; feishu lists normal bots only).")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	role := fs.String("role", "", "bot role: manager or worker")
	botType := fs.String("type", "", "bot type filter: normal or notification (default: all types allowed for channel)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("bot list does not accept positional arguments")
	}

	client := run.APIClient(globals)
	typeFilter := strings.TrimSpace(*botType)
	if typeFilter != "" {
		typeFilter = botdomain.NormalizeBotType(typeFilter)
	}
	bots, err := client.ListBots(ctx, *channelName, *role, typeFilter)
	if err != nil {
		return err
	}
	return renderBotList(run, globals, bots)
}

func renderBotList(run *command.Context, globals command.GlobalOptions, bots []apitypes.Bot) error {
	if strings.TrimSpace(run.Program) == "csgclaw-cli" {
		return command.RenderCompactBotList(globals.Output, run.Stdout, bots)
	}
	return command.RenderFullBotList(globals.Output, run.Stdout, bots)
}

func (c cmd) runCreate(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("bot create", run.Program+" bot create [flags]", "Create a bot.")
	id := fs.String("id", "", "bot id")
	name := fs.String("name", "", "bot name")
	description := fs.String("description", "", "bot description")
	role := fs.String("role", "", "bot role: manager or worker")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	modelID := fs.String("model-id", "", "agent model identifier")
	runtimeKind := fs.String("runtime", "", "agent runtime kind for worker bots (for example: picoclaw_sandbox, openclaw_sandbox, codex)")
	botType := fs.String("type", botdomain.BotTypeNormal, "bot type: normal or notification")
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

	req := apitypes.CreateBotRequest{
		ID:          *id,
		Name:        *name,
		Description: *description,
		Type:        botdomain.NormalizeBotType(*botType),
		Role:        *role,
		Channel:     *channelName,
		RuntimeKind: *runtimeKind,
	}
	if strings.TrimSpace(*modelID) != "" {
		req.AgentProfile = &apitypes.CreateAgentProfile{ModelID: *modelID}
	}
	client := run.APIClient(globals)
	var created apitypes.Bot
	var err error
	if req.Type == botdomain.BotTypeNotification {
		created, err = client.CreateNotificationBot(ctx, req)
	} else {
		created, err = client.CreateBot(ctx, req)
	}
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
