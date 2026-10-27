package retiredclaws

import "strings"

// BootstrapDefaults clears resolved and raw overrides together, preserving all
// unrelated environment references when the owning loader writes its snapshot.
func BootstrapDefaults(manager, worker, rawManager, rawWorker *string) bool {
	changed := false
	for i, value := range []*string{manager, worker} {
		if retiredBuiltinTemplate(*value) {
			*value = ""
			*([]*string{rawManager, rawWorker}[i]) = ""
			changed = true
		}
	}
	return changed
}

func retiredBuiltinTemplate(ref string) bool {
	ref = strings.TrimSpace(ref)
	ref = strings.Replace(ref, "builtin/", "builtin.", 1)
	switch ref {
	case "builtin.openclaw-manager", "builtin.openclaw-worker", "builtin.picoclaw-manager", "builtin.picoclaw-worker":
		return true
	default:
		return false
	}
}

func ObsoleteBootstrapKey(key string) bool {
	switch key {
	case "manager_image_override", "manager_image", "runtime_kind":
		return true
	default:
		return false
	}
}
