package sandboxproviders

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/sandbox"
)

// providerFactory converts config into a concrete sandbox provider. Providers
// register themselves from init funcs.
type providerFactory func(config.SandboxConfig) (sandbox.Provider, error)

var factories = map[string]providerFactory{}

// Register adds a sandbox provider that is compiled into the current binary.
// Sandbox implementations should register unconditionally so they ship in all
// builds unless a future provider introduces an explicit compile-time gate.
func Register(name string, factory providerFactory) {
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

// Provider resolves the configured sandbox provider against the set of
// providers compiled into the current binary.
func Provider(cfg config.SandboxConfig) (sandbox.Provider, error) {
	cfg = cfg.Resolved()
	factory, ok := factories[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("unsupported sandbox provider %q; supported values are %s", cfg.Provider, SupportedProvidersText())
	}
	provider, err := factory(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.Provider != config.DockerProvider && cfg.Provider != config.BoxLiteProvider {
		return provider, nil
	}
	return deferredAvailabilityProvider{
		cfg:      cfg,
		provider: provider,
	}, nil
}

// deferredAvailabilityProvider keeps host-only runtimes usable when the
// configured local sandbox CLI is not installed. Availability is checked at
// the first operation that actually needs the sandbox instead of while the
// Agent service is starting.
type deferredAvailabilityProvider struct {
	cfg      config.SandboxConfig
	provider sandbox.Provider
}

func (p deferredAvailabilityProvider) Name() string {
	return p.provider.Name()
}

func (p deferredAvailabilityProvider) Open(ctx context.Context, homeDir string) (sandbox.Runtime, error) {
	if err := Availability(p.cfg); err != nil {
		return nil, err
	}
	return p.provider.Open(ctx, homeDir)
}

func (p deferredAvailabilityProvider) ListImages(ctx context.Context, homeDir string) ([]string, error) {
	if err := Availability(p.cfg); err != nil {
		return nil, err
	}
	return p.provider.ListImages(ctx, homeDir)
}

func (p deferredAvailabilityProvider) CheckAvailability(ctx context.Context) error {
	if err := Availability(p.cfg); err != nil {
		return err
	}
	checker, ok := p.provider.(sandbox.AvailabilityChecker)
	if !ok {
		return nil
	}
	return checker.CheckAvailability(ctx)
}

// ServiceOptions resolves the configured sandbox provider against the set of
// providers compiled into the current binary.
func ServiceOptions(cfg config.SandboxConfig) ([]agent.ServiceOption, error) {
	provider, err := Provider(cfg)
	if err != nil {
		return nil, err
	}
	return []agent.ServiceOption{
		agent.WithSandboxProvider(provider),
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
