package codex

import (
	"csgclaw/internal/diagnostics"
	"fmt"
	"testing"
	"time"
)

func TestDiagnosticCompletionUsesReceiveTimeAndExactTurn(t *testing.T) {
	s := diagnostics.New("")
	r := s.Begin("r", "s", "", "a", "engine-turn")
	r.Running()
	r.RuntimeStart()
	w := &appServerTurnWaiter{diagnostic: r}
	stale := time.Now()
	for i := 0; i < 10; i++ {
		w.recordDiagnosticCompletion(fmt.Sprintf("stale-%d", i), stale)
	}
	w.recordDiagnosticCompletion("previous-turn", stale)
	received := stale.Add(time.Millisecond)
	w.setTurnID("native-turn")
	w.recordDiagnosticCompletion("native-turn", received)

	time.Sleep(10 * time.Millisecond)
	w.setTurnID("native-turn")
	first := *r.Snapshot().RuntimeEndMS
	w.recordDiagnosticCompletion("previous-turn", time.Now())
	w.recordDiagnosticCompletion("native-turn", time.Now())
	r.Finish("succeeded")
	v := r.Snapshot()
	if *v.RuntimeEndMS != first || v.TotalMS-first < 5 {
		t.Fatalf("CSGClaw processing counted as Runtime time: %+v", v)
	}
}

func TestDiagnosticCompletionBeforeStartResponse(t *testing.T) {
	store := diagnostics.New("")
	record := store.Begin("room", "source", "", "agent", "turn")
	record.Running()
	record.RuntimeStart()
	waiter := &appServerTurnWaiter{diagnostic: record}
	waiter.recordDiagnosticCompletion("old", time.Now())
	received := time.Now()
	waiter.recordDiagnosticCompletion("current", received)
	if record.Snapshot().RuntimeEndMS != nil {
		t.Fatal("unbound completion changed diagnostics")
	}
	time.Sleep(5 * time.Millisecond)
	waiter.setTurnID("current")
	record.Finish("succeeded")
	snap := record.Snapshot()
	if snap.RuntimeEndMS == nil || snap.TotalMS-*snap.RuntimeEndMS < 4 {
		t.Fatalf("receive time was not preserved: %+v", snap)
	}
}
