package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"csgclaw/internal/codexcli"
	"csgclaw/internal/runtimecatalog"
)

func (h *Handler) listAgentRuntimes(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || h.agentRuntimes == nil {
		http.Error(w, "agent runtime service is not configured", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, h.agentRuntimes.List())
}

func (h *Handler) installAgentRuntime(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || h.agentRuntimes == nil {
		http.Error(w, "agent runtime service is not configured", http.StatusServiceUnavailable)
		return
	}
	runtimeStatus, err := h.agentRuntimes.Install(r.Context(), chi.URLParam(r, "name"))
	if err != nil {
		switch {
		case errors.Is(err, runtimecatalog.ErrRuntimeNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, runtimecatalog.ErrInstallUnsupported), errors.Is(err, codexcli.ErrUnsupportedPlatform):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		return
	}
	writeJSON(w, http.StatusOK, runtimeStatus)
}
