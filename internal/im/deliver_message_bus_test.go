package im

import (
	"testing"
	"time"
)

func TestDeliverMessagePublishesMessageCreatedEvent(t *testing.T) {
	bus := NewBus()
	svc := NewServiceFromBootstrapWithBus(Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "manager", Name: "manager", Role: "manager"},
			{ID: "u-p-w-0604", Name: "worker", Role: "worker"},
		},
		Rooms: []Room{{
			ID:      "room-1",
			Title:   "task room",
			Members: []string{"manager", "u-p-w-0604"},
		}},
	}, bus)

	events, cancel := bus.Subscribe()
	defer cancel()

	_, err := svc.DeliverMessage(DeliverMessageRequest{
		RoomID:    "room-1",
		SenderID:  "manager",
		MentionID: "u-p-w-0604",
		Content:   "manager dispatched task task-17 to u-p-w-0604",
	})
	if err != nil {
		t.Fatalf("DeliverMessage() error = %v", err)
	}

	select {
	case evt := <-events:
		if evt.Type != EventTypeMessageCreated || evt.Message == nil {
			t.Fatalf("event = %+v, want message.created", evt)
		}
		if len(evt.Message.Mentions) == 0 || evt.Message.Mentions[0].ID != "user-p-w-0604" {
			t.Fatalf("mentions = %+v, want user-p-w-0604", evt.Message.Mentions)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message.created event")
	}
}
