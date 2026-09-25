package ingress

import (
	"strings"

	channeltypes "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu/transport"
)

func eventLogAttrs(binding channeltypes.Binding, event transport.Event, extra ...any) []any {
	attrs := []any{
		"binding_id", binding.ID,
		"agent_id", binding.AgentID,
		"participant_id", binding.ParticipantID,
		"event_kind", event.Kind,
		"event_id", event.EventID,
	}
	switch {
	case event.Message != nil:
		message := event.Message
		attrs = append(attrs,
			"message_id", message.ID,
			"chat_id", message.ChatID,
			"chat_type", message.ChatType,
			"thread_id", message.ThreadID,
			"root_id", message.RootID,
			"parent_id", message.ParentID,
			"content_type", message.ContentType,
			"resource_count", len(message.Resources),
			"sender_id", message.Sender.OpenID,
			"mentioned_bot", message.MentionedBot,
		)
	case event.CardAction != nil:
		action := event.CardAction
		attrs = append(attrs,
			"message_id", action.MessageID,
			"chat_id", action.ChatID,
			"chat_type", action.ChatType,
			"thread_id", action.ThreadID,
			"delivery_type", action.DeliveryType,
			"operator_id", action.Operator.OpenID,
			"action_field_count", len(action.ActionValue),
			"operation_hint", controlLogOperation(firstMapString(action.ActionValue, "operation", "action", "csgclaw_action")),
			"command_hint", controlLogOperation(firstMapString(action.ActionValue, "cmd")),
		)
	case event.Comment != nil:
		comment := event.Comment
		attrs = append(attrs,
			"file_token", comment.FileToken,
			"file_type", comment.FileType,
			"comment_id", comment.CommentID,
			"reply_id", comment.ReplyID,
			"mentioned_bot", comment.MentionedBot,
		)
	}
	return append(attrs, extra...)
}

func sourceLogAttrs(source channeltypes.Source, extra ...any) []any {
	attrs := []any{
		"binding_id", source.BindingID,
		"participant_id", source.ParticipantID,
		"source_event_id", source.EventID,
		"dedup_id", source.DedupID,
		"message_id", source.MessageID,
		"chat_id", source.ChatID,
		"chat_type", source.ChatType,
		"thread_id", source.ThreadID,
		"root_id", source.RootID,
		"parent_id", source.ParentID,
	}
	return append(attrs, extra...)
}

func inboundMessageLogAttrs(message channeltypes.InboundMessage, extra ...any) []any {
	attrs := sourceLogAttrs(message.Source,
		"agent_id", message.AgentID,
		"turn_id", message.TurnID,
		"conversation_key", message.ConversationKey,
		"file_count", len(message.Files),
		"has_text", strings.TrimSpace(message.Text) != "",
	)
	return append(attrs, extra...)
}

func cardLogAttrs(card normalizedCardAction, extra ...any) []any {
	attrs := sourceLogAttrs(card.source,
		"agent_id", card.input.AgentID,
		"turn_id", firstNonEmpty(card.input.TurnID, card.turnID),
		"conversation_key", card.conversationKey,
		"operation", card.input.Action.Operation,
		"trusted", card.trusted,
	)
	return append(attrs, extra...)
}

func commentLogAttrs(comment normalizedComment, extra ...any) []any {
	attrs := sourceLogAttrs(comment.Source,
		"turn_id", comment.TurnID,
		"conversation_key", comment.ConversationKey,
		"file_token", comment.FileToken,
		"file_type", comment.FileType,
		"comment_id", comment.CommentID,
		"reply_id", comment.ReplyID,
	)
	return append(attrs, extra...)
}

func intakeItemLogAttrs(binding channeltypes.Binding, item intakeItem, extra ...any) []any {
	switch {
	case item.stop != nil:
		return inboundMessageLogAttrs(*item.stop, extra...)
	case item.message != nil:
		return inboundMessageLogAttrs(*item.message, extra...)
	case item.card != nil:
		return cardLogAttrs(*item.card, extra...)
	case item.comment != nil:
		return commentLogAttrs(*item.comment, append([]any{"agent_id", binding.AgentID}, extra...)...)
	default:
		return append([]any{
			"binding_id", binding.ID,
			"agent_id", binding.AgentID,
			"participant_id", binding.ParticipantID,
		}, extra...)
	}
}

// Log only recognized control names; arbitrary action values can contain user data.
func controlLogOperation(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "stop", "cancel", "reset", "resolve":
		return strings.ToLower(strings.TrimSpace(value))
	case "":
		return "absent"
	default:
		return "unrecognized"
	}
}
