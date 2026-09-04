package interaction

import (
	"context"
	"errors"
	"strings"
	"testing"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/agentengine/enginetest"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
)

type staticParticipants struct{}

func (staticParticipants) Get(_ string, id string) (apitypes.Participant, bool) {
	return apitypes.Participant{ID: id, ChannelUserRef: "user-" + id}, true
}

func detachedFixture(t *testing.T) (*Coordinator, agentengine.Interface, channel.TurnContext, agentengine.InteractionRequest) {
	t.Helper()
	engine := enginetest.NewMemoryClient(agentengine.Agent{ID: "agent-1", Spec: agentengine.AgentSpec{Name: "worker", Runtime: agentengine.RuntimeSpec{Adapter: "codex"}}, Status: agentengine.AgentStatus{State: agentengine.AgentStateRunning, Ready: true}})
	engine.SetTurnBehavior(func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		err := sink.Emit(ctx, agentengine.TurnEvent{Kind: agentengine.TurnEventOutputItem, Output: &agentengine.OutputItem{Kind: agentengine.OutputItemRequestUserInput, Payload: activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{ID: "choice", Header: "Choice", Question: "Continue?", IsOther: true}}}}})
		if err != nil {
			t.Fatal(err)
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
	})
	turn := channel.TurnContext{AgentID: "agent-1", BindingID: "binding-1", ParticipantID: "participant-1", RoomID: "room-1", SourceMessageID: "message-1", ConversationKey: "conversation-1", TurnID: "turn-1"}
	result := engine.Conversations(turn.AgentID).Run(context.Background(), agentengine.TurnRequest{ID: turn.TurnID, ConversationKey: turn.ConversationKey, Interaction: agentengine.InteractionResolve, Input: []agentengine.InputPart{{Kind: agentengine.InputPartText, Text: "question"}}}, nil)
	if len(result.Interactions) != 1 {
		t.Fatalf("Run=%+v, want Engine-owned detached question", result)
	}
	coordinator := NewCoordinator(engine, staticParticipants{})
	request, err := coordinator.Bind(turn, result.Interactions[0])
	if err != nil {
		t.Fatal(err)
	}
	return coordinator, engine, turn, request
}

func TestDetachedAnswerResolvesThroughEngine(t *testing.T) {
	coordinator, engine, turn, request := detachedFixture(t)
	calls := 0
	response := activity.UserInputResponseRequest{Channel: "csgclaw", RoomID: turn.RoomID, ActivityID: request.ID, ResponderID: "user-admin", Response: activity.RequestUserInputResponse{Answers: map[string]activity.RequestUserInputAnswer{"choice": {Answers: []string{"user_note: continue"}}}}, RecordTranscript: func(_ context.Context, snapshot activity.UserInputSnapshot) error {
		calls++
		if snapshot.RoomID != turn.RoomID || snapshot.RequesterID != "user-participant-1" {
			t.Fatalf("unbound transcript: %+v", snapshot)
		}
		return nil
	}}
	snapshot, err := coordinator.Respond(context.Background(), response)
	if err != nil || snapshot.Status != activity.UserInputStatusAnswered {
		t.Fatalf("Respond=(%+v,%v)", snapshot, err)
	}
	stored, err := engine.Conversations(turn.AgentID).GetInteraction(context.Background(), turn.ConversationKey, request.ID)
	if err != nil || stored.Payload.(activity.UserInputSnapshot).Status != activity.UserInputStatusAnswered {
		t.Fatalf("Engine did not receive resolution: %+v %v", stored, err)
	}
	if _, err := coordinator.Respond(context.Background(), response); !errors.Is(err, activity.ErrUserInputAlreadyResolved) {
		t.Fatalf("duplicate=%v", err)
	}
	if calls != 1 {
		t.Fatalf("transcript calls=%d", calls)
	}
}

func TestDetachedTranscriptFailureLeavesEngineRequestPending(t *testing.T) {
	coordinator, engine, turn, request := detachedFixture(t)
	_, err := coordinator.Respond(context.Background(), activity.UserInputResponseRequest{Channel: "csgclaw", RoomID: turn.RoomID, ActivityID: request.ID, ResponderID: "user-admin", Response: activity.RequestUserInputResponse{Answers: map[string]activity.RequestUserInputAnswer{"choice": {Answers: []string{"user_note: continue"}}}}, RecordTranscript: func(context.Context, activity.UserInputSnapshot) error { return errors.New("store unavailable") }})
	if err == nil || !strings.Contains(err.Error(), "store unavailable") {
		t.Fatalf("error=%v", err)
	}
	item, _ := engine.Conversations(turn.AgentID).GetInteraction(context.Background(), turn.ConversationKey, request.ID)
	if item.Payload.(activity.UserInputSnapshot).Status != activity.UserInputStatusPending {
		t.Fatalf("not pending: %+v", item)
	}
}
