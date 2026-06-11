package codex

import (
	"sync"
	"testing"
	"time"
)

type recordingSink struct {
	mu     sync.Mutex
	events []SessionEvent
}

func (s *recordingSink) Publish(event SessionEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *recordingSink) Subscribe(string) (<-chan SessionEvent, func()) {
	ch := make(chan SessionEvent)
	close(ch)
	return ch, func() {}
}

func (s *recordingSink) snapshot() []SessionEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SessionEvent, len(s.events))
	copy(out, s.events)
	return out
}

func waitForRuntime(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for runtime condition")
}
