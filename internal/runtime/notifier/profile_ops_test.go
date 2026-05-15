package notifier

import (
	"testing"

	agentruntime "csgclaw/internal/runtime"
)

func TestMergeFlatForProfilePatchOverlaysRuntimeOptions(t *testing.T) {
	baseExt := map[string]any{"delivery_mode": "webhook", "webhook_token": "a"}
	patchExt := map[string]any{"delivery_mode": "webhook", "webhook_token": "b"}
	got := MergeFlatForProfilePatch(baseExt, patchExt)
	if got["webhook_token"] != "b" {
		t.Fatalf("webhook_token = %v", got["webhook_token"])
	}
}

func TestProfileLLMEndpointFieldsForRuntime(t *testing.T) {
	b, m := ProfileLLMEndpointFieldsForRuntime(agentruntime.KindNotifier, "https://x", "gpt")
	if b != "" || m != "" {
		t.Fatalf("notifier: want cleared, got base_url=%q model_id=%q", b, m)
	}
	b2, m2 := ProfileLLMEndpointFieldsForRuntime(agentruntime.KindPicoClawSandbox, "https://x", "gpt")
	if b2 != "https://x" || m2 != "gpt" {
		t.Fatalf("sandbox: want unchanged, got base_url=%q model_id=%q", b2, m2)
	}
}

func TestMergeFlatForAgentPatchPreservesTokensWhenPatchSendsEmpty(t *testing.T) {
	agentExt := map[string]any{
		"delivery_mode": "remote_pull",
		"remote_token":  "secret-rt",
		"remote_url":    "http://old/inbox",
		"webhook_token": "wh-keep",
	}
	patchExt := map[string]any{
		"delivery_mode": "remote_pull",
		"remote_url":    "http://new/inbox",
		"remote_token":  "",
		"webhook_token": "",
	}
	got := MergeFlatForAgentPatch(agentExt, patchExt)
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
	agentExt := map[string]any{
		"delivery_mode":       "remote_pull",
		"remote_url":          "http://inbox",
		"remote_messages_url": "http://messages",
		"remote_ack_url":      "http://ack",
	}
	patchExt := map[string]any{
		"delivery_mode":       "remote_pull",
		"remote_url":          "http://inbox",
		"remote_messages_url": "",
		"remote_ack_url":      "",
	}
	got := MergeFlatForAgentPatch(agentExt, patchExt)
	if got["remote_messages_url"] != "http://messages" {
		t.Fatalf("remote_messages_url = %q", got["remote_messages_url"])
	}
	if got["remote_ack_url"] != "http://ack" {
		t.Fatalf("remote_ack_url = %q", got["remote_ack_url"])
	}
}

func TestProfileCompleteFromAgentExtensionsPrefersMergedFlat(t *testing.T) {
	merged := map[string]any{"delivery_mode": "webhook", "webhook_token": "x"}
	if !ProfileCompleteFromAgentExtensions(false, false, nil, agentruntime.KindNotifier, merged) {
		t.Fatal("notifier: want complete from merged flat")
	}
	if ProfileCompleteFromAgentExtensions(false, false, nil, agentruntime.KindNotifier, nil) {
		t.Fatal("notifier: want incomplete with no storage and no merged flat")
	}
}

func TestPersistNotifierFlatToProfileNoOpWhenEmpty(t *testing.T) {
	ext := map[string]any{"other": 1}
	ro := map[string]any{"foo": "bar"}
	outExt, outRO := PersistNotifierFlatToProfile(ext, ro, map[string]any{})
	if len(outExt) != len(ext) || outExt["other"] != ext["other"] {
		t.Fatalf("extensions changed: %#v", outExt)
	}
	if len(outRO) != len(ro) || outRO["foo"] != ro["foo"] {
		t.Fatalf("request_options changed: %#v", outRO)
	}
}

func TestProfileCompleteForAgentRuntimeGatewayVsNotifier(t *testing.T) {
	flat := map[string]any{"delivery_mode": "webhook", "webhook_token": "x"}
	if ProfileCompleteForAgentRuntime(true, false, flat, agentruntime.KindPicoClawSandbox) {
		t.Fatal("gateway: want false when LLM incomplete")
	}
	if !ProfileCompleteForAgentRuntime(true, true, flat, agentruntime.KindPicoClawSandbox) {
		t.Fatal("gateway: want true when LLM complete")
	}
	if ProfileCompleteForAgentRuntime(false, true, nil, agentruntime.KindNotifier) {
		t.Fatal("notifier: want false when delivery incomplete")
	}
	if !ProfileCompleteForAgentRuntime(false, false, flat, agentruntime.KindNotifier) {
		t.Fatal("notifier: want true when delivery complete")
	}
	if !ProfileCompleteForAgentRuntime(false, true, nil, agentruntime.KindCodex) {
		t.Fatal("codex: want true when LLM complete without delivery")
	}
}

func TestApplyNotifierFlatPersistenceMergesOntoAgentExtPreservesOtherKeys(t *testing.T) {
	agentRE := map[string]any{
		"keep":                   "yes",
		RuntimeOptionKeyNotifier: map[string]any{"delivery_mode": "webhook", "webhook_token": "old"},
	}
	flat := map[string]any{"delivery_mode": "webhook", "webhook_token": "new"}
	nextRE, _ := ApplyNotifierFlatPersistence(&agentRE, nil, nil, flat)
	if nextRE != nil {
		t.Fatalf("nextRE = %#v", nextRE)
	}
	if agentRE["keep"] != "yes" {
		t.Fatalf("lost sibling key: %#v", agentRE)
	}
	if _, nested := agentRE[RuntimeOptionKeyNotifier]; nested {
		t.Fatal("legacy nested notifier key should be stripped on persist")
	}
	if agentRE["webhook_token"] != "new" {
		t.Fatalf("webhook_token = %v", agentRE["webhook_token"])
	}
}
