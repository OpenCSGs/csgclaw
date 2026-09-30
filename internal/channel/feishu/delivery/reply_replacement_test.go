package delivery

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	channel "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu/presentation"
	state "csgclaw/internal/channel/feishu/state"
	"csgclaw/internal/channel/feishu/transport"
)

func TestFinalReplyReplacement(t *testing.T) {
	for _, scenario := range []string{"failed-create", "failed-update", "final-create", "retry-exhausted"} {
		t.Run(scenario, func(t *testing.T) {
			store := state.NewStore()
			adapter := &recordingAdapter{messageID: "remote"}
			d, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: adapter, RetryInterval: time.Nanosecond})
			create := channel.DeliveryIntent{ID: "turn:reply:000000:create", BindingID: "binding", TurnID: "turn", ChatID: "chat", ReplyTo: "origin", ThreadID: "thread", Kind: channel.DeliveryCard, Card: presentation.Card("partial")}
			final := create
			final.ID = "turn:reply:000000:final"
			final.Kind = channel.DeliveryCardUpdate
			final.RelatedID = create.ID
			final.FinalReply = true
			final.Card = presentation.Card("完整最终内容😀")
			if scenario == "final-create" {
				final.ID = create.ID
				final.Kind = channel.DeliveryCard
				final.RelatedID = ""
				adapter.textErr = errors.New("rejected")
			} else {
				_ = store.Enqueue(create)
				if scenario == "failed-create" {
					adapter.textErr = errors.New("rejected")
				}
				d.drain(context.Background())
				adapter.textErr = nil
				adapter.updateErr = errors.New("deleted card")
				if scenario == "retry-exhausted" {
					adapter.updateErr = &transport.APIError{HTTPStatus: 503}
				}
			}
			_ = store.Enqueue(final)
			attempts := 1
			if scenario == "retry-exhausted" {
				attempts = 3
			}
			for i := 0; i < attempts; i++ {
				d.drain(context.Background())
				if i < attempts-1 {
					if _, ok := store.Intent(final.ID + ":replacement"); ok {
						t.Fatal("replacement before retries exhausted")
					}
				}
			}
			replacement, ok := store.Intent(final.ID + ":replacement")
			if !ok || replacement.RelatedID != "" || replacement.MessageID != "" || replacement.FinalReply || replacement.Attempts != 0 || !reflect.DeepEqual(replacement.Card, final.Card) {
				t.Fatalf("replacement=%+v", replacement)
			}
			adapter.textErr = nil
			d.drain(context.Background())
			d.drain(context.Background())
			d.enqueueFinalReplyReplacement(final) // Stable ID must not replay a delivered replacement.
			d.drain(context.Background())
			sends := 0
			for _, req := range adapter.texts {
				if req.IdempotencyKey == replacement.ID {
					sends++
					if req.ChatID != "chat" || req.ReplyTo != "origin" || req.ThreadID != "thread" {
						t.Fatal("routing changed")
					}
				}
			}
			if sends != 1 {
				t.Fatalf("replacement sends=%d", sends)
			}
			saved, _ := store.Intent(replacement.ID)
			if saved.Status != channel.DeliveryDelivered {
				t.Fatalf("replacement status=%s", saved.Status)
			}
		})
	}
}

func TestReplacementFailureIsBoundedAndUsesSameKey(t *testing.T) {
	store := state.NewStore()
	adapter := &recordingAdapter{textErr: &transport.APIError{HTTPStatus: 503}}
	d, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: adapter, RetryInterval: time.Nanosecond})
	final := channel.DeliveryIntent{ID: "final", BindingID: "binding", TurnID: "turn", ChatID: "chat", Kind: channel.DeliveryCard, FinalReply: true, Card: presentation.Card("answer")}
	_ = store.Enqueue(final)
	for i := 0; i < 12; i++ {
		d.drain(context.Background())
	}
	if len(adapter.texts) != 6 {
		t.Fatalf("unbounded or missing retries: %d", len(adapter.texts))
	}
	if _, ok := store.Intent("final:replacement:replacement"); ok {
		t.Fatal("recursive replacement")
	}
	for i, req := range adapter.texts {
		want := "final"
		if i >= 3 {
			want = "final:replacement"
		}
		if req.IdempotencyKey != want {
			t.Fatal("unstable retry key")
		}
	}
}

func TestStreamingAndInteractionFailuresDoNotReplace(t *testing.T) {
	store := state.NewStore()
	adapter := &recordingAdapter{textErr: errors.New("rejected")}
	d, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: adapter})
	for _, id := range []string{"stream", "interaction"} {
		_ = store.Enqueue(channel.DeliveryIntent{ID: id, BindingID: "binding", TurnID: "turn", ChatID: "chat", Kind: channel.DeliveryCard, Card: presentation.Card("not final")})
	}
	for i := 0; i < 3; i++ {
		d.drain(context.Background())
	}
	if len(adapter.texts) != 2 {
		t.Fatal("non-final failure replaced")
	}
}

func TestFinalReplyPagesReplaceFailedDependencyInOrder(t *testing.T) {
	store := state.NewStore()
	adapter := &recordingAdapter{textErr: errors.New("rejected"), messageID: "remote"}
	d, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: adapter})
	for i, id := range []string{"page0", "page1"} {
		related := ""
		if i > 0 {
			related = "page0"
		}
		_ = store.Enqueue(channel.DeliveryIntent{ID: id, BindingID: "binding", TurnID: "turn", ChatID: "chat", Kind: channel.DeliveryCard, RelatedID: related, FinalReply: true, Card: presentation.Card(id)})
	}
	d.drain(context.Background())
	adapter.textErr = nil
	d.drain(context.Background())
	if len(adapter.texts) != 3 {
		t.Fatalf("sends=%d", len(adapter.texts))
	}
	for i, id := range []string{"page0:replacement", "page1:replacement"} {
		if adapter.texts[i+1].IdempotencyKey != id {
			t.Fatal("replacement page order changed")
		}
	}
}
