package agent

import (
	"strings"
	"testing"
)

func TestResolveRuntimeSelection(t *testing.T) {
	t.Run("derives sandbox runtime from legacy kind", func(t *testing.T) {
		kind, name, sandboxEnabled, err := resolveRuntimeSelection(RuntimeKindOpenClawSandbox, "", false)
		if err != nil {
			t.Fatalf("resolveRuntimeSelection() error = %v", err)
		}
		if kind != RuntimeKindOpenClawSandbox || name != RuntimeNameOpenClaw || !sandboxEnabled {
			t.Fatalf("resolveRuntimeSelection() = %q/%q/%t, want %q/%q/%t", kind, name, sandboxEnabled, RuntimeKindOpenClawSandbox, RuntimeNameOpenClaw, true)
		}
	})

	t.Run("derives legacy kind from split runtime fields", func(t *testing.T) {
		kind, name, sandboxEnabled, err := resolveRuntimeSelection("", RuntimeNamePicoClaw, true)
		if err != nil {
			t.Fatalf("resolveRuntimeSelection() error = %v", err)
		}
		if kind != RuntimeKindPicoClawSandbox || name != RuntimeNamePicoClaw || !sandboxEnabled {
			t.Fatalf("resolveRuntimeSelection() = %q/%q/%t, want %q/%q/%t", kind, name, sandboxEnabled, RuntimeKindPicoClawSandbox, RuntimeNamePicoClaw, true)
		}
	})

	t.Run("rejects conflicting runtime kind and name", func(t *testing.T) {
		_, _, _, err := resolveRuntimeSelection(RuntimeKindOpenClawSandbox, RuntimeNamePicoClaw, true)
		if err == nil || !strings.Contains(err.Error(), "conflicts") {
			t.Fatalf("resolveRuntimeSelection() error = %v, want conflict error", err)
		}
	})
}

func TestMergeReplaceSpecNormalizesRuntimeFields(t *testing.T) {
	existing := Agent{
		ID:             "agent-alice",
		Name:           "alice",
		RuntimeKind:    RuntimeKindPicoClawSandbox,
		RuntimeName:    RuntimeNamePicoClaw,
		SandboxEnabled: true,
		Role:           RoleWorker,
		Status:         "running",
	}

	merged, err := mergeReplaceSpec(existing, CreateAgentSpec{
		RuntimeKind: RuntimeKindOpenClawSandbox,
	}, []string{"runtime_kind"})
	if err != nil {
		t.Fatalf("mergeReplaceSpec() error = %v", err)
	}
	if merged.RuntimeKind != RuntimeKindOpenClawSandbox || merged.RuntimeName != RuntimeNameOpenClaw || !merged.SandboxEnabled {
		t.Fatalf("mergeReplaceSpec() runtime = %q/%q/%t, want %q/%q/%t", merged.RuntimeKind, merged.RuntimeName, merged.SandboxEnabled, RuntimeKindOpenClawSandbox, RuntimeNameOpenClaw, true)
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
