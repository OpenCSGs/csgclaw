package codex

import (
	"csgclaw/internal/modelusage"
	agentruntime "csgclaw/internal/runtime"
	"testing"
)

func TestResponseUsageIsScopedToExactNativeTurn(t *testing.T) {
	m := newAppServerManager(managerDeps{EventSink: &recordingSink{}})
	live := &liveSession{spec: SessionSpec{RuntimeID: "r", Profile: agentruntime.Profile{ModelID: "auto"}}}
	waiter, _ := live.registerAppServerTurnWaiter("s")
	known := &modelusage.Usage{Known: true, Input: 100, Output: 10, Total: 110, Cached: 50, Reasoning: 5}
	m.observeModelResponseUsage(live, "other-session", "t1", known)
	m.observeModelResponseUsage(live, "s", "stale", known)
	m.observeModelResponseUsage(live, "s", "t1", nil)
	m.observeModelResponseUsage(live, "s", "t1", known) // May precede turn/start reply.
	waiter.setTurnID("t1")
	m.publishTokenUsage("r", live, "s", "t1", nil, "snapshot")
	if got := waiter.tokenUsage; !got.Known || !got.Partial || got.Total != 110 || got.Cached != 50 || got.Reasoning != 5 || got.ModelID != "auto" {
		t.Fatalf("usage=%+v", got)
	}
	m.observeModelResponseUsage(live, "s", "t1", known)
	if got := waiter.tokenUsage; got.Total != 220 || !got.Partial {
		t.Fatalf("sum=%+v", got)
	}
	live.removeAppServerTurnWaiter("s", waiter)
	waiter, _ = live.registerAppServerTurnWaiter("s")
	waiter.setTurnID("t2")
	m.observeModelResponseUsage(live, "s", "t1", known)
	m.observeModelResponseUsage(live, "s", "t2", nil)
	if got := waiter.tokenUsage; got.Known || !got.Partial || got.Total != 0 {
		t.Fatalf("missing=%+v", got)
	}
}

func TestNativeHistoricalCountersCannotChargeCurrentTurn(t *testing.T) {
	for _, reported := range []bool{false, true} {
		m := newAppServerManager(managerDeps{EventSink: &recordingSink{}})
		live := &liveSession{spec: SessionSpec{Profile: agentruntime.Profile{ModelID: "auto"}}}
		waiter, _ := live.registerAppServerTurnWaiter("s")
		waiter.setTurnID("new")
		m.observeModelResponseUsage(live, "s", "new", nil)
		historical := map[string]any{"inputTokens": 100, "outputTokens": 10, "totalTokens": 110}
		params := map[string]any{"turnId": "new", "tokenUsage": map[string]any{"total": historical, "last": historical}}
		m.publishContextUsage("r", live, "s", params, "")
		if reported {
			m.observeModelResponseUsage(live, "s", "new", &modelusage.Usage{Known: true, Input: 100, Output: 10, Total: 110})
		} else {
			m.observeModelResponseUsage(live, "s", "new", nil)
		}
		m.publishContextUsage("r", live, "s", params, "")
		m.publishContextUsage("r", live, "s", params, "") // Replayed native counters never add usage.
		want := int64(0)
		if reported {
			want = 110
		}
		if got := waiter.tokenUsage; got.Total != want || got.Known != reported || !got.Partial {
			t.Fatalf("reported=%v usage=%+v", reported, got)
		}
	}
}

func TestPendingUsageWaitsForConfirmedTurnAndDiscardsStaleObservations(t *testing.T) {
	for _, known := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing-stale", true: "reported-stale"}[known], func(t *testing.T) {
			sink := &recordingSink{}
			m := newAppServerManager(managerDeps{EventSink: sink})
			live := &liveSession{}
			waiter, _ := live.registerAppServerTurnWaiter("s")
			usage := &modelusage.Usage{Known: true, Input: 100, Output: 10, Total: 110}
			var stale *modelusage.Usage
			if known {
				stale = usage
			}
			m.observeModelResponseUsage(live, "s", "old", stale)
			m.observeModelResponseUsage(live, "s", "new", usage)
			m.publishTokenUsage("r", live, "s", "old", nil, "")
			m.publishTokenUsage("r", live, "s", "new", nil, "")
			if waiter.tokenUsage.Known || waiter.tokenUsage.Partial || len(sink.snapshot()) != 0 {
				t.Fatalf("unconfirmed usage published: %+v", waiter.tokenUsage)
			}
			waiter.setTurnID("new")
			m.publishTokenUsage("r", live, "s", "old", nil, "")
			m.publishTokenUsage("r", live, "s", "new", nil, "snapshot")
			m.publishTokenUsage("r", live, "s", "new", nil, "snapshot")
			if got := waiter.tokenUsage; !got.Known || got.Total != 110 || got.Partial || len(waiter.pendingUsage) != 0 {
				t.Fatalf("usage=%+v pending=%v", got, waiter.pendingUsage)
			}
		})
	}
}
