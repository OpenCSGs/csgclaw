package binding

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/execution"
)

type synchronizedLogBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}
func (b *synchronizedLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func TestTaskRuntimeFailureLogsAssignmentAndRedactsCredentials(t *testing.T) {
	previous := slog.Default()
	var logs synchronizedLogBuffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	engine := &fakeEngine{run: func(context.Context, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
		return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: "model unavailable Authorization: Bearer fixture-log-secret"}}
	}}
	adapter, err := execution.New(engine, fakeRenderer{})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(adapter)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	value := channel.Binding{ParticipantID: "pt-worker", AgentID: "agent-worker"}
	ensureManagerBinding(t, manager, value)
	if err := manager.Submit(value, channel.Event{MessageID: "dispatch-log", RoomID: "room-log", TaskID: "task-log", TaskAttempt: 4, Text: "Build"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(logs.String(), "room task Agent runtime failed") {
		if time.Now().After(deadline) {
			t.Fatalf("runtime failure not logged: %s", logs.String())
		}
		time.Sleep(time.Millisecond)
	}
	got := logs.String()
	for _, field := range []string{`"task_id":"task-log"`, `"attempt":4`, `"message_id":"dispatch-log"`, `"room_id":"room-log"`, `"participant_id":"pt-worker"`, `"agent_id":"agent-worker"`, `"phase":"agent_turn"`, `"room_role":"worker"`, `"error_code":"runtime_failed"`, `"turn_status":"failed"`, `"duration":`} {
		if !strings.Contains(got, field) {
			t.Fatalf("missing field %s: %s", field, got)
		}
	}
	if strings.Contains(got, "fixture-log-secret") {
		t.Fatal("runtime credential leaked in task log")
	}
}
