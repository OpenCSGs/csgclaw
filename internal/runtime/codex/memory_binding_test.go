package codex

import (
	"context"
	"testing"
)

func TestAppServerUnboundSessionKeepsContextAndIsolatesPriorMemory(t *testing.T) {
	for _, priorMemory := range []bool{false, true} {
		t.Run(map[bool]string{false: "unbound", true: "memory-bound"}[priorMemory], func(t *testing.T) {
			withAppServerHelperCommand(t, "resume-success")
			spec := testAppServerSessionSpec(t.TempDir())
			key := "private-room"
			if priorMemory {
				key = "csgclaw-memory:old-policy:private-room"
			}
			spec.ConversationSessions = map[string]string{key: "cold-thread"}
			spec.FilePublishingConversations = map[string]bool{key: true}
			manager := newAppServerManager(testAppServerManagerDeps())
			if _, err := manager.Start(context.Background(), spec); err != nil {
				t.Fatal(err)
			}
			defer manager.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID})
			thread, err := manager.EnsureEngineSession(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID}, "private-room")
			if err != nil {
				t.Fatal(err)
			}
			live := manager.liveSession(spec.RuntimeID)
			live.mu.Lock()
			defer live.mu.Unlock()
			if priorMemory {
				if live.conversationSessions[key] != "" {
					t.Fatal("prior memory binding remains resumable")
				}
				if thread == "cold-thread" || thread == "resumed-thread" {
					t.Fatal("memory-free turn resumed prior memory")
				}
			} else if thread != "resumed-thread" {
				t.Fatalf("unbound conversation was replaced: %s", thread)
			}
		})
	}
}
