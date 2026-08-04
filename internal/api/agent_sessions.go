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

	"csgclaw/internal/activity"
	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	"csgclaw/internal/worklease"
)

const agentSessionResponseBodyLimit = 1024 * 1024

var (
	agentSessionIDPattern         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~-]{0,127}$`)
	agentSessionResponseTimeout   = 5 * time.Minute
	agentSessionIdleGrace         = 500 * time.Millisecond
	agentSessionCleanupGrace      = 60 * time.Second
	agentSessionHeartbeatInterval = 15 * time.Second

	errSessionAgentUnavailable           = errors.New("agent participant is unavailable")
	errSessionRuntimeEnded               = errors.New("agent turn ended without a final response")
	errSessionInteractiveTurnUnsupported = errors.New("interactive approval and user-input requests are not supported for anonymous streamed sessions")
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

type resolvedSessionAgent struct {
	agent         agent.Agent
	participant   apitypes.Participant
	participantID string
	userID        string
}

type agentSessionTurnWaiter struct {
	handler          *Handler
	sessionID        string
	roomID           string
	requestID        string
	participantID    string
	agentUserID      string
	imEvents         <-chan im.Event
	cancelIM         func()
	workEvents       <-chan worklease.Event
	cancelWork       func()
	latestWork       *apitypes.ParticipantWorkUpdate
	runtimeEvents    <-chan activity.RuntimeEvent
	runtimeID        string
	runtimeSessionID string
	onTextDelta      func(string) error
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
	resolved, err := h.resolveSessionAgent(strings.TrimSpace(pathValue(r, "id")))
	if err != nil {
		if errors.Is(err, errSessionAgentUnavailable) {
			writeAgentSessionError(w, http.StatusServiceUnavailable, "agent_unavailable", err.Error(), "agent")
			return
		}
		writeAgentSessionError(w, http.StatusNotFound, "agent_not_found", err.Error(), "agent")
		return
	}
	if !h.beginSessionTurn(sessionID) {
		writeAgentSessionError(w, http.StatusConflict, "session_busy", "another response is already running for this session", "session_id")
		return
	}
	turnOwned := true
	defer func() {
		if turnOwned {
			h.endSessionTurn(sessionID)
		}
	}()

	if h.im == nil || h.imBus == nil {
		writeAgentSessionError(w, http.StatusServiceUnavailable, "session_service_unavailable", "session messaging is not configured", nil)
		return
	}
	room, err := h.im.EnsureAgentSessionRoom(im.EnsureAgentSessionRoomRequest{
		SessionID:   sessionID,
		AgentID:     resolved.agent.ID,
		AgentName:   resolved.agent.Name,
		AgentUserID: resolved.userID,
	})
	if err != nil {
		h.writeSessionRoomError(w, err)
		return
	}

	imEvents, cancelIM := h.imBus.Subscribe()
	var workEvents <-chan worklease.Event
	cancelWork := func() {}
	if h.workBus != nil {
		workEvents, cancelWork = h.workBus.Subscribe()
	}
	waiter := &agentSessionTurnWaiter{
		handler:       h,
		sessionID:     sessionID,
		roomID:        room.ID,
		participantID: resolved.participantID,
		agentUserID:   resolved.userID,
		imEvents:      imEvents,
		cancelIM:      cancelIM,
		workEvents:    workEvents,
		cancelWork:    cancelWork,
	}
	waiterOwned := true
	defer func() {
		if waiterOwned {
			waiter.close()
		}
	}()

	createdAt := time.Now().UTC()
	responseID := newAgentSessionAPIID("resp")
	outputID := newAgentSessionAPIID("msg")
	response := agentSessionResponse{
		ID:        responseID,
		Object:    "response",
		CreatedAt: createdAt.Unix(),
		Status:    "in_progress",
		Model:     resolved.agent.ID,
		Output:    []agentSessionResponseOutput{},
		Metadata: map[string]string{
			"session_id": sessionID,
			"room_id":    room.ID,
			"agent_id":   resolved.agent.ID,
		},
	}
	var eventStream *agentSessionEventStream
	var runtimeEvents <-chan activity.RuntimeEvent
	cancelRuntime := func() {}
	runtimeSessionID := ""
	runtimeID := strings.TrimSpace(resolved.agent.RuntimeID)
	directCodexStream := stream &&
		strings.EqualFold(strings.TrimSpace(resolved.agent.RuntimeKind), agent.RuntimeKindCodex) &&
		h.sessionEventSource != nil &&
		runtimeID != ""
	if directCodexStream {
		runtimeSessionID, err = h.sessionEventSource.EnsureSession(r.Context(), runtimeID, room.ID)
		if err != nil {
			writeAgentSessionError(w, http.StatusServiceUnavailable, "session_stream_failed", err.Error(), nil)
			return
		}
		runtimeEvents, cancelRuntime = subscribeAgentSessionEvents(h.sessionEventSource, runtimeID, runtimeSessionID)
	}
	defer cancelRuntime()
	if stream {
		eventStream = newAgentSessionEventStream(w)
		if err := eventStream.writeMessageStart(responseID); err != nil {
			return
		}
		eventStream.startHeartbeat(agentSessionHeartbeatInterval)
		defer eventStream.stopHeartbeat()
	}
	message, err := h.im.CreateMessage(im.CreateMessageRequest{
		RoomID:   room.ID,
		SenderID: im.AdminUserID,
		Content:  prompt,
		Metadata: map[string]any{
			"csgclaw": map[string]any{
				"agent_id":                    resolved.agent.ID,
				"session_id":                  sessionID,
				"sender_kind":                 "anonymous_admin",
				"agent_session_stream_direct": directCodexStream,
			},
		},
	})
	if err != nil {
		if eventStream != nil {
			eventStream.writeFailure(response, err)
			return
		}
		writeAgentSessionError(w, http.StatusServiceUnavailable, "message_delivery_failed", err.Error(), nil)
		return
	}
	if directCodexStream {
		h.publishMessageCreated(room.ID, im.AdminUserID, message)
		finalText, waitErr := h.streamDirectCodexSessionResponse(
			r.Context(), eventStream, response, resolved, room.ID, message.ID, prompt, runtimeEvents, runtimeID, runtimeSessionID,
		)
		if waitErr != nil {
			if errors.Is(waitErr, context.Canceled) {
				return
			}
			eventStream.writeFailure(response, waitErr)
			return
		}
		completedAt := time.Now().UTC()
		completedAtUnix := completedAt.Unix()
		response.CompletedAt = &completedAtUnix
		response.Status = "completed"
		response.Output = []agentSessionResponseOutput{{
			ID:     outputID,
			Type:   "message",
			Status: "completed",
			Role:   "assistant",
			Content: []agentSessionResponseContent{{
				Type:        "output_text",
				Text:        finalText,
				Annotations: []any{},
			}},
		}}
		_ = eventStream.writeCompleted(finalText)
		return
	}
	waiter.requestID = message.ID
	waiter.runtimeEvents = runtimeEvents
	waiter.runtimeID = runtimeID
	waiter.runtimeSessionID = runtimeSessionID
	if eventStream != nil && runtimeEvents != nil {
		waiter.onTextDelta = func(delta string) error {
			return eventStream.writeDelta(delta)
		}
	}
	h.publishMessageCreated(room.ID, im.AdminUserID, message)

	finalMessage, waitErr, detached := waiter.wait(r.Context())
	if detached {
		waiterOwned = false
		turnOwned = false
	}
	if waitErr != nil {
		if errors.Is(waitErr, context.Canceled) {
			return
		}
		if eventStream != nil {
			eventStream.writeFailure(response, waitErr)
			return
		}
		switch {
		case errors.Is(waitErr, context.DeadlineExceeded):
			writeAgentSessionError(w, http.StatusGatewayTimeout, "response_timeout", "the agent did not finish within five minutes", nil)
		case errors.Is(waitErr, errSessionRuntimeEnded):
			writeAgentSessionError(w, http.StatusServiceUnavailable, "agent_turn_ended", waitErr.Error(), nil)
		default:
			writeAgentSessionError(w, http.StatusServiceUnavailable, "agent_response_failed", waitErr.Error(), nil)
		}
		return
	}

	completedAt := time.Now().UTC()
	completedAtUnix := completedAt.Unix()
	response.CompletedAt = &completedAtUnix
	response.Status = "completed"
	response.Output = []agentSessionResponseOutput{{
		ID:     outputID,
		Type:   "message",
		Status: "completed",
		Role:   "assistant",
		Content: []agentSessionResponseContent{{
			Type:        "output_text",
			Text:        finalMessage.Content,
			Annotations: []any{},
		}},
	}}
	if eventStream != nil {
		_ = eventStream.writeCompleted(finalMessage.Content)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func subscribeAgentSessionEvents(source SessionEventSource, runtimeID, sessionID string) (<-chan activity.RuntimeEvent, func()) {
	if scoped, ok := source.(scopedSessionEventSource); ok {
		return scoped.SubscribeSession(runtimeID, sessionID)
	}
	return source.Subscribe(runtimeID)
}

func (h *Handler) streamDirectCodexSessionResponse(
	ctx context.Context,
	eventStream *agentSessionEventStream,
	response agentSessionResponse,
	resolved resolvedSessionAgent,
	roomID string,
	requestID string,
	prompt string,
	runtimeEvents <-chan activity.RuntimeEvent,
	runtimeID string,
	runtimeSessionID string,
) (string, error) {
	if h.sessionEventSource == nil || runtimeEvents == nil {
		return "", errSessionRuntimeEnded
	}
	ctx, cancel := context.WithTimeout(ctx, agentSessionResponseTimeout)
	defer cancel()
	promptDone := make(chan error, 1)
	go func() {
		promptDone <- h.sessionEventSource.Prompt(ctx, runtimeID, runtimeSessionID, prompt)
	}()
	promptReturned := false
	runtimeDone := false
	finalize := func() (string, error) {
		finalText := eventStream.text.String()
		if strings.TrimSpace(finalText) != "" {
			if _, err := h.im.DeliverMessage(im.DeliverMessageRequest{
				RoomID:   roomID,
				SenderID: resolved.userID,
				Content:  finalText,
				Metadata: map[string]any{"codex": map[string]any{
					"delivery_kind":     "final",
					"request_id":        requestID,
					"source_message_id": requestID,
				}},
			}); err != nil {
				return "", fmt.Errorf("persist final assistant message: %w", err)
			}
		}
		return finalText, nil
	}
	for {
		if runtimeDone && eventStream.text.Len() > 0 {
			cancel()
			return finalize()
		}
		if promptReturned && runtimeDone {
			return finalize()
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case err := <-promptDone:
			promptReturned = true
			if err != nil {
				return "", err
			}
		case event, ok := <-runtimeEvents:
			if !ok {
				runtimeDone = true
				continue
			}
			if strings.TrimSpace(event.RuntimeID) != strings.TrimSpace(runtimeID) ||
				strings.TrimSpace(event.SessionID) != strings.TrimSpace(runtimeSessionID) {
				continue
			}
			if event.Kind == activity.RuntimeEventPromptFailed {
				if strings.TrimSpace(event.Error) != "" {
					return "", fmt.Errorf("%s", event.Error)
				}
				return "", errSessionRuntimeEnded
			}
			if unsupportedInteractiveSessionEvent(event) {
				return "", errSessionInteractiveTurnUnsupported
			}
			if event.Kind == activity.RuntimeEventTextDelta {
				phase := runtimeEventPhase(event)
				if phase == "" || phase == "final_answer" {
					if err := eventStream.writeDelta(event.Text); err != nil {
						return "", err
					}
				}
			}
			if event.Kind == activity.RuntimeEventToolCallStart {
				if err := eventStream.writeToolUse(event); err != nil {
					return "", err
				}
			}
			if event.Kind == activity.RuntimeEventToolCallUpdate {
				if err := eventStream.writeToolResult(event); err != nil {
					return "", err
				}
			}
			if event.Kind == activity.RuntimeEventPromptCompleted {
				runtimeDone = true
			}
		}
	}
}

func unsupportedInteractiveSessionEvent(event activity.RuntimeEvent) bool {
	switch event.Kind {
	case activity.RuntimeEventActionRequest, activity.RuntimeEventUserInputRequest:
		return true
	case activity.RuntimeEventStructuredOutput:
		artifact, ok := event.Payload.(activity.StructuredOutputArtifact)
		return ok && artifact.RequestUserInput != nil
	default:
		return false
	}
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
	return s.writeLocked(eventType, fields)
}

func (s *agentSessionEventStream) writeLocked(eventType string, fields map[string]any) error {
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
			"index":         s.contentIndex,
			"content_block": map[string]any{"type": "text", "text": ""},
		}); err != nil {
			return err
		}
		s.textBlockOpen = true
	}
	if err := s.write("content_block_delta", map[string]any{
		"index": s.contentIndex,
		"delta": map[string]any{"type": "text_delta", "text": delta},
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

func (s *agentSessionEventStream) writeToolUse(event activity.RuntimeEvent) error {
	if err := s.closeTextBlock(); err != nil {
		return err
	}
	toolID := strings.TrimSpace(event.ToolCallID)
	if toolID == "" {
		toolID = newAgentSessionAPIID("toolu")
	}
	toolName, inputValue := agentSessionToolUse(event)
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
		"index": s.contentIndex,
		"delta": map[string]any{"type": "input_json_delta", "partial_json": string(encoded)},
	}); err != nil {
		return err
	}
	if err := s.write("content_block_stop", map[string]any{"index": s.contentIndex}); err != nil {
		return err
	}
	s.contentIndex++
	return nil
}

func agentSessionToolUse(event activity.RuntimeEvent) (string, map[string]any) {
	name := strings.TrimSpace(event.ToolKind)
	input := any(nil)
	payload, _ := event.Payload.(map[string]any)
	if payload != nil {
		switch name {
		case "mcp_tool_call", "dynamic_tool_call":
			if tool := strings.TrimSpace(agentSessionStringValue(payload["tool"])); tool != "" {
				name = tool
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
		if summary := strings.TrimSpace(event.ToolInputSummary); summary != "" && json.Unmarshal([]byte(summary), &decoded) == nil {
			input = decoded
		}
	}
	if name == "" {
		name = strings.TrimSpace(event.ToolTitle)
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
			if strings.Contains(lower, "token") || strings.Contains(lower, "secret") ||
				strings.Contains(lower, "password") || strings.Contains(lower, "api_key") ||
				strings.Contains(lower, "apikey") || strings.Contains(lower, "authorization") {
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

func (s *agentSessionEventStream) writeToolResult(event activity.RuntimeEvent) error {
	if err := s.closeTextBlock(); err != nil {
		return err
	}
	toolID := strings.TrimSpace(event.ToolCallID)
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
		"index": s.contentIndex,
		"delta": map[string]any{"type": "tool_result_delta", "content": event.ToolOutputSummary},
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

func (h *Handler) resolveSessionAgent(selector string) (resolvedSessionAgent, error) {
	if selector == "" || h.svc == nil {
		return resolvedSessionAgent{}, fmt.Errorf("agent not found")
	}
	selected, ok := h.svc.Agent(selector)
	if !ok {
		selected, ok = h.svc.AgentByName(selector)
	}
	if !ok {
		return resolvedSessionAgent{}, fmt.Errorf("agent %q not found", selector)
	}
	if h.participant == nil || h.im == nil {
		return resolvedSessionAgent{}, errSessionAgentUnavailable
	}
	items := h.participant.List(participant.ListOptions{
		Channel: participant.ChannelCSGClaw,
		Type:    participant.TypeAgent,
		AgentID: selected.ID,
	})
	active := make([]apitypes.Participant, 0, 1)
	for _, item := range items {
		if !strings.EqualFold(strings.TrimSpace(item.LifecycleStatus), participant.LifecycleStatusActive) ||
			item.ChannelUserKind != participant.ChannelUserKindLocalUserID ||
			strings.TrimSpace(item.ChannelUserRef) == "" {
			continue
		}
		if _, exists := h.im.User(item.ChannelUserRef); !exists {
			continue
		}
		active = append(active, item)
	}
	if len(active) != 1 {
		return resolvedSessionAgent{}, fmt.Errorf("%w: agent %q requires exactly one active local participant", errSessionAgentUnavailable, selected.ID)
	}
	return resolvedSessionAgent{
		agent:         selected,
		participant:   active[0],
		participantID: strings.TrimSpace(active[0].ID),
		userID:        h.im.ResolveUserID(active[0].ChannelUserRef),
	}, nil
}

func (h *Handler) beginSessionTurn(sessionID string) bool {
	h.sessionTurnsMu.Lock()
	defer h.sessionTurnsMu.Unlock()
	if h.sessionTurns == nil {
		h.sessionTurns = make(map[string]struct{})
	}
	if _, exists := h.sessionTurns[sessionID]; exists {
		return false
	}
	h.sessionTurns[sessionID] = struct{}{}
	return true
}

func (h *Handler) endSessionTurn(sessionID string) {
	h.sessionTurnsMu.Lock()
	delete(h.sessionTurns, sessionID)
	h.sessionTurnsMu.Unlock()
}

func (h *Handler) writeSessionRoomError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, im.ErrSessionAgentConflict):
		writeAgentSessionError(w, http.StatusConflict, "session_agent_conflict", err.Error(), "session_id")
	case errors.Is(err, im.ErrSessionRoomMembersConflict):
		writeAgentSessionError(w, http.StatusConflict, "session_room_invalid", err.Error(), "session_id")
	case errors.Is(err, im.ErrSessionRoomConflict):
		writeAgentSessionError(w, http.StatusConflict, "session_room_ambiguous", err.Error(), "session_id")
	default:
		writeAgentSessionError(w, http.StatusServiceUnavailable, "session_room_failed", err.Error(), nil)
	}
}

func (w *agentSessionTurnWaiter) wait(ctx context.Context) (im.Message, error, bool) {
	timeout := time.NewTimer(agentSessionResponseTimeout)
	defer timeout.Stop()
	var idleTimer *time.Timer
	var idle <-chan time.Time
	var finalMessage *im.Message
	runtimeDone := w.runtimeEvents == nil
	defer func() {
		if idleTimer != nil {
			idleTimer.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			w.detachCleanup()
			return im.Message{}, ctx.Err(), true
		case <-timeout.C:
			w.detachCleanup()
			return im.Message{}, context.DeadlineExceeded, true
		case <-idle:
			return im.Message{}, errSessionRuntimeEnded, false
		case event, ok := <-w.imEvents:
			if !ok {
				w.imEvents = nil
				continue
			}
			if w.matchesFinalMessage(event) {
				message := *event.Message
				finalMessage = &message
				if runtimeDone {
					return message, nil, false
				}
			}
		case event, ok := <-w.workEvents:
			if !ok {
				w.workEvents = nil
				continue
			}
			if !w.matchesWork(event.Work) {
				continue
			}
			update := event.Work
			w.latestWork = &update
			if update.State == apitypes.ParticipantWorkStateIdle {
				if idleTimer == nil {
					idleTimer = time.NewTimer(agentSessionIdleGrace)
				} else {
					idleTimer.Reset(agentSessionIdleGrace)
				}
				idle = idleTimer.C
			}
		case event, ok := <-w.runtimeEvents:
			if !ok {
				w.runtimeEvents = nil
				runtimeDone = true
				if finalMessage != nil {
					return *finalMessage, nil, false
				}
				continue
			}
			if !w.matchesRuntimeSession(event) {
				continue
			}
			if w.matchesRuntimeTextDelta(event) && w.onTextDelta != nil {
				if err := w.onTextDelta(event.Text); err != nil {
					return im.Message{}, err, false
				}
			}
			if event.Kind == activity.RuntimeEventPromptCompleted || event.Kind == activity.RuntimeEventPromptFailed {
				runtimeDone = true
				if finalMessage != nil {
					return *finalMessage, nil, false
				}
			}
		}
	}
}

func (w *agentSessionTurnWaiter) matchesRuntimeSession(event activity.RuntimeEvent) bool {
	return strings.TrimSpace(event.RuntimeID) == strings.TrimSpace(w.runtimeID) &&
		strings.TrimSpace(event.SessionID) == strings.TrimSpace(w.runtimeSessionID)
}

func (w *agentSessionTurnWaiter) matchesRuntimeTextDelta(event activity.RuntimeEvent) bool {
	if event.Kind != activity.RuntimeEventTextDelta {
		return false
	}
	phase := runtimeEventPhase(event)
	return phase == "" || phase == "final_answer"
}

func runtimeEventPhase(event activity.RuntimeEvent) string {
	payload, ok := event.Payload.(map[string]any)
	if !ok {
		return ""
	}
	phase, ok := payload["phase"]
	if !ok || phase == nil {
		return ""
	}
	return strings.TrimSpace(strings.ToLower(fmt.Sprint(phase)))
}

func (w *agentSessionTurnWaiter) detachCleanup() {
	go func() {
		defer w.close()
		defer w.handler.endSessionTurn(w.sessionID)
		cleanupTimer := time.NewTimer(agentSessionCleanupGrace)
		defer cleanupTimer.Stop()
		stopRequested := false
		requestStop := func(update *apitypes.ParticipantWorkUpdate) {
			if stopRequested || update == nil || update.State != apitypes.ParticipantWorkStateWorking {
				return
			}
			stopRequested = true
			w.handler.requestSessionTurnStop(*update)
		}
		requestStop(w.latestWork)
		for {
			select {
			case <-cleanupTimer.C:
				return
			case event, ok := <-w.imEvents:
				if !ok {
					w.imEvents = nil
					continue
				}
				if w.matchesFinalMessage(event) {
					return
				}
			case event, ok := <-w.workEvents:
				if !ok {
					w.workEvents = nil
					continue
				}
				if !w.matchesWork(event.Work) {
					continue
				}
				update := event.Work
				w.latestWork = &update
				if update.State == apitypes.ParticipantWorkStateIdle {
					return
				}
				requestStop(&update)
			}
		}
	}()
}

func (w *agentSessionTurnWaiter) matchesFinalMessage(event im.Event) bool {
	if event.Type != im.EventTypeMessageCreated || event.RoomID != w.roomID || event.Message == nil {
		return false
	}
	message := event.Message
	if w.handler.im.ResolveUserID(message.SenderID) != w.handler.im.ResolveUserID(w.agentUserID) {
		return false
	}
	if strings.TrimSpace(message.Content) == "" || message.Kind == im.MessageKindEvent {
		return false
	}

	hasDeliveryMetadata := false
	for _, namespace := range []string{"csgclaw", "codex", "openclaw", "picoclaw"} {
		metadata, ok := message.Metadata[namespace].(map[string]any)
		if !ok {
			continue
		}
		kind := strings.TrimSpace(fmt.Sprint(metadata["delivery_kind"]))
		if kind == "" {
			continue
		}
		hasDeliveryMetadata = true
		if !strings.EqualFold(kind, "final") {
			continue
		}
		requestID := strings.TrimSpace(fmt.Sprint(metadata["request_id"]))
		sourceMessageID := strings.TrimSpace(fmt.Sprint(metadata["source_message_id"]))
		if requestID == w.requestID || sourceMessageID == w.requestID {
			return true
		}
	}
	return !hasDeliveryMetadata && message.RelatesTo == nil
}

func (w *agentSessionTurnWaiter) matchesWork(update apitypes.ParticipantWorkUpdate) bool {
	return update.RoomID == w.roomID &&
		update.RequestID == w.requestID &&
		(update.ParticipantID == w.participantID || w.handler.im.ResolveUserID(update.UserID) == w.handler.im.ResolveUserID(w.agentUserID))
}

func (w *agentSessionTurnWaiter) close() {
	if w.cancelIM != nil {
		w.cancelIM()
		w.cancelIM = nil
	}
	if w.cancelWork != nil {
		w.cancelWork()
		w.cancelWork = nil
	}
}

func (h *Handler) requestSessionTurnStop(update apitypes.ParticipantWorkUpdate) {
	controller, ok := h.participantWork.(worklease.ParticipantWorkController)
	if !ok || strings.TrimSpace(update.LeaseID) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), participantTurnStopTimeout)
	defer cancel()
	_, _ = controller.RequestStop(ctx, update.ParticipantID, apitypes.ParticipantWorkStopRequest{
		RoomID:    update.RoomID,
		LeaseID:   update.LeaseID,
		RequestID: update.RequestID,
	})
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
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}
