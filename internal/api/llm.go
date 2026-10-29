package api

import (
	"crypto/rand"
	"crypto/sha256"
	"csgclaw/internal/diagnostics"
	"csgclaw/internal/modelusage"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"csgclaw/internal/llm"
)

// Multimodal turns carry inline images and accumulated tool output. Keep a
// memory bound, but reject excess bytes explicitly instead of parsing a prefix.
const maxLLMRequestBodyBytes = 128 << 20

func readLLMRequestBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	id := rand.Text()
	w.Header().Set("X-CSGClaw-Request-ID", id)
	*r = *r.WithContext(diagnostics.WithIngressID(r.Context(), id))
	observed := int64(0)
	var readFailure error
	readComplete := false
	evidence := func(status int, code string, body []byte) {
		fields := map[string]any{"request_id": id, "operation": r.URL.Path, "content_length": r.ContentLength, "observed_bytes": observed, "limit_bytes": maxLLMRequestBodyBytes, "http_status": status, "result": code, "body_read_complete": readComplete, "retained_bytes": len(body)}
		if body != nil {
			digest := sha256.Sum256(body)
			fields["body_sha256"] = hex.EncodeToString(digest[:])
			fields["body_sha256_complete"] = readComplete
		}
		if code == "invalid_json" {
			var parsed any
			if err := json.Unmarshal(body, &parsed); err != nil {
				var syntax *json.SyntaxError
				if errors.As(err, &syntax) {
					fields["parse_offset"] = syntax.Offset
				}
			}
		}
		diagnostics.Capture(r.Context(), "llm.ingress", fields)
		if status >= 400 {
			failure := readFailure
			if failure == nil {
				failure = errors.New(code)
			}
			diagnostics.ReportFailure(r.Context(), "llm.ingress", failure, fields)
		}
	}
	tooLarge := func(body []byte) {
		evidence(http.StatusRequestEntityTooLarge, "request_too_large", body)
		writeLLMError(w, &llm.HTTPError{Status: http.StatusRequestEntityTooLarge, Code: "request_too_large", Message: fmt.Sprintf("LLM request body exceeds %d MiB limit", maxLLMRequestBodyBytes>>20)})
	}
	if r.ContentLength > maxLLMRequestBodyBytes {
		tooLarge(nil)
		return nil, false
	}
	counted := &llmIngressBody{ReadCloser: r.Body}
	body, err := io.ReadAll(http.MaxBytesReader(w, counted, maxLLMRequestBodyBytes))
	observed = counted.bytes
	readFailure = err
	readComplete = err == nil
	if err != nil {
		var maximum *http.MaxBytesError
		if errors.As(err, &maximum) {
			tooLarge(body)
		} else {
			evidence(http.StatusBadRequest, "request_body_read_failed", body)
			writeLLMError(w, &llm.HTTPError{Status: http.StatusBadRequest, Code: "request_body_read_failed", Message: "read request body"})
		}
		return nil, false
	}

	evidence(0, "accepted", body)
	return body, true
}

func (h *Handler) handleAgentLLMModelsByID(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.requireAgentLLMID(w, r)
	if !ok {
		return
	}
	r, ok = h.taskLLMContext(w, r, agentID)
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
	r, ok = h.taskLLMContext(w, r, agentID)
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
	r, ok = h.taskLLMContext(w, r, agentID)
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
	r, ok = h.taskLLMContext(w, r, agentID)
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
	body, ok := readLLMRequestBody(w, r)
	if !ok || !validateLLMRequestJSON(w, r, body) {
		return
	}
	resp, callErr := h.llm.ChatCompletionsStream(r.Context(), botID, body)
	if callErr != nil {
		diagnostics.Capture(r.Context(), "llm.failed_request", map[string]any{"request_id": diagnostics.IngressID(r.Context()), "body": json.RawMessage(body), "error": callErr.Error()})
		diagnostics.ReportFailure(r.Context(), "llm.forward", callErr, map[string]any{"ingress_request_id": diagnostics.IngressID(r.Context())})
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
	body, ok := readLLMRequestBody(w, r)
	if !ok || !validateLLMRequestJSON(w, r, body) {
		return
	}
	resp, callErr := h.llm.Responses(r.Context(), botID, body)
	if callErr != nil {
		diagnostics.Capture(r.Context(), "llm.failed_request", map[string]any{"request_id": diagnostics.IngressID(r.Context()), "body": json.RawMessage(body), "error": callErr.Error()})
		diagnostics.ReportFailure(r.Context(), "llm.forward", callErr, map[string]any{"ingress_request_id": diagnostics.IngressID(r.Context())})
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
	if h.im != nil {
		r = r.WithContext(diagnostics.WithNativeLookup(r.Context(), h.im.Diagnostics(), botID, "", ""))
	}

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
// Bind diagnostics and usage completeness only with that exact identity.
// Requests without it are never assigned by Agent identity alone.
func (h *Handler) withLLMDiagnostic(r *http.Request, agentID string) *http.Request {
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
	ctx := llm.WithNativeRequestMetadata(r.Context(), meta.Thread, meta.Turn)
	if h.im != nil {
		ctx = diagnostics.WithNativeLookup(ctx, h.im.Diagnostics(), agentID, meta.Thread, meta.Turn)
	}
	if runtime := h.connectorCodexRuntime(); runtime != nil {
		ctx = modelusage.WithRequestObserver(ctx, func(usage *modelusage.Usage) {
			runtime.ObserveModelResponseUsage(agentID, meta.Thread, meta.Turn, usage)
		})
	}
	diagnostics.BindRequestLogScope(r.Context(), ctx)
	return r.WithContext(ctx)
}

type llmIngressBody struct {
	io.ReadCloser
	bytes int64
}

func (b *llmIngressBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	b.bytes += int64(n)
	return n, err
}

func validateLLMRequestJSON(w http.ResponseWriter, r *http.Request, body []byte) bool {
	if json.Valid(body) {
		return true
	}
	var parsed any
	err := json.Unmarshal(body, &parsed)
	digest := sha256.Sum256(body)
	fields := map[string]any{"request_id": diagnostics.IngressID(r.Context()), "operation": r.URL.Path, "content_length": r.ContentLength, "observed_bytes": len(body), "retained_bytes": len(body), "limit_bytes": maxLLMRequestBodyBytes, "http_status": 400, "result": "invalid_json", "body_sha256": hex.EncodeToString(digest[:]), "body_read_complete": true, "body_sha256_complete": true}
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		fields["parse_offset"] = syntax.Offset
	}
	diagnostics.Capture(r.Context(), "llm.ingress", fields)
	diagnostics.ReportFailure(r.Context(), "llm.ingress", err, fields)
	writeLLMError(w, &llm.HTTPError{Status: 400, Code: "invalid_json", Message: "decode request: invalid JSON"})
	return false
}
