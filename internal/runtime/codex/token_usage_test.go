package codex

import (
	"csgclaw/internal/activity"
	"csgclaw/internal/modelusage"
	agentruntime "csgclaw/internal/runtime"
	"testing"
)

func TestMissingUsageIsStickyAndScopedToExactNativeTurn(t *testing.T) {
	sink := &recordingSink{}
	m := newAppServerManager(managerDeps{EventSink: sink})
	live := &liveSession{spec: SessionSpec{RuntimeID: "r", Profile: agentruntime.Profile{ModelID: "auto"}}}
	waiter, _ := live.registerAppServerTurnWaiter("s")
	m.observeModelResponseUsage(live, "other-session", "t1")
	m.observeModelResponseUsage(live, "s", "stale")
	m.observeModelResponseUsage(live, "s", "t1") // Response precedes turn/start reply.
	waiter.setTurnID("t1")
	counters := func(i, o int64) map[string]any {
		return map[string]any{"inputTokens": i, "outputTokens": o, "totalTokens": i + o}
	}
	params := map[string]any{"turnId": "t1", "tokenUsage": map[string]any{"total": counters(100, 10), "last": counters(100, 10)}}
	m.publishContextUsage("r", live, "s", params, "")
	if !waiter.tokenUsage.Known || !waiter.tokenUsage.Partial || waiter.tokenUsage.Total != 110 {
		t.Fatalf("usage=%+v", waiter.tokenUsage)
	}
	params["tokenUsage"] = map[string]any{"total": counters(200, 20), "last": counters(100, 10)}
	m.publishContextUsage("r", live, "s", params, "")
	if !waiter.tokenUsage.Partial || waiter.tokenUsage.Total != 220 {
		t.Fatalf("later valid usage cleared partial=%+v", waiter.tokenUsage)
	}
	live.removeAppServerTurnWaiter("s", waiter)
	waiter, _ = live.registerAppServerTurnWaiter("s")
	waiter.setTurnID("t2")
	m.observeModelResponseUsage(live, "s", "t1") // Late completion from previous turn.
	m.publishTokenUsage("r", live, "s", "t2", nil, "snapshot")
	if waiter.tokenUsage.Partial {
		t.Fatal("missing flag leaked into next turn")
	}
	m.observeModelResponseUsage(live, "s", "t2")
	if waiter.tokenUsage.Known || !waiter.tokenUsage.Partial {
		t.Fatalf("unknown usage=%+v", waiter.tokenUsage)
	}
}

func TestTokenUsageSumsRequestsNotContextAndResetsPerTurn(t *testing.T) {
	sink := &recordingSink{}
	m := newAppServerManager(managerDeps{EventSink: sink})
	live := &liveSession{spec: SessionSpec{Profile: agentruntime.Profile{ModelID: "auto"}}}
	waiter, _ := live.registerAppServerTurnWaiter("s")
	waiter.setTurnID("t1")
	emit := func(turn string, input, output, lastIn, lastOut int64) {
		counters := func(i, o int64) map[string]any {
			return map[string]any{"inputTokens": i, "outputTokens": o, "totalTokens": i + o}
		}
		m.publishContextUsage("r", live, "s", map[string]any{"turnId": turn, "tokenUsage": map[string]any{"total": counters(input, output), "last": counters(lastIn, lastOut)}}, "")
	}
	// Cold restored thread: its previous 900 input/90 output belong to history.
	emit("t1", 1000, 100, 100, 10)
	emit("t1", 1200, 120, 200, 20)
	emit("t1", 1200, 120, 200, 20) // Duplicate cumulative notification.
	emit("stale", 9000, 900, 7800, 780)
	if got := waiter.tokenUsage; got.Input != 300 || got.Output != 30 || got.Total != 330 || got.ModelID != "auto" {
		t.Fatalf("first=%+v", got)
	}
	live.removeAppServerTurnWaiter("s", waiter)
	waiter, _ = live.registerAppServerTurnWaiter("s")
	waiter.setTurnID("t2")
	emit("t2", 1250, 125, 50, 5)
	if got := waiter.tokenUsage; got.Input != 50 || got.Output != 5 || got.Total != 55 {
		t.Fatalf("second=%+v", got)
	}
	found := false
	for _, e := range sink.snapshot() {
		if e.Kind == activity.RuntimeEventTokenUsage {
			u, ok := e.Payload.(modelusage.Usage)
			if !ok || !u.Valid() {
				t.Fatalf("event=%+v", e)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("no usage events")
	}
}
