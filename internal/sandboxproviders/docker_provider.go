package sandboxproviders

import (
	"csgclaw/internal/config"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/dockercli"
)

func init() {
	Register(config.DockerProvider, func(cfg config.SandboxConfig) (sandbox.Provider, error) {
		if err := Availability(config.SandboxConfig{Provider: config.DockerProvider, DockerCLIPath: cfg.DockerCLIPath}); err != nil {
			return nil, err
		}
		return dockercli.NewProvider(dockercli.WithPath(cfg.EffectiveDockerCLIPath())), nil
	})
}
