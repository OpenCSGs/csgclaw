package api

import (
	"io"
	"net/http"
	"strings"
)

var openCSGMCPGatewayHTTPClient = &http.Client{
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// handleOpenCSGMCPGatewayProxy keeps the user's OpenCSG credential out of
// templates and agent runtimes. Only the configured, authenticated AIGateway
// endpoint receives that credential.
func (h *Handler) handleOpenCSGMCPGatewayProxy(w http.ResponseWriter, r *http.Request) {
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	connection, err := loadKnowledgeBaseConnection(r.Context())
	if err != nil {
		writeKnowledgeBaseError(w, err)
		return
	}
	baseURL := strings.TrimRight(strings.TrimSpace(connection.AIGatewayBaseURL), "/")
	if baseURL == "" {
		http.Error(w, "AIGateway base URL is not configured", http.StatusBadGateway)
		return
	}
	upstream, err := http.NewRequestWithContext(r.Context(), r.Method, baseURL+"/gateway/mcp", r.Body)
	if err != nil {
		http.Error(w, "create OpenCSG MCP Gateway request", http.StatusBadGateway)
		return
	}
	copyKnowledgeBaseRequestHeaders(upstream.Header, r.Header)
	upstream.Header.Set("Authorization", "Bearer "+strings.TrimSpace(connection.CSGHubAccessToken))
	response, err := openCSGMCPGatewayHTTPClient.Do(upstream)
	if err != nil {
		http.Error(w, "OpenCSG MCP Gateway is temporarily unavailable", http.StatusBadGateway)
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
