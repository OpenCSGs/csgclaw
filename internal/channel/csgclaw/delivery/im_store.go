package delivery

import (
	"context"
	"fmt"
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

// IMTranscriptStore persists final responses, failures, and tool activity in
// the built-in IM service using the participant's local channel identity.
type IMTranscriptStore struct {
	im           *im.Service
	participants participantResolver
}

func NewIMTranscriptStore(imService *im.Service, participants participantResolver) (*IMTranscriptStore, error) {
	if imService == nil {
		return nil, fmt.Errorf("IM service is required")
	}
	if participants == nil {
		return nil, fmt.Errorf("participant resolver is required")
	}
	return &IMTranscriptStore{im: imService, participants: participants}, nil
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
