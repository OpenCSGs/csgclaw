package api

import (
	"context"
	"log/slog"
	"strings"

	"csgclaw/internal/agent"
	csgclawchannel "csgclaw/internal/channel/csgclaw"
	"csgclaw/internal/im"
	"csgclaw/internal/slashcommand"
)

func newConversationCommandReason(content string) (string, bool, error) {
	cmd, ok, err := slashcommand.Parse(content)
	if err != nil || !ok {
		return "", ok, err
	}
	if !slashcommand.IsNewConversationCommand(cmd) {
		return "", false, nil
	}
	return strings.TrimSpace(cmd.Body), true, nil
}

func (h *Handler) publishNewConversationBotEvent(ctx context.Context, room im.Room, sender im.User, message im.Message, reason string) []string {
	if h == nil || h.svc == nil || h.botBridge == nil {
		return nil
	}
	var missed []string
	threadRootID := conversationThreadRootID(message)
	for _, target := range h.newConversationBridgeTargets(room, message) {
		botID := target.bridgeID
		agentID := h.runtimeAgentIDForBridgeID(botID)
		action, err := h.svc.NewConversationAction(ctx, agent.NewConversationRequest{
			Channel:      csgclawchannel.ChannelID,
			BotID:        agentID,
			RoomID:       room.ID,
			ThreadRootID: threadRootID,
			Reason:       reason,
		})
		if err != nil {
			slog.Warn("new conversation action failed", "channel", csgclawchannel.ChannelID, "bot_id", botID, "room_id", room.ID, "error", err)
			continue
		}
		switch action.Mode {
		case agent.NewConversationActionBotEvent:
			if action.BotEventText == "" {
				continue
			}
			if !h.enqueueBotMessageEventForBridgeTarget(room, sender, message, target, action.BotEventText) {
				missed = append(missed, botID)
			}
		case agent.NewConversationActionInternal:
			if !h.enqueueBotMessageEventForBridgeTarget(room, sender, message, target, "") {
				missed = append(missed, botID)
			}
		}
	}
	return missed
}

func (h *Handler) newConversationBridgeTargets(room im.Room, message im.Message) []botBridgeTarget {
	if h == nil {
		return nil
	}
	targets := make([]botBridgeTarget, 0)
	for _, target := range h.botBridgeTargetsForRoom(room) {
		if strings.TrimSpace(target.bridgeID) == "" || target.matches(message.SenderID) || !h.isAgentSender(target.bridgeID) {
			continue
		}
		if !room.IsDirect && !messageMentionsBridgeTarget(message, target) {
			continue
		}
		targets = append(targets, target)
	}
	return targets
}

func messageMentionsBridgeTarget(message im.Message, target botBridgeTarget) bool {
	for _, mention := range message.Mentions {
		if target.matches(mention.ID) {
			return true
		}
	}
	return false
}

func newConversationTargets(room im.Room, message im.Message, isAgent func(string) bool) []string {
	if isAgent == nil {
		return nil
	}
	targets := make([]string, 0)
	for _, memberID := range room.Members {
		memberID = strings.TrimSpace(memberID)
		if memberID == "" || memberID == strings.TrimSpace(message.SenderID) || !isAgent(memberID) {
			continue
		}
		if !room.IsDirect && !messageMentions(message, memberID) {
			continue
		}
		targets = append(targets, memberID)
	}
	return targets
}

func messageMentions(message im.Message, userID string) bool {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false
	}
	for _, mention := range message.Mentions {
		if strings.TrimSpace(mention.ID) == userID {
			return true
		}
	}
	return im.HasMentionTagForUser(message.Content, userID)
}

func conversationThreadRootID(message im.Message) string {
	if message.RelatesTo == nil || strings.TrimSpace(message.RelatesTo.RelType) != im.RelationTypeThread {
		return ""
	}
	return strings.TrimSpace(message.RelatesTo.EventID)
}
