package message

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
	return "message"
}

func (cmd) Summary() string {
	return "Manage IM messages."
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
		return fmt.Errorf("unknown message subcommand %q", args[0])
	}
}

func (c cmd) usage(run *command.Context) {
	run.UsageCommandGroup(c, run.Program+" message <subcommand> [flags]", []string{
		"list               List messages",
		"create             Create a message",
	})
}

func (c cmd) runList(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("message list", run.Program+" message list [flags]", "List messages.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	roomID := fs.String("room-id", "", "target room id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("message list does not accept positional arguments")
	}
	if *roomID == "" {
		return fmt.Errorf("room_id is required")
	}

	messages, err := run.APIClient(globals).ListMessagesByChannel(ctx, *channelName, *roomID)
	if err != nil {
		return err
	}
	return command.RenderMessages(globals.Output, run.Stdout, messages)
}

func (c cmd) runCreate(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("message create", run.Program+" message create [flags]", "Create a message.")
	channelName := fs.String("channel", "csgclaw", "channel name: csgclaw or feishu")
	roomID := fs.String("room-id", "", "target room id")
	senderID := fs.String("sender-id", "", "sender bot id")
	content := fs.String("content", "", "message content")
	mentionID := fs.String("mention-id", "", "mentioned bot id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("message create does not accept positional arguments")
	}
	if *roomID == "" {
		return fmt.Errorf("room_id is required")
	}
	if *senderID == "" {
		return fmt.Errorf("--sender-id is required")
	}
	if *content == "" {
		return fmt.Errorf("content is required")
	}

	message, err := run.APIClient(globals).SendMessageByChannel(ctx, *channelName, apitypes.CreateMessageRequest{
		RoomID:    *roomID,
		SenderID:  *senderID,
		Content:   *content,
		MentionID: *mentionID,
	})
	if err != nil {
		return err
	}
	return command.RenderMessages(globals.Output, run.Stdout, []apitypes.Message{message})
}
