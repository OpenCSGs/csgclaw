package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/agentengine"
)

const agentSessionResponseBodyLimit = 1024 * 1024

var (
	agentSessionIDPattern         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~-]{0,127}$`)
	agentSessionResponseTimeout   = 5 * time.Minute
	agentSessionHeartbeatInterval = 15 * time.Second
)

type agentSessionResponseRequest struct {
	Input  json.RawMessage `json:"input"`
	Stream *bool           `json:"stream,omitempty"`
}

type agentSessionInputMessage struct {
	Type    string          `json:"type,omitempty"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type agentSessionInputPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type agentSessionResponse struct {
	ID                string                       `json:"id"`
	Object            string                       `json:"object"`
	CreatedAt         int64                        `json:"created_at"`
	CompletedAt       *int64                       `json:"completed_at"`
	Status            string                       `json:"status"`
	Error             any                          `json:"error"`
	IncompleteDetails any                          `json:"incomplete_details"`
	Model             string                       `json:"model"`
	Output            []agentSessionResponseOutput `json:"output"`
	Metadata          map[string]string            `json:"metadata"`
}

type agentSessionResponseOutput struct {
	ID      string                        `json:"id"`
	Type    string                        `json:"type"`
	Status  string                        `json:"status"`
	Role    string                        `json:"role"`
	Content []agentSessionResponseContent `json:"content"`
}

type agentSessionResponseContent struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Annotations []any  `json:"annotations"`
}

type agentSessionErrorEnvelope struct {
	Error agentSessionError `json:"error"`
}

type agentSessionError struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    string  `json:"code"`
}

type agentSessionTurn struct {
	agentID         string
	conversationKey agentengine.ConversationKey
	turnID          agentengine.TurnID
	cancel          context.CancelFunc
}

func (h *Handler) createAgentSessionResponse(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(pathValue(r, "session_id"))
	if !agentSessionIDPattern.MatchString(sessionID) {
		writeAgentSessionError(w, http.StatusBadRequest, "invalid_session_id", "session_id must be 1-128 path-safe ASCII characters", "session_id")
		return
	}

	prompt, stream, err := parseAgentSessionResponseRequest(w, r)
	if err != nil {
		writeAgentSessionError(w, http.StatusBadRequest, "invalid_request", err.Error(), "input")
		return
	}
	selected, err := h.resolveSessionAgent(strings.TrimSpace(pathValue(r, "id")))
	if err != nil {
		if errors.Is(err, errSessionAgentUnavailable) {
			writeAgentSessionError(w, http.StatusServiceUnavailable, "agent_unavailable", err.Error(), "agent")
			return
		}
		writeAgentSessionError(w, http.StatusNotFound, "agent_not_found", err.Error(), "agent")
		return
	}
	if h.agentEngine == nil || h.sessionBindings == nil {
		writeAgentSessionError(w, http.StatusServiceUnavailable, "session_service_unavailable", "session conversation service is not configured", nil)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(selected.Spec.Runtime.Adapter), "codex") {
		writeAgentSessionError(w, http.StatusServiceUnavailable, "runtime_adapter_unavailable", fmt.Sprintf("runtime adapter %q is unavailable", selected.Spec.Runtime.Adapter), "agent")
		return
	}

	binding, err := h.sessionBindings.GetOrCreate(selected.ID, sessionID)
	if err != nil {
		writeAgentSessionError(w, http.StatusServiceUnavailable, "session_binding_failed", err.Error(), nil)
		return
	}

	createdAt := time.Now().UTC()
	responseID := newAgentSessionAPIID("resp")
	outputID := newAgentSessionAPIID("msg")
	response := agentSessionResponse{
		ID:        responseID,
		Object:    "response",
		CreatedAt: createdAt.Unix(),
		Status:    "in_progress",
		Model:     selected.ID,
		Output:    []agentSessionResponseOutput{},
		Metadata: map[string]string{
			"session_id": sessionID,
			"room_id":    "",
			"agent_id":   selected.ID,
		},
	}
	turnKey := selected.ID + "\x00" + sessionID
	runCtx, cancel := context.WithTimeout(r.Context(), agentSessionResponseTimeout)
	defer cancel()
	turn := agentSessionTurn{
		agentID:         selected.ID,
		conversationKey: agentengine.ConversationKey(binding.ConversationKey),
		turnID:          agentengine.TurnID(responseID),
		cancel:          cancel,
	}
	h.addSessionTurn(turnKey, turn)
	defer h.removeSessionTurn(turnKey, turn.turnID)

	var eventStream *agentSessionEventStream
	var sink agentengine.EventSink
	if stream {
		eventStream = newAgentSessionEventStream(w)
		if err := eventStream.writeMessageStart(responseID); err != nil {
			return
		}
		eventStream.startHeartbeat(agentSessionHeartbeatInterval)
		defer eventStream.stopHeartbeat()
		sink = agentengine.EventSinkFunc(func(_ context.Context, event agentengine.TurnEvent) error {
			switch event.Kind {
			case agentengine.TurnEventTextDelta:
				return eventStream.writeDelta(event.Text)
			case agentengine.TurnEventToolCallStart:
				return eventStream.writeToolUse(event.Tool)
			case agentengine.TurnEventToolCallUpdate:
				return eventStream.writeToolResult(event.Tool)
			default:
				return nil
			}
		})
	}

	result := h.agentEngine.Conversations(selected.ID).Run(runCtx, agentengine.TurnRequest{
		ID:              agentengine.TurnID(responseID),
		ConversationKey: agentengine.ConversationKey(binding.ConversationKey),
		Admission:       agentengine.AdmissionRejectIfBusy,
		Interaction:     agentengine.InteractionReject,
		Input: []agentengine.InputPart{{
			Kind: agentengine.InputPartText,
			Text: prompt,
		}},
	}, sink)
	if result.Status != agentengine.TurnSucceeded {
		if errors.Is(r.Context().Err(), context.Canceled) {
			return
		}
		if eventStream != nil {
			eventStream.writeFailure(response, turnResultError(result))
			return
		}
		h.writeSessionTurnError(w, runCtx, result)
		return
	}

	completedAtUnix := time.Now().UTC().Unix()
	response.CompletedAt = &completedAtUnix
	response.Status = "completed"
	response.Output = []agentSessionResponseOutput{{
		ID:     outputID,
		Type:   "message",
		Status: "completed",
		Role:   "assistant",
		Content: []agentSessionResponseContent{{
			Type:        "output_text",
			Text:        result.Output,
			Annotations: []any{},
		}},
	}}
	if eventStream != nil {
		_ = eventStream.writeCompleted(result.Output)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) cancelAgentSessionResponse(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(pathValue(r, "session_id"))
	if !agentSessionIDPattern.MatchString(sessionID) {
		writeAgentSessionError(w, http.StatusBadRequest, "invalid_session_id", "session_id must be 1-128 path-safe ASCII characters", "session_id")
		return
	}

	selected, err := h.resolveSessionAgent(strings.TrimSpace(pathValue(r, "id")))
	if err != nil {
		if errors.Is(err, errSessionAgentUnavailable) {
			writeAgentSessionError(w, http.StatusServiceUnavailable, "agent_unavailable", err.Error(), "agent")
			return
		}
		writeAgentSessionError(w, http.StatusNotFound, "agent_not_found", err.Error(), "agent")
		return
	}

	if err := h.cancelSessionTurns(r.Context(), selected.ID+"\x00"+sessionID); err != nil {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var errSessionAgentUnavailable = errors.New("agent is unavailable")

func (h *Handler) resolveSessionAgent(selector string) (agentengine.Agent, error) {
	if selector == "" || h.agentEngine == nil {
		return agentengine.Agent{}, fmt.Errorf("agent not found")
	}
	selected, err := h.agentEngine.Agents().Get(context.Background(), selector)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return agentengine.Agent{}, err
		}
		return agentengine.Agent{}, fmt.Errorf("%w: %v", errSessionAgentUnavailable, err)
	}
	if selected.Status.State != agentengine.AgentStateRunning || strings.TrimSpace(selected.Status.RuntimeID) == "" {
		return agentengine.Agent{}, fmt.Errorf("%w: agent %q is not running", errSessionAgentUnavailable, selected.ID)
	}
	return selected, nil
}

func (h *Handler) addSessionTurn(key string, turn agentSessionTurn) {
	h.sessionTurnsMu.Lock()
	defer h.sessionTurnsMu.Unlock()
	if h.sessionTurns == nil {
		h.sessionTurns = make(map[string]map[agentengine.TurnID]agentSessionTurn)
	}
	if h.sessionTurns[key] == nil {
		h.sessionTurns[key] = make(map[agentengine.TurnID]agentSessionTurn)
	}
	h.sessionTurns[key][turn.turnID] = turn
}

func (h *Handler) cancelSessionTurns(ctx context.Context, key string) error {
	h.sessionTurnsMu.Lock()
	indexed := h.sessionTurns[key]
	turns := make([]agentSessionTurn, 0, len(indexed))
	for _, turn := range indexed {
		turns = append(turns, turn)
	}
	h.sessionTurnsMu.Unlock()
	for _, turn := range turns {
		if turn.cancel != nil {
			turn.cancel()
		}
		if err := h.agentEngine.Conversations(turn.agentID).Cancel(ctx, turn.conversationKey, turn.turnID); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) removeSessionTurn(key string, turnID agentengine.TurnID) {
	h.sessionTurnsMu.Lock()
	if indexed := h.sessionTurns[key]; indexed != nil {
		delete(indexed, turnID)
	}
	if len(h.sessionTurns[key]) == 0 {
		delete(h.sessionTurns, key)
	}
	h.sessionTurnsMu.Unlock()
}

func (h *Handler) writeSessionTurnError(w http.ResponseWriter, ctx context.Context, result agentengine.TurnResult) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		writeAgentSessionError(w, http.StatusGatewayTimeout, "response_timeout", "the agent did not finish within five minutes", nil)
		return
	}
	err := result.Error
	if err == nil {
		writeAgentSessionError(w, http.StatusServiceUnavailable, "agent_response_failed", "agent response failed", nil)
		return
	}
	switch err.Code {
	case agentengine.ErrorInvalidRequest:
		writeAgentSessionError(w, http.StatusBadRequest, string(err.Code), err.Message, "input")
	case agentengine.ErrorFileUnavailable:
		writeAgentSessionError(w, http.StatusBadRequest, string(err.Code), err.Message, "input")
	case agentengine.ErrorAgentUnavailable:
		writeAgentSessionError(w, http.StatusServiceUnavailable, string(err.Code), err.Message, "agent")
	case agentengine.ErrorRuntimeAdapterUnavailable:
		writeAgentSessionError(w, http.StatusServiceUnavailable, string(err.Code), err.Message, "agent")
	case agentengine.ErrorConversationBusy:
		writeAgentSessionError(w, http.StatusConflict, "session_busy", err.Message, "session_id")
	case agentengine.ErrorInteractionUnsupported:
		writeAgentSessionError(w, http.StatusServiceUnavailable, string(err.Code), err.Message, nil)
	default:
		writeAgentSessionError(w, http.StatusServiceUnavailable, "agent_response_failed", err.Message, nil)
	}
}

func turnResultError(result agentengine.TurnResult) error {
	if result.Error != nil {
		return result.Error
	}
	return errors.New("agent response failed")
}

func parseAgentSessionResponseRequest(w http.ResponseWriter, r *http.Request) (string, bool, error) {
	r.Body = http.MaxBytesReader(w, r.Body, agentSessionResponseBodyLimit)
	var request agentSessionResponseRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return "", false, fmt.Errorf("decode request: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return "", false, err
	}
	if len(request.Input) == 0 || string(request.Input) == "null" {
		return "", false, fmt.Errorf("input is required")
	}
	prompt, err := parseAgentSessionInput(request.Input)
	return prompt, request.Stream != nil && *request.Stream, err
}

func parseAgentSessionInput(raw json.RawMessage) (string, error) {
	var direct string
	if err := json.Unmarshal(raw, &direct); err == nil {
		if direct = strings.TrimSpace(direct); direct != "" {
			return direct, nil
		}
		return "", fmt.Errorf("input must not be empty")
	}

	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return "", fmt.Errorf("input must be a string or an array of user messages")
	}
	if len(items) == 0 {
		return "", fmt.Errorf("input must contain at least one user message")
	}
	texts := make([]string, 0, len(items))
	for _, rawItem := range items {
		var item agentSessionInputMessage
		if err := decodeStrictAgentSessionJSON(rawItem, &item); err != nil {
			return "", fmt.Errorf("invalid input message: %w", err)
		}
		if item.Type != "" && item.Type != "message" {
			return "", fmt.Errorf("input message type must be message")
		}
		if item.Role != "user" {
			return "", fmt.Errorf("input message role must be user")
		}
		text, err := parseAgentSessionContent(item.Content)
		if err != nil {
			return "", err
		}
		texts = append(texts, text)
	}
	return strings.Join(texts, "\n\n"), nil
}

func parseAgentSessionContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("input message content is required")
	}
	var direct string
	if err := json.Unmarshal(raw, &direct); err == nil {
		if direct = strings.TrimSpace(direct); direct != "" {
			return direct, nil
		}
		return "", fmt.Errorf("input message content must not be empty")
	}
	var rawParts []json.RawMessage
	if err := json.Unmarshal(raw, &rawParts); err != nil || len(rawParts) == 0 {
		return "", fmt.Errorf("input message content must be text or input_text parts")
	}
	parts := make([]string, 0, len(rawParts))
	for _, rawPart := range rawParts {
		var part agentSessionInputPart
		if err := decodeStrictAgentSessionJSON(rawPart, &part); err != nil {
			return "", fmt.Errorf("invalid input content part: %w", err)
		}
		if part.Type != "input_text" {
			return "", fmt.Errorf("input content part type must be input_text")
		}
		text := strings.TrimSpace(part.Text)
		if text == "" {
			return "", fmt.Errorf("input_text must not be empty")
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n\n"), nil
}

func decodeStrictAgentSessionJSON(raw json.RawMessage, dst any) error {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	return ensureJSONEOF(decoder)
}

type agentSessionEventStream struct {
	writer        http.ResponseWriter
	flusher       *http.ResponseController
	writeMu       sync.Mutex
	contentIndex  int
	textBlockOpen bool
	text          strings.Builder
	heartbeatStop chan struct{}
	heartbeatDone chan struct{}
	heartbeatOnce sync.Once
}

func newAgentSessionEventStream(w http.ResponseWriter) *agentSessionEventStream {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	return &agentSessionEventStream{writer: w, flusher: http.NewResponseController(w)}
}

func (s *agentSessionEventStream) write(eventType string, fields map[string]any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	payload := make(map[string]any, len(fields)+1)
	for key, value := range fields {
		payload[key] = value
	}
	payload["type"] = eventType
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.writer, "event: %s\ndata: %s\n\n", eventType, data); err != nil {
		return err
	}
	if err := s.flusher.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	return nil
}

func (s *agentSessionEventStream) startHeartbeat(interval time.Duration) {
	if s == nil || interval <= 0 || s.heartbeatStop != nil {
		return
	}
	s.heartbeatStop = make(chan struct{})
	s.heartbeatDone = make(chan struct{})
	go func() {
		defer close(s.heartbeatDone)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.writeMu.Lock()
				_, err := io.WriteString(s.writer, ": heartbeat\n\n")
				if err == nil {
					err = s.flusher.Flush()
				}
				s.writeMu.Unlock()
				if err != nil && !errors.Is(err, http.ErrNotSupported) {
					return
				}
			case <-s.heartbeatStop:
				return
			}
		}
	}()
}

func (s *agentSessionEventStream) stopHeartbeat() {
	if s == nil || s.heartbeatStop == nil {
		return
	}
	s.heartbeatOnce.Do(func() { close(s.heartbeatStop) })
	<-s.heartbeatDone
}

func (s *agentSessionEventStream) writeMessageStart(messageID string) error {
	return s.write("message_start", map[string]any{"message": map[string]any{
		"id": messageID, "type": "message", "role": "assistant",
		"content": []any{}, "stop_reason": nil,
	}})
}

func (s *agentSessionEventStream) writeDelta(delta string) error {
	if delta == "" {
		return nil
	}
	if !s.textBlockOpen {
		if err := s.write("content_block_start", map[string]any{
			"index": s.contentIndex, "content_block": map[string]any{"type": "text", "text": ""},
		}); err != nil {
			return err
		}
		s.textBlockOpen = true
	}
	if err := s.write("content_block_delta", map[string]any{
		"index": s.contentIndex, "delta": map[string]any{"type": "text_delta", "text": delta},
	}); err != nil {
		return err
	}
	_, _ = s.text.WriteString(delta)
	return nil
}

func (s *agentSessionEventStream) closeTextBlock() error {
	if !s.textBlockOpen {
		return nil
	}
	if err := s.write("content_block_stop", map[string]any{"index": s.contentIndex}); err != nil {
		return err
	}
	s.contentIndex++
	s.textBlockOpen = false
	return nil
}

func (s *agentSessionEventStream) writeCompleted(text string) error {
	s.stopHeartbeat()
	streamedText := s.text.String()
	switch {
	case streamedText == "":
		if err := s.writeDelta(text); err != nil {
			return err
		}
	case strings.HasPrefix(text, streamedText):
		if err := s.writeDelta(strings.TrimPrefix(text, streamedText)); err != nil {
			return err
		}
	}
	if err := s.closeTextBlock(); err != nil {
		return err
	}
	if err := s.write("message_delta", map[string]any{
		"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
	}); err != nil {
		return err
	}
	return s.write("message_stop", map[string]any{})
}

func (s *agentSessionEventStream) writeToolUse(tool *agentengine.ToolActivity) error {
	if tool == nil {
		return nil
	}
	if err := s.closeTextBlock(); err != nil {
		return err
	}
	toolID := strings.TrimSpace(tool.ID)
	if toolID == "" {
		toolID = newAgentSessionAPIID("toolu")
	}
	toolName, inputValue := agentSessionToolUse(tool)
	if err := s.write("content_block_start", map[string]any{
		"index":         s.contentIndex,
		"content_block": map[string]any{"type": "tool_use", "id": toolID, "name": toolName, "input": map[string]any{}},
	}); err != nil {
		return err
	}
	encoded, err := json.Marshal(inputValue)
	if err != nil {
		return err
	}
	if err := s.write("content_block_delta", map[string]any{
		"index": s.contentIndex, "delta": map[string]any{"type": "input_json_delta", "partial_json": string(encoded)},
	}); err != nil {
		return err
	}
	if err := s.write("content_block_stop", map[string]any{"index": s.contentIndex}); err != nil {
		return err
	}
	s.contentIndex++
	return nil
}

func agentSessionToolUse(tool *agentengine.ToolActivity) (string, map[string]any) {
	name := strings.TrimSpace(tool.Kind)
	input := any(nil)
	payload, _ := tool.Payload.(map[string]any)
	if payload != nil {
		switch name {
		case "mcp_tool_call", "dynamic_tool_call":
			if toolName := strings.TrimSpace(agentSessionStringValue(payload["tool"])); toolName != "" {
				name = toolName
			}
			input = payload["arguments"]
		case "exec_command":
			input = map[string]any{"command": payload["command"]}
		case "web_search":
			input = map[string]any{"query": payload["query"], "action": payload["action"]}
		case "patch_apply":
			if changes, ok := payload["changes"]; ok {
				input = map[string]any{"changes": changes}
			} else {
				input = payload
			}
		}
	}
	if input == nil {
		var decoded any
		if summary := strings.TrimSpace(tool.InputSummary); summary != "" && json.Unmarshal([]byte(summary), &decoded) == nil {
			input = decoded
		}
	}
	if name == "" {
		name = strings.TrimSpace(tool.Title)
	}
	if name == "" {
		name = "unknown"
	}
	return name, normalizeAgentSessionToolInput(redactAgentSessionToolValue(input))
}

func agentSessionStringValue(value any) string {
	text, _ := value.(string)
	return text
}

func normalizeAgentSessionToolInput(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if object, ok := value.(map[string]any); ok {
		return object
	}
	return map[string]any{"value": value}
}

var (
	agentSessionBearerPattern           = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-]+`)
	agentSessionAPIKeyPattern           = regexp.MustCompile(`\bsk-[A-Za-z0-9._\-]+\b`)
	agentSessionSecretAssignmentPattern = regexp.MustCompile(`(?i)\b(token|api[_-]?key|apikey|password|secret)(\s*[:=]\s*)[^\s"']+`)
)

func redactAgentSessionToolValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			lower := strings.ToLower(strings.TrimSpace(key))
			if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") ||
				strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey") || strings.Contains(lower, "authorization") {
				out[key] = "[redacted]"
				continue
			}
			out[key] = redactAgentSessionToolValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = redactAgentSessionToolValue(item)
		}
		return out
	case string:
		redacted := agentSessionBearerPattern.ReplaceAllString(typed, "Bearer [redacted]")
		redacted = agentSessionAPIKeyPattern.ReplaceAllString(redacted, "[redacted]")
		return agentSessionSecretAssignmentPattern.ReplaceAllString(redacted, "$1$2[redacted]")
	default:
		return value
	}
}

func (s *agentSessionEventStream) writeToolResult(tool *agentengine.ToolActivity) error {
	if tool == nil {
		return nil
	}
	if err := s.closeTextBlock(); err != nil {
		return err
	}
	toolID := strings.TrimSpace(tool.ID)
	if toolID == "" {
		toolID = "unknown"
	}
	if err := s.write("content_block_start", map[string]any{
		"index":         s.contentIndex,
		"content_block": map[string]any{"type": "tool_result", "tool_use_id": toolID, "content": ""},
	}); err != nil {
		return err
	}
	if err := s.write("content_block_delta", map[string]any{
		"index": s.contentIndex, "delta": map[string]any{"type": "tool_result_delta", "content": tool.OutputSummary},
	}); err != nil {
		return err
	}
	if err := s.write("content_block_stop", map[string]any{"index": s.contentIndex}); err != nil {
		return err
	}
	s.contentIndex++
	return nil
}

func (s *agentSessionEventStream) writeFailure(response agentSessionResponse, cause error) {
	s.stopHeartbeat()
	_ = s.closeTextBlock()
	_ = s.write("error", map[string]any{"error": map[string]any{
		"type": "server_error", "message": cause.Error(), "request_id": response.ID,
	}})
	_ = s.write("message_delta", map[string]any{
		"delta": map[string]any{"stop_reason": "error", "stop_sequence": nil},
	})
	_ = s.write("message_stop", map[string]any{})
}

func writeAgentSessionError(w http.ResponseWriter, status int, code, message string, param any) {
	var parameter *string
	if value, ok := param.(string); ok && value != "" {
		parameter = &value
	}
	errorType := "server_error"
	switch {
	case status == http.StatusBadRequest:
		errorType = "invalid_request_error"
	case status == http.StatusNotFound:
		errorType = "not_found_error"
	case status == http.StatusConflict:
		errorType = "conflict_error"
	}
	writeJSON(w, status, agentSessionErrorEnvelope{Error: agentSessionError{
		Message: message,
		Type:    errorType,
		Param:   parameter,
		Code:    code,
	}})
}

func newAgentSessionAPIID(prefix string) string {
	var random [12]byte
	if _, err := io.ReadFull(rand.Reader, random[:]); err == nil {
		return prefix + "_" + hex.EncodeToString(random[:])
	}
	return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
}

var _ agentengine.Interface = (*agentengine.Engine)(nil)
