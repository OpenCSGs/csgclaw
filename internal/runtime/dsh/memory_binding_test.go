package dsh

import (
	"context"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	agentruntime "csgclaw/internal/runtime"
)

func TestDSHUnboundSessionKeepsContextAndIsolatesPriorMemory(t *testing.T) {
	for _, priorMemory := range []bool{false, true} {
		t.Run(map[bool]string{false: "unbound", true: "memory-bound"}[priorMemory], func(t *testing.T) {
			client, frames := reasoningACPClient(t, modelOptions(), modelOptions())
			key := "private-room"
			if priorMemory {
				key = "csgclaw-memory:old-policy:private-room"
			}
			proc := &process{client: client, root: t.TempDir(), workspace: t.TempDir(), profile: agentruntime.Profile{ModelID: "test-model"}, meta: runtimeMetadata{Sessions: map[string]string{key: "old-session"}}, ready: map[string]bool{"old-session": true}}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c := &conversation{}
			session, err := c.ensureSession(ctx, proc, contract.TurnRequest{ConversationKey: "private-room"})
			if err != nil {
				t.Fatal(err)
			}
			if priorMemory {
				if proc.meta.Sessions[key] != "" || session == "old-session" {
					t.Fatal("memory-free turn retained prior memory")
				}
				if frame := <-frames; frame.Method != "session/close" {
					t.Fatalf("prior memory session not closed: %+v", frame)
				}
			} else if session != "old-session" || len(frames) != 0 {
				t.Fatal("unbound conversation was replaced")
			}
		})
	}
}
