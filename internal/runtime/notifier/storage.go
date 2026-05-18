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

// StripNotifierKeysFromRootMap removes flat notifier keys from a runtime_options map in place.
func StripNotifierKeysFromRootMap(m map[string]any) {
	if len(m) == 0 {
		return
	}
	for _, k := range NotifierStorageKeys {
		delete(m, k)
	}
}

// StripNotifierKeysForProfileRuntimeOptions returns profile-level runtime_options without notifier flat keys.
func StripNotifierKeysForProfileRuntimeOptions(profileRuntimeOptions map[string]any) map[string]any {
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
// notifier agents. View-only keys are ignored.
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

// ConfigFromAgentRuntimeOptions parses notifier.Config from agent-level runtime_options only.
func ConfigFromAgentRuntimeOptions(agentRuntimeOptions map[string]any) Config {
	return ConfigFromStored(NotifierFlatFromAgentRuntimeOptions(agentRuntimeOptions))
}
