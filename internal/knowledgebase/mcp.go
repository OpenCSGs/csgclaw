package knowledgebase

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"csgclaw/internal/mcpschema"
)

const (
	ManagedConfigKey     = "csgclaw"
	ManagedKind          = "agentichub_knowledge_base"
	ManagedMetaKey       = "_meta"
	ManagedMetaNamespace = "com.opencsg/mcp"
	ManagedMCPType       = "llm_wiki"
	ManagedAuthType      = "csghub_access_token"
)

type Availability string

const (
	AvailabilityAvailable   Availability = "available"
	AvailabilityUnavailable Availability = "temporarily_unavailable"
)

type ManagedMetadata struct {
	Kind            string
	KnowledgeBaseID int64
	ContentID       string
}

func AvailabilityFor(item KnowledgeBase) (Availability, string) {
	if strings.TrimSpace(item.ContentID) == "" {
		return AvailabilityUnavailable, "missing_content"
	}
	state := item.Metadata.ResourceState
	if state == nil {
		return AvailabilityUnavailable, "remote_state_unavailable"
	}
	if state.MCPStatus != "ready" || mcpEndpointFor(item) == "" {
		return AvailabilityUnavailable, "mcp_not_ready"
	}
	if state.Readiness != "" && state.Readiness != "ready" {
		return AvailabilityUnavailable, "content_not_ready"
	}
	return AvailabilityAvailable, ""
}

// mcpEndpointFor prefers the stable public endpoint returned at
// metadata.mcp_endpoint_url. The resource-state fallback keeps CSGClaw
// compatible with AgenticHub versions that returned the URL in the live state.
func mcpEndpointFor(item KnowledgeBase) string {
	if endpoint := strings.TrimSpace(item.Metadata.MCPEndpoint); endpoint != "" {
		return endpoint
	}
	if item.Metadata.ResourceState == nil {
		return ""
	}
	return strings.TrimSpace(item.Metadata.ResourceState.MCPEndpoint)
}

func ServerName(item KnowledgeBase) string {
	return "agentichub-kb-" + strconv.FormatInt(item.ID, 10)
}

func ServerConfig(item KnowledgeBase, csgHubAccessToken string) (string, map[string]any, error) {
	if item.ID < 1 {
		return "", nil, fmt.Errorf("knowledge base id is required")
	}
	if availability, reason := AvailabilityFor(item); availability != AvailabilityAvailable {
		return "", nil, fmt.Errorf("knowledge base is unavailable: %s", reason)
	}
	name := ServerName(item)
	config := map[string]any{
		"type":        "remote",
		"url":         mcpEndpointFor(item),
		"transport":   "streamable-http",
		"description": strings.TrimSpace(item.Name) + " | AgenticHub knowledge base",
		"headers": map[string]any{
			"Authorization": "Bearer " + strings.TrimSpace(csgHubAccessToken),
		},
		ManagedMetaKey: map[string]any{
			ManagedMetaNamespace: map[string]any{
				"type":        ManagedMCPType,
				"resource_id": strconv.FormatInt(item.ID, 10),
				"content_id":  item.ContentID,
				"auth_type":   ManagedAuthType,
			},
		},
	}
	if strings.TrimSpace(csgHubAccessToken) == "" {
		return "", nil, fmt.Errorf("CSGHub access token is required")
	}
	normalized, err := mcpschema.NormalizeMCPServers(map[string]any{name: config})
	if err != nil {
		return "", nil, err
	}
	return name, normalized[name].(map[string]any), nil
}

// HydrateManagedServer refreshes a managed knowledge-base MCP from the current
// user's CSGHub record. The API-provided public endpoint is used verbatim so
// CSGClaw does not duplicate AIGateway routing rules.
func HydrateManagedServer(ctx context.Context, config map[string]any, connection Connection) (map[string]any, error) {
	metadata, ok := ManagedMetadataFromServer(config)
	if !ok {
		return config, nil
	}
	baseURL := strings.TrimRight(strings.TrimSpace(connection.CSGHubBaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("hydrate knowledge base MCP: CSGHub base URL is required")
	}
	token := strings.TrimSpace(connection.CSGHubAccessToken)
	if token == "" {
		return nil, fmt.Errorf("hydrate knowledge base MCP: CSGHub access token is required")
	}

	item, err := (Client{BaseURL: baseURL, Token: token}).Get(ctx, metadata.KnowledgeBaseID)
	if err != nil {
		return nil, fmt.Errorf("refresh knowledge base MCP: %w", err)
	}
	if item.ID != metadata.KnowledgeBaseID || item.ContentID != metadata.ContentID {
		return nil, fmt.Errorf("refresh knowledge base MCP: resource identity changed")
	}
	if availability, reason := AvailabilityFor(item); availability != AvailabilityAvailable {
		return nil, fmt.Errorf("refresh knowledge base MCP: knowledge base is unavailable: %s", reason)
	}

	return hydrateServerConfig(config, mcpEndpointFor(item), token)
}

func hydrateServerConfig(config map[string]any, endpoint, token string) (map[string]any, error) {
	encoded, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("clone knowledge base MCP config: %w", err)
	}
	prepared := map[string]any{}
	if err := json.Unmarshal(encoded, &prepared); err != nil {
		return nil, fmt.Errorf("clone knowledge base MCP config: %w", err)
	}
	prepared["url"] = strings.TrimSpace(endpoint)
	prepared["transport"] = "streamable-http"
	prepared["headers"] = map[string]any{"Authorization": "Bearer " + strings.TrimSpace(token)}
	return prepared, nil
}

// HydrateTemplateServers injects the current template runner's CSGHub access
// token into managed knowledge-base MCP servers. The URL is resolved from the
// runner's current CSGHub record instead of trusting a community template URL.
func HydrateTemplateServers(ctx context.Context, servers map[string]any) (map[string]any, error) {
	hydrated, err := cloneServers(servers)
	if err != nil || hydrated == nil {
		return hydrated, err
	}
	managed := false
	for _, raw := range hydrated {
		if _, ok := ManagedMetadataFromServer(raw); ok {
			managed = true
			break
		}
	}
	if !managed {
		return hydrated, nil
	}
	connection, err := LoadConnection(ctx)
	if err != nil {
		return nil, fmt.Errorf("hydrate knowledge base MCP servers: %w", err)
	}
	for name, raw := range hydrated {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, managed := ManagedMetadataFromServer(entry); !managed {
			continue
		}
		prepared, err := HydrateManagedServer(ctx, entry, connection)
		if err != nil {
			return nil, fmt.Errorf("hydrate %s: %w", name, err)
		}
		hydrated[name] = prepared
	}
	return hydrated, nil
}

func ManagedMetadataFromServer(config any) (ManagedMetadata, bool) {
	entry, ok := config.(map[string]any)
	if !ok {
		return ManagedMetadata{}, false
	}
	if metadata, ok := managedMetadataFromMeta(entry); ok {
		return metadata, true
	}
	return managedMetadataFromLegacyConfig(entry)
}

func managedMetadataFromMeta(entry map[string]any) (ManagedMetadata, bool) {
	meta, ok := entry[ManagedMetaKey].(map[string]any)
	if !ok {
		return ManagedMetadata{}, false
	}
	raw, ok := meta[ManagedMetaNamespace].(map[string]any)
	if !ok || strings.TrimSpace(stringValue(raw["type"])) != ManagedMCPType || strings.TrimSpace(stringValue(raw["auth_type"])) != ManagedAuthType {
		return ManagedMetadata{}, false
	}
	id, err := strconv.ParseInt(strings.TrimSpace(stringValue(raw["resource_id"])), 10, 64)
	if err != nil || id < 1 {
		return ManagedMetadata{}, false
	}
	contentID := strings.TrimSpace(stringValue(raw["content_id"]))
	if contentID == "" {
		return ManagedMetadata{}, false
	}
	return ManagedMetadata{Kind: ManagedKind, KnowledgeBaseID: id, ContentID: contentID}, true
}

func managedMetadataFromLegacyConfig(entry map[string]any) (ManagedMetadata, bool) {
	raw, ok := entry[ManagedConfigKey].(map[string]any)
	if !ok || strings.TrimSpace(stringValue(raw["kind"])) != ManagedKind {
		return ManagedMetadata{}, false
	}
	id, err := strconv.ParseInt(strings.TrimSpace(stringValue(raw["knowledge_base_id"])), 10, 64)
	if err != nil || id < 1 {
		return ManagedMetadata{}, false
	}
	contentID := strings.TrimSpace(stringValue(raw["content_id"]))
	if contentID == "" {
		return ManagedMetadata{}, false
	}
	return ManagedMetadata{Kind: ManagedKind, KnowledgeBaseID: id, ContentID: contentID}, true
}

func FindConfiguredServer(servers map[string]any, knowledgeBaseID int64) string {
	for name, raw := range servers {
		metadata, ok := ManagedMetadataFromServer(raw)
		if ok && metadata.KnowledgeBaseID == knowledgeBaseID {
			return name
		}
	}
	return ""
}

// RuntimeServers removes CSGClaw-only management metadata while retaining the
// direct URL and Authorization header required by the MCP runtime.
func RuntimeServers(servers map[string]any) (map[string]any, error) {
	runtimeServers, err := cloneServers(servers)
	if err != nil || runtimeServers == nil {
		return runtimeServers, err
	}
	for _, raw := range runtimeServers {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, managed := ManagedMetadataFromServer(entry); managed {
			removeManagedMetadata(entry)
		}
	}
	return runtimeServers, nil
}

func cloneServers(servers map[string]any) (map[string]any, error) {
	if servers == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(servers)
	if err != nil {
		return nil, fmt.Errorf("clone mcp servers: %w", err)
	}
	cloned := map[string]any{}
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, fmt.Errorf("clone mcp servers: %w", err)
	}
	return cloned, nil
}

func removeManagedMetadata(entry map[string]any) {
	delete(entry, ManagedConfigKey)
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
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return string(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}
