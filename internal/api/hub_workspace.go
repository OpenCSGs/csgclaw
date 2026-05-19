package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/hub"
)

const hubWorkspacePreviewMaxBytes = 256 * 1024

func (h *Handler) presentHubTemplateDetail(ctx context.Context, item hub.Template) (apitypes.HubTemplate, error) {
	presented := presentHubTemplate(item)
	workspaceRoot, cleanup, err := h.hubWorkspaceRoot(ctx, item)
	if err != nil {
		return apitypes.HubTemplate{}, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	entries, err := buildHubWorkspaceEntries(workspaceRoot)
	if err != nil {
		return apitypes.HubTemplate{}, err
	}
	presented.Workspace.Entries = entries
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

	path := strings.TrimSpace(r.URL.Query().Get("path"))
	file, err := readHubWorkspaceFile(workspaceRoot, path)
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

func buildHubWorkspaceEntries(root string) ([]apitypes.HubTemplateWorkspaceEntry, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, nil
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat hub workspace %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("hub workspace %q is not a directory", root)
	}

	entries := make([]apitypes.HubTemplateWorkspaceEntry, 0)
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		entry := apitypes.HubTemplateWorkspaceEntry{
			Path:  rel,
			Name:  d.Name(),
			Type:  "file",
			Depth: strings.Count(rel, "/"),
			Size:  info.Size(),
		}
		if d.IsDir() {
			entry.Type = "dir"
			entry.Size = 0
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk hub workspace %q: %w", root, err)
	}
	return entries, nil
}

func readHubWorkspaceFile(root, relativePath string) (apitypes.HubTemplateWorkspaceFile, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return apitypes.HubTemplateWorkspaceFile{}, fmt.Errorf("hub workspace is not available")
	}
	if relativePath == "" {
		return apitypes.HubTemplateWorkspaceFile{}, fmt.Errorf("hub workspace path is required")
	}
	clean := filepath.Clean(filepath.FromSlash(relativePath))
	if clean == "." || clean == string(filepath.Separator) || clean == "" {
		return apitypes.HubTemplateWorkspaceFile{}, fmt.Errorf("hub workspace path is required")
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		return apitypes.HubTemplateWorkspaceFile{}, fmt.Errorf("hub workspace path is invalid")
	}
	absPath := filepath.Join(root, clean)
	info, err := os.Stat(absPath)
	if err != nil {
		return apitypes.HubTemplateWorkspaceFile{}, fmt.Errorf("stat hub workspace file %q: %w", relativePath, err)
	}
	if info.IsDir() {
		return apitypes.HubTemplateWorkspaceFile{}, fmt.Errorf("hub workspace path %q is a directory", relativePath)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return apitypes.HubTemplateWorkspaceFile{}, fmt.Errorf("read hub workspace file %q: %w", relativePath, err)
	}
	file := apitypes.HubTemplateWorkspaceFile{
		Path: filepath.ToSlash(clean),
		Size: info.Size(),
	}
	if !utf8.Valid(data) {
		file.Binary = true
		return file, nil
	}
	if len(data) > hubWorkspacePreviewMaxBytes {
		file.Truncated = true
		data = data[:hubWorkspacePreviewMaxBytes]
	}
	file.Content = string(data)
	return file, nil
}
