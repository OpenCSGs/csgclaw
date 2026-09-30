package ingress

import (
	"fmt"
	"strings"

	channeltypes "csgclaw/internal/channel"
	feishuctx "csgclaw/internal/channel/feishu/context"
	"csgclaw/internal/channel/feishu/transport"
)

func normalizeMessage(binding channeltypes.Binding, event transport.Event, bot transport.Identity) (channeltypes.InboundMessage, bool, error) {
	message := event.Message
	if message == nil {
		return channeltypes.InboundMessage{}, false, fmt.Errorf("Feishu message payload is required")
	}
	eventID := firstNonEmpty(event.EventID, message.ID)
	if eventID == "" || strings.TrimSpace(message.ID) == "" || strings.TrimSpace(message.ChatID) == "" {
		return channeltypes.InboundMessage{}, false, fmt.Errorf("Feishu event, message, and chat IDs are required")
	}
	mentions := make([]feishuctx.Mention, 0, len(message.Mentions))
	for _, mention := range message.Mentions {
		mentions = append(mentions, feishuctx.Mention{Key: mention.Key, OpenID: mention.OpenID})
	}
	if strings.TrimSpace(bot.OpenID) != "" && strings.TrimSpace(message.Sender.OpenID) == strings.TrimSpace(bot.OpenID) {
		return channeltypes.InboundMessage{}, false, nil
	}
	if !feishuctx.AcceptMessage(string(message.ChatType), message.MentionedBot, bot.OpenID, mentions) {
		return channeltypes.InboundMessage{}, false, nil
	}
	text := message.Text
	resources := append([]transport.Resource(nil), message.Resources...)
	if strings.EqualFold(strings.TrimSpace(message.ContentType), "interactive") && len(message.RawContent) > 0 {
		// Preserve the previous channel behavior: interactive messages that are
		// not card actions remain visible to the Agent as their raw JSON body.
		text = string(message.RawContent)
	}
	text = feishuctx.StripBotMention(text, bot.OpenID, mentions)
	files := make([]channeltypes.InboundFile, 0, len(resources))
	for _, resource := range resources {
		if strings.TrimSpace(resource.ID) == "" {
			continue
		}
		files = append(files, channeltypes.InboundFile{
			Kind:      strings.TrimSpace(resource.Kind),
			ID:        strings.TrimSpace(resource.ID),
			Name:      strings.TrimSpace(resource.Name),
			SizeBytes: resource.Size,
			URL:       strings.TrimSpace(resource.URL),
		})
	}
	if strings.TrimSpace(text) == "" && len(files) == 0 {
		return channeltypes.InboundMessage{}, false, nil
	}
	conversationKey := feishuctx.ChatConversationKey(binding.ID, message.ChatID, message.ThreadID)
	// Message ID, unlike the Feishu event ID, is shared by WebSocket delivery
	// and the local bot-to-agent handoff path.
	turnID := feishuctx.TurnID(binding.ID, message.ID, message.ID)
	return channeltypes.InboundMessage{
		Source: channeltypes.Source{
			SenderID:      strings.TrimSpace(message.Sender.OpenID),
			Channel:       binding.Channel,
			BindingID:     binding.ID,
			ParticipantID: binding.ParticipantID,
			EventID:       eventID,
			DedupID:       strings.TrimSpace(message.ID),
			MessageID:     strings.TrimSpace(message.ID),
			ChatID:        strings.TrimSpace(message.ChatID),
			ChatType:      strings.TrimSpace(string(message.ChatType)),
			ThreadID:      strings.TrimSpace(message.ThreadID),
			RootID:        strings.TrimSpace(message.RootID),
			ParentID:      strings.TrimSpace(message.ParentID),
		},
		AgentID:         binding.AgentID,
		ConversationKey: conversationKey,
		TurnID:          turnID,
		Text:            text,
		Files:           files,
	}, true, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
