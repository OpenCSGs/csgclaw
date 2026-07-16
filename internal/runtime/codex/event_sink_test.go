package codex

import (
	"fmt"
	"testing"
	"time"
)

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
