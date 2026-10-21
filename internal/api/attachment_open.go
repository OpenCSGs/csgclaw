package api

import (
	"net/http"
	"path/filepath"
)

func (h *Handler) resolveAttachmentLocation(r *http.Request) (workspaceLocation, error) {
	file, err := h.im.LocalRoomAttachmentFile(pathValue(r, "id"), pathValue(r, "attachment_id"))
	if err != nil {
		return workspaceLocation{}, err
	}
	absolute, err := filepath.Abs(file.Path)
	if err != nil {
		return workspaceLocation{}, err
	}
	return workspaceLocation{absolute, false, h.workspaceOpenTargets(r, absolute, false)}, nil
}

func (h *Handler) handleRoomAttachmentLocation(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeRoomAttachments(w, r) {
		return
	}
	// These paths and copies belong to the local host, not remote browsers or agents.
	if len(h.workspaceOpenTargets(r, "", true)) == 0 {
		writeCodedAPIError(w, http.StatusForbidden, "workspace_open_denied", "Local attachment opening is unavailable")
		return
	}
	location, err := h.resolveAttachmentLocation(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, location)
}

func (h *Handler) handleRoomAttachmentOpen(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeRoomAttachments(w, r) {
		return
	}
	h.handleWorkspaceOpen(w, r, func(string) (workspaceLocation, error) { return h.resolveAttachmentLocation(r) })
}
