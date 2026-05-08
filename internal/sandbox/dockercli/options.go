package dockercli

import (
	"strings"
)

const defaultCLIPath = "docker"

type ProviderOption func(*Provider)

func WithPath(path string) ProviderOption {
	return func(p *Provider) {
		p.path = strings.TrimSpace(path)
	}
}

func WithRunner(runner Runner) ProviderOption {
	return func(p *Provider) {
		if runner != nil {
			p.runner = runner
		}
	}
}

func resolvePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return defaultCLIPath
	}
	return path
}
