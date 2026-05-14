package runtimewiring

import (
	"fmt"

	"csgclaw/internal/agent"
	runtimenotifier "csgclaw/internal/runtime/notifier"
)

func WithNotifierRuntime() agent.ServiceOption {
	return func(s *agent.Service) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		return agent.WithRuntime(runtimenotifier.NewAgentRuntime())(s)
	}
}
