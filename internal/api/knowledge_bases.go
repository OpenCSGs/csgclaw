package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"csgclaw/internal/knowledgebase"
)

const (
	remoteKnowledgeBasesDefaultPage = 1
	remoteKnowledgeBasesDefaultPer  = 20
	remoteKnowledgeBasesMaxPer      = 100
)

var (
	errKnowledgeBaseSignInRequired = errors.New("OpenCSG sign-in is required to browse knowledge bases")
	errKnowledgeBaseUnavailable    = errors.New("knowledge base is not available")
)

type knowledgeBaseConnection = knowledgebase.Connection

var loadInteractiveKnowledgeBaseConnection = knowledgebase.LoadInteractiveConnection

var loadKnowledgeBaseConnection = func(ctx context.Context) (knowledgeBaseConnection, error) {
	interactive, authenticated, interactiveErr := loadInteractiveKnowledgeBaseConnection(ctx)
	if authenticated {
		return interactive, nil
	}
	if managed, ok := managedKnowledgeBaseConnection(); ok {
		return managed, nil
	}
	if interactiveErr != nil {
		return knowledgeBaseConnection{}, interactiveErr
	}
	return knowledgeBaseConnection{}, errKnowledgeBaseSignInRequired
}

func managedKnowledgeBaseConnection() (knowledgeBaseConnection, bool) {
	return knowledgebase.ManagedConnection()
}

var knowledgeBaseProxyHTTPClient = &http.Client{
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

type remoteKnowledgeBasesListResponse struct {
	Items []remoteKnowledgeBaseSummary `json:"items"`
	Page  int                          `json:"page"`
	Per   int                          `json:"per"`
	Total int                          `json:"total"`
}

type remoteKnowledgeBaseSummary struct {
	Availability      knowledgebase.Availability `json:"availability"`
	CSGHubResponse    json.RawMessage            `json:"csghub_response"`
	ConfiguredMCP     string                     `json:"configured_mcp_name,omitempty"`
	ContentID         string                     `json:"content_id"`
	Description       string                     `json:"description,omitempty"`
	ID                int64                      `json:"id"`
	Name              string                     `json:"name"`
	UnavailableReason string                     `json:"unavailable_reason,omitempty"`
}

type knowledgeBaseMCPConfigResponse struct {
	Config map[string]any `json:"config"`
	Name   string         `json:"name"`
}

func (h *Handler) handleRemoteKnowledgeBases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	page, per, err := remoteHubPageOptions(r, remoteKnowledgeBasesDefaultPage, remoteKnowledgeBasesDefaultPer, remoteKnowledgeBasesMaxPer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	connection, err := loadKnowledgeBaseConnection(r.Context())
	if err != nil {
		writeKnowledgeBaseError(w, err)
		return
	}
	result, err := (knowledgebase.Client{BaseURL: connection.CSGHubBaseURL, Token: connection.CSGHubAccessToken}).List(r.Context(), knowledgebase.ListOptions{
		Page:   page,
		Per:    per,
		Search: r.URL.Query().Get("search"),
	})
	if err != nil {
		writeKnowledgeBaseError(w, err)
		return
	}
	servers := map[string]any{}
	if h.mcp != nil {
		servers, err = h.mcp.ListServers(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	items := make([]remoteKnowledgeBaseSummary, 0, len(result.Items))
	for index, item := range result.Items {
		availability, reason := knowledgebase.AvailabilityFor(item)
		var csgHubResponse json.RawMessage
		if index < len(result.RawItems) {
			csgHubResponse = result.RawItems[index]
		}
		items = append(items, remoteKnowledgeBaseSummary{
			Availability:      availability,
			CSGHubResponse:    csgHubResponse,
			ConfiguredMCP:     knowledgebase.FindConfiguredServer(servers, item.ContentID),
			ContentID:         item.ContentID,
			Description:       item.Description,
			ID:                item.ID,
			Name:              item.Name,
			UnavailableReason: reason,
		})
	}
	writeJSON(w, http.StatusOK, remoteKnowledgeBasesListResponse{
		Items: items,
		Page:  page,
		Per:   per,
		Total: result.Total,
	})
}

func (h *Handler) handleRemoteKnowledgeBaseMCPConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(pathValue(r, "id")), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	connection, err := loadKnowledgeBaseConnection(r.Context())
	if err != nil {
		writeKnowledgeBaseError(w, err)
		return
	}
	item, err := (knowledgebase.Client{BaseURL: connection.CSGHubBaseURL, Token: connection.CSGHubAccessToken}).Get(r.Context(), id)
	if err != nil {
		writeKnowledgeBaseError(w, err)
		return
	}
	name, config, err := knowledgebase.ServerConfig(item, connection.CSGHubAccessToken)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if h.mcp != nil {
		servers, listErr := h.mcp.ListServers(r.Context())
		if listErr != nil {
			http.Error(w, listErr.Error(), http.StatusBadGateway)
			return
		}
		if existing := knowledgebase.FindConfiguredServer(servers, item.ContentID); existing != "" {
			http.Error(w, "mcp server already exists: "+existing, http.StatusConflict)
			return
		}
	}
	writeJSON(w, http.StatusOK, knowledgeBaseMCPConfigResponse{Name: name, Config: config})
}

func (h *Handler) handleKnowledgeBaseMCPProxy(w http.ResponseWriter, r *http.Request) {
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	contentID := strings.TrimSpace(pathValue(r, "content_id"))
	if !validKnowledgeBaseContentID(contentID) {
		http.NotFound(w, r)
		return
	}
	connection, err := loadKnowledgeBaseConnection(r.Context())
	if err != nil {
		writeKnowledgeBaseError(w, err)
		return
	}
	baseURL := strings.TrimRight(connection.AIGatewayBaseURL, "/")
	if baseURL == "" {
		http.Error(w, "AIGateway base URL is not configured", http.StatusBadGateway)
		return
	}
	target := baseURL + "/llmwikis/" + url.PathEscape(contentID) + "/mcp"
	upstream, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		http.Error(w, "create knowledge base request", http.StatusBadGateway)
		return
	}
	copyKnowledgeBaseRequestHeaders(upstream.Header, r.Header)
	upstream.Header.Set("Authorization", "Bearer "+connection.CSGHubAccessToken)
	response, err := knowledgeBaseProxyHTTPClient.Do(upstream)
	if err != nil {
		http.Error(w, "knowledge base service is temporarily unavailable", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	copyKnowledgeBaseResponseHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	writer := io.Writer(w)
	if flusher, ok := w.(http.Flusher); ok {
		writer = flushResponseWriter{Writer: w, Flusher: flusher}
	}
	_, _ = io.Copy(writer, response.Body)
}

func writeKnowledgeBaseError(w http.ResponseWriter, err error) {
	if errors.Is(err, errKnowledgeBaseSignInRequired) {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if errors.Is(err, errKnowledgeBaseUnavailable) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	var remoteErr *knowledgebase.HTTPError
	if errors.As(err, &remoteErr) {
		switch remoteErr.StatusCode {
		case http.StatusUnauthorized:
			http.Error(w, errKnowledgeBaseSignInRequired.Error(), http.StatusUnauthorized)
		case http.StatusForbidden:
			http.Error(w, "knowledge base access is not permitted", http.StatusForbidden)
		case http.StatusNotFound:
			http.Error(w, "knowledge base not found", http.StatusNotFound)
		default:
			http.Error(w, "AgenticHub knowledge base service is temporarily unavailable", http.StatusBadGateway)
		}
		return
	}
	http.Error(w, err.Error(), http.StatusBadGateway)
}

func validKnowledgeBaseContentID(value string) bool {
	if value == "" || value == "." || value == ".." || len(value) > 256 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("-_.:", char) {
			continue
		}
		return false
	}
	return true
}

func copyKnowledgeBaseRequestHeaders(target, source http.Header) {
	for name, values := range source {
		if shouldStripProxyHeader(name) || strings.EqualFold(name, "Authorization") || strings.EqualFold(name, "Cookie") {
			continue
		}
		for _, value := range values {
			target.Add(name, value)
		}
	}
}

func copyKnowledgeBaseResponseHeaders(target, source http.Header) {
	for name, values := range source {
		if shouldStripProxyHeader(name) || strings.EqualFold(name, "Set-Cookie") || strings.EqualFold(name, "Location") {
			continue
		}
		for _, value := range values {
			target.Add(name, value)
		}
	}
}

func shouldStripProxyHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "connection", "proxy-connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade", "x-forwarded-for", "x-forwarded-host", "x-forwarded-proto":
		return true
	default:
		return false
	}
}

type flushResponseWriter struct {
	io.Writer
	http.Flusher
}

func (w flushResponseWriter) Write(data []byte) (int, error) {
	n, err := w.Writer.Write(data)
	w.Flusher.Flush()
	return n, err
}
