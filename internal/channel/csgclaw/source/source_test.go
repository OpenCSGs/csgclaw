package source

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/channelbridge"
	"csgclaw/internal/im"
	"csgclaw/internal/participant"
)

type fakeParticipants struct {
	items []apitypes.Participant
}

func (f fakeParticipants) List(participant.ListOptions) []apitypes.Participant {
	return append([]apitypes.Participant(nil), f.items...)
}

type fakeAgents struct {
	items map[string]agentengine.Agent
}

func (f fakeAgents) Get(_ context.Context, id string) (agentengine.Agent, error) {
	item, ok := f.items[id]
	if !ok {
		return agentengine.Agent{}, &agentengine.TurnError{Code: agentengine.ErrorAgentUnavailable, Message: "agent not found"}
	}
	return item, nil
}

type fakeWorkers struct {
	ensured   chan channel.Binding
	submits   chan submittedEvent
	stops     chan channel.BindingID
	submitErr error
	submit    func(channel.Binding, channelbridge.BotEvent) error
}

type submittedEvent struct {
	binding channel.Binding
	event   channelbridge.BotEvent
}

func (f *fakeWorkers) Ensure(value channel.Binding) error {
	select {
	case f.ensured <- value:
	default:
	}
	return nil
}

func (f *fakeWorkers) Stop(id channel.BindingID) {
	if f.stops == nil {
		return
	}
	select {
	case f.stops <- id:
	default:
	}
}

func (f *fakeWorkers) Submit(value channel.Binding, event channelbridge.BotEvent) error {
	f.submits <- submittedEvent{binding: value, event: event}
	if f.submit != nil {
		return f.submit(value, event)
	}
	return f.submitErr
}

func (*fakeWorkers) Close() {}

type mutableParticipants struct {
	mu    sync.Mutex
	items []apitypes.Participant
}

func (p *mutableParticipants) List(participant.ListOptions) []apitypes.Participant {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]apitypes.Participant(nil), p.items...)
}

type mutableAgents struct {
	mu    sync.Mutex
	items map[string]agentengine.Agent
}

func (a *mutableAgents) Get(_ context.Context, id string) (agentengine.Agent, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	item, ok := a.items[id]
	if !ok {
		return agentengine.Agent{}, &agentengine.TurnError{Code: agentengine.ErrorAgentUnavailable, Message: "agent not found"}
	}
	return item, nil
}

func (a *mutableAgents) set(item agentengine.Agent) {
	a.mu.Lock()
	a.items[item.ID] = item
	a.mu.Unlock()
}

func TestSourceForwardsInProcessParticipantEventsToBindingWorker(t *testing.T) {
	bridge := im.NewParticipantBridge("")
	workers := &fakeWorkers{
		ensured: make(chan channel.Binding, 2),
		submits: make(chan submittedEvent, 1),
	}
	source, err := newSource(
		bridge,
		im.NewBus(),
		fakeParticipants{items: []apitypes.Participant{{
			ID: "pt-worker", Channel: participant.ChannelCSGClaw, Type: participant.TypeAgent,
			ChannelUserRef: "user-worker", AgentID: "agent-worker",
		}}},
		fakeAgents{items: map[string]agentengine.Agent{
			"agent-worker": {ID: "agent-worker", Spec: agentengine.AgentSpec{Runtime: agentengine.RuntimeSpec{Adapter: "codex"}}},
		}},
		workers,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := source.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer source.Close()

	select {
	case binding := <-workers.ensured:
		if binding.ParticipantID != "pt-worker" || binding.AgentID != "agent-worker" {
			t.Fatalf("binding = %+v", binding)
		}
	case <-time.After(time.Second):
		t.Fatal("binding was not ensured")
	}

	room := im.Room{ID: "room-1", IsDirect: true, Members: []string{"user-admin", "user-worker"}}
	sender := im.User{ID: "user-admin", Name: "admin"}
	message := im.Message{
		ID:       "message-1",
		SenderID: sender.ID,
		Content:  "hello",
		RelatesTo: &im.MessageRelation{
			RelType: im.RelationTypeThread,
			EventID: "root-1",
		},
	}
	if ok := bridge.EnqueueMessageEvent(room, sender, message, "pt-worker"); !ok {
		t.Fatal("participant event was not delivered to the in-process source")
	}

	select {
	case got := <-workers.submits:
		if got.binding.AgentID != "agent-worker" || got.event.MessageID != "message-1" ||
			got.event.ParticipantID != "pt-worker" || got.event.ThreadRootID != "root-1" {
			t.Fatalf("submitted = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("participant event was not submitted")
	}

	if ok := bridge.EnqueueMessageEvent(room, sender, message, "pt-worker"); !ok {
		t.Fatal("acked duplicate should be recognized as already handled")
	}
	select {
	case got := <-workers.submits:
		t.Fatalf("duplicate event was submitted after Ack: %+v", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSourceReconcilesAgentEligibilityWithoutParticipantEvent(t *testing.T) {
	bridge := im.NewParticipantBridge("")
	participants := &mutableParticipants{items: []apitypes.Participant{{
		ID: "pt-worker", Channel: participant.ChannelCSGClaw, Type: participant.TypeAgent, AgentID: "agent-worker",
	}}}
	agents := &mutableAgents{items: map[string]agentengine.Agent{
		"agent-worker": {
			ID:   "agent-worker",
			Spec: agentengine.AgentSpec{Runtime: agentengine.RuntimeSpec{Adapter: "codex", Sandboxed: true}},
		},
	}}
	workers := &fakeWorkers{
		ensured: make(chan channel.Binding, 2),
		submits: make(chan submittedEvent, 1),
		stops:   make(chan channel.BindingID, 2),
	}
	source, err := newSource(bridge, nil, participants, agents, workers)
	if err != nil {
		t.Fatalf("newSource() error = %v", err)
	}
	if err := source.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer source.Close()

	select {
	case binding := <-workers.ensured:
		t.Fatalf("sandboxed agent unexpectedly created binding %+v", binding)
	case <-time.After(50 * time.Millisecond):
	}
	if got := bridge.SubscriberCount("pt-worker"); got != 0 {
		t.Fatalf("subscriber count = %d, want 0", got)
	}

	agents.set(agentengine.Agent{
		ID:   "agent-worker",
		Spec: agentengine.AgentSpec{Runtime: agentengine.RuntimeSpec{Adapter: "codex"}},
	})
	if err := source.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() eligible error = %v", err)
	}
	select {
	case binding := <-workers.ensured:
		if binding.AgentID != "agent-worker" || binding.ParticipantID != "pt-worker" {
			t.Fatalf("binding = %+v", binding)
		}
	case <-time.After(time.Second):
		t.Fatal("eligible Agent was not reconciled")
	}
	if got := bridge.SubscriberCount("pt-worker"); got != 1 {
		t.Fatalf("subscriber count = %d, want 1", got)
	}

	agents.set(agentengine.Agent{
		ID: "agent-worker",
		Spec: agentengine.AgentSpec{Runtime: agentengine.RuntimeSpec{
			Adapter: "codex", Sandboxed: true,
		}},
	})
	if err := source.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() ineligible error = %v", err)
	}
	select {
	case id := <-workers.stops:
		if id != "pt-worker" {
			t.Fatalf("stopped binding = %q, want pt-worker", id)
		}
	case <-time.After(time.Second):
		t.Fatal("ineligible Agent binding was not stopped")
	}
	if got := bridge.SubscriberCount("pt-worker"); got != 0 {
		t.Fatalf("subscriber count = %d, want 0", got)
	}
}

func TestSourceRetriesEventWhileSubscriptionRemainsActive(t *testing.T) {
	bridge := im.NewParticipantBridge("")
	var attempts atomic.Int32
	workers := &fakeWorkers{
		ensured: make(chan channel.Binding, 1),
		submits: make(chan submittedEvent, 2),
		submit: func(channel.Binding, channelbridge.BotEvent) error {
			if attempts.Add(1) == 1 {
				return errors.New("worker stopped")
			}
			return nil
		},
	}
	source, err := newSource(
		bridge,
		nil,
		fakeParticipants{items: []apitypes.Participant{{
			ID: "pt-worker", Channel: participant.ChannelCSGClaw, Type: participant.TypeAgent, AgentID: "agent-worker",
		}}},
		fakeAgents{items: map[string]agentengine.Agent{
			"agent-worker": {ID: "agent-worker", Spec: agentengine.AgentSpec{Runtime: agentengine.RuntimeSpec{Adapter: "codex"}}},
		}},
		workers,
	)
	if err != nil {
		t.Fatalf("newSource() error = %v", err)
	}
	source.retryInitial = time.Millisecond
	source.retryMax = 2 * time.Millisecond
	if err := source.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer source.Close()

	room := im.Room{ID: "room-1", IsDirect: true, Members: []string{"user-admin", "pt-worker"}}
	sender := im.User{ID: "user-admin", Name: "admin"}
	message := im.Message{ID: "message-retry", SenderID: sender.ID, Content: "retry me"}
	if ok := bridge.EnqueueMessageEvent(room, sender, message, "pt-worker"); !ok {
		t.Fatal("participant event was not delivered")
	}
	select {
	case <-workers.submits:
	case <-time.After(time.Second):
		t.Fatal("participant event was not submitted")
	}
	select {
	case got := <-workers.submits:
		if got.event.MessageID != message.ID {
			t.Fatalf("retried message = %q, want %q", got.event.MessageID, message.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("participant event was not retried on the active subscription")
	}

	deadline := time.Now().Add(time.Second)
	for attempts.Load() != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if attempts.Load() != 2 {
		t.Fatalf("submit attempts = %d, want 2", attempts.Load())
	}
	if ok := bridge.EnqueueMessageEvent(room, sender, message, "pt-worker"); !ok {
		t.Fatal("acked retried event should be recognized as handled")
	}
}

func TestSourceDrainsBridgePendingAfterRetryBackpressure(t *testing.T) {
	bridge := im.NewParticipantBridge("")
	var available atomic.Bool
	firstAttempt := make(chan struct{})
	var firstAttemptOnce sync.Once
	succeeded := make(chan string, 32)
	workers := &fakeWorkers{
		ensured: make(chan channel.Binding, 1),
		submits: make(chan submittedEvent, 128),
		submit: func(_ channel.Binding, event channelbridge.BotEvent) error {
			if !available.Load() {
				firstAttemptOnce.Do(func() { close(firstAttempt) })
				return errors.New("worker unavailable")
			}
			succeeded <- event.MessageID
			return nil
		},
	}
	source, err := newSource(
		bridge,
		nil,
		fakeParticipants{items: []apitypes.Participant{{
			ID: "pt-worker", Channel: participant.ChannelCSGClaw, Type: participant.TypeAgent, AgentID: "agent-worker",
		}}},
		fakeAgents{items: map[string]agentengine.Agent{
			"agent-worker": {ID: "agent-worker", Spec: agentengine.AgentSpec{Runtime: agentengine.RuntimeSpec{Adapter: "codex"}}},
		}},
		workers,
	)
	if err != nil {
		t.Fatalf("newSource() error = %v", err)
	}
	source.retryInitial = time.Millisecond
	source.retryMax = 2 * time.Millisecond
	if err := source.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer source.Close()

	room := im.Room{ID: "room-1", IsDirect: true, Members: []string{"user-admin", "pt-worker"}}
	sender := im.User{ID: "user-admin", Name: "admin"}
	enqueue := func(index int) {
		message := im.Message{
			ID:       fmt.Sprintf("message-%02d", index),
			SenderID: sender.ID,
			Content:  fmt.Sprintf("message %d", index),
		}
		bridge.EnqueueMessageEvent(room, sender, message, "pt-worker")
	}

	enqueue(0)
	select {
	case <-firstAttempt:
	case <-time.After(time.Second):
		t.Fatal("source did not reach worker backpressure")
	}
	const messageCount = 24 // exceeds the ParticipantBridge subscription buffer
	for index := 1; index < messageCount; index++ {
		enqueue(index)
	}
	available.Store(true)

	seen := make(map[string]struct{}, messageCount)
	deadline := time.After(2 * time.Second)
	for len(seen) < messageCount {
		select {
		case messageID := <-succeeded:
			seen[messageID] = struct{}{}
		case <-deadline:
			t.Fatalf("successfully submitted messages = %d, want %d; pending events remained stranded", len(seen), messageCount)
		}
	}
}
