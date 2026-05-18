package api

import "net/http"

func (h *Handler) handleCsgclawRoomByID(w http.ResponseWriter, r *http.Request) {
	id := pathValue(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	h.handleLocalRoomByID(w, r, id)
}

func (h *Handler) handleCsgclawRoomMembersByID(w http.ResponseWriter, r *http.Request) {
	id := pathValue(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	h.handleRoomMembersByID(w, r, id)
}

func (h *Handler) handleCsgclawUserByID(w http.ResponseWriter, r *http.Request) {
	id := pathValue(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	h.handleLocalUserByID(w, r, id)
}
