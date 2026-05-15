package notifier

import (
	"strings"

	agentruntime "csgclaw/internal/runtime"
)

// RuntimeOptionKeyNotifier names the legacy nested bucket under runtime_options; it is
// stripped on persist and omitted from API responses. Notifier delivery fields live as flat keys
// on the same map (see NotifierStorageKeys).
const RuntimeOptionKeyNotifier = "notifier"

// SubExtensionMap returns a shallow copy of extensions[key] when it is a non-empty map[string]any.
func SubExtensionMap(extensions map[string]any, key string) map[string]any {
	if len(extensions) == 0 {
		return nil
	}
	raw, ok := extensions[key]
	if !ok || raw == nil {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok || len(m) == 0 {
		return nil
	}
	return agentruntime.CloneAnyMap(m)
}

// NotifierFlatFromProfile reads notifier flat from a runtime_options map (e.g. create payload before agent exists).
func NotifierFlatFromProfile(extensions map[string]any) map[string]any {
	return NotifierFlatFromRuntimeOptionsMap(extensions)
}

// ConfigFromNotifierProfile parses Config from profile runtime_options only.
func ConfigFromNotifierProfile(extensions map[string]any) Config {
	return ConfigFromStored(NotifierFlatFromProfile(extensions))
}

// WithRuntimeOption returns a copy of extensions with key set to flat (or deleted when flat is empty).
func WithRuntimeOption(extensions map[string]any, key string, flat map[string]any) map[string]any {
	out := agentruntime.CloneAnyMap(extensions)
	if out == nil {
		out = make(map[string]any)
	}
	if len(flat) == 0 {
		delete(out, key)
	} else {
		out[key] = agentruntime.CloneAnyMap(flat)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// WithNotifierExtension sets legacy nested runtime_options["notifier"] (prefer flat root keys on the map instead).
func WithNotifierExtension(extensions map[string]any, flat map[string]any) map[string]any {
	return WithRuntimeOption(extensions, RuntimeOptionKeyNotifier, flat)
}

// RedactRuntimeOptionsForAPI returns a shallow copy of extensions with known secret-bearing subtrees redacted.
func RedactRuntimeOptionsForAPI(extensions map[string]any) map[string]any {
	if len(extensions) == 0 {
		return nil
	}
	out := agentruntime.CloneAnyMap(extensions)
	if out == nil {
		return nil
	}
	delete(out, RuntimeOptionKeyNotifierProfile)
	// Flat notifier keys at map root (canonical storage).
	if len(copyNotifierKeysFromMap(out)) > 0 {
		redRoot := RedactDetailsForAPI(copyNotifierKeysFromMap(out))
		for _, k := range NotifierStorageKeys {
			delete(out, k)
		}
		for k, v := range redRoot {
			out[k] = v
		}
	}
	delete(out, RuntimeOptionKeyNotifier)
	if len(out) == 0 {
		return nil
	}
	return out
}

// MatchesNotifierRuntimeKind reports whether kind is the in-server notifier worker runtime.
func MatchesNotifierRuntimeKind(kind string) bool {
	return strings.EqualFold(strings.TrimSpace(kind), agentruntime.KindNotifier)
}

// ProfileLLMEndpointFieldsForRuntime returns baseURL and modelID unchanged except for the in-server notifier worker runtime,
// which does not use LLM gateway fields; those must not be persisted on notifier agents.
func ProfileLLMEndpointFieldsForRuntime(runtimeKind, baseURL, modelID string) (string, string) {
	if MatchesNotifierRuntimeKind(runtimeKind) {
		return "", ""
	}
	return baseURL, modelID
}

// MergeFlatForProfilePatch merges patch profile runtime_options onto a base options map (no agent-level storage yet).
func MergeFlatForProfilePatch(baseProfileExtensions, patchProfileExtensions map[string]any) map[string]any {
	base := NotifierFlatFromRuntimeOptionsMap(baseProfileExtensions)
	incoming := NotifierFlatFromRuntimeOptionsMap(patchProfileExtensions)
	return MergeNotifierFlatPatch(base, incoming)
}

// PersistNotifierFlatToProfile merges flat notifier keys at the root of extensions (no nested "notifier" map)
// and strips nested notifier from request_options. When mergedFlat is empty, inputs are unchanged.
func PersistNotifierFlatToProfile(extensions, requestOptions, mergedFlat map[string]any) (map[string]any, map[string]any) {
	return ApplyNotifierFlatPersistence(nil, extensions, requestOptions, mergedFlat)
}

// RequestOptionsWithoutNestedNotifier returns a copy of ro with request_options["notifier"] removed.
func RequestOptionsWithoutNestedNotifier(ro map[string]any) map[string]any {
	out := agentruntime.CloneAnyMap(ro)
	if out == nil {
		return nil
	}
	StripNestedNotifier(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

// ProfileCompleteForAgentRuntime reports whether a profile is complete for the agent's runtime class.
// Gateway sandboxes require LLM fields; notifier workers require delivery configuration;
// other non-gateway runtimes accept either LLM or notifier delivery completeness.
func ProfileCompleteForAgentRuntime(isGatewayRuntime, llmComplete bool, flat map[string]any, runtimeKind string) bool {
	deliveryComplete := ProfileDeliveryComplete(flat)
	if isGatewayRuntime {
		return llmComplete
	}
	if MatchesNotifierRuntimeKind(runtimeKind) {
		return deliveryComplete
	}
	return deliveryComplete || llmComplete
}

// ProfileCompleteFromAgentExtensions resolves notifier flat from agent-level runtime_options
// (or uses mergedFlat when non-empty, e.g. from MergeFlatForAgentPatch), then applies ProfileCompleteForAgentRuntime.
func ProfileCompleteFromAgentExtensions(isGatewayRuntime, llmComplete bool, agentExt map[string]any, runtimeKind string, mergedFlat map[string]any) bool {
	flat := mergedFlat
	if len(flat) == 0 {
		flat = NotifierFlatFromAgentStorage(agentExt)
	}
	return ProfileCompleteForAgentRuntime(isGatewayRuntime, llmComplete, flat, runtimeKind)
}

// ProfileDeliveryComplete reports whether notifier delivery is sufficiently configured from flat runtime storage.
func ProfileDeliveryComplete(flat map[string]any) bool {
	if len(flat) == 0 {
		return false
	}
	c := ParseNotifierDetails(flat)
	return c.AllowsWebhook() || c.AllowsPull()
}

// ProfileViewSummary is view-only API state derived from stored notifier configuration (never a source of truth on disk).
type ProfileViewSummary struct {
	DeliveryComplete bool `json:"delivery_complete,omitempty"`
	WebhookTokenSet  bool `json:"webhook_token_set,omitempty"`
	RemoteTokenSet   bool `json:"remote_token_set,omitempty"`
}

// ProfileViewSummaryForAPI returns nil when no notifier configuration is present in the given options map.
func ProfileViewSummaryForAPI(extensions map[string]any) *ProfileViewSummary {
	return ProfileViewSummaryForAgentStorage(NotifierFlatFromRuntimeOptionsMap(extensions))
}

// ProfileViewSummaryForAgentStorage builds a summary from agent-level notifier flat only.
func ProfileViewSummaryForAgentStorage(agentFlat map[string]any) *ProfileViewSummary {
	if len(agentFlat) == 0 {
		return nil
	}
	cfg := ConfigFromStored(agentFlat)
	if strings.TrimSpace(cfg.DeliveryMode) == "" && strings.TrimSpace(cfg.WebhookToken) == "" && strings.TrimSpace(cfg.RemoteURL) == "" {
		return nil
	}
	return &ProfileViewSummary{
		DeliveryComplete: ProfileDeliveryComplete(agentFlat),
		WebhookTokenSet:  strings.TrimSpace(cfg.WebhookToken) != "",
		RemoteTokenSet:   strings.TrimSpace(cfg.RemoteToken) != "",
	}
}

func profileViewSummaryToMap(s *ProfileViewSummary) map[string]any {
	if s == nil {
		return nil
	}
	m := make(map[string]any)
	if s.DeliveryComplete {
		m["delivery_complete"] = true
	}
	if s.WebhookTokenSet {
		m["webhook_token_set"] = true
	}
	if s.RemoteTokenSet {
		m["remote_token_set"] = true
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// requestOptionsForAgentProfileView returns request_options safe for JSON (secrets redacted).
// When agent-level notifier flat is present, nested request_options["notifier"] is omitted from the view
// to avoid duplicating what is summarized under runtime_options.notifier_profile.
func requestOptionsForAgentProfileView(agentExt map[string]any, requestOptions map[string]any) map[string]any {
	flat := NotifierFlatFromAgentStorage(agentExt)
	ro := RedactedRequestOptionsForAPIView(requestOptions)
	if len(flat) > 0 && ro != nil {
		delete(ro, "notifier")
		if len(ro) == 0 {
			ro = nil
		}
	}
	return ro
}

// RequestOptionsForAgentProfileView returns request_options safe for JSON (secrets redacted).
// When agent-level notifier flat is present, nested request_options["notifier"] is omitted from the view
// to avoid duplicating what is summarized under runtime_options.notifier_profile.
func RequestOptionsForAgentProfileView(agentExt map[string]any, requestOptions map[string]any) map[string]any {
	return requestOptionsForAgentProfileView(agentExt, requestOptions)
}

// ViewRuntimeOptionsForAPI returns runtime_options safe for JSON: redacted notifier subtree plus view-only notifier_profile summary.
// Options are treated as agent-level runtime_options (the only source of truth for notifier delivery).
func ViewRuntimeOptionsForAPI(extensions map[string]any) map[string]any {
	return ViewRuntimeOptionsForAPIUnified(extensions, nil)
}

// ViewRuntimeOptionsForAPIUnified merges agent-level and profile-level runtime_options before redacting and summarizing.
// Profile-level notifier payload is not merged (agent runtime_options is the only source of truth for delivery config).
func ViewRuntimeOptionsForAPIUnified(agentExt, profileExt map[string]any) map[string]any {
	profileExt = ProfileRuntimeOptionsWithoutNotifierPayload(profileExt)
	merged := MergeRuntimeOptionMapsForView(agentExt, profileExt)
	base := RedactRuntimeOptionsForAPI(merged)
	agentFlat := NotifierFlatFromAgentStorage(agentExt)
	summaryMap := profileViewSummaryToMap(ProfileViewSummaryForAgentStorage(agentFlat))
	if summaryMap == nil {
		if len(base) == 0 {
			return nil
		}
		return base
	}
	out := agentruntime.CloneAnyMap(base)
	if out == nil {
		out = make(map[string]any)
	}
	out[RuntimeOptionKeyNotifierProfile] = summaryMap
	return out
}
