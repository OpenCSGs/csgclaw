package retiredclaws

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

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

// PendingAgents protects legacy provider input until retirement has completed.
// An unreadable store retains that input so migration can retry after recovery.
func PendingAgents(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		return true
	}
	var state struct {
		Agents struct{ Items []map[string]json.RawMessage }
	}
	if json.Unmarshal(data, &state) != nil {
		return true
	}
	for _, agent := range state.Agents.Items {
		var runtime map[string]json.RawMessage
		if json.Unmarshal(agent["runtime"], &runtime) != nil {
			runtime = nil
		}
		for _, raw := range []json.RawMessage{runtime["kind"], runtime["name"], agent["runtime_kind"], agent["runtime_name"]} {
			if isRetiredRuntimeKind(migrationString(raw)) {
				return true
			}
		}
	}
	return false
}
