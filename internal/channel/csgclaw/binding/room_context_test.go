package binding

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/execution"
)

func TestQueuedRoomTurnReadsContextAfterPreviousTurnFinishes(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	second := make(chan string, 1)
	var reads atomic.Int32
	var completed atomic.Bool
	engine := &fakeEngine{run: func(ctx context.Context, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		if request.Input[0].Text == "before acceptance" {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return agentengine.TurnResult{Status: agentengine.TurnCanceled}
			}
			completed.Store(true)
		} else {
			second <- request.Input[0].Text
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded}
	}}
	adapter, err := execution.New(engine, fakeRenderer{}, execution.WithRoomContextProvider(func(room, actor, source, task string) (string, error) {
		reads.Add(1)
		if room != "r" || actor != "manager" {
			t.Error(room, actor)
		}
		if completed.Load() {
			return "accepted; next child eligible", nil
		}
		return "before acceptance", nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(adapter)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	binding := channel.Binding{ParticipantID: "manager", AgentID: "agent-manager"}
	ensureManagerBinding(t, manager, binding)
	if err := manager.Submit(binding, channel.Event{MessageID: "first", RoomID: "r", RoomManager: true, Text: "Review result"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first turn did not start")
	}
	if err := manager.Submit(binding, channel.Event{MessageID: "second", RoomID: "r", RoomManager: true, Text: "Next step"}); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 1 {
		t.Fatal("queued input eagerly read stale facts")
	}
	close(release)
	select {
	case got := <-second:
		if got != "accepted; next child eligible" {
			t.Fatal(got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued turn did not run")
	}
}
