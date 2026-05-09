package feishu

import (
	"testing"
	"time"

	"csgclaw/internal/im"
)

func TestFeishuMessageBusPublishesToSubscribers(t *testing.T) {
	bus := NewMessageBus()
	events, cancel := bus.Subscribe()
	defer cancel()

	message := im.Message{ID: "om_1", SenderID: "ou_manager", Content: "hello", Mentions: []im.Mention{{ID: "ou_dev"}}}
	bus.Publish(MessageEvent{
		Type:    MessageEventTypeMessageCreated,
		RoomID:  "oc_alpha",
		Message: &message,
	})

	select {
	case evt := <-events:
		if evt.Type != MessageEventTypeMessageCreated || evt.RoomID != "oc_alpha" || evt.Message == nil || evt.Message.ID != "om_1" || len(evt.Message.Mentions) != 1 || evt.Message.Mentions[0].ID != "ou_dev" {
			t.Fatalf("event = %+v, want message.created for om_1 in oc_alpha", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for feishu message event")
	}
}

func TestFeishuMessageBusCancelClosesSubscription(t *testing.T) {
	bus := NewMessageBus()
	events, cancel := bus.Subscribe()

	cancel()

	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("subscription channel open after cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscription channel to close")
	}
}
