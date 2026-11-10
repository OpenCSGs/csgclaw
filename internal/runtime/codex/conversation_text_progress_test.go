package codex

import (
	"context"
	"fmt"
	"testing"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/delivery"
)

type nativeProgressStore struct {
	answer   string
	progress activity.TurnProgress
}

func (*nativeProgressStore) DeliverMessage(context.Context, channel.TurnContext, string) error {
	return nil
}

func (*nativeProgressStore) DeliverActivity(context.Context, channel.TurnContext, contract.TurnEvent) error {
	return nil
}

func (s *nativeProgressStore) DeliverTurnProgress(_ context.Context, _ channel.TurnContext, progress activity.TurnProgress, answer string, _ bool, _ []contract.OutputFile) error {
	s.answer, s.progress = answer, progress
	return nil
}

func TestNativeHistoryReconcilesRoomCommentary(t *testing.T) {
	for mask := 0; mask < 4; mask++ {
		t.Run(fmt.Sprintf("covered=%02b", mask), func(t *testing.T) {
			store := &nativeProgressStore{}
			renderer := delivery.NewTranscriptRenderer(store)
			turn := channel.TurnContext{AgentID: "agent", TurnID: "turn", ConversationKey: "room"}
			var sequence uint64
			sink := contract.EventSinkFunc(func(ctx context.Context, event contract.TurnEvent) error {
				sequence++
				event.Sequence = sequence
				return renderer.Emit(ctx, turn, event)
			})
			adapter := &ConversationAdapter{}
			var output conversationOutput
			var files []*contract.OutputFile
			var items []any
			for i, id := range []string{"first", "second"} {
				for _, event := range []activity.RuntimeEvent{
					{Kind: activity.RuntimeEventTextDelta, Text: "Checking files.", Payload: map[string]any{"phase": "commentary"}},
					{Kind: activity.RuntimeEventToolCallStart, ToolCallID: id, ToolKind: "exec_command", ToolStatus: "started"},
					{Kind: activity.RuntimeEventToolCallUpdate, ToolCallID: id, ToolKind: "exec_command", ToolStatus: "completed"},
				} {
					if result := adapter.handleEvent(t.Context(), contract.TurnRequest{}, sink, event, &output, &files); result != nil {
						t.Fatal(result)
					}
				}
				if mask&(1<<i) != 0 {
					items = append(items, map[string]any{"type": "agentMessage", "id": "comment-" + id, "phase": "commentary", "text": "Checking files."})
				}
				items = append(items, map[string]any{"type": "commandExecution", "id": id, "status": "completed", "command": "pwd", "aggregatedOutput": "/workspace"})
			}
			answer := activity.RuntimeEvent{Kind: activity.RuntimeEventTextDelta, MessageID: "answer", Text: "Answer.", Payload: map[string]any{"phase": "final_answer"}}
			if result := adapter.handleEvent(t.Context(), contract.TurnRequest{}, sink, answer, &output, &files); result != nil {
				t.Fatal(result)
			}
			items = append(items, map[string]any{"type": "agentMessage", "id": "answer", "phase": "final_answer", "text": "Answer."})
			history := quietTurnItems(map[string]any{"itemsView": "full", "items": items})
			for range 2 {
				if err := output.restore(t.Context(), sink, history); err != nil {
					t.Fatal(err)
				}
			}
			if err := renderer.Complete(t.Context(), turn, contract.TurnResult{Status: contract.TurnSucceeded, Output: output.String(), RuntimeText: output.messageSnapshots()}); err != nil {
				t.Fatal(err)
			}
			if store.answer != "Answer." || len(store.progress.Items) != 4 {
				t.Fatalf("answer=%q progress=%+v", store.answer, store.progress.Items)
			}
			for i, item := range store.progress.Items {
				if i%2 == 0 {
					if item.Kind != "commentary" || item.Text != "Checking files." {
						t.Fatalf("commentary=%+v", item)
					}
				} else if item.Tool == nil || item.Tool.Status != "completed" || item.ID != []string{"first", "second"}[i/2] {
					t.Fatalf("tool=%+v", item)
				}
			}
		})
	}
}

func TestNativeHistoryKeepsSubsequentRoomText(t *testing.T) {
	store := &nativeProgressStore{}
	renderer := delivery.NewTranscriptRenderer(store)
	turn := channel.TurnContext{AgentID: "agent", TurnID: "turn", ConversationKey: "room"}
	var sequence uint64
	sink := contract.EventSinkFunc(func(ctx context.Context, event contract.TurnEvent) error {
		sequence++
		event.Sequence = sequence
		return renderer.Emit(ctx, turn, event)
	})
	adapter := &ConversationAdapter{}
	var output conversationOutput
	var files []*contract.OutputFile
	earlier := activity.RuntimeEvent{Kind: activity.RuntimeEventTextDelta, MessageID: "earlier", Text: "Checking.", Payload: map[string]any{"phase": "unknown"}}
	if result := adapter.handleEvent(t.Context(), contract.TurnRequest{}, sink, earlier, &output, &files); result != nil {
		t.Fatal(result)
	}
	history := quietTurnItems(map[string]any{"itemsView": "full", "items": []any{
		map[string]any{"type": "agentMessage", "id": "earlier", "phase": "unknown", "text": "Checking."},
	}})
	if err := output.restore(t.Context(), sink, history); err != nil {
		t.Fatal(err)
	}
	for _, event := range []activity.RuntimeEvent{
		{Kind: activity.RuntimeEventToolCallStart, ToolCallID: "tool", ToolKind: "exec_command", ToolStatus: "started"},
		{Kind: activity.RuntimeEventToolCallUpdate, ToolCallID: "tool", ToolKind: "exec_command", ToolStatus: "completed"},
		{Kind: activity.RuntimeEventTextDelta, MessageID: "answer", Text: "Answer.", Payload: map[string]any{"phase": "unknown"}},
	} {
		if result := adapter.handleEvent(t.Context(), contract.TurnRequest{}, sink, event, &output, &files); result != nil {
			t.Fatal(result)
		}
	}
	if err := renderer.Complete(t.Context(), turn, contract.TurnResult{Status: contract.TurnSucceeded, Output: output.String(), RuntimeText: output.messageSnapshots()}); err != nil {
		t.Fatal(err)
	}
	if store.answer != "Answer." || len(store.progress.Items) != 2 || store.progress.Items[0].Text != "Checking." || store.progress.Items[1].Tool == nil || store.progress.Items[1].Tool.Status != "completed" {
		t.Fatalf("answer=%q progress=%+v", store.answer, store.progress.Items)
	}
}
