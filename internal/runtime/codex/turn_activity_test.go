package codex

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"csgclaw/internal/activity"
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

func TestAppServerPublishesReasoningItemLifecycle(t *testing.T) {
	for _, optIn := range []bool{false, true} {
		sink := &recordingSink{}
		manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
		live := &liveSession{spec: SessionSpec{RuntimeID: "runtime-1", Profile: testProfile()}, session: &Session{SessionID: "thread-1"}}
		manager.sessions["runtime-1"] = live
		waiter, err := live.registerAppServerTurnWaiter("thread-1")
		if err != nil {
			t.Fatal(err)
		}
		waiter.setTurnID("turn-1")
		waiter.streamReasoningSummary = optIn
		notify := func(turnID, itemID, method string) {
			t.Helper()
			params := map[string]any{"threadId": "thread-1", "turnId": turnID, "itemId": itemID}
			if method == "item/started" || method == "item/completed" {
				params["item"] = map[string]any{"id": itemID, "type": "reasoning", "summary": []string{"snapshot text"}}
			} else {
				params["delta"] = "reasoning text"
			}
			manager.handleAppServerNotification("runtime-1", live, appServerNotification{Method: method, Params: mustJSONRaw(t, params)})
		}
		notify("previous-turn", "old-item", "item/completed")
		var want []SessionEvent
		for _, itemID := range []string{"item-1", "item-2"} {
			notify("turn-1", itemID, "item/started")
			notify("turn-1", itemID, "item/reasoning/textDelta")
			notify("turn-1", itemID, "item/reasoning/summaryTextDelta")
			notify("turn-1", itemID, "item/completed")
			event := SessionEvent{Kind: activity.RuntimeEventReasoningStarted, TurnID: "turn-1", MessageID: itemID}
			want = append(want, event)
			if optIn {
				event.Kind, event.Text = activity.RuntimeEventReasoningSummary, "reasoning text"
				want = append(want, event)
			}
			event.Kind, event.Text = activity.RuntimeEventReasoningCompleted, ""
			want = append(want, event)
		}
		events := sink.snapshot()
		if len(events) != len(want) {
			t.Fatalf("opt-in=%v: got %d events, want %d: %+v", optIn, len(events), len(want), events)
		}
		for i, event := range events {
			if event.Kind != want[i].Kind || event.TurnID != want[i].TurnID || event.MessageID != want[i].MessageID ||
				event.Text != want[i].Text || event.Payload != nil {
				t.Fatalf("event %d: got %+v, want %+v", i, event, want[i])
			}
		}
	}
}

func TestAppServerCompletesReasoningSummary(t *testing.T) {
	for _, test := range []struct {
		name     string
		deltas   map[int]string
		snapshot []string
		want     []string
	}{
		{name: "snapshot only", snapshot: []string{"First", "Second"}, want: []string{"First\n\nSecond"}},
		{name: "complete prefix and later part", deltas: map[int]string{0: "Fi", 1: "Second"}, snapshot: []string{"First", "Second"}, want: []string{"Fi", "rst\n\nSecond"}},
		{name: "missing earlier part", deltas: map[int]string{1: "Sec"}, snapshot: []string{"First", "Second"}, want: []string{"First\n\nSecond"}},
		{name: "no duplicate", deltas: map[int]string{0: "First"}, snapshot: []string{"First"}, want: []string{"First"}},
		{name: "no snapshot", deltas: map[int]string{0: "First", 1: "Second"}, want: []string{"First", "\n\nSecond"}},
		{name: "divergent snapshot", deltas: map[int]string{0: "First"}, snapshot: []string{"Rewritten", "Second"}, want: []string{"First", "\n\nSecond"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			sink := &recordingSink{}
			manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
			live := &liveSession{spec: SessionSpec{RuntimeID: "runtime", Profile: testProfile()}, session: &Session{SessionID: "thread"}}
			manager.sessions["runtime"] = live
			waiter, err := live.registerAppServerTurnWaiter("thread")
			if err != nil {
				t.Fatal(err)
			}
			waiter.setTurnID("turn")
			waiter.streamReasoningSummary = true

			notify := func(method string, fields map[string]any) {
				t.Helper()
				fields["threadId"], fields["turnId"], fields["itemId"] = "thread", "turn", "reasoning-1"
				manager.handleAppServerNotification("runtime", live, appServerNotification{Method: method, Params: mustJSONRaw(t, fields)})
			}
			notify("item/started", map[string]any{"item": map[string]any{"id": "reasoning-1", "type": "reasoning"}})
			for index := 0; index < 2; index++ {
				if delta, ok := test.deltas[index]; ok {
					notify("item/reasoning/summaryTextDelta", map[string]any{"summaryIndex": index, "delta": delta})
				}
			}
			item := map[string]any{"id": "reasoning-1", "type": "reasoning", "summary": test.snapshot, "content": []string{"raw"}}
			notify("item/completed", map[string]any{"item": item})
			notify("item/completed", map[string]any{"item": item})
			notify("item/reasoning/summaryTextDelta", map[string]any{"summaryIndex": 0, "delta": "late"})

			events := sink.snapshot()
			if len(events) != len(test.want)+2 || events[0].Kind != activity.RuntimeEventReasoningStarted ||
				events[len(events)-1].Kind != activity.RuntimeEventReasoningCompleted {
				t.Fatalf("unexpected lifecycle: %+v", events)
			}
			for index, text := range test.want {
				event := events[index+1]
				if event.Kind != activity.RuntimeEventReasoningSummary || event.MessageID != "reasoning-1" || event.Text != text {
					t.Fatalf("summary %d: got %+v, want %q", index, event, text)
				}
			}
		})
	}
}
