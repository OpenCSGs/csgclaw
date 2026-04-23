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
		opts := []boxlitecli.ProviderOption{boxlitecli.WithPath(cfg.BoxLiteCLIPath)}
		for _, registry := range cfg.DebianRegistries {
			opts = append(opts, boxlitecli.WithRegistry(registry))
		}
		return agent.WithSandboxProvider(boxlitecli.NewProvider(opts...)), nil
	})
}
