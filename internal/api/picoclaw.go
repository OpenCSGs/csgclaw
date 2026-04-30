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

	"csgclaw/internal/im"
)

const (
	picoClawReplayWindow      = 30 * time.Minute
	picoClawHeartbeatInterval = 15 * time.Second
)

func (h *Handler) registerPicoClawRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/bots/", h.handlePicoClawBotRoutes)
}

func (h *Handler) PublishPicoClawEvent(evt im.Event) {
	if h.picoclaw == nil || h.im == nil {
		return
	}
	if evt.Type != im.EventTypeMessageCreated || evt.Message == nil || evt.Sender == nil {
		return
	}

	room, ok := h.im.Room(evt.RoomID)
	if !ok {
		return
	}
	missed := h.picoclaw.PublishMessageEvent(room, *evt.Sender, *evt.Message)
	h.reconnectMissedPicoClawAgents(evt.Sender.ID, missed)
}

func (h *Handler) handlePicoClawBotRoutes(w http.ResponseWriter, r *http.Request) {
	botID, action, ok := parsePicoClawBotPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if h.picoclaw == nil {
		http.Error(w, "picoclaw integration is not configured", http.StatusServiceUnavailable)
		return
	}
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	switch {
	case r.Method == http.MethodGet && action == "events":
		h.handlePicoClawEvents(w, r, botID)
	case r.Method == http.MethodPost && action == "messages/send":
		h.handlePicoClawSendMessage(w, r, botID)
	case r.Method == http.MethodGet && (action == "llm/models" || action == "llm/v1/models"):
		h.handlePicoClawModels(w, r, botID)
	case r.Method == http.MethodPost && (action == "llm/chat/completions" || action == "llm/v1/chat/completions"):
		h.handlePicoClawChatCompletions(w, r, botID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handlePicoClawEvents(w http.ResponseWriter, r *http.Request, botID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	events, cancel := h.picoclaw.Subscribe(botID)
	defer func() {
		cancel()
		h.requeuePicoClawBufferedEvents(botID, events)
	}()
	controller := http.NewResponseController(w)

	if _, err := io.WriteString(w, ": connected\n\n"); err != nil {
		return
	}
	if err := flushPicoClawSSE(controller, flusher); err != nil {
		return
	}
	h.replayRecentPicoClawMessages(botID, r.Header.Get("Last-Event-ID"))
	heartbeat := time.NewTicker(picoClawHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if err := writePicoClawSSEComment(w, controller, flusher, "heartbeat"); err != nil {
				return
			}
		case evt, ok := <-events:
			if !ok {
				return
			}
			if err := writePicoClawSSEEvent(w, controller, flusher, evt); err != nil {
				h.picoclaw.Requeue(botID, evt)
				return
			}
			h.picoclaw.Ack(botID, evt.MessageID)
		}
	}
}

func writePicoClawSSEEvent(w http.ResponseWriter, controller *http.ResponseController, fallback http.Flusher, evt im.PicoClawEvent) error {
	data, err := evt.MarshalJSONLine()
	if err != nil {
		return err
	}
	if id := picoClawSSEID(evt.MessageID); id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", data); err != nil {
		return err
	}
	return flushPicoClawSSE(controller, fallback)
}

func writePicoClawSSEComment(w http.ResponseWriter, controller *http.ResponseController, fallback http.Flusher, comment string) error {
	if _, err := fmt.Fprintf(w, ": %s\n\n", comment); err != nil {
		return err
	}
	return flushPicoClawSSE(controller, fallback)
}

func flushPicoClawSSE(controller *http.ResponseController, fallback http.Flusher) error {
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

func (h *Handler) requeuePicoClawBufferedEvents(botID string, events <-chan im.PicoClawEvent) {
	if h == nil || h.picoclaw == nil {
		return
	}
	for evt := range events {
		h.picoclaw.Requeue(botID, evt)
	}
}

func (h *Handler) replayRecentPicoClawMessages(botID, lastEventID string) {
	if h == nil || h.im == nil || h.picoclaw == nil {
		return
	}
	rooms := h.im.ListRooms()
	cutoff := time.Now().UTC().Add(-picoClawReplayWindow)
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
			h.picoclaw.EnqueueMessageEvent(room, sender, message, botID)
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

func picoClawSSEID(messageID string) string {
	messageID = strings.TrimSpace(messageID)
	messageID = strings.ReplaceAll(messageID, "\r", "")
	messageID = strings.ReplaceAll(messageID, "\n", "")
	return messageID
}

func (h *Handler) reconnectMissedPicoClawAgents(senderID string, botIDs []string) {
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
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if _, err := h.svc.Recreate(ctx, botID); err != nil {
				slog.Warn("picoclaw agent reconnect failed", "agent_id", botID, "error", err)
			}
		}()
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

func (h *Handler) handlePicoClawSendMessage(w http.ResponseWriter, r *http.Request, botID string) {
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}
	var req im.PicoClawSendMessageRequest
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

func parsePicoClawBotPath(path string) (botID, action string, ok bool) {
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
