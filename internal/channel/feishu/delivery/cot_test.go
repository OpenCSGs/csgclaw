package delivery

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	channel "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu/presentation"
	feishustate "csgclaw/internal/channel/feishu/state"
	"csgclaw/internal/channel/feishu/transport"
)

type cotRecordingAdapter struct {
	*recordingAdapter
	cotMu            sync.Mutex
	events           []channel.COTEvent
	completed        int
	creates          int
	failUpdate       bool
	completeFailures int
	completeAttempts int
	block            <-chan struct{}
	entered          chan struct{}
}

func (a *cotRecordingAdapter) CreateCOT(ctx context.Context, _ transport.COTCreateRequest) (transport.COTRef, error) {
	a.cotMu.Lock()
	a.creates++
	a.cotMu.Unlock()
	if a.entered != nil {
		close(a.entered)
	}
	if a.block != nil {
		select {
		case <-a.block:
		case <-ctx.Done():
			return transport.COTRef{}, ctx.Err()
		}
	}
	return transport.COTRef{COTID: "cot", MessageID: "process"}, nil
}
func (a *cotRecordingAdapter) UpdateCOT(_ context.Context, req transport.COTUpdateRequest) error {
	a.cotMu.Lock()
	defer a.cotMu.Unlock()
	if req.Ref != (transport.COTRef{COTID: "cot", MessageID: "process"}) {
		return errors.New("incorrect COT identifiers")
	}
	if a.failUpdate {
		return errors.New("ambiguous update")
	}
	a.events = append(a.events, req.Events...)
	return nil
}
func (a *cotRecordingAdapter) CompleteCOT(_ context.Context, req transport.COTCompleteRequest) error {
	a.cotMu.Lock()
	defer a.cotMu.Unlock()
	if req.Ref != (transport.COTRef{COTID: "cot", MessageID: "process"}) {
		return errors.New("incorrect COT identifiers")
	}
	a.completeAttempts++
	if a.completeFailures > 0 {
		a.completeFailures--
		return &transport.APIError{Operation: "complete COT", HTTPStatus: 503}
	}
	a.completed++
	return nil
}
func cotIntent(id string, kind channel.DeliveryKind) channel.DeliveryIntent {
	return channel.DeliveryIntent{ID: id, TurnID: "turn", BindingID: "binding", ChatID: "chat", Kind: kind, RelatedID: "create"}
}
func TestCOTPreservesEventsAndCompletesAfterUpdates(t *testing.T) {
	store := feishustate.NewStore()
	a := &cotRecordingAdapter{recordingAdapter: &recordingAdapter{}}
	d, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: a})
	create := cotIntent("create", channel.DeliveryCOTCreate)
	create.RelatedID = ""
	_ = store.Enqueue(create)
	for _, id := range []string{"first", "second"} {
		i := cotIntent(id, channel.DeliveryCOTUpdate)
		i.Events = []channel.COTEvent{{EventType: id}}
		_ = store.Enqueue(i)
	}
	end := cotIntent("end", channel.DeliveryCOTComplete)
	end.Reason = "done"
	end.Events = []channel.COTEvent{{EventType: "finished"}}
	enqueueCOTEnd(t, store, end)
	d.drainCOT(context.Background())
	d.drainCOT(context.Background())
	if a.creates != 1 || a.completed != 1 || len(a.events) != 3 || a.events[0].EventType != "first" || a.events[2].EventType != "finished" {
		t.Fatalf("events=%+v creates=%d completes=%d", a.events, a.creates, a.completed)
	}
}
func TestCOTFailureDoesNotReplayOrSendLaterSuffix(t *testing.T) {
	store := feishustate.NewStore()
	a := &cotRecordingAdapter{recordingAdapter: &recordingAdapter{}, failUpdate: true}
	d, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: a})
	create := cotIntent("create", channel.DeliveryCOTCreate)
	create.RelatedID = ""
	_ = store.Enqueue(create)
	first := cotIntent("first", channel.DeliveryCOTUpdate)
	first.Events = []channel.COTEvent{{EventType: "first"}}
	_ = store.Enqueue(first)
	d.drainCOT(context.Background())
	a.failUpdate = false
	next := cotIntent("second", channel.DeliveryCOTUpdate)
	next.Events = []channel.COTEvent{{EventType: "second"}}
	_ = store.Enqueue(next)
	end := cotIntent("end", channel.DeliveryCOTComplete)
	end.Reason = "error"
	enqueueCOTEnd(t, store, end)
	d.drainCOT(context.Background())
	if len(a.events) != 0 || a.completed != 1 {
		t.Fatalf("events=%v completed=%d", a.events, a.completed)
	}
	if _, ok := store.Delivery("turn:cot:unavailable"); !ok {
		t.Fatal("missing unavailable notice")
	}
}
func TestSlowCOTDoesNotBlockReplyCard(t *testing.T) {
	store := feishustate.NewStore()
	block := make(chan struct{})
	a := &cotRecordingAdapter{recordingAdapter: &recordingAdapter{messageID: "reply"}, block: block, entered: make(chan struct{})}
	d, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: a})
	create := cotIntent("create", channel.DeliveryCOTCreate)
	create.RelatedID = ""
	_ = store.Enqueue(create)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	select {
	case <-a.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("COT did not start")
	}
	_ = store.Enqueue(channel.DeliveryIntent{ID: "reply", TurnID: "turn", BindingID: "binding", ChatID: "chat", Kind: channel.DeliveryCard, Card: presentation.Card("answer")})
	d.Notify()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		item, _ := store.Delivery("reply")
		if item.Status == channel.DeliveryDelivered {
			close(block)
			return
		}
		time.Sleep(time.Millisecond)
	}
	close(block)
	t.Fatal("reply waited for COT")
}

func TestCOTCompletionRetriesWithoutAppendingEventsAgain(t *testing.T) {
	store := feishustate.NewStore()
	a := &cotRecordingAdapter{recordingAdapter: &recordingAdapter{}, completeFailures: 1}
	d, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: a, RetryInterval: time.Millisecond})
	create := cotIntent("create", channel.DeliveryCOTCreate)
	create.RelatedID = ""
	_ = store.Enqueue(create)
	end := cotIntent("turn:cot:complete", channel.DeliveryCOTComplete)
	end.Events = []channel.COTEvent{{EventType: "RUN_FINISHED"}}
	end.Reason = "done"
	enqueueCOTEnd(t, store, end)
	d.drainCOT(context.Background())
	pending, _ := store.Delivery(end.ID)
	if pending.Status != channel.DeliveryPending || len(pending.Events) != 0 {
		t.Fatalf("retry=%+v", pending)
	}
	time.Sleep(2 * time.Millisecond)
	d.drainCOT(context.Background())
	finished, _ := store.Delivery(end.ID)
	if finished.Status != channel.DeliveryDelivered || len(a.events) != 1 || a.completeAttempts != 2 {
		t.Fatalf("status=%s events=%d attempts=%d", finished.Status, len(a.events), a.completeAttempts)
	}
	if _, found := store.Delivery("turn:cot:unavailable"); found {
		t.Fatal("transient completion failure sent unavailable card")
	}
}

func TestCOTCompletionCanBeRetriedByUserAfterExhaustion(t *testing.T) {
	store := feishustate.NewStore()
	a := &cotRecordingAdapter{recordingAdapter: &recordingAdapter{}, completeFailures: 3}
	d, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: a, RetryInterval: time.Nanosecond})
	create := cotIntent("create", channel.DeliveryCOTCreate)
	create.RelatedID = ""
	_ = store.Enqueue(create)
	end := cotIntent("turn:cot:complete", channel.DeliveryCOTComplete)
	end.Events = []channel.COTEvent{{EventType: "RUN_FINISHED"}}
	enqueueCOTEnd(t, store, end)
	for i := 0; i < 3; i++ {
		d.drainCOT(context.Background())
	}
	failed, _ := store.Delivery(end.ID)
	if failed.Status != channel.DeliveryFailed {
		t.Fatalf("status=%s", failed.Status)
	}
	if err := store.RetryCOTCompletion(create.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.RetryCOTCompletion(create.ID); err != nil {
		t.Fatal(err)
	}
	d.drainCOT(context.Background())
	if a.completed != 1 || len(a.events) != 1 {
		t.Fatalf("completed=%d events=%d", a.completed, len(a.events))
	}
	_ = store.RetryCOTCompletion(create.ID)
	d.drainCOT(context.Background())
	if a.completeAttempts != 4 {
		t.Fatalf("repeated completed request: %d", a.completeAttempts)
	}
}

func TestCOTFinalAppendFailureStillCompletes(t *testing.T) {
	store := feishustate.NewStore()
	a := &cotRecordingAdapter{recordingAdapter: &recordingAdapter{}, failUpdate: true}
	d, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: a})
	create := cotIntent("create", channel.DeliveryCOTCreate)
	create.RelatedID = ""
	_ = store.Enqueue(create)
	end := cotIntent("end", channel.DeliveryCOTComplete)
	end.Events = []channel.COTEvent{{EventType: "RUN_FINISHED"}}
	enqueueCOTEnd(t, store, end)
	d.drainCOT(context.Background())
	finished, _ := store.Delivery(end.ID)
	if finished.Status != channel.DeliveryDelivered || a.completed != 1 {
		t.Fatalf("status=%s complete=%d", finished.Status, a.completed)
	}
}

func enqueueCOTEnd(t *testing.T, store *feishustate.Store, end channel.DeliveryIntent) {
	t.Helper()
	if len(end.Events) > 0 {
		events := end
		events.ID += ":events"
		events.Kind = channel.DeliveryCOTUpdate
		if err := store.Enqueue(events); err != nil {
			t.Fatal(err)
		}
		end.Events = nil
	}
	if err := store.Enqueue(end); err != nil {
		t.Fatal(err)
	}
}

func TestCOTCompletionFailureKeepsRetryCardAfterAppendFailure(t *testing.T) {
	store := feishustate.NewStore()
	adapter := &cotRecordingAdapter{recordingAdapter: &recordingAdapter{}, failUpdate: true, completeFailures: 3}
	dispatcher, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: adapter, RetryInterval: time.Nanosecond})
	create := cotIntent("create", channel.DeliveryCOTCreate)
	create.RelatedID = ""
	_ = store.Enqueue(create)
	end := cotIntent("turn:cot:complete", channel.DeliveryCOTComplete)
	end.Events = []channel.COTEvent{{EventType: "RUN_FINISHED"}}
	enqueueCOTEnd(t, store, end)
	for i := 0; i < 3; i++ {
		dispatcher.drainCOT(context.Background())
	}
	for _, id := range []string{"turn:cot:unavailable", "turn:cot:completion-failed"} {
		if _, found := store.Delivery(id); !found {
			t.Fatalf("missing notice %s", id)
		}
	}
}
