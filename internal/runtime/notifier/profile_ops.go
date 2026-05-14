package notifier

import (
	"strings"

	agentruntime "csgclaw/internal/runtime"
)

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

// MergeFlatForProfilePatch merges patch profile runtime_extensions onto a base extensions map (no agent-level storage yet).
func MergeFlatForProfilePatch(baseProfileExtensions, patchProfileExtensions map[string]any) map[string]any {
	base := NotifierFlatFromRuntimeExtensionsMap(baseProfileExtensions)
	incoming := NotifierFlatFromRuntimeExtensionsMap(patchProfileExtensions)
	return MergeNotifierFlatPatch(base, incoming)
}

// PersistNotifierFlatToProfile merges flat notifier keys at the root of extensions (no nested "notifier" map)
// and strips nested notifier from request_options. When mergedFlat is empty, inputs are unchanged.
func PersistNotifierFlatToProfile(extensions, requestOptions, mergedFlat map[string]any) (map[string]any, map[string]any) {
	return ApplyNotifierFlatPersistence(nil, extensions, requestOptions, mergedFlat)
}

// RequestOptionsWithoutNestedNotifier returns a copy of ro with request_options["notifier"] removed.
func RequestOptionsWithoutNestedNotifier(ro map[string]any) map[string]any {
	out := CloneAnyMap(ro)
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

// ProfileCompleteFromAgentExtensions resolves notifier flat from agent-level runtime_extensions
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
