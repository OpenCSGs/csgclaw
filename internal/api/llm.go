package api

import (
	"io"
	"net/http"

	"csgclaw/internal/llm"
)

func (h *Handler) handlePicoClawModels(w http.ResponseWriter, r *http.Request, botID string) {
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

func (h *Handler) handlePicoClawChatCompletions(w http.ResponseWriter, r *http.Request, botID string) {
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
