package api

import (
	"csgclaw/internal/apitypes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (h *Handler) unreadParticipant(r *http.Request, provided string) (string, error) {
	provided = strings.TrimSpace(provided)
	if agentID := appRequestAgentID(r); agentID != "" {
		actor := h.participantBridgeTargetForRoomMember(h.agentPlatformParticipant(agentID))
		if provided != "" && !actor.matches(provided) {
			return "", fmt.Errorf("cannot consume another member's unread messages")
		}
		return actor.bridgeID, nil
	}
	if provided == "" {
		return "", fmt.Errorf("participant_id or an authenticated Agent is required")
	}
	return provided, nil
}
func (h *Handler) readUnreadMessages(w http.ResponseWriter, r *http.Request) {
	if h.im == nil {
		writeCodedAPIError(w, 503, "im_unavailable", "IM service unavailable")
		return
	}
	actor, err := h.unreadParticipant(r, r.URL.Query().Get("participant_id"))
	if err != nil {
		writeCodedAPIError(w, 403, "agent_access_denied", err.Error())
		return
	}
	limit := 50
	if text := r.URL.Query().Get("limit"); text != "" {
		limit, err = strconv.Atoi(text)
		if err != nil || limit < 1 || limit > 100 {
			writeCodedAPIError(w, 400, "invalid_read_limit", "limit must be between 1 and 100")
			return
		}
	}
	page, err := h.im.PrepareUnread(r.URL.Query().Get("room_id"), actor, r.URL.Query().Get("thread_root_id"), limit, parseBoolQuery(r.URL.Query().Get("peek")))
	if err != nil {
		writeCodedAPIError(w, 400, "unread_read_failed", err.Error())
		return
	}
	writeJSON(w, 200, page)
}
func (h *Handler) acknowledgeUnreadMessages(w http.ResponseWriter, r *http.Request) {
	if h.im == nil {
		writeCodedAPIError(w, 503, "im_unavailable", "IM service unavailable")
		return
	}
	var req apitypes.UnreadAck
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&req); err != nil {
		writeCodedAPIError(w, 400, "invalid_request", "invalid unread acknowledgement")
		return
	}
	actor, err := h.unreadParticipant(r, req.ParticipantID)
	if err != nil {
		writeCodedAPIError(w, 403, "agent_access_denied", err.Error())
		return
	}
	req.ParticipantID = actor
	if err = h.im.AcknowledgeUnread(req); err != nil {
		writeCodedAPIError(w, 409, "unread_ack_failed", err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"acknowledged": true})
}
