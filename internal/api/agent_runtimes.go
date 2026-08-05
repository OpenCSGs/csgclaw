package api

import (
	"net/http"
)

func (h *Handler) listAgentRuntimes(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || h.agentRuntimes == nil {
		http.Error(w, "agent runtime service is not configured", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, h.agentRuntimes.List())
}
