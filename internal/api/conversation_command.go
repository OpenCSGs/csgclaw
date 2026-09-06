package api

import (
	"context"
	"csgclaw/internal/channel/csgclaw/conv"
	"csgclaw/internal/im"
	"csgclaw/internal/slashcommand"
	"strings"
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

func (h *Handler) publishNewConversationParticipantEvent(ctx context.Context, room im.Room, sender im.User, message im.Message, reason string) []string {
	if h == nil || h.svc == nil || h.participantBridge == nil {
		return nil
	}
	var missed []string
	for _, target := range h.newConversationBridgeTargets(room, message) {
		agentID := h.runtimeAgentIDForBridgeID(target.bridgeID)
		selected, ok := h.svc.Agent(agentID)
		if !ok {
			missed = append(missed, target.bridgeID)
			continue
		}
		runtimeConfig := selected.RuntimeConfig()
		command, err := conv.ResetText(runtimeConfig.Name, runtimeConfig.Sandboxed, reason)
		if err != nil {
			missed = append(missed, target.bridgeID)
			continue
		}
		if !h.enqueueParticipantMessageEventForBridgeTarget(room, sender, message, target, command) {
			missed = append(missed, target.bridgeID)
		}
	}
	return missed
}

func (h *Handler) newConversationBridgeTargets(room im.Room, message im.Message) []participantBridgeTarget {
	if h == nil {
		return nil
	}
	notifyAllAgents := room.NotifyAllAgents && !h.isAgentSender(message.SenderID)
	targets := make([]participantBridgeTarget, 0)
	for _, target := range h.participantBridgeTargetsForRoom(room) {
		if strings.TrimSpace(target.bridgeID) == "" || target.matches(message.SenderID) || !h.isAgentSender(target.bridgeID) {
			continue
		}
		if !room.IsDirect && !notifyAllAgents && !messageMentionsBridgeTarget(message, target) {
			continue
		}
		targets = append(targets, target)
	}
	return targets
}

func messageMentionsBridgeTarget(message im.Message, target participantBridgeTarget) bool {
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
	notifyAllAgents := room.NotifyAllAgents && !isAgent(message.SenderID)
	targets := make([]string, 0)
	for _, memberID := range room.Members {
		memberID = strings.TrimSpace(memberID)
		if memberID == "" || memberID == strings.TrimSpace(message.SenderID) || !isAgent(memberID) {
			continue
		}
		if !room.IsDirect && !notifyAllAgents && !messageMentions(message, memberID) {
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
