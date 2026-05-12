package notifier

import (
	"testing"
	"time"
)

func TestParseConfigRequestOptions(t *testing.T) {
	cfg := ParseConfigFromRequestOptions(map[string]any{
		"notifier": map[string]any{
			"delivery_mode":          "both",
			"webhook_token":          "abc",
			"remote_url":             "https://relay.example.com",
			"remote_subscription_id": "sub1",
			"poll_interval":          "45s",
			"remote_token":           "tok",
		},
	})
	if cfg.normalizedDeliveryMode() != DeliveryBoth {
		t.Fatalf("delivery mode = %q", cfg.normalizedDeliveryMode())
	}
	if !cfg.AllowsWebhook() || !cfg.AllowsPull() {
		t.Fatalf("allows webhook=%v pull=%v", cfg.AllowsWebhook(), cfg.AllowsPull())
	}
	if cfg.PollIntervalDuration() != 45*time.Second {
		t.Fatalf("poll interval = %v", cfg.PollIntervalDuration())
	}

	cfg2 := ParseConfigFromRequestOptions(map[string]any{
		"notifier": map[string]any{"delivery_mode": "webhook", "webhook_token": "x"},
	})
	if !cfg2.AllowsWebhook() || cfg2.AllowsPull() {
		t.Fatalf("webhook-only: allowsWebhook=%v allowsPull=%v", cfg2.AllowsWebhook(), cfg2.AllowsPull())
	}
}
