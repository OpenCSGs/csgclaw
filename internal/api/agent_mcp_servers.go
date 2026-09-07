package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/mcpschema"
)

type batchAddAgentMCPServersRequest struct {
	Names []string `json:"names"`
}

type batchDeleteAgentMCPServersRequest struct {
	Names []string `json:"names"`
}

func (h *Handler) handleAgentMCPServersByID(w http.ResponseWriter, r *http.Request) {
	if h.agentEngine == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimSpace(pathValue(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	current, err := h.agentEngine.Agents().Get(r.Context(), id, agentengine.AgentGetOptions{AdoptMCPServers: true})
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, agent.MCPServersView{AgentID: current.ID, RuntimeKind: current.Status.RuntimeKind, Servers: serviceMCPServers(current.Spec.MCPServers)})
}

func (h *Handler) handleBatchAddAgentMCPServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.agentEngine == nil {
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
	agents := h.agentEngine.Agents()
	current, err := agents.Get(r.Context(), id, agentengine.AgentGetOptions{AdoptMCPServers: true})
	if err == nil {
		if current.Spec.MCPServers == nil {
			current.Spec.MCPServers = map[string]agentengine.MCPServerConfig{}
		}
		for _, rawName := range req.Names {
			name := strings.TrimSpace(rawName)
			raw, ok := servers[name]
			if !ok {
				err = fmt.Errorf("mcp server %q not found", name)
				break
			}
			config, ok := raw.(map[string]any)
			if !ok {
				err = fmt.Errorf("mcp server %q config must be an object", name)
				break
			}
			server, normalizeErr := agentMCPServerConfig(name, config)
			if normalizeErr != nil {
				err = normalizeErr
				break
			}
			current.Spec.MCPServers[name] = server
		}
	}
	var updated agentengine.Agent
	if err == nil {
		updated, err = agents.Update(r.Context(), id, agentengine.AgentUpdateRequest{Spec: current.Spec, FieldMask: []string{"mcp_servers"}, ResourceVersion: current.ResourceVersion})
	}
	if err != nil {
		writeAgentMCPServersMutationError(w, err)
		return
	}
	h.publishUpdatedAgentUser(serviceAgentFromEngine(updated))
	view := agent.MCPServersView{AgentID: updated.ID, RuntimeKind: updated.Status.RuntimeKind, Servers: serviceMCPServers(updated.Spec.MCPServers)}
	writeJSON(w, http.StatusOK, view)
}

func agentMCPServerConfig(name string, config map[string]any) (agentengine.MCPServerConfig, error) {
	normalized, err := mcpschema.NormalizeMCPServers(map[string]any{name: config})
	if err != nil {
		return nil, err
	}
	server, ok := normalized[name].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mcp server %q config must be an object", name)
	}
	delete(server, "description")
	return agentengine.MCPServerConfig(server), nil
}

func (h *Handler) handleBatchDeleteAgentMCPServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.agentEngine == nil {
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
	agents := h.agentEngine.Agents()
	current, err := agents.Get(r.Context(), id, agentengine.AgentGetOptions{AdoptMCPServers: true})
	if err == nil {
		for _, rawName := range req.Names {
			name := strings.TrimSpace(rawName)
			if _, ok := current.Spec.MCPServers[name]; !ok {
				err = fmt.Errorf("mcp server %q not found", name)
				break
			}
			delete(current.Spec.MCPServers, name)
		}
	}
	var updated agentengine.Agent
	if err == nil {
		updated, err = agents.Update(r.Context(), id, agentengine.AgentUpdateRequest{Spec: current.Spec, FieldMask: []string{"mcp_servers"}, ResourceVersion: current.ResourceVersion})
	}
	if err != nil {
		writeAgentMCPServersMutationError(w, err)
		return
	}
	h.publishUpdatedAgentUser(serviceAgentFromEngine(updated))
	view := agent.MCPServersView{AgentID: updated.ID, RuntimeKind: updated.Status.RuntimeKind, Servers: serviceMCPServers(updated.Spec.MCPServers)}
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
