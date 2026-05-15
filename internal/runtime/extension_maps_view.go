package runtime

// MergeRuntimeOptionMapsForView merges agent-level and profile-level option maps for API display (agent keys win).
func MergeRuntimeOptionMapsForView(agentExt, profileExt map[string]any) map[string]any {
	out := CloneAnyMap(agentExt)
	if len(profileExt) == 0 {
		return out
	}
	if out == nil {
		out = make(map[string]any, len(profileExt))
	}
	for k, v := range profileExt {
		if _, ok := out[k]; !ok {
			out[k] = v
		}
	}
	return out
}
