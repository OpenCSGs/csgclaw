package room

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/apitypes"
)

type cmd struct{}

func NewCmd() command.Command {
	return cmd{}
}

func (cmd) Name() string {
	return "room"
}

func (cmd) Summary() string {
	return "Manage conversations and collaboration spaces."
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
	case "attachments":
		return c.runAttachments(ctx, run, args[1:], globals)
	case "list":
		return c.runList(ctx, run, args[1:], globals)
	case "create":
		return c.runCreate(ctx, run, args[1:], globals)
	case "delete":
		return c.runDelete(ctx, run, args[1:], globals)
	default:
		c.usage(run)
		return fmt.Errorf("unknown room subcommand %q", args[0])
	}
}

func (c cmd) usage(run *command.Context) {
	run.UsageCommandGroup(c, run.Program+" room <subcommand> [flags]", []string{
		"list               List conversations and collaboration spaces",
		"attachments        List, download, or delete room attachments",
		"create             Create a collaboration space or Feishu group",
		"delete <id>        Delete a room",
	})
}

func (c cmd) runList(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("room list", run.Program+" room list [flags]", "List conversations and collaboration spaces. The DIRECT column identifies private chats.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("room list does not accept positional arguments")
	}

	rooms, err := run.APIClient(globals).ListRoomsByChannel(ctx, *channelName)
	if err != nil {
		return err
	}
	return command.RenderRooms(globals.Output, run.Stdout, rooms)
}

func (c cmd) runCreate(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("room create", run.Program+" room create [flags]", "Create a CSGClaw collaboration space or a Feishu group. This creates a group conversation even with one invited Agent; direct chats use separate conversation workflows.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	title := fs.String("title", "", "room title")
	description := fs.String("description", "", "room description")
	creatorID := fs.String("creator-id", "", "creator participant id")
	memberIDs := fs.String("member-ids", "", "comma-separated member participant ids")
	locale := fs.String("locale", "", "room locale")
	roomTypeValue := fs.String("type", "", "CSGClaw collaboration mode: on_demand (moderated, default) or autonomous (roundtable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("room create does not accept positional arguments")
	}

	channelValue := strings.ToLower(strings.TrimSpace(*channelName))
	if channelValue == "" {
		channelValue = "csgclaw"
	}
	roomType := apitypes.RoomType(strings.TrimSpace(*roomTypeValue))
	if roomType != "" && roomType != apitypes.RoomTypeOnDemand && roomType != apitypes.RoomTypeAutonomous {
		return fmt.Errorf("type must be one of: on_demand (moderated), autonomous (roundtable); free rooms can no longer be created")
	}
	if roomType == "" {
		roomType = apitypes.RoomTypeFree
		if channelValue == "csgclaw" {
			roomType = apitypes.RoomTypeOnDemand
		}
	}
	if (roomType == apitypes.RoomTypeOnDemand || roomType == apitypes.RoomTypeAutonomous) && channelValue != "csgclaw" {
		return fmt.Errorf("collaboration rooms currently support csgclaw only")
	}
	// Room is the shared conversation model. This entry creates group spaces;
	// IsDirect, rather than the number of members, distinguishes private chats.
	room, err := run.APIClient(globals).CreateRoomByChannel(ctx, channelValue, apitypes.CreateRoomRequest{
		IsDirect:    false,
		Type:        roomType,
		Title:       *title,
		Description: *description,
		CreatorID:   *creatorID,
		MemberIDs:   command.ParseCSV(*memberIDs),
		Locale:      *locale,
	})
	if err != nil {
		return err
	}
	return command.RenderRooms(globals.Output, run.Stdout, []apitypes.Room{room})
}

func (c cmd) runDelete(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("room delete", run.Program+" room delete <id> [flags]", "Delete a room.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("room delete requires exactly one id")
	}

	if err := run.APIClient(globals).DeleteRoom(ctx, *channelName, rest[0]); err != nil {
		return err
	}
	return command.RenderAction(globals.Output, run.Stdout, command.ActionResult{
		Command: "room",
		Action:  "delete",
		Status:  "deleted",
		ID:      rest[0],
		Channel: *channelName,
		Message: fmt.Sprintf("deleted %s room %s", *channelName, rest[0]),
	})
}
