package runtime

import "testing"

func TestRuntimeOptionsPolicyForKind_unknownUsesDefault(t *testing.T) {
	t.Parallel()
	p := RuntimeOptionsPolicyForKind("unknown-future-runtime")
	if _, ok := any(p).(defaultRuntimeOptionsPolicy); !ok {
		t.Fatalf("unexpected policy type %T", p)
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
