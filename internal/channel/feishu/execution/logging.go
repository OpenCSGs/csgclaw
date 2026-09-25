package execution

import (
	"strings"

	"csgclaw/internal/agentengine"
	channeltypes "csgclaw/internal/channel"
)

func messageLogAttrs(message channeltypes.InboundMessage, extra ...any) []any {
	attrs := []any{
		"binding_id", message.Source.BindingID,
		"agent_id", message.AgentID,
		"participant_id", message.Source.ParticipantID,
		"turn_id", message.TurnID,
		"conversation_key", message.ConversationKey,
		"source_event_id", message.Source.EventID,
		"dedup_id", message.Source.DedupID,
		"message_id", message.Source.MessageID,
		"chat_id", message.Source.ChatID,
		"chat_type", message.Source.ChatType,
		"thread_id", message.Source.ThreadID,
		"root_id", message.Source.RootID,
		"parent_id", message.Source.ParentID,
		"file_count", len(message.Files),
		"has_text", strings.TrimSpace(message.Text) != "",
	}
	if target := message.ReplyTarget; target != nil {
		attrs = append(attrs,
			"reply_target_kind", target.Kind,
			"resource_id", target.ResourceID,
			"resource_type", target.ResourceType,
			"reply_parent_id", target.ParentID,
			"reply_top_level", target.TopLevel,
		)
	}
	return append(attrs, extra...)
}

func resultLogAttrs(result agentengine.TurnResult, extra ...any) []any {
	attrs := []any{
		"status", result.Status,
		"dispatched", result.Dispatched,
		"output_bytes", len(result.Output),
		"output_file_count", len(result.Files),
	}
	if result.Error != nil {
		attrs = append(attrs,
			"error_code", result.Error.Code,
			"error", result.Error,
		)
	}
	return append(attrs, extra...)
}
