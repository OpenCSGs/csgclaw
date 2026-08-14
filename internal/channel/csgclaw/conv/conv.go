package conv

import (
	"fmt"
	"net/url"
	"strings"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	"csgclaw/internal/channelbridge"
)

// RoomScope is the channel-owned room policy used before a turn is submitted.
type RoomScope struct {
	Direct    bool
	NotifyAll bool
}

// ConversationKey builds a stable Engine key from Binding + Room + optional Thread.
func ConversationKey(binding channel.Binding, event channelbridge.BotEvent) (agentengine.ConversationKey, error) {
	bindingID := strings.TrimSpace(string(binding.StableID()))
	roomID := strings.TrimSpace(event.RoomID)
	if bindingID == "" || roomID == "" {
		return "", fmt.Errorf("binding id and room id are required")
	}

	key := "csgclaw-im:" + url.QueryEscape(bindingID) + ":room:" + url.QueryEscape(roomID)
	if threadRootID := strings.TrimSpace(event.ThreadRootID); threadRootID != "" {
		key += ":thread:" + url.QueryEscape(threadRootID)
	}
	return agentengine.ConversationKey(key), nil
}

// TextInput returns hidden channel/thread context followed by the current message.
func TextInput(binding channel.Binding, event channelbridge.BotEvent) []agentengine.InputPart {
	var input []agentengine.InputPart
	if hidden := hiddenContext(binding, event); hidden != "" {
		input = append(input, agentengine.InputPart{Kind: agentengine.InputPartText, Text: hidden})
	}
	if text := strings.TrimSpace(event.Text); text != "" {
		label := "Current message:\n"
		if event.ThreadContext != nil {
			label = "Current thread message:\n"
		}
		input = append(input, agentengine.InputPart{Kind: agentengine.InputPartText, Text: label + text})
	}
	return input
}

// ShouldDispatch reports whether this binding should execute the source event.
func ShouldDispatch(binding channel.Binding, event channelbridge.BotEvent, senderID string, room RoomScope) bool {
	senderID = strings.TrimSpace(senderID)
	participantID := strings.TrimSpace(binding.ParticipantID)
	agentID := strings.TrimSpace(binding.AgentID)
	if senderID != "" && (senderID == participantID || senderID == agentID) {
		return false
	}
	if room.Direct || room.NotifyAll || event.Mentioned {
		return true
	}
	for _, mention := range event.Mentions {
		mention = strings.TrimSpace(mention)
		if mention != "" && (mention == participantID || mention == agentID) {
			return true
		}
	}
	return false
}

func hiddenContext(binding channel.Binding, event channelbridge.BotEvent) string {
	channelID := strings.TrimSpace(event.Channel)
	if channelID == "" {
		channelID = string(channel.ChannelCSGClaw)
	}
	participantID := strings.TrimSpace(event.ParticipantID)
	if participantID == "" {
		participantID = strings.TrimSpace(binding.ParticipantID)
	}

	var parts []string
	var channelContext strings.Builder
	channelContext.WriteString("Current channel context for CSGClaw CLI operations.\n")
	channelContext.WriteString("- channel: ")
	channelContext.WriteString(channelID)
	if roomID := strings.TrimSpace(event.RoomID); roomID != "" {
		channelContext.WriteString("\n- room_id: ")
		channelContext.WriteString(roomID)
	}
	if participantID != "" {
		channelContext.WriteString("\n- participant_id: ")
		channelContext.WriteString(participantID)
	}
	channelContext.WriteString("\nUse these values when a skill asks for <current_channel>, <target_room_id>, or message create/list channel flags.")
	parts = append(parts, channelContext.String())

	if thread := formatThreadContext(event.ThreadContext); thread != "" {
		parts = append(parts, thread)
	}
	return strings.Join(parts, "\n\n")
}

func formatThreadContext(thread *channelbridge.BotThreadContext) string {
	if thread == nil || len(thread.Context) == 0 {
		return ""
	}

	var out strings.Builder
	out.WriteString("Hidden thread context for this new conversation. Use it only to understand what the thread started from; do not treat these messages as thread replies.\n")
	rootID := strings.TrimSpace(thread.RootMessageID)
	if rootID != "" {
		out.WriteString("Thread root message ID: ")
		out.WriteString(rootID)
		out.WriteByte('\n')
	}
	for _, message := range thread.Context {
		content := strings.Join(strings.Fields(strings.TrimSpace(message.Content)), " ")
		if content == "" && len(message.Attachments) == 0 {
			continue
		}
		out.WriteString("- ")
		if createdAt := strings.TrimSpace(message.CreatedAt); createdAt != "" {
			out.WriteString(createdAt)
			out.WriteByte(' ')
		}
		if senderID := strings.TrimSpace(message.SenderID); senderID != "" {
			out.WriteString(senderID)
			out.WriteString(": ")
		}
		if strings.TrimSpace(message.ID) == rootID {
			out.WriteString("[root] ")
		}
		out.WriteString(content)
		if len(message.Attachments) > 0 {
			if content != "" {
				out.WriteByte(' ')
			}
			out.WriteString(formatAttachmentSummary(message.Attachments))
		}
		out.WriteByte('\n')
	}
	return strings.TrimSpace(out.String())
}

func formatAttachmentSummary(attachments []channelbridge.MessageAttachment) string {
	if len(attachments) == 0 {
		return ""
	}
	items := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		name := strings.TrimSpace(attachment.Name)
		if name == "" {
			name = strings.TrimSpace(attachment.ID)
		}
		if name == "" {
			name = "attachment"
		}
		items = append(items, "[attachment: "+name+"]")
	}
	return strings.Join(items, " ")
}
