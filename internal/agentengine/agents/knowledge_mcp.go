package agents

import (
	"context"
	"strings"

	"csgclaw/internal/knowledgebase"
	"csgclaw/internal/mcpschema"
	"csgclaw/internal/opencsgmcp"
	"csgclaw/internal/utils"
)

func (s *Controller) materializeRuntimeMCPServers(ctx context.Context, runtimeKind string, servers map[string]any) (map[string]any, error) {
	if servers == nil {
		return cloneMCPServers(servers), nil
	}
	prepared, err := opencsgmcp.RuntimeServers(
		mcpschema.EnabledServers(servers),
		s.mcpProxyBaseURL(runtimeKind),
		strings.TrimSpace(s.server.AccessToken),
	)
	if err != nil {
		return nil, err
	}
	prepared, err = knowledgebase.RuntimeServers(prepared)
	if err != nil {
		return nil, err
	}
	for name, raw := range servers {
		if entry, ok := raw.(map[string]any); ok && !mcpschema.ServerEnabled(entry) {
			entry = utils.CloneAnyMap(entry)
			if meta, ok := entry[opencsgmcp.ManagedMetaKey].(map[string]any); ok {
				delete(meta, opencsgmcp.ManagedMetaNamespace)
				if len(meta) == 0 {
					delete(entry, opencsgmcp.ManagedMetaKey)
				}
			}
			prepared[name] = entry
		}
	}
	mcpschema.StripPresentation(prepared)
	return prepared, nil
}

func (s *Controller) mcpProxyBaseURL(runtimeKind string) string {
	if isHostRuntimeKind(runtimeKind) {
		return ResolveLocalRuntimeBaseURL(s.server)
	}
	return s.resolveManagerBaseURL(s.server)
}

// MaterializeRuntimeMCPServers resolves template-safe MCP declarations into
// runtime-only connections. Runtime adapters use this when rebuilding a live
// session from the persisted Agent, whose MCP configuration intentionally does
// not contain credentials.
func (s *Controller) MaterializeRuntimeMCPServers(ctx context.Context, runtimeKind string, servers map[string]any) (map[string]any, error) {
	return s.materializeRuntimeMCPServers(ctx, runtimeKind, servers)
}
