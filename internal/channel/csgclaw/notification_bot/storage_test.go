package notification_bot

import "testing"

func TestConfigFromStoredParsesFlat(t *testing.T) {
	flat := map[string]any{"delivery_mode": "webhook", "webhook_token": "x"}
	cfg := ConfigFromStored(flat)
	if cfg.DeliveryMode != "webhook" || cfg.WebhookToken != "x" {
		t.Fatalf("got %#v", cfg)
	}
}

func TestMergeFlatPatchKeys(t *testing.T) {
	base := map[string]any{"delivery_mode": "webhook", "webhook_token": "old"}
	over := map[string]any{"webhook_token": "", "remote_url": "https://x"}
	got := MergeFlatPatchKeys(base, over)
	if got["delivery_mode"] != "webhook" {
		t.Fatalf("delivery_mode = %v", got["delivery_mode"])
	}
	if got["webhook_token"] != "old" {
		t.Fatalf("empty patch should preserve webhook_token, got %v", got["webhook_token"])
	}
	if got["remote_url"] != "https://x" {
		t.Fatalf("remote_url = %v", got["remote_url"])
	}
}

func TestMergeFlatPatchKeys_preservesEmptySubscriptionID(t *testing.T) {
	base := map[string]any{
		"delivery_mode":          "remote_pull",
		"remote_subscription_id": "sub-existing",
		"remote_token":           "secret",
	}
	over := map[string]any{"remote_subscription_id": "", "remote_token": ""}
	got := MergeFlatPatchKeys(base, over)
	if got["remote_subscription_id"] != "sub-existing" {
		t.Fatalf("subscription_id = %v, want preserved", got["remote_subscription_id"])
	}
	if got["remote_token"] != "secret" {
		t.Fatalf("remote_token = %v, want preserved", got["remote_token"])
	}
}
