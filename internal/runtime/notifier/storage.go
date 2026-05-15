package notifier

import (
	"fmt"
	"strings"

	"csgclaw/internal/utils"
)

// NotifierStorageKeys lists flat keys for notifier delivery on runtime_options.
var NotifierStorageKeys = []string{
	"delivery_mode",
	"webhook_token",
	"remote_url",
	"remote_messages_url",
	"remote_ack_url",
	"remote_subscription_id",
	"poll_interval",
	"remote_token",
}

// IsNotifierFlatRoot reports whether m looks like flat notifier_details at map root.
func IsNotifierFlatRoot(m map[string]any) bool {
	if len(m) == 0 {
		return false
	}
	for _, k := range NotifierStorageKeys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

// NestedMapFromRequestOptions returns a shallow clone of ro["notifier"] when present and a map.
func NestedMapFromRequestOptions(ro map[string]any) map[string]any {
	if len(ro) == 0 {
		return nil
	}
	raw, ok := ro["notifier"]
	if !ok || raw == nil {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok || m == nil {
		return nil
	}
	return utils.CloneAnyMap(m)
}

// StripNestedNotifier deletes request_options["notifier"] in place.
func StripNestedNotifier(ro map[string]any) {
	if len(ro) == 0 {
		return
	}
	delete(ro, "notifier")
}

// ConfigFromStored parses notifier.Config from flat notifier_details (runtime_options storage).
func ConfigFromStored(storedFlat map[string]any) Config {
	if len(storedFlat) == 0 {
		return Config{}
	}
	return ParseNotifierDetails(storedFlat)
}

func MergeDetailMaps(base, overlay map[string]any) map[string]any {
	if len(overlay) == 0 {
		return utils.CloneAnyMap(base)
	}
	out := utils.CloneAnyMap(base)
	if out == nil {
		out = make(map[string]any, len(overlay))
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

func isEmptyNotifierSecret(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return strings.TrimSpace(fmt.Sprint(v)) == ""
}

// notifierPatchSkipEmptyIncomingKeys: if the patch sends an empty value for these keys, keep the base map's value
// (tokens are redacted in API responses; optional relay URLs may be absent from the editor draft).
var notifierPatchSkipEmptyIncomingKeys = map[string]struct{}{
	"webhook_token":       {},
	"remote_token":        {},
	"remote_messages_url": {},
	"remote_ack_url":      {},
}

// MergeNotifierFlatPatch overlays incoming notifier flat keys onto base.
// Empty values in incoming for certain keys (secrets and optional relay URLs) do not clear existing base values.
func MergeNotifierFlatPatch(base, incoming map[string]any) map[string]any {
	if len(incoming) == 0 {
		return utils.CloneAnyMap(base)
	}
	out := utils.CloneAnyMap(base)
	if out == nil {
		out = make(map[string]any, len(incoming))
	}
	for k, v := range incoming {
		if _, preserve := notifierPatchSkipEmptyIncomingKeys[k]; preserve && isEmptyNotifierSecret(v) {
			continue
		}
		out[k] = v
	}
	return out
}

func copyNotifierKeysFromMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any)
	for _, k := range NotifierStorageKeys {
		if v, ok := src[k]; ok && v != nil {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// StripNotifierKeysFromRootMap removes flat notifier keys and nested "notifier" from a runtime_options map in place.
func StripNotifierKeysFromRootMap(m map[string]any) {
	if len(m) == 0 {
		return
	}
	delete(m, RuntimeOptionKeyNotifier)
	for _, k := range NotifierStorageKeys {
		delete(m, k)
	}
}

// ProfileRuntimeOptionsWithoutNotifierPayload returns a copy of profile-level runtime_options
// with notifier payload removed (nested key and flat keys).
func ProfileRuntimeOptionsWithoutNotifierPayload(profileRuntimeOptions map[string]any) map[string]any {
	if len(profileRuntimeOptions) == 0 {
		return nil
	}
	out := utils.CloneAnyMap(profileRuntimeOptions)
	StripNotifierKeysFromRootMap(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

// NotifierFlatFromRuntimeOptionsMap returns notifier flat from a single runtime_options map.
// Storage is flat keys at the map root (delivery_mode, webhook_token, …); runtime_kind identifies
// notifier agents. View-only keys are ignored. The legacy nested runtime_options["notifier"]
// object is not read (StripNotifierKeysFromRootMap still removes it on persist).
func NotifierFlatFromRuntimeOptionsMap(runtimeOptions map[string]any) map[string]any {
	if len(runtimeOptions) == 0 {
		return nil
	}
	runtimeOptions = StripViewOnlyRuntimeOptionKeys(runtimeOptions)
	if len(runtimeOptions) == 0 {
		return nil
	}
	if flat := copyNotifierKeysFromMap(runtimeOptions); len(flat) > 0 {
		return utils.CloneAnyMap(flat)
	}
	return nil
}

// NotifierFlatFromAgentRuntimeOptions returns notifier flat stored on the agent (runtime_options only).
func NotifierFlatFromAgentRuntimeOptions(agentRuntimeOptions map[string]any) map[string]any {
	return NotifierFlatFromRuntimeOptionsMap(agentRuntimeOptions)
}

// MergeFlatForAgentPatch merges patch runtime_options onto the stored agent flat.
func MergeFlatForAgentPatch(agentRuntimeOptions, patchRuntimeOptions map[string]any) map[string]any {
	return mergeFlatForAgentPatch(agentRuntimeOptions, patchRuntimeOptions)
}

func mergeFlatForAgentPatch(agentRuntimeOptions, patchRuntimeOptions map[string]any) map[string]any {
	base := utils.CloneAnyMap(agentRuntimeOptions)
	incoming := StripViewOnlyRuntimeOptionKeys(patchRuntimeOptions)
	return MergeNotifierFlatPatch(base, incoming)
}

// ApplyNotifierFlatPersistence writes merged notifier flat onto *agentRuntimeOptions when non-nil (agent-level storage),
// otherwise merges flat keys at the root of profileRuntimeOptions (create path before Agent exists).
// It always strips nested notifier from a copy of profileRequestOptions and returns updated profile maps.
func ApplyNotifierFlatPersistence(agentRuntimeOptions *map[string]any, profileRuntimeOptions, profileRequestOptions map[string]any, mergedFlat map[string]any) (nextProfileRuntimeOptions, nextProfileRequestOptions map[string]any) {
	return applyNotifierFlatPersistence(agentRuntimeOptions, profileRuntimeOptions, profileRequestOptions, mergedFlat)
}

func applyNotifierFlatPersistence(agentRuntimeOptions *map[string]any, profileRuntimeOptions, profileRequestOptions map[string]any, mergedFlat map[string]any) (nextProfileRuntimeOptions, nextProfileRequestOptions map[string]any) {
	if len(mergedFlat) == 0 {
		return profileRuntimeOptions, profileRequestOptions
	}
	flat := utils.CloneAnyMap(mergedFlat)
	flat = EnsurePullRemoteSubscriptionInNotifierDetails(flat)
	nextRO := utils.CloneAnyMap(profileRequestOptions)
	StripNestedNotifier(nextRO)
	if len(nextRO) == 0 {
		nextRO = nil
	}
	if agentRuntimeOptions != nil {
		base := utils.CloneAnyMap(*agentRuntimeOptions)
		if base == nil {
			base = make(map[string]any)
		}
		StripNotifierKeysFromRootMap(base)
		merged := MergeNotifierFlatPatch(base, flat)
		if len(merged) == 0 {
			*agentRuntimeOptions = nil
		} else {
			*agentRuntimeOptions = merged
		}
		return ProfileRuntimeOptionsWithoutNotifierPayload(profileRuntimeOptions), nextRO
	}
	base := utils.CloneAnyMap(profileRuntimeOptions)
	StripNotifierKeysFromRootMap(base)
	for k, v := range flat {
		if _, ok := notifierStorageKeySet[k]; ok {
			if base == nil {
				base = make(map[string]any)
			}
			base[k] = v
		}
	}
	if len(base) == 0 {
		base = nil
	}
	return base, nextRO
}

var notifierStorageKeySet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(NotifierStorageKeys))
	for _, k := range NotifierStorageKeys {
		m[k] = struct{}{}
	}
	return m
}()

// ConfigFromAgentRuntimeOptions parses notifier.Config from agent-level runtime_options only.
func ConfigFromAgentRuntimeOptions(agentRuntimeOptions map[string]any) Config {
	return ConfigFromStored(NotifierFlatFromAgentRuntimeOptions(agentRuntimeOptions))
}
