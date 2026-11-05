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
						pending := coordinator.Pending("agent", "conversation")
						if len(pending) != 1 || pending[0].Payload.(activity.UserInputSnapshot).Status != activity.UserInputStatusSubmitting {
							t.Errorf("in-flight answer should remain visible past its deadline: %+v", pending)
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

func TestDetachedSessionFormLifetime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var coordinator Coordinator
		args := activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{ID: "q", Header: "Q", Question: "Continue?"}}}
		roomNotifications := 0
		room, err := coordinator.CreateDetached("agent", "room", "turn", args, contract.InteractionResolve, func(contract.InteractionRequest) { roomNotifications++ })
		if err != nil || room.Payload.(activity.UserInputSnapshot).AutoResolveAt != nil {
			t.Fatalf("unexpected room deadline: %+v, %v", room, err)
		}
		request, err := coordinator.CreateDetached("agent", "session", "turn", args, contract.InteractionUserInputOnly, func(contract.InteractionRequest) {
			t.Error("session retained the completed turn's event callback")
		})
		if err != nil {
			t.Fatal(err)
		}
		snapshot := request.Payload.(activity.UserInputSnapshot)
		if snapshot.AutoResolveAt == nil || snapshot.AutoResolveAt.Sub(snapshot.RequestedAt) != 24*time.Hour {
			t.Fatalf("unexpected session deadline: %+v", snapshot)
		}
		explicitMS := uint64(time.Minute / time.Millisecond)
		args.AutoResolutionMS = &explicitMS
		explicit, err := coordinator.CreateDetached("agent", "another-session", "turn", args, contract.InteractionUserInputOnly, nil)
		if err != nil {
			t.Fatal(err)
		}
		short := explicit.Payload.(activity.UserInputSnapshot)
		if short.AutoResolveAt == nil || short.AutoResolveAt.Sub(short.RequestedAt) != time.Minute {
			t.Fatalf("unexpected explicit deadline: %+v", short)
		}
		time.Sleep(24*time.Hour + terminalRetention)
		synctest.Wait()
		// Inspect storage before another request can trigger lazy pruning.
		if len(coordinator.items) != 1 || len(coordinator.Pending("agent", "room")) != 1 {
			t.Fatalf("expected only the room form after session deadlines; retained %d records", len(coordinator.items))
		}
		coordinator.Interrupt("agent", "room", "", true)
		if roomNotifications != 1 {
			t.Fatalf("room received %d terminal notifications", roomNotifications)
		}
	})
}
