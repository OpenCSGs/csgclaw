package delivery

import (
	"context"
	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	channelrender "csgclaw/internal/channel/csgclaw/render"
	"csgclaw/internal/im"
	"encoding/json"
	"strings"
)

func (s *IMTranscriptStore) DeliverTurnProgress(ctx context.Context, turn channel.TurnContext, progress activity.TurnProgress, answer string, persist bool, files []agentengine.OutputFile) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	kind := "activity"
	if progress.Status == "succeeded" && (strings.TrimSpace(answer) != "" || len(files) > 0) {
		kind = "final"
	}
	internalError := strings.TrimSpace(progress.Error)
	var publicError channelrender.PublicPromptError
	if progress.Error != "" {
		renderer := channelrender.NewTurnRenderer()
		renderer.SetLocale(turn.Locale)
		renderer.SetPromptError(progress.Error)
		publicError = renderer.PromptError()
		progress.Error = strings.Join(renderer.FinalMessages(), "\n\n")
	}
	// Freeze nested slices before publishing to the IM bus and history readers.
	raw, err := json.Marshal(progress)
	if err != nil {
		return err
	}
	var snapshot map[string]any
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		return err
	}
	metadata := mergeCSGClawMetadata(transcriptMetadata(kind, turn, nil), map[string]any{"delivery_kind": kind, "turn_id": string(turn.TurnID), "turn_progress": snapshot})
	if internalError != "" {
		metadata = mergeCSGClawMetadata(metadata, map[string]any{channelrender.RuntimeErrorMetaKey: true, channelrender.RuntimeErrorDetailKey: internalError, "error_code": publicError.Code, "presentation_version": 2})
	}
	uploads, rejected := s.outputFileSources(ctx, turn.AgentID, files)
	defer closeOutputFileSources(uploads)
	answer = outputFileDeliveryText(answer, rejected, turn.Locale)
	if strings.TrimSpace(answer) == "" {
		answer = "\u200b"
	}
	_, err = s.im.DeliverMessage(im.DeliverMessageRequest{RoomID: turn.RoomID, SenderID: s.senderID(turn.ParticipantID), MessageID: finalMessageID(turn), ThreadRootID: turn.ThreadRootID, Content: answer, Metadata: metadata, AttachmentSources: uploads, Transient: !persist})
	return err
}
