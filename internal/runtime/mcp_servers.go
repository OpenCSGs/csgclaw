package runtime

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strings"
)

// MCPServersKey is the catalog response key. Agent state and agent APIs store
// the server map directly rather than wrapping it in another object.
const MCPServersKey = "mcpServers"

type MCPServersSnapshot struct {
	Servers map[string]any
}

type MCPServersChange struct {
	Previous MCPServersSnapshot
	Current  MCPServersSnapshot
}

type MCPServersController interface {
	ValidateMCPServers(ctx context.Context, current MCPServersSnapshot) error
	MCPServersRestartRequired(change MCPServersChange) (bool, error)
}

// MCPServersReconciler is implemented by runtimes whose MCP configuration can
// be safely applied to a live runtime. Runtimes such as OpenClaw apply MCP
// configuration only as part of provisioning or recreation.
type MCPServersReconciler interface {
	ReconcileMCPServers(ctx context.Context, h Handle, change MCPServersChange) error
}

type MCPServersListController interface {
	ListMCPServers(ctx context.Context, h Handle, current MCPServersSnapshot) (MCPServersSnapshot, error)
}

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

func MCPServersNeedsRestart(previous, current map[string]any) (bool, error) {
	previousNormalized, previousErr := NormalizeMCPServers(previous)
	currentNormalized, currentErr := NormalizeMCPServers(current)
	if currentErr != nil {
		return false, currentErr
	}
	if previousErr != nil {
		return true, nil
	}
	return !reflect.DeepEqual(previousNormalized, currentNormalized), nil
}

func UpdateJSONMCPServers(cfg map[string]any, servers map[string]any) error {
	normalized, err := NormalizeMCPServers(servers)
	if err != nil {
		return err
	}
	if normalized == nil {
		mcpRoot, ok := cfg["mcp"].(map[string]any)
		if !ok {
			return nil
		}
		delete(mcpRoot, "servers")
		if len(mcpRoot) == 0 {
			delete(cfg, "mcp")
		}
		return nil
	}
	mcpRoot, _ := cfg["mcp"].(map[string]any)
	if mcpRoot == nil {
		mcpRoot = map[string]any{}
		cfg["mcp"] = mcpRoot
	}
	mcpRoot["servers"] = normalized
	return nil
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
	// value that overflows int64 when Codex renders TOML.
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
