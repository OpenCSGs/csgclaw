package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentruntime "csgclaw/internal/runtime"
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
