package agents

import (
	"strings"
	"testing"

	agentruntime "csgclaw/internal/runtime"
)

func TestResolveRuntimeSelection(t *testing.T) {
	t.Run("normalizes runtime config from legacy kind", func(t *testing.T) {
		cfg, err := agentruntime.RuntimeConfigFromSelection(RuntimeKindDSH, "", false)
		if err != nil {
			t.Fatalf("RuntimeConfigFromSelection() error = %v", err)
		}
		if cfg != (agentruntime.RuntimeConfig{Name: RuntimeNameDSH, Sandboxed: false}) {
			t.Fatalf("RuntimeConfigFromSelection() = %#v, want %#v", cfg, agentruntime.RuntimeConfig{Name: RuntimeNameDSH, Sandboxed: false})
		}
		if cfg.LegacyKind() != RuntimeKindDSH {
			t.Fatalf("LegacyKind() = %q, want %q", cfg.LegacyKind(), RuntimeKindDSH)
		}
	})

	t.Run("derives host runtime from legacy kind", func(t *testing.T) {
		kind, name, sandboxEnabled, err := resolveRuntimeSelection(RuntimeKindDSH, "", false)
		if err != nil {
			t.Fatalf("resolveRuntimeSelection() error = %v", err)
		}
		if kind != RuntimeKindDSH || name != RuntimeNameDSH || sandboxEnabled {
			t.Fatalf("resolveRuntimeSelection() = %q/%q/%t, want %q/%q/%t", kind, name, sandboxEnabled, RuntimeKindDSH, RuntimeNameDSH, false)
		}
	})

	t.Run("derives legacy kind from split runtime fields", func(t *testing.T) {
		kind, name, sandboxEnabled, err := resolveRuntimeSelection("", RuntimeNameCodex, false)
		if err != nil {
			t.Fatalf("resolveRuntimeSelection() error = %v", err)
		}
		if kind != RuntimeKindCodex || name != RuntimeNameCodex || sandboxEnabled {
			t.Fatalf("resolveRuntimeSelection() = %q/%q/%t, want %q/%q/%t", kind, name, sandboxEnabled, RuntimeKindCodex, RuntimeNameCodex, false)
		}
	})

	t.Run("rejects conflicting runtime kind and name", func(t *testing.T) {
		_, _, _, err := resolveRuntimeSelection(RuntimeKindDSH, RuntimeNameCodex, false)
		if err == nil || !strings.Contains(err.Error(), "conflicts") {
			t.Fatalf("resolveRuntimeSelection() error = %v, want conflict error", err)
		}
	})
}

func TestAgentRuntimeConfigRoundTrip(t *testing.T) {
	item := Agent{RuntimeKind: RuntimeKindCodex}
	if got := item.RuntimeConfig(); got != (agentruntime.RuntimeConfig{Name: RuntimeNameCodex, Sandboxed: false}) {
		t.Fatalf("RuntimeConfig() = %#v, want %#v", got, agentruntime.RuntimeConfig{Name: RuntimeNameCodex, Sandboxed: false})
	}

	item.SetRuntimeConfig(agentruntime.RuntimeConfig{Name: RuntimeNameCodex, Sandboxed: false})
	if item.RuntimeKind != RuntimeKindCodex || item.RuntimeName != RuntimeNameCodex || item.SandboxEnabled {
		t.Fatalf("SetRuntimeConfig() runtime = %q/%q/%t, want %q/%q/%t", item.RuntimeKind, item.RuntimeName, item.SandboxEnabled, RuntimeKindCodex, RuntimeNameCodex, false)
	}
}

func TestMergeReplaceSpecNormalizesRuntimeFields(t *testing.T) {
	existing := Agent{
		ID:             "agent-alice",
		Name:           "alice",
		RuntimeKind:    RuntimeKindCodex,
		RuntimeName:    RuntimeNameCodex,
		SandboxEnabled: false,
		Role:           RoleWorker,
		Status:         "running",
	}

	merged, err := mergeReplaceSpec(existing, CreateAgentSpec{
		RuntimeKind: RuntimeKindDSH,
	}, []string{"runtime_kind"})
	if err != nil {
		t.Fatalf("mergeReplaceSpec() error = %v", err)
	}
	if merged.RuntimeKind != RuntimeKindDSH || merged.RuntimeName != RuntimeNameDSH || merged.SandboxEnabled {
		t.Fatalf("mergeReplaceSpec() runtime = %q/%q/%t, want %q/%q/%t", merged.RuntimeKind, merged.RuntimeName, merged.SandboxEnabled, RuntimeKindDSH, RuntimeNameDSH, false)
	}

	merged, err = mergeReplaceSpec(existing, CreateAgentSpec{
		RuntimeName:    RuntimeNameCodex,
		SandboxEnabled: false,
	}, []string{"runtime_name", "sandbox_enabled"})
	if err != nil {
		t.Fatalf("mergeReplaceSpec() error = %v", err)
	}
	if merged.RuntimeKind != RuntimeKindCodex || merged.RuntimeName != RuntimeNameCodex || merged.SandboxEnabled {
		t.Fatalf("mergeReplaceSpec() runtime = %q/%q/%t, want %q/%q/%t", merged.RuntimeKind, merged.RuntimeName, merged.SandboxEnabled, RuntimeKindCodex, RuntimeNameCodex, false)
	}
}
