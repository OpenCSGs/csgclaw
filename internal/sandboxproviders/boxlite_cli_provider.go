package sandboxproviders

import (
	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/sandbox/boxlitecli"
)

// Non-SDK sandbox providers register unconditionally so they remain available
// in every csgclaw build, including binaries compiled without boxlite_sdk.
func init() {
	Register(config.BoxLiteCLIProvider, func(cfg config.SandboxConfig) (agent.ServiceOption, error) {
		return agent.WithSandboxProvider(boxlitecli.NewProvider(boxlitecli.WithPath(cfg.BoxLiteCLIPath))), nil
	})
}
