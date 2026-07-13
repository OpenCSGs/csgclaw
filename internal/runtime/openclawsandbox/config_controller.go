package openclawsandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	agentruntime "csgclaw/internal/runtime"
)

var _ agentruntime.RuntimeConfigController = (*Runtime)(nil)
var _ agentruntime.MCPServersController = (*Runtime)(nil)
var _ agentruntime.MCPServersListController = (*Runtime)(nil)

func (r *Runtime) ValidateConfig(_ context.Context, current agentruntime.RuntimeConfigSnapshot) error {
	return nil
}

func (r *Runtime) RestartRequired(change agentruntime.RuntimeConfigChange) (bool, error) {
	return false, nil
}

func (r *Runtime) ReconcileConfig(_ context.Context, h agentruntime.Handle, _ agentruntime.RuntimeConfigChange) error {
	agentRef, err := r.ResolveAgentForHandle(h)
	if err != nil {
		return err
	}
	agentHome, err := r.AgentHomeForAgentID(agentRef.ID)
	if err != nil {
		return err
	}
	return refreshWorkspaceAgentsFile(filepath.Join(r.Layout(agentHome).WorkspaceRoot, "AGENTS.md"), agentRef.Instructions)
}

func (r *Runtime) ValidateMCPServers(_ context.Context, current agentruntime.MCPServersSnapshot) error {
	return validateOpenClawMCPServers(current.Servers)
}

func (r *Runtime) MCPServersRestartRequired(change agentruntime.MCPServersChange) (bool, error) {
	return openClawMCPRestartRequired(change.Previous.Servers, change.Current.Servers)
}

func (r *Runtime) ListMCPServers(_ context.Context, h agentruntime.Handle, _ agentruntime.MCPServersSnapshot) (agentruntime.MCPServersSnapshot, error) {
	agentHome, err := r.AgentHomeForHandle(h)
	if err != nil {
		return agentruntime.MCPServersSnapshot{}, err
	}
	return readOpenClawMCPServers(filepath.Join(Root(agentHome), HostConfig))
}

func readOpenClawMCPServers(path string) (agentruntime.MCPServersSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return agentruntime.MCPServersSnapshot{}, nil
		}
		return agentruntime.MCPServersSnapshot{}, fmt.Errorf("read openclaw mcp config: %w", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return agentruntime.MCPServersSnapshot{}, fmt.Errorf("decode openclaw mcp config: %w", err)
	}
	rawMCPRoot, ok := cfg["mcp"]
	if !ok || rawMCPRoot == nil {
		return agentruntime.MCPServersSnapshot{}, nil
	}
	mcpRoot, ok := rawMCPRoot.(map[string]any)
	if !ok {
		return agentruntime.MCPServersSnapshot{}, fmt.Errorf("openclaw mcp config mcp must be an object")
	}
	rawServers, ok := mcpRoot["servers"]
	if !ok || rawServers == nil {
		return agentruntime.MCPServersSnapshot{}, nil
	}
	servers, ok := rawServers.(map[string]any)
	if !ok {
		return agentruntime.MCPServersSnapshot{}, fmt.Errorf("openclaw mcp config servers must be an object")
	}
	normalized, err := agentruntime.NormalizeMCPServers(servers)
	if err != nil {
		return agentruntime.MCPServersSnapshot{}, err
	}
	return agentruntime.MCPServersSnapshot{Servers: normalized}, nil
}
