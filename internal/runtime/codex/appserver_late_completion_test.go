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

func TestRecoveredTurnStillAllowsResultsForCurrentItems(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)
	const thread = "main-thread"
	waiter, err := live.registerAppServerTurnWaiter(thread)
	if err != nil {
		t.Fatal(err)
	}
	waiter.setTurnID("old")
	version := waiter.progressVersion()
	if !manager.handleRawTurnCompleted("rt", live, thread, map[string]any{"turnId": "old", "status": "completed"}, &version) {
		t.Fatal("expected historical completion")
	}
	live.removeAppServerTurnWaiter(thread, waiter)
	first, err := live.registerAppServerTurnWaiter(thread)
	if err != nil {
		t.Fatal(err)
	}
	first.setTurnID("current")
	second, err := live.registerAppServerTurnWaiter("other")
	if err != nil {
		t.Fatal(err)
	}
	second.setTurnID("other-current")

	note := func(method string, params map[string]any) {
		manager.handleAppServerNotification("rt", live, appServerNotification{Method: method, Params: mustJSONRaw(t, params)})
	}
	for _, call := range []struct{ thread, turn, id string }{{thread, "current", "cmd"}, {"other", "other-current", "other-cmd"}, {thread, "current", "shared"}, {"other", "other-current", "shared"}} {
		note("item/started", map[string]any{"threadId": call.thread, "turnId": call.turn, "item": map[string]any{"id": call.id, "type": "commandExecution", "command": "pwd"}})
	}
	end := func(threadID, id string) {
		note("item/completed", map[string]any{"threadId": threadID, "item": map[string]any{"id": id, "type": "commandExecution", "status": "completed", "aggregatedOutput": "/work"}})
	}
	before := len(sink.snapshot())
	end("", "shared") // Same item ID in two active threads.
	end(thread, "unknown")
	if len(sink.snapshot()) != before {
		t.Fatal("unclaimed tool result escaped its thread")
	}
	end("", "cmd")
	end("other", "shared")
	events := sink.snapshot()[before:]
	if len(events) != 2 || events[0].Kind != SessionEventToolCallUpdate || events[0].SessionID != thread || events[0].ToolCallID != "cmd" || events[1].SessionID != "other" || events[1].ToolCallID != "shared" {
		t.Fatalf("tool results: %+v", events)
	}
	manager.handleLegacyResponseItemEvent("rt", live, map[string]any{"type": "function_call", "threadId": "other", "turn_id": "other-current", "call_id": "legacy-new", "name": "update_plan", "arguments": "{}"})
	manager.handleLegacyResponseItemEvent("rt", live, map[string]any{"type": "function_call_output", "call_id": "legacy-new", "output": "Plan updated"})
	events = sink.snapshot()
	if last := events[len(events)-1]; last.Kind != SessionEventToolCallUpdate || last.SessionID != "other" || last.ToolCallID != "legacy-new" || last.ToolTitle != "update_plan" {
		t.Fatalf("legacy result routed to wrong thread: %+v", last)
	}

	live.removeAppServerTurnWaiter(thread, first)
	before = len(sink.snapshot())
	manager.handleLegacyResponseItemEvent("rt", live, map[string]any{"type": "message", "id": "unscoped", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "unknown owner"}}})
	if len(sink.snapshot()) != before {
		t.Fatal("unscoped legacy message was assigned to the remaining foreign thread")
	}
}

func TestLegacyContentAfterIdentifiedSuccessor(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)
	const thread = "main-thread"
	register := func(id string) *appServerTurnWaiter {
		t.Helper()
		waiter, err := live.registerAppServerTurnWaiter(thread)
		if err != nil {
			t.Fatal(err)
		}
		waiter.setTurnID(id)
		live.trackAppServerTurn(thread, id)
		return waiter
	}
	first := register("first")
	version := first.progressVersion()
	if !manager.handleRawTurnCompleted("rt", live, thread, map[string]any{"turn": map[string]any{"id": "first", "status": "completed", "items": []any{map[string]any{"id": "old-message", "type": "agentMessage"}}}}, &version) {
		t.Fatal("expected recovery")
	}
	live.removeAppServerTurnWaiter(thread, first)
	second := register("second")
	if !manager.handleRawTurnCompleted("rt", live, thread, map[string]any{"turnId": "second", "status": "completed"}, nil) {
		t.Fatal("normal successor did not complete")
	}
	live.removeAppServerTurnWaiter(thread, second)
	third := register("third")
	message := func(id, text string) {
		manager.handleLegacyResponseItemEvent("rt", live, map[string]any{"type": "message", "id": id, "role": "assistant", "phase": "final_answer", "content": []any{map[string]any{"type": "output_text", "text": text}}})
	}
	message("old-message", "late")
	message("", "unidentified")
	manager.handleLegacyAppServerEvent("rt", live, map[string]any{"type": "turn_aborted", "turn_id": "first"})
	message("new-message", "Third answer")
	events := sink.snapshot()
	if len(events) != 1 || events[0].Kind != SessionEventTextDelta || events[0].Text != "Third answer" {
		t.Fatalf("third turn content: %+v", events)
	}
	if _, done := third.terminalResult(); done {
		t.Fatal("legacy content without a turn ID ended the current turn")
	}
	manager.handleLegacyAppServerEvent("rt", live, map[string]any{"type": "task_complete", "turn_id": "third"})
	if result, done := third.terminalResult(); !done || !result.success || result.turnID != "third" {
		t.Fatalf("third terminal: %+v done=%v", result, done)
	}
}

func TestIdentifiedSuccessorRecoversOnlyItsOwnItems(t *testing.T) {
	for _, tc := range []struct {
		name, terminal, history, want string
	}{
		{"legacy", "legacy", "current", "Next answer"},
		{"native", "native", "current", "Next answer"},
		{"native with items", "inline", "", "Next answer"},
		{"reused legacy item ID", "native", "reused", "Next answer"},
		{"contradictory inline turn", "mismatched", "current", "Next answer"},
		{"different turn", "native", "previous", ""},
		{"unavailable history", "legacy", "timeout", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := startQuietTurn(t)
				defer close(f.live.done)
				// The preceding turn was recovered before its final wire items
				// arrived. A rollout message ID can differ from its native item ID.
				f.live.mu.Lock()
				f.live.trackAppServerTurnLocked("thread", "old", true)
				if tc.history == "reused" {
					old := f.live.turnThreads["old"]
					old.recoveredItems = map[string]struct{}{"wire-1": {}, "wire-2": {}}
					f.live.turnThreads["old"] = old
				}
				f.live.mu.Unlock()
				f.live.appClient.onNotification = func(note appServerNotification) {
					f.manager.handleAppServerNotification("rt", f.live, note)
				}
				f.live.appClient.handleLine(`{"type":"response_item","payload":{"type":"message","id":"wire-1","role":"assistant","phase":"final_answer","content":[{"type":"output_text","text":"Previous answer"}]}}`)
				f.live.appClient.handleLine(`{"type":"response_item","payload":{"type":"message","id":"wire-2","role":"assistant","phase":"final_answer","content":[{"type":"output_text","text":"Next answer"}]}}`)
				f.requireWaiting(t)

				const items = `"itemsView":"full","items":[{"id":"native-2","type":"agentMessage","text":"Next answer","phase":"final_answer"}]`
				switch tc.terminal {
				case "legacy":
					f.live.appClient.handleLine(`{"type":"event_msg","payload":{"type":"task_complete","turn_id":"turn"}}`)
				case "native":
					f.live.appClient.handleLine(`{"jsonrpc":"2.0","method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed"}}}`)
				case "inline":
					f.live.appClient.handleLine(`{"jsonrpc":"2.0","method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed",` + items + `}}}`)
				case "mismatched":
					f.live.appClient.handleLine(`{"jsonrpc":"2.0","method":"turn/completed","params":{"threadId":"thread","turnId":"turn","turn":{"id":"old","status":"completed","itemsView":"full","items":[{"type":"agentMessage","id":"old-item","text":"Wrong turn"}]}}}`)
				}
				switch tc.history {
				case "current", "reused":
					f.reply(t, "thread/turns/list", `{"data":[{"id":"turn","status":"completed",`+items+`}]}`)
				case "previous":
					f.reply(t, "thread/turns/list", `{"data":[{"id":"old","status":"completed",`+items+`}]}`)
				case "timeout":
					f.receive(t, "thread/turns/list")
					time.Sleep(5 * time.Second)
				}
				synctest.Wait()
				select {
				case <-f.done:
				default:
					t.Fatal("identified terminal kept waiting")
				}
				var output conversationOutput
				if err := output.restore(t.Context(), nil, f.response.history); err != nil {
					t.Fatal(err)
				}
				if f.err != nil || f.response.StopReason != StopReasonEndTurn || output.String() != tc.want {
					t.Fatalf("result=%v error=%v output=%q want=%q", f.response, f.err, output.String(), tc.want)
				}
			})
		})
	}
}
