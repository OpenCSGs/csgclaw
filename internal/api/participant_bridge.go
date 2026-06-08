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
	participantReplayWindow      = 30 * time.Minute
	participantHeartbeatInterval = 15 * time.Second
)

func (h *Handler) PublishParticipantEvent(evt im.Event) {
	if h.participantBridge == nil || h.im == nil {
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
		missed := h.publishNewConversationParticipantEvent(context.Background(), room, *evt.Sender, *evt.Message, reason)
		h.reconnectMissedParticipantAgents(evt.Sender.ID, missed)
		return
	}
	missed := h.publishMessageParticipantEvent(room, *evt.Sender, *evt.Message)
	h.reconnectMissedParticipantAgents(evt.Sender.ID, missed)
}

type participantBridgeTarget struct {
	bridgeID string
	aliases  []string
}

func newParticipantBridgeTarget(bridgeID string, aliases ...string) participantBridgeTarget {
	bridgeID = strings.TrimSpace(bridgeID)
	if bridgeID == "" {
		return participantBridgeTarget{}
	}
	seen := map[string]struct{}{bridgeID: {}}
	out := participantBridgeTarget{
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

func (t participantBridgeTarget) matches(id string) bool {
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

func (h *Handler) publishMessageParticipantEvent(room im.Room, sender im.User, message im.Message) []string {
	var missed []string
	for _, target := range h.participantBridgeTargetsForRoom(room) {
		if !h.enqueueParticipantMessageEventForBridgeTarget(room, sender, message, target, "") {
			missed = append(missed, target.bridgeID)
		}
	}
	return missed
}

func (h *Handler) enqueueParticipantMessageEventForBridgeID(room im.Room, sender im.User, message im.Message, bridgeID string, text string) bool {
	return h.enqueueParticipantMessageEventForBridgeTarget(room, sender, message, h.participantBridgeTargetForBridgeID(bridgeID), text)
}

func (h *Handler) enqueueParticipantMessageEventForBridgeTarget(room im.Room, sender im.User, message im.Message, target participantBridgeTarget, text string) bool {
	if h == nil || h.participantBridge == nil || strings.TrimSpace(target.bridgeID) == "" {
		return true
	}
	if target.matches(message.SenderID) {
		return true
	}
	deliveryRoom := roomForParticipantBridgeTarget(room, target)
	deliveryMessage := messageForParticipantBridgeTarget(message, target)
	if strings.TrimSpace(text) != "" {
		return h.participantBridge.EnqueueMessageEventWithText(deliveryRoom, sender, deliveryMessage, target.bridgeID, text)
	}
	return h.participantBridge.EnqueueMessageEvent(deliveryRoom, sender, deliveryMessage, target.bridgeID)
}

func (h *Handler) participantBridgeTargetsForRoom(room im.Room) []participantBridgeTarget {
	targets := make([]participantBridgeTarget, 0, len(room.Members))
	seen := make(map[string]struct{}, len(room.Members))
	for _, memberID := range room.Members {
		target := h.participantBridgeTargetForRoomMember(memberID)
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

func (h *Handler) participantBridgeTargetForRoomMember(memberID string) participantBridgeTarget {
	memberID = strings.TrimSpace(memberID)
	if memberID == "" {
		return participantBridgeTarget{}
	}
	if h != nil && h.participant != nil {
		if item, ok := h.participant.Get(participant.ChannelCSGClaw, memberID); ok && isCSGClawAgentParticipant(item) {
			return participantBridgeTargetForParticipant(item, memberID)
		}
		for _, item := range h.participant.List(participant.ListOptions{Channel: participant.ChannelCSGClaw}) {
			if !isCSGClawAgentParticipant(item) || !participantMatchesIdentity(item, memberID) {
				continue
			}
			return participantBridgeTargetForParticipant(item, memberID)
		}
	}
	return newParticipantBridgeTarget(memberID, memberID)
}

func (h *Handler) participantBridgeTargetForBridgeID(bridgeID string) participantBridgeTarget {
	bridgeID = strings.TrimSpace(bridgeID)
	if bridgeID == "" {
		return participantBridgeTarget{}
	}
	if h != nil && h.participant != nil {
		if item, ok := h.participant.Get(participant.ChannelCSGClaw, bridgeID); ok && isCSGClawAgentParticipant(item) {
			return participantBridgeTargetForParticipant(item, bridgeID)
		}
		for _, item := range h.participant.List(participant.ListOptions{Channel: participant.ChannelCSGClaw}) {
			if !isCSGClawAgentParticipant(item) || !participantMatchesIdentity(item, bridgeID) {
				continue
			}
			return participantBridgeTargetForParticipant(item, bridgeID)
		}
	}
	if bridgeID == agent.ManagerParticipantID {
		return newParticipantBridgeTarget(agent.ManagerParticipantID, agent.ManagerUserID)
	}
	return newParticipantBridgeTarget(bridgeID, bridgeID)
}

func participantBridgeTargetForParticipant(item apitypes.Participant, aliases ...string) participantBridgeTarget {
	allAliases := []string{item.ID, item.ChannelUserRef, item.AgentID}
	allAliases = append(allAliases, aliases...)
	return newParticipantBridgeTarget(item.ID, allAliases...)
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

func roomForParticipantBridgeTarget(room im.Room, target participantBridgeTarget) im.Room {
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

func messageForParticipantBridgeTarget(message im.Message, target participantBridgeTarget) im.Message {
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
	out.Content = contentForParticipantBridgeTarget(message.Content, target)
	return out
}

func contentForParticipantBridgeTarget(content string, target participantBridgeTarget) string {
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

func (h *Handler) handleParticipantEventsStream(w http.ResponseWriter, r *http.Request, participantID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	events, cancel := h.participantBridge.Subscribe(participantID)
	defer func() {
		cancel()
		h.requeueBufferedParticipantEvents(participantID, events)
	}()
	controller := http.NewResponseController(w)

	if _, err := io.WriteString(w, ": connected\n\n"); err != nil {
		return
	}
	if err := flushParticipantSSE(controller, flusher); err != nil {
		return
	}
	h.replayRecentParticipantMessages(participantID, r.Header.Get("Last-Event-ID"))
	heartbeat := time.NewTicker(participantHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if err := writeParticipantSSEComment(w, controller, flusher, "heartbeat"); err != nil {
				return
			}
		case evt, ok := <-events:
			if !ok {
				return
			}
			if err := writeParticipantSSEEvent(w, controller, flusher, evt); err != nil {
				h.participantBridge.Requeue(participantID, evt)
				return
			}
			h.participantBridge.Ack(participantID, evt.MessageID)
		}
	}
}

func writeParticipantSSEEvent(w http.ResponseWriter, controller *http.ResponseController, fallback http.Flusher, evt im.ParticipantEvent) error {
	data, err := evt.MarshalJSONLine()
	if err != nil {
		return err
	}
	if id := participantSSEID(evt.MessageID); id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", data); err != nil {
		return err
	}
	return flushParticipantSSE(controller, fallback)
}

func writeParticipantSSEComment(w http.ResponseWriter, controller *http.ResponseController, fallback http.Flusher, comment string) error {
	if _, err := fmt.Fprintf(w, ": %s\n\n", comment); err != nil {
		return err
	}
	return flushParticipantSSE(controller, fallback)
}

func flushParticipantSSE(controller *http.ResponseController, fallback http.Flusher) error {
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

func (h *Handler) requeueBufferedParticipantEvents(participantID string, events <-chan im.ParticipantEvent) {
	if h == nil || h.participantBridge == nil {
		return
	}
	for evt := range events {
		h.participantBridge.Requeue(participantID, evt)
	}
}

func (h *Handler) replayRecentParticipantMessages(participantID, lastEventID string) {
	if h == nil || h.im == nil || h.participantBridge == nil {
		return
	}
	rooms := h.im.ListRoomsWithOptions(im.ListMessagesOptions{IncludeThreadReplies: true})
	cutoff := time.Now().UTC().Add(-participantReplayWindow)
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
			if h.hasLaterMessageFromBridgeTarget(room.Messages[idx+1:], participantID) {
				continue
			}
			sender, ok := h.im.User(message.SenderID)
			if !ok {
				continue
			}
			if reason, ok, err := newConversationCommandReason(message.Content); err != nil {
				slog.Warn("parse new conversation command failed", "participant_id", participantID, "message_id", message.ID, "error", err)
				h.enqueueParticipantMessageEventForBridgeID(room, sender, message, participantID, "")
				continue
			} else if ok {
				missed := h.publishNewConversationParticipantEvent(context.Background(), room, sender, message, reason)
				h.reconnectMissedParticipantAgents(sender.ID, missed)
				continue
			}
			// Route replay through the bridge so the stable message ID remains the
			// dedupe key for events already delivered live or drained from pending.
			h.enqueueParticipantMessageEventForBridgeID(room, sender, message, participantID, "")
		}
	}
}

func (h *Handler) hasLaterMessageFromBridgeTarget(messages []im.Message, bridgeID string) bool {
	target := h.participantBridgeTargetForBridgeID(bridgeID)
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

func participantSSEID(messageID string) string {
	messageID = strings.TrimSpace(messageID)
	messageID = strings.ReplaceAll(messageID, "\r", "")
	messageID = strings.ReplaceAll(messageID, "\n", "")
	return messageID
}

func (h *Handler) reconnectMissedParticipantAgents(senderID string, participantIDs []string) {
	if h == nil || h.svc == nil || h.isAgentSender(senderID) || len(participantIDs) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(participantIDs))
	for _, participantID := range participantIDs {
		agentID := h.runtimeAgentIDForBridgeID(participantID)
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
		go h.recoverMissedParticipantDelivery(agentID)
	}
}

func (h *Handler) recoverMissedParticipantDelivery(participantID string) {
	if h == nil || h.svc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	view, err := h.svc.RuntimeView(ctx, participantID)
	if err != nil {
		slog.Warn("participant delivery recovery failed", "agent_id", participantID, "error", err)
		return
	}
	if err := h.applyParticipantDeliveryRecoveryPolicy(ctx, view); err != nil {
		slog.Warn("participant delivery recovery failed", "agent_id", participantID, "runtime_kind", view.RuntimeKind, "state", view.State, "error", err)
	}
}

func (h *Handler) applyParticipantDeliveryRecoveryPolicy(ctx context.Context, view agent.RuntimeView) error {
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

func (h *Handler) handleParticipantSendMessage(w http.ResponseWriter, r *http.Request, participantID string) {
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}
	var req im.ParticipantSendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	roomID := req.ResolvedRoomID()
	text := req.ResolvedText()
	threadRootID := req.ResolvedThreadRootID()

	message, err := h.im.DeliverMessage(im.DeliverMessageRequest{
		RoomID:       roomID,
		SenderID:     participantID,
		Content:      text,
		MessageID:    req.MessageID,
		ThreadRootID: threadRootID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.publishMessageCreated(roomID, participantID, message)
	h.publishThreadUpdated(roomID, message)
	writeJSON(w, http.StatusOK, map[string]string{"message_id": message.ID})
}
