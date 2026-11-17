package dsh

import (
	"context"
	"crypto/rand"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"csgclaw/internal/identity"
	agentruntime "csgclaw/internal/runtime"
)

const agentMCPServerName = "csgclaw"

// AppMCPSessionHeader carries an opaque binding issued when an ACP session
// mounts the built-in MCP server. Agent authentication is still required.
const AppMCPSessionHeader = "X-CSGClaw-DSH-Session-Token"

func (p *process) sessionMCPServers() ([]acpMCPServer, string) {
	for index, server := range p.mcp {
		if server.Name != agentMCPServerName {
			continue
		}
		token := rand.Text()
		servers := slices.Clone(p.mcp)
		servers[index].Headers = append(slices.Clone(server.Headers), acpNameValue{Name: AppMCPSessionHeader, Value: token})
		return servers, token
	}
	return p.mcp, ""
}

// AgentAppMCPTurnContext resolves only the currently running turn of the
// bound ACP session. Pending work leases never take part in this lookup.
func (r *Runtime) AgentAppMCPTurnContext(agentID, token string) (context.Context, string, bool) {
	if token == "" {
		return nil, "", false
	}
	agentID = identity.CanonicalAgentID(agentID)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, proc := range r.processes {
		proc.mu.Lock()
		if proc.meta.AgentID == agentID {
			for sessionID, turn := range proc.active {
				if proc.appMCPSessionTokens[sessionID] == token && turn.ctx.Err() == nil {
					proc.mu.Unlock()
					return turn.ctx, string(turn.request.ConversationKey), true
				}
			}
		}
		proc.mu.Unlock()
	}
	return nil, "", false
}

func (r *Runtime) SetAgentMCPRevisionSource(source func(string) uint64) {
	r.mcpCatalogMu.Lock()
	r.mcpCatalogRevisionSource = source
	r.mcpCatalogMu.Unlock()
}

func (r *Runtime) agentMCPRevision(agentID string) uint64 {
	agentID = identity.CanonicalAgentID(agentID)
	r.mcpCatalogMu.RLock()
	revision := r.mcpCatalogRevisions[agentID]
	source := r.mcpCatalogRevisionSource
	r.mcpCatalogMu.RUnlock()
	if source != nil {
		revision = max(revision, source(agentID))
	}
	return revision
}

func (r *Runtime) runtimeMCPServers(ctx context.Context, agentID string, profile agentruntime.Profile, manual map[string]any) (map[string]any, uint64, error) {
	servers := manual
	if r.deps.MaterializeMCPServers != nil {
		var err error
		servers, err = r.deps.MaterializeMCPServers(ctx, manual)
		if err != nil {
			return nil, 0, err
		}
	}
	revision := r.agentMCPRevision(identity.CanonicalAgentID(agentID))
	projected, err := projectAgentMCP(profile, servers, revision)
	return projected, revision, err
}

// The App endpoint uses the same Agent-scoped credential as Codex. DSH ACP
// requires explicit HTTP headers instead of Codex's bearer-token env option.
func projectAgentMCP(profile agentruntime.Profile, manual map[string]any, revision uint64) (map[string]any, error) {
	agentID := strings.TrimSpace(profile.Env["CSGCLAW_CALLER_AGENT_ID"])
	baseURL := strings.TrimRight(strings.TrimSpace(profile.Env["CSGCLAW_BASE_URL"]), "/")
	token := strings.TrimSpace(profile.Env["CSGCLAW_ACCESS_TOKEN"])
	if agentID == "" || baseURL == "" || token == "" {
		return manual, nil
	}
	if _, exists := manual[agentMCPServerName]; exists {
		return nil, fmt.Errorf("mcpServers.%s is reserved for Agent Apps", agentMCPServerName)
	}
	servers := maps.Clone(manual)
	if servers == nil {
		servers = make(map[string]any)
	}
	servers[agentMCPServerName] = map[string]any{
		"url":     baseURL + "/api/v1/agents/" + url.PathEscape(agentID) + "/mcp?catalog_revision=" + strconv.FormatUint(revision, 10),
		"headers": map[string]any{"Authorization": "Bearer " + token},
	}
	return servers, nil
}

// DSH ACP binds MCP servers when a session is created or resumed. Restarting
// an idle process preserves the session log while remounting the current App
// catalog. Active turns finish before the next turn performs the refresh.
func (r *Runtime) RefreshAgentMCP(ctx context.Context, agentID string, revision uint64) error {
	agentID = identity.CanonicalAgentID(agentID)
	r.mcpCatalogMu.Lock()
	if r.mcpCatalogRevisions == nil {
		r.mcpCatalogRevisions = make(map[string]uint64)
	}
	r.mcpCatalogRevisions[agentID] = max(r.mcpCatalogRevisions[agentID], revision)
	r.mcpCatalogMu.Unlock()
	r.mu.Lock()
	var runtimeID string
	for id, proc := range r.processes {
		if proc.meta.AgentID == agentID {
			runtimeID = id
			break
		}
	}
	r.mu.Unlock()
	if runtimeID == "" {
		return nil
	}
	return r.ensureAgentMCPCurrent(ctx, runtimeID)
}

func (r *Runtime) ensureAgentMCPCurrent(ctx context.Context, runtimeID string) error {
	r.mcpRefreshMu.Lock()
	defer r.mcpRefreshMu.Unlock()
	return r.ensureAgentMCPCurrentLocked(ctx, runtimeID)
}

func (r *Runtime) processForTurn(ctx context.Context, runtimeID string) (*process, func(), error) {
	r.mcpRefreshMu.Lock()
	defer r.mcpRefreshMu.Unlock()
	if err := r.ensureAgentMCPCurrentLocked(ctx, runtimeID); err != nil {
		return nil, nil, err
	}
	proc, err := r.process(runtimeID)
	if err != nil {
		return nil, nil, err
	}
	proc.mu.Lock()
	proc.inFlightTurns++
	proc.mu.Unlock()
	release := func() {
		proc.mu.Lock()
		proc.inFlightTurns--
		proc.mu.Unlock()
	}
	return proc, release, nil
}

func (r *Runtime) ensureAgentMCPCurrentLocked(ctx context.Context, runtimeID string) error {
	r.mu.Lock()
	proc := r.processes[runtimeID]
	r.mu.Unlock()
	if proc == nil || proc.mcpCatalogRevision == r.agentMCPRevision(proc.meta.AgentID) {
		return nil
	}
	proc.mu.Lock()
	active := proc.inFlightTurns > 0
	proc.mu.Unlock()
	if active {
		return nil
	}
	h := agentruntime.Handle{RuntimeID: runtimeID}
	if _, err := r.Stop(ctx, h); err != nil {
		return fmt.Errorf("stop DSH for App catalog refresh: %w", err)
	}
	if _, err := r.Start(ctx, h); err != nil {
		return fmt.Errorf("restart DSH for App catalog refresh: %w", err)
	}
	return nil
}
