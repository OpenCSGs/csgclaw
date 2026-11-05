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

func TestEventSinkSessionSubscriptionOnlyQueuesMatchingSession(t *testing.T) {
	t.Parallel()

	sink := NewEventSink()
	events, cancel := sink.SubscribeSession("rt-1", "session-active")
	defer cancel()

	for index := 0; index < defaultSessionEventBuffer*3; index++ {
		sink.Publish(SessionEvent{
			RuntimeID: "rt-1",
			SessionID: "session-idle",
			Kind:      SessionEventTextDelta,
			Text:      strconv.Itoa(index),
		})
	}
	sink.Publish(SessionEvent{
		RuntimeID: "rt-1",
		SessionID: "session-active",
		Kind:      SessionEventPromptCompleted,
	})

	select {
	case event := <-events:
		if event.Kind != SessionEventPromptCompleted || event.SessionID != "session-active" {
			t.Fatalf("event = %#v, want only matching session completion", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for matching session event")
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
		ToolStatus: "running",
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

func TestEventSinkPreservesToolLifecycleWithSlowSubscriber(t *testing.T) {
	t.Parallel()

	for _, status := range []string{"completed", "failed", "canceled"} {
		t.Run(status, func(t *testing.T) {
			sink := NewEventSink()
			events, cancel := sink.SubscribeSession("rt-1", "session-1")
			defer cancel()

			publish := func(event SessionEvent) {
				event.RuntimeID = "rt-1"
				event.SessionID = "session-1"
				sink.Publish(event)
			}
			for i := 0; i < defaultSessionEventBuffer; i++ {
				publish(SessionEvent{Kind: SessionEventToolCallUpdate, ToolCallID: "filler", ToolStatus: "running"})
			}

			want := []SessionEvent{
				{Kind: SessionEventTextDelta, Text: "before"},
				{Kind: SessionEventToolCallStart, ToolCallID: "call"},
				{Kind: SessionEventToolCallUpdate, ToolCallID: "call", ToolStatus: status},
				{Kind: SessionEventTextDelta, Text: "after"},
				{Kind: SessionEventPromptCompleted},
			}
			for _, event := range want {
				publish(event)
				if event.Kind == SessionEventToolCallStart {
					publish(SessionEvent{Kind: SessionEventToolCallUpdate, ToolCallID: "call", ToolStatus: "running"})
				}
			}

			for i := 0; i < defaultSessionEventBuffer; i++ {
				if got := <-events; got.ToolCallID != "filler" {
					t.Fatalf("buffered event %d = %+v", i, got)
				}
			}
			deadline := time.After(time.Second)
			for _, expected := range want {
				select {
				case got := <-events:
					if got.Kind != expected.Kind || got.ToolCallID != expected.ToolCallID || got.ToolStatus != expected.ToolStatus || got.Text != expected.Text {
						t.Fatalf("got %+v, want %+v", got, expected)
					}
				case <-deadline:
					t.Fatalf("timed out waiting for %+v", expected)
				}
			}
		})
	}
}

func TestEventSinkKeepsToolProgressBetweenStartAndCompletion(t *testing.T) {
	t.Parallel()

	sink := NewEventSink()
	events, cancel := sink.Subscribe("rt-1")
	defer cancel()
	want := []SessionEvent{
		{RuntimeID: "rt-1", Kind: SessionEventTextDelta, Text: "before"},
		{RuntimeID: "rt-1", Kind: SessionEventToolCallStart, ToolCallID: "call"},
		{RuntimeID: "rt-1", Kind: SessionEventToolCallUpdate, ToolCallID: "call", ToolStatus: "running"},
		{RuntimeID: "rt-1", Kind: SessionEventToolCallUpdate, ToolCallID: "call", ToolStatus: "completed"},
		{RuntimeID: "rt-1", Kind: SessionEventTextDelta, Text: "after"},
	}
	for _, event := range want {
		sink.Publish(event)
	}
	deadline := time.After(time.Second)
	for _, expected := range want {
		select {
		case got := <-events:
			if got != expected {
				t.Fatalf("got %+v, want %+v", got, expected)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %+v", expected)
		}
	}
}
