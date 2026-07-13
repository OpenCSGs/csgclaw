package picoclawsandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/sandboxgateway"
)

var _ agentruntime.MCPServersController = (*Runtime)(nil)
var _ agentruntime.MCPServersReconciler = (*Runtime)(nil)
var _ agentruntime.MCPServersListController = (*Runtime)(nil)

func (r *Runtime) ValidateMCPServers(_ context.Context, current agentruntime.MCPServersSnapshot) error {
	return agentruntime.ValidateMCPServers(current.Servers)
}

func (r *Runtime) MCPServersRestartRequired(change agentruntime.MCPServersChange) (bool, error) {
	return agentruntime.MCPServersNeedsRestart(change.Previous.Servers, change.Current.Servers)
}

func (r *Runtime) ReconcileMCPServers(_ context.Context, h agentruntime.Handle, change agentruntime.MCPServersChange) error {
	prepared, err := r.PreparedGatewayProvisionForHandle(h)
	if errors.Is(err, sandboxgateway.ErrPreparedGatewayProvisionNotAvailable) {
		return nil
	}
	if err != nil {
		return err
	}
	agentHome, err := r.AgentHomeForHandle(h)
	if err != nil {
		return err
	}
	profile := prepared.Profile.Normalized()
	if profile.ModelID == "" {
		profile.ModelID = prepared.ModelID
	}
	_, err = EnsureConfigWithMCPServers(
		agentHome,
		prepared.ParticipantID,
		prepared.AgentID,
		prepared.Server,
		configModelFromProfile(profile),
		change.Current.Servers,
		fixedBaseURL(prepared.ManagerBaseURL),
		r.CurrentFeishuProvider(),
	)
	return err
}

func (r *Runtime) ListMCPServers(_ context.Context, h agentruntime.Handle, _ agentruntime.MCPServersSnapshot) (agentruntime.MCPServersSnapshot, error) {
	agentHome, err := r.AgentHomeForHandle(h)
	if err != nil {
		return agentruntime.MCPServersSnapshot{}, err
	}
	return readPicoClawMCPServers(filepath.Join(Root(agentHome), HostConfig))
}

func readPicoClawMCPServers(path string) (agentruntime.MCPServersSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return agentruntime.MCPServersSnapshot{}, nil
		}
		return agentruntime.MCPServersSnapshot{}, fmt.Errorf("read picoclaw mcp config: %w", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return agentruntime.MCPServersSnapshot{}, fmt.Errorf("decode picoclaw mcp config: %w", err)
	}
	rawTools, ok := cfg["tools"]
	if !ok || rawTools == nil {
		return agentruntime.MCPServersSnapshot{}, nil
	}
	tools, ok := rawTools.(map[string]any)
	if !ok {
		return agentruntime.MCPServersSnapshot{}, fmt.Errorf("picoclaw mcp config tools must be an object")
	}
	rawMCPRoot, ok := tools["mcp"]
	if !ok || rawMCPRoot == nil {
		return agentruntime.MCPServersSnapshot{}, nil
	}
	mcpRoot, ok := rawMCPRoot.(map[string]any)
	if !ok {
		return agentruntime.MCPServersSnapshot{}, fmt.Errorf("picoclaw mcp config tools.mcp must be an object")
	}
	rawServers, ok := mcpRoot["servers"]
	if !ok || rawServers == nil {
		if enabled, _ := mcpRoot["enabled"].(bool); enabled {
			normalized, err := agentruntime.NormalizeMCPServers(map[string]any{})
			if err != nil {
				return agentruntime.MCPServersSnapshot{}, err
			}
			return agentruntime.MCPServersSnapshot{Servers: normalized}, nil
		}
		return agentruntime.MCPServersSnapshot{}, nil
	}
	servers, ok := rawServers.(map[string]any)
	if !ok {
		return agentruntime.MCPServersSnapshot{}, fmt.Errorf("picoclaw mcp config servers must be an object")
	}
	normalized, err := agentruntime.NormalizeMCPServers(servers)
	if err != nil {
		return agentruntime.MCPServersSnapshot{}, err
	}
	return agentruntime.MCPServersSnapshot{Servers: normalized}, nil
}
