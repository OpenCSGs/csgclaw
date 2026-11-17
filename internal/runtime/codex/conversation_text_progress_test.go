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

func TestNativeHistorySuffixPreservesLiveAnswerOrder(t *testing.T) {
	for _, phase := range []string{"unknown", "commentary"} {
		for _, liveContinuation := range []bool{false, true} {
			t.Run(fmt.Sprintf("phase=%s/live_continuation=%t", phase, liveContinuation), func(t *testing.T) {
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
				for _, event := range []activity.RuntimeEvent{
					{Kind: activity.RuntimeEventTextDelta, MessageID: "A", Text: "Final ", Payload: map[string]any{"phase": "unknown"}},
					{Kind: activity.RuntimeEventTextDelta, MessageID: "B", Text: "Checking", Payload: map[string]any{"phase": phase}},
					{Kind: activity.RuntimeEventToolCallStart, ToolCallID: "tool", ToolKind: "exec_command", ToolStatus: "started"},
					{Kind: activity.RuntimeEventTextDelta, MessageID: "A", Text: "answer.", Payload: map[string]any{"phase": "unknown"}},
				} {
					if result := adapter.handleEvent(t.Context(), contract.TurnRequest{}, sink, event, &output, &files); result != nil {
						t.Fatal(result)
					}
				}
				history := quietTurnItems(map[string]any{"itemsView": "full", "items": []any{
					map[string]any{"type": "agentMessage", "id": "A", "phase": "unknown", "text": "Final answer."},
					map[string]any{"type": "agentMessage", "id": "B", "phase": "unknown", "text": "Checking..."},
					map[string]any{"type": "commandExecution", "id": "tool", "status": "completed", "command": "pwd", "aggregatedOutput": "/workspace"},
				}})
				for range 2 {
					if err := output.restore(t.Context(), sink, history); err != nil {
						t.Fatal(err)
					}
				}
				want := "Final answer."
				if liveContinuation {
					event := activity.RuntimeEvent{Kind: activity.RuntimeEventTextDelta, MessageID: "B", Text: " Done.", Payload: map[string]any{"phase": "unknown"}}
					if result := adapter.handleEvent(t.Context(), contract.TurnRequest{}, sink, event, &output, &files); result != nil {
						t.Fatal(result)
					}
					want = "Checking... Done."
				}
				if err := renderer.Complete(t.Context(), turn, contract.TurnResult{Status: contract.TurnSucceeded, Output: output.String(), RuntimeText: output.messageSnapshots()}); err != nil {
					t.Fatal(err)
				}
				if store.answer != want || len(store.progress.Items) != 2 || store.progress.Items[1].Tool == nil || store.progress.Items[1].Tool.Status != "completed" {
					t.Fatalf("answer=%q, want %q; progress=%+v", store.answer, want, store.progress.Items)
				}
			})
		}
	}
}

func TestNativeHistoryPositionsMissedToolsBeforeMatchingCommentary(t *testing.T) {
	for _, anchorPhase := range []string{"unknown", "final_answer"} {
		for _, commentBeforeTool := range []bool{false, true} {
			t.Run(fmt.Sprintf("phase=%s/comment_before_tool=%t", anchorPhase, commentBeforeTool), func(t *testing.T) {
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
				for _, event := range []activity.RuntimeEvent{
					{Kind: activity.RuntimeEventTextDelta, MessageID: "anchor", Text: "Answer.", Payload: map[string]any{"phase": anchorPhase}},
					{Kind: activity.RuntimeEventTextDelta, Text: "Checking.", Payload: map[string]any{"phase": "commentary"}},
				} {
					if result := adapter.handleEvent(t.Context(), contract.TurnRequest{}, sink, event, &output, &files); result != nil {
						t.Fatal(result)
					}
				}
				comment := map[string]any{"type": "agentMessage", "id": "recovered", "phase": "commentary", "text": "Checking."}
				tool := map[string]any{"type": "commandExecution", "id": "missed", "status": "completed", "command": "pwd", "aggregatedOutput": "/workspace"}
				anchor := map[string]any{"type": "agentMessage", "id": "anchor", "phase": anchorPhase, "text": "Answer."}
				items := []any{tool, anchor, comment}
				if commentBeforeTool {
					items = []any{comment, tool, anchor}
				}
				history := quietTurnItems(map[string]any{"itemsView": "full", "items": items})
				if err := output.restore(t.Context(), sink, history); err != nil {
					t.Fatal(err)
				}
				if err := renderer.Complete(t.Context(), turn, contract.TurnResult{Status: contract.TurnSucceeded, Output: output.String(), RuntimeText: output.messageSnapshots()}); err != nil {
					t.Fatal(err)
				}
				wantItems, toolIndex := 2, 0
				if commentBeforeTool {
					wantItems, toolIndex = 3, 1
				}
				if store.answer != "Answer." || len(store.progress.Items) != wantItems {
					t.Fatalf("answer=%q progress=%+v", store.answer, store.progress.Items)
				}
				for i, item := range store.progress.Items {
					if i == toolIndex {
						if item.ID != "missed" || item.Tool == nil || item.Tool.Status != "completed" {
							t.Fatalf("tool=%+v", item)
						}
					} else if item.Kind != "commentary" || item.Text != "Checking." {
						t.Fatalf("commentary=%+v", item)
					}
				}
			})
		}
	}
}

func TestNativeHistoryCanSupplyNewRoomAnswer(t *testing.T) {
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
	for _, event := range []activity.RuntimeEvent{
		{Kind: activity.RuntimeEventTextDelta, MessageID: "earlier", Text: "Working.", Payload: map[string]any{"phase": "unknown"}},
		{Kind: activity.RuntimeEventToolCallStart, ToolCallID: "tool", ToolKind: "exec_command", ToolStatus: "started"},
	} {
		if result := adapter.handleEvent(t.Context(), contract.TurnRequest{}, sink, event, &output, &files); result != nil {
			t.Fatal(result)
		}
	}
	history := quietTurnItems(map[string]any{"itemsView": "full", "items": []any{
		map[string]any{"type": "agentMessage", "id": "earlier", "phase": "unknown", "text": "Working."},
		map[string]any{"type": "commandExecution", "id": "tool", "status": "completed", "command": "pwd", "aggregatedOutput": "/workspace"},
		map[string]any{"type": "agentMessage", "id": "answer", "phase": "unknown", "text": "Answer."},
	}})
	if err := output.restore(t.Context(), sink, history); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Complete(t.Context(), turn, contract.TurnResult{Status: contract.TurnSucceeded, Output: output.String(), RuntimeText: output.messageSnapshots()}); err != nil {
		t.Fatal(err)
	}
	if store.answer != "Answer." || len(store.progress.Items) != 2 || store.progress.Items[0].Text != "Working." || store.progress.Items[1].Tool == nil || store.progress.Items[1].Tool.Status != "completed" {
		t.Fatalf("answer=%q progress=%+v", store.answer, store.progress.Items)
	}
}

func TestNativeHistoryPreservesRecoveredTextAndToolOrder(t *testing.T) {
	for _, phase := range []string{"", "unknown", "commentary"} {
		answerPhase := phase
		if phase == "commentary" {
			answerPhase = "final_answer"
		}
		for livePrefix := 0; livePrefix <= 5; livePrefix++ {
			t.Run(fmt.Sprintf("phase=%s/live_prefix=%d", phase, livePrefix), func(t *testing.T) {
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
				events := []activity.RuntimeEvent{
					{Kind: activity.RuntimeEventTextDelta, MessageID: "checking", Text: "Checking.", Payload: map[string]any{"phase": phase}},
					{Kind: activity.RuntimeEventToolCallStart, ToolCallID: "A", ToolKind: "exec_command", ToolStatus: "started"},
					{Kind: activity.RuntimeEventTextDelta, MessageID: "verified", Text: "Verified.", Payload: map[string]any{"phase": phase}},
					{Kind: activity.RuntimeEventToolCallStart, ToolCallID: "B", ToolKind: "exec_command", ToolStatus: "started"},
					{Kind: activity.RuntimeEventTextDelta, MessageID: "answer", Text: "Done.", Payload: map[string]any{"phase": answerPhase}},
				}
				for _, event := range events[:livePrefix] {
					if result := adapter.handleEvent(t.Context(), contract.TurnRequest{}, sink, event, &output, &files); result != nil {
						t.Fatal(result)
					}
				}
				history := quietTurnItems(map[string]any{"itemsView": "full", "items": []any{
					map[string]any{"type": "agentMessage", "id": "checking", "phase": phase, "text": "Checking."},
					map[string]any{"type": "commandExecution", "id": "A", "status": "completed", "command": "pwd"},
					map[string]any{"type": "agentMessage", "id": "verified", "phase": phase, "text": "Verified."},
					map[string]any{"type": "commandExecution", "id": "B", "status": "completed", "command": "pwd"},
					map[string]any{"type": "agentMessage", "id": "answer", "phase": answerPhase, "text": "Done."},
				}})
				for range 2 {
					if err := output.restore(t.Context(), sink, history); err != nil {
						t.Fatal(err)
					}
				}
				if err := renderer.Complete(t.Context(), turn, contract.TurnResult{Status: contract.TurnSucceeded, Output: output.String(), RuntimeText: output.messageSnapshots()}); err != nil {
					t.Fatal(err)
				}
				if store.answer != "Done." || len(store.progress.Items) != 4 {
					t.Fatalf("answer=%q progress=%+v", store.answer, store.progress.Items)
				}
				for i, item := range store.progress.Items {
					if i%2 == 0 {
						if item.Kind != "commentary" || item.Text != []string{"Checking.", "Verified."}[i/2] {
							t.Fatalf("progress[%d]=%+v", i, item)
						}
					} else if item.Tool == nil || item.ID != []string{"A", "B"}[i/2] || item.Tool.Status != "completed" {
						t.Fatalf("progress[%d]=%+v", i, item)
					}
				}
			})
		}
	}
}
