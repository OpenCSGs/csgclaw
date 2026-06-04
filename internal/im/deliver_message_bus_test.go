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
			{ID: "u-manager", Name: "manager", Handle: "manager", Role: "manager"},
			{ID: "u-p-w-0604", Name: "worker", Handle: "p-w-0604", Role: "worker"},
		},
		Rooms: []Room{{
			ID:      "room-1",
			Title:   "task room",
			Members: []string{"u-manager", "u-p-w-0604"},
		}},
	}, bus)

	events, cancel := bus.Subscribe()
	defer cancel()

	_, err := svc.DeliverMessage(DeliverMessageRequest{
		RoomID:    "room-1",
		SenderID:  "u-manager",
		MentionID: "u-p-w-0604",
		Content:   "[team] Task task-17 is ready for you",
	})
	if err != nil {
		t.Fatalf("DeliverMessage() error = %v", err)
	}

	select {
	case evt := <-events:
		if evt.Type != EventTypeMessageCreated || evt.Message == nil {
			t.Fatalf("event = %+v, want message.created", evt)
		}
		if len(evt.Message.Mentions) == 0 || evt.Message.Mentions[0].ID != "u-p-w-0604" {
			t.Fatalf("mentions = %+v, want u-p-w-0604", evt.Message.Mentions)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message.created event")
	}
}
