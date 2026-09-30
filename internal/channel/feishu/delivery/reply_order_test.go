package delivery

import (
	"context"
	"testing"
	"time"

	channel "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu/presentation"
	state "csgclaw/internal/channel/feishu/state"
	"csgclaw/internal/channel/feishu/transport"
)

type orderedPageAdapter struct {
	*recordingAdapter
	first     bool
	successes []string
}

func (a *orderedPageAdapter) SendCard(_ context.Context, r transport.SendCardRequest) (transport.SendResult, error) {
	if r.IdempotencyKey == "page0:replacement" && !a.first {
		a.first = true
		return transport.SendResult{}, &transport.APIError{HTTPStatus: 503}
	}
	a.successes = append(a.successes, r.IdempotencyKey)
	return transport.SendResult{MessageID: r.IdempotencyKey}, nil
}
func TestReplacementPreservesOrderOnRetry(t *testing.T) {
	s := state.NewStore()
	a := &orderedPageAdapter{recordingAdapter: &recordingAdapter{}}
	d, _ := NewDispatcher(DispatcherOptions{State: s, Adapter: a, RetryInterval: time.Hour})
	for _, id := range []string{"page0", "page1"} {
		d.enqueueFinalReplyReplacement(channel.DeliveryIntent{ID: id, BindingID: "b", TurnID: "t", ChatID: "c", Kind: channel.DeliveryCard, FinalReply: true, Card: presentation.Card(id)})
	}
	d.drain(context.Background())
	if len(a.successes) != 0 {
		t.Fatalf("later page overtook retrying first page: %v", a.successes)
	}
	if err := s.MarkRetryable("page0:replacement", nil, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	d.drain(context.Background())
	if len(a.successes) != 2 || a.successes[0] != "page0:replacement" || a.successes[1] != "page1:replacement" {
		t.Fatalf("wrong order: %v", a.successes)
	}
}

func TestReplacementPermanentFailureReleasesNextPage(t *testing.T) {
	s := state.NewStore()
	a := &orderedPageAdapter{recordingAdapter: &recordingAdapter{}}
	d, _ := NewDispatcher(DispatcherOptions{State: s, Adapter: a, RetryInterval: time.Hour})
	for _, id := range []string{"page0", "page1"} {
		d.enqueueFinalReplyReplacement(channel.DeliveryIntent{ID: id, BindingID: "b", TurnID: "t", ChatID: "c", Kind: channel.DeliveryCard, FinalReply: true, Card: presentation.Card(id)})
	}
	d.drain(context.Background())
	if err := s.MarkFailed("page0:replacement", nil); err != nil {
		t.Fatal(err)
	}
	d.drain(context.Background())
	if len(a.successes) != 1 || a.successes[0] != "page1:replacement" {
		t.Fatalf("later page stranded: %v", a.successes)
	}
}
