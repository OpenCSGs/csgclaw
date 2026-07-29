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
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
	"csgclaw/internal/worklease"
)

const agentSessionResponseBodyLimit = 1024 * 1024

var (
	agentSessionIDPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~-]{0,127}$`)
	agentSessionResponseTimeout = 5 * time.Minute
	agentSessionIdleGrace       = 500 * time.Millisecond
	agentSessionCleanupGrace    = 60 * time.Second

	errSessionAgentUnavailable = errors.New("agent participant is unavailable")
	errSessionRuntimeEnded     = errors.New("agent turn ended without a final response")
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
	ID          string                       `json:"id"`
	Object      string                       `json:"object"`
	CreatedAt   int64                        `json:"created_at"`
	CompletedAt int64                        `json:"completed_at"`
	Status      string                       `json:"status"`
	Model       string                       `json:"model"`
	Output      []agentSessionResponseOutput `json:"output"`
	Metadata    map[string]string            `json:"metadata"`
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
	handler       *Handler
	sessionID     string
	roomID        string
	requestID     string
	participantID string
	agentUserID   string
	imEvents      <-chan im.Event
	cancelIM      func()
	workEvents    <-chan worklease.Event
	cancelWork    func()
	latestWork    *apitypes.ParticipantWorkUpdate
}

func (h *Handler) createAgentSessionResponse(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(pathValue(r, "session_id"))
	if !agentSessionIDPattern.MatchString(sessionID) {
		writeAgentSessionError(w, http.StatusBadRequest, "invalid_session_id", "session_id must be 1-128 path-safe ASCII characters", "session_id")
		return
	}

	prompt, err := parseAgentSessionResponseRequest(w, r)
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
	message, err := h.im.CreateMessage(im.CreateMessageRequest{
		RoomID:   room.ID,
		SenderID: im.AdminUserID,
		Content:  prompt,
		Metadata: map[string]any{
			"csgclaw": map[string]any{
				"agent_id":    resolved.agent.ID,
				"session_id":  sessionID,
				"sender_kind": "anonymous_admin",
			},
		},
	})
	if err != nil {
		writeAgentSessionError(w, http.StatusServiceUnavailable, "message_delivery_failed", err.Error(), nil)
		return
	}
	waiter.requestID = message.ID
	h.publishMessageCreated(room.ID, im.AdminUserID, message)

	finalMessage, waitErr, detached := waiter.wait(r.Context())
	if detached {
		waiterOwned = false
		turnOwned = false
	}
	if waitErr != nil {
		switch {
		case errors.Is(waitErr, context.Canceled):
			return
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
	writeJSON(w, http.StatusOK, agentSessionResponse{
		ID:          newAgentSessionAPIID("resp"),
		Object:      "response",
		CreatedAt:   createdAt.Unix(),
		CompletedAt: completedAt.Unix(),
		Status:      "completed",
		Model:       resolved.agent.ID,
		Output: []agentSessionResponseOutput{{
			ID:     finalMessage.ID,
			Type:   "message",
			Status: "completed",
			Role:   "assistant",
			Content: []agentSessionResponseContent{{
				Type:        "output_text",
				Text:        finalMessage.Content,
				Annotations: []any{},
			}},
		}},
		Metadata: map[string]string{
			"session_id": sessionID,
			"room_id":    room.ID,
			"agent_id":   resolved.agent.ID,
		},
	})
}

func parseAgentSessionResponseRequest(w http.ResponseWriter, r *http.Request) (string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, agentSessionResponseBodyLimit)
	var request agentSessionResponseRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return "", fmt.Errorf("decode request: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return "", err
	}
	if request.Stream != nil && *request.Stream {
		return "", fmt.Errorf("streaming is not supported")
	}
	if len(request.Input) == 0 || string(request.Input) == "null" {
		return "", fmt.Errorf("input is required")
	}
	return parseAgentSessionInput(request.Input)
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
				return *event.Message, nil, false
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
		}
	}
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
