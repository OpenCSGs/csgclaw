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

func TestPollIntervalDurationBareNumberAndUnits(t *testing.T) {
	t.Parallel()
	if got := (Config{PollInterval: "2"}).PollIntervalDuration(); got != 2*time.Second {
		t.Fatalf("bare 2 = %v, want 2s", got)
	}
	if got := (Config{PollInterval: "2s"}).PollIntervalDuration(); got != 2*time.Second {
		t.Fatalf("2s = %v", got)
	}
	if got := (Config{PollInterval: "45s"}).PollIntervalDuration(); got != 45*time.Second {
		t.Fatalf("45s = %v", got)
	}
	if got := (Config{PollInterval: "0.5"}).PollIntervalDuration(); got != 30*time.Second {
		t.Fatalf("0.5s below min = %v, want 30s default", got)
	}
	if got := (Config{PollInterval: "nope"}).PollIntervalDuration(); got != 30*time.Second {
		t.Fatalf("invalid = %v", got)
	}
}
