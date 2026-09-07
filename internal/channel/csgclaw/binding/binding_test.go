package binding

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/execution"
)

type fakeEngine struct {
	mu      sync.Mutex
	agentID string
	request agentengine.TurnRequest
	run     func(context.Context, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult
	resets  atomic.Int32
}

func (f *fakeEngine) Conversations(agentID string) agentengine.ConversationInterface {
	f.mu.Lock()
	f.agentID = agentID
	f.mu.Unlock()
	return fakeConversation{engine: f}
}

type fakeConversation struct {
	engine *fakeEngine
}

func (fakeConversation) Files() agentengine.FileInterface {
	return nil
}

func (f fakeConversation) Run(ctx context.Context, request agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
	f.engine.mu.Lock()
	f.engine.request = request
	f.engine.mu.Unlock()
	if f.engine.run != nil {
		return f.engine.run(ctx, request, sink)
	}
	return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "done"}
}

func (f *fakeEngine) requestSnapshot() agentengine.TurnRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.request
}

func (fakeConversation) Cancel(context.Context, agentengine.ConversationKey, agentengine.TurnID) error {
	return nil
}

func (f fakeConversation) Reset(context.Context, agentengine.ConversationKey) error {
	f.engine.resets.Add(1)
	return nil
}

func (fakeConversation) Resolve(context.Context, agentengine.InteractionResolution) error {
	return nil
}

type fakeRenderer struct{}

func (fakeRenderer) Emit(context.Context, channel.TurnContext, agentengine.TurnEvent) error {
	return nil
}

func (fakeRenderer) Complete(context.Context, channel.TurnContext, agentengine.TurnResult) error {
	return nil
}

func TestManagerSubmitDedupesSourceMessage(t *testing.T) {
	var runs atomic.Int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	engine := &fakeEngine{run: func(context.Context, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
		runs.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "ok"}
	}}
	adapter, err := execution.New(engine, fakeRenderer{}, execution.WithTurnIDGenerator(func() (agentengine.TurnID, error) {
		return "turn-1", nil
	}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	manager, err := NewManager(adapter)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer manager.Close()
	defer close(release)

	value := channel.Binding{ParticipantID: "manager", AgentID: "u-manager"}
	ensureManagerBinding(t, manager, value)
	event := channel.Event{MessageID: "message-1", RoomID: "room-1", Text: "hello"}
	if err := manager.Submit(value, event); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if err := manager.Submit(value, event); err != nil {
		t.Fatalf("duplicate Submit() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first turn")
	}
	waitFor(t, func() bool { return runs.Load() == 1 })
}

func TestManagerSubmitResetDoesNotRunEngine(t *testing.T) {
	engine := &fakeEngine{}
	adapter, err := execution.New(engine, fakeRenderer{}, execution.WithTurnIDGenerator(func() (agentengine.TurnID, error) {
		return "turn-reset", nil
	}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	manager, err := NewManager(adapter)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer manager.Close()

	binding := channel.Binding{ParticipantID: "manager", AgentID: "u-manager"}
	ensureManagerBinding(t, manager, binding)
	if err := manager.Submit(binding, channel.Event{
		MessageID: "message-new",
		RoomID:    "room-1",
		Text:      `<slash-command name="new" arg="conversation"></slash-command>`,
	}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	waitFor(t, func() bool { return engine.resets.Load() == 1 })
	if request := engine.requestSnapshot(); request.ID != "" {
		t.Fatalf("engine ran %+v", request)
	}
}

func TestManagerRunsIndependentConversationsConcurrently(t *testing.T) {
	roomOneStarted := make(chan struct{})
	roomTwoStarted := make(chan struct{})
	releaseRoomOne := make(chan struct{})
	engine := &fakeEngine{run: func(_ context.Context, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		if strings.Contains(string(request.ConversationKey), "room-1") {
			close(roomOneStarted)
			<-releaseRoomOne
		} else if strings.Contains(string(request.ConversationKey), "room-2") {
			close(roomTwoStarted)
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "ok"}
	}}
	adapter, err := execution.New(engine, fakeRenderer{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	manager, err := NewManager(adapter)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer manager.Close()
	defer close(releaseRoomOne)

	binding := channel.Binding{ParticipantID: "manager", AgentID: "u-manager"}
	ensureManagerBinding(t, manager, binding)
	if err := manager.Submit(binding, channel.Event{MessageID: "message-1", RoomID: "room-1", Text: "first"}); err != nil {
		t.Fatalf("first Submit() error = %v", err)
	}
	select {
	case <-roomOneStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for room-1 turn")
	}
	if err := manager.Submit(binding, channel.Event{MessageID: "message-2", RoomID: "room-2", Text: "second"}); err != nil {
		t.Fatalf("second Submit() error = %v", err)
	}
	select {
	case <-roomTwoStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("room-2 turn was blocked by room-1")
	}
}

func TestManagerPreservesOrderWithinConversation(t *testing.T) {
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var runs atomic.Int32
	engine := &fakeEngine{run: func(context.Context, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
		switch runs.Add(1) {
		case 1:
			close(firstStarted)
			<-releaseFirst
		case 2:
			close(secondStarted)
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "ok"}
	}}
	adapter, err := execution.New(engine, fakeRenderer{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	manager, err := NewManager(adapter)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer manager.Close()

	binding := channel.Binding{ParticipantID: "manager", AgentID: "u-manager"}
	ensureManagerBinding(t, manager, binding)
	if err := manager.Submit(binding, channel.Event{MessageID: "message-1", RoomID: "room-1", Text: "first"}); err != nil {
		t.Fatalf("first Submit() error = %v", err)
	}
	select {
	case <-firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first turn")
	}
	if err := manager.Submit(binding, channel.Event{MessageID: "message-2", RoomID: "room-1", Text: "second"}); err != nil {
		t.Fatalf("second Submit() error = %v", err)
	}
	select {
	case <-secondStarted:
		t.Fatal("second turn started before the first completed")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	select {
	case <-secondStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second turn")
	}
}

func TestStoppedWorkerRejectsSubmit(t *testing.T) {
	adapter, err := execution.New(&fakeEngine{}, fakeRenderer{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	manager, err := NewManager(adapter)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	binding := channel.Binding{ParticipantID: "manager", AgentID: "u-manager"}
	if err := manager.Ensure(binding); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	manager.mu.Lock()
	worker := manager.workers[binding.StableID()]
	manager.mu.Unlock()
	manager.Stop(binding.StableID())
	if err := worker.Submit(channel.Event{MessageID: "message-late", RoomID: "room-1", Text: "late"}); err == nil {
		t.Fatal("stopped worker accepted a late event")
	}
	if err := manager.Submit(binding, channel.Event{MessageID: "message-revive", RoomID: "room-1", Text: "revive"}); err == nil {
		t.Fatal("late submit recreated a stopped binding worker")
	}
	manager.mu.Lock()
	recreated := manager.workers[binding.StableID()]
	manager.mu.Unlock()
	if recreated != nil {
		t.Fatal("stopped binding worker was recreated")
	}
}

func TestManagerSubmitResetCancelsActiveTurnBeforeReset(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	engine := &fakeEngine{run: func(ctx context.Context, _ agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		close(started)
		<-ctx.Done()
		close(canceled)
		return agentengine.TurnResult{Status: agentengine.TurnCanceled}
	}}
	adapter, err := execution.New(engine, fakeRenderer{}, execution.WithTurnIDGenerator(func() (agentengine.TurnID, error) {
		return "turn-reset", nil
	}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	manager, err := NewManager(adapter)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer manager.Close()

	binding := channel.Binding{ParticipantID: "manager", AgentID: "u-manager"}
	ensureManagerBinding(t, manager, binding)
	if err := manager.Submit(binding, channel.Event{
		MessageID: "message-run", RoomID: "room-1", Text: "keep working",
	}); err != nil {
		t.Fatalf("run Submit() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for active turn")
	}

	if err := manager.Submit(binding, channel.Event{
		MessageID: "message-new",
		RoomID:    "room-1",
		Text:      `<slash-command name="new" arg="conversation"></slash-command>`,
	}); err != nil {
		t.Fatalf("reset Submit() error = %v", err)
	}
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("reset did not cancel the active turn")
	}
	waitFor(t, func() bool { return engine.resets.Load() == 1 })
}

func TestManagerSubmitResetDiscardsOlderQueuedMessages(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	nextInput := make(chan string, 1)
	var runs atomic.Int32
	engine := &fakeEngine{run: func(ctx context.Context, request agentengine.TurnRequest, _ agentengine.EventSink) agentengine.TurnResult {
		if runs.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			close(canceled)
			return agentengine.TurnResult{Status: agentengine.TurnCanceled}
		}
		if len(request.Input) > 0 {
			nextInput <- request.Input[len(request.Input)-1].Text
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "ok"}
	}}
	adapter, err := execution.New(engine, fakeRenderer{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	manager, err := NewManager(adapter)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer manager.Close()

	binding := channel.Binding{ParticipantID: "manager", AgentID: "u-manager"}
	ensureManagerBinding(t, manager, binding)
	if err := manager.Submit(binding, channel.Event{MessageID: "message-active", RoomID: "room-1", Text: "active"}); err != nil {
		t.Fatalf("active Submit() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for active turn")
	}
	if err := manager.Submit(binding, channel.Event{MessageID: "message-stale", RoomID: "room-1", Text: "stale"}); err != nil {
		t.Fatalf("stale Submit() error = %v", err)
	}
	if err := manager.Submit(binding, channel.Event{
		MessageID: "message-new",
		RoomID:    "room-1",
		Text:      `<slash-command name="new" arg="conversation"></slash-command>`,
	}); err != nil {
		t.Fatalf("reset Submit() error = %v", err)
	}
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("reset did not cancel the active turn")
	}
	waitFor(t, func() bool { return engine.resets.Load() == 1 })

	if err := manager.Submit(binding, channel.Event{MessageID: "message-fresh", RoomID: "room-1", Text: "fresh"}); err != nil {
		t.Fatalf("fresh Submit() error = %v", err)
	}
	select {
	case input := <-nextInput:
		if !strings.Contains(input, "fresh") {
			t.Fatalf("next input = %q, want fresh message; stale queued message crossed reset", input)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for post-reset turn")
	}
	if got := runs.Load(); got != 2 {
		t.Fatalf("engine runs = %d, want active and fresh only", got)
	}
}

func ensureManagerBinding(t *testing.T, manager *Manager, binding channel.Binding) {
	t.Helper()
	if err := manager.Ensure(binding); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
}

func waitFor(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out")
}

func (fakeConversation) GetInteraction(context.Context, agentengine.ConversationKey, string) (agentengine.InteractionRequest, error) {
	return agentengine.InteractionRequest{}, &agentengine.TurnError{Code: agentengine.ErrorInteractionNotFound, Message: "no interaction in this test fixture"}
}
