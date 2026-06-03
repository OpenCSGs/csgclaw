package sandboxproviders

import (
	"csgclaw/internal/config"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/dockercli"
)

func init() {
	Register(config.DockerProvider, func(cfg config.SandboxConfig) (sandbox.Provider, error) {
		return dockercli.NewProvider(dockercli.WithPath(cfg.EffectiveDockerCLIPath())), nil
	})
}
