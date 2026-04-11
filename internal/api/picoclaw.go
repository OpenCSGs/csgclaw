package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"csgclaw/internal/im"
)

func (h *Handler) registerPicoClawRoutes(mux *http.ServeMux) {
	// step 7.6 Bot-facing PicoClaw routes live outside /api/v1 because they follow PicoClaw's callback contract.
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
	// step 7.6.1 Only message.created events flow through this branch; room/user events stay browser-only.
	h.picoclaw.PublishMessageEvent(room, *evt.Sender, *evt.Message)
}

func (h *Handler) handlePicoClawBotRoutes(w http.ResponseWriter, r *http.Request) {
	// step 7.6.2 A bot can do two things here:
	// step 7.6.2.1 subscribe to its message event stream
	// step 7.6.2.2 send a reply back into an IM room.
	// step 7.6.2.3 reach the host-side LLM bridge through an OpenAI-compatible endpoint.
	botID, action, ok := parsePicoClawBotPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if h.picoclaw == nil {
		http.Error(w, "picoclaw integration is not configured", http.StatusServiceUnavailable)
		return
	}
	if !h.picoclaw.ValidateAccessToken(r.Header.Get("Authorization")) {
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
	// step 7.6.2.1 Bots receive SSE here instead of the server actively calling out to them.
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	events, cancel := h.picoclaw.Subscribe(botID)
	defer cancel()

	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt := <-events:
			data, err := evt.MarshalJSONLine()
			if err != nil {
				return
			}
			_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (h *Handler) handlePicoClawSendMessage(w http.ResponseWriter, r *http.Request, botID string) {
	if h.im == nil {
		http.Error(w, "im service is not configured", http.StatusServiceUnavailable)
		return
	}

	// step 7.6.2.2 Bot replies are written back into IM as normal messages authored by that bot's user ID.
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
