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

	"csgclaw/internal/activity"
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
	var persisted []map[string]string
	deps := testAppServerManagerDeps()
	deps.OnConversationSessionsChange = func(_ *Session, conversations map[string]string) error {
		persisted = append(persisted, cloneConversationSessions(conversations))
		return nil
	}
	manager := newAppServerManager(deps)
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
	if len(persisted) != 3 {
		t.Fatalf("persisted conversation snapshots = %#v, want create/reset/recreate", persisted)
	}
	if got := persisted[0]["room-1"]; got != thread {
		t.Fatalf("first persisted room thread = %q, want %q", got, thread)
	}
	if len(persisted[1]) != 0 {
		t.Fatalf("reset persisted conversations = %#v, want empty", persisted[1])
	}
	if got := persisted[2]["room-1"]; got != threadAfterReset {
		t.Fatalf("recreated persisted room thread = %q, want %q", got, threadAfterReset)
	}
}

func TestAppServerManagerRestoresConversationThreadMapping(t *testing.T) {
	withAppServerHelperCommand(t, "resume-success")
	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	spec.ConversationSessions = map[string]string{"room-1": "persisted-room-thread"}
	manager := newAppServerManager(testAppServerManagerDeps())
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

	thread, err := manager.EnsureSession(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, "room-1")
	if err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}
	if got, want := thread, "resumed-thread"; got != want {
		t.Fatalf("EnsureSession() = %q, want restored %q", got, want)
	}
	resp, err := manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
		SessionID: thread,
		Prompt:    []PromptContentBlock{TextBlock("continue")},
	})
	if err != nil {
		t.Fatalf("Prompt(restored conversation) error = %v", err)
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Fatalf("Prompt(restored conversation) stop reason = %q, want %q", resp.StopReason, StopReasonEndTurn)
	}
}

func TestAppServerManagerPreservesColdEngineConversationContext(t *testing.T) {
	withAppServerHelperCommand(t, "resume-success")
	spec := testAppServerSessionSpec(t.TempDir())
	spec.ConversationSessions = map[string]string{"conversation-1": "cold-thread"}
	var persisted map[string]string
	deps := testAppServerManagerDeps()
	deps.OnConversationSessionsChange = func(_ *Session, conversations map[string]string) error {
		persisted = cloneConversationSessions(conversations)
		return nil
	}
	manager := newAppServerManager(deps)
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })
	threadID, err := manager.EnsureEngineSession(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, "conversation-1")
	if err != nil {
		t.Fatalf("cold EnsureEngineSession error=%v", err)
	}
	if threadID != "resumed-thread" || persisted["conversation-1"] != threadID {
		t.Fatalf("resumed Engine thread=%q persisted=%v", threadID, persisted)
	}
	if got, ok, err := manager.ExistingEngineSession(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, "conversation-1"); err != nil || !ok || got != threadID {
		t.Fatalf("loaded ExistingEngineSession=%q ok=%v err=%v", got, ok, err)
	}
	live := manager.liveSession(spec.RuntimeID)
	live.mu.Lock()
	filePublishing := live.filePublishingThreads[threadID]
	live.mu.Unlock()
	if filePublishing {
		t.Fatal("cold resumed thread was incorrectly marked file-capable")
	}
}

func TestAppServerManagerSerializesConversationPersistence(t *testing.T) {
	withAppServerHelperCommand(t, "conversation-thread")
	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	deps := testAppServerManagerDeps()
	deps.OnConversationSessionsChange = func(_ *Session, _ map[string]string) error {
		entered <- struct{}{}
		<-release
		return nil
	}
	manager := newAppServerManager(deps)
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

	errCh := make(chan error, 2)
	go func() {
		_, err := manager.EnsureSession(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, "room-1")
		errCh <- err
	}()
	<-entered
	go func() {
		_, err := manager.EnsureSession(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, "room-2")
		errCh <- err
	}()
	select {
	case <-entered:
		t.Fatal("conversation persistence callbacks overlapped")
	case <-time.After(50 * time.Millisecond):
	}
	release <- struct{}{}
	<-entered
	release <- struct{}{}
	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("EnsureSession() error = %v", err)
		}
	}
}

func TestAppServerManagerEnsureSessionHandlesThreadNotificationBeforeResponse(t *testing.T) {
	withAppServerHelperCommand(t, "conversation-thread-notification-before-result")
	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	manager := newAppServerManager(testAppServerManagerDeps())
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	thread, err := manager.EnsureSession(ctx, SessionHandle{RuntimeID: spec.RuntimeID}, "room-1")
	if err != nil {
		t.Fatalf("EnsureSession(conversation) error = %v", err)
	}
	if thread != "conversation-thread-2" {
		t.Fatalf("conversation thread = %q, want conversation-thread-2", thread)
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

func TestAppServerManagerStaleExitDoesNotDetachReplacement(t *testing.T) {
	manager := newAppServerManager(testAppServerManagerDeps())
	stale := &liveSession{}
	replacement := &liveSession{}
	manager.sessions["runtime-1"] = replacement

	if manager.detachSession("runtime-1", stale) {
		t.Fatal("detachSession() = true for stale session, want false")
	}
	if got := manager.sessions["runtime-1"]; got != replacement {
		t.Fatalf("sessions[runtime-1] = %p, want replacement %p", got, replacement)
	}
	if !manager.detachSession("runtime-1", replacement) {
		t.Fatal("detachSession() = false for current session, want true")
	}
	if _, ok := manager.sessions["runtime-1"]; ok {
		t.Fatal("current session still registered after detach")
	}
}

func TestAppServerManagerCloseStopsAllSessions(t *testing.T) {
	withAppServerHelperCommand(t, "pending")
	manager := newAppServerManager(testAppServerManagerDeps())
	var pids []int
	for _, runtimeID := range []string{"runtime-1", "runtime-2"} {
		spec := testAppServerSessionSpec(filepath.Join(t.TempDir(), runtimeID))
		spec.RuntimeID = runtimeID
		session, err := manager.Start(context.Background(), spec)
		if err != nil {
			t.Fatalf("Start(%s) error = %v", runtimeID, err)
		}
		pids = append(pids, session.ProcessID)
	}

	if err := manager.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	manager.mu.RLock()
	remaining := len(manager.sessions)
	manager.mu.RUnlock()
	if remaining != 0 {
		t.Fatalf("live sessions after Close() = %d, want 0", remaining)
	}
	for _, pid := range pids {
		if processAlive(pid) {
			t.Fatalf("codex app-server process %d is still alive after Close()", pid)
		}
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

func TestAppServerManagerCheckpointsMemoryWithoutPublishingMaintenanceOutput(t *testing.T) {
	withAppServerHelperCommand(t, "memory-checkpoint")
	originalIdle := appServerMemoryCheckpointIdleTime
	originalTimeout := appServerMemoryMaintenanceTimeout
	originalDedup := appServerMemoryMaintenanceDedup
	originalCleanup := appServerMemoryCheckpointCleanup
	appServerMemoryCheckpointIdleTime = 20 * time.Millisecond
	appServerMemoryMaintenanceTimeout = 2 * time.Second
	appServerMemoryMaintenanceDedup = time.Second
	appServerMemoryCheckpointCleanup = 20 * time.Millisecond
	t.Cleanup(func() {
		appServerMemoryCheckpointIdleTime = originalIdle
		appServerMemoryMaintenanceTimeout = originalTimeout
		appServerMemoryMaintenanceDedup = originalDedup
		appServerMemoryCheckpointCleanup = originalCleanup
	})
	deleteMarker := filepath.Join(t.TempDir(), "checkpoint-deleted")
	t.Setenv("CSGCLAW_MEMORY_CHECKPOINT_DELETE_MARKER", deleteMarker)

	sink := &recordingSink{}
	manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
	spec := testAppServerSessionSpec(t.TempDir())
	spec.MemoryEnabled = true
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

	roomThread, err := manager.EnsureEngineSession(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, "room-1")
	if err != nil {
		t.Fatalf("EnsureEngineSession() error = %v", err)
	}
	if roomThread != "room-thread" {
		t.Fatalf("room thread = %q, want room-thread", roomThread)
	}
	if _, err := manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
		SessionID: roomThread,
		Prompt:    []PromptContentBlock{TextBlock("remember my preference")},
	}); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	live := manager.liveSession(spec.RuntimeID)
	waitForRuntime(t, func() bool {
		live.memoryCheckpointMu.Lock()
		defer live.memoryCheckpointMu.Unlock()
		return !live.memoryLastMaintenance.IsZero() && live.memoryMaintenanceID == ""
	})
	waitForRuntime(t, func() bool {
		_, err := os.Stat(deleteMarker)
		return err == nil
	})
	if _, err := manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
		SessionID: roomThread,
		Prompt:    []PromptContentBlock{TextBlock("second preference")},
	}); err != nil {
		t.Fatalf("second Prompt() error = %v", err)
	}
	for _, event := range sink.snapshot() {
		if event.SessionID == "main-thread" {
			t.Fatalf("hidden maintenance event leaked to runtime sink: %#v", event)
		}
	}
}

func TestAppServerManagerKeepsThreadBusyWhileMemoryForkIsInFlight(t *testing.T) {
	withAppServerHelperCommand(t, "memory-checkpoint")
	forkEntered := filepath.Join(t.TempDir(), "fork-entered")
	forkRelease := filepath.Join(t.TempDir(), "fork-release")
	t.Setenv("CSGCLAW_MEMORY_CHECKPOINT_FORK_ENTERED", forkEntered)
	t.Setenv("CSGCLAW_MEMORY_CHECKPOINT_FORK_RELEASE", forkRelease)

	manager := newAppServerManager(testAppServerManagerDeps())
	spec := testAppServerSessionSpec(t.TempDir())
	spec.MemoryEnabled = true
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })
	roomThread, err := manager.EnsureEngineSession(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, "room-1")
	if err != nil {
		t.Fatalf("EnsureEngineSession() error = %v", err)
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
			SessionID: roomThread,
			Prompt:    []PromptContentBlock{TextBlock("remember my preference")},
		})
		firstDone <- err
	}()
	waitForRuntime(t, func() bool {
		_, err := os.Stat(forkEntered)
		return err == nil
	})
	_, secondErr := manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
		SessionID: roomThread,
		Prompt:    []PromptContentBlock{TextBlock("second preference")},
	})
	if secondErr == nil || !strings.Contains(secondErr.Error(), "turn already in progress") {
		t.Fatalf("second Prompt() error = %v, want in-progress rejection while fork is blocked", secondErr)
	}
	if err := os.WriteFile(forkRelease, []byte("release"), 0o600); err != nil {
		t.Fatalf("release blocked fork: %v", err)
	}
	if err := <-firstDone; err != nil {
		t.Fatalf("first Prompt() error = %v", err)
	}
}

func TestAppServerManagerThrottlesFailedMemoryForks(t *testing.T) {
	withAppServerHelperCommand(t, "memory-checkpoint")
	t.Setenv("CSGCLAW_MEMORY_CHECKPOINT_FORK_ERROR", "1")
	manager := newAppServerManager(testAppServerManagerDeps())
	spec := testAppServerSessionSpec(t.TempDir())
	spec.MemoryEnabled = true
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })
	roomThread, err := manager.EnsureEngineSession(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, "room-1")
	if err != nil {
		t.Fatalf("EnsureEngineSession() error = %v", err)
	}
	for _, prompt := range []string{"remember my preference", "second preference"} {
		if _, err := manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
			SessionID: roomThread,
			Prompt:    []PromptContentBlock{TextBlock(prompt)},
		}); err != nil {
			t.Fatalf("Prompt(%q) error = %v", prompt, err)
		}
	}
}

func TestAppServerManagerPublishFileDynamicToolEndToEnd(t *testing.T) {
	withAppServerHelperCommand(t, "prompt-publish-file")
	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	sink := &recordingSink{}
	manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
	_, err := manager.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })
	engineThread, err := manager.EnsureEngineSession(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, "engine-files")
	if err != nil {
		t.Fatalf("EnsureEngineSession() error = %v", err)
	}

	resp, err := manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
		SessionID: engineThread,
		Prompt:    []PromptContentBlock{TextBlock("publish the report")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Fatalf("StopReason = %q, want %q", resp.StopReason, StopReasonEndTurn)
	}

	waitForRuntime(t, func() bool { return len(sink.snapshot()) >= 3 })
	events := sink.snapshot()
	if len(events) != 3 || events[0].Kind != SessionEventFileOutput || events[1].Kind != SessionEventTextDelta || events[2].Kind != SessionEventPromptCompleted {
		t.Fatalf("events = %#v, want file output, text, and prompt completion", events)
	}
	file, ok := events[0].Payload.(activity.RuntimeFile)
	if !ok || file.Path != filepath.Join("reports", "result.pdf") || file.Name != "result.pdf" || file.MIMEType != "application/pdf" {
		t.Fatalf("file output = %#v", events[0])
	}
}

func TestAppServerManagerQuestionRoundTripForManagerAndWorker(t *testing.T) {
	for _, tc := range []struct {
		name      string
		runtimeID string
		agentID   string
		agentName string
	}{
		{name: "manager", runtimeID: "runtime-manager", agentID: "manager", agentName: "manager"},
		{name: "worker", runtimeID: "runtime-worker", agentID: "worker-1", agentName: "alice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withAppServerHelperCommand(t, "prompt-user-input-complete")
			dir := t.TempDir()
			spec := testAppServerSessionSpec(dir)
			spec.RuntimeID = tc.runtimeID
			spec.AgentID = tc.agentID
			spec.AgentName = tc.agentName
			sink := &recordingSink{}
			broker := NewUserInputBroker(sink)
			deps := testAppServerManagerDepsWithSink(sink)
			deps.UserInput = broker
			manager := newAppServerManager(deps)
			session, err := manager.Start(context.Background(), spec)
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

			promptResult := make(chan error, 1)
			go func() {
				_, err := manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
					SessionID: session.SessionID,
					Prompt:    []PromptContentBlock{TextBlock("ask me")},
				})
				promptResult <- err
			}()

			requestID := waitForUserInputRequest(t, sink)
			var requestEvent activity.RuntimeEvent
			for _, event := range sink.snapshot() {
				if event.Kind == activity.RuntimeEventUserInputRequest && event.UserInputID == requestID {
					requestEvent = event
					break
				}
			}
			if requestEvent.RuntimeID != tc.runtimeID || requestEvent.SessionID != "main-thread" || requestEvent.TurnID != "turn-question" || requestEvent.ToolCallID != "item-question" {
				t.Fatalf("question execution linkage = %+v", requestEvent)
			}
			request, ok := broker.Get(requestID)
			if !ok || len(request.Questions) != 1 || request.Questions[0].ID != "color" {
				t.Fatalf("request snapshot = %+v", request)
			}
			if _, err := broker.Bind(requestID, "csgclaw", "room-1", "", "pt-agent"); err != nil {
				t.Fatalf("Bind() error = %v", err)
			}
			if _, err := broker.Respond(context.Background(), activity.UserInputResponseRequest{
				Channel: "csgclaw", ActivityID: requestID, RoomID: "room-1", ResponderID: "u-admin",
				Response: activity.RequestUserInputResponse{Answers: map[string]activity.RequestUserInputAnswer{
					"color": {Answers: []string{"Green", "user_note: deep shade"}},
				}},
			}); err != nil {
				t.Fatalf("Respond() error = %v", err)
			}
			select {
			case err := <-promptResult:
				if err != nil {
					t.Fatalf("Prompt() error = %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("prompt did not continue after answer")
			}
			waitForRuntime(t, func() bool {
				var continued, completed bool
				for _, event := range sink.snapshot() {
					continued = continued || event.Kind == SessionEventTextDelta && event.Text == "continued with Green"
					completed = completed || event.Kind == SessionEventPromptCompleted
				}
				return continued && completed
			})
		})
	}
}

func TestAppServerManagerPausesTurnWatchdogsWhileWaitingForUser(t *testing.T) {
	withAppServerHelperCommand(t, "prompt-user-input-complete")
	originalSemantic := appServerSemanticInactivityTimeout
	originalNoProgress := appServerFirstTurnNoProgressTimeout
	originalMaximum := appServerMaximumTurnDuration
	appServerSemanticInactivityTimeout = 20 * time.Millisecond
	appServerFirstTurnNoProgressTimeout = 20 * time.Millisecond
	appServerMaximumTurnDuration = 20 * time.Millisecond
	t.Cleanup(func() {
		appServerSemanticInactivityTimeout = originalSemantic
		appServerFirstTurnNoProgressTimeout = originalNoProgress
		appServerMaximumTurnDuration = originalMaximum
	})

	spec := testAppServerSessionSpec(t.TempDir())
	sink := &recordingSink{}
	broker := NewUserInputBroker(sink)
	deps := testAppServerManagerDepsWithSink(sink)
	deps.UserInput = broker
	manager := newAppServerManager(deps)
	session, err := manager.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

	promptResult := make(chan error, 1)
	go func() {
		_, err := manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
			SessionID: session.SessionID,
			Prompt:    []PromptContentBlock{TextBlock("ask me")},
		})
		promptResult <- err
	}()
	requestID := waitForUserInputRequest(t, sink)
	_, _ = broker.Bind(requestID, "csgclaw", "room-1", "", "pt-agent")
	time.Sleep(75 * time.Millisecond)
	select {
	case err := <-promptResult:
		t.Fatalf("prompt ended while waiting for user input: %v", err)
	default:
	}
	_, err = broker.Respond(context.Background(), activity.UserInputResponseRequest{
		Channel: "csgclaw", ActivityID: requestID, RoomID: "room-1", ResponderID: "u-admin",
		Response: activity.RequestUserInputResponse{Answers: map[string]activity.RequestUserInputAnswer{
			"color": {Answers: []string{"Green", "user_note: deep shade"}},
		}},
	})
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	select {
	case err := <-promptResult:
		if err != nil {
			t.Fatalf("Prompt() error after answer = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("prompt did not finish after answer")
	}
}

func TestAppServerTurnWaiterTracksConcurrentUserInputRequests(t *testing.T) {
	t.Parallel()

	live := &liveSession{}
	waiter, err := live.registerAppServerTurnWaiter("thread-1")
	if err != nil {
		t.Fatalf("register waiter: %v", err)
	}
	defer live.removeAppServerTurnWaiter("thread-1", waiter)

	for _, delta := range []int{1, 1, -1, -1} {
		if !live.notifyAppServerTurn("thread-1", appServerTurnResult{userInputDelta: delta}) {
			t.Fatal("user-input state change was not delivered")
		}
	}
	wantWaiting := []bool{true, true, true, false}
	for index, want := range wantWaiting {
		select {
		case result := <-waiter.ch:
			if !result.userInputStateChanged || result.waitingForUser != want {
				t.Fatalf("state change %d = %+v, want waiting=%v", index, result, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for state change %d", index)
		}
	}
}

func TestAppServerManagerKeepsTurnOpenWhenAgentMessagePrecedesQuestion(t *testing.T) {
	withAppServerHelperCommand(t, "prompt-agent-message-before-user-input")
	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	sink := &recordingSink{}
	broker := NewUserInputBroker(sink)
	deps := testAppServerManagerDepsWithSink(sink)
	deps.UserInput = broker
	manager := newAppServerManager(deps)
	session, err := manager.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

	promptResult := make(chan error, 1)
	go func() {
		_, err := manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
			SessionID: session.SessionID,
			Prompt:    []PromptContentBlock{TextBlock("report then ask me")},
		})
		promptResult <- err
	}()

	requestID := waitForUserInputRequest(t, sink)
	select {
	case err := <-promptResult:
		t.Fatalf("prompt ended before the user-input request was answered: %v", err)
	default:
	}
	if _, err := broker.Bind(requestID, "csgclaw", "room-1", "", "pt-agent"); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if _, err := broker.Respond(context.Background(), activity.UserInputResponseRequest{
		Channel: "csgclaw", ActivityID: requestID, RoomID: "room-1", ResponderID: "u-admin",
		Response: activity.RequestUserInputResponse{Answers: map[string]activity.RequestUserInputAnswer{
			"next": {Answers: []string{"Continue"}},
		}},
	}); err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	select {
	case err := <-promptResult:
		if err != nil {
			t.Fatalf("Prompt() error after answer = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("prompt did not continue after answer")
	}
}

func TestAppServerManagerPromptReplaysLegacyRolloutResponseItems(t *testing.T) {
	withAppServerHelperCommand(t, "prompt-legacy-rollout-complete")
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
		Prompt:    []PromptContentBlock{TextBlock("put it in a file")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Fatalf("StopReason = %q, want %q", resp.StopReason, StopReasonEndTurn)
	}

	waitForRuntime(t, func() bool { return len(sink.snapshot()) >= 4 })
	events := sink.snapshot()
	kinds := make([]SessionEventKind, 0, len(events))
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	want := []SessionEventKind{
		SessionEventTextDelta,
		SessionEventToolCallStart,
		SessionEventToolCallUpdate,
		SessionEventTextDelta,
		SessionEventPromptCompleted,
	}
	if fmt.Sprint(kinds) != fmt.Sprint(want) {
		t.Fatalf("event kinds = %#v, want %#v; events=%#v", kinds, want, events)
	}
	if events[0].Text != "checking workspace" || events[3].Text != "已经放到文件里了" {
		t.Fatalf("events = %#v, want commentary then final text", events)
	}
	if events[0].Payload.(map[string]any)["phase"] != "commentary" || events[3].Payload.(map[string]any)["phase"] != "final_answer" {
		t.Fatalf("payload phases = %#v / %#v, want commentary/final_answer", events[0].Payload, events[3].Payload)
	}
}

func TestAppServerManagerPromptHandlesLargeCommandOutput(t *testing.T) {
	withAppServerHelperCommand(t, "prompt-large-command-output")
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
		Prompt:    []PromptContentBlock{TextBlock("hello large output")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Fatalf("StopReason = %q, want %q", resp.StopReason, StopReasonEndTurn)
	}

	waitForRuntime(t, func() bool { return len(sink.snapshot()) >= 4 })
	events := sink.snapshot()
	if len(events) < 4 {
		t.Fatalf("events len = %d, want at least 4: %#v", len(events), events)
	}
	if events[0].Kind != SessionEventToolCallStart || events[0].ToolCallID != "call-large" {
		t.Fatalf("first event = %#v, want tool start for call-large", events[0])
	}
	if events[1].Kind != SessionEventToolCallUpdate || events[1].ToolStatus != "completed" {
		t.Fatalf("second event = %#v, want completed tool update", events[1])
	}
	if events[2].Kind != SessionEventTextDelta || events[2].Text != "done" {
		t.Fatalf("third event = %#v, want agent text delta", events[2])
	}
	if events[len(events)-1].Kind != SessionEventPromptCompleted {
		t.Fatalf("last event = %#v, want prompt completed", events[len(events)-1])
	}
}

func TestAppServerManagerPromptPublishesStructuredDeltaBeforeCompletion(t *testing.T) {
	withAppServerHelperCommand(t, "prompt-command-output-delta")
	spec := testAppServerSessionSpec(t.TempDir())
	sink := &recordingSink{}
	manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
	session, err := manager.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

	resp, err := manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
		SessionID: session.SessionID,
		Prompt:    []PromptContentBlock{TextBlock("run structured delta")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Fatalf("StopReason = %q, want %q", resp.StopReason, StopReasonEndTurn)
	}

	events := sink.snapshot()
	want := []SessionEventKind{
		SessionEventToolCallStart,
		SessionEventStructuredOutput,
		SessionEventToolCallUpdate,
		SessionEventPromptCompleted,
	}
	if len(events) != len(want) {
		t.Fatalf("events = %#v, want kinds %#v", events, want)
	}
	for index, kind := range want {
		if events[index].Kind != kind {
			t.Fatalf("events[%d] = %#v, want kind %s", index, events[index], kind)
		}
	}
	artifact := events[1].Payload.(activity.StructuredOutputArtifact)
	if artifact.RequestUserInput == nil || artifact.RequestUserInput.Questions[0].ID != "step_two" {
		t.Fatalf("artifact = %#v, want step_two question", artifact)
	}
	if events[1].Text != "ordinary" {
		t.Fatalf("structured fallback = %q, want cleaned ordinary stdout", events[1].Text)
	}
	if strings.Contains(events[2].ToolOutputSummary, structuredOutputPrefix) || !strings.Contains(events[2].ToolOutputSummary, "ordinary") {
		t.Fatalf("tool output summary = %q, want cleaned ordinary output", events[2].ToolOutputSummary)
	}
}

func TestAppServerManagerPromptCancellationReturnsConfirmedStop(t *testing.T) {
	withAppServerHelperCommand(t, "prompt-cancel-confirmed")
	spec := testAppServerSessionSpec(t.TempDir())
	manager := newAppServerManager(testAppServerManagerDeps())
	session, err := manager.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

	ctx, cancel := context.WithCancel(context.Background())
	type promptResult struct {
		response PromptResponse
		err      error
	}
	promptDone := make(chan promptResult, 1)
	go func() {
		response, promptErr := manager.Prompt(ctx, SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
			SessionID: session.SessionID,
			Prompt:    []PromptContentBlock{TextBlock("keep working")},
		})
		promptDone <- promptResult{response: response, err: promptErr}
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
	case result := <-promptDone:
		if result.response.StopReason != StopReasonInterrupted || !errors.Is(result.err, context.Canceled) {
			t.Fatalf("Prompt() result = %#v, error = %v; want interrupted response with context cancellation", result.response, result.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Prompt() did not return after confirmed runtime stop")
	}
}

func TestAppServerManagerPromptCancellationDoesNotConfirmFailedStop(t *testing.T) {
	originalStopTimeout := appServerStopTimeout
	appServerStopTimeout = 10 * time.Millisecond
	t.Cleanup(func() { appServerStopTimeout = originalStopTimeout })

	manager := newAppServerManager(testAppServerManagerDeps())
	live := &liveSession{
		spec:      SessionSpec{RuntimeID: "runtime-1"},
		done:      make(chan struct{}),
		appClient: newAppServerClient(appServerFailingWriter{}, nil),
	}
	manager.sessions[live.spec.RuntimeID] = live
	waiter := &appServerTurnWaiter{
		threadID: "thread-1",
		turnID:   "turn-1",
		ch:       make(chan appServerTurnResult, 1),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	response, err := manager.stopCanceledAppServerTurn(ctx, live, waiter, "prompt context canceled")
	if response.StopReason != "" || err == nil || !strings.Contains(err.Error(), "stop app-server") {
		t.Fatalf("stopCanceledAppServerTurn() response = %#v, error = %v; want unconfirmed stop failure", response, err)
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
		!strings.Contains(err.Error(), "Codex stopped responding after 25ms") ||
		!strings.Contains(err.Error(), "turn was canceled") ||
		strings.Contains(err.Error(), "stderr_tail") ||
		strings.Contains(err.Error(), "runtime_id") {
		t.Fatalf("Prompt() error = %v, want concise canceled-turn message", err)
	}
	events := sink.snapshot()
	if len(events) != 1 || events[0].Kind != SessionEventPromptFailed {
		t.Fatalf("events = %#v, want one prompt failed event", events)
	}
}

func TestAppServerManagerReasoningCountsAsInitialActivity(t *testing.T) {
	withAppServerHelperCommand(t, "prompt-delayed-reasoning")
	originalSemantic := appServerSemanticInactivityTimeout
	originalNoProgress := appServerFirstTurnNoProgressTimeout
	appServerSemanticInactivityTimeout = 100 * time.Millisecond
	appServerFirstTurnNoProgressTimeout = 40 * time.Millisecond
	t.Cleanup(func() {
		appServerSemanticInactivityTimeout = originalSemantic
		appServerFirstTurnNoProgressTimeout = originalNoProgress
	})

	dir := t.TempDir()
	spec := testAppServerSessionSpec(dir)
	manager := newAppServerManager(testAppServerManagerDeps())
	session, err := manager.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })

	resp, err := manager.Prompt(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, PromptRequest{
		SessionID: session.SessionID,
		Prompt:    []PromptContentBlock{TextBlock("think before answering")},
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Fatalf("StopReason = %q, want %q", resp.StopReason, StopReasonEndTurn)
	}
}

func TestAppServerManagerMCPToolCallCountsAsProgress(t *testing.T) {
	withAppServerHelperCommand(t, "prompt-mcp-progress-hangs")
	originalSemantic := appServerSemanticInactivityTimeout
	originalNoProgress := appServerFirstTurnNoProgressTimeout
	appServerSemanticInactivityTimeout = 100 * time.Millisecond
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
		Prompt:    []PromptContentBlock{TextBlock("hang after mcp progress")},
	})
	if err == nil ||
		!strings.Contains(err.Error(), "Codex stopped responding after 100ms") ||
		!strings.Contains(err.Error(), "turn was canceled") {
		t.Fatalf("Prompt() error = %v, want canceled turn after inactivity", err)
	}

	events := sink.snapshot()
	if len(events) < 2 {
		t.Fatalf("events = %#v, want MCP tool event and prompt failure", events)
	}
	if events[0].Kind != SessionEventToolCallStart ||
		events[0].ToolKind != "mcp_tool_call" ||
		events[0].ToolCallID != "call-mcp" {
		t.Fatalf("events[0] = %#v, want MCP tool start", events[0])
	}
	if events[len(events)-1].Kind != SessionEventPromptFailed ||
		!strings.Contains(events[len(events)-1].Error, "Codex stopped responding after 100ms") {
		t.Fatalf("last event = %#v, want semantic timeout failure", events[len(events)-1])
	}
}

func TestAppServerItemIsProgressIncludesInteractiveToolItems(t *testing.T) {
	for _, itemType := range []string{
		"agentMessage",
		"commandExecution",
		"fileChange",
		"mcpToolCall",
		"dynamicToolCall",
		"webSearch",
	} {
		if !appServerItemIsProgress(itemType) {
			t.Fatalf("appServerItemIsProgress(%q) = false, want true", itemType)
		}
	}
	if appServerItemIsProgress("reasoning") {
		t.Fatalf("appServerItemIsProgress(reasoning) = true, want false")
	}
	if !appServerItemSignalsAssistantActivity("reasoning") {
		t.Fatal("appServerItemSignalsAssistantActivity(reasoning) = false, want true")
	}
	if appServerItemSignalsAssistantActivity("userMessage") {
		t.Fatal("appServerItemSignalsAssistantActivity(userMessage) = true, want false")
	}
}

func TestAppServerManagerDefaultNoProgressTimeoutIsFastFail(t *testing.T) {
	if appServerFirstTurnNoProgressTimeout <= 0 {
		t.Fatalf("appServerFirstTurnNoProgressTimeout = %s, want positive fast-fail timeout", appServerFirstTurnNoProgressTimeout)
	}
	if appServerSemanticInactivityTimeout <= appServerFirstTurnNoProgressTimeout {
		t.Fatalf("semantic timeout = %s, no-progress timeout = %s, want semantic timeout longer than no-progress timeout", appServerSemanticInactivityTimeout, appServerFirstTurnNoProgressTimeout)
	}
	if appServerMaximumTurnDuration != 0 {
		t.Fatalf("maximum turn duration = %s, want disabled so active long-running turns can finish", appServerMaximumTurnDuration)
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

func TestAppServerEventAdapterStructuredOutputRawAndLegacyRoutes(t *testing.T) {
	t.Parallel()

	record := `::csgclaw-output::resource_link {"type":"resource_link","name":"docs","uri":"https://example.com/docs"}`
	tests := []struct {
		name string
		run  func(*appServerManager, *liveSession)
	}{
		{
			name: "raw commandExecution",
			run: func(manager *appServerManager, live *liveSession) {
				manager.handleRawItemNotification("runtime-1", live, "main-thread", "item/completed", map[string]any{
					"item": map[string]any{"id": "raw-tool", "type": "commandExecution", "status": "completed", "aggregatedOutput": "ordinary\n" + record},
				})
			},
		},
		{
			name: "legacy exec_command_end",
			run: func(manager *appServerManager, live *liveSession) {
				manager.handleLegacyAppServerEvent("runtime-1", live, map[string]any{
					"type": "exec_command_end", "call_id": "legacy-tool", "output": "ordinary\n" + record,
				})
			},
		},
		{
			name: "legacy function_call_output",
			run: func(manager *appServerManager, live *liveSession) {
				manager.handleLegacyResponseItemEvent("runtime-1", live, map[string]any{
					"type": "function_call_output", "call_id": "response-tool", "output": "ordinary\n" + record,
				})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, live, sink := testAppServerEventAdapter(t)
			test.run(manager, live)
			events := sink.snapshot()
			if len(events) != 2 || events[0].Kind != SessionEventStructuredOutput || events[1].Kind != SessionEventToolCallUpdate {
				t.Fatalf("events = %#v, want structured output then cleaned tool update", events)
			}
			if !strings.Contains(events[1].ToolOutputSummary, "ordinary") || strings.Contains(events[1].ToolOutputSummary, structuredOutputPrefix) {
				t.Fatalf("tool output summary = %q, want ordinary stdout only", events[1].ToolOutputSummary)
			}
		})
	}
}

func TestAppServerEventAdapterStreamsAgentMessageDeltasWithoutCompletedDuplicate(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)
	waiter, err := live.registerAppServerTurnWaiter("main-thread")
	if err != nil {
		t.Fatal(err)
	}

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/started",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turnId":   "turn-1",
			"item": map[string]any{
				"id": "msg-1", "type": "agentMessage", "phase": "final_answer",
			},
		}),
	})
	for _, delta := range []string{"hello", " world"} {
		manager.handleAppServerNotification("runtime-1", live, appServerNotification{
			Method: "item/agentMessage/delta",
			Params: mustJSONRaw(t, map[string]any{
				"threadId": "main-thread",
				"turnId":   "turn-1",
				"itemId":   "msg-1",
				"delta":    delta,
			}),
		})
	}
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turnId":   "turn-1",
			"item": map[string]any{
				"id":   "msg-1",
				"type": "agentMessage",
				"text": "hello world",
			},
		}),
	})

	events := sink.snapshot()
	if len(events) != 2 {
		t.Fatalf("events = %#v, want only two agent message deltas", events)
	}
	if events[0].Text != "hello" || events[1].Text != " world" {
		t.Fatalf("events = %#v, want ordered text deltas", events)
	}
	for _, event := range events {
		if event.Kind != SessionEventTextDelta ||
			event.SessionID != "main-thread" ||
			event.TurnID != "turn-1" ||
			event.MessageID != "msg-1" ||
			event.Payload.(map[string]any)["phase"] != "final_answer" {
			t.Fatalf("event = %#v, want scoped agent message delta", event)
		}
	}
	completed := false
	for len(waiter.ch) > 0 {
		result := <-waiter.ch
		completed = completed || result.success
	}
	if !completed {
		t.Fatal("item/completed without phase did not finish the final-answer turn")
	}
	if phase := live.agentMessagePhase("msg-1"); phase != "" || live.hasStreamedAgentMessage("msg-1") {
		t.Fatalf("completed agent message retained stream state: phase=%q streamed=%v", phase, live.hasStreamedAgentMessage("msg-1"))
	}
}

func TestAppServerEventAdapterIgnoresStaleCompletionForNewTurn(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)
	waiter, err := live.registerAppServerTurnWaiter("main-thread")
	if err != nil {
		t.Fatal(err)
	}
	waiter.setTurnID("turn-new")

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "turn/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turn":     map[string]any{"id": "turn-old", "status": "completed"},
		}),
	})
	if len(waiter.ch) != 0 {
		t.Fatalf("stale completion reached new waiter: %#v", <-waiter.ch)
	}
	if events := sink.snapshot(); len(events) != 0 {
		t.Fatalf("stale completion leaked as runtime event: %#v", events)
	}

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "turn/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turn":     map[string]any{"id": "turn-new", "status": "completed"},
		}),
	})
	select {
	case result := <-waiter.ch:
		if !result.success || result.turnID != "turn-new" {
			t.Fatalf("new turn completion = %#v", result)
		}
	default:
		t.Fatal("matching completion did not reach new waiter")
	}
}

func TestAppServerEventAdapterWaitsForTurnCompletionWhenProviderOmitsAgentMessagePhase(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)
	waiter, err := live.registerAppServerTurnWaiter("main-thread")
	if err != nil {
		t.Fatal(err)
	}
	assertNotCompleted := func(stage string) {
		for len(waiter.ch) > 0 {
			if result := <-waiter.ch; result.success {
				t.Fatalf("turn completed after %s", stage)
			}
		}
	}

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/started",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turnId":   "turn-1",
			"item": map[string]any{
				"id": "msg-qwen", "type": "agentMessage",
			},
		}),
	})
	for _, delta := range []string{"你好", "，世界"} {
		manager.handleAppServerNotification("runtime-1", live, appServerNotification{
			Method: "item/agentMessage/delta",
			Params: mustJSONRaw(t, map[string]any{
				"threadId": "main-thread",
				"turnId":   "turn-1",
				"itemId":   "msg-qwen",
				"delta":    delta,
			}),
		})
	}
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turnId":   "turn-1",
			"item": map[string]any{
				"id": "msg-qwen", "type": "agentMessage", "text": "你好，世界",
			},
		}),
	})
	assertNotCompleted("the pre-tool agent message")

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/started",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turnId":   "turn-1",
			"item": map[string]any{
				"id": "call-search", "type": "mcpToolCall", "server": "wiki", "tool": "search",
			},
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turnId":   "turn-1",
			"item": map[string]any{
				"id": "call-search", "type": "mcpToolCall", "server": "wiki", "tool": "search", "status": "completed",
			},
		}),
	})
	assertNotCompleted("the MCP tool call")

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/started",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turnId":   "turn-1",
			"item": map[string]any{
				"id": "msg-final", "type": "agentMessage",
			},
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/agentMessage/delta",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread", "turnId": "turn-1", "itemId": "msg-final", "delta": "查询结果",
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turnId":   "turn-1",
			"item": map[string]any{
				"id": "msg-final", "type": "agentMessage", "text": "查询结果",
			},
		}),
	})
	assertNotCompleted("the final no-phase agent message")

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "turn/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turn":     map[string]any{"id": "turn-1", "status": "completed"},
		}),
	})
	select {
	case result := <-waiter.ch:
		if !result.success {
			t.Fatalf("turn/completed result = %#v, want success", result)
		}
	default:
		t.Fatal("turn/completed did not finish the turn")
	}

	events := sink.snapshot()
	if len(events) != 5 {
		t.Fatalf("events = %#v, want three text deltas and two MCP events", events)
	}
	for index, want := range []string{"你好", "，世界"} {
		event := events[index]
		if event.Kind != SessionEventTextDelta ||
			event.Text != want ||
			event.Payload.(map[string]any)["phase"] != "final_answer" {
			t.Fatalf("event[%d] = %#v, want final-answer delta %q", index, event, want)
		}
	}
	if events[2].Kind != SessionEventToolCallStart || events[3].Kind != SessionEventToolCallUpdate {
		t.Fatalf("tool events = %#v, want MCP start and update", events[2:4])
	}
	if event := events[4]; event.Kind != SessionEventTextDelta || event.Text != "查询结果" {
		t.Fatalf("final event = %#v, want final streamed text", event)
	}
}

func TestAppServerEventAdapterDoesNotClassifyUnknownAgentDeltaAsFinal(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/agentMessage/delta",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turnId":   "turn-1",
			"itemId":   "msg-1",
			"delta":    "unclassified",
		}),
	})

	events := sink.snapshot()
	if len(events) != 1 || events[0].Payload.(map[string]any)["phase"] != "unknown" {
		t.Fatalf("events = %#v, want unknown phase preserved for downstream filtering", events)
	}
}

func TestAppServerEventAdapterFallsBackToCompletedFinalAfterUnknownDelta(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/agentMessage/delta",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread", "turnId": "turn-1", "itemId": "msg-1", "delta": "answer",
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turnId":   "turn-1",
			"item": map[string]any{
				"id": "msg-1", "type": "agentMessage", "phase": "final_answer", "text": "answer",
			},
		}),
	})

	events := sink.snapshot()
	if len(events) != 2 ||
		events[0].Payload.(map[string]any)["phase"] != "unknown" ||
		events[1].Payload.(map[string]any)["phase"] != "final_answer" {
		t.Fatalf("events = %#v, want unknown delta followed by completed final fallback", events)
	}
}

func TestAppServerEventAdapterDecodesStructuredOutputAfterStreamedDeltas(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)
	record := `::csgclaw-output::resource_link {"type":"resource_link","name":"example","uri":"https://example.com"}`

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/agentMessage/delta",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread", "turnId": "turn-1", "itemId": "msg-1", "delta": record,
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turnId":   "turn-1",
			"item": map[string]any{
				"id": "msg-1", "type": "agentMessage", "phase": "final_answer", "text": record,
			},
		}),
	})

	events := sink.snapshot()
	if len(events) != 1 || events[0].Kind != SessionEventStructuredOutput {
		t.Fatalf("events = %#v, want decoded structured artifact without raw text delta", events)
	}
}

func TestAppServerEventAdapterKeepsLegacyFinalAfterUnclassifiedTypedDeltas(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/event",
		Params: mustJSONRaw(t, map[string]any{"type": "task_started"}),
	})
	for _, delta := range []string{"mixed", " protocol"} {
		manager.handleAppServerNotification("runtime-1", live, appServerNotification{
			Method: "item/agentMessage/delta",
			Params: mustJSONRaw(t, map[string]any{
				"threadId": "main-thread",
				"turnId":   "turn-1",
				"itemId":   "msg-1",
				"delta":    delta,
			}),
		})
	}
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/event",
		Params: mustJSONRaw(t, map[string]any{
			"type":    "agent_message",
			"message": "mixed protocol",
			"phase":   "final_answer",
		}),
	})

	events := sink.snapshot()
	if len(events) != 3 ||
		events[0].Payload.(map[string]any)["phase"] != "unknown" ||
		events[1].Payload.(map[string]any)["phase"] != "unknown" ||
		events[2].Payload.(map[string]any)["phase"] != "final_answer" {
		t.Fatalf("events = %#v, want unclassified typed deltas followed by legacy final", events)
	}
	if live.appProtocol != appServerProtocolLegacy {
		t.Fatalf("protocol = %q, want legacy connection classification", live.appProtocol)
	}
}

func TestAppServerEventAdapterSuppressesResponseItemFinalAfterTypedFinalDeltas(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)

	for _, delta := range []string{"hello", " world"} {
		manager.handleAppServerNotification("runtime-1", live, appServerNotification{
			Method: "item/agentMessage/delta",
			Params: mustJSONRaw(t, map[string]any{
				"threadId": "main-thread",
				"turnId":   "turn-1",
				"itemId":   "msg-1",
				"phase":    "final_answer",
				"delta":    delta,
			}),
		})
	}
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/response_item",
		Params: mustJSONRaw(t, map[string]any{
			"type":  "message",
			"id":    "msg-1",
			"role":  "assistant",
			"phase": "final_answer",
			"content": []map[string]any{
				{"type": "output_text", "text": "hello world"},
			},
		}),
	})

	events := sink.snapshot()
	if len(events) != 2 || events[0].Text != "hello" || events[1].Text != " world" {
		t.Fatalf("events = %#v, want typed deltas without response_item duplicate", events)
	}
}

func TestAppServerEventAdapterHandlesRawTurnCompletedOnLegacyConnection(t *testing.T) {
	manager, live, _ := testAppServerEventAdapter(t)
	waiter, err := live.registerAppServerTurnWaiter("main-thread")
	if err != nil {
		t.Fatal(err)
	}

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/event",
		Params: mustJSONRaw(t, map[string]any{"type": "task_started"}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/agentMessage/delta",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread", "turnId": "turn-1", "itemId": "msg-1", "delta": "answer",
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "turn/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turn":     map[string]any{"id": "turn-1", "status": "completed"},
		}),
	})

	completed := false
	for len(waiter.ch) > 0 {
		result := <-waiter.ch
		completed = completed || result.success
	}
	if !completed {
		t.Fatal("raw turn/completed was ignored after legacy protocol classification")
	}
	if live.appProtocol != appServerProtocolLegacy {
		t.Fatalf("protocol = %q, want legacy classification retained", live.appProtocol)
	}
}

func TestAppServerEventAdapterAccumulatesCanonicalCommandOutputDeltas(t *testing.T) {
	t.Parallel()

	manager, live, sink := testAppServerEventAdapter(t)
	record := `::csgclaw-output::request_user_input {"questions":[{"id":"verification","header":"Checks","question":"How cautious should verification be?","options":[{"label":"Standard","description":"Use targeted checks."}]}]}`
	output := "ordinary stdout\n" + record + "\n"
	split := strings.Index(output, `"question"`)
	for _, delta := range []string{output[:split], output[split:]} {
		manager.handleAppServerNotification("runtime-1", live, appServerNotification{
			Method: "item/commandExecution/outputDelta",
			Params: mustJSONRaw(t, map[string]any{
				"threadId": "main-thread",
				"turnId":   "turn-delta",
				"itemId":   "call-delta",
				"delta":    delta,
			}),
		})
	}
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turnId":   "turn-delta",
			"item": map[string]any{
				"id":               "call-delta",
				"type":             "commandExecution",
				"status":           "completed",
				"aggregatedOutput": "",
			},
		}),
	})

	events := sink.snapshot()
	if len(events) != 2 || events[0].Kind != SessionEventStructuredOutput || events[1].Kind != SessionEventToolCallUpdate {
		t.Fatalf("events = %#v, want structured output before completed tool update", events)
	}
	artifact, ok := events[0].Payload.(activity.StructuredOutputArtifact)
	if !ok || artifact.RequestUserInput == nil || len(artifact.RequestUserInput.Questions) != 1 || artifact.RequestUserInput.Questions[0].ID != "verification" {
		t.Fatalf("structured artifact = %#v, want verification question", events[0].Payload)
	}
	if !strings.Contains(events[1].ToolOutputSummary, "ordinary stdout") || strings.Contains(events[1].ToolOutputSummary, structuredOutputPrefix) {
		t.Fatalf("tool output summary = %q, want ordinary stdout without control record", events[1].ToolOutputSummary)
	}
	if len(live.commandOutputs) != 0 {
		t.Fatalf("command outputs = %#v, want completed item removed", live.commandOutputs)
	}
}

func TestAppServerEventAdapterDeltaDecoderSurvivesLargeOrdinaryOutput(t *testing.T) {
	t.Parallel()

	manager, live, sink := testAppServerEventAdapter(t)
	record := `::csgclaw-output::resource_link {"type":"resource_link","name":"docs","uri":"https://example.com/docs"}`
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/commandExecution/outputDelta",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread", "turnId": "turn-large-delta", "itemId": "call-large-delta",
			"delta": strings.Repeat("x", maxStructuredOutputRecordBytes+1024) + "\n" + record,
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread", "turnId": "turn-large-delta",
			"item": map[string]any{"id": "call-large-delta", "type": "commandExecution", "status": "completed", "aggregatedOutput": ""},
		}),
	})

	events := sink.snapshot()
	if len(events) != 2 || events[0].Kind != SessionEventStructuredOutput {
		t.Fatalf("events = %#v, want structured link after oversized ordinary stdout", events)
	}
	artifact := events[0].Payload.(activity.StructuredOutputArtifact)
	if len(artifact.ResourceLinks) != 1 || artifact.ResourceLinks[0].URI != "https://example.com/docs" {
		t.Fatalf("artifact = %#v, want decoded resource link", artifact)
	}
}

func TestAppServerEventAdapterFailedDeltaOutputDoesNotActivateStructuredRecords(t *testing.T) {
	t.Parallel()

	manager, live, sink := testAppServerEventAdapter(t)
	record := `::csgclaw-output::resource_link {"type":"resource_link","name":"docs","uri":"https://example.com/docs"}`
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/commandExecution/outputDelta",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread", "turnId": "turn-failed-delta", "itemId": "call-failed-delta", "delta": record,
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread", "turnId": "turn-failed-delta",
			"item": map[string]any{"id": "call-failed-delta", "type": "commandExecution", "status": "failed", "aggregatedOutput": ""},
		}),
	})

	events := sink.snapshot()
	if len(events) != 1 || events[0].Kind != SessionEventToolCallUpdate || events[0].ToolStatus != "failed" {
		t.Fatalf("events = %#v, want failed tool update only", events)
	}
	if !strings.Contains(events[0].ToolOutputSummary, structuredOutputPrefix) {
		t.Fatalf("failed output summary = %q, want untouched control record", events[0].ToolOutputSummary)
	}
}

func TestAppServerEventAdapterAggregatedOutputWinsOverDeltaFallback(t *testing.T) {
	t.Parallel()

	manager, live, sink := testAppServerEventAdapter(t)
	deltaRecord := `::csgclaw-output::resource_link {"type":"resource_link","name":"stale","uri":"https://example.com/stale"}`
	aggregatedRecord := `::csgclaw-output::resource_link {"type":"resource_link","name":"current","uri":"https://example.com/current"}`
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/commandExecution/outputDelta",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread", "turnId": "turn-aggregate", "itemId": "call-aggregate", "delta": deltaRecord,
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread", "turnId": "turn-aggregate",
			"item": map[string]any{"id": "call-aggregate", "type": "commandExecution", "status": "completed", "aggregatedOutput": aggregatedRecord},
		}),
	})

	events := sink.snapshot()
	if len(events) != 2 || events[0].Kind != SessionEventStructuredOutput {
		t.Fatalf("events = %#v, want one structured output and tool update", events)
	}
	links := events[0].Payload.(activity.StructuredOutputArtifact).ResourceLinks
	if len(links) != 1 || links[0].URI != "https://example.com/current" {
		t.Fatalf("links = %#v, want authoritative aggregated output only", links)
	}
	if len(live.commandOutputs) != 0 {
		t.Fatalf("command outputs = %#v, want fallback discarded", live.commandOutputs)
	}
}

func TestAppServerEventAdapterTurnCompletionClearsAbandonedDeltaOutput(t *testing.T) {
	t.Parallel()

	manager, live, _ := testAppServerEventAdapter(t)
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/commandExecution/outputDelta",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread", "turnId": "turn-abandoned", "itemId": "call-abandoned", "delta": "partial",
		}),
	})
	if len(live.commandOutputs) != 1 {
		t.Fatalf("command outputs = %#v, want one pending output", live.commandOutputs)
	}
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "turn/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turn":     map[string]any{"id": "turn-abandoned", "status": "cancelled"},
		}),
	})
	if len(live.commandOutputs) != 0 {
		t.Fatalf("command outputs = %#v, want canceled turn cleanup", live.commandOutputs)
	}
}

func TestAppServerEventAdapterRoutesLegacyStructuredOutputToActiveConversationTurn(t *testing.T) {
	t.Parallel()

	manager, live, sink := testAppServerEventAdapter(t)
	live.conversationSessions["room-1"] = "conversation-thread"
	live.trackAppServerTurn("conversation-thread", "turn-3")
	// The compatibility rollout record can arrive after turn/completed and
	// after Prompt has removed its waiter. The turn correlation must survive
	// that ordering so the full output is not assigned to the primary thread.
	if len(live.turnWaiters) != 0 {
		t.Fatalf("turn waiters = %#v, want compatibility output after waiter removal", live.turnWaiters)
	}

	record := `::csgclaw-output::request_user_input {"questions":[{"id":"final_action","header":"Final action","question":"What should happen next?","options":[{"label":"Continue","description":"Run step three."}]}]}`
	manager.handleLegacyResponseItemEvent("runtime-1", live, map[string]any{
		"type":    "function_call_output",
		"call_id": "step-3-tool",
		"output":  "STAGE_3_QUESTIONS_EMITTED\n" + record,
		"internal_chat_message_metadata_passthrough": map[string]any{"turn_id": "turn-3"},
	})
	// The raw duplicate can contain no aggregated output. It must not be the
	// only correctly routed event or the question disappears nondeterministically.
	manager.handleRawItemNotification("runtime-1", live, "conversation-thread", "item/completed", map[string]any{
		"item": map[string]any{"id": "step-3-item", "type": "commandExecution", "status": "completed", "aggregatedOutput": ""},
	})

	events := sink.snapshot()
	if len(events) != 3 {
		t.Fatalf("events = %#v, want structured legacy output plus legacy and raw tool updates", events)
	}
	for index, event := range events {
		if event.SessionID != "conversation-thread" {
			t.Fatalf("events[%d].SessionID = %q, want active conversation thread", index, event.SessionID)
		}
	}
	if events[0].Kind != SessionEventStructuredOutput || events[0].Payload.(activity.StructuredOutputArtifact).RequestUserInput == nil {
		t.Fatalf("events[0] = %#v, want structured request_user_input", events[0])
	}
	if events[1].Kind != SessionEventToolCallUpdate || !strings.Contains(events[1].ToolOutputSummary, "STAGE_3_QUESTIONS_EMITTED") {
		t.Fatalf("events[1] = %#v, want cleaned ordinary legacy stdout", events[1])
	}
	if events[2].Kind != SessionEventToolCallUpdate {
		t.Fatalf("events[2] = %#v, want raw duplicate tool update", events[2])
	}
}

func TestAppServerEventAdapterRoutesAssistantStructuredOutput(t *testing.T) {
	t.Parallel()

	manager, live, sink := testAppServerEventAdapter(t)
	text := "Choose the next step.\n\n:::csgclaw-output::request_user_input\n" +
		`{"questions":[{"id":"next","header":"Next step","question":"What should happen next?","options":[{"label":"Continue","description":"Keep working."}]}]}`
	manager.handleRawItemNotification("runtime-1", live, "main-thread", "item/completed", map[string]any{
		"item": map[string]any{"id": "assistant-question", "type": "agentMessage", "text": text},
	})

	events := sink.snapshot()
	if len(events) != 2 || events[0].Kind != SessionEventStructuredOutput || events[1].Kind != SessionEventTextDelta {
		t.Fatalf("events = %#v, want structured request followed by cleaned assistant text", events)
	}
	request := events[0].Payload.(activity.StructuredOutputArtifact).RequestUserInput
	if request == nil || request.Questions[0].ID != "next" {
		t.Fatalf("request = %+v, want next question", request)
	}
	if events[1].Text != "Choose the next step.\n" {
		t.Fatalf("assistant text = %q, want control record removed", events[1].Text)
	}
}

func TestAppServerEventAdapterIgnoresStructuredOutputFromFailedLegacyCommand(t *testing.T) {
	t.Parallel()

	manager, live, sink := testAppServerEventAdapter(t)
	record := `::csgclaw-output::resource_link {"type":"resource_link","name":"docs","uri":"https://example.com/docs"}`
	manager.handleLegacyResponseItemEvent("runtime-1", live, map[string]any{
		"type": "function_call_output", "call_id": "failed-tool", "output": record + "\nProcess exited with code 1",
	})
	events := sink.snapshot()
	if len(events) != 1 || events[0].Kind != SessionEventToolCallUpdate || events[0].ToolStatus != "failed" {
		t.Fatalf("events = %#v, want failed tool update only", events)
	}
	if !strings.Contains(events[0].ToolOutputSummary, structuredOutputPrefix) {
		t.Fatalf("failed output summary = %q, want untouched control line", events[0].ToolOutputSummary)
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

func TestAppServerEventAdapterLegacyTurnAbortedAcceptsStructuredOutputBoundary(t *testing.T) {
	t.Parallel()

	manager, live, sink := testAppServerEventAdapter(t)
	waiter, err := live.registerAppServerTurnWaiter("main-thread")
	if err != nil {
		t.Fatalf("register waiter: %v", err)
	}
	defer live.removeAppServerTurnWaiter("main-thread", waiter)
	if !waiter.markStructuredOutputBoundary() {
		t.Fatal("mark structured-output boundary = false, want true")
	}

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/event",
		Params: mustJSONRaw(t, map[string]any{
			"type": "turn_aborted",
		}),
	})

	select {
	case result := <-waiter.ch:
		if !result.success || result.err != nil || result.stopReason != StopReasonEndTurn {
			t.Fatalf("turn result = %+v, want successful structured-output boundary", result)
		}
		if result.activity != "legacy:turn_structured-output" {
			t.Fatalf("turn activity = %q, want legacy:turn_structured-output", result.activity)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for legacy structured-output boundary")
	}
	if events := sink.snapshot(); len(events) != 0 {
		t.Fatalf("events = %#v, want no fallback prompt event while waiter is active", events)
	}
}

func TestAppServerEventAdapterLegacyResponseItemExecCommandFallback(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/response_item",
		Params: mustJSONRaw(t, map[string]any{
			"type":      "function_call",
			"name":      "exec_command",
			"call_id":   "call-legacy",
			"arguments": `{"cmd":"cat > hello.py <<'EOF'\nprint(\"Hello, World!\")\nEOF","workdir":"/tmp/work"}`,
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/response_item",
		Params: mustJSONRaw(t, map[string]any{
			"type":    "function_call_output",
			"call_id": "call-legacy",
			"output":  "Process exited with code 1\nOutput:\nzsh:1: operation not permitted: hello.py",
		}),
	})

	events := sink.snapshot()
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2: %#v", len(events), events)
	}
	if events[0].Kind != SessionEventToolCallStart ||
		events[0].ToolCallID != "call-legacy" ||
		events[0].ToolKind != "exec_command" ||
		events[0].ToolStatus != "started" ||
		!strings.Contains(events[0].ToolInputSummary, "hello.py") {
		t.Fatalf("fallback start = %#v, want exec command start", events[0])
	}
	if events[1].Kind != SessionEventToolCallUpdate ||
		events[1].ToolCallID != "call-legacy" ||
		events[1].ToolStatus != "failed" ||
		!strings.Contains(events[1].ToolOutputSummary, "operation not permitted") {
		t.Fatalf("fallback output = %#v, want failed exec command output", events[1])
	}
}

func TestAppServerEventAdapterCanonicalExecCommandSuppressesResponseItemFallback(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/event",
		Params: mustJSONRaw(t, map[string]any{"type": "exec_command_begin", "call_id": "call-1", "command": "go test"}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/event",
		Params: mustJSONRaw(t, map[string]any{"type": "exec_command_end", "call_id": "call-1", "output": "ok"}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/response_item",
		Params: mustJSONRaw(t, map[string]any{
			"type":      "function_call",
			"name":      "exec_command",
			"call_id":   "call-1",
			"arguments": `{"cmd":"duplicate"}`,
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/response_item",
		Params: mustJSONRaw(t, map[string]any{"type": "function_call_output", "call_id": "call-1", "output": "duplicate"}),
	})

	events := sink.snapshot()
	if len(events) != 2 {
		t.Fatalf("events len = %d, want only canonical begin/end: %#v", len(events), events)
	}
	if events[0].ToolInputSummary == "" || strings.Contains(events[0].ToolInputSummary, "duplicate") {
		t.Fatalf("canonical start = %#v, want no fallback duplicate", events[0])
	}
	if events[1].ToolOutputSummary == "" || strings.Contains(events[1].ToolOutputSummary, "duplicate") {
		t.Fatalf("canonical output = %#v, want no fallback duplicate", events[1])
	}
}

func TestAppServerEventAdapterResponseItemFallbackDoesNotLockOutRawEvents(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/response_item",
		Params: mustJSONRaw(t, map[string]any{
			"type":      "function_call",
			"name":      "exec_command",
			"call_id":   "call-legacy",
			"arguments": `{"cmd":"ls"}`,
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "item/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"item": map[string]any{
				"id":   "msg-final",
				"type": "agentMessage",
				"text": "done",
			},
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "turn/completed",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "main-thread",
			"turn": map[string]any{
				"id":     "turn-1",
				"status": "completed",
			},
		}),
	})

	events := sink.snapshot()
	if len(events) != 3 {
		t.Fatalf("events len = %d, want fallback tool, raw final, raw completed: %#v", len(events), events)
	}
	if events[0].Kind != SessionEventToolCallStart || events[0].ToolCallID != "call-legacy" {
		t.Fatalf("first event = %#v, want response_item fallback tool", events[0])
	}
	if events[1].Kind != SessionEventTextDelta || events[1].Text != "done" {
		t.Fatalf("second event = %#v, want raw final text", events[1])
	}
	if events[2].Kind != SessionEventPromptCompleted {
		t.Fatalf("third event = %#v, want raw prompt completed", events[2])
	}
	if live.appProtocol != appServerProtocolRaw {
		t.Fatalf("protocol = %q, want raw", live.appProtocol)
	}
}

func TestAppServerEventAdapterResponseItemMessageFallbackDedupesEventMsg(t *testing.T) {
	manager, live, sink := testAppServerEventAdapter(t)

	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/event",
		Params: mustJSONRaw(t, map[string]any{
			"type":    "agent_message",
			"message": "same final",
			"phase":   "final_answer",
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/response_item",
		Params: mustJSONRaw(t, map[string]any{
			"type":  "message",
			"id":    "msg-duplicate",
			"role":  "assistant",
			"phase": "final_answer",
			"content": []map[string]any{
				{"type": "output_text", "text": "same final"},
			},
		}),
	})
	manager.handleAppServerNotification("runtime-1", live, appServerNotification{
		Method: "codex/response_item",
		Params: mustJSONRaw(t, map[string]any{
			"type":  "message",
			"id":    "msg-fallback",
			"role":  "assistant",
			"phase": "final_answer",
			"content": []map[string]any{
				{"type": "output_text", "text": "response item only final"},
			},
		}),
	})

	events := sink.snapshot()
	if len(events) != 2 {
		t.Fatalf("events len = %d, want canonical final plus response_item-only final: %#v", len(events), events)
	}
	if events[0].Text != "same final" || events[1].Text != "response item only final" {
		t.Fatalf("events = %#v, want deduped response_item message fallback", events)
	}
	if events[1].MessageID != "msg-fallback" {
		t.Fatalf("fallback MessageID = %q, want msg-fallback", events[1].MessageID)
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
	appServerCommandContext = func(ctx context.Context, _ string, _ []string) (*exec.Cmd, error) {
		args := []string{"-test.run=TestAppServerManagerHelperProcess", "--", mode}
		return exec.CommandContext(ctx, os.Args[0], args...), nil
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

type appServerFailingWriter struct{}

func (appServerFailingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
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

func TestAppServerParamsMapOffToCodexNone(t *testing.T) {
	spec := testAppServerSessionSpec(t.TempDir())
	spec.Profile.ReasoningEffort = "off"

	threadConfig := appServerThreadStartParams(spec, false)["config"].(map[string]any)
	if got, want := threadConfig["model_reasoning_effort"], "none"; got != want {
		t.Fatalf("model_reasoning_effort = %v, want %v", got, want)
	}
	turn := appServerTurnStartParams(spec, "thread-1", "hello")
	if got, want := turn["effort"], "none"; got != want {
		t.Fatalf("turn effort = %v, want %v", got, want)
	}
}

func TestAppServerParamsOmitReasoningForModelDefault(t *testing.T) {
	spec := testAppServerSessionSpec(t.TempDir())
	spec.Profile.ReasoningEffort = "auto"

	if config, ok := appServerThreadStartParams(spec, false)["config"]; ok {
		t.Fatalf("thread config = %v, want omitted", config)
	}
	turn := appServerTurnStartParams(spec, "thread-1", "hello")
	if effort, ok := turn["effort"]; ok {
		t.Fatalf("turn effort = %v, want omitted", effort)
	}
}

func TestAppServerReadOnlyParamsAndOverrides(t *testing.T) {
	spec := testAppServerSessionSpec(t.TempDir())
	spec.ExecutionMode = ExecutionModeReadOnly

	thread := appServerThreadStartParams(spec, false)
	if thread["approvalPolicy"] != "never" || thread["permissions"] != readOnlyPermissionsProfile {
		t.Fatalf("thread params = %#v", thread)
	}
	resume := appServerThreadResumeParams(spec, "thread-1")
	if resume["approvalPolicy"] != "never" || resume["permissions"] != readOnlyPermissionsProfile {
		t.Fatalf("resume params = %#v", resume)
	}
	turn := appServerTurnStartParams(spec, "thread-1", "hello")
	if turn["approvalPolicy"] != "never" || turn["permissions"] != readOnlyPermissionsProfile {
		t.Fatalf("turn params = %#v", turn)
	}
	overrides := appServerOverrides(spec)
	joined := strings.Join(overrides, " ")
	for _, want := range []string{"--strict-config", "--disable shell_tool", "--disable unified_exec", `approval_policy="never"`, `default_permissions="csgclaw-read-only"`, `project_doc_max_bytes=0`} {
		if !strings.Contains(joined, want) {
			t.Fatalf("overrides missing %q: %q", want, overrides)
		}
	}
	if strings.Contains(joined, "tools.view_image") {
		t.Fatalf("overrides contain unsupported tools.view_image setting: %q", overrides)
	}
	projectOverride := fmt.Sprintf(`projects={%s={trust_level="untrusted"}}`, tomlQuotedKey(spec.WorkspaceDir))
	if !strings.Contains(joined, projectOverride) {
		t.Fatalf("overrides missing project trust override %q: %q", projectOverride, overrides)
	}
}

func TestAppServerThreadStartRegistersPublishFileDynamicTool(t *testing.T) {
	spec := testAppServerSessionSpec(t.TempDir())
	params := appServerThreadStartParams(spec, true)
	tools, ok := params["dynamicTools"].([]map[string]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("dynamicTools = %#v, want one typed tool", params["dynamicTools"])
	}
	tool := tools[0]
	description := strings.TrimSpace(fmt.Sprint(tool["description"]))
	if tool["name"] != "csgclaw_publish_file" || description == "" {
		t.Fatalf("publish file tool = %#v", tool)
	}
	for _, want := range []string{"generate, send, return, or download a file", "call this tool immediately after creating it", "Do not search for or use csgclaw-cli, curl, Feishu APIs, or other upload methods"} {
		if !strings.Contains(description, want) {
			t.Fatalf("publish file tool description missing %q in %q", want, description)
		}
	}
	schema, _ := tool["inputSchema"].(map[string]any)
	properties, _ := schema["properties"].(map[string]any)
	required, _ := schema["required"].([]string)
	if schema["type"] != "object" || properties["path"] == nil || properties["name"] == nil || properties["mimeType"] == nil || len(required) != 1 || required[0] != "path" || schema["additionalProperties"] != false {
		t.Fatalf("publish file input schema = %#v", schema)
	}
}

func TestAppServerThreadStartOmitsPublishFileToolForLegacyThread(t *testing.T) {
	params := appServerThreadStartParams(testAppServerSessionSpec(t.TempDir()), false)
	if tools, ok := params["dynamicTools"]; ok {
		t.Fatalf("legacy dynamicTools = %#v, want omitted", tools)
	}
}

func TestAppServerPublishFileDynamicToolEmitsTypedFileEvent(t *testing.T) {
	sink := &recordingSink{}
	manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
	live := &liveSession{spec: testAppServerSessionSpec(t.TempDir()), filePublishingThreads: map[string]bool{"thread-1": true}}
	response, err := manager.handleAppServerServerRequest("runtime-1", live, appServerServerRequest{
		Method: "item/tool/call",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "thread-1",
			"turnId":   "turn-1",
			"callId":   "call-1",
			"tool":     "csgclaw_publish_file",
			"arguments": map[string]any{
				"path": "reports/result.pdf", "mimeType": "application/pdf",
			},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	result, _ := response.(map[string]any)
	items, _ := result["contentItems"].([]map[string]any)
	if result["success"] != true || len(items) != 1 || items[0]["type"] != "inputText" {
		t.Fatalf("dynamic tool response = %#v", response)
	}
	events := sink.snapshot()
	if len(events) != 1 || events[0].Kind != activity.RuntimeEventFileOutput || events[0].RuntimeID != "runtime-1" || events[0].SessionID != "thread-1" || events[0].TurnID != "turn-1" || events[0].ToolCallID != "call-1" {
		t.Fatalf("file events = %#v", events)
	}
	file, ok := events[0].Payload.(activity.RuntimeFile)
	if !ok || file.Path != filepath.Join("reports", "result.pdf") || file.Name != "result.pdf" || file.MIMEType != "application/pdf" {
		t.Fatalf("file event payload = %#v", events[0].Payload)
	}
}

func TestAppServerPublishFileDynamicToolRejectsUnsafeArguments(t *testing.T) {
	tests := []struct {
		name      string
		arguments map[string]any
	}{
		{name: "absolute path", arguments: map[string]any{"path": "/etc/passwd"}},
		{name: "path traversal", arguments: map[string]any{"path": "../secret.txt"}},
		{name: "nested delivery name", arguments: map[string]any{"path": "report.txt", "name": "nested/report.txt"}},
		{name: "dot delivery name", arguments: map[string]any{"path": "report.txt", "name": ".."}},
		{name: "control character", arguments: map[string]any{"path": "report.txt", "name": "line\nbreak.txt"}},
		{name: "oversized name", arguments: map[string]any{"path": "report.txt", "name": strings.Repeat("a", 256)}},
		{name: "invalid MIME type", arguments: map[string]any{"path": "report.txt", "mimeType": "not a mime"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink := &recordingSink{}
			manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
			response, err := manager.handleAppServerServerRequest("runtime-1", &liveSession{filePublishingThreads: map[string]bool{"thread-1": true}}, appServerServerRequest{
				Method: "item/tool/call",
				Params: mustJSONRaw(t, map[string]any{
					"threadId": "thread-1", "turnId": "turn-1", "callId": "call-1", "tool": appServerPublishFileToolName,
					"arguments": test.arguments,
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			result, _ := response.(map[string]any)
			if result["success"] != false {
				t.Fatalf("response = %#v, want typed failure", response)
			}
			if events := sink.snapshot(); len(events) != 0 {
				t.Fatalf("unsafe file events = %#v", events)
			}
		})
	}
}

func TestAppServerPublishFileDynamicToolRejectsUnownedThread(t *testing.T) {
	manager := newAppServerManager(testAppServerManagerDepsWithSink(&recordingSink{}))
	live := &liveSession{filePublishingThreads: map[string]bool{"owned-thread": true}}
	_, err := manager.handleAppServerServerRequest("runtime-1", live, appServerServerRequest{
		Method: "item/tool/call",
		Params: mustJSONRaw(t, map[string]any{
			"threadId": "other-thread", "turnId": "turn-1", "callId": "call-1", "tool": appServerPublishFileToolName,
			"arguments": map[string]any{"path": "report.txt"},
		}),
	})
	if err == nil || !strings.Contains(err.Error(), "unknown or unsupported thread") {
		t.Fatalf("unowned dynamic tool error = %v", err)
	}
}

func TestAppServerReadOnlyDeclinesMutationApprovals(t *testing.T) {
	manager := newAppServerManager(testAppServerManagerDeps())
	live := &liveSession{spec: SessionSpec{ExecutionMode: ExecutionModeReadOnly}}
	for _, method := range []string{"item/commandExecution/requestApproval", "item/fileChange/requestApproval"} {
		got, err := manager.handleAppServerServerRequest("runtime-1", live, appServerServerRequest{Method: method})
		if err != nil {
			t.Fatalf("%s error = %v", method, err)
		}
		decision, _ := got.(map[string]any)
		if decision["decision"] != "decline" {
			t.Fatalf("%s decision = %#v", method, got)
		}
	}
	got, err := manager.handleAppServerServerRequest("runtime-1", live, appServerServerRequest{Method: "item/permissions/requestApproval"})
	if err != nil {
		t.Fatal(err)
	}
	permissionResponse, _ := got.(map[string]any)
	permissions, ok := permissionResponse["permissions"].(map[string]any)
	if !ok || len(permissions) != 0 || permissionResponse["scope"] != "turn" {
		t.Fatalf("permission response = %#v", got)
	}
	got, err = manager.handleAppServerServerRequest("runtime-1", live, appServerServerRequest{Method: "mcpServer/elicitation/request"})
	if err != nil {
		t.Fatal(err)
	}
	if response, _ := got.(map[string]any); response["action"] != "decline" {
		t.Fatalf("elicitation response = %#v", got)
	}
}

func TestAppServerTurnStartPreservesNativeBlocksAndClientMessageID(t *testing.T) {
	spec := testAppServerSessionSpec(t.TempDir())
	input := []map[string]any{
		{"type": "text", "text": "before"},
		{"type": "localImage", "path": "/runtime/image.png"},
		{"type": "text", "text": "after"},
	}
	params := appServerTurnStartParamsWithInput(spec, "thread-1", input, "caller-turn")
	if got := params["clientUserMessageId"]; got != "caller-turn" {
		t.Fatalf("clientUserMessageId = %v", got)
	}
	gotInput, ok := params["input"].([]map[string]any)
	if !ok || len(gotInput) != 3 || gotInput[1]["type"] != "localImage" || gotInput[1]["path"] != "/runtime/image.png" {
		t.Fatalf("input = %#v", params["input"])
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
	case "conversation-thread-notification-before-result":
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			if msg["method"] != "thread/start" {
				return nil, false
			}
			if index == 1 {
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			}
			threadID := fmt.Sprintf("conversation-thread-%d", index)
			writeRPCNotification(t, "thread/started", map[string]any{"threadId": threadID})
			return rpcResult(msg["id"], map[string]any{"threadId": threadID}), true
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
	case "memory-checkpoint":
		threadStarts := 0
		forks := 0
		roomTurns := 0
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			switch msg["method"] {
			case "thread/start":
				threadStarts++
				threadID := "main-thread"
				if threadStarts > 1 {
					threadID = "room-thread"
				}
				return rpcResult(msg["id"], map[string]any{"threadId": threadID}), true
			case "thread/fork":
				forks++
				if forks > 1 {
					t.Fatalf("checkpoint fork count = %d, want one within throttle interval", forks)
				}
				params, _ := msg["params"].(map[string]any)
				if params["threadId"] != "room-thread" {
					t.Fatalf("checkpoint fork params = %#v", params)
				}
				if entered := strings.TrimSpace(os.Getenv("CSGCLAW_MEMORY_CHECKPOINT_FORK_ENTERED")); entered != "" {
					if err := os.WriteFile(entered, []byte("entered"), 0o600); err != nil {
						t.Fatalf("write fork-entered marker: %v", err)
					}
					release := os.Getenv("CSGCLAW_MEMORY_CHECKPOINT_FORK_RELEASE")
					deadline := time.Now().Add(2 * time.Second)
					for {
						if _, err := os.Stat(release); err == nil {
							break
						}
						if time.Now().After(deadline) {
							t.Fatalf("timed out waiting to release blocked checkpoint fork")
						}
						time.Sleep(5 * time.Millisecond)
					}
				}
				if os.Getenv("CSGCLAW_MEMORY_CHECKPOINT_FORK_ERROR") != "" {
					return rpcError(msg["id"], -32000, "fork unavailable"), true
				}
				return rpcResult(msg["id"], map[string]any{"thread": map[string]any{"id": "checkpoint-thread"}}), true
			case "thread/delete":
				params, _ := msg["params"].(map[string]any)
				if params["threadId"] != "checkpoint-thread" {
					t.Fatalf("checkpoint delete params = %#v", params)
				}
				if err := os.WriteFile(os.Getenv("CSGCLAW_MEMORY_CHECKPOINT_DELETE_MARKER"), []byte("deleted"), 0o600); err != nil {
					t.Fatalf("write checkpoint delete marker: %v", err)
				}
				return rpcResult(msg["id"], map[string]any{}), true
			case "turn/start":
				params, _ := msg["params"].(map[string]any)
				threadID, _ := params["threadId"].(string)
				turnID := "turn-room"
				message := "room done"
				if threadID == "main-thread" {
					turnID = "turn-maintenance"
					message = "OK"
					assertTurnStartParams(t, msg, "main-thread", "medium", memoryMaintenancePrompt)
				} else {
					roomTurns++
					prompt := "remember my preference"
					if roomTurns > 1 {
						prompt = "second preference"
					}
					assertTurnStartParams(t, msg, "room-thread", "medium", prompt)
				}
				writeRPCNotification(t, "turn/started", map[string]any{"threadId": threadID, "turn": map[string]any{"id": turnID}})
				writeRPCNotification(t, "item/completed", map[string]any{"threadId": threadID, "turnId": turnID, "item": map[string]any{"id": "message-" + turnID, "type": "agentMessage", "text": message}})
				writeRPCNotification(t, "turn/completed", map[string]any{"threadId": threadID, "turn": map[string]any{"id": turnID, "status": "completed"}})
				return rpcResult(msg["id"], map[string]any{"turnId": turnID}), true
			default:
				return nil, false
			}
		})
	case "prompt-publish-file":
		awaitingTool := false
		threadStarts := 0
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			if awaitingTool {
				assertServerRequestResponse(t, msg, 9010, func(result map[string]any) {
					if result["success"] != true {
						t.Fatalf("publish file result = %#v", result)
					}
					items, _ := result["contentItems"].([]any)
					if len(items) != 1 {
						t.Fatalf("publish file contentItems = %#v", result["contentItems"])
					}
				})
				awaitingTool = false
				writeRPCNotification(t, "item/completed", map[string]any{"threadId": "engine-thread", "item": map[string]any{"id": "message-1", "type": "agentMessage", "text": "published"}})
				writeRPCNotification(t, "turn/completed", map[string]any{"threadId": "engine-thread", "turn": map[string]any{"id": "turn-publish", "status": "completed"}})
				return nil, false
			}
			switch msg["method"] {
			case "thread/start":
				params, _ := msg["params"].(map[string]any)
				tools, _ := params["dynamicTools"].([]any)
				if threadStarts == 0 {
					threadStarts++
					if len(tools) != 0 {
						t.Fatalf("primary thread dynamicTools = %#v, want none", params["dynamicTools"])
					}
					return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
				}
				if len(tools) != 1 {
					t.Fatalf("thread/start dynamicTools = %#v", params["dynamicTools"])
				}
				tool, _ := tools[0].(map[string]any)
				if tool["name"] != appServerPublishFileToolName {
					t.Fatalf("thread/start publish tool = %#v", tool)
				}
				return rpcResult(msg["id"], map[string]any{"threadId": "engine-thread"}), true
			case "turn/start":
				awaitingTool = true
				writeRPCServerRequest(t, 9010, "item/tool/call", map[string]any{
					"threadId": "engine-thread", "turnId": "turn-publish", "callId": "call-publish", "tool": appServerPublishFileToolName,
					"arguments": map[string]any{"path": "reports/result.pdf", "name": "result.pdf", "mimeType": "application/pdf"},
				})
				return rpcResult(msg["id"], map[string]any{"turnId": "turn-publish"}), true
			default:
				return nil, false
			}
		})
	case "prompt-user-input-complete":
		awaitingAnswer := false
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			if awaitingAnswer {
				assertServerRequestResponse(t, msg, 9002, func(result map[string]any) {
					answers, _ := result["answers"].(map[string]any)
					color, _ := answers["color"].(map[string]any)
					values, _ := color["answers"].([]any)
					if len(values) != 2 || values[0] != "Green" || values[1] != "user_note: deep shade" {
						t.Fatalf("user-input response = %#v, want selected label and user note", result)
					}
				})
				awaitingAnswer = false
				writeRPCNotification(t, "item/completed", map[string]any{"threadId": "main-thread", "item": map[string]any{"id": "item-final", "type": "agentMessage", "text": "continued with Green"}})
				writeRPCNotification(t, "turn/completed", map[string]any{"threadId": "main-thread", "turn": map[string]any{"id": "turn-question", "status": "completed"}})
				return nil, false
			}
			switch msg["method"] {
			case "thread/start":
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			case "turn/start":
				writeRPCNotification(t, "turn/started", map[string]any{"threadId": "main-thread", "turn": map[string]any{"id": "turn-question"}})
				awaitingAnswer = true
				writeRPCServerRequest(t, 9002, "item/tool/requestUserInput", map[string]any{
					"threadId": "main-thread",
					"turnId":   "turn-question",
					"itemId":   "item-question",
					"questions": []map[string]any{{
						"id": "color", "header": "Color", "question": "Choose a color",
						"options": []map[string]any{{"label": "Blue", "description": "cool"}, {"label": "Green", "description": "natural"}},
						"isOther": false, "isSecret": false,
					}},
				})
				return rpcResult(msg["id"], map[string]any{"turnId": "turn-question"}), true
			default:
				return nil, false
			}
		})
	case "prompt-agent-message-before-user-input":
		awaitingAnswer := false
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			if awaitingAnswer {
				assertServerRequestResponse(t, msg, 9003, func(result map[string]any) {
					answers, _ := result["answers"].(map[string]any)
					next, _ := answers["next"].(map[string]any)
					values, _ := next["answers"].([]any)
					if len(values) != 1 || values[0] != "Continue" {
						t.Fatalf("user-input response = %#v, want selected label", result)
					}
				})
				awaitingAnswer = false
				writeRPCNotification(t, "item/completed", map[string]any{"threadId": "main-thread", "item": map[string]any{"id": "item-final", "type": "agentMessage", "text": "continued"}})
				writeRPCNotification(t, "turn/completed", map[string]any{"threadId": "main-thread", "turn": map[string]any{"id": "turn-question", "status": "completed"}})
				return nil, false
			}
			switch msg["method"] {
			case "thread/start":
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			case "turn/start":
				assertTurnStartParams(t, msg, "main-thread", "medium", "report then ask me")
				writeRPCNotification(t, "turn/started", map[string]any{"threadId": "main-thread", "turn": map[string]any{"id": "turn-question"}})
				writeRPCNotification(t, "item/completed", map[string]any{"threadId": "main-thread", "item": map[string]any{"id": "item-progress", "type": "agentMessage", "text": "report ready"}})
				awaitingAnswer = true
				writeRPCServerRequest(t, 9003, "item/tool/requestUserInput", map[string]any{
					"threadId": "main-thread",
					"turnId":   "turn-question",
					"itemId":   "item-question",
					"questions": []map[string]any{{
						"id": "next", "header": "Next", "question": "What next?",
						"options": []map[string]any{{"label": "Continue", "description": "keep going"}},
						"isOther": false, "isSecret": false,
					}},
				})
				return rpcResult(msg["id"], map[string]any{"turnId": "turn-question"}), true
			default:
				return nil, false
			}
		})
	case "prompt-legacy-rollout-complete":
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			switch msg["method"] {
			case "thread/start":
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			case "turn/start":
				assertTurnStartParams(t, msg, "main-thread", "medium", "put it in a file")
				writeRolloutRecord(t, "event_msg", map[string]any{"type": "agent_message", "message": "checking workspace", "phase": "commentary"})
				writeRolloutRecord(t, "response_item", map[string]any{
					"type":      "function_call",
					"name":      "exec_command",
					"call_id":   "call-write",
					"arguments": `{"cmd":"printf 'print(\"Hello, World!\")\n' > hello.py","workdir":"/tmp/work"}`,
				})
				writeRolloutRecord(t, "response_item", map[string]any{
					"type":    "function_call_output",
					"call_id": "call-write",
					"output":  "Process exited with code 0\nOutput:\n-rw-r--r-- hello.py",
				})
				writeRolloutRecord(t, "response_item", map[string]any{
					"type":  "message",
					"id":    "msg-final",
					"role":  "assistant",
					"phase": "final_answer",
					"content": []map[string]any{
						{"type": "output_text", "text": "已经放到文件里了"},
					},
				})
				writeRolloutRecord(t, "event_msg", map[string]any{"type": "task_complete", "turn_id": "turn-legacy"})
				return rpcResult(msg["id"], map[string]any{"turnId": "turn-legacy"}), true
			default:
				return nil, false
			}
		})
	case "prompt-large-command-output":
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			switch msg["method"] {
			case "thread/start":
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			case "turn/start":
				largeOutput := strings.Repeat("room-list-line\n", 128*1024)
				writeRPCNotification(t, "turn/started", map[string]any{"threadId": "main-thread", "turn": map[string]any{"id": "turn-large-output"}})
				writeRPCNotification(t, "item/started", map[string]any{"threadId": "main-thread", "item": map[string]any{"id": "call-large", "type": "commandExecution", "command": "csgclaw-cli room list"}})
				writeRPCNotification(t, "item/completed", map[string]any{"threadId": "main-thread", "item": map[string]any{"id": "call-large", "type": "commandExecution", "aggregatedOutput": largeOutput}})
				writeRPCNotification(t, "item/completed", map[string]any{"threadId": "main-thread", "item": map[string]any{"id": "item-large", "type": "agentMessage", "text": "done"}})
				writeRPCNotification(t, "turn/completed", map[string]any{"threadId": "main-thread", "turn": map[string]any{"id": "turn-large-output", "status": "completed"}})
				return rpcResult(msg["id"], map[string]any{"turnId": "turn-large-output"}), true
			default:
				return nil, false
			}
		})
	case "prompt-command-output-delta":
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			switch msg["method"] {
			case "thread/start":
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			case "turn/start":
				record := `::csgclaw-output::request_user_input {"questions":[{"id":"step_two","header":"Step two","question":"Choose step two.","options":[{"label":"Continue","description":"Continue the demo."}]}]}`
				output := "ordinary\n" + record + "\n"
				split := strings.Index(output, `"question"`)
				writeRPCNotification(t, "turn/started", map[string]any{"threadId": "main-thread", "turn": map[string]any{"id": "turn-delta"}})
				writeRPCNotification(t, "item/started", map[string]any{"threadId": "main-thread", "turnId": "turn-delta", "item": map[string]any{"id": "call-delta", "type": "commandExecution", "command": "emit demo"}})
				writeRPCNotification(t, "item/commandExecution/outputDelta", map[string]any{"threadId": "main-thread", "turnId": "turn-delta", "itemId": "call-delta", "delta": output[:split]})
				writeRPCNotification(t, "item/commandExecution/outputDelta", map[string]any{"threadId": "main-thread", "turnId": "turn-delta", "itemId": "call-delta", "delta": output[split:]})
				writeRPCNotification(t, "item/completed", map[string]any{"threadId": "main-thread", "turnId": "turn-delta", "item": map[string]any{"id": "call-delta", "type": "commandExecution", "status": "completed", "aggregatedOutput": ""}})
				return rpcResult(msg["id"], map[string]any{"turnId": "turn-delta"}), true
			case "turn/interrupt":
				assertTurnInterruptParams(t, msg, "main-thread", "turn-delta")
				writeRPCNotification(t, "turn/completed", map[string]any{
					"threadId": "main-thread",
					"turn":     map[string]any{"id": "turn-delta", "status": "interrupted"},
				})
				return rpcResult(msg["id"], map[string]any{}), true
			default:
				return nil, false
			}
		})
	case "prompt-cancel-confirmed":
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			switch msg["method"] {
			case "thread/start":
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			case "turn/start":
				writeRPCNotification(t, "turn/started", map[string]any{
					"threadId": "main-thread",
					"turn":     map[string]any{"id": "turn-cancel"},
				})
				return rpcResult(msg["id"], map[string]any{"turnId": "turn-cancel"}), true
			case "turn/interrupt":
				assertTurnInterruptParams(t, msg, "main-thread", "turn-cancel")
				writeRPCNotification(t, "turn/completed", map[string]any{
					"threadId": "main-thread",
					"turn":     map[string]any{"id": "turn-cancel", "status": "interrupted"},
				})
				return rpcResult(msg["id"], map[string]any{}), true
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
			case "turn/interrupt":
				assertTurnInterruptParams(t, msg, "main-thread", "turn-hang")
				writeRPCNotification(t, "turn/completed", map[string]any{
					"threadId": "main-thread",
					"turn":     map[string]any{"id": "turn-hang", "status": "interrupted"},
				})
				return rpcResult(msg["id"], map[string]any{}), true
			default:
				return nil, false
			}
		})
	case "prompt-delayed-reasoning":
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			switch msg["method"] {
			case "thread/start":
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			case "turn/start":
				writeRPCNotification(t, "turn/started", map[string]any{"threadId": "main-thread", "turn": map[string]any{"id": "turn-reasoning"}})
				go func() {
					time.Sleep(20 * time.Millisecond)
					writeRPCNotification(t, "item/started", map[string]any{
						"threadId": "main-thread",
						"item":     map[string]any{"id": "reasoning-1", "type": "reasoning"},
					})
					time.Sleep(50 * time.Millisecond)
					writeRPCNotification(t, "item/completed", map[string]any{
						"threadId": "main-thread",
						"item":     map[string]any{"id": "message-1", "type": "agentMessage", "text": "done"},
					})
					writeRPCNotification(t, "turn/completed", map[string]any{
						"threadId": "main-thread",
						"turn":     map[string]any{"id": "turn-reasoning", "status": "completed"},
					})
				}()
				return rpcResult(msg["id"], map[string]any{"turnId": "turn-reasoning"}), true
			default:
				return nil, false
			}
		})
	case "prompt-mcp-progress-hangs":
		runAppServerHelper(t, func(index int, msg map[string]any) (map[string]any, bool) {
			switch msg["method"] {
			case "thread/start":
				return rpcResult(msg["id"], map[string]any{"threadId": "main-thread"}), true
			case "turn/start":
				writeRPCNotification(t, "turn/started", map[string]any{"threadId": "main-thread", "turn": map[string]any{"id": "turn-mcp-hang"}})
				writeRPCNotification(t, "item/started", map[string]any{
					"threadId": "main-thread",
					"item": map[string]any{
						"id":        "call-mcp",
						"type":      "mcpToolCall",
						"server":    "grafana_opencsg_stg_readonly",
						"tool":      "query_loki_logs",
						"arguments": map[string]any{},
						"status":    "in_progress",
					},
				})
				return rpcResult(msg["id"], map[string]any{"turnId": "turn-mcp-hang"}), true
			case "turn/interrupt":
				assertTurnInterruptParams(t, msg, "main-thread", "turn-mcp-hang")
				writeRPCNotification(t, "turn/completed", map[string]any{
					"threadId": "main-thread",
					"turn":     map[string]any{"id": "turn-mcp-hang", "status": "interrupted"},
				})
				return rpcResult(msg["id"], map[string]any{}), true
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

func writeRolloutRecord(t *testing.T, recordType string, payload any) {
	t.Helper()
	msg := map[string]any{
		"type":    recordType,
		"payload": payload,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal rollout record: %v", err)
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

func assertTurnInterruptParams(t *testing.T, msg map[string]any, wantThreadID, wantTurnID string) {
	t.Helper()
	params, _ := msg["params"].(map[string]any)
	if params == nil {
		t.Fatalf("turn/interrupt params = %#v, want object", msg["params"])
	}
	if got := params["threadId"]; got != wantThreadID {
		t.Fatalf("turn/interrupt threadId = %#v, want %q", got, wantThreadID)
	}
	if got := params["turnId"]; got != wantTurnID {
		t.Fatalf("turn/interrupt turnId = %#v, want %q", got, wantTurnID)
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
