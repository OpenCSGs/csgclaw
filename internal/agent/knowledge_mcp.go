package agent

import (
	"context"

	"csgclaw/internal/knowledgebase"
)

func (s *Service) materializeRuntimeMCPServers(ctx context.Context, runtimeKind string, servers map[string]any) (map[string]any, error) {
	if servers == nil {
		return cloneMCPServers(servers), nil
	}
	hydrated, err := knowledgebase.HydrateTemplateServers(ctx, servers)
	if err != nil {
		return nil, err
	}
	return knowledgebase.RuntimeServers(hydrated)
}
