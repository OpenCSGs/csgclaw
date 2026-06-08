package notification

import (
	"fmt"
	"strings"

	"csgclaw/internal/utils"
)

// StorageKeys lists flat keys for notification delivery on participant metadata.
var StorageKeys = []string{
	"delivery_mode",
	"webhook_token",
	"remote_url",
	"remote_messages_url",
	"remote_ack_url",
	"remote_subscription_id",
	"poll_interval",
	"remote_token",
}

// ConfigFromStored parses Config from flat storage map.
func ConfigFromStored(storedFlat map[string]any) Config {
	if len(storedFlat) == 0 {
		return Config{}
	}
	return ParseNotifierDetails(storedFlat)
}

// ConfigFromMetadata parses Config from participant metadata.
func ConfigFromMetadata(metadata map[string]any) Config {
	return ConfigFromStored(FlatFromMetadataMap(metadata))
}

func isEmptySecret(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return strings.TrimSpace(fmt.Sprint(v)) == ""
}

var patchSkipEmptyIncomingKeys = map[string]struct{}{
	"webhook_token":          {},
	"remote_token":           {},
	"remote_messages_url":    {},
	"remote_ack_url":         {},
	"remote_subscription_id": {},
}

// MergeFlatPatchKeys overlays incoming flat keys onto base.
func MergeFlatPatchKeys(base, incoming map[string]any) map[string]any {
	if len(incoming) == 0 {
		return utils.CloneAnyMap(base)
	}
	out := utils.CloneAnyMap(base)
	if out == nil {
		out = make(map[string]any, len(incoming))
	}
	for k, v := range incoming {
		if _, preserve := patchSkipEmptyIncomingKeys[k]; preserve && isEmptySecret(v) {
			continue
		}
		out[k] = v
	}
	return out
}

func copyStorageKeysFromMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any)
	for _, k := range StorageKeys {
		if v, ok := src[k]; ok && v != nil {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeSecretForStorage(v any) string {
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" {
		return ""
	}
	if len(s) >= 7 && strings.EqualFold(s[:7], "bearer ") {
		s = strings.TrimSpace(s[7:])
	}
	return s
}

// NormalizeMetadataForStorage canonicalizes flat metadata before persisting to disk.
func NormalizeMetadataForStorage(flat map[string]any) map[string]any {
	if len(flat) == 0 {
		return nil
	}
	out := utils.CloneAnyMap(flat)
	if out == nil {
		return nil
	}
	if raw, ok := out["remote_url"]; ok {
		if normalized := NormalizeRemoteURLForStorage(fmt.Sprint(raw)); normalized != "" {
			out["remote_url"] = normalized
		}
	}
	for _, k := range []string{"remote_messages_url", "remote_ack_url"} {
		if isEmptySecret(out[k]) {
			delete(out, k)
		}
	}
	if raw, ok := out["webhook_token"]; ok {
		if s := normalizeSecretForStorage(raw); s != "" {
			out["webhook_token"] = s
		}
	}
	if raw, ok := out["remote_token"]; ok {
		if s := normalizeSecretForStorage(raw); s != "" {
			out["remote_token"] = s
		}
	}
	return out
}

// FlatFromMetadataMap returns delivery flat keys from metadata.
func FlatFromMetadataMap(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	metadata = StripViewOnlyMetadataKeys(metadata)
	if len(metadata) == 0 {
		return nil
	}
	if flat := copyStorageKeysFromMap(metadata); len(flat) > 0 {
		return utils.CloneAnyMap(flat)
	}
	return nil
}
