package notifier

import "testing"

func TestProfileViewSummaryForAPINilWithoutNotifier(t *testing.T) {
	if ProfileViewSummaryForAPI(nil) != nil {
		t.Fatal("want nil")
	}
}

func TestViewRuntimeExtensionsForAPIInjectsNotifierProfile(t *testing.T) {
	ext := map[string]any{
		"delivery_mode": "webhook",
		"webhook_token": "secret",
		"remote_token":  "",
		"remote_url":    "",
	}
	out := ViewRuntimeExtensionsForAPI(ext)
	raw, ok := out[RuntimeExtensionKeyNotifierProfile]
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

func TestRequestOptionsForAgentProfileViewOmitsNestedWhenFlatOnAgentExt(t *testing.T) {
	agentExt := map[string]any{"delivery_mode": "remote_pull", "remote_url": "http://inbox"}
	ro := map[string]any{
		"foo":      "bar",
		"notifier": map[string]any{"remote_token": "secret", "delivery_mode": "remote_pull"},
	}
	got := RequestOptionsForAgentProfileView(agentExt, ro)
	if _, ok := got["notifier"]; ok {
		t.Fatalf("want nested notifier omitted: %#v", got)
	}
	if got["foo"] != "bar" {
		t.Fatalf("foo = %#v", got["foo"])
	}
}
