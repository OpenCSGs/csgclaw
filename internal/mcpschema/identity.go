package mcpschema

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

const DisplayNameKey = "display_name"
const MarketplaceMetaKey = "com.opencsg/marketplace"

// ServerDisplayName returns presentation text, never a runtime lookup key.
func ServerDisplayName(id string, raw any) string {
	if entry, ok := raw.(map[string]any); ok {
		if name, ok := entry[DisplayNameKey].(string); ok && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	return id
}

func ValidServerID(id string) bool {
	if id == "" {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// NewServerID chooses an identity once. Readable legal keys can be retained;
// subsequent renames only change display_name and never call this function.
func NewServerID(candidate string, used func(string) bool) string {
	if ValidServerID(candidate) && !used(candidate) {
		return candidate
	}
	for {
		var value [16]byte
		_, _ = rand.Read(value[:])
		id := "mcp_" + hex.EncodeToString(value[:])
		if !used(id) {
			return id
		}
	}
}

// WithServerIdentities imports a template or native runtime map into managed
// state. Existing legal keys stay stable; all entries acquire display names.
func WithServerIdentities(servers map[string]any) (map[string]any, error) {
	normalized, err := NormalizeMCPServers(servers)
	if err != nil || normalized == nil {
		return normalized, err
	}
	names := make([]string, 0, len(normalized))
	for name := range normalized {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make(map[string]any, len(normalized))
	for _, name := range names {
		entry := normalized[name].(map[string]any)
		id := name
		if !ValidServerID(id) {
			id = NewServerID("", func(candidate string) bool { _, a := normalized[candidate]; _, b := result[candidate]; return a || b })
		}
		entry[DisplayNameKey] = ServerDisplayName(name, entry)
		result[id] = entry
	}
	return result, nil
}

func validateDisplayName(entry map[string]any) error {
	if raw, exists := entry[DisplayNameKey]; exists {
		name, ok := raw.(string)
		if !ok || strings.TrimSpace(name) == "" {
			return fmt.Errorf("display_name must be a non-empty string")
		}
		entry[DisplayNameKey] = strings.TrimSpace(name)
	}
	return nil
}

// StripPresentation removes CSGClaw-owned fields from an already copied map.
func StripPresentation(servers map[string]any) {
	for _, raw := range servers {
		if entry, ok := raw.(map[string]any); ok {
			delete(entry, DisplayNameKey)
			if meta, ok := entry["_meta"].(map[string]any); ok {
				delete(meta, MarketplaceMetaKey)
				if len(meta) == 0 {
					delete(entry, "_meta")
				}
			}
		}
	}
}
