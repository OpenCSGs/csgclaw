package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentruntime "csgclaw/internal/runtime"
)

func TestAppServerManagerStartFailureIncludesRedactedStderrTail(t *testing.T) {
	withAppServerHelperCommand(t, "fail-before-handshake")
	dir := t.TempDir()
	manager := newAppServerManager(testAppServerManagerDeps())
	spec := testAppServerSessionSpec(dir)
	spec.Profile.APIKey = "sk-secret"

	_, err := manager.Start(context.Background(), spec)
	if err == nil {
		t.Fatal("Start() error = nil, want startup error")
	}
	if !strings.Contains(err.Error(), "stderr tail") || !strings.Contains(err.Error(), "login failed") {
		t.Fatalf("Start() error = %q, want stderr tail", err.Error())
	}
	if strings.Contains(err.Error(), "sk-secret") {
		t.Fatalf("Start() error leaked API key: %q", err.Error())
	}
}

func TestAppServerManagerStartFallsBackToThreadStartWhenResumeFails(t *testing.T) {
	withAppServerHelperCommand(t, "resume-fallback")
	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	if err := os.WriteFile(filepath.Join(spec.RuntimeDir, sessionFileName), []byte(`{"session_id":"old-thread"}`), 0o600); err != nil {
		t.Fatalf("write session metadata: %v", err)
	}

	manager := newAppServerManager(testAppServerManagerDeps())
	session, err := manager.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

	if got, want := session.SessionID, "new-thread"; got != want {
		t.Fatalf("SessionID = %q, want %q", got, want)
	}
}

func TestAppServerManagerEnsureSessionCreatesConversationThread(t *testing.T) {
	withAppServerHelperCommand(t, "conversation-thread")
	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	manager := newAppServerManager(testAppServerManagerDeps())
	session, err := manager.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

	mainThread, err := manager.EnsureSession(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, "")
	if err != nil {
		t.Fatalf("EnsureSession(main) error = %v", err)
	}
	if mainThread != session.SessionID {
		t.Fatalf("main thread = %q, want %q", mainThread, session.SessionID)
	}

	thread, err := manager.EnsureSession(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, "room-1")
	if err != nil {
		t.Fatalf("EnsureSession(conversation) error = %v", err)
	}
	if thread != "conversation-thread-2" {
		t.Fatalf("conversation thread = %q, want conversation-thread-2", thread)
	}
	again, err := manager.EnsureSession(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, "room-1")
	if err != nil {
		t.Fatalf("EnsureSession(conversation again) error = %v", err)
	}
	if again != thread {
		t.Fatalf("conversation thread after cache = %q, want %q", again, thread)
	}

	if err := manager.ResetConversationHistory(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, "room-1"); err != nil {
		t.Fatalf("ResetConversationHistory() error = %v", err)
	}
	threadAfterReset, err := manager.EnsureSession(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, "room-1")
	if err != nil {
		t.Fatalf("EnsureSession(after reset) error = %v", err)
	}
	if threadAfterReset == thread {
		t.Fatalf("conversation thread after reset = %q, want a new thread", threadAfterReset)
	}
}

func TestAppServerManagerStopClosesPendingRequests(t *testing.T) {
	withAppServerHelperCommand(t, "pending")
	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	manager := newAppServerManager(testAppServerManagerDeps())
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	manager.mu.RLock()
	live := manager.sessions[spec.RuntimeID]
	manager.mu.RUnlock()
	if live == nil || live.appClient == nil {
		t.Fatal("live app-server client is nil")
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := live.appClient.request(context.Background(), "turn/start", map[string]any{"threadId": "main-thread", "input": "hi"})
		errCh <- err
	}()

	waitForAppServerPending(t, live.appClient, 1)
	if err := manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "stopping") {
		t.Fatalf("pending request error = %v, want stopping", err)
	}
	if got := appServerPendingLen(live.appClient); got != 0 {
		t.Fatalf("pending len = %d, want 0", got)
	}
}

func TestAppServerManagerPromptCompletesTurn(t *testing.T) {
	withAppServerHelperCommand(t, "prompt-complete")
	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	sink := &recordingSink{}
	manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
	session, err := manager.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

	resp, err := manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
		SessionID: session.SessionID,
		Prompt:    []PromptContentBlock{TextBlock("hello codex")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Fatalf("StopReason = %q, want %q", resp.StopReason, StopReasonEndTurn)
	}

	waitForRuntime(t, func() bool { return len(sink.snapshot()) >= 2 })
	events := sink.snapshot()
	if len(events) != 2 ||
		events[0].Kind != SessionEventTextDelta ||
		events[1].Kind != SessionEventPromptCompleted ||
		events[1].SessionID != session.SessionID {
		t.Fatalf("events = %#v, want text delta then prompt completed event", events)
	}
}

func TestAppServerManagerPromptFailedTurnPublishesFailure(t *testing.T) {
	withAppServerHelperCommand(t, "prompt-failed")
	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	sink := &recordingSink{}
	manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
	session, err := manager.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

	_, err = manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
		SessionID: session.SessionID,
		Prompt:    []PromptContentBlock{TextBlock("fail please")},
	})
	if err == nil || !strings.Contains(err.Error(), "model failed") {
		t.Fatalf("Prompt() error = %v, want model failed", err)
	}
	events := sink.snapshot()
	if len(events) != 1 || events[0].Kind != SessionEventPromptFailed || !strings.Contains(events[0].Error, "model failed") {
		t.Fatalf("events = %#v, want one prompt failed event", events)
	}
}

func TestAppServerManagerPromptNoProgressTimeout(t *testing.T) {
	withAppServerHelperCommand(t, "prompt-no-progress")
	originalSemantic := appServerSemanticInactivityTimeout
	originalNoProgress := appServerFirstTurnNoProgressTimeout
	appServerSemanticInactivityTimeout = time.Second
	appServerFirstTurnNoProgressTimeout = 25 * time.Millisecond
	t.Cleanup(func() {
		appServerSemanticInactivityTimeout = originalSemantic
		appServerFirstTurnNoProgressTimeout = originalNoProgress
	})

	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	sink := &recordingSink{}
	manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
	session, err := manager.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

	_, err = manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
		SessionID: session.SessionID,
		Prompt:    []PromptContentBlock{TextBlock("hang")},
	})
	if err == nil ||
		!strings.Contains(err.Error(), "codex app-server no progress timeout") ||
		!strings.Contains(err.Error(), "runtime_id=runtime-1") ||
		!strings.Contains(err.Error(), "thread_id=main-thread") ||
		!strings.Contains(err.Error(), "turn_id=turn-hang") ||
		!strings.Contains(err.Error(), "stderr_tail=still working") {
		t.Fatalf("Prompt() error = %v, want no-progress diagnostic", err)
	}
	events := sink.snapshot()
	if len(events) != 1 || events[0].Kind != SessionEventPromptFailed {
		t.Fatalf("events = %#v, want one prompt failed event", events)
	}
}

func TestAppServerManagerAutoAcceptsCommandApproval(t *testing.T) {
	withAppServerHelperCommand(t, "approval-command")
	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	manager := newAppServerManager(testAppServerManagerDeps())
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })
}

func TestAppServerManagerAutoAcceptsFileApproval(t *testing.T) {
	withAppServerHelperCommand(t, "approval-file")
	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	manager := newAppServerManager(testAppServerManagerDeps())
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })
}

func TestAppServerManagerAutoAcceptsMCPElicitation(t *testing.T) {
	withAppServerHelperCommand(t, "approval-elicitation")
	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	manager := newAppServerManager(testAppServerManagerDeps())
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })
}

func TestAppServerManagerPromptAutoAcceptDoesNotBlockLifecycle(t *testing.T) {
	withAppServerHelperCommand(t, "prompt-approval-complete")
	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	sink := &recordingSink{}
	manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
	session, err := manager.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

	resp, err := manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
		SessionID: session.SessionID,
		Prompt:    []PromptContentBlock{TextBlock("hello with approval")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Fatalf("StopReason = %q, want %q", resp.StopReason, StopReasonEndTurn)
	}

	waitForRuntime(t, func() bool { return len(sink.snapshot()) >= 2 })
	events := sink.snapshot()
	if len(events) != 2 ||
		events[0].Kind != SessionEventTextDelta ||
		events[1].Kind != SessionEventPromptCompleted {
		t.Fatalf("events = %#v, want text delta then prompt completed", events)
	}
}

func TestAppServerEventAdapterRawTextAndToolEvents(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/started",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"item": map[string]any{
				"id":      "tool-1",
				"type":    "commandExecution",
				"command": "echo sk-secret",
			},
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"item": map[string]any{
				"id":               "tool-1",
				"type":             "commandExecution",
				"aggregatedOutput": "token=abc\nok",
			},
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"item": map[string]any{
				"id":   "msg-1",
				"type": "agentMessage",
				"text": "hello",
			},
		}),
	})

	events := sink.snapshot()
	if len(events) != 3 {
		t.Fatalf("events len = %d, want 3: %#v", len(events), events)
	}
	if events[0].Kind != SessionEventToolCallStart ||
		events[0].ToolCallID != "tool-1" ||
		events[0].ToolKind != "exec_command" ||
		!strings.Contains(events[0].ToolInputSummary, "[redacted]") {
		t.Fatalf("tool start = %#v, want redacted exec start", events[0])
	}
	if events[1].Kind != SessionEventToolCallUpdate ||
		events[1].ToolStatus != "completed" ||
		!strings.Contains(events[1].ToolOutputSummary, "[redacted]") {
		t.Fatalf("tool update = %#v, want redacted exec update", events[1])
	}
	if events[2].Kind != SessionEventTextDelta || events[2].Text != "hello" || events[2].MessageID != "msg-1" {
		t.Fatalf("text event = %#v, want agent message", events[2])
	}
}

func TestAppServerEventAdapterRawFileChangeAndFailures(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/started",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"item": map[string]any{
				"id":   "patch-1",
				"type": "fileChange",
				"path": "main.go",
			},
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"item": map[string]any{
				"id":     "patch-1",
				"type":   "fileChange",
				"status": "failed",
			},
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "turn/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turn": map[string]any{
				"id":     "turn-1",
				"status": "failed",
				"error":  map[string]any{"message": "bad turn"},
			},
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "turn/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turn": map[string]any{
				"id":     "turn-2",
				"status": "cancelled",
			},
		}),
	})

	events := sink.snapshot()
	if len(events) != 4 {
		t.Fatalf("events len = %d, want 4: %#v", len(events), events)
	}
	if events[0].Kind != SessionEventToolCallStart || events[0].ToolKind != "patch_apply" {
		t.Fatalf("file start = %#v, want patch_apply start", events[0])
	}
	if events[1].Kind != SessionEventToolCallUpdate || events[1].ToolStatus != "failed" {
		t.Fatalf("file update = %#v, want failed patch update", events[1])
	}
	if events[2].Kind != SessionEventPromptFailed || !strings.Contains(events[2].Error, "bad turn") {
		t.Fatalf("failed turn = %#v, want prompt failed", events[2])
	}
	if events[3].Kind != SessionEventPromptFailed || !strings.Contains(events[3].Error, "cancelled") {
		t.Fatalf("cancelled turn = %#v, want prompt failed", events[3])
	}
}

func TestAppServerEventAdapterLegacyEvents(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)

	for _, params := range []map[string]any{
		{"type": "agent_message", "message": "legacy text"},
		{"type": "exec_command_begin", "call_id": "call-1", "command": "go test"},
		{"type": "exec_command_end", "call_id": "call-1", "output": "ok"},
		{"type": "patch_apply_begin", "call_id": "patch-1"},
		{"type": "patch_apply_end", "call_id": "patch-1"},
		{"type": "task_complete"},
		{"type": "turn_aborted"},
	} {
		manager.handleAppServerNotification("runtime-1", live, appServerNotification{
			Method: "codex/event",
			Params: mustJSONRaw(t, params),
		})
	}

	events := sink.snapshot()
	kinds := make([]SessionEventKind, 0, len(events))
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	want := []SessionEventKind{
		SessionEventTextDelta,
		SessionEventToolCallStart,
		SessionEventToolCallUpdate,
		SessionEventToolCallStart,
		SessionEventToolCallUpdate,
		SessionEventPromptCompleted,
		SessionEventPromptFailed,
	}
	if fmt.Sprint(kinds) != fmt.Sprint(want) {
		t.Fatalf("event kinds = %#v, want %#v; events=%#v", kinds, want, events)
	}
	if live.appProtocol != appServerProtocolLegacy {
		t.Fatalf("protocol = %q, want legacy", live.appProtocol)
	}
}

func TestAppServerEventAdapterProtocolDetectionAndSubagentFilter(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "subagent-thread",
			"item": map[string]any{
				"id":   "msg-sub",
				"type": "agentMessage",
				"text": "ignore me",
			},
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/event",
		Params: mustJSONRaw(t, map[string]any{"type": "agent_message", "message": "legacy ignored after raw"}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"item": map[string]any{
				"id":   "msg-main",
				"type": "agentMessage",
				"text": "main",
			},
		}),
	})

	events := sink.snapshot()
	if len(events) != 1 || events[0].Text != "main" {
		t.Fatalf("events = %#v, want only main-thread raw event", events)
	}
	if live.appProtocol != appServerProtocolRaw {
		t.Fatalf("protocol = %q, want raw", live.appProtocol)
	}
}

func withAppServerHelperCommand(t *testing.T, mode string) {
	t.Helper()
	original := appServerCommandContext
	appServerCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		args := []string{"-test.run=TestAppServerManagerHelperProcess", "--", mode}
		return exec.CommandContext(ctx, os.Args[0], args...)
	}
	t.Cleanup(func() { appServerCommandContext = original })
}

func testAppServerManagerDeps() managerDeps {
	return managerDeps{
		OpenFile:  os.OpenFile,
		ReadFile:  os.ReadFile,
		WriteFile: os.WriteFile,
	}
}

func testAppServerManagerDepsWithSink(sink SessionEventSink) managerDeps {
	deps := testAppServerManagerDeps()
	deps.EventSink = sink
	return deps
}

func testAppServerSessionSpec(dir string) SessionSpec {
	runtimeDir := filepath.Join(dir, ".codex")
	workspaceDir := filepath.Join(runtimeDir, workspaceDirName)
	homeDir := filepath.Join(runtimeDir, homeDirName)
	for _, path := range []string{runtimeDir, workspaceDir, homeDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			panic(err)
		}
	}
	return SessionSpec{
		RuntimeID:    "runtime-1",
		AgentID:      "agent-1",
		AgentName:    "alice",
		BinaryPath:   "codex",
		RuntimeDir:   runtimeDir,
		WorkspaceDir: workspaceDir,
		HomeDir:      homeDir,
		CodexHomeDir: homeDir,
		StderrPath:   filepath.Join(homeDir, stderrLogFileName),
		Profile:      testProfile(),
	}
}

func testProfile() agentruntime.Profile {
	return agentruntime.Profile{
		BaseURL:         "https://api.example.com/v1",
		APIKey:          "sk-test",
		ModelID:         "gpt-5",
		ReasoningEffort: "medium",
	}
}

func waitForAppServerPending(t *testing.T, client *appServerClient, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := appServerPendingLen(client); got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("pending len = %d, want %d", appServerPendingLen(client), want)
}

func testAppServerEventAdapter(t *testing.T) (*appServerManager, *liveSession, *recordingSink) {
	t.Helper()
	sink := &recordingSink{}
	manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
	live := &liveSession{
		session: &Session{
			RuntimeID: "runtime-1",
			SessionID: "main-thread",
		},
		conversationSessions: make(map[string]string),
		turnWaiters:          make(map[string]*appServerTurnWaiter),
	}
	return manager, live, sink
}

func mustJSONRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal raw json: %v", err)
	}
	return data
}

func TestAppServerManagerHelperProcess(t *testing.T) {
	args := os.Args
	if len(args) < 3 || args[len(args)-2] != "--" {
		return
	}
	mode := args[len(args)-1]
	switch mode {
	case "fail-before-handshake":
		_, _ = fmt.Fprintln(os.Stderr, "login failed")
		_, _ = fmt.Fprintln(os.Stderr, "api_key = sk-secret")
		os.Exit(2)
	case "resume-fallback":
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			switch msg["method"] {
			case "thread/resume":
				return rpcError(msg["id"], -32000, "thread not found"), true
			case "thread/start":
				return rpcResult(msg["id"], map[string]any{"threadId": "new-thread"}), true
			default:
				return nil, false
			}
		})
	case "resume-success":
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			switch msg["method"] {
			case "thread/resume":
				return rpcResult(msg["id"], map[string]any{"threadId": "resumed-thread"}), true
			case "thread/start":
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			case "turn/start":
				params, _ := msg["params"].(map[string]any)
				threadID, _ := params["threadId"].(string)
				if threadID == "" {
					t.Fatalf("turn/start threadId missing in %#v", params)
				}
				writeRPCNotification(t, "turn/started", map[string]any{"threadId": threadID, "turn": map[string]any{"id": "turn-1"}})
				writeRPCNotification(t, "item/completed", map[string]any{"threadId": threadID, "item": map[string]any{"id": "item-1", "type": "agentMessage", "text": "done"}})
				writeRPCNotification(t, "turn/completed", map[string]any{"threadId": threadID, "turn": map[string]any{"id": "turn-1", "status": "completed"}})
				return rpcResult(msg["id"], map[string]any{"turnId": "turn-1"}), true
			default:
				return nil, false
			}
		})
	case "conversation-thread":
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			if msg["method"] != "thread/start" {
				return nil, false
			}
			if index == 1 {
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			}
			return rpcResult(msg["id"], map[string]any{"threadId": fmt.Sprintf("conversation-thread-%d", index)}), true
		})
	case "pending":
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			if msg["method"] == "thread/start" {
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			}
			return nil, false
		})
	case "prompt-complete":
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			switch msg["method"] {
			case "thread/start":
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			case "turn/start":
				assertTurnStartParams(t, msg, "main-thread", "medium", "hello codex")
				writeRPCNotification(t, "turn/started", map[string]any{"threadId": "main-thread", "turn": map[string]any{"id": "turn-1"}})
				writeRPCNotification(t, "item/completed", map[string]any{"threadId": "main-thread", "item": map[string]any{"id": "item-1", "type": "agentMessage", "text": "done"}})
				writeRPCNotification(t, "turn/completed", map[string]any{"threadId": "main-thread", "turn": map[string]any{"id": "turn-1", "status": "completed"}})
				return rpcResult(msg["id"], map[string]any{"turnId": "turn-1"}), true
			default:
				return nil, false
			}
		})
	case "prompt-failed":
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			switch msg["method"] {
			case "thread/start":
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			case "turn/start":
				writeRPCNotification(t, "turn/started", map[string]any{"threadId": "main-thread", "turn": map[string]any{"id": "turn-failed"}})
				writeRPCNotification(t, "turn/completed", map[string]any{
					"threadId": "main-thread",
					"turn": map[string]any{
						"id":     "turn-failed",
						"status": "failed",
						"error":  map[string]any{"message": "model failed"},
					},
				})
				return rpcResult(msg["id"], map[string]any{"turnId": "turn-failed"}), true
			default:
				return nil, false
			}
		})
	case "prompt-no-progress":
		_, _ = fmt.Fprintln(os.Stderr, "still working")
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			switch msg["method"] {
			case "thread/start":
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			case "turn/start":
				writeRPCNotification(t, "turn/started", map[string]any{"threadId": "main-thread", "turn": map[string]any{"id": "turn-hang"}})
				return rpcResult(msg["id"], map[string]any{"turnId": "turn-hang"}), true
			default:
				return nil, false
			}
		})
	case "approval-command":
		awaitingApproval := false
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			if awaitingApproval {
				assertServerRequestResponse(t, msg, 9001, func(result map[string]any) {
					if got := result["decision"]; got != "accept" {
						t.Fatalf("command approval decision = %#v, want accept", got)
					}
				})
				awaitingApproval = false
				return nil, false
			}
			switch msg["method"] {
			case "thread/start":
				awaitingApproval = true
				writeRPCServerRequest(t, 9001, "item/commandExecution/requestApproval", map[string]any{})
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			default:
				return nil, false
			}
		})
	case "approval-file":
		awaitingApproval := false
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			if awaitingApproval {
				assertServerRequestResponse(t, msg, 9001, func(result map[string]any) {
					if got := result["decision"]; got != "accept" {
						t.Fatalf("file approval decision = %#v, want accept", got)
					}
				})
				awaitingApproval = false
				return nil, false
			}
			switch msg["method"] {
			case "thread/start":
				awaitingApproval = true
				writeRPCServerRequest(t, 9001, "applyPatchApproval", map[string]any{})
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			default:
				return nil, false
			}
		})
	case "approval-elicitation":
		awaitingApproval := false
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			if awaitingApproval {
				assertServerRequestResponse(t, msg, 9001, func(result map[string]any) {
					if got := result["action"]; got != "accept" {
						t.Fatalf("elicitation action = %#v, want accept", got)
					}
					if _, ok := result["content"]; !ok {
						t.Fatal("elicitation result missing content")
					}
					if _, ok := result["_meta"]; !ok {
						t.Fatal("elicitation result missing _meta")
					}
				})
				awaitingApproval = false
				return nil, false
			}
			switch msg["method"] {
			case "thread/start":
				awaitingApproval = true
				writeRPCServerRequest(t, 9001, "mcpServer/elicitation/request", map[string]any{})
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			default:
				return nil, false
			}
		})
	case "prompt-approval-complete":
		awaitingApproval := false
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			if awaitingApproval {
				assertServerRequestResponse(t, msg, 9001, func(result map[string]any) {
					if got := result["decision"]; got != "accept" {
						t.Fatalf("turn approval decision = %#v, want accept", got)
					}
				})
				awaitingApproval = false
				writeRPCNotification(t, "turn/started", map[string]any{"threadId": "main-thread", "turn": map[string]any{"id": "turn-approval"}})
				writeRPCNotification(t, "item/completed", map[string]any{"threadId": "main-thread", "item": map[string]any{"id": "item-approval", "type": "agentMessage", "text": "approved"}})
				writeRPCNotification(t, "turn/completed", map[string]any{"threadId": "main-thread", "turn": map[string]any{"id": "turn-approval", "status": "completed"}})
				return nil, false
			}
			switch msg["method"] {
			case "thread/start":
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			case "turn/start":
				assertTurnStartParams(t, msg, "main-thread", "medium", "hello with approval")
				awaitingApproval = true
				writeRPCServerRequest(t, 9001, "item/commandExecution/requestApproval", map[string]any{})
				return rpcResult(msg["id"], map[string]any{"turnId": "turn-approval"}), true
			default:
				return nil, false
			}
		})
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
}

func runAppServerHelper(t *testing.T, handle func(index int, msg map[string]any) (map[string]any, bool)) {
	t.Helper()
	scanner := bufio.NewScanner(os.Stdin)
	index := 0
	for scanner.Scan() {
		var msg map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			t.Fatalf("helper unmarshal request: %v", err)
		}
		if method, _ := msg["method"].(string); method == "initialize" {
			resp := rpcResult(msg["id"], map[string]any{
				"capabilities": map[string]any{"experimentalApi": true},
			})
			data, err := json.Marshal(resp)
			if err != nil {
				t.Fatalf("helper marshal initialize response: %v", err)
			}
			_, _ = fmt.Fprintln(os.Stdout, string(data))
			continue
		} else if method == "initialized" {
			continue
		}
		index++
		resp, ok := handle(index, msg)
		if !ok {
			continue
		}
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("helper marshal response: %v", err)
		}
		_, _ = fmt.Fprintln(os.Stdout, string(data))
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, os.ErrClosed) {
		t.Fatalf("helper scanner: %v", err)
	}
}

func rpcResult(id any, result any) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
}

func rpcError(id any, code int, message string) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
}

func writeRPCNotification(t *testing.T, method string, params any) {
	t.Helper()
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}
	_, _ = fmt.Fprintln(os.Stdout, string(data))
}

func assertTurnStartParams(t *testing.T, msg map[string]any, wantThreadID string, wantEffort string, wantPrompt string) {
	t.Helper()
	params, _ := msg["params"].(map[string]any)
	if params == nil {
		t.Fatalf("turn/start params = %#v, want object", msg["params"])
	}
	if got := params["threadId"]; got != wantThreadID {
		t.Fatalf("turn/start threadId = %#v, want %q", got, wantThreadID)
	}
	if got := params["effort"]; got != wantEffort {
		t.Fatalf("turn/start effort = %#v, want %q", got, wantEffort)
	}
	input, _ := params["input"].([]any)
	if len(input) != 1 {
		t.Fatalf("turn/start input = %#v, want one text block", params["input"])
	}
	block, _ := input[0].(map[string]any)
	if block["type"] != "text" || block["text"] != wantPrompt {
		t.Fatalf("turn/start input block = %#v, want text prompt", block)
	}
}

func writeRPCServerRequest(t *testing.T, id int, method string, params any) {
	t.Helper()
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal server request: %v", err)
	}
	_, _ = fmt.Fprintln(os.Stdout, string(data))
}

func assertServerRequestResponse(t *testing.T, msg map[string]any, wantID int, assertResult func(map[string]any)) {
	t.Helper()
	if got := msg["id"]; got != float64(wantID) {
		t.Fatalf("server request response id = %#v, want %d", got, wantID)
	}
	result, _ := msg["result"].(map[string]any)
	if result == nil {
		t.Fatalf("server request response result = %#v, want object", msg["result"])
	}
	assertResult(result)
}
