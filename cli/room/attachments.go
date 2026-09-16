package room

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"csgclaw/cli/command"
	"csgclaw/internal/apitypes"
)

func (c cmd) runAttachments(ctx context.Context, run *command.Context, args []string, globals command.GlobalOptions) error {
	if len(args) == 0 || command.IsHelpArg(args[0]) {
		fmt.Fprintln(run.Stderr, "Usage: "+run.Program+" room attachments <list|download> [flags]")
		return flag.ErrHelp
	}
	action := args[0]
	if action != "list" && action != "download" {
		return fmt.Errorf("unknown attachments subcommand %q", action)
	}
	fs := run.NewFlagSet("room attachments "+action, run.Program+" room attachments "+action+" [flags]", "Find and download files published to a CSGClaw room.")
	roomID := fs.String("room-id", "", "room id")
	var query, messageID, attachmentID, output string
	var from, limit int
	if action == "list" {
		fs.StringVar(&query, "query", "", "case-insensitive filename substring")
		fs.StringVar(&messageID, "message-id", "", "source message id")
		fs.IntVar(&from, "from", 0, "pagination offset")
		fs.IntVar(&limit, "limit", 50, "page size (1-200)")
	} else {
		fs.StringVar(&attachmentID, "attachment-id", "", "attachment id from the room listing")
		fs.StringVar(&output, "output", "", "new local file path; existing files are never replaced")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*roomID) == "" || len(fs.Args()) != 0 {
		return fmt.Errorf("room-id is required; positional arguments are not accepted")
	}
	client := run.APIClient(globals)
	if action == "list" {
		if from < 0 || limit < 1 || limit > 200 {
			return fmt.Errorf("from must be nonnegative and limit must be between 1 and 200")
		}
		result, err := client.ListRoomAttachments(ctx, *roomID, apitypes.RoomAttachmentListOptions{Query: query, MessageID: messageID, From: from, Limit: limit})
		if err != nil {
			return err
		}
		return command.WriteJSON(run.Stdout, result)
	}
	if strings.TrimSpace(attachmentID) == "" || strings.TrimSpace(output) == "" {
		return fmt.Errorf("attachment-id and output are required")
	}
	path, err := client.DownloadRoomAttachment(ctx, *roomID, attachmentID, output)
	if err != nil {
		return err
	}
	return command.WriteJSON(run.Stdout, map[string]string{"attachment_id": attachmentID, "path": path})
}
