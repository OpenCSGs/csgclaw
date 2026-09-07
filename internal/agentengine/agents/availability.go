package agents

import (
	"context"
	agentruntime "csgclaw/internal/runtime"
)

func (s *Controller) checkRuntimeAvailability(ctx context.Context, kind string) error {
	rt, err := s.runtimeForKind(kind)
	if err != nil {
		return err
	}
	if checker, ok := rt.(agentruntime.AvailabilityChecker); ok {
		return checker.CheckAvailability(ctx)
	}
	return nil
}
