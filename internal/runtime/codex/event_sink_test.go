package codex

import (
	"fmt"
	"strconv"
	"testing"
	"time"
)

func TestEventSinkReliableBurstDoesNotBlockPublisher(t *testing.T) {
	t.Parallel()

	sink := NewEventSink()
	events, cancel := sink.Subscribe("rt-1")
	defer cancel()

	eventCount := defaultSessionEventBuffer * 3
	published := make(chan struct{})
	go func() {
		defer close(published)
		for i := 0; i < eventCount; i++ {
			sink.Publish(SessionEvent{
				RuntimeID: "rt-1",
				Kind:      SessionEventTextDelta,
				Text:      strconv.Itoa(i),
			})
		}
		sink.Publish(SessionEvent{RuntimeID: "rt-1", Kind: SessionEventPromptCompleted})
	}()

	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("reliable event burst blocked the publisher before the subscriber drained")
	}

	for i := 0; i < eventCount; i++ {
		select {
		case event := <-events:
			if event.Kind != SessionEventTextDelta || event.Text != strconv.Itoa(i) {
				t.Fatalf("event %d = %#v, want ordered text delta", i, event)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for text delta %d", i)
		}
	}
	select {
	case event := <-events:
		if event.Kind != SessionEventPromptCompleted {
			t.Fatalf("last event = %#v, want prompt completed", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for prompt completion after reliable burst")
	}
}

func TestEventSinkReliablyDeliversActionEventsWhenSubscriberBufferIsFull(t *testing.T) {
	t.Parallel()

	sink := NewEventSink()
	events, cancel := sink.Subscribe("rt-1")
	defer cancel()

	for i := 0; i < defaultSessionEventBuffer; i++ {
		sink.Publish(SessionEvent{
			RuntimeID: "rt-1",
			Kind:      SessionEventToolCallUpdate,
		})
	}
	sink.Publish(SessionEvent{
		RuntimeID: "rt-1",
		Kind:      SessionEventPermissionRequest,
		ActionID:  "perm-1",
		Payload: PermissionSnapshot{
			ID:     "perm-1",
			Kind:   PermissionKindPermission,
			Status: PermissionStatusPending,
		},
	})

	deadline := time.After(3 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Kind == SessionEventPermissionRequest && event.ActionID == "perm-1" {
				return
			}
		case <-deadline:
			t.Fatal("permission action event was not delivered after draining full subscriber buffer")
		}
	}
}

func TestEventSinkPreservesReliableActionEventOrder(t *testing.T) {
	t.Parallel()

	sink := NewEventSink()
	events, cancel := sink.Subscribe("rt-1")
	defer cancel()

	for i := 0; i < defaultSessionEventBuffer; i++ {
		sink.Publish(SessionEvent{
			RuntimeID: "rt-1",
			Kind:      SessionEventToolCallUpdate,
		})
	}
	sink.Publish(SessionEvent{
		RuntimeID: "rt-1",
		Kind:      SessionEventPermissionRequest,
		ActionID:  "perm-1",
	})
	sink.Publish(SessionEvent{
		RuntimeID: "rt-1",
		Kind:      SessionEventPermissionDecision,
		ActionID:  "perm-1",
	})

	var got []SessionEventKind
	deadline := time.After(3 * time.Second)
	for len(got) < 2 {
		select {
		case event := <-events:
			if event.ActionID == "perm-1" {
				got = append(got, event.Kind)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for reliable events, got %v", got)
		}
	}
	if got[0] != SessionEventPermissionRequest || got[1] != SessionEventPermissionDecision {
		t.Fatalf("reliable event order = %v, want request then decision", got)
	}
}

func TestEventSinkPreservesReliableUserInputEventOrder(t *testing.T) {
	t.Parallel()

	sink := NewEventSink()
	events, cancel := sink.Subscribe("rt-1")
	defer cancel()

	for i := 0; i < defaultSessionEventBuffer; i++ {
		sink.Publish(SessionEvent{RuntimeID: "rt-1", Kind: SessionEventToolCallUpdate})
	}
	sink.Publish(SessionEvent{RuntimeID: "rt-1", Kind: SessionEventUserInputRequest, UserInputID: "question-1"})
	sink.Publish(SessionEvent{RuntimeID: "rt-1", Kind: SessionEventUserInputResolved, UserInputID: "question-1"})

	var got []SessionEventKind
	deadline := time.After(3 * time.Second)
	for len(got) < 2 {
		select {
		case event := <-events:
			if event.UserInputID == "question-1" {
				got = append(got, event.Kind)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for reliable user-input events, got %v", got)
		}
	}
	if got[0] != SessionEventUserInputRequest || got[1] != SessionEventUserInputResolved {
		t.Fatalf("reliable user-input event order = %v, want request then resolution", got)
	}
}

func TestEventSinkPreservesQuestionResolutionBeforeContinuedAssistantResponse(t *testing.T) {
	t.Parallel()

	sink := NewEventSink()
	events, cancel := sink.Subscribe("rt-1")
	defer cancel()

	for i := 0; i < defaultSessionEventBuffer; i++ {
		sink.Publish(SessionEvent{RuntimeID: "rt-1", Kind: SessionEventToolCallUpdate})
	}
	sink.Publish(SessionEvent{RuntimeID: "rt-1", Kind: SessionEventUserInputResolved, UserInputID: "question-1"})
	sink.Publish(SessionEvent{RuntimeID: "rt-1", Kind: SessionEventTextDelta, Text: "continued response"})
	sink.Publish(SessionEvent{RuntimeID: "rt-1", Kind: SessionEventPromptCompleted})

	var got []SessionEventKind
	deadline := time.After(3 * time.Second)
	for len(got) < 3 {
		select {
		case event := <-events:
			if event.UserInputID == "question-1" || event.Text == "continued response" || event.Kind == SessionEventPromptCompleted {
				got = append(got, event.Kind)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for continued response events, got %v", got)
		}
	}
	want := []SessionEventKind{SessionEventUserInputResolved, SessionEventTextDelta, SessionEventPromptCompleted}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("continued response event order = %v, want %v", got, want)
	}
}

func TestEventSinkStillDropsNonReliableEventsWhenSubscriberBufferIsFull(t *testing.T) {
	t.Parallel()

	sink := NewEventSink()
	events, cancel := sink.Subscribe("rt-1")
	defer cancel()

	for i := 0; i < defaultSessionEventBuffer; i++ {
		sink.Publish(SessionEvent{
			RuntimeID: "rt-1",
			Kind:      SessionEventToolCallUpdate,
		})
	}
	sink.Publish(SessionEvent{
		RuntimeID:  "rt-1",
		Kind:       SessionEventToolCallUpdate,
		ToolCallID: "dropped-tool",
		ToolStatus: "completed",
		ReceivedAt: time.Now().UTC(),
	})

	for i := 0; i < defaultSessionEventBuffer; i++ {
		event := <-events
		if event.ToolCallID == "dropped-tool" {
			t.Fatal("non-reliable tool event was delivered despite full subscriber buffer")
		}
	}
	select {
	case event := <-events:
		t.Fatalf("unexpected extra event after draining buffer: %+v", event)
	case <-time.After(50 * time.Millisecond):
	}
}
