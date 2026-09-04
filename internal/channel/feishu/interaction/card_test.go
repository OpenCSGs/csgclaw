package interaction

import (
	"context"
	"errors"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
)

type recordingEngine struct {
	agentID      string
	conversation *recordingConversation
}

func (e *recordingEngine) Agents() agentengine.AgentInterface                             { return nil }
func (e *recordingEngine) RuntimeExtensions(string) agentengine.RuntimeExtensionInterface { return nil }

func (e *recordingEngine) Conversations(agentID string) agentengine.ConversationInterface {
	e.agentID = agentID
	return e.conversation
}

type recordingConversation struct {
	cancelKey    agentengine.ConversationKey
	cancelTurnID agentengine.TurnID
	resetKey     agentengine.ConversationKey
	err          error
}

func (*recordingConversation) Files() agentengine.FileInterface {
	return agentengine.NewFileStore().Scope("agent-1")
}

func (*recordingConversation) Run(context.Context, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
	panic("unexpected Run call")
}

func (c *recordingConversation) Cancel(_ context.Context, key agentengine.ConversationKey, turnID agentengine.TurnID) error {
	c.cancelKey = key
	c.cancelTurnID = turnID
	return c.err
}

func (c *recordingConversation) Reset(_ context.Context, key agentengine.ConversationKey) error {
	c.resetKey = key
	return c.err
}

func (*recordingConversation) Resolve(context.Context, agentengine.InteractionResolution) error {
	panic("unexpected Resolve call")
}

func newRecordingHandler(t *testing.T) (*Handler, *recordingEngine, *recordingConversation) {
	t.Helper()
	conversation := &recordingConversation{}
	engine := &recordingEngine{conversation: conversation}
	handler, err := NewHandler(engine)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return handler, engine, conversation
}

func TestHandlerDispatchesCancelUsingTrustedInputIdentity(t *testing.T) {
	t.Parallel()
	handler, engine, conversation := newRecordingHandler(t)

	err := handler.Handle(context.Background(), Input{
		AgentID:         "agent-trusted",
		ConversationKey: "conversation-trusted",
		TurnID:          "turn-trusted",
		Action:          CardAction{Operation: OperationCancel},
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if engine.agentID != "agent-trusted" || conversation.cancelKey != "conversation-trusted" || conversation.cancelTurnID != "turn-trusted" {
		t.Fatalf("cancel target = agent %q conversation %q turn %q", engine.agentID, conversation.cancelKey, conversation.cancelTurnID)
	}
}

func TestHandlerDispatchesResetUsingTrustedInputIdentity(t *testing.T) {
	t.Parallel()
	handler, engine, conversation := newRecordingHandler(t)

	err := handler.Handle(context.Background(), Input{
		AgentID:         "agent-trusted",
		ConversationKey: "conversation-trusted",
		Action:          CardAction{Operation: OperationReset},
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if engine.agentID != "agent-trusted" || conversation.resetKey != "conversation-trusted" {
		t.Fatalf("reset target = agent %q conversation %q", engine.agentID, conversation.resetKey)
	}
}

func TestHandlerRejectsUnknownAndIncompleteActions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input Input
		want  string
	}{
		{name: "agent", input: Input{ConversationKey: "conversation", Action: CardAction{Operation: OperationReset}}, want: "agent_id is required"},
		{name: "conversation", input: Input{AgentID: "agent", Action: CardAction{Operation: OperationReset}}, want: "conversation_key is required"},
		{name: "cancel turn", input: Input{AgentID: "agent", ConversationKey: "conversation", Action: CardAction{Operation: OperationCancel}}, want: "turn_id is required for cancel"},
		{name: "unknown", input: Input{AgentID: "agent", ConversationKey: "conversation", Action: CardAction{Operation: "approve_everything"}}, want: "unsupported operation"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			handler, _, _ := newRecordingHandler(t)
			err := handler.Handle(context.Background(), test.input)
			if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Handle() error = %v, want invalid input containing %q", err, test.want)
			}
		})
	}
}

func TestHandlerPropagatesEngineErrorWithoutFallback(t *testing.T) {
	t.Parallel()
	handler, _, conversation := newRecordingHandler(t)
	wantErr := errors.New("engine unavailable")
	conversation.err = wantErr

	err := handler.Handle(context.Background(), Input{
		AgentID:         "agent",
		ConversationKey: "conversation",
		TurnID:          "turn",
		Action:          CardAction{Operation: OperationCancel},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Handle() error = %v, want %v", err, wantErr)
	}
}

func TestNewHandlerRequiresEngine(t *testing.T) {
	t.Parallel()
	_, err := NewHandler(nil)
	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "agent engine is required") {
		t.Fatalf("NewHandler(nil) error = %v", err)
	}
}

func (*recordingConversation) GetInteraction(context.Context, agentengine.ConversationKey, string) (agentengine.InteractionRequest, error) {
	return agentengine.InteractionRequest{}, &agentengine.TurnError{Code: agentengine.ErrorInteractionNotFound, Message: "no interaction in this test fixture"}
}
