package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMutationDrainsExecutionAndDoesNotBlockOtherAgents(t *testing.T) {
	c := &Coordinator{}
	release, err := c.Execution(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	finish := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- c.Mutate(context.Background(), "a", func(ctx context.Context) error {
			if err := c.Mutate(ctx, "a", func(context.Context) error { return nil }); err != nil {
				return err
			}
			close(entered)
			<-finish
			return nil
		})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Mutate(ctx, "b", func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
		t.Fatal("mutation entered before execution drained")
	case <-time.After(10 * time.Millisecond):
	}
	release()
	release()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("mutation did not enter")
	}
	if _, err := c.Execution(ctx, "a"); err == nil {
		t.Fatal("admission remained open")
	}
	close(finish)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	release, err = c.Execution(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestCanceledMutationWaitAndDrainReopenAdmission(t *testing.T) {
	c := &Coordinator{DrainTimeout: 10 * time.Millisecond}
	release, err := c.Execution(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Mutate(context.Background(), "a", func(context.Context) error { t.Fatal("drain did not time out"); return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("drain error=%v", err)
	}
	release()
	ctx, unlock, err := c.Mutation(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := c.Mutation(waitCtx, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("lock wait=%v", err)
	}
	if err := c.Mutate(ctx, "a", func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	unlock()
	release, err = c.Execution(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestMutationPreparationValidatesBeforeDrainAndRunsForNestedUpdates(t *testing.T) {
	c := &Coordinator{}
	releaseExecution, err := c.Execution(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseExecution()
	invalid := errors.New("invalid stop request")
	if _, _, err := c.MutationPrepare(context.Background(), "a", func(context.Context) error { return invalid }); !errors.Is(err, invalid) {
		t.Fatalf("invalid preparation waited for execution: %v", err)
	}
	admission, err := c.Execution(context.Background(), "a")
	if err != nil {
		t.Fatalf("failed preparation left admission closed: %v", err)
	}
	admission()
	releaseExecution()
	ctx, release, err := c.MutationPrepare(context.Background(), "a", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	prepared := false
	_, nestedRelease, err := c.MutationPrepare(ctx, "a", func(context.Context) error { prepared = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	nestedRelease()
	if !prepared {
		t.Fatal("nested resource update skipped preparation")
	}
}
