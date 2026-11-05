package codex

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"csgclaw/internal/activity"
)

func TestLateNativeCompletionCannotInterruptNextTurn(t *testing.T) {
	events, inputEvents := &recordingSink{}, &recordingSink{}
	input := NewUserInputBroker(inputEvents)
	deps := testAppServerManagerDepsWithSink(events)
	deps.UserInput = input
	manager := newAppServerManager(deps)
	live := &liveSession{}
	previous, err := live.registerAppServerTurnWaiter("thread")
	if err != nil {
		t.Fatal(err)
	}
	previous.setTurnID("previous")
	complete := func(id, status string, version *uint64) bool {
		t.Helper()
		return manager.handleRawTurnCompleted("rt", live, "thread",
			map[string]any{"turn": map[string]any{"id": id, "status": status}}, version)
	}
	version := previous.progressVersion()
	if !complete("previous", "interrupted", &version) {
		t.Fatal("historical terminal state ignored")
	}
	live.removeAppServerTurnWaiter("thread", previous)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	decisions := make(chan UserInputDecision, 1)
	go func() {
		decision, _ := input.Request(ctx, PendingUserInputRequest{
			Execution: activity.ExecutionRef{RuntimeID: "rt", SessionID: "thread", TurnID: "next"},
			Questions: []activity.UserInputQuestionSnapshot{{ID: "q", Header: "Q", Question: "Continue?"}},
		})
		decisions <- decision
	}()
	requestID := waitForUserInputRequest(t, inputEvents)
	if complete("previous", "interrupted", nil) || len(events.snapshot()) != 0 {
		t.Fatal("late terminal event reached subscribers before next waiter registration")
	}
	next, err := live.registerAppServerTurnWaiter("thread")
	if err != nil {
		t.Fatal(err)
	}
	if complete("previous", "interrupted", nil) {
		t.Fatal("late terminal event reached unbound waiter")
	}
	next.setTurnID("next")
	if complete("previous", "interrupted", nil) || complete("unrelated", "interrupted", nil) {
		t.Fatal("another turn affected bound waiter")
	}
	// A cancellation by native ID may race with registering the next question.
	input.CancelTurn("rt", "thread", "previous")
	if _, done := next.terminalResult(); done {
		t.Fatal("next turn already has terminal result")
	}
	if question, ok := input.Get(requestID); !ok || question.Status != activity.UserInputStatusPending {
		t.Fatalf("next question = %+v, present=%v", question, ok)
	}
	if complete("", "interrupted", nil) {
		t.Fatal("unidentified interruption consumed the new turn")
	}
	if question, ok := input.Get(requestID); !ok || question.Status != activity.UserInputStatusPending {
		t.Fatalf("unidentified interruption canceled the next question: %+v", question)
	}
	if !complete("next", "interrupted", nil) {
		t.Fatal("identified interruption for the current turn was ignored")
	}
	select {
	case decision := <-decisions:
		if decision.Snapshot.Status != activity.UserInputStatusInterrupted {
			t.Fatalf("question status = %s", decision.Snapshot.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("new turn's own interruption did not cancel its question")
	}
}

func TestNativeCompletionResolvesOnlyItsThread(t *testing.T) {
	live := &liveSession{session: &Session{SessionID: "primary"}}
	if got := live.nativeNotificationThread("", "", false); got != "primary" {
		t.Fatalf("single session = %q", got)
	}
	for _, id := range []string{"left", "right"} {
		waiter, err := live.registerAppServerTurnWaiter(id)
		if err != nil {
			t.Fatal(err)
		}
		waiter.setTurnID("turn-" + id)
	}
	live.trackAppServerTurn("right", "known")
	for _, tc := range []struct{ thread, turn, want string }{
		{"left", "", "left"}, {"", "turn-right", "right"}, {"", "known", "right"},
		{"left", "known", ""}, {"", "untracked", ""}, {"", "", ""},
	} {
		if got := live.nativeNotificationThread(tc.thread, tc.turn, false); got != tc.want {
			t.Errorf("thread(%q, %q) = %q, want %q", tc.thread, tc.turn, got, tc.want)
		}
	}
}

func TestLateRecoveredTurnNotificationsDoNotReachAnotherTurn(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)
	const threadID = "main-thread"
	old, err := live.registerAppServerTurnWaiter(threadID)
	if err != nil {
		t.Fatal(err)
	}
	old.setTurnID("old")
	version := old.progressVersion()
	if !manager.handleRawTurnCompleted("rt", live, threadID, map[string]any{"turnId": "old", "status": "completed"}, &version) {
		t.Fatal("failed to recover previous turn")
	}
	live.removeAppServerTurnWaiter(threadID, old)
	late := func(turnID string) {
		t.Helper()
		for _, note := range []struct {
			method string
			params map[string]any
		}{
			{"turn/started", map[string]any{}},
			{"item/agentMessage/delta", map[string]any{"itemId": "msg", "delta": "late text"}},
			{"item/started", map[string]any{"item": map[string]any{"id": "cmd", "type": "commandExecution", "command": "echo old"}}},
			{"item/commandExecution/outputDelta", map[string]any{"itemId": "cmd", "delta": "late stdout"}},
			{"item/completed", map[string]any{"item": map[string]any{"id": "cmd", "type": "commandExecution", "status": "completed"}}},
			{"error", map[string]any{"willRetry": false, "error": map[string]any{"message": "late error"}}},
			{"turn/completed", map[string]any{"status": "failed"}},
			{"codex/response_item", map[string]any{"type": "message", "role": "assistant", "id": "msg", "content": []any{map[string]any{"type": "output_text", "text": "late rollout"}}}},
		} {
			note.params["threadId"] = threadID
			if turnID != "" {
				note.params["turnId"] = turnID
			}
			manager.handleAppServerNotification("rt", live, appServerNotification{Method: note.method, Params: mustJSONRaw(t, note.params)})
		}
		if events := sink.snapshot(); len(events) != 0 || len(live.commandOutputs) != 0 {
			t.Fatalf("late events %v, command outputs %v", events, live.commandOutputs)
		}
	}
	late("old")
	late("")
	next, err := live.registerAppServerTurnWaiter(threadID)
	if err != nil {
		t.Fatal(err)
	}
	late("old")
	late("")
	if id := next.currentTurnID(); id != "" {
		t.Fatalf("unbound waiter claimed by %q", id)
	}
	next.setTurnID("current")
	late("old")
	late("")
	if _, done := next.terminalResult(); done || next.progressVersion() != 0 {
		t.Fatal("old events changed the new waiter")
	}
	manager.handleAppServerNotification("rt", live, appServerNotification{Method: "item/agentMessage/delta",
		Params: mustJSONRaw(t, map[string]any{"threadId": threadID, "turnId": "current", "itemId": "msg-current", "delta": "current text"})})
	if events := sink.snapshot(); len(events) != 1 || events[0].Text != "current text" {
		t.Fatalf("current events = %+v", events)
	}
	manager.handleLegacyAppServerEvent("rt", live, map[string]any{"type": "task_complete", "turn_id": "old"})
	if _, done := next.terminalResult(); done {
		t.Fatal("old rollout completed the current turn")
	}
	manager.handleLegacyAppServerEvent("rt", live, map[string]any{"type": "task_complete", "turn_id": "current"})
	if result, done := next.terminalResult(); !done || !result.success || result.turnID != "current" {
		t.Fatalf("current rollout completion = %+v, done=%v", result, done)
	}
}

func TestUnidentifiedCompletionAfterRecoveryWaitsForNativeTurn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := startQuietTurn(t)
		defer close(f.live.done)
		f.live.mu.Lock()
		f.live.trackAppServerTurnLocked("thread", "previous", true)
		f.live.mu.Unlock()
		f.manager.handleAppServerNotification("rt", f.live, appServerNotification{Method: "turn/completed", Params: mustJSONRaw(t, map[string]any{"threadId": "thread", "status": "failed"})})
		f.requireWaiting(t)
		time.Sleep(quietTurnInterval)
		f.reply(t, "thread/read", idleThread)
		f.reply(t, "thread/turns/list", `{"data":[{"id":"turn","status":"completed","completedAt":11,"itemsView":"full","items":[]}]}`)
		synctest.Wait()
		select {
		case <-f.done:
			if f.err != nil || f.response.StopReason != StopReasonEndTurn {
				t.Fatalf("response=%+v error=%v", f.response, f.err)
			}
		default:
			t.Fatal("verified current turn remained pending")
		}
	})
}

func TestUnidentifiedCompletionWithoutRecoveryUsesBoundTurn(t *testing.T) {
	manager, live, _ := testAppServerEventAdapter(t)
	waiter, err := live.registerAppServerTurnWaiter("main-thread")
	if err != nil {
		t.Fatal(err)
	}
	waiter.setTurnID("proper")
	// History recovery of another thread does not affect this thread's protocol.
	live.mu.Lock()
	live.trackAppServerTurnLocked("other-thread", "previous", true)
	live.mu.Unlock()
	manager.handleAppServerNotification("rt", live, appServerNotification{Method: "turn/completed", Params: mustJSONRaw(t, map[string]any{"threadId": "main-thread", "status": "interrupted"})})
	if result, done := waiter.terminalResult(); !done || result.turnID != "proper" || result.err == nil {
		t.Fatalf("legacy terminal=%+v done=%v", result, done)
	}
}
