package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/channelbridge/runtimebridge"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
)

const (
	channelMetadataKey     = "channel"
	channelMetadataVersion = 2
)

type participantResolver interface {
	Get(channel, id string) (apitypes.Participant, bool)
}

type outputFileSource interface {
	Conversations(agentID string) agentengine.ConversationInterface
}

// IMTranscriptStore persists final responses, failures, and tool activity in
// the built-in IM service using the participant's local channel identity.
type IMTranscriptStore struct {
	im           *im.Service
	participants participantResolver
	files        outputFileSource
}

func NewIMTranscriptStore(imService *im.Service, participants participantResolver, files outputFileSource) (*IMTranscriptStore, error) {
	if imService == nil {
		return nil, fmt.Errorf("IM service is required")
	}
	if participants == nil {
		return nil, fmt.Errorf("participant resolver is required")
	}
	return &IMTranscriptStore{im: imService, participants: participants, files: files}, nil
}

func (s *IMTranscriptStore) DeliverMessage(ctx context.Context, turn channel.TurnContext, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	_, err := s.im.DeliverMessage(im.DeliverMessageRequest{
		RoomID:       strings.TrimSpace(turn.RoomID),
		SenderID:     s.senderID(turn.ParticipantID),
		Content:      text,
		MessageID:    finalMessageID(turn),
		ThreadRootID: strings.TrimSpace(turn.ThreadRootID),
		Metadata:     transcriptMetadata("final", turn, nil),
	})
	return err
}

func (s *IMTranscriptStore) DeliverFinalMessage(
	ctx context.Context,
	turn channel.TurnContext,
	text string,
	files []agentengine.OutputFile,
) error {
	uploads, rejected := s.outputFileUploads(ctx, turn.AgentID, files)
	text = outputFileDeliveryText(text, rejected, turn.Locale)
	if strings.TrimSpace(text) == "" && len(uploads) == 0 {
		return nil
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	_, err := s.im.DeliverMessage(im.DeliverMessageRequest{
		RoomID:       strings.TrimSpace(turn.RoomID),
		SenderID:     s.senderID(turn.ParticipantID),
		Content:      text,
		MessageID:    finalMessageID(turn),
		ThreadRootID: strings.TrimSpace(turn.ThreadRootID),
		Metadata:     transcriptMetadata("final", turn, nil),
		Attachments:  uploads,
	})
	return err
}

func (s *IMTranscriptStore) outputFileUploads(
	ctx context.Context,
	agentID string,
	files []agentengine.OutputFile,
) ([]im.MessageAttachmentUpload, []string) {
	uploads := make([]im.MessageAttachmentUpload, 0, min(len(files), im.MaxAttachmentsPerMessage))
	rejected := make([]string, 0)
	var totalBytes int64
	for _, file := range files {
		name := strings.TrimSpace(file.Name)
		if name == "" {
			name = "attachment"
		}
		if len(uploads) >= im.MaxAttachmentsPerMessage || file.SizeBytes <= 0 ||
			file.SizeBytes > im.MaxAttachmentFileBytes || file.SizeBytes > im.MaxAttachmentMessageBytes-totalBytes {
			rejected = append(rejected, name)
			continue
		}
		if s == nil || s.files == nil {
			rejected = append(rejected, name)
			continue
		}
		conversation := s.files.Conversations(strings.TrimSpace(agentID))
		if conversation == nil || conversation.Files() == nil {
			rejected = append(rejected, name)
			continue
		}
		content, err := conversation.Files().Get(ctx, file.ID)
		if err != nil {
			rejected = append(rejected, name)
			continue
		}
		data, valid := readOutputFile(ctx, content, file)
		if !valid {
			rejected = append(rejected, name)
			continue
		}
		uploads = append(uploads, im.MessageAttachmentUpload{
			Name:      content.Metadata.Name,
			MediaType: content.Metadata.MediaType,
			Data:      data,
		})
		totalBytes += int64(len(data))
	}
	return uploads, rejected
}

func readOutputFile(ctx context.Context, content agentengine.FileContent, expected agentengine.OutputFile) ([]byte, bool) {
	if content.Content == nil || content.Metadata != expected.OutputFileMetadata {
		if content.Content != nil {
			_ = content.Content.Close()
		}
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(content.Content, expected.SizeBytes+1))
	closeErr := content.Content.Close()
	if err != nil || closeErr != nil || int64(len(data)) != expected.SizeBytes || contextError(ctx) != nil {
		return nil, false
	}
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:]) == expected.SHA256
}

func outputFileDeliveryText(text string, rejected []string, locale string) string {
	text = strings.TrimSpace(text)
	if len(rejected) == 0 {
		return text
	}
	names := make([]string, 0, len(rejected))
	for _, name := range rejected {
		names = append(names, fmt.Sprintf("%q", name))
	}
	warning := "Could not deliver generated file(s): " + strings.Join(names, ", ") + "."
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "zh") {
		warning = "无法发送生成的文件：" + strings.Join(names, "、") + "。"
	}
	if text == "" {
		return warning
	}
	return text + "\n\n" + warning
}

// DeliverFailure keeps Runtime internals out of the transcript and preserves
// the legacy localized error presentation and metadata contract.
func (s *IMTranscriptStore) DeliverFailure(ctx context.Context, turn channel.TurnContext, internalError string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	renderer := runtimebridge.NewTurnRenderer()
	renderer.SetLocale(turn.Locale)
	renderer.SetPromptError(internalError)
	publicError := renderer.PromptError()
	messages := renderer.FinalMessages()
	if len(messages) == 0 {
		return nil
	}
	metadata := transcriptMetadata("final", turn, nil)
	metadata = mergeCSGClawMetadata(metadata, map[string]any{
		runtimebridge.RuntimeErrorMetaKey: true,
		"error_code":                      publicError.Code,
		"presentation_version":            2,
	})
	_, err := s.im.DeliverMessage(im.DeliverMessageRequest{
		RoomID:       strings.TrimSpace(turn.RoomID),
		SenderID:     s.senderID(turn.ParticipantID),
		Content:      strings.TrimSpace(strings.Join(messages, "\n\n")),
		MessageID:    finalMessageID(turn),
		ThreadRootID: strings.TrimSpace(turn.ThreadRootID),
		Metadata:     metadata,
	})
	return err
}

func (s *IMTranscriptStore) DeliverActivity(ctx context.Context, turn channel.TurnContext, event agentengine.TurnEvent) error {
	if event.Tool == nil {
		return nil
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	text := renderToolActivity(*event.Tool)
	if text == "" {
		return nil
	}
	_, err := s.im.DeliverMessage(im.DeliverMessageRequest{
		RoomID:       strings.TrimSpace(turn.RoomID),
		SenderID:     s.senderID(turn.ParticipantID),
		Content:      text,
		MessageID:    toolMessageID(turn, *event.Tool),
		ThreadRootID: strings.TrimSpace(turn.ThreadRootID),
		Metadata:     transcriptMetadata("tool", turn, event.Tool),
	})
	return err
}

// DeliverRenderedActivity persists the same activity payload and update key as
// the legacy Codex bridge so the existing browser activity cards keep working.
func (s *IMTranscriptStore) DeliverRenderedActivity(ctx context.Context, turn channel.TurnContext, rendered ActivityDelivery) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	text := strings.TrimSpace(rendered.Text)
	if text == "" {
		return nil
	}
	senderID := s.senderID(turn.ParticipantID)
	threadRootID := strings.TrimSpace(rendered.ThreadRootID)
	if rendered.EnsureTurnRoot && threadRootID == "" {
		threadRootID = activityRootMessageID(turn)
		if _, err := s.im.DeliverMessage(im.DeliverMessageRequest{
			RoomID:    strings.TrimSpace(turn.RoomID),
			SenderID:  senderID,
			Content:   "\u200b",
			MessageID: threadRootID,
			Metadata:  withChannelMetadata(nil),
		}); err != nil {
			return err
		}
	}
	metadata := cloneMetadata(rendered.Metadata)
	if rendered.Event.Tool != nil {
		metadata = mergeMetadata(transcriptMetadata("tool", turn, rendered.Event.Tool), metadata)
	} else {
		metadata = withChannelMetadata(metadata)
	}
	_, err := s.im.DeliverMessage(im.DeliverMessageRequest{
		RoomID:       strings.TrimSpace(turn.RoomID),
		SenderID:     senderID,
		Content:      text,
		MessageID:    strings.TrimSpace(rendered.MessageID),
		ThreadRootID: threadRootID,
		Metadata:     metadata,
	})
	return err
}

// DeliverThought updates one turn-scoped commentary message. A stable ID
// prevents streaming deltas from creating an unbounded message list.
func (s *IMTranscriptStore) DeliverThought(ctx context.Context, turn channel.TurnContext, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	_, err := s.im.DeliverMessage(im.DeliverMessageRequest{
		RoomID:       strings.TrimSpace(turn.RoomID),
		SenderID:     s.senderID(turn.ParticipantID),
		Content:      text,
		MessageID:    thoughtMessageID(turn),
		ThreadRootID: strings.TrimSpace(turn.ThreadRootID),
		Metadata:     transcriptMetadata("thought", turn, nil),
	})
	return err
}

func (s *IMTranscriptStore) senderID(participantID string) string {
	participantID = strings.TrimSpace(participantID)
	if item, ok := s.participants.Get(participant.ChannelCSGClaw, participantID); ok {
		if channelUserRef := strings.TrimSpace(item.ChannelUserRef); channelUserRef != "" {
			return channelUserRef
		}
	}
	return participantID
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func renderToolActivity(tool agentengine.ToolActivity) string {
	title := strings.TrimSpace(tool.Title)
	if title == "" {
		title = strings.TrimSpace(tool.Kind)
	}
	if title == "" {
		title = "tool"
	}
	var lines []string
	lines = append(lines, "🔧 "+title)
	if status := strings.TrimSpace(tool.Status); status != "" {
		lines = append(lines, "status: "+status)
	}
	if summary := strings.TrimSpace(tool.OutputSummary); summary != "" {
		lines = append(lines, summary)
	} else if summary := strings.TrimSpace(tool.InputSummary); summary != "" {
		lines = append(lines, summary)
	}
	return strings.Join(lines, "\n")
}

func finalMessageID(turn channel.TurnContext) string {
	turnID := strings.TrimSpace(string(turn.TurnID))
	if turnID == "" {
		turnID = strings.TrimSpace(turn.SourceMessageID)
	}
	if turnID == "" {
		return ""
	}
	return turnID + "-final"
}

func thoughtMessageID(turn channel.TurnContext) string {
	turnID := strings.TrimSpace(string(turn.TurnID))
	if turnID == "" {
		turnID = strings.TrimSpace(turn.SourceMessageID)
	}
	if turnID == "" {
		return ""
	}
	return turnID + "-thought"
}

func activityRootMessageID(turn channel.TurnContext) string {
	turnID := strings.TrimSpace(string(turn.TurnID))
	if turnID == "" {
		turnID = strings.TrimSpace(turn.SourceMessageID)
	}
	if turnID == "" {
		return ""
	}
	return turnID + "-activity-root"
}

func toolMessageID(turn channel.TurnContext, tool agentengine.ToolActivity) string {
	turnID := strings.TrimSpace(string(turn.TurnID))
	toolID := strings.TrimSpace(tool.ID)
	if toolID == "" {
		toolID = strings.TrimSpace(tool.Kind)
	}
	if toolID == "" {
		toolID = "activity"
	}
	return turnID + "-tool-" + toolID
}

func transcriptMetadata(kind string, turn channel.TurnContext, tool *agentengine.ToolActivity) map[string]any {
	entry := map[string]any{
		"delivery_kind":     strings.TrimSpace(kind),
		"request_id":        strings.TrimSpace(turn.SourceMessageID),
		"source_message_id": strings.TrimSpace(turn.SourceMessageID),
	}
	if tool != nil {
		entry["tool_call_id"] = strings.TrimSpace(tool.ID)
		entry["tool_kind"] = strings.TrimSpace(tool.Kind)
		entry["tool_status"] = strings.TrimSpace(tool.Status)
	}
	metadata := map[string]any{
		"codex":    cloneMetadata(entry),
		"openclaw": cloneMetadata(entry),
	}
	return withChannelMetadata(metadata)
}

// withChannelMetadata identifies messages emitted through the reworked
// built-in channel while legacy Runtime metadata remains intact.
func withChannelMetadata(metadata map[string]any) map[string]any {
	out := cloneMetadata(metadata)
	if out == nil {
		out = make(map[string]any)
	}
	out[channelMetadataKey] = map[string]any{
		"type":    string(channel.ChannelCSGClaw),
		"version": channelMetadataVersion,
	}
	return out
}

func mergeCSGClawMetadata(metadata map[string]any, values map[string]any) map[string]any {
	out := cloneMetadata(metadata)
	if out == nil {
		out = make(map[string]any)
	}
	namespace, _ := out[runtimebridge.CSGClawMetadataKey].(map[string]any)
	namespace = cloneMetadata(namespace)
	if namespace == nil {
		namespace = make(map[string]any)
	}
	for key, value := range values {
		namespace[key] = value
	}
	out[runtimebridge.CSGClawMetadataKey] = namespace
	return out
}

func cloneMetadata(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func mergeMetadata(values ...map[string]any) map[string]any {
	var out map[string]any
	for _, value := range values {
		for key, item := range value {
			if out == nil {
				out = make(map[string]any)
			}
			out[key] = item
		}
	}
	return out
}
