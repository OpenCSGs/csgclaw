package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/agentworkspace"
)

var (
	errAgentWorkspaceNotFound    = errors.New("agent not found")
	errAgentWorkspaceUnsupported = errors.New("agent workspace is not supported for this runtime")
)

func (h *Handler) handleAgentWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimSpace(pathValue(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	root, err := h.agentWorkspaceRoot(id)
	if err != nil {
		writeAgentWorkspaceError(w, err)
		return
	}
	listing, err := agentworkspace.List(root, r.URL.Query().Get("path"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, listing)
}

func (h *Handler) handleAgentWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimSpace(pathValue(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	root, err := h.agentWorkspaceRoot(id)
	if err != nil {
		writeAgentWorkspaceError(w, err)
		return
	}
	file, err := agentworkspace.ReadFile(root, r.URL.Query().Get("path"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, file)
}

func (h *Handler) agentWorkspaceRoot(id string) (string, error) {
	item, ok := h.svc.Agent(id)
	if !ok {
		return "", errAgentWorkspaceNotFound
	}
	switch strings.TrimSpace(item.RuntimeKind) {
	case agent.RuntimeKindPicoClawSandbox, agent.RuntimeKindOpenClawSandbox:
	default:
		return "", fmt.Errorf("%w: %s", errAgentWorkspaceUnsupported, item.RuntimeKind)
	}
	return agent.WorkspaceRoot(item.Name, item.RuntimeKind)
}

func writeAgentWorkspaceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAgentWorkspaceNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, errAgentWorkspaceUnsupported):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
