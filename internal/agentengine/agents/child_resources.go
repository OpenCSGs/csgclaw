package agents

import "context"

// SetAgentResourceCleanup binds independently owned App installations to actual
// Agent deletion. Runtime stop/replacement never invokes this callback.
func (s *Controller) SetAgentResourceCleanup(cleanup func(context.Context, string) error) {
	s.mu.Lock()
	s.agentResourceCleanup = cleanup
	s.mu.Unlock()
}
