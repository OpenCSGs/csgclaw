package agent

import "testing"

const codexSandboxRuntimeKind = "codex_sandbox"

func TestGatewayRuntimeKindClassifiesCodexSandboxButNotHostCodex(t *testing.T) {
	if !isGatewayRuntimeKind(codexSandboxRuntimeKind) {
		t.Fatalf("isGatewayRuntimeKind(%q) = false, want true", codexSandboxRuntimeKind)
	}
	if got := runtimeKindForGatewayRuntime(codexSandboxRuntimeKind); got != codexSandboxRuntimeKind {
		t.Fatalf("runtimeKindForGatewayRuntime(%q) = %q, want %q", codexSandboxRuntimeKind, got, codexSandboxRuntimeKind)
	}

	if isGatewayRuntimeKind(RuntimeKindCodex) {
		t.Fatalf("host-local %q runtime must not be classified as a gateway sandbox runtime", RuntimeKindCodex)
	}
	if got := runtimeKindForGatewayRuntime(RuntimeKindCodex); got != "" {
		t.Fatalf("runtimeKindForGatewayRuntime(%q) = %q, want empty", RuntimeKindCodex, got)
	}
}
