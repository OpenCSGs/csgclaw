package api

import (
	"csgclaw/internal/diagnostics"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"csgclaw/internal/llm"
)

func (h *Handler) handleAgentLLMModelsByID(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.requireAgentLLMID(w, r)
	if !ok {
		return
	}
	h.handleBotLLMModels(w, r, agentID)
}

func (h *Handler) handleAgentLLMChatCompletionsByID(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.requireAgentLLMID(w, r)
	if !ok {
		return
	}
	h.handleBotLLMChatCompletions(w, r, agentID)
}

func (h *Handler) handleAgentLLMResponsesByID(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.requireAgentLLMID(w, r)
	if !ok {
		return
	}
	h.handleBotLLMResponses(w, r, agentID)
}

func (h *Handler) handleAgentLLMResponsesWebsocketByID(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.requireAgentLLMID(w, r)
	if !ok {
		return
	}
	h.handleBotLLMResponsesWebsocket(w, r, agentID)
}

func (h *Handler) requireAgentLLMID(w http.ResponseWriter, r *http.Request) (string, bool) {
	agentID := strings.TrimSpace(pathValue(r, "id"))
	if agentID == "" {
		http.NotFound(w, r)
		return "", false
	}
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) && !h.authorizesAgentToken(agentID, r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	return agentID, true
}

func (h *Handler) handleBotLLMModels(w http.ResponseWriter, r *http.Request, botID string) {
	if h.llm == nil {
		http.Error(w, "llm bridge is not configured", http.StatusServiceUnavailable)
		return
	}
	body, status, contentType, err := h.llm.Models(r.Context(), botID)
	if err != nil {
		writeLLMError(w, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (h *Handler) handleBotLLMChatCompletions(w http.ResponseWriter, r *http.Request, botID string) {
	r = h.withLLMDiagnostic(r, botID)
	if h.llm == nil {
		http.Error(w, "llm bridge is not configured", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		http.Error(w, "read request body", http.StatusBadRequest)
		return
	}
	resp, callErr := h.llm.ChatCompletionsStream(r.Context(), botID, body)
	if callErr != nil {
		writeLLMError(w, callErr)
		return
	}
	defer resp.Body.Close()
	writeLLMUpstreamResponse(w, resp)
}

func (h *Handler) handleBotLLMResponses(w http.ResponseWriter, r *http.Request, botID string) {
	r = h.withLLMDiagnostic(r, botID)
	if h.llm == nil {
		http.Error(w, "llm bridge is not configured", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		http.Error(w, "read request body", http.StatusBadRequest)
		return
	}
	resp, callErr := h.llm.Responses(r.Context(), botID, body)
	if callErr != nil {
		writeLLMError(w, callErr)
		return
	}
	defer resp.Body.Close()
	writeLLMUpstreamResponse(w, resp)
}

func writeLLMUpstreamResponse(w http.ResponseWriter, resp *llm.UpstreamResponse) {
	copyLLMHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if isLLMEventStream(resp.Header.Get("Content-Type")) {
		_ = copyAndFlushLLMStream(w, resp.Body)
		return
	}
	_, _ = io.Copy(w, resp.Body)
}

func isLLMEventStream(contentType string) bool {
	mediaType, _, _ := strings.Cut(contentType, ";")
	return strings.EqualFold(strings.TrimSpace(mediaType), "text/event-stream")
}

func copyAndFlushLLMStream(w http.ResponseWriter, src io.Reader) error {
	controller := http.NewResponseController(w)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			if flushErr := controller.Flush(); flushErr != nil && !errors.Is(flushErr, http.ErrNotSupported) {
				return flushErr
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

func (h *Handler) handleBotLLMResponsesWebsocket(w http.ResponseWriter, r *http.Request, botID string) {
	if h.llm == nil {
		http.Error(w, "llm bridge is not configured", http.StatusServiceUnavailable)
		return
	}
	if err := h.llm.ResponsesWebsocket(w, r, botID); err != nil {
		writeLLMError(w, err)
	}
}

func copyLLMHeaders(dst, src http.Header) {
	for key, values := range src {
		if strings.EqualFold(key, "connection") || strings.EqualFold(key, "content-length") {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func writeLLMError(w http.ResponseWriter, err error) {
	if httpErr, ok := err.(*llm.HTTPError); ok {
		if httpErr.Code != "" {
			writeJSON(w, httpErr.Status, map[string]any{
				"error": map[string]any{
					"code":     httpErr.Code,
					"message":  httpErr.Message,
					"provider": httpErr.Provider,
				},
			})
			return
		}
		http.Error(w, httpErr.Message, httpErr.Status)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// Codex includes stable session and turn IDs on each HTTP model request.
// Requests without that contract (including current DSH and WebSockets) remain
// explicitly unmeasured rather than being assigned by Agent identity alone.
func (h *Handler) withLLMDiagnostic(r *http.Request, agentID string) *http.Request {
	if h.im == nil {
		return r
	}
	raw := r.Header.Get("X-Codex-Turn-Metadata")
	if len(raw) > 8192 || raw == "" {
		return r
	}
	var meta struct {
		Thread  string `json:"thread_id"`
		Session string `json:"session_id"`
		Turn    string `json:"turn_id"`
	}
	if json.Unmarshal([]byte(raw), &meta) != nil {
		return r
	}
	if meta.Thread == "" {
		meta.Thread = meta.Session
	}
	if meta.Thread == "" || meta.Turn == "" || len(meta.Thread) > 256 || len(meta.Turn) > 256 {
		return r
	}
	return r.WithContext(diagnostics.WithNativeLookup(r.Context(), h.im.Diagnostics(), agentID, meta.Thread, meta.Turn))
}
