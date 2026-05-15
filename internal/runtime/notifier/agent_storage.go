package notifier

import agentruntime "csgclaw/internal/runtime"

// NotifierStorageKeys lists flat keys for notifier delivery (canonical list in runtime.NotifierFlatStorageKeys).
var NotifierStorageKeys = agentruntime.NotifierFlatStorageKeys

// IsNotifierFlatRoot reports whether m looks like flat notifier_details at map root.
func IsNotifierFlatRoot(m map[string]any) bool {
	return agentruntime.ExtensionsHaveNotifierFlatKeys(m)
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
func ProfileRuntimeOptionsWithoutNotifierPayload(ext map[string]any) map[string]any {
	if len(ext) == 0 {
		return nil
	}
	out := CloneAnyMap(ext)
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
func NotifierFlatFromRuntimeOptionsMap(ext map[string]any) map[string]any {
	if len(ext) == 0 {
		return nil
	}
	ext = StripViewOnlyRuntimeOptionKeys(ext)
	if len(ext) == 0 {
		return nil
	}
	if flat := copyNotifierKeysFromMap(ext); len(flat) > 0 {
		return CloneAnyMap(flat)
	}
	return nil
}

// NotifierFlatFromAgentStorage returns notifier flat stored on the agent (runtime_options only).
func NotifierFlatFromAgentStorage(agentExt map[string]any) map[string]any {
	return NotifierFlatFromRuntimeOptionsMap(agentExt)
}

// MergeFlatForAgentPatch merges patch runtime_options onto the stored agent flat.
func MergeFlatForAgentPatch(agentExt, patchProfileRuntimeExt map[string]any) map[string]any {
	return mergeFlatForAgentPatch(agentExt, patchProfileRuntimeExt)
}

func mergeFlatForAgentPatch(agentExt, patchProfileRuntimeExt map[string]any) map[string]any {
	base := NotifierFlatFromAgentStorage(agentExt)
	incoming := NotifierFlatFromRuntimeOptionsMap(patchProfileRuntimeExt)
	return MergeNotifierFlatPatch(base, incoming)
}

// ApplyNotifierFlatPersistence writes merged notifier flat onto *agentRE when non-nil (agent-level storage),
// otherwise merges flat keys at the root of profile runtime_options (create path before Agent exists).
// It always strips nested notifier from a copy of profileRO and returns updated profile maps.
func ApplyNotifierFlatPersistence(agentRE *map[string]any, profileRE, profileRO map[string]any, mergedFlat map[string]any) (nextProfileRE, nextProfileRO map[string]any) {
	return applyNotifierFlatPersistence(agentRE, profileRE, profileRO, mergedFlat)
}

func applyNotifierFlatPersistence(agentRE *map[string]any, profileRE, profileRO map[string]any, mergedFlat map[string]any) (nextProfileRE, nextProfileRO map[string]any) {
	if len(mergedFlat) == 0 {
		return profileRE, profileRO
	}
	flat := CloneAnyMap(mergedFlat)
	flat = EnsurePullRemoteSubscriptionInNotifierDetails(flat)
	nextRO := CloneAnyMap(profileRO)
	StripNestedNotifier(nextRO)
	if len(nextRO) == 0 {
		nextRO = nil
	}
	if agentRE != nil {
		base := CloneAnyMap(*agentRE)
		if base == nil {
			base = make(map[string]any)
		}
		StripNotifierKeysFromRootMap(base)
		for k, v := range flat {
			if _, ok := notifierStorageKeySet[k]; ok {
				base[k] = v
			}
		}
		if len(base) == 0 {
			base = nil
		}
		*agentRE = base
		return ProfileRuntimeOptionsWithoutNotifierPayload(profileRE), nextRO
	}
	base := CloneAnyMap(profileRE)
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

// MergeRuntimeOptionMapsForView merges agent-level and profile-level option maps for API display (agent keys win).
func MergeRuntimeOptionMapsForView(agentExt, profileExt map[string]any) map[string]any {
	return agentruntime.MergeRuntimeOptionMapsForView(agentExt, profileExt)
}

// ConfigFromAgentParts parses notifier.Config from agent-level runtime_options only.
func ConfigFromAgentParts(agentLevelExt map[string]any) Config {
	return ConfigFromStored(NotifierFlatFromAgentStorage(agentLevelExt))
}
