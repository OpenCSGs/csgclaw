//go:build !windows

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestSecondInterruptStopsBlockedShutdown(t *testing.T) {
	if os.Getenv("CSGCLAW_TEST_BLOCKED_SHUTDOWN") == "1" {
		_ = executeWithSignalContext(nil, func(ctx context.Context, _ []string) error {
			fmt.Println("ready")
			<-ctx.Done()
			fmt.Println("stopping")
			select {}
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSecondInterruptStopsBlockedShutdown$")
	cmd.Env = append(os.Environ(), "CSGCLAW_TEST_BLOCKED_SHUTDOWN=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	scanner := bufio.NewScanner(stdout)
	for _, want := range []string{"ready", "stopping"} {
		if !scanner.Scan() || scanner.Text() != want {
			t.Fatalf("subprocess output = %q, want %q; error: %v", scanner.Text(), want, scanner.Err())
		}
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			t.Fatal(err)
		}
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		t.Fatal("second Ctrl+C did not terminate blocked shutdown")
	}
	if err == nil {
		t.Fatal("expected the second Ctrl+C to terminate the process by signal")
	}
	status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGINT {
		t.Fatalf("process exit = %v, want SIGINT", cmd.ProcessState)
	}
}
