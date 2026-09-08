package agents

import (
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/agentengine/registry"
	skilllocal "csgclaw/internal/skill/local"
	"errors"
)

func (s *WorkspaceService) SkillSummaries(ctx context.Context, agentID string) ([]contract.SkillSummary, error) {
	layout, err := s.AgentLayout(agentID)
	if err != nil {
		return nil, err
	}
	items, err := skilllocal.ListSummaries(ctx, layout.SkillsRoot)
	if err != nil {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("Agent skill metadata is unavailable")
	}
	out := make([]contract.SkillSummary, 0, len(items))
	for _, item := range items {
		out = append(out, contract.SkillSummary{Name: item.Name, Description: item.Description, Error: item.Error})
	}
	return out, nil
}

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
