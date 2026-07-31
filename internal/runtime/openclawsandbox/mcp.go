package openclawsandbox

import (
	"fmt"
	"math"
	"path"
	"reflect"
	goruntime "runtime"
	"strings"

	"csgclaw/internal/mcpschema"
	agentruntime "csgclaw/internal/runtime"
)

func validateOpenClawMCPServers(config map[string]any) error {
	_, err := resolveOpenClawMCPWorkspaceConfig(config, "")
	return err
}

func openClawMCPRestartRequired(previous, current map[string]any) (bool, error) {
	return agentruntime.MCPServersNeedsRestart(previous, current)
}

func updateOpenClawMCP(cfg map[string]any, mcpServers map[string]any) error {
	resolved, err := resolveOpenClawMCPWorkspaceConfig(mcpServers, workspaceGuestPathForGOOS(goruntime.GOOS))
	if err != nil {
		return err
	}
	return updateOpenClawJSONMCPServers(cfg, resolved)
}

func resolveOpenClawMCPWorkspaceConfig(mcpServers map[string]any, workspaceGuestPath string) (map[string]any, error) {
	servers, err := mcpschema.NormalizeMCPServers(mcpServers)
	if err != nil {
		return nil, err
	}
	if servers == nil {
		return nil, nil
	}
	resolvedServers := make(map[string]any, len(servers))
	for name, rawEntry := range servers {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			resolvedServers[name] = rawEntry
			continue
		}
		next := make(map[string]any, len(entry))
		for key, value := range entry {
			next[key] = value
		}
		if err := projectOpenClawMCPTimeouts(next); err != nil {
			return nil, fmt.Errorf("%s.%s: %w", mcpschema.MCPServersKey, name, err)
		}
		if args, ok := next["args"].([]any); ok {
			next["args"] = resolveOpenClawMCPWorkspaceArgs(args, workspaceGuestPath)
		}
		if transport, ok := next["transport"].(string); ok {
			normalizedTransport, err := normalizeOpenClawMCPTransport(transport)
			if err != nil {
				return nil, fmt.Errorf("%s.%s.transport: %w", mcpschema.MCPServersKey, name, err)
			}
			next["transport"] = normalizedTransport
		}
		resolvedServers[name] = next
	}
	return resolvedServers, nil
}

// openClawMCPServersToGeneric restores OpenClaw's native timeout fields to
// CSGClaw's shared MCP schema before agent management persists the servers.
func openClawMCPServersToGeneric(servers map[string]any) (map[string]any, error) {
	if servers == nil {
		return nil, nil
	}
	genericServers := make(map[string]any, len(servers))
	for name, rawEntry := range servers {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			genericServers[name] = rawEntry
			continue
		}
		next := make(map[string]any, len(entry))
		for key, value := range entry {
			next[key] = value
		}
		if err := restoreOpenClawMCPTimeouts(next); err != nil {
			return nil, fmt.Errorf("%s.%s: %w", mcpschema.MCPServersKey, name, err)
		}
		genericServers[name] = next
	}
	return mcpschema.NormalizeMCPServers(genericServers)
}

// updateOpenClawJSONMCPServers writes the already-normalized MCP map into
// OpenClaw's native config shape. This projection belongs to the OpenClaw
// adapter because other runtimes use different configuration formats.
func updateOpenClawJSONMCPServers(cfg map[string]any, servers map[string]any) error {
	if servers == nil {
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
	mcpRoot["servers"] = servers
	return nil
}

// projectOpenClawMCPTimeouts translates CSGClaw's runtime-neutral seconds
// fields into OpenClaw's per-server millisecond settings. Do not persist the
// source fields in openclaw.json: OpenClaw does not consume them.
func projectOpenClawMCPTimeouts(entry map[string]any) error {
	if err := projectOpenClawMCPTimeout(entry, "startup_timeout_sec", "connectionTimeoutMs"); err != nil {
		return err
	}
	return projectOpenClawMCPTimeout(entry, "tool_timeout_sec", "requestTimeoutMs")
}

func restoreOpenClawMCPTimeouts(entry map[string]any) error {
	if err := restoreOpenClawMCPTimeout(entry, "connectionTimeoutMs", "startup_timeout_sec"); err != nil {
		return err
	}
	return restoreOpenClawMCPTimeout(entry, "requestTimeoutMs", "tool_timeout_sec")
}

func projectOpenClawMCPTimeout(entry map[string]any, source, target string) error {
	value, ok := entry[source]
	if !ok {
		return nil
	}
	seconds, ok := openClawMCPPositiveInteger(value)
	if !ok {
		return fmt.Errorf("%s must be a positive integer", source)
	}
	if seconds > math.MaxInt64/1000 {
		return fmt.Errorf("%s is too large", source)
	}
	entry[target] = seconds * 1000
	delete(entry, source)
	return nil
}

func restoreOpenClawMCPTimeout(entry map[string]any, source, target string) error {
	value, ok := entry[source]
	if !ok {
		return nil
	}
	milliseconds, ok := openClawMCPPositiveInteger(value)
	if !ok || milliseconds%1000 != 0 {
		return fmt.Errorf("%s must be a positive multiple of 1000", source)
	}
	entry[target] = milliseconds / 1000
	delete(entry, source)
	return nil
}

func openClawMCPPositiveInteger(value any) (int64, bool) {
	reflectValue := reflect.ValueOf(value)
	switch reflectValue.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		integer := reflectValue.Int()
		return integer, integer > 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		integer := reflectValue.Uint()
		if integer == 0 || integer > math.MaxInt64 {
			return 0, false
		}
		return int64(integer), true
	case reflect.Float32, reflect.Float64:
		integer := reflectValue.Float()
		if math.IsNaN(integer) || math.IsInf(integer, 0) || math.Trunc(integer) != integer || integer <= 0 || integer >= float64(math.MaxInt64) {
			return 0, false
		}
		return int64(integer), true
	default:
		return 0, false
	}
}

func normalizeOpenClawMCPTransport(transport string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "sse":
		return "sse", nil
	case "streamable-http", "streamable_http":
		return "streamable-http", nil
	default:
		return "", fmt.Errorf("must be %q or %q", "sse", "streamable-http")
	}
}

func resolveOpenClawMCPWorkspaceArgs(args []any, workspaceGuestPath string) []any {
	workspaceGuestPath = strings.TrimSpace(workspaceGuestPath)
	if workspaceGuestPath == "" {
		return args
	}
	out := make([]any, len(args))
	for idx, arg := range args {
		text, ok := arg.(string)
		if !ok {
			out[idx] = arg
			continue
		}
		out[idx] = resolveOpenClawMCPWorkspaceArg(text, workspaceGuestPath)
	}
	return out
}

func resolveOpenClawMCPWorkspaceArg(arg, workspaceGuestPath string) string {
	for _, placeholder := range []string{"${workspace}", "${workspaceDir}", "{workspace}", "{workspaceDir}"} {
		if arg == placeholder {
			return workspaceGuestPath
		}
		if strings.HasPrefix(arg, placeholder+"/") {
			return path.Join(workspaceGuestPath, strings.TrimPrefix(arg, placeholder+"/"))
		}
	}
	return arg
}
