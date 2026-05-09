package user

import (
	"context"
	"flag"
	"fmt"
	"net/http"

	"csgclaw/cli/command"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel/feishu"
)

type cmd struct{}

func NewCmd() command.Command {
	return cmd{}
}

func (cmd) Name() string {
	return "user"
}

func (cmd) Summary() string {
	return "Manage IM users."
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
	default:
		c.usage(run)
		return fmt.Errorf("unknown user subcommand %q", args[0])
	}
}

func (c cmd) usage(run *command.Context) {
	run.UsageCommandGroup(c, run.Program+" user <subcommand> [flags]", []string{
		"list               List users",
		"create             Create a user",
		"delete <id>        Remove a user",
	})
}

func (c cmd) runList(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("user list", run.Program+" user list [flags]", "List users.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("user list does not accept positional arguments")
	}

	users, err := run.APIClient(globals).ListUsersByChannel(ctx, *channelName)
	if err != nil {
		return err
	}
	return command.RenderUsers(globals.Output, run.Stdout, users)
}

func (c cmd) runCreate(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("user create", run.Program+" user create [flags]", "Create a user.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	id := fs.String("id", "", "user id")
	name := fs.String("name", "", "user name")
	handle := fs.String("handle", "", "user handle")
	role := fs.String("role", "", "user role")
	avatar := fs.String("avatar", "", "user avatar initials")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("user create does not accept positional arguments")
	}
	if *name == "" {
		return fmt.Errorf("user create requires --name")
	}

	var (
		user apitypes.User
		err  error
	)
	switch *channelName {
	case "csgclaw":
		user, err = run.APIClient(globals).CreateUser(ctx, *channelName, apitypes.CreateUserRequest{
			ID:     *id,
			Name:   *name,
			Handle: *handle,
			Role:   *role,
		})
	case "feishu":
		err = run.APIClient(globals).DoJSON(ctx, http.MethodPost, "/api/v1/channels/feishu/users", feishu.CreateUserRequest{
			ID:     *id,
			Name:   *name,
			Handle: *handle,
			Role:   *role,
			Avatar: *avatar,
		}, &user)
	default:
		return fmt.Errorf("unsupported channel %q", *channelName)
	}
	if err != nil {
		return err
	}

	return command.RenderUsers(globals.Output, run.Stdout, []apitypes.User{user})
}

func (c cmd) runDelete(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("user delete", run.Program+" user delete <id> [flags]", "Remove a user.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("user delete requires exactly one id")
	}

	if err := run.APIClient(globals).DeleteUser(ctx, *channelName, rest[0]); err != nil {
		return err
	}
	return command.RenderAction(globals.Output, run.Stdout, command.ActionResult{
		Command: "user",
		Action:  "delete",
		Status:  "removed",
		ID:      rest[0],
		Message: fmt.Sprintf("removed user %s", rest[0]),
	})
}
