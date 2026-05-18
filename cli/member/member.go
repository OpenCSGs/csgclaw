package member

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
	return "member"
}

func (cmd) Summary() string {
	return "Manage IM room members."
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
	default:
		c.usage(run)
		return fmt.Errorf("unknown member subcommand %q", args[0])
	}
}

func (c cmd) usage(run *command.Context) {
	run.UsageCommandGroup(c, run.Program+" member <subcommand> [flags]", []string{
		"list               List room members",
		"create             Add a member to a room",
	})
}

func (c cmd) runList(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("member list", run.Program+" member list [flags]", "List room members.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	roomID := fs.String("room-id", "", "target room id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("member list does not accept positional arguments")
	}

	users, err := run.APIClient(globals).ListRoomMembersByChannel(ctx, *channelName, *roomID)
	if err != nil {
		return err
	}
	return command.RenderUsers(globals.Output, run.Stdout, users)
}

func (c cmd) runCreate(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("member create", run.Program+" member create [flags]", "Add a member to a room.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	roomID := fs.String("room-id", "", "target room id")
	userID := fs.String("user-id", "", "bot id to add")
	inviterID := fs.String("inviter-id", "", "inviter bot id")
	locale := fs.String("locale", "", "room locale")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("member create does not accept positional arguments")
	}
	if *userID == "" {
		return fmt.Errorf("--user-id bot id is required")
	}

	room, err := run.APIClient(globals).AddRoomMemberByChannel(ctx, *channelName, apitypes.AddRoomMembersRequest{
		RoomID:    *roomID,
		InviterID: *inviterID,
		UserIDs:   []string{*userID},
		Locale:    *locale,
	})
	if err != nil {
		return err
	}
	return command.RenderRooms(globals.Output, run.Stdout, []apitypes.Room{room})
}
