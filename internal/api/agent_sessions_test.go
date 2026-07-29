package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		{name: "streaming", sessionID: "valid", body: map[string]any{"input": "hello", "stream": true}, code: "invalid_request"},
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
