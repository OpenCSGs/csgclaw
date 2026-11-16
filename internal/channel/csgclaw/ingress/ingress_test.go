package ingress

import (
	"context"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/execution"
	"csgclaw/internal/channel/csgclaw/state"
	"fmt"
	"testing"
)

func testBufferedWorker() *Worker {
	return &Worker{adapter: &execution.Adapter{}, binding: channel.Binding{ID: "b", AgentID: "a", ParticipantID: "pt-a"}, queue: make(chan queuedEvent, defaultQueueSize), queued: map[string]struct{}{}, notifications: map[agentengine.ConversationKey]string{}, latest: map[string]string{}, seen: state.NewSeenWindow(defaultSeenWindow), processing: map[string]struct{}{}, generation: map[agentengine.ConversationKey]uint64{}}
}
func TestNotificationsCoalescePerScopeKeepingEarliestSource(t *testing.T) {
	w := testBufferedWorker()
	w.active = make(map[agentengine.ConversationKey]map[string]activeEvent)
	first, ok, err := w.acceptAndEnqueue(channel.Event{RoomID: "r", MessageID: "first", NotificationOnly: true})
	if err != nil || !ok {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		_, ok, err = w.acceptAndEnqueue(channel.Event{RoomID: "r", MessageID: fmt.Sprint(i), NotificationOnly: true})
		if err != nil || ok {
			t.Fatalf("burst not merged: %v %v", ok, err)
		}
	}
	if len(w.queue) != 1 || (<-w.queue).event.MessageID != "first" || !w.IsCurrent(first.conversationKey, "first") {
		t.Fatal("lost the retained notification identity")
	}
	w.beginProcessing(first.dedupeKey)
	_, cancel := context.WithCancel(t.Context())
	defer cancel()
	updates := make(chan struct{}, 1)
	if !w.setActive(first, activeEvent{cancel: cancel, unread: updates}) {
		t.Fatal("could not register active turn")
	}
	_, ok, err = w.acceptAndEnqueue(channel.Event{RoomID: "r", MessageID: "while-working", NotificationOnly: true})
	if err != nil || !ok {
		t.Fatal("new notification during work lost")
	}
	select {
	case <-updates:
	default:
		t.Fatal("active turn was not notified")
	}
	for _, e := range []channel.Event{{RoomID: "r", ThreadRootID: "thread", MessageID: "thread-reply", NotificationOnly: true}, {RoomID: "other", MessageID: "other", NotificationOnly: true}, {RoomID: "r", MessageID: "direct-one", Text: "one"}, {RoomID: "r", MessageID: "direct-two", Text: "two"}} {
		if _, ok, err := w.acceptAndEnqueue(e); err != nil || !ok {
			t.Fatalf("independent event swallowed: %+v %v", e, err)
		}
	}
	if len(w.queue) != 5 {
		t.Fatalf("queue=%d", len(w.queue))
	}
	select {
	case <-updates:
		t.Fatal("another scope or non-notification event signaled the active turn")
	default:
	}
	for _, id := range []string{"more-one", "more-two"} {
		_, accepted, err := w.acceptAndEnqueue(channel.Event{RoomID: "r", MessageID: id, NotificationOnly: true})
		if accepted || err != nil {
			t.Fatalf("coalesced update: accepted=%v, err=%v", accepted, err)
		}
	}
	if len(updates) != 1 || len(w.queue) != 5 {
		t.Fatal("coalescing lost the active signal or changed queued fallback")
	}
}
func TestResetDoesNotMergeNewNotificationsIntoDiscardedGeneration(t *testing.T) {
	w := testBufferedWorker()
	first, _, _ := w.acceptAndEnqueue(channel.Event{RoomID: "r", MessageID: "first", NotificationOnly: true})
	_, ok, err := w.acceptAndEnqueue(channel.Event{RoomID: "r", MessageID: "reset", Text: `<slash-command name="new" arg="conversation"></slash-command>`})
	if err != nil || !ok {
		t.Fatal(err)
	}
	next, ok, err := w.acceptAndEnqueue(channel.Event{RoomID: "r", MessageID: "next", NotificationOnly: true})
	if err != nil || !ok {
		t.Fatal("next notification lost")
	}
	if w.isCurrentGeneration(first) || !w.isCurrentGeneration(next) {
		t.Fatal("wrong generation")
	}
	w.discardQueued(first.dedupeKey)
	if w.notifications[next.conversationKey] != next.dedupeKey {
		t.Fatal("old cleanup removed current notification")
	}
}

func TestTaskTopicSignalsItsRunningRequestAndKeepsQueuedFallback(t *testing.T) {
	w := testBufferedWorker()
	w.active = make(map[agentengine.ConversationKey]map[string]activeEvent)
	main, _, _ := w.acceptAndEnqueue(channel.Event{RoomID: "r", MessageID: "request", NotificationOnly: true})
	w.beginProcessing(main.dedupeKey)
	updates := make(chan struct{}, 1)
	if !w.setActive(main, activeEvent{unread: updates}) {
		t.Fatal("could not register running request")
	}
	topic, accepted, err := w.acceptAndEnqueue(channel.Event{RoomID: "r", MessageID: "claim", ThreadRootID: "request", NotificationOnly: true})
	if err != nil || !accepted || len(updates) != 1 {
		t.Fatalf("task update should signal the running request and stay queued: %v, %v", accepted, err)
	}
	other, _, _ := w.acceptAndEnqueue(channel.Event{RoomID: "r", MessageID: "other-reply", ThreadRootID: "other-request", NotificationOnly: true})
	pending := []queuedEvent{topic, other}
	active := map[agentengine.ConversationKey]queuedEvent{main.conversationKey: main}
	if got := nextDispatchable(pending, active); got != 1 {
		t.Fatalf("unrelated topic should run while the matching topic waits, got %d", got)
	}
	delete(active, main.conversationKey)
	if got := nextDispatchable(pending, active); got != 0 {
		t.Fatalf("queued task topic should run after its request, got %d", got)
	}
}
