package api

import "testing"

func TestReadOnlyRuntimeSupportsCodexAndDSH(t *testing.T) {
	tests := []struct {
		name    string
		kind    string
		options map[string]any
		want    bool
	}{
		{name: "Codex read only", kind: "codex", options: map[string]any{"execution_mode": "read_only"}, want: true},
		{name: "Codex standard", kind: "codex", options: map[string]any{"execution_mode": "standard"}},
		{name: "DSH read only", kind: "dsh", options: map[string]any{"permission_mode": "read-only"}, want: true},
		{name: "DSH workspace", kind: "dsh", options: map[string]any{"permission_mode": "workspace-write"}},
		{name: "DSH full access", kind: "dsh", options: map[string]any{"permission_mode": "danger-full-access"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := readOnlyRuntime(test.kind, test.options); got != test.want {
				t.Fatalf("readOnlyRuntime(%q, %#v) = %v, want %v", test.kind, test.options, got, test.want)
			}
		})
	}
}
