package feedback

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestAttemptsReserveOnlyOneConcurrentSubmission(t *testing.T) {
	store := Attempts{Path: filepath.Join(t.TempDir(), "feedback-submissions.json")}
	const id = "0123456789abcdef0123456789abcdef"
	var reservations atomic.Int32
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			attempt, reserved, err := store.Begin("https://example.test", "uuid:account", id)
			if err != nil || attempt.State != AttemptUnknown {
				t.Errorf("attempt = %+v, err = %v", attempt, err)
			}
			if reserved {
				reservations.Add(1)
			}
		})
	}
	workers.Wait()
	if count := reservations.Load(); count != 1 {
		t.Fatalf("got %d concurrent reservations, want 1", count)
	}
}
