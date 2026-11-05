package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/processgroup"
)

func TestAppServerStdoutFailureEndsTurnAndReapsWorker(t *testing.T) {
	for _, mode := range []string{"eof", "invalid-json", "completed-then-eof", "completed-then-exit", "exit-73"} {
		t.Run(mode, func(t *testing.T) {
			original := appServerCommandContext
			appServerCommandContext = func(ctx context.Context, _ string, _ []string) (*exec.Cmd, error) {
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAppServerStdoutFailureHelper$", "--", mode), nil
			}
			t.Cleanup(func() { appServerCommandContext = original })

			spec := testAppServerSessionSpec(t.TempDir())
			manager := newAppServerManager(testAppServerManagerDeps())
			if _, err := manager.Start(context.Background(), spec); err != nil {
				t.Fatal(err)
			}
			handle := SessionHandle{RuntimeID: spec.RuntimeID}
			t.Cleanup(func() { _ = manager.Stop(context.Background(), handle) })
			live := manager.liveSession(spec.RuntimeID)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			accepted := make(chan struct{})
			finished := make(chan error, 1)
			go func() {
				_, err := manager.Prompt(ctx, handle, PromptRequest{
					SessionID: "main-thread", Prompt: []PromptContentBlock{TextBlock("test stdout")},
					OnAccepted: func() { close(accepted) },
				})
				finished <- err
			}()
			select {
			case <-accepted:
			case err := <-finished:
				t.Fatalf("prompt ended before acceptance: %v", err)
			case <-ctx.Done():
				t.Fatal("prompt was not accepted")
			}

			_, err := live.appClient.request(ctx, "test/close-stdout", nil)
			wantError := "stdout closed"
			switch mode {
			case "completed-then-exit":
				wantError = "exited with code 0"
			case "exit-73":
				wantError = "exited with code 73"
			}
			if err == nil || !strings.Contains(err.Error(), wantError) {
				t.Fatalf("pending RPC error = %v, want %q", err, wantError)
			}
			select {
			case err := <-finished:
				completed := strings.HasPrefix(mode, "completed-")
				if completed && err != nil {
					t.Fatalf("lost completed turn: %v", err)
				}
				if !completed && (err == nil || ctx.Err() != nil) {
					t.Fatalf("turn did not fail on its own: %v", err)
				}
			case <-ctx.Done():
				t.Fatal("turn kept waiting after stdout failed")
			}
			select {
			case <-live.done:
				if manager.liveSession(spec.RuntimeID) != nil || processAlive(live.cmd.Process.Pid) {
					t.Fatal("unusable worker was not reclaimed")
				}
			case <-ctx.Done():
				t.Fatal("worker remained alive after stdout failed")
			}
		})
	}
}

func TestAppServerStdoutFailureHelper(t *testing.T) {
	args := os.Args
	if len(args) < 3 || args[len(args)-2] != "--" {
		return
	}
	mode := args[len(args)-1]
	if mode == "hold-stdout" {
		time.Sleep(10 * time.Second)
		return
	}
	if mode == "write-stdout" {
		if _, err := fmt.Fprintln(os.Stdout); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile("stdout-writer-ready", nil, 0600); err != nil {
			t.Fatal(err)
		}
		for {
			time.Sleep(15 * time.Millisecond)
			if _, err := fmt.Fprintln(os.Stdout); err != nil {
				return
			}
		}
	}
	runAppServerHelper(t, func(_ int, msg map[string]any) (map[string]any, bool) {
		switch msg["method"] {
		case "thread/start":
			return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
		case "turn/start":
			return rpcResult(msg["id"], map[string]any{"turnId": "turn-1"}), true
		case "test/hold-stdout", "test/write-stdout":
			childMode := strings.TrimPrefix(msg["method"].(string), "test/")
			child := exec.Command(os.Args[0], "-test.run=^TestAppServerStdoutFailureHelper$", "--", childMode)
			child.Stdout = os.Stdout
			if err := processgroup.Start(child); err != nil {
				t.Fatal(err)
			}
			pid := child.Process.Pid
			_ = child.Process.Release()
			return rpcResult(msg["id"], map[string]any{"pid": pid}), true
		case "test/close-stdout":
			switch mode {
			case "exit-73":
				os.Exit(73)
			case "invalid-json":
				_, _ = fmt.Fprintln(os.Stdout, "{invalid")
			case "completed-then-eof", "completed-then-exit":
				writeRPCNotification(t, "turn/completed", map[string]any{
					"threadId": "main-thread", "turn": map[string]any{"id": "turn-1", "status": "completed"},
				})
				if mode == "completed-then-exit" {
					os.Exit(0)
				}
			}
			_ = os.Stdout.Close()
			// Keep the broken worker alive until the parent reclaims it.
			time.Sleep(time.Minute)
		}
		return nil, false
	})
}

func TestAppServerStopWhileDetachedChildHoldsStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows jobs retain descendants; this fixture needs an independent process group")
	}
	for _, mode := range []string{"hold-stdout", "write-stdout"} {
		t.Run(mode, func(t *testing.T) {
			original := appServerCommandContext
			appServerCommandContext = func(ctx context.Context, _ string, _ []string) (*exec.Cmd, error) {
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAppServerStdoutFailureHelper$", "--", "stop"), nil
			}
			t.Cleanup(func() { appServerCommandContext = original })
			spec := testAppServerSessionSpec(t.TempDir())
			manager := newAppServerManager(testAppServerManagerDeps())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := manager.Start(ctx, spec); err != nil {
				t.Fatal(err)
			}
			handle := SessionHandle{RuntimeID: spec.RuntimeID}
			t.Cleanup(func() { _ = manager.Stop(context.Background(), handle) })
			result, err := manager.liveSession(spec.RuntimeID).appClient.request(ctx, "test/"+mode, nil)
			if err != nil {
				t.Fatal(err)
			}
			var child struct{ PID int }
			if err := json.Unmarshal(result, &child); err != nil || child.PID <= 0 {
				t.Fatalf("invalid helper pid: %s, %v", result, err)
			}
			t.Cleanup(func() { _ = processgroup.Stop(child.PID, true) })
			if mode == "write-stdout" {
				waitForRuntime(t, func() bool {
					_, err := os.Stat(filepath.Join(spec.WorkspaceDir, "stdout-writer-ready"))
					return err == nil
				})
			}
			stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
			defer stopCancel()
			if err := manager.Stop(stopCtx, handle); err != nil {
				t.Fatalf("stop with inherited stdout: %v", err)
			}
			log, err := os.ReadFile(spec.StderrPath)
			if err != nil || strings.Contains(string(log), "stdout unavailable") {
				t.Fatalf("unexpected stdout error on stop: %s, %v", log, err)
			}
		})
	}
}

func TestAppServerStdoutDrainsRemainingDataAfterSlowConsumer(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if _, err := io.WriteString(w, "ab"); err != nil {
		t.Fatal(err)
	}
	reader := &drainingAppServerStdout{File: r}
	buf := make([]byte, 1)
	if _, err := io.ReadFull(reader, buf); err != nil || string(buf) != "a" {
		t.Fatalf("first read = %q, %v", buf, err)
	}
	reader.drain()
	time.Sleep(2 * appServerStdoutExitGracePeriod)
	if _, err := io.ReadFull(reader, buf); err != nil || string(buf) != "b" {
		t.Fatalf("buffered data after exit = %q, %v", buf, err)
	}
}
