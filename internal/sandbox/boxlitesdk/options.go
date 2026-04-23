package boxlitesdk

import (
	"fmt"
	"strings"

	boxlitesdk "github.com/RussellLuo/boxlite/sdks/go"

	"csgclaw/internal/sandbox"
)

type ProviderOption func(*Provider)

func WithRegistries(registries ...string) ProviderOption {
	return func(p *Provider) {
		if p == nil {
			return
		}
		p.registries = normalizeRegistries(registries)
	}
}

func boxOptions(spec sandbox.CreateSpec) ([]boxlitesdk.BoxOption, error) {
	var opts []boxlitesdk.BoxOption
	if strings.TrimSpace(spec.Name) != "" {
		opts = append(opts, boxlitesdk.WithName(spec.Name))
	}
	opts = append(opts,
		boxlitesdk.WithDetach(spec.Detach),
		boxlitesdk.WithAutoRemove(spec.AutoRemove),
	)
	for key, value := range spec.Env {
		if strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid sandbox env: key is required")
		}
		opts = append(opts, boxlitesdk.WithEnv(key, value))
	}
	for _, mount := range spec.Mounts {
		if strings.TrimSpace(mount.HostPath) == "" {
			return nil, fmt.Errorf("invalid sandbox mount: host path is required")
		}
		if strings.TrimSpace(mount.GuestPath) == "" {
			return nil, fmt.Errorf("invalid sandbox mount: guest path is required")
		}
		if mount.ReadOnly {
			opts = append(opts, boxlitesdk.WithVolumeReadOnly(mount.HostPath, mount.GuestPath))
			continue
		}
		opts = append(opts, boxlitesdk.WithVolume(mount.HostPath, mount.GuestPath))
	}
	if len(spec.Entrypoint) > 0 {
		opts = append(opts, boxlitesdk.WithEntrypoint(spec.Entrypoint...))
	}
	if len(spec.Cmd) > 0 {
		opts = append(opts, boxlitesdk.WithCmd(spec.Cmd...))
	}
	return opts, nil
}
