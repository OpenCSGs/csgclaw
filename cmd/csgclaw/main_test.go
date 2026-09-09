package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunUsesCancelableContext(t *testing.T) {
	called := false
	var runCtx context.Context
	err := executeWithSignalContext([]string{"serve"}, func(ctx context.Context, _ []string) error {
		called = true
		runCtx = ctx
		if ctx == context.Background() {
			t.Fatal("executeWithSignalContext() passed context.Background(), want signal-aware context")
		}

		cancelable := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				close(cancelable)
			case <-time.After(10 * time.Millisecond):
			}
		}()

		select {
		case <-cancelable:
			t.Fatal("context should not be canceled before executeWithSignalContext() returns")
		case <-time.After(20 * time.Millisecond):
		}
		return errors.New("stop")
	})
	if !called {
		t.Fatal("execFn was not called")
	}
	if runCtx.Err() != context.Canceled {
		t.Fatal("command context was not canceled after executeWithSignalContext() returned")
	}
	if err == nil || err.Error() != "stop" {
		t.Fatalf("executeWithSignalContext() error = %v, want stop", err)
	}
}
