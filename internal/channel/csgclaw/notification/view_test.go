package notification

import "testing"

func TestStripViewOnlyMetadataKeys_nilEmpty(t *testing.T) {
	t.Parallel()
	if got := StripViewOnlyMetadataKeys(nil); got != nil {
		t.Fatalf("nil: got %#v, want nil", got)
	}
	if got := StripViewOnlyMetadataKeys(map[string]any{}); got != nil {
		t.Fatalf("empty: got %#v, want nil", got)
	}
}

func TestStripViewOnlyMetadataKeys_noViewKeys(t *testing.T) {
	t.Parallel()
	ext := map[string]any{"delivery_mode": "webhook"}
	got := StripViewOnlyMetadataKeys(ext)
	if got["delivery_mode"] != "webhook" || len(got) != 1 {
		t.Fatalf("want delivery preserved, got %#v", got)
	}
}

func TestStripViewOnlyMetadataKeys_stripsNotificationProfile(t *testing.T) {
	t.Parallel()
	ext := map[string]any{
		"delivery_mode":                "webhook",
		MetadataKeyNotificationProfile: map[string]any{"delivery_complete": true},
	}
	got := StripViewOnlyMetadataKeys(ext)
	if _, ok := got[MetadataKeyNotificationProfile]; ok {
		t.Fatalf("notification_profile should be removed, got %#v", got)
	}
	if got["delivery_mode"] != "webhook" {
		t.Fatalf("delivery_mode: got %#v", got["delivery_mode"])
	}
	if _, ok := ext[MetadataKeyNotificationProfile]; !ok {
		t.Fatal("original map must be unchanged")
	}
}
