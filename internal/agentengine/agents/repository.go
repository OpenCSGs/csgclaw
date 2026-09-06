package agents

import (
	"csgclaw/internal/config"
	"sync"
)

// Repository owns Agent desired state, Runtime observations and Extension records
// in the existing local store. It never selects, invokes or starts a Runtime.
type Repository struct {
	mu                        sync.RWMutex
	state                     string
	agents                    map[string]Agent
	runtimeRecords            map[string]RuntimeRecord
	profileDefaults           AgentProfile
	detectionResults          []ProfileDetectionResult
	normalizeProfileReference func(AgentProfile) AgentProfile
}

// putAgentLocked changes parent state without replacing independently managed
// child resources. Only RuntimeExtension repository operations can change them.
func (s *Repository) putAgentLocked(id string, item Agent) {
	if s.agents == nil {
		s.agents = make(map[string]Agent)
	}
	if current, exists := s.agents[id]; exists {
		item.RuntimeExtensions = cloneRawMessages(current.RuntimeExtensions)
	}
	s.agents[id] = item
}

func profileCatalogNormalizer(catalog config.LLMConfig) func(AgentProfile) AgentProfile {
	catalog = catalog.Normalized()
	return func(profile AgentProfile) AgentProfile {
		if value, ok := CatalogReferenceProfile(catalog, profile); ok {
			return value
		}
		return profile
	}
}
