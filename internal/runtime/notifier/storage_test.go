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

func TestRedactedRequestOptionsForAPIViewStripsNotifierTokens(t *testing.T) {
	ro := map[string]any{
		"other": "ok",
		"notifier": map[string]any{
			"delivery_mode": "webhook",
			"webhook_token": "secret",
			"remote_token":  "rt",
		},
	}
	got := RedactedRequestOptionsForAPIView(ro)
	n, ok := got["notifier"].(map[string]any)
	if !ok {
		t.Fatalf("notifier = %#v", got["notifier"])
	}
	if _, ok := n["webhook_token"]; ok {
		t.Fatal("webhook_token should be redacted")
	}
	if _, ok := n["remote_token"]; ok {
		t.Fatal("remote_token should be redacted")
	}
	if n["delivery_mode"] != "webhook" {
		t.Fatalf("delivery_mode = %v", n["delivery_mode"])
	}
	if got["other"] != "ok" {
		t.Fatalf("other = %v", got["other"])
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
