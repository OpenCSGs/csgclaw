package api

import (
	"csgclaw/internal/utils/filebrowse"
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/agentworkspace"
)

var (
	errAgentWorkspaceNotFound = errors.New("agent not found")
)

func (h *Handler) handleAgentWorkspace(w http.ResponseWriter, r *http.Request) {
	h.handleAgentWorkspaceListing(w, r, func(id string) (string, error) {
		return h.conversationWorkspaceRoot(id, r.URL.Query().Get("room_id"))
	})
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
	list := agentworkspace.List
	if r.URL.Query().Get("depth") == "1" {
		list = agentworkspace.ListDirectory
	}
	listing, err := list(root, r.URL.Query().Get("path"))
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
	return h.workspace.WorkspaceRoot(item.Name)
}

// Room configuration locks the same root used by task execution, preview, and sharing.
func (h *Handler) conversationWorkspaceRoot(agentID, roomID string) (string, error) {
	root, err := h.agentWorkspaceRoot(agentID)
	if err != nil || strings.TrimSpace(roomID) == "" {
		return root, err
	}
	if h.im == nil {
		return "", errors.New("conversation is unavailable")
	}
	room, ok := h.im.Room(strings.TrimSpace(roomID))
	if !ok {
		return "", errors.New("conversation is unavailable")
	}
	member := false
	for _, id := range room.Members {
		if h.participantBridgeTargetForRoomMember(id).matches(agentID) {
			member = true
			break
		}
	}
	if !member {
		return "", errors.New("agent is not a conversation member")
	}
	if room.IsDirect && room.Configuration != nil && room.Configuration.WorkspaceDir != "" {
		return room.Configuration.WorkspaceDir, nil
	}
	return root, nil
}

func (h *Handler) agentSkillsRoot(id string) (string, error) {
	item, ok := h.svc.Agent(id)
	if !ok {
		return "", errAgentWorkspaceNotFound
	}
	return h.workspace.SkillsRoot(item.Name)
}

func writeAgentWorkspaceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAgentWorkspaceNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

// handleAgentWorkspaceContent streams bytes without converting media into JSON.
func (h *Handler) handleAgentWorkspaceContent(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil || h.workspace == nil {
		writeCodedAPIError(w, http.StatusServiceUnavailable, "workspace_file_unavailable", "Workspace is unavailable")
		return
	}
	root, err := h.conversationWorkspaceRoot(strings.TrimSpace(pathValue(r, "id")), r.URL.Query().Get("room_id"))
	if err != nil {
		writeCodedAPIError(w, http.StatusNotFound, "workspace_file_unavailable", "Workspace is unavailable")
		return
	}
	file, err := filebrowse.OpenFile(root, r.URL.Query().Get("path"))
	if err != nil {
		writeCodedAPIError(w, http.StatusBadRequest, "workspace_file_unavailable", "Workspace file is unavailable")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeCodedAPIError(w, http.StatusBadRequest, "workspace_file_unavailable", "Workspace file is unavailable")
		return
	}
	mediaType := mime.TypeByExtension(filepath.Ext(info.Name()))
	if mediaType == "" {
		var prefix [512]byte
		n, _ := file.Read(prefix[:])
		mediaType = http.DetectContentType(prefix[:n])
		_, _ = file.Seek(0, 0)
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Name()}))
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}
