package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type batchAddAgentMCPServersRequest struct {
	Names []string `json:"names"`
}

type batchDeleteAgentMCPServersRequest struct {
	Names []string `json:"names"`
}

func (h *Handler) handleAgentMCPServersByID(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimSpace(pathValue(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	view, err := h.svc.MCPServersView(r.Context(), id)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *Handler) handleBatchAddAgentMCPServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.svc == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	if h.mcp == nil {
		http.Error(w, "mcp service is not configured", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimSpace(pathValue(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	var req batchAddAgentMCPServersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	hasName := false
	for _, name := range req.Names {
		if strings.TrimSpace(name) != "" {
			hasName = true
			break
		}
	}
	if !hasName {
		http.Error(w, "names is required", http.StatusBadRequest)
		return
	}
	servers, err := h.mcp.ListServers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	updated, err := h.svc.AddMCPServersFromHub(r.Context(), id, req.Names, servers)
	if err != nil {
		writeAgentMCPServersMutationError(w, err)
		return
	}
	h.publishUpdatedAgentUser(updated)
	view, err := h.svc.MCPServersView(r.Context(), updated.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *Handler) handleBatchDeleteAgentMCPServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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
	var req batchDeleteAgentMCPServersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	if !hasMCPServerName(req.Names) {
		http.Error(w, "names is required", http.StatusBadRequest)
		return
	}
	updated, err := h.svc.DeleteMCPServers(r.Context(), id, req.Names)
	if err != nil {
		writeAgentMCPServersMutationError(w, err)
		return
	}
	h.publishUpdatedAgentUser(updated)
	view, err := h.svc.MCPServersView(r.Context(), updated.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func writeAgentMCPServersMutationError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "not found") {
		status = http.StatusNotFound
	} else if strings.Contains(message, "mcp server") && strings.Contains(message, "config") {
		status = http.StatusBadGateway
	}
	http.Error(w, err.Error(), status)
}

func hasMCPServerName(names []string) bool {
	for _, name := range names {
		if strings.TrimSpace(name) != "" {
			return true
		}
	}
	return false
}
