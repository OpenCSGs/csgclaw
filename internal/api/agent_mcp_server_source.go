package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/knowledgebase"
)

type agentMCPServerSourceSyncResponse struct {
	Agent       agent.MCPServersView          `json:"agent"`
	GlobalState map[string]any                `json:"global_state"`
	Source      mcpServerSourceStatusResponse `json:"source"`
}

type resolvedAgentMCPServerSource struct {
	Agent           agentengine.Agent
	AgentName       string
	GlobalName      string
	GlobalRefreshed map[string]any
	Managed         resolvedManagedMCPServerSource
}

func (h *Handler) handleAgentMCPServerSource(w http.ResponseWriter, r *http.Request) {
	if h.agentEngine == nil {
		http.Error(w, "agent service is not configured", http.StatusServiceUnavailable)
		return
	}
	if h.mcp == nil {
		http.Error(w, "mcp service is not configured", http.StatusServiceUnavailable)
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

	globalState, globalName, err := h.persistManagedMCPGlobalSnapshot(r.Context(), resolved)
	if err != nil {
		writeMCPServerError(w, err)
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
	status.GlobalServerName = globalName
	status.GlobalUpdateAvailable = false
	status.UpdateAvailable = false
	writeJSON(w, http.StatusOK, agentMCPServerSourceSyncResponse{
		Agent: agent.MCPServersView{
			AgentID:     updated.ID,
			RuntimeKind: updated.Status.RuntimeKind,
			Servers:     serviceMCPServers(updated.Spec.MCPServers),
		},
		GlobalState: globalState,
		Source:      status,
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
		return resolvedAgentMCPServerSource{}, err
	}
	servers, err := h.mcp.ListServers(ctx)
	if err != nil {
		return resolvedAgentMCPServerSource{}, err
	}
	metadata, _ := knowledgebase.ManagedMetadataFromServer(config)
	globalName := knowledgebase.FindConfiguredServer(servers, metadata.ContentID)
	globalUpdateAvailable := globalName == ""
	var globalRefreshed map[string]any
	if globalName != "" {
		globalConfig, ok := servers[globalName].(map[string]any)
		if !ok {
			return resolvedAgentMCPServerSource{}, fmt.Errorf("mcp server %q config must be an object", globalName)
		}
		globalRefreshed, err = knowledgebase.RefreshManagedServerConfig(globalConfig, managed.Item, managed.Connection.CSGHubAccessToken)
		if err != nil {
			return resolvedAgentMCPServerSource{}, err
		}
		globalUpdateAvailable = managedMCPRuntimeSnapshotChanged(globalConfig, globalRefreshed)
	}
	managed.Status.AgentUpdateAvailable = managedMCPRuntimeSnapshotChanged(config, managed.Refreshed)
	managed.Status.GlobalServerName = globalName
	managed.Status.GlobalUpdateAvailable = globalUpdateAvailable
	managed.Status.UpdateAvailable = managed.Status.AgentUpdateAvailable || globalUpdateAvailable
	return resolvedAgentMCPServerSource{
		Agent:           current,
		AgentName:       name,
		GlobalName:      globalName,
		GlobalRefreshed: globalRefreshed,
		Managed:         managed,
	}, nil
}

func (h *Handler) persistManagedMCPGlobalSnapshot(ctx context.Context, resolved resolvedAgentMCPServerSource) (map[string]any, string, error) {
	if resolved.GlobalName != "" {
		state, err := h.mcp.UpdateServer(ctx, resolved.GlobalName, resolved.GlobalName, resolved.GlobalRefreshed)
		return state, resolved.GlobalName, err
	}
	state, err := h.mcp.CreateServer(ctx, resolved.Managed.CanonicalName, resolved.Managed.CanonicalConfig)
	return state, resolved.Managed.CanonicalName, err
}
