package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	agentruntime "csgclaw/internal/runtime"
)

const ServersKey = agentruntime.MCPServersKey

func normalizeServerInput(name string, config map[string]any) (string, map[string]any, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, fmt.Errorf("mcp server name is required")
	}
	if config == nil {
		return "", nil, fmt.Errorf("mcp server config is required")
	}
	if _, wrapped := config[ServersKey]; wrapped {
		return "", nil, fmt.Errorf("mcp server config must be a single server object, not an %s map", ServersKey)
	}

	normalized, err := agentruntime.NormalizeMCPServers(map[string]any{name: config})
	if err != nil {
		return "", nil, err
	}
	normalizedServer, ok := normalized[name].(map[string]any)
	if !ok {
		return "", nil, fmt.Errorf("mcp server config for %q must be an object", name)
	}
	return name, cloneMap(normalizedServer), nil
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		out := make(map[string]any, len(value))
		for key, item := range value {
			out[key] = item
		}
		return out
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}
