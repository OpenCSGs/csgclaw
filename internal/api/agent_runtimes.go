package api

import (
	"errors"
	"net/http"
	"strings"

	"csgclaw/internal/runtimecatalog"

	"github.com/go-chi/chi/v5"
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
		writeCodedAPIError(w, http.StatusServiceUnavailable, "agent_runtimes_unavailable", "Agent runtime service is not configured")
		return
	}
	installation, err := h.agentRuntimes.StartInstall(strings.TrimSpace(chi.URLParam(r, "name")))
	if err != nil {
		writeAgentRuntimeInstallationError(w, err)
		return
	}
	status := http.StatusAccepted
	if installation.Status != runtimecatalog.InstallStatusRunning {
		status = http.StatusOK
	}
	writeJSON(w, status, installation)
}

func (h *Handler) getAgentRuntimeInstallation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h == nil || h.agentRuntimes == nil {
		writeCodedAPIError(w, http.StatusServiceUnavailable, "agent_runtimes_unavailable", "Agent runtime service is not configured")
		return
	}
	installation, err := h.agentRuntimes.Installation(strings.TrimSpace(chi.URLParam(r, "name")))
	if err != nil {
		writeAgentRuntimeInstallationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, installation)
}

func writeAgentRuntimeInstallationError(w http.ResponseWriter, err error) {
	if errors.Is(err, runtimecatalog.ErrRuntimeNotInstallable) {
		writeCodedAPIError(w, http.StatusBadRequest, "runtime_not_installable", err.Error())
		return
	}
	writeCodedAPIError(w, http.StatusInternalServerError, "dsh_install_failed", err.Error())
}
