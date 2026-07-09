package templateembed

import (
	"testing"

	"csgclaw/internal/runtime"
)

func TestLookupBuiltin(t *testing.T) {
	tests := []struct {
		id          string
		runtimeKind string
		role        string
		root        string
	}{
		{id: "manager-codex", runtimeKind: runtime.KindCodex, role: roleManager, root: CodexManagerRoot},
		{id: "codex-worker", runtimeKind: runtime.KindCodex, role: roleWorker, root: CodexWorkerRoot},
		{id: "openclaw-worker", runtimeKind: runtime.KindOpenClawSandbox, role: roleWorker, root: OpenClawWorkerRoot},
		{id: "picoclaw-worker", runtimeKind: runtime.KindPicoClawSandbox, role: roleWorker, root: PicoClawWorkerRoot},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got, ok := LookupBuiltin(tt.id)
			if !ok {
				t.Fatalf("LookupBuiltin(%q) ok = false, want true", tt.id)
			}
			if got.ID != tt.id {
				t.Fatalf("LookupBuiltin(%q).ID = %q, want %q", tt.id, got.ID, tt.id)
			}
			if got.RuntimeKind != tt.runtimeKind {
				t.Fatalf("LookupBuiltin(%q).RuntimeKind = %q, want %q", tt.id, got.RuntimeKind, tt.runtimeKind)
			}
			if got.Role != tt.role {
				t.Fatalf("LookupBuiltin(%q).Role = %q, want %q", tt.id, got.Role, tt.role)
			}
			if got.Root != tt.root {
				t.Fatalf("LookupBuiltin(%q).Root = %q, want %q", tt.id, got.Root, tt.root)
			}
		})
	}
}

func TestBuiltinsReturnsClone(t *testing.T) {
	got := Builtins()
	if len(got) == 0 {
		t.Fatal("Builtins() returned empty slice")
	}
	got[0].ID = "changed"
	again := Builtins()
	if again[0].ID == "changed" {
		t.Fatal("Builtins() should return a cloned slice")
	}
}
