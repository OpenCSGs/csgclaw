package boxlitecli

import (
	"fmt"
	"sort"
	"strings"

	"csgclaw/internal/sandbox"
)

type ProviderOption func(*Provider)

func WithPath(path string) ProviderOption {
	return func(p *Provider) {
		p.path = path
	}
}

func WithConfig(path string) ProviderOption {
	return func(p *Provider) {
		p.config = path
	}
}

func WithRegistry(registry string) ProviderOption {
	return func(p *Provider) {
		if strings.TrimSpace(registry) != "" {
			p.registries = append(p.registries, registry)
		}
	}
}

func WithRunner(runner Runner) ProviderOption {
	return func(p *Provider) {
		if runner != nil {
			p.runner = runner
		}
	}
}

func runArgs(spec sandbox.CreateSpec) ([]string, error) {
	if strings.TrimSpace(spec.Image) == "" {
		return nil, fmt.Errorf("invalid sandbox image: image is required")
	}
	if len(spec.Entrypoint) > 0 {
		return nil, fmt.Errorf("unsupported sandbox option: entrypoint")
	}

	args := []string{"run"}
	if strings.TrimSpace(spec.Name) != "" {
		args = append(args, "--name", spec.Name)
	}
	if spec.Detach {
		args = append(args, "--detach")
	}
	if spec.AutoRemove {
		args = append(args, "--rm")
	}

	keys := make([]string, 0, len(spec.Env))
	for key := range spec.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid sandbox env: key is required")
		}
		args = append(args, "-e", key+"="+spec.Env[key])
	}

	if runUser := strings.TrimSpace(spec.RunUser); runUser != "" {
		args = append(args, "-u", runUser)
	}

	for _, mount := range spec.Mounts {
		if strings.TrimSpace(mount.HostPath) == "" {
			return nil, fmt.Errorf("invalid sandbox mount: host path is required")
		}
		if strings.TrimSpace(mount.GuestPath) == "" {
			return nil, fmt.Errorf("invalid sandbox mount: guest path is required")
		}
		value := mount.HostPath + ":" + mount.GuestPath
		if mount.ReadOnly {
			value += ":ro"
		}
		args = append(args, "-v", value)
	}

	args = append(args, spec.Image)
	args = append(args, spec.Cmd...)
	return args, nil
}
