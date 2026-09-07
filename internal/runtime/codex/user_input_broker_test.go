package codex

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/activity"
)

func TestUserInputBrokerRespondsWithCodexLabelsAndNotes(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{}
	broker := NewUserInputBroker(sink)
	resultCh := make(chan UserInputDecision, 1)
	go func() {
		decision, _ := broker.Request(context.Background(), PendingUserInputRequest{
			Execution: activity.ExecutionRef{RuntimeID: "rt-1", SessionID: "session-1", TurnID: "turn-1"},
			Questions: []activity.UserInputQuestionSnapshot{
				{
					ID: "color", Header: "Color", Question: "Choose a color", IsOther: true,
					Options: []activity.UserInputOptionSnapshot{{Label: "Blue"}, {Label: "Green"}},
				},
				{ID: "detail", Header: "Detail", Question: "Add detail"},
			},
		})
		resultCh <- decision
	}()

	requestID := waitForUserInputRequest(t, sink)
	snapshot, err := respondUserInputForTest(broker, context.Background(), activity.UserInputResponseRequest{
		Channel: "csgclaw", ActivityID: requestID, RoomID: "room-1", ResponderID: "user-1",
		Response: activity.RequestUserInputResponse{Answers: map[string]activity.RequestUserInputAnswer{
			"color":  {Answers: []string{"Green", "user_note: darker"}},
			"detail": {Answers: []string{"user_note: matte finish"}},
		}},
	})
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if snapshot.Status != activity.UserInputStatusAnswered || snapshot.ResponderID != "user-1" {
		t.Fatalf("snapshot = %+v, want pt-agent question answered by user-1", snapshot)
	}

	select {
	case decision := <-resultCh:
		got := decision.Response.Answers
		if strings.Join(got["color"].Answers, "|") != "Green|user_note: darker" {
			t.Fatalf("color answers = %#v, want exact option label and user note", got["color"].Answers)
		}
		if strings.Join(got["detail"].Answers, "|") != "user_note: matte finish" {
			t.Fatalf("detail answers = %#v, want user note", got["detail"].Answers)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("broker request did not return")
	}
}

func TestUserInputBrokerSynthesizesOtherAndRedactsSecrets(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{}
	broker := NewUserInputBroker(sink)
	resultCh := make(chan UserInputDecision, 1)
	go func() {
		decision, _ := broker.Request(context.Background(), PendingUserInputRequest{
			Execution: activity.ExecutionRef{RuntimeID: "rt-1", SessionID: "session-1"},
			Questions: []activity.UserInputQuestionSnapshot{{
				ID: "token", Header: "Token", Question: "Provide token", IsOther: true, IsSecret: true,
				Options: []activity.UserInputOptionSnapshot{{Label: "Existing"}},
			}},
		})
		resultCh <- decision
	}()

	requestID := waitForUserInputRequest(t, sink)
	snapshot, err := respondUserInputForTest(broker, context.Background(), activity.UserInputResponseRequest{
		Channel: "csgclaw", ActivityID: requestID, RoomID: "room-1", ResponderID: "user-1",
		Response: activity.RequestUserInputResponse{Answers: map[string]activity.RequestUserInputAnswer{
			"token": {Answers: []string{"None of the above", "user_note: super-secret"}},
		}},
	})
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	answer := snapshot.Answers["token"]
	if answer.Text != "******" || !answer.Secret || !answer.Answered || answer.OptionLabel != "None of the above" {
		t.Fatalf("public secret answer = %+v, want masked Other answer", answer)
	}

	decision := <-resultCh
	if got := strings.Join(decision.Response.Answers["token"].Answers, "|"); got != "None of the above|user_note: super-secret" {
		t.Fatalf("Codex secret response = %q, want raw in-memory response", got)
	}
	for _, event := range sink.snapshot() {
		if snapshot, ok := event.Payload.(activity.UserInputSnapshot); ok && strings.Contains(snapshot.Answers["token"].Text, "super-secret") {
			t.Fatalf("event payload leaked secret: %+v", snapshot)
		}
	}
}

func TestUserInputBrokerFirstResponseWins(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{}
	broker := NewUserInputBroker(sink)
	resultCh := make(chan UserInputDecision, 1)
	go func() {
		decision, _ := broker.Request(context.Background(), PendingUserInputRequest{
			Execution: activity.ExecutionRef{RuntimeID: "rt-1", SessionID: "session-1"},
			Questions: []activity.UserInputQuestionSnapshot{{ID: "q", Header: "Q", Question: "Answer?"}},
		})
		resultCh <- decision
	}()

	requestID := waitForUserInputRequest(t, sink)
	responses := []activity.UserInputResponseRequest{
		{Channel: "csgclaw", ActivityID: requestID, RoomID: "room-1", ResponderID: "user-1", Response: activity.RequestUserInputResponse{Answers: map[string]activity.RequestUserInputAnswer{"q": {Answers: []string{"user_note: first"}}}}},
		{Channel: "csgclaw", ActivityID: requestID, RoomID: "room-1", ResponderID: "user-2", Response: activity.RequestUserInputResponse{Answers: map[string]activity.RequestUserInputAnswer{"q": {Answers: []string{"user_note: second"}}}}},
	}
	errs := make(chan error, len(responses))
	var wg sync.WaitGroup
	for _, response := range responses {
		wg.Add(1)
		go func(response activity.UserInputResponseRequest) {
			defer wg.Done()
			_, err := respondUserInputForTest(broker, context.Background(), response)
			errs <- err
		}(response)
	}
	wg.Wait()
	close(errs)

	var successes, conflicts int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrUserInputAlreadyResolved):
			conflicts++
		default:
			t.Fatalf("concurrent Respond() error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d, want one each", successes, conflicts)
	}
	<-resultCh
}

func TestUserInputBrokerTimeoutAndCancellation(t *testing.T) {
	t.Parallel()

	t.Run("timeout", func(t *testing.T) {
		broker := NewUserInputBroker(nil)
		decision, err := broker.Request(context.Background(), PendingUserInputRequest{
			Questions:   []activity.UserInputQuestionSnapshot{{ID: "q", Header: "Q", Question: "Answer?"}},
			AutoResolve: 20 * time.Millisecond,
		})
		if err != nil || decision.Snapshot.Status != activity.UserInputStatusExpired || len(decision.Response.Answers) != 0 {
			t.Fatalf("timeout decision = %+v, err = %v", decision, err)
		}
	})

	t.Run("session interruption", func(t *testing.T) {
		sink := &recordingSink{}
		broker := NewUserInputBroker(sink)
		resultCh := make(chan UserInputDecision, 1)
		go func() {
			decision, _ := broker.Request(context.Background(), PendingUserInputRequest{
				Execution: activity.ExecutionRef{RuntimeID: "rt-1", SessionID: "session-1"},
				Questions: []activity.UserInputQuestionSnapshot{{ID: "q", Header: "Q", Question: "Answer?"}},
			})
			resultCh <- decision
		}()
		_ = waitForUserInputRequest(t, sink)
		broker.CancelSession("rt-1", "session-1")
		select {
		case decision := <-resultCh:
			if decision.Snapshot.Status != activity.UserInputStatusInterrupted {
				t.Fatalf("status = %s, want interrupted", decision.Snapshot.Status)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("interrupted request did not return")
		}
	})
}

func TestUserInputBrokerRejectsMalformedQuestionsAndAnswers(t *testing.T) {
	t.Parallel()

	broker := NewUserInputBroker(nil)
	tooMany := make([]activity.UserInputQuestionSnapshot, maxStructuredOutputQuestions+1)
	for index := range tooMany {
		id := string(rune('a' + index))
		tooMany[index] = activity.UserInputQuestionSnapshot{ID: id, Header: id, Question: id}
	}
	for name, questions := range map[string][]activity.UserInputQuestionSnapshot{
		"none":       nil,
		"too many":   tooMany,
		"missing id": {{Header: "Q", Question: "Answer?"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := broker.Request(context.Background(), PendingUserInputRequest{Questions: questions}); !errors.Is(err, ErrUserInputInvalidResponse) {
				t.Fatalf("Request() error = %v, want invalid response", err)
			}
		})
	}
}

func waitForUserInputRequest(t *testing.T, sink *recordingSink) string {
	t.Helper()
	var requestID string
	waitForRuntime(t, func() bool {
		for _, event := range sink.snapshot() {
			if event.Kind == activity.RuntimeEventUserInputRequest && event.UserInputID != "" {
				requestID = event.UserInputID
				return true
			}
		}
		return false
	})
	return requestID
}

func respondUserInputForTest(b *MemoryUserInputBroker, ctx context.Context, req activity.UserInputResponseRequest) (activity.UserInputSnapshot, error) {
	return b.RespondDirect(ctx, req.ActivityID, req.ResponderID, req.Response)
}
