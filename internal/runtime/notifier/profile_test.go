package notifier

import (
	"testing"

	agentruntime "csgclaw/internal/runtime"
)

func TestViewRuntimeOptionsForAPIInjectsNotifierProfile(t *testing.T) {
	runtimeOptions := map[string]any{
		"delivery_mode": "webhook",
		"webhook_token": "secret",
		"remote_token":  "",
		"remote_url":    "",
	}
	out := ViewRuntimeOptionsForAPI(runtimeOptions)
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

func TestMergeFlatRuntimeOptionsForProfilePatchOverlaysRuntimeOptions(t *testing.T) {
	baseRuntimeOptions := map[string]any{"delivery_mode": "webhook", "webhook_token": "a"}
	patchRuntimeOptions := map[string]any{"delivery_mode": "webhook", "webhook_token": "b"}
	got := MergeFlatRuntimeOptionsForProfilePatch(baseRuntimeOptions, patchRuntimeOptions)
	if got["webhook_token"] != "b" {
		t.Fatalf("webhook_token = %v", got["webhook_token"])
	}
}

func TestStripProfileLLMFieldsForRuntime(t *testing.T) {
	b, m := StripProfileLLMFieldsForRuntime(agentruntime.KindNotifier, "https://x", "gpt")
	if b != "" || m != "" {
		t.Fatalf("notifier: want cleared, got base_url=%q model_id=%q", b, m)
	}
	b2, m2 := StripProfileLLMFieldsForRuntime(agentruntime.KindPicoClawSandbox, "https://x", "gpt")
	if b2 != "https://x" || m2 != "gpt" {
		t.Fatalf("sandbox: want unchanged, got base_url=%q model_id=%q", b2, m2)
	}
}

func TestMergeFlatForAgentPatchPreservesTokensWhenPatchSendsEmpty(t *testing.T) {
	agentRuntimeOptions := map[string]any{
		"delivery_mode": "remote_pull",
		"remote_token":  "secret-rt",
		"remote_url":    "http://old/inbox",
		"webhook_token": "wh-keep",
	}
	patchRuntimeOptions := map[string]any{
		"delivery_mode": "remote_pull",
		"remote_url":    "http://new/inbox",
		"remote_token":  "",
		"webhook_token": "",
	}
	got := MergeFlatForAgentPatch(agentRuntimeOptions, patchRuntimeOptions)
	if got["remote_token"] != "secret-rt" {
		t.Fatalf("remote_token = %q", got["remote_token"])
	}
	if got["webhook_token"] != "wh-keep" {
		t.Fatalf("webhook_token = %q", got["webhook_token"])
	}
	if got["remote_url"] != "http://new/inbox" {
		t.Fatalf("remote_url = %q", got["remote_url"])
	}
}

func TestMergeFlatForAgentPatchPreservesOptionalRelayURLsWhenPatchSendsEmpty(t *testing.T) {
	agentRuntimeOptions := map[string]any{
		"delivery_mode":       "remote_pull",
		"remote_url":          "http://inbox",
		"remote_messages_url": "http://messages",
		"remote_ack_url":      "http://ack",
	}
	patchRuntimeOptions := map[string]any{
		"delivery_mode":       "remote_pull",
		"remote_url":          "http://inbox",
		"remote_messages_url": "",
		"remote_ack_url":      "",
	}
	got := MergeFlatForAgentPatch(agentRuntimeOptions, patchRuntimeOptions)
	if got["remote_messages_url"] != "http://messages" {
		t.Fatalf("remote_messages_url = %q", got["remote_messages_url"])
	}
	if got["remote_ack_url"] != "http://ack" {
		t.Fatalf("remote_ack_url = %q", got["remote_ack_url"])
	}
}

func TestProfileDeliveryComplete(t *testing.T) {
	merged := map[string]any{"delivery_mode": "webhook", "webhook_token": "x"}
	if !ProfileDeliveryComplete(merged) {
		t.Fatal("notifier: want complete from flat runtime_options")
	}
	if ProfileDeliveryComplete(nil) {
		t.Fatal("notifier: want incomplete with no runtime_options")
	}
}
