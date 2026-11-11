package api

import (
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/agentmemory"
	"csgclaw/internal/identity"
	"csgclaw/internal/im"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

func (h *Handler) getAgentMemory(w http.ResponseWriter, r *http.Request) {
	if h.memoryMode != "" {
		h.getManagedMemory(w, r)
		return
	}
	if h.agentEngine == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	current, err := h.agentEngine.Agents().Get(r.Context(), pathValue(r, "id"), agentengine.AgentGetOptions{IncludeDocuments: true})
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if current.Status.Memory == nil || current.Status.Memory.Error != "" {
		message := "agent memory is unavailable"
		if current.Status.Memory != nil {
			message = current.Status.Memory.Error
		}
		http.Error(w, message, http.StatusNotFound)
		return
	}
	document := memoryDocumentFromEngine(current.Status.Memory)
	writeJSON(w, http.StatusOK, document)
}

func (h *Handler) putAgentMemory(w http.ResponseWriter, r *http.Request) {
	if h.memoryMode != "" {
		h.putManagedMemory(w, r)
		return
	}
	if h.agentEngine == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	var request struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if request.Enabled == nil {
		http.Error(w, "enabled is required", http.StatusBadRequest)
		return
	}
	agents := h.agentEngine.Agents()
	current, err := agents.Get(r.Context(), pathValue(r, "id"), agentengine.AgentGetOptions{IncludeDocuments: true})
	if err == nil {
		current.Spec.Memory = &agentengine.MemorySpec{Enabled: *request.Enabled}
		_, err = agents.Update(r.Context(), current.ID, agentengine.AgentUpdateRequest{
			Spec: current.Spec, FieldMask: []string{"memory"}, ResourceVersion: current.ResourceVersion,
		})
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	current, err = agents.Get(r.Context(), current.ID, agentengine.AgentGetOptions{IncludeDocuments: true})
	if err != nil || current.Status.Memory == nil || current.Status.Memory.Error != "" {
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else if current.Status.Memory != nil {
			http.Error(w, current.Status.Memory.Error, http.StatusBadRequest)
		} else {
			http.Error(w, "agent memory is unavailable", http.StatusBadRequest)
		}
		return
	}
	document := memoryDocumentFromEngine(current.Status.Memory)
	writeJSON(w, http.StatusOK, document)
}

func memoryDocumentFromEngine(status *agentengine.MemoryStatus) agent.MemoryDocument {
	return agent.MemoryDocument{
		Enabled: status.Enabled, Ready: status.Ready, Name: status.Name,
		Location: status.Location, Content: status.Content,
	}
}

func (h *Handler) SetAgentMemory(store *agentmemory.Store, mode string) {
	h.agentMemory, h.memoryMode = store, mode
}

func (h *Handler) memoryUser(r *http.Request) string {
	if h.agentMemory == nil || !localWorkspaceClient(r) || appRequestAgentID(r) != "" ||
		r.Header.Get("X-CSGClaw-Caller-Agent") != "" {
		return ""
	}
	for _, name := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"} {
		if r.Header.Get(name) != "" {
			return ""
		}
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host && origin != "https://"+r.Host {
		return ""
	}
	header := r.Header.Get("Authorization")
	switch h.memoryMode {
	case "desktop":
		if h.desktopSessionToken == "" || header != "Bearer "+h.desktopSessionToken {
			return ""
		}
	case "local":
		if strings.HasPrefix(header, "Bearer agent.") ||
			(!h.serverNoAuth || header != "") && !h.validServerCredential(header) {
			return ""
		}
	default:
		return ""
	}
	return im.AdminUserID
}

func (h *Handler) managedMemoryScope(w http.ResponseWriter, r *http.Request) (agentmemory.Scope, bool) {
	userID := h.memoryUser(r)
	if userID == "" {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, agentmemory.Document{Available: false, Name: "MEMORY.md", Entries: []agentmemory.Entry{}})
		} else {
			writeCodedAPIError(w, http.StatusForbidden, "memory_unavailable", "Memory is unavailable for this client")
		}
		return agentmemory.Scope{}, false
	}
	current, err := h.agentEngine.Agents().Get(r.Context(), pathValue(r, "id"), agentengine.AgentGetOptions{})
	if err != nil {
		writeCodedAPIError(w, http.StatusNotFound, "agent_not_found", "Agent not found")
		return agentmemory.Scope{}, false
	}
	if !current.Status.Capabilities.Memory {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, agentmemory.Document{Available: false, Name: "MEMORY.md", Entries: []agentmemory.Entry{}})
		} else {
			writeCodedAPIError(w, http.StatusForbidden, "memory_unavailable", "Memory is unavailable for this Agent")
		}
		return agentmemory.Scope{}, false
	}
	return agentmemory.Scope{AgentID: identity.CanonicalAgentID(current.ID), UserID: userID}, true
}

func (h *Handler) getManagedMemory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	scope, ok := h.managedMemoryScope(w, r)
	if !ok {
		return
	}
	engine := h.agentEngine.(*agentengine.Engine)
	var document agentmemory.Document
	err := engine.WithMemoryExecution(r.Context(), scope.AgentID, func() error {
		var err error
		document, err = h.agentMemory.GetOrInitialize(scope)
		return err
	})
	if errors.Is(err, agentmemory.ErrUnavailable) {
		document, err = h.agentMemory.Get(scope)
	}
	if r.URL.Query().Get("view") == "marks" && err == nil {
		sources := make([]agentmemory.Source, 0, len(document.Entries))
		seen := make(map[agentmemory.Source]bool)
		for _, entry := range document.Entries {
			if entry.Source != nil && !seen[*entry.Source] {
				seen[*entry.Source] = true
				sources = append(sources, *entry.Source)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"available": true, "enabled": document.Enabled,
			"revision": document.Revision, "sources": sources})
		return
	}
	writeMemoryResult(w, document, err)
}

func (h *Handler) putManagedMemory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	scope, ok := h.managedMemoryScope(w, r)
	if !ok {
		return
	}
	var request agentmemory.Update
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(new(any)) != io.EOF {
		writeMemoryResult(w, agentmemory.Document{}, agentmemory.ErrInvalid)
		return
	}
	engine := h.agentEngine.(*agentengine.Engine)
	var document agentmemory.Document
	err := engine.WithMemoryExecution(r.Context(), scope.AgentID, func() error {
		var err error
		document, err = h.agentMemory.Update(scope, request)
		return err
	})
	writeMemoryResult(w, document, err)
}

func (h *Handler) deleteAgentMemorySource(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	scope, ok := h.managedMemoryScope(w, r)
	if !ok {
		return
	}
	source := agentmemory.Source{RoomID: pathValue(r, "room"), MessageID: pathValue(r, "message")}
	if h.im == nil || !h.im.MemorySourceExists(scope, source) {
		writeCodedAPIError(w, http.StatusNotFound, "memory_source_not_found", "Memory source not found")
		return
	}
	engine := h.agentEngine.(*agentengine.Engine)
	var document agentmemory.Document
	err := engine.WithMemoryExecution(r.Context(), scope.AgentID, func() error {
		var err error
		document, err = h.agentMemory.Forget(scope, source)
		return err
	})
	writeMemoryResult(w, document, err)
}

func writeMemoryResult(w http.ResponseWriter, document agentmemory.Document, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, document)
	case errors.Is(err, agentmemory.ErrConflict):
		writeCodedAPIError(w, http.StatusConflict, "memory_conflict", "Memory changed. Reload it before saving.")
	case errors.Is(err, agentmemory.ErrCapacity):
		writeCodedAPIError(w, http.StatusRequestEntityTooLarge, "memory_capacity", "The memory document is full")
	case errors.Is(err, agentmemory.ErrInvalid):
		writeCodedAPIError(w, http.StatusBadRequest, "memory_invalid", "Invalid memory update")
	case errors.Is(err, agentmemory.ErrUnavailable):
		writeCodedAPIError(w, http.StatusConflict, "memory_unavailable", "Memory is unavailable")
	default:
		writeCodedAPIError(w, http.StatusInternalServerError, "memory_storage_error", "Memory could not be saved or loaded")
	}
}
