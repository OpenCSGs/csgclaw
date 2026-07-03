package runtime

import "testing"

func TestRuntimeConfigForKindSupportsBareSandboxKinds(t *testing.T) {
	tests := []struct {
		name string
		kind string
		want RuntimeConfig
	}{
		{name: "picoclaw", kind: NamePicoClaw, want: RuntimeConfig{Name: NamePicoClaw, Sandboxed: true}},
		{name: "openclaw", kind: NameOpenClaw, want: RuntimeConfig{Name: NameOpenClaw, Sandboxed: true}},
		{name: "legacy picoclaw", kind: KindPicoClawSandbox, want: RuntimeConfig{Name: NamePicoClaw, Sandboxed: true}},
		{name: "codex", kind: KindCodex, want: RuntimeConfig{Name: NameCodex, Sandboxed: false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RuntimeConfigForKind(tt.kind); got != tt.want {
				t.Fatalf("RuntimeConfigForKind(%q) = %#v, want %#v", tt.kind, got, tt.want)
			}
		})
	}
}

func TestRuntimeConfigKindPrefersShortSandboxNames(t *testing.T) {
	if got := (RuntimeConfig{Name: NamePicoClaw, Sandboxed: true}).Kind(); got != NamePicoClaw {
		t.Fatalf("Kind() = %q, want %q", got, NamePicoClaw)
	}
	if got := (RuntimeConfig{Name: NameOpenClaw, Sandboxed: true}).Kind(); got != NameOpenClaw {
		t.Fatalf("Kind() = %q, want %q", got, NameOpenClaw)
	}
	if got := (RuntimeConfig{Name: NameCodex, Sandboxed: false}).Kind(); got != KindCodex {
		t.Fatalf("Kind() = %q, want %q", got, KindCodex)
	}
}
