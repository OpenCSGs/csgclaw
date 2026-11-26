package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/taskconfig"
)

func TestCodexRestartKeepsUsedThreadsAndReplacesEmptyThreads(t *testing.T) {
	for _, usePrimary := range []bool{false, true} {
		name := "empty_primary"
		if usePrimary {
			name = "used_primary"
		}
		t.Run(name, func(t *testing.T) {
			withAppServerHelperCommand(t, "room-configuration")
			t.Setenv("CODEX_HOME", t.TempDir())
			root := t.TempDir()
			spec := agentruntime.Spec{AgentID: "agent-alice", AgentName: "alice", RuntimeID: "rt-agent-alice"}
			deps := Dependencies{
				BinaryProvider: fakeBinaryProvider{path: "/tmp/codex"},
				AgentHome:      func(id string) (string, error) { return filepath.Join(root, id), nil },
				ResolveAgent: func(h agentruntime.Handle) (AgentRef, error) {
					return AgentRef{ID: spec.AgentID, Name: spec.AgentName, RuntimeID: h.RuntimeID}, nil
				},
			}
			ctx := t.Context()
			first := New(deps)
			t.Cleanup(func() { _ = first.Close() })
			handle, err := first.New(ctx, spec)
			if err != nil {
				t.Fatal(err)
			}
			room, err := first.EnsureSession(ctx, spec.RuntimeID, "room")
			if err != nil {
				t.Fatal(err)
			}
			used, empty := room, handle.HandleID
			if usePrimary {
				used, empty = empty, used
			}
			if err := first.Prompt(ctx, spec.RuntimeID, used, "message"); err != nil {
				t.Fatal(err)
			}
			if err := first.Close(); err != nil {
				t.Fatal(err)
			}

			restarted := New(deps)
			t.Cleanup(func() { _ = restarted.Close() })
			handle, err = restarted.New(ctx, spec)
			if err != nil {
				t.Fatal(err)
			}
			room, err = restarted.EnsureSession(ctx, spec.RuntimeID, "room")
			if err != nil {
				t.Fatal(err)
			}
			newUsed, newEmpty := room, handle.HandleID
			if usePrimary {
				newUsed, newEmpty = newEmpty, newUsed
			}
			if used != newUsed || empty == newEmpty {
				t.Fatalf("used thread %q -> %q, empty thread %q -> %q", used, newUsed, empty, newEmpty)
			}
			meta, err := restarted.readSessionMetadata(spec.RuntimeID)
			if err != nil || meta.EmptyThreads[used] || !meta.EmptyThreads[newEmpty] || len(meta.EmptyThreads) != 1 {
				t.Fatalf("saved empty threads = %v, %v", meta.EmptyThreads, err)
			}
		})
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
	if !manager.liveSession(spec.RuntimeID).emptyThreads[session.SessionID] {
		t.Fatal("failed metadata write lost the empty marker")
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
