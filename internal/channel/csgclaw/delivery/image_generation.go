package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/channel"
	"csgclaw/internal/im"
)

func (s *IMTranscriptStore) DeliverImageGeneration(ctx context.Context, turn channel.TurnContext, task contract.ImageGenerationTask) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	metadata := transcriptMetadata("image_generation", turn, nil)
	metadata["image_generation"] = task
	metadata["image_generation_context"] = turn
	digest := sha256.Sum256([]byte(turn.AgentID + "\x00" + string(turn.ConversationKey) + "\x00" + task.ID))
	messageID := "image-" + hex.EncodeToString(digest[:16])
	var uploads []im.MessageAttachmentUpload
	if task.State == "completed" && task.File != nil {
		var rejected []string
		uploads, rejected = s.outputFileUploads(ctx, turn.AgentID, []agentengine.OutputFile{*task.File})
		if len(rejected) > 0 || len(uploads) != 1 {
			return fmt.Errorf("generated image could not be attached")
		}
	}
	_, err := s.im.DeliverMessage(im.DeliverMessageRequest{
		RoomID: turn.RoomID, SenderID: s.senderID(turn.ParticipantID), MessageID: messageID,
		ThreadRootID: turn.ThreadRootID, Content: task.Prompt, Metadata: metadata, Attachments: uploads,
	})
	return err
}
