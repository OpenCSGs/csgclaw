package notification_bot

import "testing"

func TestStripViewOnlyRuntimeOptionKeys_nilEmpty(t *testing.T) {
	t.Parallel()
	if got := StripViewOnlyRuntimeOptionKeys(nil); got != nil {
		t.Fatalf("nil: got %#v, want nil", got)
	}
	if got := StripViewOnlyRuntimeOptionKeys(map[string]any{}); got != nil {
		t.Fatalf("empty: got %#v, want nil", got)
	}
}

func TestStripViewOnlyRuntimeOptionKeys_noViewKeys(t *testing.T) {
	t.Parallel()
	ext := map[string]any{"delivery_mode": "webhook"}
	got := StripViewOnlyRuntimeOptionKeys(ext)
	if got["delivery_mode"] != "webhook" || len(got) != 1 {
		t.Fatalf("want delivery preserved, got %#v", got)
	}
}

func TestStripViewOnlyRuntimeOptionKeys_stripsNotificationProfile(t *testing.T) {
	t.Parallel()
	ext := map[string]any{
		"delivery_mode":                     "webhook",
		RuntimeOptionKeyNotificationProfile: map[string]any{"delivery_complete": true},
	}
	got := StripViewOnlyRuntimeOptionKeys(ext)
	if _, ok := got[RuntimeOptionKeyNotificationProfile]; ok {
		t.Fatalf("notification_profile should be removed, got %#v", got)
	}
	if got["delivery_mode"] != "webhook" {
		t.Fatalf("delivery_mode: got %#v", got["delivery_mode"])
	}
	if _, ok := ext[RuntimeOptionKeyNotificationProfile]; !ok {
		t.Fatal("original map must be unchanged")
	}
}
