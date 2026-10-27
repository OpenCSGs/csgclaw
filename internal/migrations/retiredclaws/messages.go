package retiredclaws

import "maps"

// Normalize historical message envelopes at the storage boundary. The rest of
// the application can keep using the current activity format after an upgrade.
func MessageMetadata(metadata map[string]any) map[string]any {
	var result map[string]any
	for _, key := range []string{"openclaw", "picoclaw"} {
		legacy, ok := metadata[key].(map[string]any)
		if !ok {
			continue
		}
		if result == nil {
			result = maps.Clone(metadata)
		}
		current, _ := result["codex"].(map[string]any)
		current = maps.Clone(current)
		if current == nil {
			current = make(map[string]any)
		}
		for name, value := range legacy {
			if _, exists := current[name]; !exists {
				current[name] = value
			}
		}
		result["codex"] = current
		delete(result, key)
	}
	if result != nil {
		return result
	}
	return metadata
}
