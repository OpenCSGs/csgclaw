package agents

import "csgclaw/internal/agentengine/registry"

// WorkspaceService owns layout lookup, read-only browsing/export and logs.
// It does not expose Agent lifecycle or conversation execution operations.
type WorkspaceService struct {
	*Repository
	registry   *registry.Registry
	agentsRoot string
}

func (s *Controller) Workspace() *WorkspaceService {
	if s == nil {
		return nil
	}
	s.bindResources()
	return &s.WorkspaceService
}

func (s *Controller) bindResources() {
	s.resourcesOnce.Do(func() {
		if s.runtimeRegistry == nil {
			s.runtimeRegistry = &registry.Registry{}
		}
		s.ModelConfiguration.Repository = &s.Repository
		s.WorkspaceService.Repository = &s.Repository
		s.WorkspaceService.registry = s.runtimeRegistry
	})
}
