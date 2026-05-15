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

func TestRuntimeOptionsPolicyForKind_unknownUsesDefault(t *testing.T) {
	t.Parallel()
	p := RuntimeOptionsPolicyForKind("unknown-future-runtime")
	got := p.MergeFlatForAgentPatch(map[string]any{"a": 1}, map[string]any{"b": 2})
	if got["a"] != 1 || got["b"] != 2 {
		t.Fatalf("default merge = %#v", got)
	}
}

func TestDefaultIsCompleteUsesLLMComplete(t *testing.T) {
	t.Parallel()
	p := RuntimeOptionsPolicyForKind(KindCodex)
	if p.IsComplete(false, nil, nil) {
		t.Fatal("want false when LLM profile incomplete")
	}
	if !p.IsComplete(true, nil, nil) {
		t.Fatal("want true when LLM profile complete")
	}
}
