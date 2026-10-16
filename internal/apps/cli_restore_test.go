package apps

import (
	"context"
	"errors"
	"testing"
	"time"
)

type restoreRuntime struct {
	*cliRuntimeStub
	entered chan struct{}
	release chan struct{}
}

func (r *restoreRuntime) Configure(ctx context.Context, agentID, resourceID string, p *CLIProjection) error {
	close(r.entered)
	select {
	case <-r.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return r.cliRuntimeStub.Configure(ctx, agentID, resourceID, p)
}

func TestRestoreAllWaitsForPendingCLIConfiguration(t *testing.T) {
	for _, outcome := range []string{"success", "failure", "cancel"} {
		t.Run(outcome, func(t *testing.T) {
			s, fixture, id := newCLIFixture(t)
			if op := waitCLIOperation(t, s, id, startCLITest(t, s, id, "connect", "connect")); op.Status != "completed" {
				t.Fatal(op)
			}
			binding, err := s.Bind(context.Background(), "worker", BindRequest{ResourceID: id})
			if err != nil {
				t.Fatal(err)
			}
			waitCLIBinding(t, s, binding.InstallationID, "worker")
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s.mu.Lock()
			entry := s.entries[binding.InstallationID]
			entry.record.CLIApply.Pending = true
			err = s.persistLocked(binding.InstallationID, &entry.record)
			s.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			rt := &restoreRuntime{cliRuntimeStub: fixture.runtime, entered: make(chan struct{}), release: make(chan struct{})}
			if outcome == "failure" {
				rt.fail = "configure:worker"
			}
			opts := s.options
			opts.Runtime = rt
			restored, err := NewService(s.path, opts)
			if err != nil {
				t.Fatal(err)
			}
			defer restored.Close()
			released := false
			defer func() {
				if !released {
					close(rt.release)
				}
			}()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- restored.RestoreAll(ctx) }()
			select {
			case <-rt.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("configuration did not begin")
			}
			select {
			case err := <-done:
				t.Fatalf("restore returned before configuration completed: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
			if outcome == "cancel" {
				cancel()
				select {
				case err := <-done:
					if !errors.Is(err, context.Canceled) {
						t.Fatal(err)
					}
				case <-time.After(time.Second):
					t.Fatal("restore ignored cancellation")
				}
			}
			close(rt.release)
			released = true
			if outcome == "cancel" {
				go func() { done <- restored.RestoreAll(context.Background()) }()
			}
			select {
			case err := <-done:
				if (err != nil) != (outcome == "failure") {
					t.Fatalf("restore after %s: %v", outcome, err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("restore did not finish")
			}
			err = restored.CheckCLIReady("worker")
			if (err != nil) != (outcome == "failure") {
				t.Fatalf("worker readiness after %s: %v", outcome, err)
			}
			rt.mu.Lock()
			running := rt.running["worker"]
			configured := rt.configured["worker"]
			rt.mu.Unlock()
			if running {
				t.Fatal("restore started a stopped worker")
			}
			if outcome != "failure" && configured == nil {
				t.Fatal("worker configuration missing")
			}
		})
	}
}
