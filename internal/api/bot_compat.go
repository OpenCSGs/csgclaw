package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
)

const (
	botReplayWindow      = 30 * time.Minute
	botHeartbeatInterval = 15 * time.Second
)

func (h *Handler) PublishBotEvent(evt im.Event) {
	if h.botBridge == nil || h.im == nil {
		return
	}
	if evt.Type != im.EventTypeMessageCreated || evt.Message == nil || evt.Sender == nil {
		return
	}

	room, ok := h.im.Room(evt.RoomID)
	if !ok {
		return
	}
	if reason, ok, err := newConversationCommandReason(evt.Message.Content); err != nil {
		slog.Warn("parse new conversation command failed", "room_id", evt.RoomID, "message_id", evt.Message.ID, "error", err)
	} else if ok {
		missed := h.publishNewConversationBotEvent(context.Background(), room, *evt.Sender, *evt.Message, reason)
		h.reconnectMissedBotAgents(evt.Sender.ID, missed)
		return
	}
	missed := h.publishMessageBotEvent(room, *evt.Sender, *evt.Message)
	h.reconnectMissedBotAgents(evt.Sender.ID, missed)
}

type botBridgeTarget struct {
	bridgeID string
	aliases  []string
}

func newBotBridgeTarget(bridgeID string, aliases ...string) botBridgeTarget {
	bridgeID = strings.TrimSpace(bridgeID)
	if bridgeID == "" {
		return botBridgeTarget{}
	}
	seen := map[string]struct{}{bridgeID: {}}
	out := botBridgeTarget{
		bridgeID: bridgeID,
		aliases:  []string{bridgeID},
	}
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		if _, ok := seen[alias]; ok {
			continue
		}
		seen[alias] = struct{}{}
		out.aliases = append(out.aliases, alias)
	}
	return out
}

func (t botBridgeTarget) matches(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, alias := range t.aliases {
		if strings.TrimSpace(alias) == id {
			return true
		}
	}
	return false
}

func (h *Handler) publishMessageBotEvent(room im.Room, sender im.User, message im.Message) []string {
	var missed []string
	for _, target := range h.botBridgeTargetsForRoom(room) {
		if !h.enqueueBotMessageEventForBridgeTarget(room, sender, message, target, "") {
			missed = append(missed, target.bridgeID)
		}
	}
	return missed
}

func (h *Handler) enqueueBotMessageEventForBridgeID(room im.Room, sender im.User, message im.Message, bridgeID string, text string) bool {
	return h.enqueueBotMessageEventForBridgeTarget(room, sender, message, h.botBridgeTargetForBridgeID(bridgeID), text)
}

func (h *Handler) enqueueBotMessageEventForBridgeTarget(room im.Room, sender im.User, message im.Message, target botBridgeTarget, text string) bool {
	if h == nil || h.botBridge == nil || strings.TrimSpace(target.bridgeID) == "" {
		return true
	}
	if target.matches(message.SenderID) {
		return true
	}
	deliveryRoom := roomForBotBridgeTarget(room, target)
	deliveryMessage := messageForBotBridgeTarget(message, target)
	if strings.TrimSpace(text) != "" {
		return h.botBridge.EnqueueMessageEventWithText(deliveryRoom, sender, deliveryMessage, target.bridgeID, text)
	}
	return h.botBridge.EnqueueMessageEvent(deliveryRoom, sender, deliveryMessage, target.bridgeID)
}

func (h *Handler) botBridgeTargetsForRoom(room im.Room) []botBridgeTarget {
	targets := make([]botBridgeTarget, 0, len(room.Members))
	seen := make(map[string]struct{}, len(room.Members))
	for _, memberID := range room.Members {
		target := h.botBridgeTargetForRoomMember(memberID)
		if strings.TrimSpace(target.bridgeID) == "" {
			continue
		}
		if _, ok := seen[target.bridgeID]; ok {
			continue
		}
		seen[target.bridgeID] = struct{}{}
		targets = append(targets, target)
	}
	return targets
}

func (h *Handler) botBridgeTargetForRoomMember(memberID string) botBridgeTarget {
	memberID = strings.TrimSpace(memberID)
	if memberID == "" {
		return botBridgeTarget{}
	}
	if h != nil && h.participant != nil {
		if item, ok := h.participant.Get(participant.ChannelCSGClaw, memberID); ok && isCSGClawAgentParticipant(item) {
			return botBridgeTargetForParticipant(item, memberID)
		}
		for _, item := range h.participant.List(participant.ListOptions{Channel: participant.ChannelCSGClaw}) {
			if !isCSGClawAgentParticipant(item) || !participantMatchesIdentity(item, memberID) {
				continue
			}
			return botBridgeTargetForParticipant(item, memberID)
		}
	}
	return newBotBridgeTarget(memberID, memberID)
}

func (h *Handler) botBridgeTargetForBridgeID(bridgeID string) botBridgeTarget {
	bridgeID = strings.TrimSpace(bridgeID)
	if bridgeID == "" {
		return botBridgeTarget{}
	}
	if h != nil && h.participant != nil {
		if item, ok := h.participant.Get(participant.ChannelCSGClaw, bridgeID); ok && isCSGClawAgentParticipant(item) {
			return botBridgeTargetForParticipant(item, bridgeID)
		}
		for _, item := range h.participant.List(participant.ListOptions{Channel: participant.ChannelCSGClaw}) {
			if !isCSGClawAgentParticipant(item) || !participantMatchesIdentity(item, bridgeID) {
				continue
			}
			return botBridgeTargetForParticipant(item, bridgeID)
		}
	}
	if bridgeID == agent.ManagerParticipantID {
		return newBotBridgeTarget(agent.ManagerParticipantID, agent.ManagerUserID)
	}
	return newBotBridgeTarget(bridgeID, bridgeID)
}

func botBridgeTargetForParticipant(item apitypes.Participant, aliases ...string) botBridgeTarget {
	allAliases := []string{item.ID, item.ChannelUserRef, item.AgentID}
	allAliases = append(allAliases, aliases...)
	return newBotBridgeTarget(item.ID, allAliases...)
}

func isCSGClawAgentParticipant(item apitypes.Participant) bool {
	return strings.TrimSpace(item.ID) != "" &&
		strings.EqualFold(strings.TrimSpace(item.Channel), participant.ChannelCSGClaw) &&
		strings.EqualFold(strings.TrimSpace(item.Type), participant.TypeAgent)
}

func participantMatchesIdentity(item apitypes.Participant, id string) bool {
	id = strings.TrimSpace(id)
	return id != "" && (strings.TrimSpace(item.ID) == id ||
		strings.TrimSpace(item.ChannelUserRef) == id ||
		strings.TrimSpace(item.AgentID) == id)
}

func roomForBotBridgeTarget(room im.Room, target botBridgeTarget) im.Room {
	if strings.TrimSpace(target.bridgeID) == "" {
		return room
	}
	out := room
	out.Members = make([]string, 0, len(room.Members))
	seen := make(map[string]struct{}, len(room.Members))
	for _, memberID := range room.Members {
		deliveryID := strings.TrimSpace(memberID)
		if target.matches(deliveryID) {
			deliveryID = target.bridgeID
		}
		if deliveryID == "" {
			continue
		}
		if _, ok := seen[deliveryID]; ok {
			continue
		}
		seen[deliveryID] = struct{}{}
		out.Members = append(out.Members, deliveryID)
	}
	return out
}

func messageForBotBridgeTarget(message im.Message, target botBridgeTarget) im.Message {
	if strings.TrimSpace(target.bridgeID) == "" || len(target.aliases) == 0 {
		return message
	}
	out := message
	if len(message.Mentions) > 0 {
		out.Mentions = append([]im.Mention(nil), message.Mentions...)
		for idx := range out.Mentions {
			if target.matches(out.Mentions[idx].ID) {
				out.Mentions[idx].ID = target.bridgeID
			}
		}
	}
	out.Content = contentForBotBridgeTarget(message.Content, target)
	return out
}

func contentForBotBridgeTarget(content string, target botBridgeTarget) string {
	if content == "" {
		return content
	}
	for _, alias := range target.aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || alias == target.bridgeID {
			continue
		}
		content = strings.ReplaceAll(content, fmt.Sprintf(`<at user_id="%s">`, alias), fmt.Sprintf(`<at user_id="%s">`, target.bridgeID))
	}
	return content
}

func (h *Handler) handleBotEvents(w http.ResponseWriter, r *http.Request, botID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	events, cancel := h.botBridge.Subscribe(botID)
	defer func() {
		cancel()
		h.requeueBufferedBotEvents(botID, events)
	}()
	controller := http.NewResponseController(w)

	if _, err := io.WriteString(w, ": connected\n\n"); err != nil {
		return
	}
	if err := flushBotSSE(controller, flusher); err != nil {
		return
	}
	h.replayRecentBotMessages(botID, r.Header.Get("Last-Event-ID"))
	heartbeat := time.NewTicker(botHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if err := writeBotSSEComment(w, controller, flusher, "heartbeat"); err != nil {
				return
			}
		case evt, ok := <-events:
			if !ok {
				return
			}
			if err := writeBotSSEEvent(w, controller, flusher, evt); err != nil {
				h.botBridge.Requeue(botID, evt)
				return
			}
			h.botBridge.Ack(botID, evt.MessageID)
		}
	}
}

func writeBotSSEEvent(w http.ResponseWriter, controller *http.ResponseController, fallback http.Flusher, evt im.BotEvent) error {
	data, err := evt.MarshalJSONLine()
	if err != nil {
		return err
	}
	if id := botSSEID(evt.MessageID); id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", data); err != nil {
		return err
	}
	return flushBotSSE(controller, fallback)
}

func writeBotSSEComment(w http.ResponseWriter, controller *http.ResponseController, fallback http.Flusher, comment string) error {
	if _, err := fmt.Fprintf(w, ": %s\n\n", comment); err != nil {
		return err
	}
	return flushBotSSE(controller, fallback)
}

func flushBotSSE(controller *http.ResponseController, fallback http.Flusher) error {
	if controller != nil {
		if err := controller.Flush(); err == nil {
			return nil
		} else if !errors.Is(err, http.ErrNotSupported) {
			return err
		}
	}
	if fallback == nil {
		return nil
	}
	fallback.Flush()
	return nil
}

func (h *Handler) requeueBufferedBotEvents(botID string, events <-chan im.BotEvent) {
	if h == nil || h.botBridge == nil {
		return
	}
	for evt := range events {
		h.botBridge.Requeue(botID, evt)
	}
}

func (h *Handler) replayRecentBotMessages(botID, lastEventID string) {
	if h == nil || h.im == nil || h.botBridge == nil {
		return
	}
	rooms := h.im.ListRoomsWithOptions(im.ListMessagesOptions{IncludeThreadReplies: true})
	cutoff := time.Now().UTC().Add(-botReplayWindow)
	replayAfter, hasReplayCursor := replayCursor(rooms, lastEventID)
	for _, room := range rooms {
		for idx, message := range room.Messages {
			if !message.CreatedAt.IsZero() && message.CreatedAt.Before(cutoff) {
				continue
			}
			if hasReplayCursor && isAtOrBeforeReplayCursor(message, lastEventID, replayAfter) {
				continue
			}
			if h.isAgentSender(message.SenderID) {
				continue
			}
			if h.hasLaterMessageFromBridgeTarget(room.Messages[idx+1:], botID) {
				continue
			}
			sender, ok := h.im.User(message.SenderID)
			if !ok {
				continue
			}
			if reason, ok, err := newConversationCommandReason(message.Content); err != nil {
				slog.Warn("parse new conversation command failed", "bot_id", botID, "message_id", message.ID, "error", err)
				h.enqueueBotMessageEventForBridgeID(room, sender, message, botID, "")
				continue
			} else if ok {
				missed := h.publishNewConversationBotEvent(context.Background(), room, sender, message, reason)
				h.reconnectMissedBotAgents(sender.ID, missed)
				continue
			}
			// Route replay through the bridge so the stable message ID remains the
			// dedupe key for events already delivered live or drained from pending.
			h.enqueueBotMessageEventForBridgeID(room, sender, message, botID, "")
		}
	}
}

func (h *Handler) hasLaterMessageFromBridgeTarget(messages []im.Message, bridgeID string) bool {
	target := h.botBridgeTargetForBridgeID(bridgeID)
	for _, message := range messages {
		if target.matches(message.SenderID) {
			return true
		}
	}
	return false
}

func replayCursor(rooms []im.Room, lastEventID string) (time.Time, bool) {
	lastEventID = strings.TrimSpace(lastEventID)
	if lastEventID == "" {
		return time.Time{}, false
	}
	for _, room := range rooms {
		for _, message := range room.Messages {
			if message.ID == lastEventID {
				return message.CreatedAt, true
			}
		}
	}
	return time.Time{}, false
}

func isAtOrBeforeReplayCursor(message im.Message, lastEventID string, replayAfter time.Time) bool {
	if message.ID == strings.TrimSpace(lastEventID) {
		return true
	}
	if replayAfter.IsZero() || message.CreatedAt.IsZero() {
		return false
	}
	return !message.CreatedAt.After(replayAfter)
}

func botSSEID(messageID string) string {
	messageID = strings.TrimSpace(messageID)
	messageID = strings.ReplaceAll(messageID, "\r", "")
	messageID = strings.ReplaceAll(messageID, "\n", "")
	return messageID
}

func (h *Handler) reconnectMissedBotAgents(senderID string, botIDs []string) {
	if h == nil || h.svc == nil || h.isAgentSender(senderID) || len(botIDs) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(botIDs))
	for _, botID := range botIDs {
		agentID := h.runtimeAgentIDForBridgeID(botID)
		if agentID == "" {
			continue
		}
		if _, ok := seen[agentID]; ok {
			continue
		}
		seen[agentID] = struct{}{}
		if _, ok := h.svc.Agent(agentID); !ok {
			continue
		}
		go h.recoverMissedBotDelivery(agentID)
	}
}

func (h *Handler) recoverMissedBotDelivery(botID string) {
	if h == nil || h.svc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	view, err := h.svc.RuntimeView(ctx, botID)
	if err != nil {
		slog.Warn("bot delivery recovery failed", "agent_id", botID, "error", err)
		return
	}
	if err := h.applyBotDeliveryRecoveryPolicy(ctx, view); err != nil {
		slog.Warn("bot delivery recovery failed", "agent_id", botID, "runtime_kind", view.RuntimeKind, "state", view.State, "error", err)
	}
}

func (h *Handler) applyBotDeliveryRecoveryPolicy(ctx context.Context, view agent.RuntimeView) error {
	if h == nil || h.svc == nil {
		return nil
	}
	switch view.State {
	case agentruntime.StateCreated, agentruntime.StateStopped, agentruntime.StateExited, agentruntime.StateFailed:
		_, err := h.svc.Start(ctx, view.AgentID)
		return err
	case agentruntime.StateRunning:
		_, err := h.svc.Start(ctx, view.AgentID)
		return err
	case "", agentruntime.StateUnknown:
		fallthrough
	default:
		_, err := h.svc.Recreate(ctx, view.AgentID)
		return err
	}
}

func (h *Handler) isAgentSender(senderID string) bool {
	if h == nil || h.svc == nil {
		return false
	}
	_, ok := h.svc.Agent(h.runtimeAgentIDForBridgeID(senderID))
	return ok
}

func (h *Handler) runtimeAgentIDForBridgeID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if id == agent.ManagerParticipantID {
		return agent.ManagerUserID
	}
	if h != nil && h.participant != nil {
		if item, ok := h.participant.Get(participant.ChannelCSGClaw, id); ok {
			if agentID := strings.TrimSpace(item.AgentID); agentID != "" {
				return agentID
			}
		}
	}
	return id
}

func hasLaterMessageFrom(messages []im.Message, senderID string) bool {
	senderID = strings.TrimSpace(senderID)
	if senderID == "" {
		return false
	}
	for _, message := range messages {
		if message.SenderID == senderID {
			return true
		}
	}
	return false
}

func (h *Handler) handleBotSendMessage(w http.ResponseWriter, r *http.Request, botID string) {
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}
	var req im.BotSendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	roomID := req.ResolvedRoomID()
	text := req.ResolvedText()
	threadRootID := req.ResolvedThreadRootID()

	message, err := h.im.DeliverMessage(im.DeliverMessageRequest{
		RoomID:       roomID,
		SenderID:     botID,
		Content:      text,
		MessageID:    req.MessageID,
		ThreadRootID: threadRootID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.publishMessageCreated(roomID, botID, message)
	h.publishThreadUpdated(roomID, message)
	writeJSON(w, http.StatusOK, map[string]string{"message_id": message.ID})
}
