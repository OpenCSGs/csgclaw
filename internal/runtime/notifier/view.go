package notifier

import "csgclaw/internal/utils"

// RuntimeOptionKeyNotifierProfile is the runtime_options key for derived notifier API summary.
// It is merged for JSON responses and must be stripped before persisting profile or agent options.
const RuntimeOptionKeyNotifierProfile = "notifier_profile"

var viewOnlyRuntimeOptionRootKeys = []string{
	RuntimeOptionKeyNotifierProfile,
}

// StripViewOnlyRuntimeOptionKeys removes API-only keys that must never be persisted (e.g. notifier_profile summary).
func StripViewOnlyRuntimeOptionKeys(ext map[string]any) map[string]any {
	if len(ext) == 0 {
		return nil
	}
	needsCopy := false
	for _, k := range viewOnlyRuntimeOptionRootKeys {
		if _, ok := ext[k]; ok {
			needsCopy = true
			break
		}
	}
	if !needsCopy {
		return ext
	}
	out := utils.CloneAnyMap(ext)
	for _, k := range viewOnlyRuntimeOptionRootKeys {
		delete(out, k)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// RedactDetailsForAPI returns a copy of notifier details with secret token fields removed.
func RedactDetailsForAPI(nd map[string]any) map[string]any {
	if len(nd) == 0 {
		return nil
	}
	out := utils.CloneAnyMap(nd)
	delete(out, "webhook_token")
	delete(out, "remote_token")
	if len(out) == 0 {
		return nil
	}
	return out
}

// MergeRuntimeOptionMapsForView merges agent-level and profile-level option maps for API display (agent keys win).
func MergeRuntimeOptionMapsForView(agentRuntimeOptions, profileRuntimeOptions map[string]any) map[string]any {
	out := utils.CloneAnyMap(agentRuntimeOptions)
	if len(profileRuntimeOptions) == 0 {
		return out
	}
	if out == nil {
		out = make(map[string]any, len(profileRuntimeOptions))
	}
	for k, v := range profileRuntimeOptions {
		if _, ok := out[k]; !ok {
			out[k] = v
		}
	}
	return out
}
