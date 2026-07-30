package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
)

func TestAgentSessionResponsesCreatesAuditableAdminRoomAndReturnsFinalText(t *testing.T) {
	handler, imSvc, bus := newAgentSessionTestHandler(t, []agent.Agent{
		completeWorkerAgent("agent-alpha", "Alpha"),
	})
	events, cancel := bus.Subscribe()
	defer cancel()
	go func() {
		for event := range events {
			if event.Type != im.EventTypeMessageCreated || event.Message == nil || event.Message.SenderID != im.AdminUserID {
				continue
			}
			_, _ = imSvc.DeliverMessage(im.DeliverMessageRequest{
				RoomID:   event.RoomID,
				SenderID: "user-alpha",
				Content:  "final answer",
				Metadata: map[string]any{
					"codex": map[string]any{
						"delivery_kind":     "final",
						"request_id":        event.Message.ID,
						"source_message_id": event.Message.ID,
					},
				},
			})
			return
		}
	}()

	recorder := performAgentSessionRequest(t, handler, "Alpha", "audit-123", map[string]any{"input": "Review this"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response agentSessionResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "completed" || response.Model != "agent-alpha" || len(response.Output) != 1 || response.Output[0].Content[0].Text != "final answer" {
		t.Fatalf("response = %+v", response)
	}
	roomID := response.Metadata["room_id"]
	room, ok := imSvc.Room(roomID)
	if !ok {
		t.Fatalf("room %q not found", roomID)
	}
	wantTitle := "Anonymous Session: audit-123 | Agent: Alpha (agent-alpha)"
	if room.Title != wantTitle || room.SessionID != "audit-123" || !room.NotifyAllAgents || len(room.Members) != 2 {
		t.Fatalf("room = %+v, want auditable notify-all session room", room)
	}
	messages, err := imSvc.ListMessages(room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) < 3 || messages[1].SenderID != im.AdminUserID || messages[1].Content != "Review this" {
		t.Fatalf("messages = %#v, want anonymous input persisted as admin", messages)
	}
}

func TestAgentSessionResponsesSupportsMessageItemsAndRejectsAgentReuse(t *testing.T) {
	handler, imSvc, bus := newAgentSessionTestHandler(t, []agent.Agent{
		completeWorkerAgent("agent-alpha", "Alpha"),
		completeWorkerAgent("agent-beta", "Beta"),
	})
	events, cancel := bus.Subscribe()
	defer cancel()
	go replyToNextAdminMessage(events, imSvc, "user-alpha", "alpha result")

	recorder := performAgentSessionRequest(t, handler, "agent-alpha", "bound-session", map[string]any{
		"input": []map[string]any{
			{"type": "message", "role": "user", "content": "First"},
			{"role": "user", "content": []map[string]any{{"type": "input_text", "text": "Second"}}},
		},
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("first status = %d; body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = performAgentSessionRequest(t, handler, "agent-beta", "bound-session", map[string]any{"input": "switch"})
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "session_agent_conflict") {
		t.Fatalf("reuse status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAgentSessionResponsesRejectsOverlappingTurnButAllowsOtherSessions(t *testing.T) {
	handler, imSvc, bus := newAgentSessionTestHandler(t, []agent.Agent{
		completeWorkerAgent("agent-alpha", "Alpha"),
	})
	events, cancel := bus.Subscribe()
	defer cancel()
	firstSource := make(chan im.Event, 1)
	go func() {
		for event := range events {
			if event.Type == im.EventTypeMessageCreated && event.Message != nil && event.Message.SenderID == im.AdminUserID {
				firstSource <- event
				return
			}
		}
	}()
	firstResult := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstResult <- performAgentSessionRequest(t, handler, "agent-alpha", "busy-session", map[string]any{"input": "wait"})
	}()
	firstEvent := <-firstSource

	overlap := performAgentSessionRequest(t, handler, "agent-alpha", "busy-session", map[string]any{"input": "overlap"})
	if overlap.Code != http.StatusConflict || !strings.Contains(overlap.Body.String(), "session_busy") {
		t.Fatalf("overlap status = %d, body=%s", overlap.Code, overlap.Body.String())
	}

	otherEvents, cancelOther := bus.Subscribe()
	defer cancelOther()
	go replyToNextAdminMessage(otherEvents, imSvc, "user-alpha", "other result")
	other := performAgentSessionRequest(t, handler, "agent-alpha", "parallel-session", map[string]any{"input": "parallel"})
	if other.Code != http.StatusOK {
		t.Fatalf("parallel status = %d, body=%s", other.Code, other.Body.String())
	}

	_, err := imSvc.DeliverMessage(im.DeliverMessageRequest{
		RoomID: firstEvent.RoomID, SenderID: "user-alpha", Content: "first result",
		Metadata: map[string]any{"codex": map[string]any{
			"delivery_kind": "final", "request_id": firstEvent.Message.ID, "source_message_id": firstEvent.Message.ID,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-firstResult:
		if result.Code != http.StatusOK {
			t.Fatalf("first status = %d, body=%s", result.Code, result.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first session response did not finish")
	}
}

func TestAgentSessionResponsesReturnsOpenAIStyleValidationErrors(t *testing.T) {
	handler, _, _ := newAgentSessionTestHandler(t, []agent.Agent{completeWorkerAgent("agent-alpha", "Alpha")})
	tests := []struct {
		name      string
		sessionID string
		body      map[string]any
		code      string
	}{
		{name: "invalid session", sessionID: "bad!session", body: map[string]any{"input": "hello"}, code: "invalid_session_id"},
		{name: "unknown field", sessionID: "valid", body: map[string]any{"input": "hello", "unknown": true}, code: "invalid_request"},
		{name: "assistant role", sessionID: "valid", body: map[string]any{"input": []map[string]any{{"role": "assistant", "content": "hello"}}}, code: "invalid_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := performAgentSessionRequest(t, handler, "agent-alpha", test.sessionID, test.body)
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestAgentSessionResponsesStreamsOpenAIResponseEvents(t *testing.T) {
	handler, imSvc, bus := newAgentSessionTestHandler(t, []agent.Agent{
		completeWorkerAgent("agent-alpha", "Alpha"),
	})
	events, cancel := bus.Subscribe()
	defer cancel()
	go replyToNextAdminMessage(events, imSvc, "user-alpha", "streamed answer")

	recorder := performAgentSessionRequest(t, handler, "agent-alpha", "stream-session", map[string]any{
		"input":  "Stream this",
		"stream": true,
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	var eventTypes []string
	var sequenceNumbers []int
	for _, block := range strings.Split(strings.TrimSpace(recorder.Body.String()), "\n\n") {
		lines := strings.Split(block, "\n")
		if len(lines) != 2 || !strings.HasPrefix(lines[0], "event: ") || !strings.HasPrefix(lines[1], "data: ") {
			t.Fatalf("invalid SSE block %q", block)
		}
		eventType := strings.TrimPrefix(lines[0], "event: ")
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "data: ")), &payload); err != nil {
			t.Fatalf("decode SSE payload: %v", err)
		}
		if payload["type"] != eventType {
			t.Fatalf("payload type = %v, event = %q", payload["type"], eventType)
		}
		eventTypes = append(eventTypes, eventType)
		sequenceNumbers = append(sequenceNumbers, int(payload["sequence_number"].(float64)))
	}
	wantEvents := []string{
		"response.created",
		"response.in_progress",
		"response.output_item.added",
		"response.content_part.added",
		"response.output_text.delta",
		"response.output_text.done",
		"response.content_part.done",
		"response.output_item.done",
		"response.completed",
	}
	if strings.Join(eventTypes, ",") != strings.Join(wantEvents, ",") {
		t.Fatalf("event types = %v, want %v", eventTypes, wantEvents)
	}
	for index, sequence := range sequenceNumbers {
		if sequence != index {
			t.Fatalf("sequence_numbers = %v, want monotonically increasing from zero", sequenceNumbers)
		}
	}
	if !strings.Contains(recorder.Body.String(), `"delta":"streamed answer"`) {
		t.Fatalf("SSE body = %q, want output text delta", recorder.Body.String())
	}
	if strings.Count(recorder.Body.String(), "streamed answer") != 1 {
		t.Fatalf("SSE body = %q, want full text only in delta event", recorder.Body.String())
	}
}

func TestAgentSessionResponsesStreamsCodexRuntimeDeltasAsTheyArrive(t *testing.T) {
	codexAgent := completeWorkerAgent("agent-alpha", "Alpha")
	codexAgent.RuntimeKind = agent.RuntimeKindCodex
	codexAgent.RuntimeID = "rt-agent-alpha"
	handler, _, _ := newAgentSessionTestHandler(t, []agent.Agent{codexAgent})
	source := newFakeSessionEventSource("codex-thread-1")
	handler.SetSessionEventSource(source)
	promptCalled := make(chan struct{}, 1)
	source.prompt = func(_ context.Context, runtimeID, sessionID, prompt string) error {
		promptCalled <- struct{}{}
		source.mu.Lock()
		subscriberCount := len(source.subscribers)
		source.mu.Unlock()
		if subscriberCount == 0 {
			t.Error("Prompt called before Subscribe")
		}
		if runtimeID != "rt-agent-alpha" || sessionID != "codex-thread-1" || prompt != "Stream this" {
			t.Errorf("Prompt(%q, %q, %q), want rt-agent-alpha/codex-thread-1/Stream this", runtimeID, sessionID, prompt)
		}
		source.publish(activity.RuntimeEvent{
			RuntimeID: "rt-agent-alpha", SessionID: "codex-thread-1",
			Kind: activity.RuntimeEventTextDelta, MessageID: "codex-msg-1", Text: "hello",
			Payload: map[string]any{"phase": "final_answer"},
		})
		source.publish(activity.RuntimeEvent{
			RuntimeID: "rt-agent-alpha", SessionID: "codex-thread-1",
			Kind: activity.RuntimeEventTextDelta, MessageID: "codex-msg-1", Text: " world",
			Payload: map[string]any{"phase": "final_answer"},
		})
		source.publish(activity.RuntimeEvent{
			RuntimeID: "rt-agent-alpha", SessionID: "codex-thread-1",
			Kind: activity.RuntimeEventPromptCompleted,
		})
		return nil
	}

	recorder := performAgentSessionRequest(t, handler, "agent-alpha", "codex-stream", map[string]any{
		"input": "Stream this", "stream": true,
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	select {
	case <-promptCalled:
	default:
		t.Fatal("direct Codex source Prompt was not called")
	}
	body := recorder.Body.String()
	if strings.Count(body, "event: response.output_text.delta") != 2 ||
		!strings.Contains(body, `"delta":"hello"`) ||
		!strings.Contains(body, `"delta":" world"`) {
		t.Fatalf("SSE body = %q, want two source text deltas", body)
	}
	if source.conversationKey != recorderRoomID(t, body) {
		t.Fatalf("EnsureSession conversation key = %q, want response room id", source.conversationKey)
	}
	messages, err := handler.im.ListMessages(source.conversationKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) < 3 || messages[len(messages)-1].SenderID != "user-alpha" || messages[len(messages)-1].Content != "hello world" {
		t.Fatalf("messages = %#v, want direct Codex final audit message", messages)
	}
}

func TestAgentSessionResponsesWaitsForCodexPromptCompletion(t *testing.T) {
	codexAgent := completeWorkerAgent("agent-alpha", "Alpha")
	codexAgent.RuntimeKind = agent.RuntimeKindCodex
	codexAgent.RuntimeID = "rt-agent-alpha"
	handler, _, _ := newAgentSessionTestHandler(t, []agent.Agent{codexAgent})
	source := newFakeSessionEventSource("codex-thread-1")
	handler.SetSessionEventSource(source)
	source.prompt = func(_ context.Context, runtimeID, sessionID, prompt string) error {
		source.publish(activity.RuntimeEvent{
			RuntimeID: runtimeID, SessionID: sessionID,
			Kind: activity.RuntimeEventTextDelta, MessageID: "codex-msg-1", Text: "tail",
			Payload: map[string]any{"phase": "final_answer"},
		})
		time.Sleep(30 * time.Millisecond)
		source.publish(activity.RuntimeEvent{
			RuntimeID: runtimeID, SessionID: sessionID,
			Kind: activity.RuntimeEventTextDelta, MessageID: "codex-msg-1", Text: " end",
			Payload: map[string]any{"phase": "final_answer"},
		})
		source.publish(activity.RuntimeEvent{
			RuntimeID: runtimeID, SessionID: sessionID,
			Kind: activity.RuntimeEventPromptCompleted,
		})
		return nil
	}

	startedAt := time.Now()
	recorder := performAgentSessionRequest(t, handler, "agent-alpha", "codex-tail", map[string]any{
		"input": "Stream this", "stream": true,
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if time.Since(startedAt) < 30*time.Millisecond {
		t.Fatal("request completed before Codex prompt completion")
	}
	if !strings.Contains(body, `"delta":"tail"`) ||
		!strings.Contains(body, `"delta":" end"`) ||
		!strings.Contains(body, "event: response.completed") {
		t.Fatalf("SSE body = %q, want all deltas before completion", body)
	}
}

type fakeSessionEventSource struct {
	sessionID       string
	conversationKey string
	prompt          func(context.Context, string, string, string) error

	mu          sync.Mutex
	subscribers []chan activity.RuntimeEvent
}

func newFakeSessionEventSource(sessionID string) *fakeSessionEventSource {
	return &fakeSessionEventSource{sessionID: sessionID}
}

func (s *fakeSessionEventSource) EnsureSession(_ context.Context, _, conversationKey string) (string, error) {
	s.conversationKey = conversationKey
	return s.sessionID, nil
}

func (s *fakeSessionEventSource) Prompt(ctx context.Context, runtimeID, sessionID, prompt string) error {
	if s.prompt != nil {
		return s.prompt(ctx, runtimeID, sessionID, prompt)
	}
	return nil
}

func (s *fakeSessionEventSource) Subscribe(string) (<-chan activity.RuntimeEvent, func()) {
	ch := make(chan activity.RuntimeEvent, 8)
	s.mu.Lock()
	s.subscribers = append(s.subscribers, ch)
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		for i, candidate := range s.subscribers {
			if candidate == ch {
				s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		close(ch)
	}
}

func (s *fakeSessionEventSource) publish(event activity.RuntimeEvent) {
	s.mu.Lock()
	subscribers := append([]chan activity.RuntimeEvent(nil), s.subscribers...)
	s.mu.Unlock()
	for _, ch := range subscribers {
		ch <- event
	}
}

func recorderRoomID(t *testing.T, body string) string {
	t.Helper()
	for _, block := range strings.Split(strings.TrimSpace(body), "\n\n") {
		lines := strings.Split(block, "\n")
		if len(lines) != 2 || lines[0] != "event: response.created" {
			continue
		}
		var payload struct {
			Response agentSessionResponse `json:"response"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "data: ")), &payload); err != nil {
			t.Fatal(err)
		}
		return payload.Response.Metadata["room_id"]
	}
	t.Fatal("response.created event not found")
	return ""
}

func newAgentSessionTestHandler(t *testing.T, agents []agent.Agent) (*Handler, *im.Service, *im.Bus) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	statePath := filepath.Join(t.TempDir(), "agents.json")
	data, err := json.Marshal(map[string]any{"agents": agents})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	agentSvc, err := agent.NewService(
		config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath,
		agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindPicoClawSandbox}),
		agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}

	users := []im.User{{ID: im.AdminUserID, Name: "admin", Role: "admin"}}
	participants := make([]apitypes.Participant, 0, len(agents))
	for _, item := range agents {
		suffix := strings.TrimPrefix(item.ID, agent.AgentIDPrefix)
		userID := "user-" + suffix
		participantID := "pt-" + suffix
		users = append(users, im.User{ID: userID, Name: item.Name, Role: item.Role})
		participants = append(participants, apitypes.Participant{
			ID: participantID, Channel: participant.ChannelCSGClaw, Type: participant.TypeAgent,
			Name: item.Name, ChannelUserRef: userID, ChannelUserKind: participant.ChannelUserKindLocalUserID,
			AgentID: item.ID, LifecycleStatus: participant.LifecycleStatusActive, Mentionable: true,
		})
	}
	bus := im.NewBus()
	imSvc := im.NewServiceFromBootstrapWithBus(im.Bootstrap{CurrentUserID: im.AdminUserID, Users: users}, bus)
	participantSvc := participant.NewService(participant.NewMemoryStore(participants))
	handler := NewHandler(agentSvc, imSvc, bus, im.NewParticipantBridge(""), nil, nil)
	handler.SetParticipantService(participantSvc)
	return handler, imSvc, bus
}

func performAgentSessionRequest(t *testing.T, handler *Handler, agentSelector, sessionID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/agents/" + agentSelector + "/sessions/" + sessionID + "/responses"
	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload)))
	return recorder
}

func replyToNextAdminMessage(events <-chan im.Event, imSvc *im.Service, senderID, response string) {
	for event := range events {
		if event.Type != im.EventTypeMessageCreated || event.Message == nil || event.Message.SenderID != im.AdminUserID {
			continue
		}
		_, _ = imSvc.DeliverMessage(im.DeliverMessageRequest{
			RoomID: event.RoomID, SenderID: senderID, Content: response,
			Metadata: map[string]any{"codex": map[string]any{
				"delivery_kind": "final", "request_id": event.Message.ID, "source_message_id": event.Message.ID,
			}},
		})
		return
	}
}
