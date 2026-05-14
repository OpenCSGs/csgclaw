package runtime

import "testing"

func TestStripViewOnlyRuntimeExtensions_nilEmpty(t *testing.T) {
	t.Parallel()
	if got := StripViewOnlyRuntimeExtensions(nil); got != nil {
		t.Fatalf("nil: got %#v, want nil", got)
	}
	if got := StripViewOnlyRuntimeExtensions(map[string]any{}); got != nil {
		t.Fatalf("empty: got %#v, want nil", got)
	}
}

func TestStripViewOnlyRuntimeExtensions_noViewKeys(t *testing.T) {
	t.Parallel()
	ext := map[string]any{"delivery_mode": "webhook"}
	got := StripViewOnlyRuntimeExtensions(ext)
	if got["delivery_mode"] != "webhook" || len(got) != 1 {
		t.Fatalf("want delivery preserved, got %#v", got)
	}
}

func TestStripViewOnlyRuntimeExtensions_stripsNotifierProfile(t *testing.T) {
	t.Parallel()
	ext := map[string]any{
		"delivery_mode":                    "webhook",
		RuntimeExtensionKeyNotifierProfile: map[string]any{"delivery_complete": true},
	}
	got := StripViewOnlyRuntimeExtensions(ext)
	if _, ok := got[RuntimeExtensionKeyNotifierProfile]; ok {
		t.Fatalf("notifier_profile should be removed, got %#v", got)
	}
	if got["delivery_mode"] != "webhook" {
		t.Fatalf("delivery_mode: got %#v", got["delivery_mode"])
	}
	if _, ok := ext[RuntimeExtensionKeyNotifierProfile]; !ok {
		t.Fatal("original map must be unchanged")
	}
}
