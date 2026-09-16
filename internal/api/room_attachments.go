package api

import (
	"net/http"
	"strconv"
	"strings"

	"csgclaw/internal/apitypes"
)

func (h *Handler) authorizeRoomAttachments(w http.ResponseWriter, r *http.Request) bool {
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	if h.im == nil {
		http.Error(w, "IM service unavailable", http.StatusServiceUnavailable)
		return false
	}
	room, ok := h.im.Room(pathValue(r, "id"))
	if !ok {
		http.NotFound(w, r)
		return false
	}
	if id := strings.TrimSpace(r.Header.Get("X-CSGClaw-Caller-Agent")); id != "" {
		caller := h.participantBridgeTargetForRoomMember(id)
		for _, member := range room.Members {
			if caller.matches(member) {
				return true
			}
		}
		http.Error(w, "caller is not a current room member", http.StatusForbidden)
		return false
	}
	return true
}

func (h *Handler) handleRoomAttachments(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeRoomAttachments(w, r) {
		return
	}
	q := r.URL.Query()
	opts := apitypes.RoomAttachmentListOptions{Query: q.Get("query"), MessageID: q.Get("message_id"), Limit: 50}
	for key, dst := range map[string]*int{"from": &opts.From, "limit": &opts.Limit} {
		if q.Has(key) {
			value, err := strconv.Atoi(q.Get(key))
			if err != nil || value < 0 || (key == "limit" && (value == 0 || value > 200)) {
				http.Error(w, "invalid "+key, http.StatusBadRequest)
				return
			}
			*dst = value
		}
	}
	result, err := h.im.ListRoomAttachments(pathValue(r, "id"), opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleRoomAttachmentDownload(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeRoomAttachments(w, r) {
		return
	}
	file, err := h.im.RoomAttachmentFile(pathValue(r, "id"), pathValue(r, "attachment_id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("X-CSGClaw-File-SHA256", file.Attachment.SHA256)
	w.Header().Set("X-CSGClaw-File-Size", strconv.FormatInt(file.Attachment.SizeBytes, 10))
	// Membership and live room references are checked on every download.
	serveAttachment(w, r, file, false)
}
