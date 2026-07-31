package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	remoteServersPath          = "/api/v1/agent/mcp-servers"
	remoteServersRequestLimit  = 4 * 1024 * 1024
	remoteServerEnabled        = true
	remoteServerStartupTimeout = 30
	remoteServerToolTimeout    = 60
)

var remoteServersHTTPClient = &http.Client{Timeout: 20 * time.Second}

// RemoteServerListOptions describes the supported marketplace list filters.
type RemoteServerListOptions struct {
	Page   int
	Per    int
	Search string
}

// RemoteServer is an installable MCP server from the configured OpenCSG Hub.
type RemoteServer struct {
	Description string
	Headers     map[string]string
	ID          string
	Name        string
	Protocol    string
	URL         string
}

// Config returns the CSGClaw MCP configuration for a fully resolved remote
// server. List responses are summaries and must be resolved by ID before this
// configuration is installed.
func (s RemoteServer) Config() map[string]any {
	config := map[string]any{
		"enabled":             remoteServerEnabled,
		"startup_timeout_sec": remoteServerStartupTimeout,
		"tool_timeout_sec":    remoteServerToolTimeout,
		"url":                 s.URL,
	}
	if transport := remoteServerTransport(s.Protocol); transport != "" {
		config["transport"] = transport
	}
	if headers := remoteServerHeaders(s.Headers); len(headers) > 0 {
		config["headers"] = headers
	}
	if description := strings.TrimSpace(s.Description); description != "" {
		config["description"] = description
	}
	return config
}

// RemoteServerPage is one page of OpenCSG Hub MCP servers.
type RemoteServerPage struct {
	// RecordCount is the number of upstream records before incomplete entries
	// are discarded from Items.
	RecordCount int
	Items       []RemoteServer
	Total       *int
}

// ListRemoteServers lists MCP servers published by an OpenCSG Hub. The caller
// supplies the Hub base URL selected by the current CSGClaw environment and
// the current user's access token when the Hub requires authentication.
func ListRemoteServers(
	ctx context.Context,
	baseURL, accessToken string,
	options RemoteServerListOptions,
) (RemoteServerPage, error) {
	endpoint, err := remoteServersURL(baseURL, options)
	if err != nil {
		return RemoteServerPage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return RemoteServerPage{}, fmt.Errorf("create remote MCP Hub request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if token := strings.TrimSpace(accessToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	body, err := requestRemoteServers(req)
	if err != nil {
		return RemoteServerPage{}, err
	}
	page, err := decodeRemoteServerPage(body)
	if err != nil {
		return RemoteServerPage{}, err
	}
	return page, nil
}

// GetRemoteServer resolves an installable MCP server's complete configuration
// from the configured OpenCSG Hub. Credentials in the returned headers stay
// server-side until CSGClaw writes the MCP catalog entry.
func GetRemoteServer(ctx context.Context, baseURL, accessToken, id string) (RemoteServer, error) {
	endpoint, err := remoteServerDetailURL(baseURL, id)
	if err != nil {
		return RemoteServer{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return RemoteServer{}, fmt.Errorf("create remote MCP Hub detail request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if token := strings.TrimSpace(accessToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	body, err := requestRemoteServers(req)
	if err != nil {
		return RemoteServer{}, err
	}
	server, err := decodeRemoteServerDetail(body)
	if err != nil {
		return RemoteServer{}, err
	}
	if server.ID == "" {
		server.ID = strings.TrimSpace(id)
	}
	return server, nil
}

func remoteServersURL(baseURL string, options RemoteServerListOptions) (string, error) {
	u, err := remoteServersBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	query := u.Query()
	page := options.Page
	if page < 1 {
		page = 1
	}
	per := options.Per
	if per < 1 {
		per = 12
	}
	query.Set("page", strconv.Itoa(page))
	query.Set("per", strconv.Itoa(per))
	if search := strings.TrimSpace(options.Search); search != "" {
		query.Set("search", search)
	} else {
		query.Del("search")
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func remoteServerDetailURL(baseURL, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" || strings.ContainsAny(id, "/\\") {
		return "", fmt.Errorf("invalid remote MCP server id")
	}
	u, err := remoteServersBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + id
	u.RawPath = ""
	return u.String(), nil
}

func remoteServersBaseURL(baseURL string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid remote MCP Hub URL")
	}
	basePath := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(basePath, "/api") {
		u.Path = path.Join(basePath, "v1", "agent", "mcp-servers")
	} else {
		u.Path = path.Join(basePath, remoteServersPath)
	}
	u.RawPath = ""
	u.RawQuery = ""
	return u, nil
}

func requestRemoteServers(req *http.Request) ([]byte, error) {
	resp, err := remoteServersHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request remote MCP Hub: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, remoteServersRequestLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read remote MCP Hub response: %w", err)
	}
	if len(body) > remoteServersRequestLimit {
		return nil, fmt.Errorf("remote MCP Hub response exceeds %d bytes", remoteServersRequestLimit)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf(
			"remote MCP Hub request failed with status %d: %s",
			resp.StatusCode,
			truncateRemoteServerBody(body),
		)
	}
	return body, nil
}

func decodeRemoteServerPage(body []byte) (RemoteServerPage, error) {
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Items json.RawMessage `json:"items"`
		Total *int            `json:"total"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return RemoteServerPage{}, fmt.Errorf("decode remote MCP Hub response: %w", err)
	}
	itemsRaw := envelope.Items
	total := envelope.Total
	if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		var nested struct {
			Data  json.RawMessage `json:"data"`
			Items json.RawMessage `json:"items"`
			Total *int            `json:"total"`
		}
		if err := json.Unmarshal(envelope.Data, &nested); err == nil && (len(nested.Data) > 0 || len(nested.Items) > 0) {
			if len(nested.Data) > 0 {
				itemsRaw = nested.Data
			} else {
				itemsRaw = nested.Items
			}
			if nested.Total != nil {
				total = nested.Total
			}
		} else {
			itemsRaw = envelope.Data
		}
	}
	if len(itemsRaw) == 0 || string(itemsRaw) == "null" {
		return RemoteServerPage{Items: []RemoteServer{}, Total: total}, nil
	}
	var records []remoteServerRecord
	if err := json.Unmarshal(itemsRaw, &records); err != nil {
		return RemoteServerPage{}, fmt.Errorf("decode remote MCP Hub server list: %w", err)
	}
	items := make([]RemoteServer, 0, len(records))
	for _, record := range records {
		item, ok := record.remoteServerSummary()
		if ok {
			items = append(items, item)
		}
	}
	return RemoteServerPage{Items: items, RecordCount: len(records), Total: total}, nil
}

func decodeRemoteServerDetail(body []byte) (RemoteServer, error) {
	raw := json.RawMessage(body)
	for range 2 {
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
			break
		}
		raw = envelope.Data
	}
	var record remoteServerRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return RemoteServer{}, fmt.Errorf("decode remote MCP server detail: %w", err)
	}
	server, ok := record.remoteServerDetail()
	if !ok {
		return RemoteServer{}, fmt.Errorf("remote MCP server detail is incomplete")
	}
	return server, nil
}

type remoteServerRecord struct {
	Description string            `json:"description"`
	Headers     map[string]string `json:"headers"`
	ID          json.RawMessage   `json:"id"`
	Name        string            `json:"name"`
	Protocol    string            `json:"protocol"`
	URL         string            `json:"url"`
}

func (r remoteServerRecord) remoteServerSummary() (RemoteServer, bool) {
	id := remoteServerID(r.ID)
	name := strings.TrimSpace(r.Name)
	if id == "" || name == "" {
		return RemoteServer{}, false
	}
	return RemoteServer{
		Description: strings.TrimSpace(r.Description),
		ID:          id,
		Name:        name,
		Protocol:    remoteServerTransport(r.Protocol),
		URL:         strings.TrimSpace(r.URL),
	}, true
}

func (r remoteServerRecord) remoteServerDetail() (RemoteServer, bool) {
	server, ok := r.remoteServerSummary()
	if !ok || strings.TrimSpace(server.URL) == "" || server.Protocol == "" {
		return RemoteServer{}, false
	}
	server.Headers = cloneRemoteServerHeaders(r.Headers)
	return server, true
}

func remoteServerTransport(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "streamable", "streamable-http", "streamable_http":
		return "streamable-http"
	case "sse":
		return "sse"
	default:
		return ""
	}
}

func remoteServerID(value json.RawMessage) string {
	value = json.RawMessage(strings.TrimSpace(string(value)))
	if len(value) == 0 || string(value) == "null" {
		return ""
	}
	var stringID string
	if err := json.Unmarshal(value, &stringID); err == nil {
		return strings.TrimSpace(stringID)
	}
	var number json.Number
	if err := json.Unmarshal(value, &number); err != nil {
		return ""
	}
	id := number.String()
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return ""
	}
	return id
}

func cloneRemoteServerHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(headers))
	for name, value := range headers {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		cloned[name] = value
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}

func remoteServerHeaders(headers map[string]string) map[string]any {
	cloned := cloneRemoteServerHeaders(headers)
	if len(cloned) == 0 {
		return nil
	}
	configHeaders := make(map[string]any, len(cloned))
	for name, value := range cloned {
		configHeaders[name] = value
	}
	return configHeaders
}

func truncateRemoteServerBody(body []byte) string {
	const max = 1024
	message := strings.TrimSpace(string(body))
	if len(message) <= max {
		return message
	}
	return message[:max] + "..."
}
