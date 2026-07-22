package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (h *Handler) getAgentInstructions(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	document, err := h.svc.InstructionsDocument(pathValue(r, "id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, document)
}

func (h *Handler) putAgentInstructions(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	var request struct {
		Effective string `json:"effective"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(request.Effective) == "" {
		http.Error(w, "effective instructions are required", http.StatusBadRequest)
		return
	}
	document, err := h.svc.UpdateEffectiveInstructions(r.Context(), pathValue(r, "id"), request.Effective)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, document)
}
