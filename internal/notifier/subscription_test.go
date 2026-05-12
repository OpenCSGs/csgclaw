package notifier

import (
	"strings"
	"testing"
)

func TestEnsurePullRemoteSubscriptionInRequestOptions(t *testing.T) {
	ro := map[string]any{
		"notifier": map[string]any{
			"delivery_mode": "remote_pull",
			"remote_url":    "https://relay.example.com",
		},
	}
	out := EnsurePullRemoteSubscriptionInRequestOptions(ro)
	n := out["notifier"].(map[string]any)["remote_subscription_id"].(string)
	if n == "" || !strings.HasPrefix(n, "sub-") {
		t.Fatalf("unexpected id %q", n)
	}
	// idempotent
	EnsurePullRemoteSubscriptionInRequestOptions(out)
	if got := out["notifier"].(map[string]any)["remote_subscription_id"].(string); got != n {
		t.Fatalf("id changed: %q -> %q", n, got)
	}
}

func TestEnsurePullRemoteSubscriptionInRequestOptionsWebhookNoop(t *testing.T) {
	ro := map[string]any{
		"notifier": map[string]any{
			"delivery_mode": "webhook",
			"webhook_token": "x",
		},
	}
	out := EnsurePullRemoteSubscriptionInRequestOptions(ro)
	if _, ok := out["notifier"].(map[string]any)["remote_subscription_id"]; ok {
		t.Fatal("should not set subscription for webhook-only")
	}
}
