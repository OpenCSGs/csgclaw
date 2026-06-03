package sandboxproviders

import (
	"csgclaw/internal/config"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/csghub"
)

func init() {
	Register(config.CSGHubProvider, func(cfg config.SandboxConfig) (sandbox.Provider, error) {
		return csghub.NewProvider(csghub.WithPVCMountSubpathPrefix(cfg.StoragePath)), nil
	})
}
