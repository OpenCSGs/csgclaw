package interactionstate

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

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

func TestDetachedAnswerDeadlineDuringDispatch(t *testing.T) {
	for _, tc := range []struct {
		name      string
		delivered bool
		status    activity.UserInputStatus
	}{
		{"delivered", true, activity.UserInputStatusAnswered},
		{"failed_before_dispatch", false, activity.UserInputStatusExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var coordinator Coordinator
				deadline := time.Now().Add(time.Minute)
				coordinator.Register("agent", "conversation", "turn", contract.InteractionRequest{
					ID: "question", Kind: contract.InteractionUserInput, Detached: true,
					Payload: activity.UserInputSnapshot{
						ID: "question", Status: activity.UserInputStatusPending, AutoResolveAt: &deadline,
						Questions: []activity.UserInputQuestionSnapshot{{ID: "q", Header: "Q", Question: "Continue?", IsOther: true}},
					},
				}, nil, nil)
				err := coordinator.Resolve(t.Context(), "agent", contract.InteractionResolution{
					ConversationKey: "conversation", InteractionID: "question",
					Answers: map[string]contract.InteractionAnswer{"q": {Values: []string{"user_note: yes"}}},
					ContinueDetached: func(ctx context.Context, _ contract.InteractionRequest) bool {
						time.Sleep(2 * time.Minute)
						synctest.Wait()
						if ctx.Err() != nil {
							t.Errorf("answer submitted before deadline was canceled: %v", ctx.Err())
						}
						return tc.delivered
					},
				})
				if (err == nil) != tc.delivered {
					t.Fatalf("delivered=%v, resolve error=%v", tc.delivered, err)
				}
				item, err := coordinator.Get("agent", "conversation", "question")
				if err != nil || item.Payload.(activity.UserInputSnapshot).Status != tc.status {
					t.Fatalf("want %s after deadline, got %+v, %v", tc.status, item, err)
				}
			})
		})
	}
}
