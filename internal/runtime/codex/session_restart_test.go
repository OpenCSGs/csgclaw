package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/turnjournal"
	"csgclaw/internal/taskconfig"
)

func TestCodexDefaultThreadAcrossRestart(t *testing.T) {
	withAppServerHelperCommand(t, "room-configuration")
	t.Setenv("CODEX_HOME", t.TempDir())
	root := t.TempDir()
	spec := agentruntime.Spec{AgentID: "agent-alice", AgentName: "alice", RuntimeID: "rt-agent-alice"}
	deps := Dependencies{
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: spec.AgentID, Name: spec.AgentName, RuntimeID: h.RuntimeID}, nil
		},
		ResolveRoomProfile: func(string, string, taskconfig.Config) (agentruntime.Profile, error) {
			return agentruntime.Profile{}, nil
		},
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		AgentHome:      func(id string) (string, error) { return filepath.Join(root, id), nil },
	}
	start := func() (*Runtime, agentruntime.Handle) {
		t.Helper()
		rt := New(deps)
		t.Cleanup(func() { _ = rt.Close() })
		h, err := rt.New(t.Context(), spec)
		if err != nil {
			t.Fatal(err)
		}
		return rt, h
	}
	// Simulate a beta runtime stopped after creating an unused default thread.
	stateDir := filepath.Join(root, spec.AgentID, hostStateDirName)
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(stateDir, sessionFileName)
	legacy := []byte(`{"session_id":"unused","empty_threads":{"unused":true,"idle-room":true},"conversation_sessions":{"new":"idle-room","used":"history"}}`)
	if err := os.WriteFile(statePath, legacy, 0600); err != nil {
		t.Fatal(err)
	}

	first, h := start()
	if h.HandleID != "unused" {
		t.Fatalf("idle handle = %q", h.HandleID)
	}
	data, err := os.ReadFile(statePath)
	if err != nil || strings.Contains(string(data), "empty_threads") {
		t.Fatalf("unexpected migrated session metadata: %s, %v", data, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, h := start()
	idle, err := second.readSessionMetadata(spec.RuntimeID)
	if err != nil || idle.SessionID != "unused" || idle.ConversationSessions["new"] != "idle-room" || idle.ConversationSessions["used"] != "history" {
		t.Fatalf("idle session after restart = %q, %v", idle.SessionID, err)
	}
	if err := second.Prompt(t.Context(), spec.RuntimeID, h.HandleID, "hello"); err != nil {
		t.Fatal(err)
	}
	used, err := second.readSessionMetadata(spec.RuntimeID)
	if err != nil || used.SessionID == "" || used.SessionID == "unused" {
		t.Fatalf("saved default = %q, %v", used.SessionID, err)
	}
	room, err := second.EnsureSession(t.Context(), spec.RuntimeID, "new")
	if err != nil || room == "" || room == "idle-room" {
		t.Fatalf("new room = %q, %v", room, err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	third, _ := start()
	if id, ok, err := third.ExistingEngineSession(t.Context(), spec.RuntimeID, ""); err != nil || !ok || id != used.SessionID {
		t.Fatalf("existing default after restart = %q, %v, %v", id, ok, err)
	}
	if err := third.Prompt(t.Context(), spec.RuntimeID, "", "again"); err != nil {
		t.Fatal(err)
	}
	meta, err := third.readSessionMetadata(spec.RuntimeID)
	if err != nil || meta.SessionID != used.SessionID {
		t.Fatalf("resumed default = %q, want %q, error %v", meta.SessionID, used.SessionID, err)
	}
}

func TestFirstTurnRequiresDurableHistoryState(t *testing.T) {
	withAppServerHelperCommand(t, "room-configuration")
	spec := testAppServerSessionSpec(t.TempDir())
	deps := testAppServerManagerDeps()
	deps.SaveSessionMetadata = func(_ string, meta sessionMetadata) error { return errors.New("write failed") }
	manager := newAppServerManager(deps)
	session, err := manager.Start(t.Context(), spec)
	if err != nil {
		t.Fatal(err)
	}
	handle := SessionHandle{RuntimeID: spec.RuntimeID}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), handle) })
	_, err = manager.Prompt(t.Context(), handle, PromptRequest{SessionID: session.SessionID, Prompt: []PromptContentBlock{TextBlock("message")}})
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("Prompt() error = %v", err)
	}
	if current, err := manager.LiveSession(handle); err != nil || current.SessionID != "" {
		t.Fatalf("failed write published a thread: %v, %v", current, err)
	}
	log, err := os.ReadFile(spec.StderrPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "method=initialize") || !strings.Contains(string(log), "duration_ms=") {
		t.Fatal("missing startup RPC timing")
	}
}

func TestRestartRetainsRuntimeIDWhenSessionFileIsMissing(t *testing.T) {
	withAppServerHelperCommand(t, "resume-success")
	spec := testAppServerSessionSpec(t.TempDir())
	if err := os.WriteFile(filepath.Join(spec.RuntimeDir, runtimeFileName), []byte(`{"session_id":"original"}`), 0600); err != nil {
		t.Fatal(err)
	}
	manager := newAppServerManager(testAppServerManagerDeps())
	session, err := manager.Start(t.Context(), spec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}) })
	if session.SessionID != "original" {
		t.Fatalf("restored %q, want original", session.SessionID)
	}
}

func TestColdRoomRejectsDifferentResumedID(t *testing.T) {
	withAppServerHelperCommand(t, "resume-mismatched-id")
	spec := testAppServerSessionSpec(t.TempDir())
	spec.ConversationSessions = map[string]string{"room": "original"}
	deps := testAppServerManagerDeps()
	deps.SaveSessionMetadata = func(string, sessionMetadata) error { t.Fatal("rewrote room binding after failed restore"); return nil }
	manager := newAppServerManager(deps)
	if _, err := manager.Start(t.Context(), spec); err != nil {
		t.Fatal(err)
	}
	handle := SessionHandle{RuntimeID: spec.RuntimeID}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), handle) })
	if _, err := manager.EnsureSession(t.Context(), handle, "room"); err == nil {
		t.Fatal("accepted a different thread ID")
	}
	live := manager.liveSession(spec.RuntimeID)
	live.mu.Lock()
	defer live.mu.Unlock()
	if live.conversationSessions["room"] != "original" || live.loadedConversations["room"] {
		t.Fatalf("lost saved room binding: %v", live.conversationSessions)
	}
}

func TestColdRoomRetainsUnknownThreadWhenToolSetChanges(t *testing.T) {
	withAppServerHelperCommand(t, "room-configuration")
	spec := testAppServerSessionSpec(t.TempDir())
	spec.ConversationSessions = map[string]string{"room": "original"}
	manager := newAppServerManager(testAppServerManagerDeps())
	if _, err := manager.Start(t.Context(), spec); err != nil {
		t.Fatal(err)
	}
	handle := SessionHandle{RuntimeID: spec.RuntimeID}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), handle) })
	if _, err := manager.EnsureEngineSession(t.Context(), handle, "room"); err == nil {
		t.Fatal("replaced an existing room to change its tools")
	}
	if id, err := manager.EnsureSession(t.Context(), handle, "room"); err != nil || id != "original" {
		t.Fatalf("saved thread = %q, %v", id, err)
	}
}

func TestRestartAfterLegacyRoomImportKeepsPrimaryAndRoomIDs(t *testing.T) {
	withAppServerHelperCommand(t, "room-configuration")
	t.Setenv("CODEX_HOME", t.TempDir())
	home := t.TempDir()
	legacy := filepath.Join(home, "task-sessions", "room", hostStateDirName)
	current := filepath.Join(home, hostStateDirName)
	for _, dir := range []string{legacy, current} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeJSONFile(os.WriteFile, filepath.Join(legacy, sessionFileName), sessionMetadata{
		ConversationSessions: map[string]string{"room": "old-room"},
	}); err != nil {
		t.Fatal(err)
	}
	const runtimeID = "rt-agent-alice"
	if err := writeJSONFile(os.WriteFile, filepath.Join(current, runtimeFileName), runtimeMetadata{
		RuntimeID: runtimeID, SessionID: "old-primary", State: agentruntime.StateStopped,
	}); err != nil {
		t.Fatal(err)
	}
	rt := New(Dependencies{
		AgentHome:      func(string) (string, error) { return home, nil },
		BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
		ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
			return AgentRef{ID: "agent-alice", Name: "alice", RuntimeID: h.RuntimeID}, nil
		},
		ResolveRoomProfile: func(string, string, taskconfig.Config) (agentruntime.Profile, error) {
			return agentruntime.Profile{}, nil
		},
	})
	t.Cleanup(func() { _ = rt.Close() })
	if _, err := rt.Start(t.Context(), agentruntime.Handle{RuntimeID: runtimeID}); err != nil {
		t.Fatal(err)
	}
	meta, err := rt.readSessionMetadata(runtimeID)
	if err != nil || meta.SessionID != "old-primary" || meta.ConversationSessions["room"] != "old-room" {
		t.Fatalf("after import: primary=%q room=%q err=%v", meta.SessionID, meta.ConversationSessions["room"], err)
	}
}

func TestActiveRoomCanSendDuringColdRoomRestore(t *testing.T) {
	for _, scenario := range []string{"resume", "reset_during_resume"} {
		t.Run(scenario, func(t *testing.T) {
			withAppServerHelperCommand(t, "resume-during-turn")
			spec := testAppServerSessionSpec(t.TempDir())
			spec.ConversationSessions = map[string]string{"cold": "cold-thread"}
			manager := newAppServerManager(testAppServerManagerDeps())
			if _, err := manager.Start(t.Context(), spec); err != nil {
				t.Fatal(err)
			}
			h := SessionHandle{RuntimeID: spec.RuntimeID}
			t.Cleanup(func() { _ = manager.Stop(context.Background(), h) })
			send := func() error {
				id, err := manager.EnsureEngineSession(t.Context(), h, "active")
				if err != nil {
					return err
				}
				_, err = manager.Prompt(t.Context(), h, PromptRequest{SessionID: id, Prompt: []PromptContentBlock{TextBlock("message")}})
				return err
			}
			if err := send(); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := manager.EnsureSession(ctx, h, "cold")
				done <- err
			}()
			waitForRuntime(t, func() bool {
				log, _ := os.ReadFile(spec.StderrPath)
				return strings.Contains(string(log), "waiting for active turn before resuming cold thread")
			})
			if scenario == "reset_during_resume" {
				if err := manager.ResetConversationHistory(ctx, h, "cold"); err != nil {
					t.Fatal(err)
				}
			}
			// The helper replies to the pending resume only when it receives the active room's next turn.
			if err := send(); err != nil {
				t.Fatal(err)
			}
			err := <-done
			if scenario == "reset_during_resume" {
				if err == nil || !strings.Contains(err.Error(), "changed during restore") {
					t.Fatalf("restore after reset returned %v", err)
				}
				if _, bound, err := manager.ExistingSession(ctx, h, "cold"); err != nil || bound {
					t.Fatalf("stale binding after reset: bound=%v err=%v", bound, err)
				}
			} else if err != nil {
				t.Fatalf("cold restore did not complete during the active turn: %v", err)
			}
		})
	}
}

func TestConcurrentFirstCodexMessagesShareThread(t *testing.T) {
	for _, tc := range []struct {
		name, room, saved string
	}{
		{"new_default", "", ""}, {"saved_default", "", "old-default"},
		{"new_room", "room", ""}, {"saved_room", "room", "old-room"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withAppServerHelperCommand(t, "room-configuration")
			spec := testAppServerSessionSpec(t.TempDir())
			if tc.saved != "" {
				meta := sessionMetadata{SessionID: tc.saved}
				if tc.room != "" {
					meta.SessionID = ""
					spec.ConversationSessions = map[string]string{tc.room: tc.saved}
					meta.ConversationSessions = spec.ConversationSessions
				}
				if err := writeJSONFile(os.WriteFile, filepath.Join(spec.RuntimeDir, sessionFileName), meta); err != nil {
					t.Fatal(err)
				}
			}
			m := newAppServerManager(testAppServerManagerDeps())
			h := SessionHandle{RuntimeID: spec.RuntimeID}
			if _, err := m.Start(t.Context(), spec); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = m.Stop(context.Background(), h) })
			readLog := func() string {
				t.Helper()
				data, err := os.ReadFile(spec.StderrPath)
				if err != nil {
					t.Fatal(err)
				}
				return string(data)
			}
			if strings.Contains(readLog(), "method=thread/") {
				t.Fatal("startup sent a thread RPC")
			}

			const callers = 8
			var ready sync.WaitGroup
			ready.Add(callers)
			start := make(chan struct{})
			results := make(chan error, callers)
			for range callers {
				go func() {
					ready.Done()
					<-start
					var id string
					var err error
					if tc.room != "" {
						id, err = m.EnsureSession(t.Context(), h, tc.room)
					}
					if err == nil {
						_, err = m.Prompt(t.Context(), h, PromptRequest{SessionID: id, Prompt: []PromptContentBlock{TextBlock("hello")}})
					}
					results <- err
				}()
			}
			ready.Wait()
			close(start)
			var completed int
			for range callers {
				err := <-results
				if err == nil {
					completed++
				} else if !strings.Contains(err.Error(), "turn already in progress") {
					t.Fatal(err)
				}
			}
			if completed == 0 {
				t.Fatal("no first message completed")
			}
			log := readLog()
			countRPC := func(method string) int {
				return strings.Count(log, `"Codex session RPC started" method=`+method+" ")
			}
			starts, resumes := countRPC("thread/start"), countRPC("thread/resume")
			if tc.saved == "" && (starts != 1 || resumes != 0) || tc.saved != "" && (starts != 0 || resumes != 1) {
				t.Fatalf("thread/start = %d, thread/resume = %d for saved %q", starts, resumes, tc.saved)
			}
		})
	}
}

func TestRestartBetweenBindingAndFirstSend(t *testing.T) {
	for _, key := range []string{"", "room"} {
		t.Run("key="+key, func(t *testing.T) {
			withAppServerHelperCommand(t, "missing-rollout")
			spec := testAppServerSessionSpec(t.TempDir())
			path := filepath.Join(spec.RuntimeDir, sessionFileName)
			h := SessionHandle{RuntimeID: spec.RuntimeID}
			start := func() *appServerManager {
				t.Helper()
				deps := testAppServerManagerDeps()
				deps.SaveSessionMetadata = func(_ string, meta sessionMetadata) error { return writeJSONFile(os.WriteFile, path, meta) }
				meta, err := readSavedThreads(os.ReadFile, spec.RuntimeDir)
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
				spec.ConversationSessions = meta.ConversationSessions
				m := newAppServerManager(deps)
				if _, err := m.Start(t.Context(), spec); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = m.Stop(context.Background(), h) })
				return m
			}
			first := start()
			unused, err := first.EnsureSession(t.Context(), h, key)
			if err != nil || unused == "" {
				t.Fatalf("first binding: %q %v", unused, err)
			}
			if err := first.Stop(t.Context(), h); err != nil {
				t.Fatal(err)
			}
			second := start()
			replacement, err := second.EnsureSession(t.Context(), h, key)
			if err != nil || replacement == "" || replacement == unused {
				t.Fatalf("replacement: %q %v", replacement, err)
			}
			if _, err := second.Prompt(t.Context(), h, PromptRequest{SessionID: replacement, ConversationKey: key, ClientUserMessageID: "message-1", Prompt: []PromptContentBlock{TextBlock("hello")}}); err != nil {
				t.Fatal(err)
			}
			if err := second.Stop(t.Context(), h); err != nil {
				t.Fatal(err)
			}
			third := start()
			if _, err := third.EnsureSession(t.Context(), h, key); err == nil {
				t.Fatal("replaced a thread with an attempted message")
			}
			saved, err := readSavedThreads(os.ReadFile, spec.RuntimeDir)
			if err != nil {
				t.Fatal(err)
			}
			id := saved.SessionID
			if key != "" {
				id = saved.ConversationSessions[key]
			}
			if id != replacement {
				t.Fatalf("lost original message binding: %q", id)
			}
		})
	}
}

func TestCodexLostStartAcknowledgement(t *testing.T) {
	for _, mode := range []string{"lost-ack", "lost-ack-active", "lost-ack-missing", "lost-ack-conflict"} {
		t.Run(mode, func(t *testing.T) {
			withAppServerHelperCommand(t, mode)
			spec := testAppServerSessionSpec(t.TempDir())
			h := SessionHandle{RuntimeID: spec.RuntimeID}
			start := func() *appServerManager {
				deps := testAppServerManagerDeps()
				deps.SaveSessionMetadata = func(dir string, meta sessionMetadata) error {
					return writeJSONFile(os.WriteFile, filepath.Join(dir, sessionFileName), meta)
				}
				m := newAppServerManager(deps)
				if _, err := m.Start(t.Context(), spec); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = m.Stop(context.Background(), h) })
				return m
			}
			req := PromptRequest{ClientUserMessageID: "shared-id", Prompt: []PromptContentBlock{TextBlock("hello")}}
			first := start()
			if _, err := first.Prompt(t.Context(), h, req); !errors.Is(err, turnjournal.ErrUnconfirmed) {
				t.Fatalf("lost ACK returned %v", err)
			}
			second := start()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			replay, err := second.Prompt(ctx, h, req)
			switch mode {
			case "lost-ack":
				if err != nil || len(replay.history) != 1 || replay.history[0].Text != "original reply" {
					t.Fatalf("replay=%+v err=%v", replay, err)
				}
			case "lost-ack-active":
				if err != nil || replay.StopReason != StopReasonEndTurn {
					t.Fatalf("resume active turn=%+v err=%v", replay, err)
				}
			default:
				if !errors.Is(err, turnjournal.ErrUnconfirmed) {
					t.Fatalf("ambiguous history returned %v", err)
				}
			}
			data, err := os.ReadFile(filepath.Join(spec.WorkspaceDir, "accepted-messages"))
			if err != nil || string(data) != "shared-id\n" {
				t.Fatalf("peer received %q, err=%v", data, err)
			}
		})
	}
}

func TestNativeHistoryOverridesLegacyUnusedMarker(t *testing.T) {
	withAppServerHelperCommand(t, "missing-rollout")
	spec := testAppServerSessionSpec(t.TempDir())
	const id = "legacy-with-history"
	spec.ConversationSessions = map[string]string{"room": id}
	if err := os.WriteFile(filepath.Join(spec.RuntimeDir, sessionFileName), []byte(`{"conversation_sessions":{"room":"legacy-with-history"},"empty_threads":{"legacy-with-history":true}}`), 0600); err != nil {
		t.Fatal(err)
	}
	m := newAppServerManager(testAppServerManagerDeps())
	h := SessionHandle{RuntimeID: spec.RuntimeID}
	if _, err := m.Start(t.Context(), spec); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(context.Background(), h) })
	if _, err := m.EnsureSession(t.Context(), h, "room"); err == nil {
		t.Fatal("replaced native history after a failed resume")
	}
	if saved := m.liveSession(h.RuntimeID).conversationSessions["room"]; saved != id {
		t.Fatalf("saved = %q", saved)
	}
	if unused, err := turnjournal.Unused(spec.RuntimeDir, id); unused || err != nil {
		t.Fatalf("observed history still eligible: %v %v", unused, err)
	}
}
