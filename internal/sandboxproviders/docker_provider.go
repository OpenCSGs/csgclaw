package sandboxproviders

import (
	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/sandbox/dockercli"
)

func init() {
	Register(config.DockerProvider, func(cfg config.SandboxConfig) (agent.ServiceOption, error) {
		return agent.WithSandboxProvider(dockercli.NewProvider(dockercli.WithPath(cfg.EffectiveDockerCLIPath()))), nil
	})
}
