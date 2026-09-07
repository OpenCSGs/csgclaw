package api

import (
	"context"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	"fmt"
	"net/http"
	"strings"
)

type agentMCPServerSourceSyncResponse struct {
	Agent  agent.MCPServersView          `json:"agent"`
	Source mcpServerSourceStatusResponse `json:"source"`
}

type resolvedAgentMCPServerSource struct {
	Agent       agentengine.Agent
	AgentName   string
	Managed     resolvedManagedMCPServerSource
	SourceError error
}

func (h *Handler) handleAgentMCPServerSource(w http.ResponseWriter, r *http.Request) {
	if h.agentEngine == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	agentID := strings.TrimSpace(pathValue(r, "id"))
	name := strings.TrimSpace(pathValue(r, "name"))
	if agentID == "" || name == "" {
		http.NotFound(w, r)
		return
	}
	resolved, err := h.resolveAgentMCPServerSource(r.Context(), agentID, name)
	if err != nil {
		writeMCPServerSourceError(w, err)
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, resolved.Managed.Status)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if resolved.SourceError != nil {
		writeMCPServerSourceError(w, resolved.SourceError)
		return
	}

	server, err := agentMCPServerConfig(resolved.AgentName, resolved.Managed.Refreshed)
	if err != nil {
		writeAgentMCPServersMutationError(w, err)
		return
	}
	if resolved.Agent.Spec.MCPServers == nil {
		resolved.Agent.Spec.MCPServers = map[string]agentengine.MCPServerConfig{}
	}
	resolved.Agent.Spec.MCPServers[resolved.AgentName] = server
	updated, err := h.agentEngine.Agents().Update(r.Context(), resolved.Agent.ID, agentengine.AgentUpdateRequest{
		Spec:            resolved.Agent.Spec,
		FieldMask:       []string{"mcp_servers"},
		ResourceVersion: resolved.Agent.ResourceVersion,
	})
	if err != nil {
		writeAgentMCPServersMutationError(w, err)
		return
	}
	h.publishUpdatedAgentUser(serviceAgentFromEngine(updated))
	status := resolved.Managed.Status
	status.AgentUpdateAvailable = false
	status.ConfiguredEndpointURL = status.LatestEndpointURL
	status.UpdateAvailable = false
	writeJSON(w, http.StatusOK, agentMCPServerSourceSyncResponse{
		Agent: agent.MCPServersView{
			AgentID:     updated.ID,
			RuntimeKind: updated.Status.RuntimeKind,
			Servers:     serviceMCPServers(updated.Spec.MCPServers),
		},
		Source: status,
	})
}

func (h *Handler) resolveAgentMCPServerSource(ctx context.Context, agentID, name string) (resolvedAgentMCPServerSource, error) {
	current, err := h.agentEngine.Agents().Get(ctx, agentID, agentengine.AgentGetOptions{AdoptMCPServers: true})
	if err != nil {
		return resolvedAgentMCPServerSource{}, err
	}
	raw, ok := current.Spec.MCPServers[name]
	if !ok {
		return resolvedAgentMCPServerSource{}, fmt.Errorf("mcp server not found: %s", name)
	}
	config := map[string]any(raw)
	managed, err := h.resolveManagedMCPServerSource(ctx, config)
	if err != nil {
		unavailable, missing := unavailableManagedMCPServerSource(config, err)
		if !missing {
			return resolvedAgentMCPServerSource{}, err
		}
		return resolvedAgentMCPServerSource{
			Agent:       current,
			AgentName:   name,
			Managed:     unavailable,
			SourceError: err,
		}, nil
	}
	managed.Status.AgentUpdateAvailable = managedMCPRuntimeSnapshotChanged(config, managed.Refreshed)
	managed.Status.UpdateAvailable = managed.Status.AgentUpdateAvailable
	return resolvedAgentMCPServerSource{
		Agent:     current,
		AgentName: name,
		Managed:   managed,
	}, nil
}
