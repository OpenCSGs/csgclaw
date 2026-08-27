package agent

import "csgclaw/internal/knowledgebase"

func (s *Service) materializeRuntimeMCPServers(runtimeKind string, servers map[string]any) (map[string]any, error) {
	if servers == nil {
		return cloneMCPServers(servers), nil
	}
	return knowledgebase.RuntimeServers(servers)
}
