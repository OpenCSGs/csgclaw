package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"csgclaw/internal/agentworkspace"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/hub"
)

func (h *Handler) presentHubTemplateDetail(ctx context.Context, item hub.Template) (apitypes.HubTemplate, error) {
	presented := presentHubTemplate(item)
	workspaceRoot, cleanup, err := h.hubWorkspaceRoot(ctx, item)
	if err != nil {
		return apitypes.HubTemplate{}, err
	}
	if cleanup != nil {
		defer cleanup()
	}
	if strings.TrimSpace(workspaceRoot) == "" {
		return presented, nil
	}

	listing, err := agentworkspace.List(workspaceRoot, "")
	if err != nil {
		return apitypes.HubTemplate{}, err
	}
	presented.Workspace.Entries = listing.Entries
	return presented, nil
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
	item, err := h.hub.Get(r.Context(), id)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	workspaceRoot, cleanup, err := h.hubWorkspaceRoot(r.Context(), item)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	if cleanup != nil {
		defer cleanup()
	}
	if strings.TrimSpace(workspaceRoot) == "" {
		http.Error(w, "hub workspace is not available", http.StatusBadRequest)
		return
	}

	path := strings.TrimSpace(r.URL.Query().Get("path"))
	file, err := agentworkspace.ReadFile(workspaceRoot, path)
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

func (h *Handler) hubWorkspaceRoot(ctx context.Context, item hub.Template) (string, func(), error) {
	switch item.Source.Kind {
	case hub.RegistryKindBuiltin, hub.RegistryKindRemote:
		workspace, err := h.hub.FetchWorkspace(ctx, item.ID)
		if err != nil {
			return "", nil, err
		}
		return workspace.Path, func() { _ = os.RemoveAll(workspace.Path) }, nil
	default:
		return strings.TrimSpace(item.WorkspaceRef.Path), nil, nil
	}
}
