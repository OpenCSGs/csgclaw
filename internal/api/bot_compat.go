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

	"github.com/go-chi/chi/v5"

	"csgclaw/internal/agent"
	"csgclaw/internal/im"
	agentruntime "csgclaw/internal/runtime"
)

const (
	botReplayWindow      = 30 * time.Minute
	botHeartbeatInterval = 15 * time.Second
)

func (h *Handler) registerBotCompatibilityRoutes(router chi.Router) {
	router.Route("/api/bots/{id}", func(r chi.Router) {
		r.Get("/events", h.handleBotCompatibilityEvents)
		r.Post("/messages/send", h.handleBotCompatibilitySendMessage)
		r.Get("/llm/models", h.handleBotCompatibilityLLMModels)
		r.Get("/llm/v1/models", h.handleBotCompatibilityLLMModels)
		r.Post("/llm/chat/completions", h.handleBotCompatibilityLLMChatCompletions)
		r.Post("/llm/v1/chat/completions", h.handleBotCompatibilityLLMChatCompletions)
	})
}

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
	missed := h.botBridge.PublishMessageEvent(room, *evt.Sender, *evt.Message)
	h.reconnectMissedBotAgents(evt.Sender.ID, missed)
}

func (h *Handler) handleBotCompatibilityEvents(w http.ResponseWriter, r *http.Request) {
	botID, ok := h.requireBotCompatibilityBotID(w, r)
	if !ok {
		return
	}
	h.handleBotEvents(w, r, botID)
}

func (h *Handler) handleBotCompatibilitySendMessage(w http.ResponseWriter, r *http.Request) {
	botID, ok := h.requireBotCompatibilityBotID(w, r)
	if !ok {
		return
	}
	h.handleBotSendMessage(w, r, botID)
}

func (h *Handler) handleBotCompatibilityLLMModels(w http.ResponseWriter, r *http.Request) {
	botID, ok := h.requireBotCompatibilityBotID(w, r)
	if !ok {
		return
	}
	h.handleBotLLMModels(w, r, botID)
}

func (h *Handler) handleBotCompatibilityLLMChatCompletions(w http.ResponseWriter, r *http.Request) {
	botID, ok := h.requireBotCompatibilityBotID(w, r)
	if !ok {
		return
	}
	h.handleBotLLMChatCompletions(w, r, botID)
}

func (h *Handler) requireBotCompatibilityBotID(w http.ResponseWriter, r *http.Request) (string, bool) {
	botID := pathValue(r, "id")
	if botID == "" {
		http.NotFound(w, r)
		return "", false
	}
	if h.botBridge == nil {
		http.Error(w, "picoclaw integration is not configured", http.StatusServiceUnavailable)
		return "", false
	}
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	return botID, true
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
	rooms := h.im.ListRooms()
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
			if hasLaterMessageFrom(room.Messages[idx+1:], botID) {
				continue
			}
			sender, ok := h.im.User(message.SenderID)
			if !ok {
				continue
			}
			// Route replay through the bridge so the stable message ID remains the
			// dedupe key for events already delivered live or drained from pending.
			h.botBridge.EnqueueMessageEvent(room, sender, message, botID)
		}
	}
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
		botID = strings.TrimSpace(botID)
		if botID == "" {
			continue
		}
		if _, ok := seen[botID]; ok {
			continue
		}
		seen[botID] = struct{}{}
		if _, ok := h.svc.Agent(botID); !ok {
			continue
		}
		go h.recoverMissedBotDelivery(botID)
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
	_, ok := h.svc.Agent(senderID)
	return ok
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

	message, err := h.im.DeliverMessage(im.DeliverMessageRequest{
		RoomID:   req.RoomID,
		SenderID: botID,
		Content:  req.Text,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.publishMessageCreated(req.RoomID, botID, message)
	writeJSON(w, http.StatusOK, map[string]string{"message_id": message.ID})
}

func parseBotCompatibilityPath(path string) (botID, action string, ok bool) {
	const prefix = "/api/bots/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}

	rest := strings.TrimPrefix(path, prefix)
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) < 2 || parts[0] == "" {
		return "", "", false
	}

	botID = parts[0]
	action = strings.Join(parts[1:], "/")
	switch action {
	case "events", "messages/send", "llm/models", "llm/v1/models", "llm/chat/completions", "llm/v1/chat/completions":
		return botID, action, true
	default:
		return "", "", false
	}
}
