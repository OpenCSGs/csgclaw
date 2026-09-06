package interactionstate

import (
	"context"
	"testing"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine/contract"
)

func TestSuccessfulTurnCompletionPreservesNativeResponseInFlight(t *testing.T) {
	var coordinator Coordinator
	nativeAccepted, release := make(chan struct{}), make(chan struct{})
	coordinator.Register("agent", "conversation", "turn", contract.InteractionRequest{
		ID: "question", Kind: contract.InteractionUserInput,
		Payload: activity.UserInputSnapshot{ID: "question", Status: activity.UserInputStatusPending, Questions: []activity.UserInputQuestionSnapshot{{ID: "q", Header: "Q", Question: "Continue?", IsOther: true}}},
	}, func(context.Context, contract.InteractionRequest, contract.InteractionResolution) *contract.TurnError {
		close(nativeAccepted)
		<-release
		return nil
	}, nil)
	done := make(chan error, 1)
	go func() {
		done <- coordinator.Resolve(context.Background(), "agent", contract.InteractionResolution{ConversationKey: "conversation", InteractionID: "question", Answers: map[string]contract.InteractionAnswer{"q": {Values: []string{"user_note: yes"}}}})
	}()
	<-nativeAccepted
	coordinator.CompleteTurn("agent", "conversation", "turn")
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	item, err := coordinator.Get("agent", "conversation", "question")
	if err != nil || item.Payload.(activity.UserInputSnapshot).Status != activity.UserInputStatusAnswered {
		t.Fatalf("completed native answer=%+v %v", item, err)
	}
}
