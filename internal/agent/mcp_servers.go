package agent

import (
	"context"
	"fmt"
	"strings"

	"csgclaw/internal/utils"
)

type MCPServersView struct {
	AgentID     string         `json:"agent_id"`
	RuntimeKind string         `json:"runtime_kind"`
	Servers     map[string]any `json:"servers"`
}

// cloneMCPServers preserves the distinction between no managed MCP state and
// an explicitly managed empty server map. Generic map helpers intentionally
// collapse empty maps, which would turn a user-initiated clear into an
// unmanaged state after a save or reload.
func cloneMCPServers(servers map[string]any) map[string]any {
	if servers == nil {
		return nil
	}
	if len(servers) == 0 {
		return map[string]any{}
	}
	return utils.CloneAnyMap(servers)
}

func (s *Service) MCPServersView(ctx context.Context, id string) (MCPServersView, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return MCPServersView{}, fmt.Errorf("agent id is required")
	}
	got, ok := s.agentSnapshot(id)
	if !ok {
		return MCPServersView{}, fmt.Errorf("agent %q not found", id)
	}
	runtimeKind := strings.TrimSpace(got.RuntimeKind)
	view := MCPServersView{
		AgentID:     got.ID,
		RuntimeKind: runtimeKind,
	}
	servers, err := s.currentMCPServersForManagement(ctx, got)
	if err != nil {
		view.Servers = cloneMCPServers(got.MCPServers)
		return view, nil
	}
	view.Servers = cloneMCPServers(servers)
	return view, nil
}
