package utils

import "testing"

func TestOverlayAnyMap(t *testing.T) {
	dst := map[string]any{"foo": "bar"}
	overlay := map[string]any{
		"notifier": map[string]any{"webhook_token": "secret", "delivery_mode": "webhook"},
	}
	got := OverlayAnyMap(dst, overlay)
	if got["foo"] != "bar" {
		t.Fatal("existing key lost")
	}
	n, ok := got["notifier"].(map[string]any)
	if !ok || n["webhook_token"] != "secret" {
		t.Fatalf("overlay = %#v", got["notifier"])
	}
	if OverlayAnyMap(nil, nil) != nil {
		t.Fatal("nil overlay should leave nil dst")
	}
}

func TestCloneAnyMapShallowNestedStringMaps(t *testing.T) {
	t.Parallel()
	inner := map[string]any{"a": 1}
	src := map[string]any{"nested": inner, "plain": "x"}
	got := CloneAnyMapShallowNestedStringMaps(src)
	inner["a"] = 999
	n, ok := got["nested"].(map[string]any)
	if !ok || n["a"] != 1 {
		t.Fatalf("nested map should be copied, got nested=%#v", got["nested"])
	}
	if got["plain"] != "x" {
		t.Fatalf("plain = %q", got["plain"])
	}
	if CloneAnyMapShallowNestedStringMaps(nil) != nil {
		t.Fatal("nil src should return nil")
	}
	if CloneAnyMapShallowNestedStringMaps(map[string]any{}) != nil {
		t.Fatal("empty src should return nil")
	}
}
