package sandboxproviders

import (
	"fmt"
	"sort"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
)

// serviceOptionFactory converts config into the agent service option that wires
// a concrete sandbox provider. Providers register themselves from init funcs.
type serviceOptionFactory func(config.SandboxConfig) (agent.ServiceOption, error)

var factories = map[string]serviceOptionFactory{}

// Register adds a sandbox provider that is compiled into the current binary.
// Only the BoxLite SDK-backed provider is gated by a build tag; other sandbox
// implementations should register unconditionally so they ship in all builds.
func Register(name string, factory serviceOptionFactory) {
	name = strings.TrimSpace(name)
	if name == "" {
		panic("sandbox provider name is required")
	}
	if factory == nil {
		panic("sandbox provider factory is required")
	}
	if _, exists := factories[name]; exists {
		panic("sandbox provider already registered: " + name)
	}
	factories[name] = factory
}

// ServiceOptions resolves the configured sandbox provider against the set of
// providers compiled into the current binary.
func ServiceOptions(cfg config.SandboxConfig) ([]agent.ServiceOption, error) {
	cfg = cfg.Resolved()
	factory, ok := factories[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("unsupported sandbox provider %q; supported values are %s", cfg.Provider, SupportedProvidersText())
	}
	provider, err := factory(cfg)
	if err != nil {
		return nil, err
	}
	return []agent.ServiceOption{
		provider,
		agent.WithSandboxHomeDirName(cfg.HomeDirName),
	}, nil
}

// SupportedProviders reports the providers compiled into the current binary.
func SupportedProviders() []string {
	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func SupportedProvidersText() string {
	names := SupportedProviders()
	if len(names) == 0 {
		return "(none compiled in)"
	}
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, fmt.Sprintf("%q", name))
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " or " + quoted[len(quoted)-1]
}
