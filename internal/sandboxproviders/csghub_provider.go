package sandboxproviders

import (
	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/sandbox/csghub"
)

func init() {
	Register(config.CSGHubProvider, func(config.SandboxConfig, config.BootstrapConfig) (agent.ServiceOption, error) {
		return agent.WithSandboxProvider(csghub.NewProvider()), nil
	})
}
