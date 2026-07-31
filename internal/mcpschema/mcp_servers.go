// Package mcpschema defines the runtime-neutral MCP server configuration
// accepted by CSGClaw's catalog, agent profiles, and runtime adapters.
package mcpschema

import (
	"fmt"
	"math"
	"strings"
)

// MCPServersKey is the JSON field used for a direct map of MCP server
// definitions. The catalog stores this map in its root state section, while
// agent state stores the map directly on each agent.
const MCPServersKey = "mcpServers"

// NormalizeMCPServers validates and copies a direct MCP server map. A nil map
// means that CSGClaw does not manage MCP servers for this agent; an empty map
// means that it manages an explicitly empty set.
func NormalizeMCPServers(servers map[string]any) (map[string]any, error) {
	if servers == nil {
		return nil, nil
	}
	normalized := make(map[string]any, len(servers))
	for rawName, rawEntry := range servers {
		name := strings.TrimSpace(rawName)
		if name == "" {
			return nil, fmt.Errorf("%s contains an empty server name", MCPServersKey)
		}
		if _, exists := normalized[name]; exists {
			return nil, fmt.Errorf("%s contains duplicate server name %q", MCPServersKey, name)
		}
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s.%s must be an object", MCPServersKey, name)
		}
		server, err := normalizeMCPServerEntry(name, entry)
		if err != nil {
			return nil, err
		}
		normalized[name] = server
	}
	return normalized, nil
}

func ValidateMCPServers(servers map[string]any) error {
	_, err := NormalizeMCPServers(servers)
	return err
}

func normalizeMCPServerEntry(name string, entry map[string]any) (map[string]any, error) {
	normalized, ok := cloneMCPJSONObject(entry).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s.%s must be an object", MCPServersKey, name)
	}
	command, hasCommand, err := mcpStringField(normalized, "command")
	if err != nil {
		return nil, fmt.Errorf("%s.%s.command %s", MCPServersKey, name, err)
	}
	url, hasURL, err := mcpStringField(normalized, "url")
	if err != nil {
		return nil, fmt.Errorf("%s.%s.url %s", MCPServersKey, name, err)
	}
	if !hasCommand && !hasURL {
		return nil, fmt.Errorf("%s.%s must declare command or url", MCPServersKey, name)
	}
	if hasCommand {
		normalized["command"] = command
	} else {
		delete(normalized, "command")
	}
	if hasURL {
		normalized["url"] = url
	} else {
		delete(normalized, "url")
	}
	if err := validateMCPStringSliceField(normalized, "args"); err != nil {
		return nil, fmt.Errorf("%s.%s.args must be an array of strings", MCPServersKey, name)
	}
	if err := validateMCPStringMapField(normalized, "env"); err != nil {
		return nil, fmt.Errorf("%s.%s.env must be an object with string values", MCPServersKey, name)
	}
	if err := validateMCPStringMapField(normalized, "headers"); err != nil {
		return nil, fmt.Errorf("%s.%s.headers must be an object with string values", MCPServersKey, name)
	}
	if headers, _ := normalized["headers"].(map[string]any); len(headers) > 0 {
		for headerName := range headers {
			if !ValidMCPHTTPHeaderName(headerName) {
				return nil, fmt.Errorf("%s.%s.headers contains an invalid HTTP header name", MCPServersKey, name)
			}
		}
	}
	if err := validateMCPIntegerField(normalized, "startup_timeout_sec"); err != nil {
		return nil, fmt.Errorf("%s.%s.startup_timeout_sec %s", MCPServersKey, name, err)
	}
	if err := validateMCPIntegerField(normalized, "tool_timeout_sec"); err != nil {
		return nil, fmt.Errorf("%s.%s.tool_timeout_sec %s", MCPServersKey, name, err)
	}
	if err := validateMCPStringField(normalized, "transport"); err != nil {
		return nil, fmt.Errorf("%s.%s.transport must be a string", MCPServersKey, name)
	}
	return normalized, nil
}

func mcpStringField(values map[string]any, key string) (string, bool, error) {
	raw, ok := values[key]
	if !ok || raw == nil {
		return "", false, nil
	}
	text, ok := raw.(string)
	if !ok {
		return "", false, fmt.Errorf("must be a string")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false, fmt.Errorf("must not be blank")
	}
	return text, true, nil
}

func validateMCPStringField(values map[string]any, key string) error {
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil
	}
	if _, ok := raw.(string); !ok {
		return fmt.Errorf("not a string")
	}
	return nil
}

func validateMCPStringSliceField(values map[string]any, key string) error {
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("not an array")
	}
	for _, item := range items {
		if _, ok := item.(string); !ok {
			return fmt.Errorf("contains non-string value")
		}
	}
	return nil
}

func validateMCPStringMapField(values map[string]any, key string) error {
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil
	}
	items, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("not an object")
	}
	for _, value := range items {
		if _, ok := value.(string); !ok {
			return fmt.Errorf("contains non-string value")
		}
	}
	return nil
}

// ValidMCPHTTPHeaderName reports whether value is an HTTP field-name token.
func ValidMCPHTTPHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			strings.ContainsRune("!#$%&'*+-.^_`|~", char) {
			continue
		}
		return false
	}
	return true
}

func validateMCPIntegerField(values map[string]any, key string) error {
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case int:
		if typed > 0 {
			return nil
		}
	case int8:
		if typed > 0 {
			return nil
		}
	case int16:
		if typed > 0 {
			return nil
		}
	case int32:
		if typed > 0 {
			return nil
		}
	case int64:
		if typed > 0 {
			return nil
		}
	case uint:
		if typed > 0 && uint64(typed) <= math.MaxInt64 {
			return nil
		}
	case uint8:
		if typed > 0 {
			return nil
		}
	case uint16:
		if typed > 0 {
			return nil
		}
	case uint32:
		if typed > 0 {
			return nil
		}
	case uint64:
		if typed > 0 && typed <= math.MaxInt64 {
			return nil
		}
	case float64:
		if validMCPIntegerFloat(typed) {
			return nil
		}
	}
	return fmt.Errorf("must be a positive integer")
}

func validMCPIntegerFloat(value float64) bool {
	// float64(math.MaxInt64) rounds up to 1<<63, so a <= check would allow a
	// value that overflows int64 when a runtime renders its native config.
	return !math.IsNaN(value) &&
		!math.IsInf(value, 0) &&
		math.Trunc(value) == value &&
		value > 0 &&
		value < float64(math.MaxInt64)
}

func cloneMCPJSONObject(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = cloneMCPJSONObject(item)
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for idx, item := range typed {
			out[idx] = cloneMCPJSONObject(item)
		}
		return out
	case []string:
		out := make([]any, len(typed))
		for idx, item := range typed {
			out[idx] = item
		}
		return out
	default:
		return value
	}
}
