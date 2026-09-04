package agents

import (
	"context"
	"csgclaw/internal/agentengine/lifecycle"
	"fmt"
)

func (s *Controller) LifecycleCoordinator() *lifecycle.Coordinator {
	s.agentLifecycleMu.Lock()
	defer s.agentLifecycleMu.Unlock()
	if s.lifecycle == nil {
		s.lifecycle = &lifecycle.Coordinator{}
	}
	return s.lifecycle
}
func (s *Controller) acquireAgentLifecycle(ctx context.Context, id string) (context.Context, func(), error) {
	if s == nil {
		return ctx, nil, fmt.Errorf("agent controller is required")
	}
	return s.LifecycleCoordinator().Mutation(ctx, canonicalAgentID(id))
}
func (s *Controller) WithAgentLifecycle(ctx context.Context, id string, operation func(context.Context) error) error {
	if s == nil {
		return fmt.Errorf("agent controller is required")
	}
	return s.LifecycleCoordinator().Mutate(ctx, canonicalAgentID(id), operation)
}
