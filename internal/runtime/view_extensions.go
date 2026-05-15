package runtime

// RuntimeOptionKeyNotifierProfile is the runtime_options key for derived notifier API summary.
// It is merged for JSON responses and must be stripped before persisting profile or agent options.
const RuntimeOptionKeyNotifierProfile = "notifier_profile"

// viewOnlyRuntimeOptionRootKeys lists root-level runtime_options keys that are API/view derived
// and must not be written to storage. When a new runtime adds echo-back metadata at this map level,
// register its key here (and add any nested stripping logic in StripViewOnlyRuntimeOptions if needed).
var viewOnlyRuntimeOptionRootKeys = []string{
	RuntimeOptionKeyNotifierProfile,
}

// StripViewOnlyRuntimeOptions removes view-only keys from runtime_options before normalize/persist.
// It is the single entry point for agent profile handling; runtime-specific rules stay in one list above.
func StripViewOnlyRuntimeOptions(ext map[string]any) map[string]any {
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
	out := CloneAnyMap(ext)
	for _, k := range viewOnlyRuntimeOptionRootKeys {
		delete(out, k)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
