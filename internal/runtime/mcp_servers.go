package runtime

import (
	"context"
	"reflect"

	"csgclaw/internal/mcpschema"
)

// MCPServersSnapshot is the runtime-facing view of an agent's MCP servers.
type MCPServersSnapshot struct {
	Servers map[string]any
}

type MCPServersChange struct {
	Previous MCPServersSnapshot
	Current  MCPServersSnapshot
}

// MCPServersController is implemented by runtimes that can validate and
// determine restart requirements for their native MCP configuration.
type MCPServersController interface {
	ValidateMCPServers(ctx context.Context, current MCPServersSnapshot) error
	MCPServersRestartRequired(change MCPServersChange) (bool, error)
}

// MCPServersReconciler is implemented by runtimes whose MCP configuration can
// be safely applied to a live runtime. Runtimes such as OpenClaw apply MCP
// configuration only as part of provisioning or recreation.
type MCPServersReconciler interface {
	ReconcileMCPServers(ctx context.Context, h Handle, change MCPServersChange) error
}

// MCPServersListController is implemented by runtimes that can read their
// active MCP configuration back into the shared MCP schema.
type MCPServersListController interface {
	ListMCPServers(ctx context.Context, h Handle, current MCPServersSnapshot) (MCPServersSnapshot, error)
}

// MCPServersNeedsRestart compares the schema-normalized configurations. It is
// runtime lifecycle policy; concrete runtimes may implement stricter rules.
func MCPServersNeedsRestart(previous, current map[string]any) (bool, error) {
	previousNormalized, previousErr := mcpschema.NormalizeMCPServers(previous)
	currentNormalized, currentErr := mcpschema.NormalizeMCPServers(current)
	if currentErr != nil {
		return false, currentErr
	}
	if previousErr != nil {
		return true, nil
	}
	return !reflect.DeepEqual(previousNormalized, currentNormalized), nil
}
