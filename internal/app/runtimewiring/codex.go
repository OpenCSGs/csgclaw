package runtimewiring

import (
	"fmt"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/codexcli"
	agentruntime "csgclaw/internal/runtime"
	runtimecodex "csgclaw/internal/runtime/codex"
)

func WithCodexRuntime() agent.ServiceOption {
	return func(s *agent.Service) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}

		host := s.PicoClawRuntimeHost()
		events := runtimecodex.NewEventSink()
		permissions := runtimecodex.NewPermissionBroker(events)
		userInputs := runtimecodex.NewUserInputBroker(events)
		rt := runtimecodex.New(runtimecodex.Dependencies{
			BinaryProvider: codexcli.Provider{},
			ResolveAgent: func(h agentruntime.Handle) (runtimecodex.AgentRef, error) {
				got, err := host.ResolveAgent(h)
				if err != nil {
					return runtimecodex.AgentRef{}, err
				}
				profile, err := host.ResolveRuntimeProfile(h)
				if err != nil {
					return runtimecodex.AgentRef{}, err
				}
				return runtimecodex.AgentRef{
					ID:             got.ID,
					Name:           got.Name,
					RuntimeID:      strings.TrimSpace(got.RuntimeID),
					HandleID:       strings.TrimSpace(got.BoxID),
					Instructions:   got.Instructions,
					RuntimeOptions: got.RuntimeOptions,
					MCPServers:     got.MCPServers,
					Profile:        profile,
				}, nil
			},
			AgentHome:  host.AgentHome,
			EventSink:  events,
			Permission: permissions,
			UserInput:  userInputs,
		})
		return agent.WithRuntime(rt)(s)
	}
}
