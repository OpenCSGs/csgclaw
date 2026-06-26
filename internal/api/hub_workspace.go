package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"csgclaw/internal/apitypes"
	hub "csgclaw/internal/template"
)

func (h *Handler) presentHubTemplateDetail(ctx context.Context, item hub.Template) (apitypes.HubTemplate, error) {
	return presentHubTemplate(item), nil
}

func (h *Handler) handleHubTemplateWorkspaceByID(w http.ResponseWriter, r *http.Request) {
	h.handleHubTemplateWorkspace(w, r, pathValue(r, "id"))
}

func (h *Handler) handleHubTemplateWorkspace(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.hub == nil {
		http.Error(w, "hub service is not configured", http.StatusServiceUnavailable)
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		http.NotFound(w, r)
		return
	}
	listing, err := h.hub.ListWorkspace(r.Context(), id, strings.TrimSpace(r.URL.Query().Get("path")))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, os.ErrNotExist) || strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, listing)
}

func (h *Handler) handleHubTemplateWorkspaceFile(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.hub == nil {
		http.Error(w, "hub service is not configured", http.StatusServiceUnavailable)
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		http.NotFound(w, r)
		return
	}
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	file, err := h.hub.ReadWorkspaceFile(r.Context(), id, path)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, file)
}
