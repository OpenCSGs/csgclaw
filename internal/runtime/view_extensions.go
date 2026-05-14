package runtime

// RuntimeExtensionKeyNotifierProfile is the runtime_extensions key for derived notifier API summary.
// It is merged for JSON responses and must be stripped before persisting profile or agent extensions.
const RuntimeExtensionKeyNotifierProfile = "notifier_profile"

// viewOnlyRuntimeExtensionRootKeys lists root-level runtime_extensions keys that are API/view derived
// and must not be written to storage. When a new runtime adds echo-back metadata at this map level,
// register its key here (and add any nested stripping logic in StripViewOnlyRuntimeExtensions if needed).
var viewOnlyRuntimeExtensionRootKeys = []string{
	RuntimeExtensionKeyNotifierProfile,
}

// StripViewOnlyRuntimeExtensions removes view-only keys from runtime_extensions before normalize/persist.
// It is the single entry point for agent profile handling; runtime-specific rules stay in one list above.
func StripViewOnlyRuntimeExtensions(ext map[string]any) map[string]any {
	if len(ext) == 0 {
		return nil
	}
	needsCopy := false
	for _, k := range viewOnlyRuntimeExtensionRootKeys {
		if _, ok := ext[k]; ok {
			needsCopy = true
			break
		}
	}
	if !needsCopy {
		return ext
	}
	out := CloneAnyMap(ext)
	for _, k := range viewOnlyRuntimeExtensionRootKeys {
		delete(out, k)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
