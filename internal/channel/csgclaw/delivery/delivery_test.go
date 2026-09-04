package delivery

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
)

type recordingStore struct {
	messages   []deliveredMessage
	activities int
}

func TestTranscriptRendererPreservesStructuredQuestionText(t *testing.T) {
	for _, text := range []string{"## Step 1\n\nChoose the workflow branch.", "## 交互式输出演示 - 第 2/3 步\n\n请配置验证方式。", ""} {
		t.Run(text, func(t *testing.T) {
			store := &recordingStore{}
			renderer := NewTranscriptRenderer(store)
			turn := channel.TurnContext{RoomID: "room-demo", SourceMessageID: "message-demo"}
			for _, event := range []agentengine.TurnEvent{
				{Kind: agentengine.TurnEventTextDelta, Text: "Earlier model commentary."},
				{Kind: agentengine.TurnEventOutputItem, Text: text, Output: &agentengine.OutputItem{
					Kind:    agentengine.OutputItemRequestUserInput,
					Payload: activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{ID: "demo_kind", Header: "Demo", Question: "Which workflow?"}}},
				}},
				{Kind: agentengine.TurnEventOutputItem, Output: &agentengine.OutputItem{
					Kind:    agentengine.OutputItemResourceLink,
					Payload: activity.ResourceLink{Type: "resource_link", Name: "docs", URI: "https://example.com/docs"},
				}},
			} {
				if err := renderer.Emit(context.Background(), turn, event); err != nil {
					t.Fatal(err)
				}
			}
			if err := renderer.Complete(context.Background(), turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded}); err != nil {
				t.Fatal(err)
			}
			want := text
			if want == "" {
				want = "Earlier model commentary."
			}
			want += "\n\nLinks\n- [docs](<https://example.com/docs>)"
			if len(store.messages) != 1 || store.messages[0].text != want {
				t.Fatalf("messages = %+v, want %q", store.messages, want)
			}
		})
	}
}

type deliveredMessage struct {
	turn channel.TurnContext
	text string
}

type thoughtRecordingStore struct {
	recordingStore
	thoughts []string
}

func (s *thoughtRecordingStore) DeliverThought(_ context.Context, _ channel.TurnContext, text string) error {
	s.thoughts = append(s.thoughts, text)
	return nil
}

func (s *recordingStore) DeliverMessage(_ context.Context, turn channel.TurnContext, text string) error {
	s.messages = append(s.messages, deliveredMessage{turn: turn, text: text})
	return nil
}

func (s *recordingStore) DeliverActivity(_ context.Context, _ channel.TurnContext, _ agentengine.TurnEvent) error {
	s.activities++
	return nil
}

func TestTranscriptRendererWritesFinalTextAndSkipsCanceled(t *testing.T) {
	store := &recordingStore{}
	renderer := NewTranscriptRenderer(store)
	turn := channel.TurnContext{RoomID: "room-1", SourceMessageID: "m1"}

	if err := renderer.Emit(context.Background(), turn, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "hel"}); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	if err := renderer.Emit(context.Background(), turn, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "lo"}); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
	if err := renderer.Emit(context.Background(), turn, agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallStart}); err != nil {
		t.Fatalf("Emit tool error = %v", err)
	}
	if err := renderer.Complete(context.Background(), turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded}); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if store.activities != 1 || len(store.messages) != 1 || store.messages[0].text != "hello" {
		t.Fatalf("store = %+v", store)
	}

	if err := renderer.Complete(context.Background(), turn, agentengine.TurnResult{Status: agentengine.TurnCanceled}); err != nil {
		t.Fatalf("canceled Complete() error = %v", err)
	}
	if len(store.messages) != 1 {
		t.Fatalf("canceled turn wrote transcript: %+v", store.messages)
	}

	if err := renderer.Complete(context.Background(), turn, agentengine.TurnResult{
		Status: agentengine.TurnFailed,
		Error:  &agentengine.TurnError{Message: "runtime exploded"},
	}); err != nil {
		t.Fatalf("failed Complete() error = %v", err)
	}
	if len(store.messages) != 2 || store.messages[1].text != "runtime exploded" {
		t.Fatalf("failed transcript = %+v", store.messages)
	}
}

func TestTranscriptRendererIsolatesInterleavedTurns(t *testing.T) {
	store := &recordingStore{}
	renderer := NewTranscriptRenderer(store)
	first := channel.TurnContext{
		BindingID:       "binding-1",
		AgentID:         "agent-1",
		SourceMessageID: "message-1",
		ConversationKey: "conversation-1",
		TurnID:          "turn-1",
	}
	second := channel.TurnContext{
		BindingID:       "binding-2",
		AgentID:         "agent-2",
		SourceMessageID: "message-2",
		ConversationKey: "conversation-2",
		TurnID:          "turn-2",
	}

	for _, item := range []struct {
		turn channel.TurnContext
		text string
	}{
		{turn: first, text: "first "},
		{turn: second, text: "second "},
		{turn: first, text: "answer"},
		{turn: second, text: "answer"},
	} {
		if err := renderer.Emit(context.Background(), item.turn, agentengine.TurnEvent{
			Kind: agentengine.TurnEventTextDelta,
			Text: item.text,
		}); err != nil {
			t.Fatalf("Emit() error = %v", err)
		}
	}

	if err := renderer.Complete(context.Background(), first, agentengine.TurnResult{Status: agentengine.TurnSucceeded}); err != nil {
		t.Fatalf("first Complete() error = %v", err)
	}
	if err := renderer.Complete(context.Background(), second, agentengine.TurnResult{Status: agentengine.TurnSucceeded}); err != nil {
		t.Fatalf("second Complete() error = %v", err)
	}

	if len(store.messages) != 2 {
		t.Fatalf("messages = %+v, want two isolated results", store.messages)
	}
	if store.messages[0].turn.TurnID != first.TurnID || store.messages[0].text != "first answer" {
		t.Fatalf("first message = %+v", store.messages[0])
	}
	if store.messages[1].turn.TurnID != second.TurnID || store.messages[1].text != "second answer" {
		t.Fatalf("second message = %+v", store.messages[1])
	}
}

func TestTranscriptRendererCoalescesAndBoundsThoughts(t *testing.T) {
	store := &thoughtRecordingStore{}
	renderer := NewTranscriptRenderer(store)
	now := time.Unix(100, 0)
	renderer.now = func() time.Time { return now }
	turn := channel.TurnContext{TurnID: "turn-thought", SourceMessageID: "message-thought"}

	if err := renderer.Emit(context.Background(), turn, agentengine.TurnEvent{
		Kind: agentengine.TurnEventThoughtDelta, Thought: "start ",
	}); err != nil {
		t.Fatalf("first Emit() error = %v", err)
	}
	now = now.Add(100 * time.Millisecond)
	if err := renderer.Emit(context.Background(), turn, agentengine.TurnEvent{
		Kind: agentengine.TurnEventThoughtDelta, Thought: strings.Repeat("界", 800),
	}); err != nil {
		t.Fatalf("coalesced Emit() error = %v", err)
	}
	if len(store.thoughts) != 1 {
		t.Fatalf("thought deliveries = %d, want 1 before flush interval", len(store.thoughts))
	}

	now = now.Add(thoughtFlushEvery)
	if err := renderer.Emit(context.Background(), turn, agentengine.TurnEvent{
		Kind: agentengine.TurnEventThoughtDelta, Thought: " end",
	}); err != nil {
		t.Fatalf("flush Emit() error = %v", err)
	}
	if len(store.thoughts) != 2 {
		t.Fatalf("thought deliveries = %d, want 2", len(store.thoughts))
	}
	got := store.thoughts[1]
	if len(got) > thoughtTailBytes || !utf8.ValidString(got) || !strings.HasSuffix(got, " end") {
		t.Fatalf("bounded thought len=%d valid=%v suffix=%v", len(got), utf8.ValidString(got), strings.HasSuffix(got, " end"))
	}
}

func TestTranscriptRendererFlushesPendingThoughtOnComplete(t *testing.T) {
	store := &thoughtRecordingStore{}
	renderer := NewTranscriptRenderer(store)
	now := time.Unix(100, 0)
	renderer.now = func() time.Time { return now }
	turn := channel.TurnContext{TurnID: "turn-thought", SourceMessageID: "message-thought"}

	if err := renderer.Emit(context.Background(), turn, agentengine.TurnEvent{
		Kind: agentengine.TurnEventThoughtDelta, Thought: "first",
	}); err != nil {
		t.Fatalf("first Emit() error = %v", err)
	}
	now = now.Add(100 * time.Millisecond)
	if err := renderer.Emit(context.Background(), turn, agentengine.TurnEvent{
		Kind: agentengine.TurnEventThoughtDelta, Thought: " second",
	}); err != nil {
		t.Fatalf("second Emit() error = %v", err)
	}
	if err := renderer.Complete(context.Background(), turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded}); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if len(store.thoughts) != 2 || store.thoughts[1] != "first second" {
		t.Fatalf("thoughts = %#v, want final coalesced flush", store.thoughts)
	}
}
