package codex

import (
	"context"
	"fmt"
	"maps"
	"net/url"
	"strconv"
	"strings"

	agentruntime "csgclaw/internal/runtime"
)

const (
	AgentMCPServerName  = "csgclaw"
	agentAccessTokenEnv = "CSGCLAW_ACCESS_TOKEN"
)

// SetAgentMCPRevisionSource binds the durable App catalog after services have
// been constructed. The source is read on both startup and every prompt.
func (r *Runtime) SetAgentMCPRevisionSource(source func(string) uint64) {
	r.mcpCatalogMu.Lock()
	r.mcpCatalogRevisionSource = source
	r.mcpCatalogMu.Unlock()
}

func (r *Runtime) agentMCPRevision(agentID string) uint64 {
	r.mcpCatalogMu.RLock()
	revision := r.mcpCatalogRevisions[agentID]
	source := r.mcpCatalogRevisionSource
	r.mcpCatalogMu.RUnlock()
	if source != nil {
		revision = max(revision, source(agentID))
	}
	return revision
}

// projectAgentMCP preserves manual configuration while adding the platform
// connection only to the runtime projection, never to the Agent's MCP state.
func (r *Runtime) projectAgentMCP(profile agentruntime.Profile, manual map[string]any, current string) (map[string]any, error) {
	agentID := strings.TrimSpace(profile.Env["CSGCLAW_CALLER_AGENT_ID"])
	baseURL := strings.TrimRight(strings.TrimSpace(profile.Env["CSGCLAW_BASE_URL"]), "/")
	if agentID == "" || baseURL == "" || strings.TrimSpace(profile.Env[agentAccessTokenEnv]) == "" {
		return manual, nil
	}
	servers := maps.Clone(manual)
	if manual == nil {
		var err error
		servers, err = parseCodexMCPServers(current)
		if err != nil {
			return nil, fmt.Errorf("preserve manual MCP configuration: %w", err)
		}
	}
	if servers == nil {
		servers = make(map[string]any)
	}
	// The platform catalog is essential to every Agent turn. Optional MCP startup
	// can omit a slow or refreshing catalog and make connected tools look absent.
	servers[AgentMCPServerName] = map[string]any{
		"url":                  baseURL + "/api/v1/agents/" + url.PathEscape(agentID) + "/mcp?catalog_revision=" + strconv.FormatUint(r.agentMCPRevision(agentID), 10),
		"bearer_token_env_var": agentAccessTokenEnv,
		"enabled":              true,
		"required":             true,
	}
	return servers, nil
}

// RefreshAgentMCP updates a live app-server without replacing its threads.
// Stopped Agents pick up the latest revision on startup. A failed reload stays
// pending and is retried before the next turn.
func (r *Runtime) RefreshAgentMCP(ctx context.Context, agentID string, revision uint64) error {
	agentID = canonicalRuntimeAgentID(agentID)
	r.mcpCatalogMu.Lock()
	if r.mcpCatalogRevisions == nil {
		r.mcpCatalogRevisions = make(map[string]uint64)
	}
	r.mcpCatalogRevisions[agentID] = max(r.mcpCatalogRevisions[agentID], revision)
	r.mcpCatalogMu.Unlock()
	manager, ok := r.currentSessionManager().(*appServerManager)
	if !ok {
		return nil
	}
	manager.mu.RLock()
	var handle SessionHandle
	for runtimeID, live := range manager.sessions {
		if live.spec.AgentID == agentID {
			handle.RuntimeID = runtimeID
			break
		}
	}
	manager.mu.RUnlock()
	if handle.RuntimeID == "" {
		return nil
	}
	return r.refreshSessionMCP(ctx, handle)
}

func (r *Runtime) refreshSessionMCP(ctx context.Context, handle SessionHandle) error {
	manager, ok := r.currentSessionManager().(*appServerManager)
	if !ok {
		return nil
	}
	manager.mu.RLock()
	live := manager.sessions[handle.RuntimeID]
	manager.mu.RUnlock()
	if live == nil {
		return nil
	}
	live.mcpReloadMu.Lock()
	defer live.mcpReloadMu.Unlock()
	revision := r.agentMCPRevision(live.spec.AgentID)
	if live.mcpCatalogRevision == revision {
		return nil
	}
	ref, err := r.resolveAgent(agentruntime.Handle{RuntimeID: handle.RuntimeID})
	if err != nil {
		return err
	}
	servers, err := r.runtimeMCPServers(ctx, live.spec.AgentID, ref.MCPServers)
	if err != nil {
		return fmt.Errorf("materialize Codex MCP servers for catalog refresh: %w", err)
	}
	if err := r.seedCodexHomeConfig(live.spec.CodexHomeDir, live.spec.WorkspaceDir, ref.Profile.Normalized(), ref.RuntimeOptions, servers); err != nil {
		return err
	}
	if _, err := live.appClient.request(ctx, "config/mcpServer/reload", map[string]any{}); err != nil {
		return fmt.Errorf("refresh Agent MCP catalog: %w", err)
	}
	live.mcpCatalogRevision = revision
	return nil
}
