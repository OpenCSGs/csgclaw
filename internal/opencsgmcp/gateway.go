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
		entry["url"] = strings.TrimRight(strings.TrimSpace(proxyURL), "/") + "/api/v1/opencsg-mcp-gateway/mcp"
		entry["transport"] = "streamable-http"
		if meta := managedMetadata(entry); meta != nil {
			if bindings, ok := meta["file_bindings"].(map[string]any); ok && len(bindings) > 0 {
				entry[RuntimeFileBindingsKey] = bindings
			}
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

// FileBindings returns the template-declared file argument mappings.
func FileBindings(config any) map[string]any {
	entry, _ := config.(map[string]any)
	meta := managedMetadata(entry)
	bindings, _ := meta["file_bindings"].(map[string]any)
	return bindings
}

// FileBridgeToken scopes the internal server credential to one Agent and MCP
// server, so a Runtime cannot reuse it to access another Agent's files.
func FileBridgeToken(serverToken, agentID, serverName string) string {
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(serverToken)))
	_, _ = mac.Write([]byte(strings.TrimSpace(agentID) + "\x00" + strings.TrimSpace(serverName)))
	return hex.EncodeToString(mac.Sum(nil))
}

func ValidFileBridgeToken(token, serverToken, agentID, serverName string) bool {
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
