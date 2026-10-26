package runtime

import "testing"

func TestRuntimeConfigForSupportedKinds(t *testing.T) {
	for _, kind := range []string{NameCodex, NameDSH} {
		if got := RuntimeConfigForKind(kind); got != (RuntimeConfig{Name: kind}) {
			t.Fatalf("RuntimeConfigForKind(%q) = %#v", kind, got)
		}
	}

}

func TestRuntimeConfigKindRejectsSandboxedHostRuntimes(t *testing.T) {
	for _, name := range []string{NameCodex, NameDSH} {
		if got := (RuntimeConfig{Name: name}).Kind(); got != name {
			t.Fatalf("Kind() = %q, want %q", got, name)
		}
		if got := (RuntimeConfig{Name: name, Sandboxed: true}).Kind(); got != "" {
			t.Fatalf("sandbox Kind() = %q, want empty", got)
		}
	}

}

func TestRuntimeConfigIsHostRuntime(t *testing.T) {
	tests := []struct {
		name   string
		config RuntimeConfig
		want   bool
	}{
		{name: "codex", config: RuntimeConfig{Name: NameCodex}, want: true},
		{name: "dsh", config: RuntimeConfig{Name: NameDSH}, want: true},
		{name: "sandboxed codex", config: RuntimeConfig{Name: NameCodex, Sandboxed: true}},
		{name: "dsh", config: RuntimeConfig{Name: NameDSH, Sandboxed: true}},
		{name: "unknown", config: RuntimeConfig{Name: "custom"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsHostRuntime(); got != tt.want {
				t.Fatalf("IsHostRuntime() = %t, want %t", got, tt.want)
			}
		})
	}
}
