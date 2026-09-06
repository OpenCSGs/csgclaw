package codex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestAppServerManagerWaitsForTurnCompletionBeforeNextPrompt(t *testing.T) {
	manager, spec := startLifecycleAppServer(t, "terminal-boundary")
	sink := &recordingSink{}
	manager.deps.EventSink = sink
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		for index := 1; index <= 2; index++ {
			_, err := manager.Prompt(ctx, SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
				SessionID: "main-thread", Prompt: []PromptContentBlock{TextBlock(fmt.Sprintf("question %d", index))},
			})
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	waitForRuntime(t, func() bool { return len(sink.snapshot()) > 0 })
	select {
	case err := <-done:
		t.Fatalf("next Prompt was dispatched before native completion: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	live := manager.liveSession(spec.RuntimeID)
	if _, err := live.appClient.request(ctx, "test/complete-turn", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("sequential Prompts: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Prompts did not finish after native completion")
	}
	events := sink.snapshot()
	var outputs []string
	for _, event := range events {
		if event.Kind == SessionEventTextDelta {
			outputs = append(outputs, event.Text)
		}
	}
	if strings.Join(outputs, ",") != "answer-1,answer-2" {
		t.Fatalf("consecutive prompt output = %v", outputs)
	}
}

func TestAppServerManagerPromptIgnoresStaleCompletionBeforeStartResponse(t *testing.T) {
	for _, previousStatus := range []string{"completed", "failed"} {
		t.Run(previousStatus, func(t *testing.T) {
			manager, spec := startLifecycleAppServer(t, "stale-"+previousStatus)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			request := PromptRequest{SessionID: "main-thread", Prompt: []PromptContentBlock{TextBlock("first question")}}
			if _, err := manager.Prompt(ctx, SessionHandle{RuntimeID: spec.RuntimeID}, request); err != nil {
				t.Fatalf("first Prompt: %v", err)
			}
			request.Prompt = []PromptContentBlock{TextBlock("second question")}
			response, err := manager.Prompt(ctx, SessionHandle{RuntimeID: spec.RuntimeID}, request)
			if previousStatus == "completed" {
				if err == nil || !strings.Contains(err.Error(), "new turn failed") {
					t.Fatalf("new failed turn used previous completion: response=%+v err=%v", response, err)
				}
			} else if err != nil || response.StopReason != StopReasonEndTurn {
				t.Fatalf("new successful turn used previous failure: response=%+v err=%v", response, err)
			}
		})
	}
}

func TestAppServerManagerPromptRetainsTerminalAfterProgressBurst(t *testing.T) {
	for _, status := range []string{"completed", "failed"} {
		t.Run(status, func(t *testing.T) {
			manager, spec := startLifecycleAppServer(t, "burst-"+status)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			response, err := manager.Prompt(ctx, SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
				SessionID: "main-thread", Prompt: []PromptContentBlock{TextBlock("run a verbose command")},
			})
			if status == "failed" {
				if err == nil || !strings.Contains(err.Error(), "new turn failed") {
					t.Fatalf("lost terminal failure after progress burst: response=%+v err=%v", response, err)
				}
			} else if err != nil || response.StopReason != StopReasonEndTurn {
				t.Fatalf("lost terminal completion after progress burst: response=%+v err=%v", response, err)
			}
		})
	}
}

func startLifecycleAppServer(t *testing.T, mode string) (*appServerManager, SessionSpec) {
	t.Helper()
	original := appServerCommandContext
	appServerCommandContext = func(ctx context.Context, _ string, _ []string) (*exec.Cmd, error) {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAppServerLifecycleHelperProcess$", "--", mode), nil
	}
	t.Cleanup(func() { appServerCommandContext = original })
	spec := testAppServerSessionSpec(t.TempDir())
	manager := newAppServerManager(testAppServerManagerDeps())
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })
	return manager, spec
}

// This subprocess exercises the real JSON-RPC reader and Prompt lifecycle.
// It emits notifications before returning turn/start, so the waiter cannot
// consume the burst until every notification has been dispatched.
func TestAppServerLifecycleHelperProcess(t *testing.T) {
	args := os.Args
	if len(args) < 3 || args[len(args)-2] != "--" {
		return
	}
	mode := args[len(args)-1]
	turns := 0
	active := false
	complete := func(turnID, status string) {
		turn := map[string]any{"id": turnID, "status": status}
		if status == "failed" {
			turn["error"] = map[string]any{"message": "new turn failed"}
		}
		writeRPCNotification(t, "turn/completed", map[string]any{"threadId": "main-thread", "turn": turn})
	}
	runAppServerHelper(t, func(_ int, msg map[string]any) (map[string]any, bool) {
		switch msg["method"] {
		case "thread/start":
			return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
		case "turn/interrupt":
			complete("turn-new", "interrupted")
			return rpcResult(msg["id"], map[string]any{}), true
		case "test/complete-turn":
			active = false
			complete("turn-1", "completed")
			return rpcResult(msg["id"], map[string]any{}), true
		case "turn/start":
			turns++
			if mode == "terminal-boundary" {
				if active {
					return rpcError(msg["id"], -32000, "previous turn is still active; the next input would steer it"), true
				}
				active = true
				turnID := fmt.Sprintf("turn-%d", turns)
				writeRPCNotification(t, "turn/started", map[string]any{"threadId": "main-thread", "turnId": turnID})
				writeRPCNotification(t, "item/completed", map[string]any{
					"threadId": "main-thread", "turnId": turnID,
					"item": map[string]any{"id": fmt.Sprintf("answer-%d", turns), "type": "agentMessage", "phase": "final_answer", "text": fmt.Sprintf("answer-%d", turns)},
				})
				writeRPCNotification(t, "thread/status/changed", map[string]any{"threadId": "main-thread", "status": map[string]any{"type": "idle"}})
				if turns == 2 {
					active = false
					complete(turnID, "completed")
				}
				return rpcResult(msg["id"], map[string]any{"turnId": turnID}), true
			}
			if strings.HasPrefix(mode, "stale-") && turns == 1 {
				writeRPCNotification(t, "turn/started", map[string]any{"threadId": "main-thread", "turnId": "turn-old"})
				writeRPCNotification(t, "item/completed", map[string]any{
					"threadId": "main-thread", "turnId": "turn-old",
					"item": map[string]any{"id": "old-answer", "type": "agentMessage", "phase": "final_answer", "text": "old response"},
				})
				complete("turn-old", "completed")
				return rpcResult(msg["id"], map[string]any{"turnId": "turn-old"}), true
			}
			if strings.HasPrefix(mode, "stale-") {
				complete("turn-old", strings.TrimPrefix(mode, "stale-"))
			}
			writeRPCNotification(t, "turn/started", map[string]any{"threadId": "main-thread", "turnId": "turn-new"})
			if strings.HasPrefix(mode, "burst-") {
				for i := 0; i < 16; i++ {
					writeRPCNotification(t, "item/commandExecution/outputDelta", map[string]any{
						"threadId": "main-thread", "turnId": "turn-new", "itemId": "command", "delta": "command output\n",
					})
				}
			}
			status := "completed"
			if mode == "stale-completed" || mode == "burst-failed" {
				status = "failed"
			}
			complete("turn-new", status)
			return rpcResult(msg["id"], map[string]any{"turnId": "turn-new"}), true
		default:
			return nil, false
		}
	})
}
