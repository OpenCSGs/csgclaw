package codex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func platformFileFixture(t *testing.T) (*Runtime, *liveSession, context.CancelFunc) {
	t.Helper()
	manager := newAppServerManager(managerDeps{})
	spec := testAppServerSessionSpec(t.TempDir())
	spec.AgentID = "agent-alice"
	live := &liveSession{spec: spec, filePublishingThreads: map[string]bool{"thread-a": true}, conversationSessions: map[string]string{"room-a": "thread-a"}}
	manager.sessions[spec.RuntimeID] = live
	waiter, err := live.registerAppServerTurnWaiter("thread-a")
	if err != nil {
		t.Fatal(err)
	}
	waiter.setTurnID("turn-a")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	live.setAppServerTurnContext("thread-a", waiter, ctx)
	return New(Dependencies{Manager: manager}), live, cancel
}

func TestPlatformFileStreamsOnlyValidatedSize(t *testing.T) {
	rt, live, _ := platformFileFixture(t)
	path := filepath.Join(live.spec.WorkspaceDir, "report.txt")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := rt.WithPlatformFile(context.Background(), "agent-alice", "thread-a", "turn-a", "report.txt", func(ctx context.Context, name string, size int64, body io.Reader) (json.RawMessage, error) {
		if name != "report.txt" || size != 8 {
			t.Fatalf("file metadata name=%q size=%d", name, size)
		}
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.WriteString(" appended after validation"); err != nil {
			t.Fatal(err)
		}
		if err = file.Close(); err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(body)
		if err != nil {
			return nil, err
		}
		if string(raw) != "original" {
			t.Fatalf("read beyond validated size: %q", raw)
		}
		return json.RawMessage(`{"uploaded":true}`), nil
	})
	if err != nil || string(result) != `{"uploaded":true}` {
		t.Fatalf("result=%s error=%v", result, err)
	}
}

func TestPlatformFileRejectsInvalidScopeAndUnsafeFiles(t *testing.T) {
	rt, live, _ := platformFileFixture(t)
	if err := os.WriteFile(filepath.Join(live.spec.WorkspaceDir, "valid.txt"), []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(live.spec.WorkspaceDir, "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	large, err := os.Create(filepath.Join(live.spec.WorkspaceDir, "large.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if err = large.Truncate(appServerMaxUploadBytes + 1); err != nil {
		t.Fatal(err)
	}
	_ = large.Close()
	cases := []struct{ name, agent, thread, turn, path string }{
		{"foreign Agent", "agent-bob", "thread-a", "turn-a", "valid.txt"},
		{"foreign thread", "agent-alice", "thread-b", "turn-a", "valid.txt"},
		{"foreign turn", "agent-alice", "thread-a", "turn-b", "valid.txt"},
		{"absolute file", "agent-alice", "thread-a", "turn-a", outside},
		{"parent escape", "agent-alice", "thread-a", "turn-a", "../outside.txt"},
		{"directory", "agent-alice", "thread-a", "turn-a", "directory"},
		{"missing", "agent-alice", "thread-a", "turn-a", "missing"},
		{"oversized", "agent-alice", "thread-a", "turn-a", "large.dat"},
	}
	if err := os.Symlink(outside, filepath.Join(live.spec.WorkspaceDir, "escape")); err == nil {
		cases = append(cases, struct{ name, agent, thread, turn, path string }{"symlink escape", "agent-alice", "thread-a", "turn-a", "escape"})
	}
	consume := func(context.Context, string, int64, io.Reader) (json.RawMessage, error) {
		t.Error("invalid request reached file consumer")
		return nil, nil
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := rt.WithPlatformFile(context.Background(), test.agent, test.thread, test.turn, test.path, consume); err == nil {
				t.Fatal("invalid file request accepted")
			}
		})
	}
	live.spec.ExecutionMode = ExecutionModeReadOnly
	if _, err := rt.WithPlatformFile(context.Background(), "agent-alice", "thread-a", "turn-a", "valid.txt", consume); err == nil {
		t.Fatal("read-only file upload accepted")
	}
}

func TestPlatformFileReaderEndsWithRequestAndTurn(t *testing.T) {
	for _, cancelRequest := range []bool{true, false} {
		name := "turn"
		if cancelRequest {
			name = "request"
		}
		t.Run(name, func(t *testing.T) {
			rt, live, cancelTurn := platformFileFixture(t)
			if err := os.WriteFile(filepath.Join(live.spec.WorkspaceDir, "file.txt"), []byte("file"), 0600); err != nil {
				t.Fatal(err)
			}
			requestCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := rt.WithPlatformFile(requestCtx, "agent-alice", "thread-a", "turn-a", "file.txt", func(ctx context.Context, _ string, _ int64, body io.Reader) (json.RawMessage, error) {
				if cancelRequest {
					cancel()
				} else {
					cancelTurn()
				}
				<-ctx.Done()
				_, err := io.ReadAll(body)
				return nil, err
			})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("upload cancellation error=%v", err)
			}
		})
	}
}
