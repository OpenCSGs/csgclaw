package agent

import (
	"fmt"
	"strings"
)

// RuntimeConfig is the internal runtime selection model used by core agent logic.
// Legacy runtime kinds remain the compatibility format for storage, APIs, and registries.
type RuntimeConfig struct {
	Name      string
	Sandboxed bool
}

func (c RuntimeConfig) Normalized() RuntimeConfig {
	return RuntimeConfig{
		Name:      normalizeRuntimeName(c.Name),
		Sandboxed: c.Sandboxed,
	}
}

func (c RuntimeConfig) LegacyKind() string {
	return runtimeKindFromNameAndSandbox(c.Name, c.Sandboxed)
}

func runtimeConfigForKind(kind string) RuntimeConfig {
	return RuntimeConfig{
		Name:      runtimeNameForKind(kind),
		Sandboxed: sandboxEnabledForKind(kind),
	}.Normalized()
}

func runtimeConfigFromSelection(kind, name string, sandboxEnabled bool) (RuntimeConfig, error) {
	kind = strings.TrimSpace(kind)
	cfg := RuntimeConfig{Name: name, Sandboxed: sandboxEnabled}.Normalized()
	if kind != "" {
		resolved := runtimeConfigForKind(kind)
		if cfg.Name != "" && resolved.Name != "" && cfg.Name != resolved.Name {
			return RuntimeConfig{}, fmt.Errorf("runtime_kind %q conflicts with runtime_name %q", kind, cfg.Name)
		}
		return resolved, nil
	}
	return cfg, nil
}
