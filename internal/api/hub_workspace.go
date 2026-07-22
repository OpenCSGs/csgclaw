package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"csgclaw/internal/apitypes"
	hub "csgclaw/internal/template"
)

func (h *Handler) handleHubTemplateWorkspaceFileWrite(w http.ResponseWriter, r *http.Request, id string) {
	hubSvc, err := h.hubServiceForRequest(r)
	if err != nil || hubSvc == nil {
		http.Error(w, "hub service is not configured", http.StatusServiceUnavailable)
		return
	}
	var request struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	workspacePath := strings.TrimSpace(r.URL.Query().Get("path"))
	if err := hubSvc.WriteWorkspaceFile(r.Context(), strings.TrimSpace(id), workspacePath, request.Content); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, hub.ErrRegistryNotWritable) {
			status = http.StatusForbidden
		}
		http.Error(w, err.Error(), status)
		return
	}
	file, err := hubSvc.ReadWorkspaceFile(r.Context(), strings.TrimSpace(id), workspacePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, file)
}

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
	hubSvc, err := h.hubServiceForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if hubSvc == nil {
		http.Error(w, "hub service is not configured", http.StatusServiceUnavailable)
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		http.NotFound(w, r)
		return
	}
	listing, err := hubSvc.ListWorkspace(r.Context(), id, strings.TrimSpace(r.URL.Query().Get("path")))
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
	hubSvc, err := h.hubServiceForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if hubSvc == nil {
		http.Error(w, "hub service is not configured", http.StatusServiceUnavailable)
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		http.NotFound(w, r)
		return
	}
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	file, err := hubSvc.ReadWorkspaceFile(r.Context(), id, path)
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
