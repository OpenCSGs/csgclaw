package interaction

import (
	"context"
	"testing"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/delivery"
	"csgclaw/internal/im"
	runtimecodex "csgclaw/internal/runtime/codex"
)

type staticParticipants struct{}

func (staticParticipants) Get(_ string, id string) (apitypes.Participant, bool) {
	return apitypes.Participant{ID: id, ChannelUserRef: "user-" + id}, true
}

type staticAgents struct {
	agent agentengine.Agent
}

func (a staticAgents) Get(context.Context, string) (agentengine.Agent, error) {
	return a.agent, nil
}

type staticSessions struct {
	runtimeID       string
	conversationKey string
	sessionID       string
}

func (s *staticSessions) ExistingEngineSession(_ context.Context, runtimeID, conversationKey string) (string, bool, error) {
	s.runtimeID = runtimeID
	s.conversationKey = conversationKey
	return s.sessionID, true, nil
}

func TestCoordinatorDetachedRequestUsesRuntimeSessionCancellationIdentity(t *testing.T) {
	broker := runtimecodex.NewUserInputBroker(nil)
	store, err := delivery.NewIMTranscriptStore(im.NewService(), staticParticipants{})
	if err != nil {
		t.Fatalf("NewIMTranscriptStore() error = %v", err)
	}
	sessions := &staticSessions{sessionID: "session-1"}
	coordinator := NewCoordinator(
		broker,
		staticParticipants{},
		store,
		WithRuntimeIdentity(staticAgents{agent: agentengine.Agent{
			ID:     "agent-1",
			Status: agentengine.AgentStatus{RuntimeID: "runtime-1"},
		}}, sessions),
	)

	snapshot, err := coordinator.Activate(context.Background(), channel.TurnContext{
		BindingID:       "binding-1",
		ParticipantID:   "participant-1",
		AgentID:         "agent-1",
		RoomID:          "room-1",
		SourceMessageID: "message-1",
		ConversationKey: "conversation-1",
		TurnID:          "turn-1",
	}, activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{
		ID: "choice", Header: "Choice", Question: "Continue?",
	}}})
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if sessions.runtimeID != "runtime-1" || sessions.conversationKey != "conversation-1" {
		t.Fatalf("session lookup = (%q, %q), want (runtime-1, conversation-1)", sessions.runtimeID, sessions.conversationKey)
	}

	broker.CancelSession("binding-1", "conversation-1")
	if pending, ok := broker.Get(snapshot.ID); !ok || pending.Status != activity.UserInputStatusPending {
		t.Fatalf("request after channel-identity cancellation = (%q, %v), want pending", pending.Status, ok)
	}

	broker.CancelSession("runtime-1", "session-1")
	resolved, ok := broker.Get(snapshot.ID)
	if !ok || resolved.Status != activity.UserInputStatusInterrupted {
		t.Fatalf("request after runtime/session cancellation = (%q, %v), want interrupted", resolved.Status, ok)
	}
}
