package delivery

import (
	"fmt"
	"reflect"
	"testing"

	"csgclaw/internal/agentengine"
)

func TestProgressSelectsLastResumedMessage(t *testing.T) {
	for _, phase := range []string{"", "unknown"} {
		for _, withNative := range []bool{false, true} {
			t.Run(fmt.Sprintf("phase=%s/native=%t", phase, withNative), func(t *testing.T) {
				renderer, service, turn := progressFixture(t)
				for i, event := range []agentengine.TurnEvent{
					{Kind: agentengine.TurnEventTextDelta, ItemID: "A", Phase: phase, Text: "Final "},
					{Kind: agentengine.TurnEventTextDelta, ItemID: "B", Phase: phase, Text: "Checking..."},
					{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "tool", Status: "started"}},
					{Kind: agentengine.TurnEventTextDelta, ItemID: "A", Phase: phase, Text: "answer."},
					{Kind: agentengine.TurnEventToolCallUpdate, Tool: &agentengine.ToolActivity{ID: "tool", Status: "completed"}},
				} {
					event.Sequence = uint64(i + 1)
					if err := renderer.Emit(t.Context(), turn, event); err != nil {
						t.Fatal(err)
					}
				}
				result := agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Final answer.Checking..."}
				if withNative {
					result.RuntimeText = []agentengine.TurnEvent{
						{Kind: agentengine.TurnEventTextDelta, ItemID: "A", Phase: phase, Text: "Final answer.", TextSnapshot: true},
						{Kind: agentengine.TurnEventTextDelta, ItemID: "B", Phase: phase, Text: "Checking...", TextSnapshot: true},
					}
				}
				if err := renderer.Complete(t.Context(), turn, result); err != nil {
					t.Fatal(err)
				}
				msg, progress := progressMessage(t, service)
				if msg.Content != "Final answer." {
					t.Fatalf("answer=%q, want %q; progress=%+v", msg.Content, "Final answer.", progress.Items)
				}
			})
		}
	}
}

func TestProgressPreservesTextOrderAcrossCorrections(t *testing.T) {
	for _, tc := range []struct {
		name   string
		update agentengine.TurnEvent
		want   string
	}{
		{"older phase", agentengine.TurnEvent{ItemID: "B", Phase: "unknown"}, "First. Final."},
		{"older snapshot", agentengine.TurnEvent{ItemID: "B", Phase: "unknown", Text: "Revised second.", TextSnapshot: true}, "First. Final."},
		{"confirmed commentary", agentengine.TurnEvent{ItemID: "A", Phase: "commentary"}, "Second."},
		{"recovered earlier message", agentengine.TurnEvent{ItemID: "C", Phase: "unknown", Text: "Recovered note.", TextSnapshot: true}, "First. Final."},
	} {
		for _, withNative := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/native=%t", tc.name, withNative), func(t *testing.T) {
				renderer, service, turn := progressFixture(t)
				events := []agentengine.TurnEvent{
					{Kind: agentengine.TurnEventTextDelta, ItemID: "A", Phase: "unknown", Text: "First. "},
					{Kind: agentengine.TurnEventTextDelta, ItemID: "B", Phase: "unknown", Text: "Second."},
					{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "tool", Status: "started"}},
					{Kind: agentengine.TurnEventTextDelta, ItemID: "A", Phase: "unknown", Text: "Final."},
				}
				update := tc.update
				update.Kind = agentengine.TurnEventTextDelta
				events = append(events, update)
				for i, event := range events {
					event.Sequence = uint64(i + 1)
					if err := renderer.Emit(t.Context(), turn, event); err != nil {
						t.Fatal(err)
					}
				}
				result := agentengine.TurnResult{Status: agentengine.TurnSucceeded}
				if withNative {
					result.RuntimeText = []agentengine.TurnEvent{
						{Kind: agentengine.TurnEventTextDelta, ItemID: "A", Phase: "unknown", Text: "First. Final.", TextSnapshot: true},
						{Kind: agentengine.TurnEventTextDelta, ItemID: "B", Phase: "unknown", Text: "Second.", TextSnapshot: true},
					}
					if update.ItemID == "C" {
						result.RuntimeText = append(result.RuntimeText, update)
					} else {
						for i := range result.RuntimeText {
							item := &result.RuntimeText[i]
							if item.ItemID == update.ItemID {
								item.Phase = update.Phase
								if update.TextSnapshot {
									item.Text = update.Text
								}
							}
						}
					}
				}
				if err := renderer.Complete(t.Context(), turn, result); err != nil {
					t.Fatal(err)
				}
				msg, progress := progressMessage(t, service)
				if msg.Content != tc.want {
					t.Fatalf("answer=%q, want %q; progress=%+v", msg.Content, tc.want, progress.Items)
				}
			})
		}
	}
}

func TestProgressMatchesRepeatedCommentaryInOrder(t *testing.T) {
	for mask := 0; mask < 4; mask++ {
		t.Run(fmt.Sprintf("covered=%02b", mask), func(t *testing.T) {
			renderer, service, turn := progressFixture(t)
			var events, native []agentengine.TurnEvent
			for i, id := range []string{"first", "second"} {
				events = append(events,
					agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Phase: "commentary", Text: "Checking files."},
					agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: id, Status: "started"}},
					agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallUpdate, Tool: &agentengine.ToolActivity{ID: id, Status: "completed"}},
				)
				if mask&(1<<i) != 0 {
					native = append(native, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, ItemID: "comment-" + id, Phase: "commentary", Text: "Checking files.", TextSnapshot: true})
				}
				native = append(native, agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: id}})
			}
			answer := agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", Text: "Answer."}
			events = append(events, answer)
			native = append(native, answer)
			for i, event := range events {
				event.Sequence = uint64(i + 1)
				if err := renderer.Emit(t.Context(), turn, event); err != nil {
					t.Fatal(err)
				}
			}
			if err := renderer.Complete(t.Context(), turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Answer.", RuntimeText: native}); err != nil {
				t.Fatal(err)
			}
			msg, progress := progressMessage(t, service)
			var got []string
			for _, item := range progress.Items {
				if item.Tool != nil {
					got = append(got, "tool:"+item.ID+":"+item.Tool.Status)
				} else {
					got = append(got, item.Kind+":"+item.Text)
				}
			}
			want := []string{"commentary:Checking files.", "tool:first:completed", "commentary:Checking files.", "tool:second:completed"}
			if msg.Content != "Answer." || !reflect.DeepEqual(got, want) {
				t.Fatalf("answer=%q progress=%v, want %v", msg.Content, got, want)
			}
		})
	}
}

func TestProgressKeepsIndependentCommentaryAcrossTool(t *testing.T) {
	for _, streamedBefore := range []bool{false, true} {
		t.Run(fmt.Sprintf("streamed_before=%t", streamedBefore), func(t *testing.T) {
			renderer, service, turn := progressFixture(t)
			comment := agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Phase: "commentary", Text: "Checking files."}
			tool := agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "tool", Status: "completed"}}
			events := []agentengine.TurnEvent{tool, comment}
			nativeComment := comment
			nativeComment.ItemID = "recovered"
			native := []agentengine.TurnEvent{nativeComment, tool}
			if streamedBefore {
				events = []agentengine.TurnEvent{comment, tool}
				native = []agentengine.TurnEvent{tool, nativeComment}
			}
			answer := agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", Text: "Answer."}
			events = append(events, answer)
			native = append(native, answer)
			for i, event := range events {
				event.Sequence = uint64(i + 1)
				if err := renderer.Emit(t.Context(), turn, event); err != nil {
					t.Fatal(err)
				}
			}
			if err := renderer.Complete(t.Context(), turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Answer.", RuntimeText: native}); err != nil {
				t.Fatal(err)
			}
			msg, progress := progressMessage(t, service)
			if msg.Content != "Answer." || len(progress.Items) != 3 || progress.Items[0].Text != "Checking files." || progress.Items[1].Tool == nil || progress.Items[1].Tool.Status != "completed" || progress.Items[2].Text != "Checking files." {
				t.Fatalf("answer=%q progress=%+v", msg.Content, progress.Items)
			}
		})
	}
}

func TestProgressReconcilesRecoveredCommentary(t *testing.T) {
	for _, covered := range []bool{false, true} {
		t.Run(fmt.Sprintf("history_covers_commentary=%t", covered), func(t *testing.T) {
			renderer, service, turn := progressFixture(t)
			for i, event := range []agentengine.TurnEvent{
				{Kind: agentengine.TurnEventTextDelta, Phase: "commentary", Text: "Checking files."},
				{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "tool", Status: "started"}},
				{Kind: agentengine.TurnEventToolCallUpdate, Tool: &agentengine.ToolActivity{ID: "tool", Status: "completed"}},
				{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", Text: "Answer."},
			} {
				event.Sequence = uint64(i + 1)
				if err := renderer.Emit(t.Context(), turn, event); err != nil {
					t.Fatal(err)
				}
			}
			native := []agentengine.TurnEvent{}
			if covered {
				native = append(native, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, ItemID: "comment", Phase: "commentary", Text: "Checking files.", TextSnapshot: true})
			}
			native = append(native, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", Text: "Answer.", TextSnapshot: true})
			if err := renderer.Complete(t.Context(), turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Answer.", RuntimeText: native}); err != nil {
				t.Fatal(err)
			}
			msg, progress := progressMessage(t, service)
			if msg.Content != "Answer." || len(progress.Items) != 2 || progress.Items[0].Kind != "commentary" || progress.Items[0].Text != "Checking files." || progress.Items[1].Kind != "tool" {
				t.Fatalf("answer=%q; progress=%+v", msg.Content, progress.Items)
			}
		})
	}
}
