package diagnostics

import (
	"context"
	"sync"
	"testing"
)

func TestDeferredNativeFailureBindingRaceDoesNotLoseEvidence(t *testing.T) {
	store := New("")
	record := store.Begin("room", "source", "", "agent", "turn")
	record.Running()
	ctx := WithNativeLookup(context.Background(), store, "agent", "session", "native")
	var group sync.WaitGroup
	group.Add(1)
	go func() { defer group.Done(); record.RuntimeRef("session", "native", "") }()
	for i := 0; i < 32; i++ {
		group.Add(1)
		go func(i int) { defer group.Done(); Capture(ctx, "llm.ingress", map[string]int{"index": i}) }(i)
	}
	group.Wait()
	record.RuntimeRef("session", "native", "")
	count := 0
	for _, entry := range contextEntries(t, store, record.Snapshot().ID) {
		if entry.Kind == "llm.ingress" {
			count++
		}
	}
	if count != 32 {
		t.Fatalf("lost early ingress evidence: %d", count)
	}
}
