package delivery

import (
	"errors"

	channeltypes "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu/transport"
)

func intentLogAttrs(intent channeltypes.DeliveryIntent, extra ...any) []any {
	attrs := []any{
		"intent_id", intent.ID,
		"binding_id", intent.BindingID,
		"turn_id", intent.TurnID,
		"sequence", intent.Sequence,
		"kind", intent.Kind,
		"status", intent.Status,
		"chat_id", intent.ChatID,
		"message_id", intent.MessageID,
		"cot_id", intent.COTID,
		"reason", intent.Reason,
		"related_id", intent.RelatedID,
		"reply_to", intent.ReplyTo,
		"thread_id", intent.ThreadID,
		"resource_id", intent.ResourceID,
		"resource_type", intent.ResourceType,
		"parent_id", intent.ParentID,
		"top_level", intent.TopLevel,
		"emoji_type", intent.EmojiType,
		"reaction_id", intent.ReactionID,
		"attempt", intent.Attempts + 1,
	}
	return append(attrs, extra...)
}

func deliveryErrorLogAttrs(intent channeltypes.DeliveryIntent, err error, extra ...any) []any {
	attrs := intentLogAttrs(intent, "error", err)
	var apiErr *transport.APIError
	if errors.As(err, &apiErr) {
		attrs = append(attrs,
			"feishu_operation", apiErr.Operation,
			"feishu_code", apiErr.Code,
			"feishu_http_status", apiErr.HTTPStatus,
			"feishu_message", apiErr.Message,
		)
	}
	return append(attrs, extra...)
}
