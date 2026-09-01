package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"csgclaw/internal/knowledgebase"
	"csgclaw/internal/mcp"
)

var errMCPServerSourceUnsupported = errors.New("mcp server has no refreshable source")

type mcpServerSourceStatusResponse struct {
	AuthType              string `json:"auth_type"`
	AgentUpdateAvailable  bool   `json:"agent_update_available,omitempty"`
	ConfiguredEndpointURL string `json:"configured_endpoint_url"`
	ContentID             string `json:"content_id"`
	GlobalServerName      string `json:"global_server_name,omitempty"`
	GlobalUpdateAvailable bool   `json:"global_update_available,omitempty"`
	Kind                  string `json:"kind"`
	LatestEndpointURL     string `json:"latest_endpoint_url"`
	ResourceID            string `json:"resource_id"`
	SourceDescription     string `json:"source_description,omitempty"`
	SourceName            string `json:"source_name,omitempty"`
	UpdateAvailable       bool   `json:"update_available"`
}

type mcpServerSourceSyncResponse struct {
	Source mcpServerSourceStatusResponse `json:"source"`
	State  map[string]any                `json:"state"`
}

type resolvedManagedMCPServerSource struct {
	CanonicalConfig map[string]any
	CanonicalName   string
	Connection      knowledgeBaseConnection
	Item            knowledgebase.KnowledgeBase
	Refreshed       map[string]any
	Status          mcpServerSourceStatusResponse
}

func (h *Handler) handleMCPServerSourceByName(w http.ResponseWriter, r *http.Request) {
	if h.mcp == nil {
		http.Error(w, "mcp service is not configured", http.StatusServiceUnavailable)
		return
	}
	name := strings.TrimSpace(pathValue(r, "name"))
	if name == "" {
		http.NotFound(w, r)
		return
	}
	resolved, err := h.resolveMCPServerSource(r.Context(), name)
	if err != nil {
		writeMCPServerSourceError(w, err)
		return
	}
	resolved.Status.GlobalServerName = name
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, resolved.Status)
	case http.MethodPost:
		state, err := h.mcp.UpdateServer(r.Context(), name, name, resolved.Refreshed)
		if err != nil {
			writeMCPServerError(w, err)
			return
		}
		resolved.Status.ConfiguredEndpointURL = resolved.Status.LatestEndpointURL
		resolved.Status.GlobalServerName = name
		resolved.Status.UpdateAvailable = false
		writeJSON(w, http.StatusOK, mcpServerSourceSyncResponse{Source: resolved.Status, State: state})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) resolveMCPServerSource(ctx context.Context, name string) (resolvedManagedMCPServerSource, error) {
	servers, err := h.mcp.ListServers(ctx)
	if err != nil {
		return resolvedManagedMCPServerSource{}, err
	}
	raw, ok := servers[name]
	if !ok {
		return resolvedManagedMCPServerSource{}, fmt.Errorf("%w: %s", mcp.ErrServerNotFound, name)
	}
	config, ok := raw.(map[string]any)
	if !ok {
		return resolvedManagedMCPServerSource{}, fmt.Errorf("mcp server %q config must be an object", name)
	}
	return h.resolveManagedMCPServerSource(ctx, config)
}

func (h *Handler) resolveManagedMCPServerSource(ctx context.Context, config map[string]any) (resolvedManagedMCPServerSource, error) {
	metadata, ok := knowledgebase.ManagedMetadataFromServer(config)
	if !ok {
		return resolvedManagedMCPServerSource{}, errMCPServerSourceUnsupported
	}
	connection, err := loadKnowledgeBaseConnection(ctx)
	if err != nil {
		return resolvedManagedMCPServerSource{}, err
	}
	item, refreshed, err := knowledgebase.RefreshManagedServerSnapshot(ctx, config, connection)
	if err != nil {
		return resolvedManagedMCPServerSource{}, err
	}
	canonicalName, canonicalConfig, err := knowledgebase.ServerConfig(item, connection.CSGHubAccessToken)
	if err != nil {
		return resolvedManagedMCPServerSource{}, err
	}
	return resolvedManagedMCPServerSource{
		CanonicalConfig: canonicalConfig,
		CanonicalName:   canonicalName,
		Connection:      connection,
		Item:            item,
		Refreshed:       refreshed,
		Status: mcpServerSourceStatusResponse{
			AuthType:              knowledgebase.ManagedAuthType,
			ConfiguredEndpointURL: mcpSourceString(config["url"]),
			ContentID:             metadata.ContentID,
			Kind:                  metadata.Kind,
			LatestEndpointURL:     mcpSourceString(refreshed["url"]),
			ResourceID:            strconv.FormatInt(metadata.KnowledgeBaseID, 10),
			SourceDescription:     strings.TrimSpace(item.Description),
			SourceName:            strings.TrimSpace(item.Name),
			UpdateAvailable:       managedMCPRuntimeSnapshotChanged(config, refreshed),
		},
	}, nil
}

func mcpSourceString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func managedMCPRuntimeSnapshotChanged(current, refreshed map[string]any) bool {
	for _, key := range []string{"type", "url", "transport", "headers"} {
		if !reflect.DeepEqual(current[key], refreshed[key]) {
			return true
		}
	}
	return false
}

func writeMCPServerSourceError(w http.ResponseWriter, err error) {
	if errors.Is(err, mcp.ErrServerNotFound) {
		writeMCPServerError(w, err)
		return
	}
	if errors.Is(err, errMCPServerSourceUnsupported) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeKnowledgeBaseError(w, err)
}
