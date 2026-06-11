package api

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"csgclaw/internal/agentworkspace"
)

var (
	errAgentWorkspaceNotFound = errors.New("agent not found")
)

func (h *Handler) handleAgentWorkspace(w http.ResponseWriter, r *http.Request) {
	h.handleAgentWorkspaceListing(w, r, h.agentWorkspaceRoot)
}

func (h *Handler) handleAgentWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	h.handleAgentWorkspaceFileRead(w, r, h.agentWorkspaceRoot)
}

func (h *Handler) handleAgentSkills(w http.ResponseWriter, r *http.Request) {
	h.handleAgentWorkspaceListing(w, r, h.agentSkillsRoot)
}

func (h *Handler) handleAgentSkillsFile(w http.ResponseWriter, r *http.Request) {
	h.handleAgentWorkspaceFileRead(w, r, h.agentSkillsRoot)
}

func (h *Handler) handleAgentWorkspaceListing(w http.ResponseWriter, r *http.Request, resolveRoot func(string) (string, error)) {
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
	root, err := resolveRoot(id)
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

func (h *Handler) handleAgentWorkspaceFileRead(w http.ResponseWriter, r *http.Request, resolveRoot func(string) (string, error)) {
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
	root, err := resolveRoot(id)
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
	return h.svc.WorkspaceRoot(item.Name)
}

func (h *Handler) agentSkillsRoot(id string) (string, error) {
	item, ok := h.svc.Agent(id)
	if !ok {
		return "", errAgentWorkspaceNotFound
	}
	return h.svc.SkillsRoot(item.Name)
}

func writeAgentWorkspaceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAgentWorkspaceNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
