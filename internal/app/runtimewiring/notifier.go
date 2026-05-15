package runtimewiring

import (
	"context"
	"fmt"

	"csgclaw/internal/agent"
	runtimenotifier "csgclaw/internal/runtime/notifier"
	notifierpull "csgclaw/internal/runtime/notifier/pull"
)

func WithNotifierRuntime() agent.ServiceOption {
	return func(s *agent.Service) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		return agent.WithRuntime(runtimenotifier.NewAgentRuntime())(s)
	}
}

// RunNotifierPullSupervisor blocks until ctx is cancelled, reconciling per-agent remote_pull loops.
// Start it from the composition layer (e.g. cli/serve) with go RunNotifierPullSupervisor(ctx, ...).
func RunNotifierPullSupervisor(ctx context.Context, agents *agent.Service, deliver runtimenotifier.Fanouter) {
	if agents == nil || deliver == nil {
		return
	}
	notifierpull.NewSupervisor(agents, deliver).Run(ctx)
}
