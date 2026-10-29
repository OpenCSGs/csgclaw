package codex

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

// Virtual time crosses the former 5/10/30 minute limits without a slow test.
func TestAppServerTurnHasNoAutomaticTimeLimit(t *testing.T) {
	for _, state := range []string{"first_response", "reasoning", "tool", "compaction", "user_input"} {
		t.Run(state, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				manager := newAppServerManager(testAppServerManagerDeps())
				live := &liveSession{done: make(chan struct{})}
				waiter, err := live.registerAppServerTurnWaiter("thread-1")
				if err != nil {
					t.Fatal(err)
				}
				live.notifyAppServerTurn("thread-1", appServerTurnResult{turnID: "turn-1", activity: "turn:started", started: true})
				switch state {
				case "reasoning", "tool", "compaction":
					live.notifyAppServerTurn("thread-1", appServerTurnResult{turnID: "turn-1", activity: state, progress: true})
				case "user_input":
					live.notifyAppServerTurn("thread-1", appServerTurnResult{turnID: "turn-1", activity: "request_user_input:pending"})
				}
				done := make(chan error, 1)
				go func() { _, err := manager.waitAppServerTurn(context.Background(), live, waiter); done <- err }()
				time.Sleep(24 * time.Hour)
				synctest.Wait()
				select {
				case err := <-done:
					t.Fatalf("turn ended after elapsed time alone: %v", err)
				default:
				}
				live.notifyAppServerTurn("thread-1", appServerTurnResult{turnID: "turn-1", activity: "turn:completed", success: true})
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestAppServerProgressNotificationsRecordActivity(t *testing.T) {
	for _, method := range []string{
		"item/reasoning/textDelta", "item/reasoning/summaryTextDelta", "item/reasoning/summaryPartAdded",
		"item/mcpToolCall/progress", "thread/tokenUsage/updated", "item/started", "item/completed",
	} {
		t.Run(method, func(t *testing.T) {
			manager := newAppServerManager(testAppServerManagerDepsWithSink(&recordingSink{}))
			live := &liveSession{spec: SessionSpec{RuntimeID: "runtime-1", Profile: testProfile()}, session: &Session{SessionID: "thread-1"}, done: make(chan struct{})}
			manager.sessions["runtime-1"] = live
			waiter, err := live.registerAppServerTurnWaiter("thread-1")
			if err != nil {
				t.Fatal(err)
			}
			waiter.setTurnID("turn-1")
			params := map[string]any{"threadId": "thread-1", "turnId": "turn-1", "itemId": "item-1", "delta": "still thinking", "message": "still working", "summaryIndex": 0, "contentIndex": 0}
			if method == "item/started" || method == "item/completed" {
				params["item"] = map[string]any{"id": "item-1", "type": "contextCompaction"}
			}
			// Late events from another turn must not update this turn's activity.
			params["turnId"] = "previous-turn"
			manager.handleAppServerNotification("runtime-1", live, appServerNotification{Method: method, Params: mustJSONRaw(t, params)})
			if got := waiter.currentLastActivity(); got != "turn/start" {
				t.Fatalf("stale activity=%q", got)
			}
			select {
			case result := <-waiter.ch:
				t.Fatalf("stale result=%+v", result)
			default:
			}
			params["turnId"] = "turn-1"
			manager.handleAppServerNotification("runtime-1", live, appServerNotification{Method: method, Params: mustJSONRaw(t, params)})
			select {
			case result := <-waiter.ch:
				if result.turnID != "turn-1" || !result.progress || result.activity == "" || result.success || result.err != nil {
					t.Fatalf("progress=%+v", result)
				}
				if got := waiter.currentLastActivity(); got != result.activity {
					t.Fatalf("recorded activity=%q, event=%q", got, result.activity)
				}
			default:
				t.Fatal("progress notification did not record activity")
			}
			if _, terminal := waiter.terminalResult(); terminal {
				t.Fatal("progress ended the turn")
			}
			if method == "item/started" || method == "item/completed" {
				if got := live.contextUsage["thread-1"].Compacting; got != (method == "item/started") {
					t.Fatalf("compacting=%v", got)
				}
			}
		})
	}
}
