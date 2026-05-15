package runtime

import "testing"

func TestStripViewOnlyRuntimeOptions_nilEmpty(t *testing.T) {
	t.Parallel()
	if got := StripViewOnlyRuntimeOptions(nil); got != nil {
		t.Fatalf("nil: got %#v, want nil", got)
	}
	if got := StripViewOnlyRuntimeOptions(map[string]any{}); got != nil {
		t.Fatalf("empty: got %#v, want nil", got)
	}
}

func TestStripViewOnlyRuntimeOptions_noViewKeys(t *testing.T) {
	t.Parallel()
	ext := map[string]any{"delivery_mode": "webhook"}
	got := StripViewOnlyRuntimeOptions(ext)
	if got["delivery_mode"] != "webhook" || len(got) != 1 {
		t.Fatalf("want delivery preserved, got %#v", got)
	}
}

func TestStripViewOnlyRuntimeOptions_stripsNotifierProfile(t *testing.T) {
	t.Parallel()
	ext := map[string]any{
		"delivery_mode":                 "webhook",
		RuntimeOptionKeyNotifierProfile: map[string]any{"delivery_complete": true},
	}
	got := StripViewOnlyRuntimeOptions(ext)
	if _, ok := got[RuntimeOptionKeyNotifierProfile]; ok {
		t.Fatalf("notifier_profile should be removed, got %#v", got)
	}
	if got["delivery_mode"] != "webhook" {
		t.Fatalf("delivery_mode: got %#v", got["delivery_mode"])
	}
	if _, ok := ext[RuntimeOptionKeyNotifierProfile]; !ok {
		t.Fatal("original map must be unchanged")
	}
}
