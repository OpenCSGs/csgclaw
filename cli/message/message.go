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
	case "read":
		return c.runRead(ctx, run, args[1:], globals)
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
		"read               Read and acknowledge a page of unread collaboration messages",
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
	senderID := fs.String("sender-id", "", "sender participant id")
	content := fs.String("content", "", "message content")
	thread := fs.String("thread-root-id", "", "reply inside this message thread")
	clientID := fs.String("client-message-id", "", "stable id for a retry-safe public message")
	mentionID := fs.String("mention-id", "", "mentioned participant id")
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
		return fmt.Errorf("--sender-id participant id is required")
	}
	if *content == "" {
		return fmt.Errorf("content is required")
	}

	var relation *apitypes.MessageRelation
	if *thread != "" {
		relation = &apitypes.MessageRelation{RelType: "m.thread", EventID: *thread}
	}
	message, err := run.APIClient(globals).SendMessageByChannel(ctx, *channelName, apitypes.CreateMessageRequest{
		RoomID:          *roomID,
		RelatesTo:       relation,
		ClientMessageID: *clientID,
		SenderID:        *senderID,
		Content:         *content,
		MentionID:       *mentionID,
	})
	if err != nil {
		return err
	}
	return command.RenderMessages(globals.Output, run.Stdout, []apitypes.Message{message})
}

func (c cmd) runRead(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	fs := run.NewFlagSet("message read", run.Program+" message read --unread --room-id ID [flags]", "Read unread messages. Acknowledge only after the entire JSON page is written successfully.")
	room := fs.String("room-id", "", "room id")
	actor := fs.String("participant-id", "", "your participant id; defaults to the authenticated Agent")
	thread := fs.String("thread-root-id", "", "read this thread only; defaults to the bound conversation or main chat")
	unread := fs.Bool("unread", false, "read one page of unread messages")
	peek := fs.Bool("peek", false, "preview without consuming unread messages")
	limit := fs.Int("limit", 50, "page size (1-100)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 || !*unread || *room == "" || *limit < 1 || *limit > 100 {
		return fmt.Errorf("message read requires --unread, --room-id and a limit from 1 to 100")
	}
	client := run.APIClient(globals)
	page, err := client.ReadUnreadMessages(ctx, *room, *actor, *thread, *limit, *peek)
	if err != nil {
		return err
	}
	if err = command.WriteJSON(run.Stdout, page); err != nil {
		return err
	}
	if *peek || page.Token == "" {
		return nil
	}
	if err = client.AcknowledgeUnreadMessages(ctx, apitypes.UnreadAck{RoomID: page.RoomID, ParticipantID: page.ParticipantID, ThreadRootID: page.ThreadRootID, Token: page.Token}); err != nil {
		return fmt.Errorf("page printed but read acknowledgement failed; retry the same read: %w", err)
	}
	return nil
}
