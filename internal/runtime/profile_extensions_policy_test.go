package runtime

import "testing"

func TestNormalizeRuntimeKind(t *testing.T) {
	t.Parallel()
	if got := NormalizeRuntimeKind("  NOTIFIER "); got != KindNotifier {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeRuntimeKind(""); got != "" {
		t.Fatalf("empty: got %q", got)
	}
}

func TestProfileExtensionsPolicyForKind_unknownUsesDefault(t *testing.T) {
	t.Parallel()
	p := ProfileExtensionsPolicyForKind("unknown-future-runtime")
	if p.MergeFlatForAgentPatch(map[string]any{"a": 1}, map[string]any{"b": 2}) != nil {
		t.Fatal("default merge should return nil")
	}
}
