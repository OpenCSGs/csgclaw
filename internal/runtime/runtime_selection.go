package runtime

import (
	"fmt"
	"strings"
)

const (
	NamePicoClaw = "picoclaw"
	NameOpenClaw = "openclaw"
	NameCodex    = "codex"
)

// RuntimeConfig is the internal runtime selection model shared by higher-level
// runtime selection and compatibility logic.
type RuntimeConfig struct {
	Name      string
	Sandboxed bool
}

func (c RuntimeConfig) Normalized() RuntimeConfig {
	return RuntimeConfig{
		Name:      NormalizeRuntimeName(c.Name),
		Sandboxed: c.Sandboxed,
	}
}

func (c RuntimeConfig) LegacyKind() string {
	switch c.Normalized().Name {
	case NamePicoClaw:
		if c.Sandboxed {
			return KindPicoClawSandbox
		}
	case NameOpenClaw:
		if c.Sandboxed {
			return KindOpenClawSandbox
		}
	case NameCodex:
		if !c.Sandboxed {
			return KindCodex
		}
	}
	return ""
}

func (c RuntimeConfig) Kind() string {
	switch c.Normalized().Name {
	case NamePicoClaw:
		if c.Sandboxed {
			return NamePicoClaw
		}
	case NameOpenClaw:
		if c.Sandboxed {
			return NameOpenClaw
		}
	case NameCodex:
		if !c.Sandboxed {
			return KindCodex
		}
	}
	return ""
}

func NormalizeRuntimeName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case NamePicoClaw, KindPicoClawSandbox:
		return NamePicoClaw
	case NameOpenClaw, KindOpenClawSandbox:
		return NameOpenClaw
	case NameCodex:
		return NameCodex
	case "":
		return ""
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

func RuntimeConfigForKind(kind string) RuntimeConfig {
	switch NormalizeRuntimeName(kind) {
	case NamePicoClaw:
		return RuntimeConfig{Name: NamePicoClaw, Sandboxed: true}
	case NameOpenClaw:
		return RuntimeConfig{Name: NameOpenClaw, Sandboxed: true}
	case NameCodex:
		return RuntimeConfig{Name: NameCodex, Sandboxed: false}
	default:
		return RuntimeConfig{
			Name:      NormalizeRuntimeName(kind),
			Sandboxed: false,
		}
	}
}

func SandboxEnabledForKind(kind string) bool {
	return RuntimeConfigForKind(kind).Sandboxed
}

func RuntimeConfigFromSelection(kind, name string, sandboxEnabled bool) (RuntimeConfig, error) {
	kind = strings.TrimSpace(kind)
	cfg := RuntimeConfig{Name: name, Sandboxed: sandboxEnabled}.Normalized()
	if kind != "" {
		resolved := RuntimeConfigForKind(kind)
		if cfg.Name != "" && resolved.Name != "" && cfg.Name != resolved.Name {
			return RuntimeConfig{}, fmt.Errorf("runtime_kind %q conflicts with runtime_name %q", kind, cfg.Name)
		}
		return resolved, nil
	}
	return cfg, nil
}
