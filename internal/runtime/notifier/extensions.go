package notifier

import agentruntime "csgclaw/internal/runtime"

// RuntimeExtensionKeyNotifier names the legacy nested bucket under runtime_extensions; it is
// stripped on persist and omitted from API responses. Notifier delivery fields live as flat keys
// on the same map (see NotifierStorageKeys).
const RuntimeExtensionKeyNotifier = "notifier"

// RuntimeExtensionKeyNotifierProfile is the notifier view summary key under runtime_extensions (API only; see runtime.StripViewOnlyRuntimeExtensions).
const RuntimeExtensionKeyNotifierProfile = agentruntime.RuntimeExtensionKeyNotifierProfile

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
	return CloneAnyMap(m)
}

// NotifierFlatFromProfile reads notifier flat from profile-level runtime_extensions (e.g. create payload before agent extensions exist).
func NotifierFlatFromProfile(extensions map[string]any) map[string]any {
	return NotifierFlatFromRuntimeExtensionsMap(extensions)
}

// ConfigFromNotifierProfile parses Config from profile runtime_extensions only.
func ConfigFromNotifierProfile(extensions map[string]any) Config {
	return ConfigFromStored(NotifierFlatFromProfile(extensions))
}

// WithRuntimeExtension returns a copy of extensions with key set to flat (or deleted when flat is empty).
func WithRuntimeExtension(extensions map[string]any, key string, flat map[string]any) map[string]any {
	out := CloneAnyMap(extensions)
	if out == nil {
		out = make(map[string]any)
	}
	if len(flat) == 0 {
		delete(out, key)
	} else {
		out[key] = CloneAnyMap(flat)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// WithNotifierExtension sets legacy nested runtime_extensions["notifier"] (prefer flat root keys on extensions instead).
func WithNotifierExtension(extensions map[string]any, flat map[string]any) map[string]any {
	return WithRuntimeExtension(extensions, RuntimeExtensionKeyNotifier, flat)
}

// RedactRuntimeExtensionsForAPI returns a shallow copy of extensions with known secret-bearing subtrees redacted.
func RedactRuntimeExtensionsForAPI(extensions map[string]any) map[string]any {
	if len(extensions) == 0 {
		return nil
	}
	out := CloneAnyMap(extensions)
	if out == nil {
		return nil
	}
	delete(out, RuntimeExtensionKeyNotifierProfile)
	// Flat notifier keys at map root (canonical storage).
	if len(copyNotifierKeysFromMap(out)) > 0 {
		redRoot := agentruntime.RedactNotifierDetailsForAPI(copyNotifierKeysFromMap(out))
		for _, k := range NotifierStorageKeys {
			delete(out, k)
		}
		for k, v := range redRoot {
			out[k] = v
		}
	}
	delete(out, RuntimeExtensionKeyNotifier)
	if len(out) == 0 {
		return nil
	}
	return out
}

// StripViewOnlyRuntimeExtensionKeys removes API-only keys that must never be persisted (e.g. notifier_profile summary).
func StripViewOnlyRuntimeExtensionKeys(ext map[string]any) map[string]any {
	return agentruntime.StripViewOnlyRuntimeExtensions(ext)
}
