package im

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestBusConcurrentSubscriberCancellation(t *testing.T) {
	bus := NewBus()
	var publishers sync.WaitGroup
	for range 8 {
		publishers.Add(1)
		go func() {
			defer publishers.Done()
			for range 1000 {
				bus.Publish(Event{Type: EventTypeMessageCreated})
				runtime.Gosched()
			}
		}()
	}
	for range 1000 {
		events, cancel := bus.Subscribe()
		runtime.Gosched()
		cancel()
		cancel()
		for range events {
		}
	}
	publishers.Wait()

	events, cancel := bus.Subscribe()
	defer cancel()
	bus.Publish(Event{Type: EventTypeRoomCreated, RoomID: "test-room"})
	select {
	case event := <-events:
		if event.Type != EventTypeRoomCreated || event.RoomID != "test-room" {
			t.Fatalf("event = %+v, want new room event", event)
		}
	case <-time.After(time.Second):
		t.Fatal("new subscription did not receive an event")
	}
}

func TestBusSlowSubscriberDoesNotBlockPublishingOrCancellation(t *testing.T) {
	bus := NewBus()
	events, cancel := bus.Subscribe()
	done := make(chan struct{})
	go func() {
		for range 100 {
			bus.Publish(Event{Type: EventTypeMessageCreated})
		}
		cancel()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("full subscriber buffer blocked publishing or cancellation")
	}
	for range events {
	}
}
