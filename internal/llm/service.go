package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/auth"
	"csgclaw/internal/cliproxy"
	"csgclaw/internal/codexmodel"
	"csgclaw/internal/config"

	"github.com/gorilla/websocket"
)

type Service struct {
	defaults              config.ModelConfig
	agents                *agent.Service
	client                *http.Client
	responsesCapabilityMu sync.Mutex
	responsesUnsupported  map[string]struct{}
}

type HTTPError struct {
	Status   int
	Message  string
	Code     string
	Provider string
}

type UpstreamResponse struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

var embeddedCLIProxyProviderBaseURL = func(ctx context.Context, provider string) (string, error) {
	return cliproxy.Default().ProviderBaseURL(ctx, provider)
}

var embeddedCLIProxyBaseURL = func(ctx context.Context) (string, error) {
	return cliproxy.Default().BaseURL(ctx)
}

var embeddedCLIProxyAuthStatus = func(ctx context.Context, provider string) (cliproxy.AuthStatus, error) {
	return cliproxy.Default().AuthStatus(ctx, provider)
}

var responsesWebsocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

var responsesWebsocketDialer = websocket.Dialer{
	Proxy: http.ProxyFromEnvironment,
}

func (e *HTTPError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func NewService(defaults config.ModelConfig, agents *agent.Service) *Service {
	return &Service{
		defaults:             defaults.Resolved(),
		agents:               agents,
		client:               &http.Client{},
		responsesUnsupported: make(map[string]struct{}),
	}
}

func (s *Service) Models(_ context.Context, botID string) ([]byte, int, string, error) {
	profile, err := s.resolveProfile(botID)
	if err != nil {
		return nil, 0, "", err
	}
	modelID := strings.TrimSpace(profile.ModelID)
	payload := map[string]any{
		"object": "list",
		"data": []map[string]any{
			{
				"id":       modelID,
				"object":   "model",
				"created":  0,
				"owned_by": "csgclaw",
			},
		},
		"models": []map[string]any{codexmodel.Metadata(codexmodel.Profile{
			ModelID:         profile.ModelID,
			ReasoningEffort: profile.ReasoningEffort,
		})},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, "", &HTTPError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("encode models response: %v", err)}
	}
	return body, http.StatusOK, "application/json", nil
}

func (s *Service) ChatCompletions(ctx context.Context, botID string, body []byte) ([]byte, int, string, error) {
	profile, err := s.resolveProfile(botID)
	if err != nil {
		return nil, 0, "", err
	}
	return s.forwardRemoteChat(ctx, profile, body)
}

func (s *Service) Responses(ctx context.Context, botID string, body []byte) (*UpstreamResponse, error) {
	profile, err := s.resolveProfile(botID)
	if err != nil {
		return nil, err
	}
	return s.forwardRemoteResponses(ctx, profile, body)
}

func (s *Service) ResponsesWebsocket(w http.ResponseWriter, r *http.Request, botID string) error {
	if w == nil || r == nil {
		return &HTTPError{Status: http.StatusBadRequest, Message: "websocket request is required"}
	}
	if !websocket.IsWebSocketUpgrade(r) {
		return &HTTPError{Status: http.StatusBadRequest, Message: "responses websocket upgrade is required"}
	}
	profile, err := s.resolveProfile(botID)
	if err != nil {
		return err
	}
	baseURL, apiKey, err := s.agentProfileWebsocketTarget(r.Context(), profile)
	if err != nil {
		return err
	}
	if baseURL == "" {
		return &HTTPError{Status: http.StatusBadRequest, Message: "profile base_url is required"}
	}
	upstreamURL, err := responsesWebsocketURL(baseURL)
	if err != nil {
		return &HTTPError{Status: http.StatusBadRequest, Message: err.Error()}
	}
	downstream, err := responsesWebsocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return &HTTPError{Status: http.StatusBadRequest, Message: fmt.Sprintf("upgrade responses websocket: %v", err)}
	}
	defer downstream.Close()

	upstream, _, err := responsesWebsocketDialer.DialContext(r.Context(), upstreamURL, websocketUpstreamHeaders(r.Header, apiKey, profile.Headers))
	if err != nil {
		writeResponsesWebsocketClose(downstream, websocket.CloseTryAgainLater, fmt.Sprintf("dial upstream responses websocket: %v", err))
		return nil
	}
	defer upstream.Close()

	return proxyResponsesWebsocket(r.Context(), downstream, upstream, responsesWebsocketClientPayloadRewriter(profile))
}

func (s *Service) resolveProfile(botID string) (agent.AgentProfile, error) {
	if s.agents == nil {
		return agent.AgentProfile{}, &HTTPError{Status: http.StatusServiceUnavailable, Message: "agent service is not configured"}
	}
	profile, err := s.agents.ResolvedAgentProfile(botID)
	if err != nil {
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "profile is incomplete") {
			status = http.StatusConflict
		}
		return agent.AgentProfile{}, &HTTPError{Status: status, Message: err.Error()}
	}
	return profile, nil
}

func (s *Service) forwardRemoteChat(ctx context.Context, profile agent.AgentProfile, body []byte) ([]byte, int, string, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, 0, "", &HTTPError{Status: http.StatusBadRequest, Message: fmt.Sprintf("decode request: %v", err)}
	}
	mergeProfilePayload(payload, profile)
	normalizeCompletionTokenLimits(payload)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, "", &HTTPError{Status: http.StatusBadRequest, Message: fmt.Sprintf("encode request: %v", err)}
	}

	baseURL, apiKey, err := s.agentProfileTarget(ctx, profile)
	if err != nil {
		return nil, 0, "", err
	}
	if baseURL == "" {
		return nil, 0, "", &HTTPError{Status: http.StatusBadRequest, Message: "profile base_url is required"}
	}
	upstreamURL := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(encoded))
	if err != nil {
		return nil, 0, "", &HTTPError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("build upstream request: %v", err)}
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range profile.Headers {
		key = strings.TrimSpace(key)
		if key == "" || strings.EqualFold(key, "authorization") || strings.EqualFold(key, "content-type") {
			continue
		}
		req.Header.Set(key, value)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, "", &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("send upstream request: %v", err)}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, 0, "", &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("read upstream response: %v", err)}
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		respBody, contentType = compactUpstreamErrorResponse(resp.Header, respBody, contentType)
	}
	return respBody, resp.StatusCode, contentType, nil
}

func (s *Service) forwardRemoteResponses(ctx context.Context, profile agent.AgentProfile, body []byte) (*UpstreamResponse, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, &HTTPError{Status: http.StatusBadRequest, Message: fmt.Sprintf("decode request: %v", err)}
	}
	mergeResponsesPayload(payload, profile)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, &HTTPError{Status: http.StatusBadRequest, Message: fmt.Sprintf("encode request: %v", err)}
	}

	baseURL, apiKey, err := s.agentProfileTarget(ctx, profile)
	if err != nil {
		return nil, err
	}
	if baseURL == "" {
		return nil, &HTTPError{Status: http.StatusBadRequest, Message: "profile base_url is required"}
	}
	if s.responsesAPIUnsupportedCached(profile, baseURL) {
		return s.forwardResponsesViaChat(ctx, profile, payload, baseURL, apiKey)
	}
	upstreamURL := strings.TrimRight(baseURL, "/") + "/responses"
	req, err := newProfileJSONRequest(ctx, upstreamURL, encoded, apiKey, profile.Headers)
	if err != nil {
		return nil, &HTTPError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("build upstream request: %v", err)}
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("send upstream request: %v", err)}
	}
	if responsesAPIUnsupportedStatus(resp.StatusCode) {
		_ = resp.Body.Close()
		s.markResponsesAPIUnsupported(profile, baseURL)
		return s.forwardResponsesViaChat(ctx, profile, payload, baseURL, apiKey)
	}
	if responsesAPITransientFallbackStatus(profile, resp.StatusCode) && !responsesPayloadHasToolSemantics(payload) {
		_ = resp.Body.Close()
		return s.forwardResponsesViaChat(ctx, profile, payload, baseURL, apiKey)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return compactUpstreamErrorStream(resp)
	}
	return &UpstreamResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       resp.Body,
	}, nil
}

func (s *Service) forwardResponsesViaChat(ctx context.Context, profile agent.AgentProfile, responsesPayload map[string]any, baseURL, apiKey string) (*UpstreamResponse, error) {
	chatPayload, err := responsesPayloadToChatPayload(responsesPayload)
	if err != nil {
		return nil, &HTTPError{Status: http.StatusBadRequest, Message: err.Error()}
	}
	mergeProfilePayload(chatPayload, profile)
	normalizeCompletionTokenLimits(chatPayload)
	encoded, err := json.Marshal(chatPayload)
	if err != nil {
		return nil, &HTTPError{Status: http.StatusBadRequest, Message: fmt.Sprintf("encode chat fallback request: %v", err)}
	}

	upstreamURL := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := newProfileJSONRequest(ctx, upstreamURL, encoded, apiKey, profile.Headers)
	if err != nil {
		return nil, &HTTPError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("build chat fallback request: %v", err)}
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("send chat fallback request: %v", err)}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return compactUpstreamErrorStream(resp)
	}
	if payloadBool(chatPayload["stream"]) {
		return streamChatCompletionAsResponse(resp, strings.TrimSpace(profile.ModelID)), nil
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("read chat fallback response: %v", err)}
	}
	converted, err := chatCompletionResponseToResponses(respBody, strings.TrimSpace(profile.ModelID))
	if err != nil {
		return nil, &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("convert chat fallback response: %v", err)}
	}
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	return &UpstreamResponse{
		StatusCode: http.StatusOK,
		Header:     header,
		Body:       io.NopCloser(bytes.NewReader(converted)),
	}, nil
}

func newProfileJSONRequest(ctx context.Context, url string, body []byte, apiKey string, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" || strings.EqualFold(key, "authorization") || strings.EqualFold(key, "content-type") {
			continue
		}
		req.Header.Set(key, value)
	}
	return req, nil
}

func compactUpstreamErrorStream(resp *http.Response) (*UpstreamResponse, error) {
	if resp == nil {
		return nil, &HTTPError{Status: http.StatusBadGateway, Message: "upstream response is nil"}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("read upstream error response: %v", err)}
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	body, contentType = compactUpstreamErrorResponse(resp.Header, body, contentType)
	header := resp.Header.Clone()
	header.Del("Content-Length")
	header.Set("Content-Type", contentType)
	return &UpstreamResponse{
		StatusCode: resp.StatusCode,
		Header:     header,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

func compactUpstreamErrorResponse(header http.Header, body []byte, contentType string) ([]byte, string) {
	if msg := extractUpstreamErrorMessage(body); msg != "" {
		return []byte(msg), "text/plain; charset=utf-8"
	}
	if strings.TrimSpace(contentType) == "" {
		if ct := strings.TrimSpace(header.Get("Content-Type")); ct != "" {
			contentType = ct
		} else {
			contentType = "text/plain; charset=utf-8"
		}
	}
	return body, contentType
}

func extractUpstreamErrorMessage(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	value, ok := payload["error"]
	if !ok {
		value = payload
	}
	errObj, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	message := strings.TrimSpace(stringValue(errObj["message"]))
	if message == "" {
		return ""
	}
	parts := make([]string, 0, 2)
	if typ := strings.TrimSpace(stringValue(errObj["type"])); typ != "" {
		parts = append(parts, "type="+typ)
	}
	if code := strings.TrimSpace(stringValue(errObj["code"])); code != "" {
		parts = append(parts, "code="+code)
	}
	if len(parts) == 0 {
		return message
	}
	return message + " (" + strings.Join(parts, ", ") + ")"
}

func responsesAPIUnsupportedStatus(status int) bool {
	return status == http.StatusNotFound || status == http.StatusMethodNotAllowed
}

func responsesAPITransientFallbackStatus(profile agent.AgentProfile, status int) bool {
	if status < http.StatusInternalServerError || status > 599 {
		return false
	}
	switch strings.TrimSpace(profile.Provider) {
	case agent.ProviderClaudeCode:
		return true
	default:
		return false
	}
}

func (s *Service) responsesAPIUnsupportedCached(profile agent.AgentProfile, baseURL string) bool {
	key := responsesCapabilityKey(profile, baseURL)
	s.responsesCapabilityMu.Lock()
	defer s.responsesCapabilityMu.Unlock()
	_, ok := s.responsesUnsupported[key]
	return ok
}

func (s *Service) markResponsesAPIUnsupported(profile agent.AgentProfile, baseURL string) {
	key := responsesCapabilityKey(profile, baseURL)
	s.responsesCapabilityMu.Lock()
	defer s.responsesCapabilityMu.Unlock()
	if s.responsesUnsupported == nil {
		s.responsesUnsupported = make(map[string]struct{})
	}
	s.responsesUnsupported[key] = struct{}{}
}

func responsesCapabilityKey(profile agent.AgentProfile, baseURL string) string {
	return strings.TrimSpace(profile.Provider) + "\x00" + strings.TrimRight(strings.TrimSpace(baseURL), "/") + "\x00" + strings.TrimSpace(profile.ModelID)
}

func responsesPayloadToChatPayload(payload map[string]any) (map[string]any, error) {
	if responsesPayloadHasToolSemantics(payload) {
		return nil, fmt.Errorf("active tool-use Responses requests are not supported by chat-completions fallback")
	}

	messages := make([]map[string]any, 0, 4)
	if instructions := strings.TrimSpace(stringValue(payload["instructions"])); instructions != "" {
		messages = append(messages, map[string]any{"role": "system", "content": instructions})
	}
	switch input := payload["input"].(type) {
	case string:
		if strings.TrimSpace(input) != "" {
			messages = append(messages, map[string]any{"role": "user", "content": input})
		}
	case []any:
		for _, item := range input {
			message, ok := responseInputItemToChatMessage(item)
			if ok {
				messages = append(messages, message)
			}
		}
	case nil:
	default:
		text := strings.TrimSpace(stringValue(input))
		if text != "" {
			messages = append(messages, map[string]any{"role": "user", "content": text})
		}
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("responses input is required for chat fallback")
	}

	chatPayload := map[string]any{"messages": messages}
	copyResponsesOptionToChat(payload, chatPayload, "stream", "stream")
	copyResponsesOptionToChat(payload, chatPayload, "temperature", "temperature")
	copyResponsesOptionToChat(payload, chatPayload, "top_p", "top_p")
	copyResponsesOptionToChat(payload, chatPayload, "presence_penalty", "presence_penalty")
	copyResponsesOptionToChat(payload, chatPayload, "frequency_penalty", "frequency_penalty")
	copyResponsesOptionToChat(payload, chatPayload, "stop", "stop")
	copyResponsesOptionToChat(payload, chatPayload, "seed", "seed")
	copyResponsesOptionToChat(payload, chatPayload, "response_format", "response_format")
	copyResponsesOptionToChat(payload, chatPayload, "user", "user")
	copyResponsesOptionToChat(payload, chatPayload, "max_output_tokens", "max_tokens")
	return chatPayload, nil
}

func responsesPayloadHasToolSemantics(payload map[string]any) bool {
	if toolChoiceRequestsTools(payload["tool_choice"]) {
		return true
	}
	return responseValueHasToolSemantics(payload["input"])
}

func toolChoiceRequestsTools(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		text := strings.ToLower(strings.TrimSpace(v))
		return text != "" && text != "none" && text != "auto"
	case map[string]any:
		if len(v) == 0 {
			return false
		}
		if len(v) == 1 && strings.EqualFold(strings.TrimSpace(stringValue(v["type"])), "none") {
			return false
		}
		return true
	default:
		return payloadValuePresent(v)
	}
}

func responseValueHasToolSemantics(value any) bool {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if responseValueHasToolSemantics(item) {
				return true
			}
		}
	case []map[string]any:
		for _, item := range v {
			if responseValueHasToolSemantics(item) {
				return true
			}
		}
	case map[string]any:
		itemType := strings.ToLower(strings.TrimSpace(stringValue(v["type"])))
		switch {
		case strings.Contains(itemType, "tool"),
			itemType == "function_call",
			itemType == "function_call_output",
			strings.HasSuffix(itemType, "_call"),
			strings.HasSuffix(itemType, "_call_output"):
			return true
		}
		if strings.EqualFold(strings.TrimSpace(stringValue(v["role"])), "tool") {
			return true
		}
		for _, key := range []string{"tool_calls", "function_call", "tool_call_id"} {
			if payloadValuePresent(v[key]) {
				return true
			}
		}
		if payloadValuePresent(v["call_id"]) && payloadValuePresent(v["output"]) {
			return true
		}
		for _, item := range v {
			if responseValueHasToolSemantics(item) {
				return true
			}
		}
	}
	return false
}

func payloadValuePresent(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case bool:
		return v
	case []any:
		return len(v) > 0
	case []map[string]any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	default:
		return true
	}
}

func responseInputItemToChatMessage(item any) (map[string]any, bool) {
	switch value := item.(type) {
	case string:
		if strings.TrimSpace(value) == "" {
			return nil, false
		}
		return map[string]any{"role": "user", "content": value}, true
	case map[string]any:
		role := normalizeChatFallbackRole(stringValue(value["role"]))
		if role == "" {
			role = "user"
		}
		content := responseContentToText(value["content"])
		if strings.TrimSpace(content) == "" {
			content = responseContentToText(value["text"])
		}
		if strings.TrimSpace(content) == "" {
			return nil, false
		}
		return map[string]any{"role": role, "content": content}, true
	default:
		text := responseContentToText(value)
		if strings.TrimSpace(text) == "" {
			return nil, false
		}
		return map[string]any{"role": "user", "content": text}, true
	}
}

func normalizeChatFallbackRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "developer", "latest_reminder", "system":
		return "system"
	case "user", "assistant", "tool":
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return "user"
	}
}

func responseContentToText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		var b strings.Builder
		for _, part := range value {
			text := responseContentToText(part)
			if text != "" {
				b.WriteString(text)
			}
		}
		return b.String()
	case map[string]any:
		if text := stringValue(value["text"]); text != "" {
			return text
		}
		if text := stringValue(value["content"]); text != "" {
			return text
		}
		return ""
	default:
		return stringValue(value)
	}
}

func copyResponsesOptionToChat(src, dst map[string]any, srcKey, dstKey string) {
	value, ok := src[srcKey]
	if !ok || value == nil {
		return
	}
	if _, exists := dst[dstKey]; exists {
		return
	}
	dst[dstKey] = value
}

func chatCompletionResponseToResponses(body []byte, fallbackModel string) ([]byte, error) {
	var chat struct {
		ID      string         `json:"id"`
		Created int64          `json:"created"`
		Model   string         `json:"model"`
		Choices []chatChoice   `json:"choices"`
		Usage   map[string]any `json:"usage,omitempty"`
	}
	if err := json.Unmarshal(body, &chat); err != nil {
		return nil, err
	}
	text := ""
	if len(chat.Choices) > 0 {
		text = responseContentToText(chat.Choices[0].Message["content"])
	}
	model := strings.TrimSpace(chat.Model)
	if model == "" {
		model = fallbackModel
	}
	response := completedResponsesPayload(responseID(chat.ID), model, chat.Created, text, chat.Usage)
	return json.Marshal(response)
}

type chatChoice struct {
	Message      map[string]any `json:"message"`
	Delta        map[string]any `json:"delta"`
	FinishReason any            `json:"finish_reason"`
}

func completedResponsesPayload(id, model string, created int64, text string, usage map[string]any) map[string]any {
	if id == "" {
		id = responseID("")
	}
	if created == 0 {
		created = time.Now().Unix()
	}
	messageID := id + "_msg"
	item := map[string]any{
		"id":     messageID,
		"type":   "message",
		"status": "completed",
		"role":   "assistant",
		"content": []map[string]any{
			{
				"type":        "output_text",
				"text":        text,
				"annotations": []any{},
			},
		},
	}
	response := map[string]any{
		"id":          id,
		"object":      "response",
		"created_at":  created,
		"status":      "completed",
		"model":       model,
		"output":      []map[string]any{item},
		"output_text": text,
	}
	if len(usage) > 0 {
		response["usage"] = usage
	}
	return response
}

func streamChatCompletionAsResponse(resp *http.Response, fallbackModel string) *UpstreamResponse {
	reader, writer := io.Pipe()
	go func() {
		defer resp.Body.Close()
		err := writeChatCompletionStreamAsResponse(writer, resp.Body, fallbackModel)
		_ = writer.CloseWithError(err)
	}()
	header := make(http.Header)
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	return &UpstreamResponse{
		StatusCode: http.StatusOK,
		Header:     header,
		Body:       reader,
	}
}

func writeChatCompletionStreamAsResponse(w io.Writer, r io.Reader, fallbackModel string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	responseIDValue := responseID("")
	messageID := responseIDValue + "_msg"
	model := fallbackModel
	created := time.Now().Unix()
	var text strings.Builder
	started := false
	completed := false

	ensureStarted := func() error {
		if started {
			return nil
		}
		started = true
		if err := writeSSE(w, "response.created", map[string]any{
			"type":     "response.created",
			"response": inProgressResponsePayload(responseIDValue, model, created),
		}); err != nil {
			return err
		}
		if err := writeSSE(w, "response.output_item.added", map[string]any{
			"type":         "response.output_item.added",
			"output_index": 0,
			"item": map[string]any{
				"id":      messageID,
				"type":    "message",
				"status":  "in_progress",
				"role":    "assistant",
				"content": []any{},
			},
		}); err != nil {
			return err
		}
		return writeSSE(w, "response.content_part.added", map[string]any{
			"type":          "response.content_part.added",
			"item_id":       messageID,
			"output_index":  0,
			"content_index": 0,
			"part": map[string]any{
				"type":        "output_text",
				"text":        "",
				"annotations": []any{},
			},
		})
	}
	complete := func() error {
		if completed {
			return nil
		}
		completed = true
		fullText := text.String()
		if err := ensureStarted(); err != nil {
			return err
		}
		if err := writeSSE(w, "response.output_text.done", map[string]any{
			"type":          "response.output_text.done",
			"item_id":       messageID,
			"output_index":  0,
			"content_index": 0,
			"text":          fullText,
		}); err != nil {
			return err
		}
		item := completedResponseItem(messageID, fullText)
		if err := writeSSE(w, "response.content_part.done", map[string]any{
			"type":          "response.content_part.done",
			"item_id":       messageID,
			"output_index":  0,
			"content_index": 0,
			"part":          item["content"].([]map[string]any)[0],
		}); err != nil {
			return err
		}
		if err := writeSSE(w, "response.output_item.done", map[string]any{
			"type":         "response.output_item.done",
			"output_index": 0,
			"item":         item,
		}); err != nil {
			return err
		}
		return writeSSE(w, "response.completed", map[string]any{
			"type":     "response.completed",
			"response": completedResponsesPayload(responseIDValue, model, created, fullText, nil),
		})
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			if err := complete(); err != nil {
				return err
			}
			continue
		}
		var chunk struct {
			ID      string       `json:"id"`
			Created int64        `json:"created"`
			Model   string       `json:"model"`
			Choices []chatChoice `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return err
		}
		if chunk.ID != "" && strings.HasPrefix(responseIDValue, "resp_csgclaw_") {
			responseIDValue = responseID(chunk.ID)
			messageID = responseIDValue + "_msg"
		}
		if chunk.Created != 0 {
			created = chunk.Created
		}
		if strings.TrimSpace(chunk.Model) != "" {
			model = strings.TrimSpace(chunk.Model)
		}
		for _, choice := range chunk.Choices {
			delta := responseContentToText(choice.Delta["content"])
			if delta != "" {
				if err := ensureStarted(); err != nil {
					return err
				}
				text.WriteString(delta)
				if err := writeSSE(w, "response.output_text.delta", map[string]any{
					"type":          "response.output_text.delta",
					"item_id":       messageID,
					"output_index":  0,
					"content_index": 0,
					"delta":         delta,
				}); err != nil {
					return err
				}
			}
			if choice.FinishReason != nil {
				if err := complete(); err != nil {
					return err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if !completed {
		return complete()
	}
	return nil
}

func inProgressResponsePayload(id, model string, created int64) map[string]any {
	if created == 0 {
		created = time.Now().Unix()
	}
	return map[string]any{
		"id":         id,
		"object":     "response",
		"created_at": created,
		"status":     "in_progress",
		"model":      model,
		"output":     []any{},
	}
}

func completedResponseItem(messageID, text string) map[string]any {
	return map[string]any{
		"id":     messageID,
		"type":   "message",
		"status": "completed",
		"role":   "assistant",
		"content": []map[string]any{
			{
				"type":        "output_text",
				"text":        text,
				"annotations": []any{},
			},
		},
	}
}

func writeSSE(w io.Writer, event string, payload map[string]any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	if flusher, ok := w.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	return nil
}

func responseID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Sprintf("resp_csgclaw_%d", time.Now().UnixNano())
	}
	if strings.HasPrefix(id, "resp_") {
		return id
	}
	return "resp_" + id
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func payloadBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func applyReasoningEffortDefault(payload map[string]any, defaultEffort string) {
	defaultEffort = strings.ToLower(strings.TrimSpace(defaultEffort))
	if defaultEffort == "" {
		return
	}
	if value, ok := payload["reasoning_effort"]; ok {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return
		}
		if value != nil {
			return
		}
	}
	payload["reasoning_effort"] = defaultEffort
}

func mergeProfilePayload(payload map[string]any, profile agent.AgentProfile) {
	payload["model"] = strings.TrimSpace(profile.ModelID)
	applyReasoningEffortDefault(payload, profile.ReasoningEffort)
	for key, value := range profile.RequestOptions {
		key = strings.TrimSpace(key)
		if key == "" || strings.EqualFold(key, "model") {
			continue
		}
		if _, exists := payload[key]; exists {
			continue
		}
		payload[key] = value
	}
	applyProviderPayloadConstraints(payload, profile)
	if !profile.EnableFastMode {
		return
	}
	switch profile.Provider {
	case agent.ProviderClaudeCode:
		if _, exists := payload["speed"]; !exists {
			payload["speed"] = "fast"
		}
	default:
		if _, exists := payload["service_tier"]; !exists {
			payload["service_tier"] = "priority"
		}
	}
}

func mergeResponsesPayload(payload map[string]any, profile agent.AgentProfile) {
	payload["model"] = strings.TrimSpace(profile.ModelID)
	for key, value := range profile.RequestOptions {
		key = strings.TrimSpace(key)
		if key == "" || strings.EqualFold(key, "model") || strings.EqualFold(key, "reasoning_effort") {
			continue
		}
		if _, exists := payload[key]; exists {
			continue
		}
		payload[key] = value
	}
	applyProviderPayloadConstraints(payload, profile)
	if !profile.EnableFastMode {
		return
	}
	if _, exists := payload["service_tier"]; !exists {
		payload["service_tier"] = "priority"
	}
}

func applyProviderPayloadConstraints(payload map[string]any, profile agent.AgentProfile) {
	switch profile.Provider {
	case agent.ProviderCodex:
		payload["store"] = false
	}
}

const (
	gatewayReasoningThinkingBudget = 32768
	reasoningCompletionHeadroom    = 1024
)

func normalizeCompletionTokenLimits(payload map[string]any) {
	if payload == nil {
		return
	}
	thinkingBudget := payloadInt(payload, "thinking_budget")
	maxCompletion := payloadInt(payload, "max_completion_tokens")
	maxTokens := payloadInt(payload, "max_tokens")
	effectiveMax := maxCompletion
	if effectiveMax <= 0 {
		effectiveMax = maxTokens
	}
	if effectiveMax <= 0 {
		return
	}

	thinkingFloor := thinkingBudget
	if thinkingFloor <= 0 {
		thinkingFloor = reasoningModelThinkingBudgetFloor(stringValue(payload["model"]))
	}
	if thinkingFloor <= 0 && (effectiveMax == gatewayReasoningThinkingBudget || effectiveMax == 16384) {
		// Picoclaw defaults can collide with gateway-side thinking budgets even when the
		// client omits thinking_budget from the forwarded payload.
		thinkingFloor = gatewayReasoningThinkingBudget
	}
	if thinkingFloor <= 0 || effectiveMax > thinkingFloor {
		return
	}
	newMax := thinkingFloor + reasoningCompletionHeadroom
	setPayloadInt(payload, "max_completion_tokens", newMax)
	setPayloadInt(payload, "max_tokens", newMax)
}

func reasoningModelThinkingBudgetFloor(model string) int {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return 0
	}
	switch {
	case strings.Contains(model, "glm-5"),
		strings.Contains(model, "glm-4.5"),
		strings.Contains(model, "glm-4.6"),
		strings.Contains(model, "glm-4.7"):
		return gatewayReasoningThinkingBudget
	case strings.Contains(model, "qwen3"),
		strings.Contains(model, "qwen-max"),
		strings.Contains(model, "qwen-plus"):
		return gatewayReasoningThinkingBudget
	default:
		return 0
	}
}

func payloadInt(payload map[string]any, key string) int {
	value, ok := payload[key]
	if !ok || value == nil {
		return 0
	}
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0
		}
		return int(n)
	default:
		return 0
	}
}

func setPayloadInt(payload map[string]any, key string, value int) {
	if value <= 0 {
		return
	}
	payload[key] = value
}

func (s *Service) agentProfileWebsocketTarget(ctx context.Context, profile agent.AgentProfile) (string, string, error) {
	switch profile.Provider {
	case agent.ProviderCodex:
		if err := ensureCLIProxyAuthenticated(ctx, profile.Provider); err != nil {
			return "", "", err
		}
		if _, err := embeddedCLIProxyProviderBaseURL(ctx, profile.Provider); err != nil {
			return "", "", &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("embedded cliproxy unavailable: %v", err)}
		}
		baseURL, err := embeddedCLIProxyBaseURL(ctx)
		if err != nil {
			return "", "", &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("embedded cliproxy unavailable: %v", err)}
		}
		return strings.TrimRight(baseURL, "/") + "/v1", cliproxy.LocalAPIKey, nil
	case agent.ProviderClaudeCode:
		return s.agentProfileTarget(ctx, profile)
	case agent.ProviderCSGHub:
		return csghubAIGatewayTarget(ctx, s.client)
	default:
		return agent.ProfileBaseURL(profile), agent.ProfileAPIKey(profile), nil
	}
}

func (s *Service) agentProfileTarget(ctx context.Context, profile agent.AgentProfile) (string, string, error) {
	switch profile.Provider {
	case agent.ProviderCodex, agent.ProviderClaudeCode:
		if err := ensureCLIProxyAuthenticated(ctx, profile.Provider); err != nil {
			return "", "", err
		}
		baseURL, err := embeddedCLIProxyProviderBaseURL(ctx, profile.Provider)
		if err != nil {
			return "", "", &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("embedded cliproxy unavailable: %v", err)}
		}
		return baseURL, cliproxy.LocalAPIKey, nil
	case agent.ProviderCSGHub:
		return csghubAIGatewayTarget(ctx, s.client)
	default:
		return agent.ProfileBaseURL(profile), agent.ProfileAPIKey(profile), nil
	}
}

func csghubAIGatewayTarget(ctx context.Context, client *http.Client) (string, string, error) {
	store, err := auth.DefaultStore()
	if err != nil {
		return "", "", &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("csghub auth unavailable: %v", err)}
	}
	baseURL, apiKey, ok, err := store.EnsureAIGatewayCredentials(ctx, client)
	if err != nil {
		return "", "", &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("csghub auth unavailable: %v", err)}
	}
	if !ok {
		return "", "", &HTTPError{Status: http.StatusUnauthorized, Message: "csghub login is required"}
	}
	return baseURL, apiKey, nil
}

func ensureCLIProxyAuthenticated(ctx context.Context, provider string) error {
	status, err := embeddedCLIProxyAuthStatus(ctx, provider)
	if err != nil {
		return &HTTPError{Status: http.StatusBadGateway, Message: fmt.Sprintf("embedded cliproxy auth unavailable: %v", err)}
	}
	if status.Authenticated {
		return nil
	}
	message := strings.TrimSpace(status.Message)
	if message == "" {
		message = fmt.Sprintf("%s auth is required. Connect this provider in the CSGClaw UI.", provider)
	}
	return &HTTPError{
		Status:   http.StatusConflict,
		Code:     "auth_required",
		Provider: provider,
		Message:  message,
	}
}

func responsesWebsocketURL(baseURL string) (string, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/responses")
	if err != nil {
		return "", fmt.Errorf("parse upstream responses websocket url: %w", err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("upstream responses websocket url must use http, https, ws, or wss")
	}
	if u.Host == "" {
		return "", fmt.Errorf("upstream responses websocket url host is required")
	}
	return u.String(), nil
}

func websocketUpstreamHeaders(in http.Header, apiKey string, profileHeaders map[string]string) http.Header {
	out := make(http.Header)
	for key, values := range in {
		if skipWebsocketForwardHeader(key) {
			continue
		}
		for _, value := range values {
			out.Add(key, value)
		}
	}
	if apiKey != "" {
		out.Set("Authorization", "Bearer "+apiKey)
	}
	for key, value := range profileHeaders {
		key = strings.TrimSpace(key)
		if key == "" || skipWebsocketForwardHeader(key) || strings.EqualFold(key, "authorization") {
			continue
		}
		out.Set(key, value)
	}
	return out
}

func skipWebsocketForwardHeader(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "host", "connection", "upgrade", "sec-websocket-key", "sec-websocket-version", "sec-websocket-extensions", "sec-websocket-protocol", "content-length":
		return true
	default:
		return false
	}
}

type responsesWebsocketPayloadRewriter func(int, []byte) []byte

func responsesWebsocketClientPayloadRewriter(profile agent.AgentProfile) responsesWebsocketPayloadRewriter {
	return func(messageType int, payload []byte) []byte {
		if messageType != websocket.TextMessage {
			return payload
		}
		return rewriteResponsesWebsocketClientPayload(payload, profile)
	}
}

func rewriteResponsesWebsocketClientPayload(payload []byte, profile agent.AgentProfile) []byte {
	var message map[string]any
	if err := json.Unmarshal(payload, &message); err != nil {
		return payload
	}
	messageType := strings.TrimSpace(stringValue(message["type"]))
	if messageType != "" && !strings.EqualFold(messageType, "response.create") {
		return payload
	}

	target := message
	if response, ok := message["response"].(map[string]any); ok {
		target = response
	}
	mergeResponsesPayload(target, profile)

	rewritten, err := json.Marshal(message)
	if err != nil {
		return payload
	}
	return rewritten
}

func proxyResponsesWebsocket(ctx context.Context, downstream, upstream *websocket.Conn, rewriteClientPayload responsesWebsocketPayloadRewriter) error {
	errCh := make(chan error, 2)
	pump := func(dst, src *websocket.Conn, rewrite responsesWebsocketPayloadRewriter) {
		for {
			messageType, payload, err := src.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			if rewrite != nil {
				payload = rewrite(messageType, payload)
			}
			if err := dst.WriteMessage(messageType, payload); err != nil {
				errCh <- err
				return
			}
		}
	}

	go pump(upstream, downstream, rewriteClientPayload)
	go pump(downstream, upstream, nil)

	select {
	case <-ctx.Done():
		_ = downstream.Close()
		_ = upstream.Close()
		return nil
	case err := <-errCh:
		_ = downstream.Close()
		_ = upstream.Close()
		if websocketCloseIsNormal(err) {
			return nil
		}
		return err
	}
}

func writeResponsesWebsocketClose(conn *websocket.Conn, code int, message string) {
	if conn == nil {
		return
	}
	deadline := time.Now().Add(time.Second)
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, message), deadline)
}

func websocketCloseIsNormal(err error) bool {
	if err == nil || errors.Is(err, io.EOF) {
		return true
	}
	return websocket.IsCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
		websocket.CloseAbnormalClosure,
	) || strings.Contains(strings.ToLower(err.Error()), "unexpected eof") ||
		strings.Contains(strings.ToLower(err.Error()), "use of closed network connection")
}
