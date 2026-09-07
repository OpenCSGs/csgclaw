package agents

import (
	"context"

	"csgclaw/internal/knowledgebase"
)

func (s *Controller) materializeRuntimeMCPServers(ctx context.Context, runtimeKind string, servers map[string]any) (map[string]any, error) {
	if servers == nil {
		return cloneMCPServers(servers), nil
	}
	return knowledgebase.RuntimeServers(servers)
}
