package runtime

import "testing"

func TestCodexSandboxKindIsDistinctFromHostLocalCodex(t *testing.T) {
	if got, want := KindCodexSandbox, "codex_sandbox"; got != want {
		t.Fatalf("KindCodexSandbox = %q, want %q", got, want)
	}
	if KindCodexSandbox == KindCodex {
		t.Fatalf("KindCodexSandbox must be a managed sandbox runtime distinct from host-local %q", KindCodex)
	}
}
