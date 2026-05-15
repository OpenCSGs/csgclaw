package notifier

import "testing"

func TestViewRuntimeOptionsForAPIInjectsNotifierProfile(t *testing.T) {
	ext := map[string]any{
		"delivery_mode": "webhook",
		"webhook_token": "secret",
		"remote_token":  "",
		"remote_url":    "",
	}
	out := ViewRuntimeOptionsForAPI(ext)
	raw, ok := out[RuntimeOptionKeyNotifierProfile]
	if !ok {
		t.Fatal("missing notifier_profile")
	}
	sm, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("notifier_profile type = %T", raw)
	}
	if sm["webhook_token_set"] != true {
		t.Fatalf("webhook_token_set = %v", sm["webhook_token_set"])
	}
	if _, bad := sm["webhook_token"]; bad {
		t.Fatal("summary must not contain token value")
	}
}
