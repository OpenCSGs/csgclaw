package notifier

import (
	"fmt"
	"strings"

	agentruntime "csgclaw/internal/runtime"
)

// CloneAnyMap returns a shallow copy of src, or nil if src is nil or has no entries.
func CloneAnyMap(src map[string]any) map[string]any {
	return agentruntime.CloneAnyMap(src)
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
	return CloneAnyMap(m)
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
		return CloneAnyMap(base)
	}
	out := CloneAnyMap(base)
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
		return CloneAnyMap(base)
	}
	out := CloneAnyMap(base)
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

// RedactDetailsForAPI returns a copy of notifier details with secret token fields removed.
func RedactDetailsForAPI(nd map[string]any) map[string]any {
	return agentruntime.RedactNotifierDetailsForAPI(nd)
}

// RedactedRequestOptionsForAPIView returns a copy of request_options safe for JSON responses:
// nested notifier webhook_token and remote_token are removed (use runtime_options.notifier_profile for summaries).
func RedactedRequestOptionsForAPIView(ro map[string]any) map[string]any {
	return agentruntime.RedactedRequestOptionsForAPIView(ro)
}
