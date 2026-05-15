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
