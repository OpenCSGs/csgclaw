package codex

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentruntime "csgclaw/internal/runtime"
)

func TestAppServerManagerHTTPDisconnectKeepsRuntime(t *testing.T) {
	withAppServerHelperCommand(t, "prompt-cancel-no-active-turn")
	spec := testAppServerSessionSpec(t.TempDir())
	manager := newAppServerManager(testAppServerManagerDeps())
	session, err := manager.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })
	firstDone := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/first" {
			_, _ = io.WriteString(w, "message_start\n")
			w.(http.Flusher).Flush()
			_, err := manager.Prompt(r.Context(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
				SessionID: session.SessionID, Prompt: []PromptContentBlock{TextBlock("keep working")},
			})
			firstDone <- err
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		response, err := manager.Prompt(ctx, SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
			SessionID: session.SessionID, Prompt: []PromptContentBlock{TextBlock("next request")},
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, string(response.StopReason))
	}))
	t.Cleanup(server.Close)
	client := server.Client()
	response, err := client.Get(server.URL + "/first")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	waitForRuntime(t, func() bool {
		live := manager.liveSession(spec.RuntimeID)
		if live == nil {
			return false
		}
		waiter := live.appServerTurnWaiter(session.SessionID)
		return waiter != nil && waiter.currentTurnID() == "turn-cancel"
	})
	// Close an unfinished streaming HTTP response: net/http cancels the
	// server request context because the client connection has gone away.
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-firstDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("disconnect error=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not observe client disconnect")
	}
	rt := New(Dependencies{Manager: manager})
	info, err := rt.Info(context.Background(), agentruntime.Handle{RuntimeID: spec.RuntimeID})
	if err != nil || info.State != agentruntime.StateRunning || !processAlive(session.ProcessID) {
		t.Fatalf("after disconnect info=%+v err=%v", info, err)
	}
	next, err := client.Get(server.URL + "/next")
	if err != nil {
		t.Fatal(err)
	}
	defer next.Body.Close()
	body, err := io.ReadAll(next.Body)
	if err != nil || next.StatusCode != http.StatusOK || string(body) != string(StopReasonEndTurn) {
		t.Fatalf("next request status=%d body=%s err=%v", next.StatusCode, body, err)
	}
	t.Logf("client disconnected; runtime=%s process_alive=%v next_request_status=%d", info.State, processAlive(session.ProcessID), next.StatusCode)
}

func TestAppServerManagerCancellationInterruptErrorCode(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    string
		stopped bool
	}{
		{"interrupt_succeeds", "prompt-cancel-confirmed", false},
		{"no_active_turn", "prompt-cancel-no-active-turn", false},
		{"invalid_request_other_message", "prompt-cancel-invalid-request", false},
		{"other_code_same_message", "prompt-cancel-interrupt-error", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withAppServerHelperCommand(t, tc.mode)
			dir := t.TempDir()
			spec := testAppServerSessionSpec(dir)
			manager := newAppServerManager(testAppServerManagerDeps())
			session, err := manager.Start(context.Background(), spec)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })
			// Persist running metadata so Info can also observe the dead process.
			if err := writeJSONFile(os.WriteFile, filepath.Join(spec.RuntimeDir, runtimeFileName), sessionToRuntimeMetadata(session)); err != nil {
				t.Fatal(err)
			}
			rt := New(Dependencies{
				Manager: manager,
				ResolveAgent: func(agentruntime.Handle) (AgentRef, error) {
					return AgentRef{ID: spec.AgentID, Name: spec.AgentName}, nil
				},
				AgentHome: func(string) (string, error) { return dir, nil },
				ReadFile:  os.ReadFile, WriteFile: os.WriteFile,
			})
			handle := agentruntime.Handle{RuntimeID: spec.RuntimeID}
			before, err := rt.Info(context.Background(), handle)
			if err != nil || before.State != agentruntime.StateRunning {
				t.Fatalf("before=%+v err=%v", before, err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := manager.Prompt(ctx, SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{SessionID: session.SessionID, Prompt: []PromptContentBlock{TextBlock("keep working")}})
				done <- err
			}()
			waitForRuntime(t, func() bool {
				live := manager.liveSession(spec.RuntimeID)
				if live == nil {
					return false
				}
				waiter := live.appServerTurnWaiter(session.SessionID)
				return waiter != nil && waiter.currentTurnID() == "turn-cancel"
			})
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("Prompt error=%v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cancellation did not finish")
			}
			after, err := rt.Info(context.Background(), handle)
			if err != nil {
				t.Fatal(err)
			}
			if tc.stopped {
				if processAlive(session.ProcessID) || after.State == agentruntime.StateRunning {
					t.Fatalf("runtime survived: %+v", after)
				}
				log, err := os.ReadFile(spec.StderrPath)
				if err != nil || !strings.Contains(string(log), "no active turn to interrupt") {
					t.Fatalf("log=%s err=%v", log, err)
				}
				if !strings.Contains(string(log), "codex app-server process exited") || !strings.Contains(string(log), "exit_code=") {
					t.Fatalf("missing process exit log: %s", log)
				}
			} else if after.State != agentruntime.StateRunning {
				t.Fatalf("healthy interrupt stopped runtime: %+v", after)
			}
			if !tc.stopped && tc.mode != "prompt-cancel-confirmed" {
				log, err := os.ReadFile(spec.StderrPath)
				if err != nil || !strings.Contains(string(log), "preserving runtime") || !strings.Contains(string(log), "rpc_code=-32600") {
					t.Fatalf("missing ignored interrupt log: %s err=%v", log, err)
				}
				live := manager.liveSession(spec.RuntimeID)
				if live == nil || live.appServerTurnWaiter(session.SessionID) != nil {
					t.Fatal("canceled turn waiter was not released")
				}
				nextCtx, cancelNext := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancelNext()
				response, err := manager.Prompt(nextCtx, SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{SessionID: session.SessionID, Prompt: []PromptContentBlock{TextBlock("next request")}})
				if err != nil || response.StopReason != StopReasonEndTurn {
					t.Fatalf("next request response=%+v err=%v", response, err)
				}
			}
			t.Logf("before=%s after=%s process_alive=%v", before.State, after.State, processAlive(session.ProcessID))
		})
	}
}
