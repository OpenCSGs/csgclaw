// Package opencsgmcp materializes trusted OpenCSG MCP Gateway connections.
package opencsgmcp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ManagedMetaKey         = "_meta"
	ManagedMetaNamespace   = "com.opencsg/mcp"
	GatewayMCPType         = "opencsg_mcp_gateway"
	CSGHubAuthType         = "csghub_access_token"
	RuntimeFileBindingsKey = "_csgclaw_file_bindings"
	DefaultMaxFileBytes    = int64(10 << 20)
)

// IsGatewayServer reports whether config declares the trusted OpenCSG MCP
// Gateway authentication contract. The template-provided URL is deliberately
// not part of this identity because it is never trusted at runtime.
func IsGatewayServer(config any) bool {
	entry, ok := config.(map[string]any)
	if !ok {
		return false
	}
	meta, ok := entry[ManagedMetaKey].(map[string]any)
	if !ok {
		return false
	}
	raw, ok := meta[ManagedMetaNamespace].(map[string]any)
	if !ok {
		return false
	}
	return strings.TrimSpace(stringValue(raw["type"])) == GatewayMCPType &&
		strings.TrimSpace(stringValue(raw["auth_type"])) == CSGHubAuthType
}

// RuntimeServers rewrites managed Gateway entries to the trusted local proxy.
// It removes CSGClaw-only metadata before handing configuration to a runtime.
func RuntimeServers(servers map[string]any, proxyURL, serverAccessToken string) (map[string]any, error) {
	if servers == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(servers)
	if err != nil {
		return nil, fmt.Errorf("clone OpenCSG MCP servers: %w", err)
	}
	runtimeServers := map[string]any{}
	if err := json.Unmarshal(encoded, &runtimeServers); err != nil {
		return nil, fmt.Errorf("clone OpenCSG MCP servers: %w", err)
	}

	for name, raw := range runtimeServers {
		entry, ok := raw.(map[string]any)
		if !ok || !IsGatewayServer(entry) {
			continue
		}
		if strings.TrimSpace(proxyURL) == "" {
			return nil, fmt.Errorf("materialize OpenCSG MCP Gateway %q: CSGClaw proxy URL is unavailable", name)
		}
		if strings.TrimSpace(serverAccessToken) == "" {
			return nil, fmt.Errorf("materialize OpenCSG MCP Gateway %q: CSGClaw access token is unavailable", name)
		}
		entry["url"] = strings.TrimRight(strings.TrimSpace(proxyURL), "/") + "/api/v1/opencsg-mcp-gateway/mcp"
		entry["transport"] = "streamable-http"
		bindings, err := FileBindings(entry)
		if err != nil {
			return nil, fmt.Errorf("materialize OpenCSG MCP Gateway %q: %w", name, err)
		}
		if len(bindings) > 0 {
			entry[RuntimeFileBindingsKey] = bindings
		}
		headers := map[string]any{}
		if existing, ok := entry["headers"].(map[string]any); ok {
			for headerName, value := range existing {
				if !strings.EqualFold(strings.TrimSpace(headerName), "Authorization") {
					headers[headerName] = value
				}
			}
		}
		if token := strings.TrimSpace(serverAccessToken); token != "" {
			headers["Authorization"] = "Bearer " + token
		}
		if len(headers) > 0 {
			entry["headers"] = headers
		} else {
			delete(entry, "headers")
		}
		removeManagedMetadata(entry)
	}
	return runtimeServers, nil
}

func managedMetadata(entry map[string]any) map[string]any {
	meta, _ := entry[ManagedMetaKey].(map[string]any)
	raw, _ := meta[ManagedMetaNamespace].(map[string]any)
	return raw
}

// FileBindings validates and expands template-declared file bindings. A list
// uses the standard OpenCSG file-tool argument convention; a map can override
// those arguments for a non-standard upstream tool.
func FileBindings(config any) (map[string]any, error) {
	entry, _ := config.(map[string]any)
	meta := managedMetadata(entry)
	return normalizeFileBindings(meta["file_bindings"])
}

func normalizeFileBindings(raw any) (map[string]any, error) {
	if raw == nil {
		return nil, nil
	}
	bindings := map[string]any{}
	switch value := raw.(type) {
	case []any:
		for _, item := range value {
			name, ok := item.(string)
			name = strings.TrimSpace(name)
			if !ok || name == "" {
				return nil, fmt.Errorf("file_bindings list must contain non-empty tool names")
			}
			if _, exists := bindings[name]; exists {
				return nil, fmt.Errorf("file_bindings contains duplicate tool %q", name)
			}
			bindings[name] = defaultFileBinding()
		}
	case []string:
		for _, item := range value {
			name := strings.TrimSpace(item)
			if name == "" {
				return nil, fmt.Errorf("file_bindings list must contain non-empty tool names")
			}
			if _, exists := bindings[name]; exists {
				return nil, fmt.Errorf("file_bindings contains duplicate tool %q", name)
			}
			bindings[name] = defaultFileBinding()
		}
	case map[string]any:
		for rawName, item := range value {
			name := strings.TrimSpace(rawName)
			binding, ok := item.(map[string]any)
			if name == "" || !ok {
				return nil, fmt.Errorf("file_bindings must map tool names to objects")
			}
			normalized, err := normalizeFileBinding(binding)
			if err != nil {
				return nil, fmt.Errorf("file_bindings.%s: %w", name, err)
			}
			bindings[name] = normalized
		}
	default:
		return nil, fmt.Errorf("file_bindings must be an array of tool names or an object")
	}
	return bindings, nil
}

func defaultFileBinding() map[string]any {
	return map[string]any{
		"encoding":              "base64",
		"content_argument":      "content_base64",
		"filename_argument":     "filename",
		"content_type_argument": "content_type",
		"max_bytes":             DefaultMaxFileBytes,
	}
}

func normalizeFileBinding(binding map[string]any) (map[string]any, error) {
	encoding := strings.ToLower(strings.TrimSpace(stringValue(binding["encoding"])))
	content := strings.TrimSpace(stringValue(binding["content_argument"]))
	filename := strings.TrimSpace(stringValue(binding["filename_argument"]))
	contentType := strings.TrimSpace(stringValue(binding["content_type_argument"]))
	if encoding != "base64" {
		return nil, fmt.Errorf("encoding must be base64")
	}
	if content == "" || filename == "" {
		return nil, fmt.Errorf("content_argument and filename_argument are required")
	}
	seen := map[string]struct{}{}
	for _, argument := range []string{content, filename, contentType} {
		if argument == "" {
			continue
		}
		if _, exists := seen[argument]; exists {
			return nil, fmt.Errorf("file argument names must be distinct")
		}
		seen[argument] = struct{}{}
	}
	maxBytes, err := bindingMaxBytes(binding["max_bytes"])
	if err != nil {
		return nil, err
	}
	if maxBytes == 0 {
		maxBytes = DefaultMaxFileBytes
	}
	return map[string]any{
		"encoding":              encoding,
		"content_argument":      content,
		"filename_argument":     filename,
		"content_type_argument": contentType,
		"max_bytes":             maxBytes,
	}, nil
}

func bindingMaxBytes(raw any) (int64, error) {
	if raw == nil {
		return 0, nil
	}
	var value int64
	switch typed := raw.(type) {
	case int:
		value = int64(typed)
	case int64:
		value = typed
	case float64:
		if typed != float64(int64(typed)) {
			return 0, fmt.Errorf("max_bytes must be an integer")
		}
		value = int64(typed)
	default:
		return 0, fmt.Errorf("max_bytes must be an integer")
	}
	if value < 1 || value > DefaultMaxFileBytes {
		return 0, fmt.Errorf("max_bytes must be between 1 and %d", DefaultMaxFileBytes)
	}
	return value, nil
}

// FileBridgeToken scopes the internal server credential to one Agent and MCP
// server, so a Runtime cannot reuse it to access another Agent's files.
func FileBridgeToken(serverToken, agentID, serverName string) string {
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(serverToken)))
	_, _ = mac.Write([]byte(strings.TrimSpace(agentID) + "\x00" + strings.TrimSpace(serverName)))
	return hex.EncodeToString(mac.Sum(nil))
}

func ValidFileBridgeToken(token, serverToken, agentID, serverName string) bool {
	if strings.TrimSpace(token) == "" || strings.TrimSpace(serverToken) == "" || strings.TrimSpace(agentID) == "" || strings.TrimSpace(serverName) == "" {
		return false
	}
	want := FileBridgeToken(serverToken, agentID, serverName)
	return hmac.Equal([]byte(strings.TrimSpace(token)), []byte(want))
}

func removeManagedMetadata(entry map[string]any) {
	meta, ok := entry[ManagedMetaKey].(map[string]any)
	if !ok {
		return
	}
	delete(meta, ManagedMetaNamespace)
	if len(meta) == 0 {
		delete(entry, ManagedMetaKey)
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
