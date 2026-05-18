package notifier

import "testing"

func TestConfigFromStoredParsesFlat(t *testing.T) {
	flat := map[string]any{"delivery_mode": "webhook", "webhook_token": "x"}
	cfg := ConfigFromStored(flat)
	if cfg.DeliveryMode != "webhook" || cfg.WebhookToken != "x" {
		t.Fatalf("got %#v", cfg)
	}
}

func TestMergeDetailMaps(t *testing.T) {
	base := map[string]any{"a": "1", "b": "2"}
	over := map[string]any{"b": "3", "c": "4"}
	got := MergeDetailMaps(base, over)
	if got["a"] != "1" || got["b"] != "3" || got["c"] != "4" {
		t.Fatalf("got %#v", got)
	}
}

func TestIsNotifierFlatRoot(t *testing.T) {
	if IsNotifierFlatRoot(nil) || IsNotifierFlatRoot(map[string]any{"other": 1}) {
		t.Fatal("want false without notifier keys")
	}
	if !IsNotifierFlatRoot(map[string]any{"delivery_mode": "webhook"}) {
		t.Fatal("want true when flat notifier key present")
	}
}
