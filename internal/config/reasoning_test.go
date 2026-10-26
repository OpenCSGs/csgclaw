package config

import "testing"

func TestNormalizeReasoningEffortUsesNoneAsTheDisabledValue(t *testing.T) {
	for _, input := range []string{"none", "NONE", " off "} {
		if got := NormalizeReasoningEffort(input); got != ReasoningEffortNone {
			t.Fatalf("NormalizeReasoningEffort(%q) = %q, want %q", input, got, ReasoningEffortNone)
		}
	}
	if !UsesModelReasoningDefault("") || !UsesModelReasoningDefault("auto") {
		t.Fatal("empty and auto reasoning efforts should use the model default")
	}
	if HasExplicitReasoningEffort("auto") || HasExplicitReasoningEffort("none") {
		t.Fatal("auto and none must not be treated as explicit enabled efforts")
	}
	if !HasExplicitReasoningEffort("high") {
		t.Fatal("high should be treated as an explicit enabled effort")
	}
	if !HasExplicitReasoningEffort("max") {
		t.Fatal("max should be accepted when explicitly configured")
	}
	for _, effort := range CommonReasoningEfforts() {
		if effort == ReasoningEffortMax {
			t.Fatal("model-specific max must not become an implicit common fallback")
		}
	}
}
