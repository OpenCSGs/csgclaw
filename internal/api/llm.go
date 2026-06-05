package api

import (
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
	if !h.validateServerAccessToken(r.Header.Get("Authorization")) {
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
	if h.llm == nil {
		http.Error(w, "llm bridge is not configured", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		http.Error(w, "read request body", http.StatusBadRequest)
		return
	}
	respBody, status, contentType, callErr := h.llm.ChatCompletions(r.Context(), botID, body)
	if callErr != nil {
		writeLLMError(w, callErr)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(respBody)
}

func (h *Handler) handleBotLLMResponses(w http.ResponseWriter, r *http.Request, botID string) {
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
	copyLLMHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
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
