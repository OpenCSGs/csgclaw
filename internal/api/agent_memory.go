package api

import (
	"encoding/json"
	"net/http"
)

func (h *Handler) getAgentMemory(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	document, err := h.svc.MemoryDocument(r.Context(), pathValue(r, "id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, document)
}

func (h *Handler) putAgentMemory(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	var request struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if request.Enabled == nil {
		http.Error(w, "enabled is required", http.StatusBadRequest)
		return
	}
	document, err := h.svc.UpdateMemoryEnabled(r.Context(), pathValue(r, "id"), *request.Enabled)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, document)
}
