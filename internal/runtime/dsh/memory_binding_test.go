package dsh

import (
	"context"
	"csgclaw/internal/runtime/turnjournal"
	"encoding/json"
	"net"
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

func TestDSHMissingStrictContinuationPreservesExistingSession(t *testing.T) {
	for _, oldMemory := range []bool{false, true} {
		t.Run(map[bool]string{false: "unbound", true: "memory-bound"}[oldMemory], func(t *testing.T) {
			client, frames := reasoningACPClient(t, modelOptions(), modelOptions())
			existing := contract.TurnRequest{ConversationKey: "private-room"}
			missing := contract.TurnRequest{ConversationKey: "private-room", Continuation: contract.ContinuationRequireExisting, RuntimeMemory: &contract.RuntimeMemory{Binding: "new-policy"}}
			if oldMemory {
				existing.RuntimeMemory = &contract.RuntimeMemory{Binding: "old-policy"}
				missing.RuntimeMemory = nil
			}
			key := contract.RuntimeSessionKey(existing)
			proc := &process{client: client, root: t.TempDir(), workspace: t.TempDir(), profile: agentruntime.Profile{ModelID: "test-model"}, meta: runtimeMetadata{Sessions: map[string]string{key: "old-session"}}, ready: map[string]bool{"old-session": true}}
			if err := writeMetadata(proc.root, proc.meta); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c := &conversation{}
			if _, err := c.ensureSession(ctx, proc, missing); err == nil || err.Code != contract.ErrorConversationNotResumable {
				t.Fatalf("strict continuation error: %v", err)
			}
			persisted, err := readMetadata(proc.root)
			if err != nil {
				t.Fatal(err)
			}
			if proc.meta.Sessions[key] != "old-session" || persisted.Sessions[key] != "old-session" || !proc.ready["old-session"] || len(frames) != 0 {
				t.Fatalf("failed strict continuation mutated existing session: memory=%v disk=%v requests=%d", proc.meta.Sessions, persisted.Sessions, len(frames))
			}
			// Simulate resuming after process restart; the original native context must remain addressable.
			proc.ready = map[string]bool{}
			existing.Continuation = contract.ContinuationRequireExisting
			session, turnErr := c.ensureSession(ctx, proc, existing)
			if turnErr != nil || session != "old-session" {
				t.Fatalf("original session cannot resume: %q %v", session, turnErr)
			}
			if frame := <-frames; frame.Method != "session/resume" || frame.Params.SessionID != "old-session" {
				t.Fatalf("wrong resume after rejected continuation: %+v", frame)
			}
		})
	}
}

func TestDSHFailedResumeOnlyReplacesUnsentNewSession(t *testing.T) {
	for _, state := range []string{"unused", "attempted", "legacy"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			if state != "legacy" {
				if err := turnjournal.Created(root, "saved-id"); err != nil {
					t.Fatal(err)
				}
			}
			if state == "attempted" {
				if _, err := turnjournal.Begin(root, "room", "saved-id", "previous"); err != nil {
					t.Fatal(err)
				}
			}
			meta := runtimeMetadata{Sessions: map[string]string{"room": "saved-id"}}
			if err := writeMetadata(root, meta); err != nil {
				t.Fatal(err)
			}
			host, peer := net.Pipe()
			t.Cleanup(func() { host.Close(); peer.Close() })
			proc := &process{root: root, workspace: t.TempDir(), client: newACPClient(host, host), meta: meta, ready: map[string]bool{}, profile: agentruntime.Profile{ModelID: "test-model"}}
			done := make(chan int, 1)
			go func() {
				var creates int
				defer func() { done <- creates }()
				decoder, encoder := json.NewDecoder(peer), json.NewEncoder(peer)
				for {
					var frame reasoningACPFrame
					if err := decoder.Decode(&frame); err != nil {
						return
					}
					reply := map[string]any{"jsonrpc": "2.0", "id": frame.ID}
					switch frame.Method {
					case "session/resume":
						reply["error"] = map[string]any{"code": -32602, "message": "unknown session"}
					case "session/new":
						creates++
						reply["result"] = sessionResult{SessionID: "assigned-by-dsh", ConfigOptions: modelOptions()}
					default:
						reply["result"] = sessionResult{}
					}
					if encoder.Encode(reply) != nil {
						return
					}
				}
			}()
			ctx, cancel := context.WithTimeout(t.Context(), time.Second*3)
			defer cancel()
			id, turnErr := (&conversation{}).ensureSession(ctx, proc, contract.TurnRequest{ID: "next", ConversationKey: "room"})
			host.Close()
			creates := <-done
			saved, err := readMetadata(root)
			if err != nil {
				t.Fatal(err)
			}
			if state == "unused" {
				if turnErr != nil || creates != 1 || id != "assigned-by-dsh" || saved.Sessions["room"] != id {
					t.Fatalf("created=%d id=%q stored=%v err=%v", creates, id, saved.Sessions, turnErr)
				}
			} else if turnErr == nil || creates != 0 || saved.Sessions["room"] != "saved-id" {
				t.Fatalf("lost %s binding: creates=%d stored=%v err=%v", state, creates, saved.Sessions, turnErr)
			}
		})
	}
}
