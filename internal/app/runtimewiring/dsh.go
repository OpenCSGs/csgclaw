package runtimewiring

import (
	"context"
	"fmt"
	"strings"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/dshcli"
	agentruntime "csgclaw/internal/runtime"
	runtimedsh "csgclaw/internal/runtime/dsh"
)

func WithDSHRuntime() agent.ControllerOption {
	return func(s *agent.Controller) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		host := s.PicoClawRuntimeHost()
		rt := runtimedsh.New(runtimedsh.Dependencies{
			ResolveBinary: func(ctx context.Context, explicit string) (dshcli.Info, error) {
				return (dshcli.Provider{ExplicitPath: explicit}).Resolve(ctx)
			},
			MaterializeMCPServers: func(ctx context.Context, servers map[string]any) (map[string]any, error) {
				return s.MaterializeRuntimeMCPServers(ctx, agent.RuntimeKindDSH, servers)
			},
			ResolveAgent: func(h agentruntime.Handle) (runtimedsh.AgentRef, error) {
				got, err := host.ResolveAgent(h)
				if err != nil {
					return runtimedsh.AgentRef{}, err
				}
				profile, err := host.ResolveRuntimeProfile(h)
				if err != nil {
					return runtimedsh.AgentRef{}, err
				}
				return runtimedsh.AgentRef{
					ID:             got.ID,
					Name:           got.Name,
					RuntimeID:      strings.TrimSpace(got.RuntimeID),
					Instructions:   got.Instructions,
					RuntimeOptions: got.RuntimeOptions,
					MCPServers:     got.MCPServers,
					Profile:        profile,
				}, nil
			},
			AgentHome: host.AgentHome,
		})
		return agent.WithRuntime(rt)(s)
	}
}
