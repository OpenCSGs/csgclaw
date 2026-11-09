package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"csgclaw/internal/relay"
)

func (h *Handler) handleRelayChat(w http.ResponseWriter, r *http.Request) {
	if h.relayManager == nil {
		writeAgentSessionError(w, 503, "relay_unavailable", "relay service is unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Minute)
	defer cancel()
	id := pathValue(r, "id")
	var result relay.Response
	var err error
	if r.Method == http.MethodGet {
		result, err = h.relayManager.ChatResponse(ctx, id, r.URL.Query().Get("connection_id"), pathValue(r, "response_id"))
	} else {
		var body struct {
			ConnectionID string `json:"connection_id"`
			Input        string `json:"input"`
			Previous     string `json:"previous_response_id,omitempty"`
			Key          string `json:"key"`
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		d.DisallowUnknownFields()
		if d.Decode(&body) != nil || d.Decode(new(any)) != io.EOF || strings.TrimSpace(body.Input) == "" || strings.TrimSpace(body.Key) == "" || len(body.Key) > 128 || strings.ContainsAny(body.Key, "\r\n") {
			writeAgentSessionError(w, 400, "invalid_request", "a text input, connection reference and request key are required", nil)
			return
		}
		result, err = h.relayManager.Chat(ctx, id, body.ConnectionID, body.Key, body.Input, body.Previous)
	}
	if err != nil {
		if errors.Is(err, relay.ErrRequestTooLarge) {
			writeAgentSessionError(w, http.StatusRequestEntityTooLarge, "relay_chat_input_too_large", err.Error(), nil)
			return
		}
		if errors.Is(err, relay.ErrChatReference) {
			writeAgentSessionError(w, 409, "relay_chat_reference_unavailable", err.Error(), nil)
			return
		}
		var remote *relay.HTTPError
		if errors.As(err, &remote) {
			if responseID := remote.PendingResponseID(); r.Method == http.MethodPost && responseID != "" {
				writeJSON(w, http.StatusAccepted, map[string]any{"id": responseID, "status": "in_progress", "model": id})
				return
			}
			writeAgentSessionError(w, remote.Status, remote.Detail.Code, "relay request failed", nil)
			return
		}
		writeAgentSessionError(w, 502, "relay_chat_failed", "unable to confirm relay response; retry the same request", nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
