package delivery

import (
	"context"
	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/im"
	"csgclaw/internal/modelusage"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLargeProgressPersistsWithoutBlockingOtherRooms(t *testing.T) {
	for _, kind := range []string{"single tool", "multiple tools", "reasoning"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			bootstrap := im.Bootstrap{
				CurrentUserID: "user",
				Users:         []im.User{{ID: "user", Name: "User"}, {ID: "agent", Name: "Agent"}},
				Rooms: []im.Room{
					{ID: "room", Title: "Room", Members: []string{"user", "agent"}},
					{ID: "other", Title: "Other room", Members: []string{"user", "agent"}},
				},
			}
			if err := im.SaveBootstrap(path, bootstrap); err != nil {
				t.Fatal(err)
			}
			service, err := im.NewServiceFromPath(path)
			if err != nil {
				t.Fatal(err)
			}
			store, err := NewIMTranscriptStore(service, fixedParticipantResolver{item: apitypes.Participant{ID: "participant", ChannelUserRef: "agent"}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			turn := channel.TurnContext{AgentID: "agent", RoomID: "room", ParticipantID: "participant", TurnID: "turn-csgclaw-large", ConversationKey: "room"}
			progress := activity.TurnProgress{ID: string(turn.TurnID), Status: "running", Items: []activity.ProgressItem{}}
			progress.Usage = &modelusage.Usage{Known: true, ModelID: "auto", Input: 1000, Output: 220, Total: 1220}
			if err := store.DeliverTurnProgress(context.Background(), turn, progress, "", true, nil); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "reasoning":
				progress.Items = append(progress.Items, activity.ProgressItem{ID: "thought", Kind: "reasoning", Text: strings.Repeat("R", 70*1024)})
			default:
				count, size := 1, 64*1024
				if kind == "multiple tools" {
					count, size = 8, 8*1024
				}
				for i := 0; i < count; i++ {
					tool := progressTool(agentengine.ToolActivity{Kind: "exec_command", Status: "completed", OutputSummary: strings.Repeat("X", size)}, nil)
					progress.Items = append(progress.Items, activity.ProgressItem{ID: fmt.Sprintf("tool-%d", i), Kind: "tool", Tool: tool})
				}
			}
			progress.Status = "succeeded"
			if err := store.DeliverTurnProgress(context.Background(), turn, progress, "Short answer", true, nil); err != nil {
				t.Fatalf("large progress delivery failed: %v", err)
			}
			if _, err := service.CreateMessage(im.CreateMessageRequest{RoomID: "other", SenderID: "user", Content: "你好"}); err != nil {
				t.Fatalf("unrelated room blocked: %v", err)
			}
			if _, err := service.CreateRoom(im.CreateRoomRequest{Type: apitypes.RoomTypeOnDemand, Title: "New on-demand room", CreatorID: "user", ManagerID: "agent"}); err != nil {
				t.Fatalf("room creation blocked: %v", err)
			}
			restarted, err := im.NewServiceFromPath(path)
			if err != nil {
				t.Fatal(err)
			}
			message, loaded := progressMessage(t, restarted)
			want, _ := json.Marshal(progress)
			got, _ := json.Marshal(loaded)
			if message.Content != "Short answer" || string(got) != string(want) {
				t.Fatal("restart lost answer or progress details")
			}
		})
	}
}

func progressFixture(t *testing.T) (*TranscriptRenderer, *im.Service, channel.TurnContext) {
	t.Helper()
	service := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: "user", Users: []im.User{{ID: "user", Name: "User"}, {ID: "agent", Name: "Agent"}}, Rooms: []im.Room{{ID: "room", Title: "Room", Members: []string{"user", "agent"}}}})
	store, err := NewIMTranscriptStore(service, fixedParticipantResolver{item: apitypes.Participant{ID: "participant", ChannelUserRef: "agent"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return NewTranscriptRenderer(store), service, channel.TurnContext{AgentID: "agent", RoomID: "room", ParticipantID: "participant", TurnID: "turn", ConversationKey: "room", Locale: "en"}
}

func TestProgressUsageReplacesSnapshotsAndRetainsPartialFailure(t *testing.T) {
	for _, status := range []agentengine.TurnStatus{agentengine.TurnSucceeded, agentengine.TurnCanceled} {
		t.Run(string(status), func(t *testing.T) {
			renderer, service, turn := progressFixture(t)
			ctx := context.Background()
			for _, input := range []int64{100, 300, 300} {
				u := modelusage.Usage{Known: true, ModelID: "selected-model", Input: input, Output: 20, Total: input + 20}
				// The HTTP client decodes arbitrary activity payloads into maps.
				encoded, _ := json.Marshal(u)
				var payload map[string]any
				_ = json.Unmarshal(encoded, &payload)
				if err := renderer.Emit(ctx, turn, agentengine.TurnEvent{Kind: agentengine.TurnEventActivityUpdate, Activity: &agentengine.ActivityUpdate{Kind: modelusage.Kind, Payload: payload}}); err != nil {
					t.Fatal(err)
				}
			}
			if err := renderer.Complete(ctx, turn, agentengine.TurnResult{Status: status, Output: "done"}); err != nil {
				t.Fatal(err)
			}
			_, p := progressMessage(t, service)
			if p.Usage == nil || p.Usage.Total != 320 || p.Usage.ModelID != "selected-model" || p.Usage.Partial != (status != agentengine.TurnSucceeded) {
				t.Fatalf("usage=%+v", p.Usage)
			}
		})
	}
}
func progressMessage(t *testing.T, service *im.Service) (im.Message, activity.TurnProgress) {
	t.Helper()
	room, ok := service.Room("room")
	if !ok || len(room.Messages) != 1 {
		t.Fatalf("room=%+v", room)
	}
	msg := room.Messages[0]
	meta := msg.Metadata["csgclaw"].(map[string]any)
	data, _ := json.Marshal(meta["turn_progress"])
	var p activity.TurnProgress
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	return msg, p
}
func TestProgressStreamsThroughIMAndCommitsOneFinal(t *testing.T) {
	renderer, service, turn := progressFixture(t)
	ctx := context.Background()
	for _, event := range []agentengine.TurnEvent{
		{Sequence: 1, Kind: agentengine.TurnEventTextDelta, ItemID: "intro", Phase: "commentary", Text: "Checking files."},
		{Sequence: 2, Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "read", Kind: "exec_command", Status: "started", Payload: map[string]any{"command": "cat README.md", "commandActions": []any{map[string]any{"type": "read"}}}}},
		{Sequence: 3, Kind: agentengine.TurnEventToolCallUpdate, Tool: &agentengine.ToolActivity{ID: "read", Kind: "exec_command", Status: "completed", Payload: map[string]any{"output": "contents", "exitCode": float64(0)}}},
		{Sequence: 4, Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", Text: "The answer"},
	} {
		if err := renderer.Emit(ctx, turn, event); err != nil {
			t.Fatal(err)
		}
	}
	msg, p := progressMessage(t, service)
	if msg.Content != "The answer" || !im.IsAgentActivityMessage(msg) || p.Status != "running" || len(p.Items) != 2 || p.Items[1].Tool.Actions[0] != "read" {
		t.Fatalf("stream=%+v progress=%+v", msg, p)
	}
	// Queue a trailing chunk, then finish before the timer. Completion must win.
	if err := renderer.Emit(ctx, turn, agentengine.TurnEvent{Sequence: 5, Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", Text: " is ready."}); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Complete(ctx, turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(70 * time.Millisecond)
	msg, p = progressMessage(t, service)
	if msg.Content != "The answer is ready." || im.IsAgentActivityMessage(msg) || p.Status != "succeeded" || p.EndedAt == "" {
		t.Fatalf("final=%+v progress=%+v", msg, p)
	}
	if err := renderer.Emit(ctx, turn, agentengine.TurnEvent{Sequence: 6, Kind: agentengine.TurnEventTextDelta, Text: "late"}); err != nil {
		t.Fatal(err)
	}
	again, _ := progressMessage(t, service)
	if again.Content != msg.Content {
		t.Fatal("late update overwrote final")
	}
}
func TestProgressUnphasedSegmentationAndCancellation(t *testing.T) {
	for _, status := range []agentengine.TurnStatus{agentengine.TurnSucceeded, agentengine.TurnCanceled, agentengine.TurnFailed} {
		t.Run(string(status), func(t *testing.T) {
			renderer, service, turn := progressFixture(t)
			ctx := context.Background()
			events := []agentengine.TurnEvent{{Sequence: 1, Kind: agentengine.TurnEventTextDelta, Text: "Working."}, {Sequence: 2, Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "x", Kind: "custom_tool", Status: "started"}}, {Sequence: 3, Kind: agentengine.TurnEventTextDelta, Text: "Final result."}}
			for _, event := range events {
				if err := renderer.Emit(ctx, turn, event); err != nil {
					t.Fatal(err)
				}
			}
			result := agentengine.TurnResult{Status: status}
			if status == agentengine.TurnFailed {
				result.Error = &agentengine.TurnError{Message: "Test failure"}
			}
			if err := renderer.Complete(ctx, turn, result); err != nil {
				t.Fatal(err)
			}
			msg, p := progressMessage(t, service)
			if msg.Content != "Final result." || len(p.Items) != 2 || p.Items[0].Text != "Working." || p.Items[1].Tool.Actions[0] != "tool" || p.Status != string(status) {
				t.Fatalf("message=%+v p=%+v", msg, p)
			}
			if status != agentengine.TurnSucceeded && !im.IsAgentActivityMessage(msg) {
				t.Fatal("incomplete answer wakes agents")
			}
		})
	}
}
func TestCanceledProgressDoesNotPresentRuntimeFailure(t *testing.T) {
	renderer, service, turn := progressFixture(t)
	ctx := context.Background()
	if err := renderer.Emit(ctx, turn, agentengine.TurnEvent{Sequence: 1, Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "cancel-tool", Kind: "custom_tool", Status: "started"}}); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Complete(ctx, turn, agentengine.TurnResult{
		Status: agentengine.TurnCanceled,
		Error:  &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: context.Canceled.Error()},
	}); err != nil {
		t.Fatal(err)
	}
	msg, progress := progressMessage(t, service)
	metadata := msg.Metadata["csgclaw"].(map[string]any)
	if progress.Status != "canceled" || progress.Error != "" || progress.ErrorCode != "" || metadata["runtime_error"] != nil || metadata["error_detail"] != nil {
		t.Fatalf("cancellation presented as failure: progress=%+v metadata=%+v", progress, metadata)
	}
	if len(progress.Items) != 1 || progress.Items[0].Tool.Status != "interrupted" || !im.IsAgentActivityMessage(msg) {
		t.Fatalf("cancellation lost interrupted activity: message=%+v progress=%+v", msg, progress)
	}
}

func TestProgressCoalescesReasoningAndFlushesTail(t *testing.T) {
	renderer, service, turn := progressFixture(t)
	ctx := context.Background()
	for i := 1; i <= 10; i++ {
		if err := renderer.Emit(ctx, turn, agentengine.TurnEvent{Sequence: uint64(i), Kind: agentengine.TurnEventThoughtDelta, Thought: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		_, p := progressMessage(t, service)
		if len(p.Items) == 1 && p.Items[0].Text == strings.Repeat("x", 10) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("tail not flushed: %+v", p)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := renderer.Complete(ctx, turn, agentengine.TurnResult{Status: agentengine.TurnCanceled}); err != nil {
		t.Fatal(err)
	}
}

func TestProgressKeepsAllToolsWithoutPublishingEveryUpdate(t *testing.T) {
	renderer, service, turn := progressFixture(t)
	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("tool-%d", i)
		for j, status := range []string{"started", "completed"} {
			kind := agentengine.TurnEventToolCallStart
			if j == 1 {
				kind = agentengine.TurnEventToolCallUpdate
			}
			if err := renderer.Emit(ctx, turn, agentengine.TurnEvent{Sequence: uint64(i*2 + j + 1), Kind: kind, Tool: &agentengine.ToolActivity{ID: id, Kind: "exec_command", Status: status}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := renderer.Complete(ctx, turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Done"}); err != nil {
		t.Fatal(err)
	}
	_, p := progressMessage(t, service)
	if len(p.Items) != 1000 || p.Revision >= 100 {
		t.Fatalf("items=%d revisions=%d", len(p.Items), p.Revision)
	}
}

func TestProgressKeepsLastUnphasedTextWhenTurnEndsWithTool(t *testing.T) {
	renderer, service, turn := progressFixture(t)
	ctx := context.Background()
	_ = renderer.Emit(ctx, turn, agentengine.TurnEvent{Sequence: 1, Kind: agentengine.TurnEventTextDelta, Text: "The useful result."})
	_ = renderer.Emit(ctx, turn, agentengine.TurnEvent{Sequence: 2, Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "x", Kind: "exec_command", Status: "started"}})
	if err := renderer.Complete(ctx, turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded}); err != nil {
		t.Fatal(err)
	}
	msg, p := progressMessage(t, service)
	if msg.Content != "The useful result." || len(p.Items) != 1 {
		t.Fatalf("msg=%+v p=%+v", msg, p)
	}
}

func TestProgressAppliesAuthoritativeMessageSnapshot(t *testing.T) {
	renderer, service, turn := progressFixture(t)
	ctx := context.Background()
	_ = renderer.Emit(ctx, turn, agentengine.TurnEvent{Sequence: 1, Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", Text: "Draft"})
	_ = renderer.Emit(ctx, turn, agentengine.TurnEvent{Sequence: 2, Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", Text: "Corrected", TextSnapshot: true})
	if err := renderer.Complete(ctx, turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Corrected"}); err != nil {
		t.Fatal(err)
	}
	message, _ := progressMessage(t, service)
	if message.Content != "Corrected" {
		t.Fatal(message.Content)
	}
}

func TestProgressUsesCompletedTextWhenItExtendsAnUnphasedAnswer(t *testing.T) {
	for _, withTool := range []bool{false, true} {
		renderer, service, turn := progressFixture(t)
		ctx := context.Background()
		if err := renderer.Emit(ctx, turn, agentengine.TurnEvent{Sequence: 1, Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Text: "Part"}); err != nil {
			t.Fatal(err)
		}
		if withTool {
			if err := renderer.Emit(ctx, turn, agentengine.TurnEvent{Sequence: 2, Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "tool", Status: "started"}}); err != nil {
				t.Fatal(err)
			}
		}
		const completed = "Part of the final answer."
		if err := renderer.Complete(ctx, turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: completed}); err != nil {
			t.Fatal(err)
		}
		msg, progress := progressMessage(t, service)
		if msg.Content != completed || progress.Status != "succeeded" {
			t.Fatalf("withTool=%t: message=%q status=%s", withTool, msg.Content, progress.Status)
		}
	}
}

func TestProgressOnlyTurnDoesNotWakeAnotherAgent(t *testing.T) {
	renderer, service, turn := progressFixture(t)
	ctx := context.Background()
	if err := renderer.Start(ctx, turn); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Complete(ctx, turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded}); err != nil {
		t.Fatal(err)
	}
	message, _ := progressMessage(t, service)
	if !im.IsAgentActivityMessage(message) {
		t.Fatal("empty progress completion became conversational input")
	}
}

func TestProgressFailureRetainsDiagnosticMetadata(t *testing.T) {
	renderer, service, turn := progressFixture(t)
	ctx := context.Background()
	if err := renderer.Start(ctx, turn); err != nil {
		t.Fatal(err)
	}
	if err := renderer.Complete(ctx, turn, agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Message: "specific runtime failure"}}); err != nil {
		t.Fatal(err)
	}
	message, _ := progressMessage(t, service)
	metadata := message.Metadata["csgclaw"].(map[string]any)
	if metadata["error_detail"] != "specific runtime failure" || metadata["runtime_error"] != true {
		t.Fatalf("missing diagnostics: %+v", metadata)
	}
}

func TestProgressPreservesStructuredContextFailures(t *testing.T) {
	for _, tc := range []struct {
		code   agentengine.ErrorCode
		zh, en string
	}{
		{"context_length_exceeded", "本次内容超过模型容量，请拆分输入、检查模型容量设置或选择容量更大的模型。已有对话已保留。", "This request exceeds the model context capacity."},
		{"context_compaction_failed", "暂时无法整理对话，已有记录已保留。请重试或选择容量更大的模型。", "The conversation could not be compacted."},
	} {
		for _, locale := range []string{"zh-CN", "en"} {
			for _, raw := range []string{"unexpected status 400: provider diagnostic", ""} {
				t.Run(string(tc.code)+"/"+locale+"/"+raw, func(t *testing.T) {
					renderer, service, turn := progressFixture(t)
					turn.Locale = locale
					ctx := context.Background()
					if err := renderer.Emit(ctx, turn, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Phase: "commentary", Text: "Checking context."}); err != nil {
						t.Fatal(err)
					}
					if err := renderer.Complete(ctx, turn, agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: tc.code, Message: raw}}); err != nil {
						t.Fatal(err)
					}
					msg, progress := progressMessage(t, service)
					want := tc.en
					if locale == "zh-CN" {
						want = tc.zh
					}
					meta := msg.Metadata["csgclaw"].(map[string]any)
					if progress.Status != "failed" || !strings.HasPrefix(progress.Error, want) || meta["error_code"] != string(tc.code) {
						t.Fatalf("lost structured failure: progress=%+v metadata=%+v", progress, meta)
					}
					if raw != "" && (meta["error_detail"] != raw || strings.Contains(progress.Error, raw) || strings.Contains(msg.Content, raw)) {
						t.Fatal("raw diagnostics must remain in activity metadata only")
					}
				})
			}
		}
	}
}

func TestProgressContinuesSameMessageAcrossTool(t *testing.T) {
	for _, tc := range []struct {
		name, phase, text, want string
		snapshot                bool
	}{
		{"unknown suffix", "unknown", " answer", "Partial answer", false},
		{"final suffix", "final_answer", " answer", "Partial answer", false},
		{"unknown snapshot", "unknown", "Revised answer", "Revised answer", true},
		{"final snapshot", "final_answer", "Revised answer", "Revised answer", true},
		{"phase only", "final_answer", "", "Partial", false},
		{"confirmed commentary", "commentary", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			renderer, service, turn := progressFixture(t)
			for i, event := range []agentengine.TurnEvent{
				{Kind: agentengine.TurnEventTextDelta, ItemID: "message", Phase: "unknown", Text: "Partial"},
				{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "tool", Status: "started"}},
				{Kind: agentengine.TurnEventTextDelta, ItemID: "message", Phase: tc.phase, Text: tc.text, TextSnapshot: tc.snapshot},
				{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "second", Status: "started"}},
				{Kind: agentengine.TurnEventToolCallUpdate, Tool: &agentengine.ToolActivity{ID: "tool", Status: "completed"}},
				{Kind: agentengine.TurnEventToolCallUpdate, Tool: &agentengine.ToolActivity{ID: "second", Status: "failed"}},
			} {
				event.Sequence = uint64(i + 1)
				if err := renderer.Emit(t.Context(), turn, event); err != nil {
					t.Fatal(err)
				}
			}
			if err := renderer.Complete(t.Context(), turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded}); err != nil {
				t.Fatal(err)
			}
			msg, progress := progressMessage(t, service)
			if tc.want == "" {
				if !im.IsAgentActivityMessage(msg) {
					t.Fatalf("commentary was promoted to the reply: %q", msg.Content)
				}
			} else if msg.Content != tc.want {
				t.Fatalf("answer = %q, want %q", msg.Content, tc.want)
			}
			items := progress.Items
			if tc.phase == "commentary" {
				if len(items) != 3 || items[0].Kind != "commentary" || items[0].Text != "Partial" {
					t.Fatalf("commentary = %+v", items)
				}
				items = items[1:]
			}
			if len(items) != 2 || items[0].ID != "tool" || items[0].Tool == nil || items[0].Tool.Status != "completed" || items[1].ID != "second" || items[1].Tool == nil || items[1].Tool.Status != "failed" {
				t.Fatalf("progress items = %+v", progress.Items)
			}
		})
	}
}

func TestProgressKeepsDistinctNativeMessagesSeparate(t *testing.T) {
	for _, withTool := range []bool{false, true} {
		for _, phase := range []string{"unknown", "final_answer"} {
			t.Run(fmt.Sprintf("tool=%t/phase=%s", withTool, phase), func(t *testing.T) {
				renderer, service, turn := progressFixture(t)
				const earlier = "The answer is 42. Verifying it."
				const answer = "The answer is 42."
				events := []agentengine.TurnEvent{{Kind: agentengine.TurnEventTextDelta, ItemID: "earlier", Phase: "unknown", Text: earlier}}
				if withTool {
					events = append(events, agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "verify", Status: "started"}})
				}
				events = append(events, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: phase, Text: answer})
				for i, event := range events {
					event.Sequence = uint64(i + 1)
					if err := renderer.Emit(t.Context(), turn, event); err != nil {
						t.Fatal(err)
					}
				}
				if err := renderer.Complete(t.Context(), turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: earlier + answer}); err != nil {
					t.Fatal(err)
				}
				msg, progress := progressMessage(t, service)
				if msg.Content != answer || len(progress.Items) == 0 || progress.Items[0].Text != earlier {
					t.Fatalf("answer=%q progress=%+v", msg.Content, progress.Items)
				}
			})
		}
	}
}

func TestProgressPublishesFinalMessagesOnce(t *testing.T) {
	for _, fallback := range []string{"", "First answer. Second answer."} {
		for _, secondTool := range []bool{false, true} {
			t.Run(fmt.Sprintf("fallback=%t/secondTool=%t", fallback != "", secondTool), func(t *testing.T) {
				renderer, service, turn := progressFixture(t)
				events := []agentengine.TurnEvent{
					{Kind: agentengine.TurnEventTextDelta, ItemID: "first", Phase: "unknown", Text: "First answer."},
					{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "first-tool", Status: "started"}},
					{Kind: agentengine.TurnEventTextDelta, ItemID: "first", Phase: "final_answer"},
					{Kind: agentengine.TurnEventTextDelta, ItemID: "second", Phase: "unknown", Text: " Second draft."},
				}
				if secondTool {
					events = append(events, agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "second-tool", Status: "started"}})
				}
				events = append(events,
					agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, ItemID: "second", Phase: "final_answer", Text: " Second answer.", TextSnapshot: true},
					agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallUpdate, Tool: &agentengine.ToolActivity{ID: "first-tool", Status: "completed"}},
				)
				for i, event := range events {
					event.Sequence = uint64(i + 1)
					if err := renderer.Emit(t.Context(), turn, event); err != nil {
						t.Fatal(err)
					}
				}
				if err := renderer.Complete(t.Context(), turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: fallback}); err != nil {
					t.Fatal(err)
				}
				msg, progress := progressMessage(t, service)
				wantItems := 1
				if secondTool {
					wantItems++
				}
				if msg.Content != "First answer. Second answer." || len(progress.Items) != wantItems {
					t.Fatalf("answer=%q progress=%+v", msg.Content, progress.Items)
				}
				for _, item := range progress.Items {
					if item.Kind != "tool" || item.Tool == nil || (item.ID == "first-tool" && item.Tool.Status != "completed") || (item.ID == "second-tool" && item.Tool.Status != "interrupted") {
						t.Fatalf("progress item=%+v", item)
					}
				}
			})
		}
	}
}

func TestProgressAppliesSnapshotsToEachFinalMessage(t *testing.T) {
	for _, withTools := range []bool{false, true} {
		t.Run(fmt.Sprintf("tools=%t", withTools), func(t *testing.T) {
			renderer, service, turn := progressFixture(t)
			var events []agentengine.TurnEvent
			for _, id := range []string{"first", "second"} {
				events = append(events, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, ItemID: id, Phase: "final_answer", Text: "Draft."})
				if withTools {
					events = append(events, agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: id, Status: "started"}})
				}
			}
			events = append(events,
				agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, ItemID: "first", Text: "First. ", TextSnapshot: true},
				agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, ItemID: "second", Text: "Second.", TextSnapshot: true},
			)
			for i, event := range events {
				event.Sequence = uint64(i + 1)
				if err := renderer.Emit(t.Context(), turn, event); err != nil {
					t.Fatal(err)
				}
			}
			if err := renderer.Complete(t.Context(), turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded}); err != nil {
				t.Fatal(err)
			}
			msg, progress := progressMessage(t, service)
			if msg.Content != "First. Second." {
				t.Fatalf("answer=%q", msg.Content)
			}
			for _, item := range progress.Items {
				if item.Kind != "tool" || item.Tool == nil || item.Tool.Status != "interrupted" {
					t.Fatalf("progress item=%+v", item)
				}
			}
		})
	}
}

func TestProgressRetainsConfirmedCommentaryUpdates(t *testing.T) {
	renderer, service, turn := progressFixture(t)
	for i, event := range []agentengine.TurnEvent{
		{Kind: agentengine.TurnEventTextDelta, ItemID: "comment", Phase: "unknown", Text: "Checking."},
		{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "check", Status: "started"}},
		{Kind: agentengine.TurnEventTextDelta, ItemID: "comment", Phase: "commentary", Text: "Checked.", TextSnapshot: true},
		{Kind: agentengine.TurnEventTextDelta, ItemID: "comment", Text: " Done."},
		{Kind: agentengine.TurnEventToolCallUpdate, Tool: &agentengine.ToolActivity{ID: "check", Status: "completed"}},
		{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", Text: "Answer."},
	} {
		event.Sequence = uint64(i + 1)
		if err := renderer.Emit(t.Context(), turn, event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Complete(t.Context(), turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Answer."}); err != nil {
		t.Fatal(err)
	}
	msg, progress := progressMessage(t, service)
	if msg.Content != "Answer." || len(progress.Items) != 2 || progress.Items[0].Text != "Checked. Done." || progress.Items[1].Tool == nil || progress.Items[1].Tool.Status != "completed" {
		t.Fatalf("answer=%q progress=%+v", msg.Content, progress.Items)
	}
}

func TestProgressDoesNotRestoreClearedTextFromAnotherMessage(t *testing.T) {
	renderer, service, turn := progressFixture(t)
	for i, event := range []agentengine.TurnEvent{
		{Kind: agentengine.TurnEventTextDelta, ItemID: "earlier", Phase: "unknown", Text: "Checking."},
		{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "check", Status: "started"}},
		{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "unknown", Text: "Draft."},
		{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", TextSnapshot: true},
	} {
		event.Sequence = uint64(i + 1)
		if err := renderer.Emit(t.Context(), turn, event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Complete(t.Context(), turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Checking."}); err != nil {
		t.Fatal(err)
	}
	msg, progress := progressMessage(t, service)
	if !im.IsAgentActivityMessage(msg) || len(progress.Items) != 2 || progress.Items[0].Text != "Checking." {
		t.Fatalf("message=%+v progress=%+v", msg, progress.Items)
	}
}

func TestProgressUsesRecoveredNativeMessages(t *testing.T) {
	for _, withTool := range []bool{false, true} {
		t.Run(fmt.Sprintf("tool=%t", withTool), func(t *testing.T) {
			renderer, service, turn := progressFixture(t)
			events := []agentengine.TurnEvent{{Kind: agentengine.TurnEventTextDelta, Text: "First part"}}
			if withTool {
				events = append(events, agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "tool", Status: "started"}})
			}
			events = append(events, agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", Text: " and final part."})
			for i, event := range events {
				event.Sequence = uint64(i + 1)
				if err := renderer.Emit(t.Context(), turn, event); err != nil {
					t.Fatal(err)
				}
			}
			const answer = "First part and final part."
			if err := renderer.Complete(t.Context(), turn, agentengine.TurnResult{
				Status: agentengine.TurnSucceeded, Output: answer,
				RuntimeText: []agentengine.TurnEvent{{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", Text: answer, TextSnapshot: true}},
			}); err != nil {
				t.Fatal(err)
			}
			msg, progress := progressMessage(t, service)
			if msg.Content != answer {
				t.Fatalf("answer=%q progress=%+v", msg.Content, progress.Items)
			}
			for _, item := range progress.Items {
				if item.Kind != "tool" || item.Tool == nil || item.Tool.Status != "interrupted" {
					t.Fatalf("progress item=%+v", item)
				}
			}
		})
	}
}

func TestProgressUsesCompletedNativePhases(t *testing.T) {
	renderer, service, turn := progressFixture(t)
	for i, event := range []agentengine.TurnEvent{
		{Kind: agentengine.TurnEventTextDelta, ItemID: "comment", Phase: "unknown", Text: "Checking."},
		{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "tool", Status: "started"}},
		{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "unknown", Text: "Draft."},
	} {
		event.Sequence = uint64(i + 1)
		if err := renderer.Emit(t.Context(), turn, event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Complete(t.Context(), turn, agentengine.TurnResult{
		Status: agentengine.TurnSucceeded, Output: "Answer.",
		RuntimeText: []agentengine.TurnEvent{
			{Kind: agentengine.TurnEventTextDelta, ItemID: "comment", Phase: "commentary", Text: "Checked.", TextSnapshot: true},
			{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", Text: "Answer.", TextSnapshot: true},
		},
	}); err != nil {
		t.Fatal(err)
	}
	msg, progress := progressMessage(t, service)
	if msg.Content != "Answer." || len(progress.Items) != 2 || progress.Items[0].Text != "Checked." || progress.Items[1].Tool == nil || progress.Items[1].Tool.Status != "interrupted" {
		t.Fatalf("answer=%q progress=%+v", msg.Content, progress.Items)
	}
}

func TestProgressContinuesUnidentifiedMessageWhenIDArrives(t *testing.T) {
	renderer, service, turn := progressFixture(t)
	for i, event := range []agentengine.TurnEvent{
		{Kind: agentengine.TurnEventTextDelta, Text: "First "},
		{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", Text: "answer."},
	} {
		event.Sequence = uint64(i + 1)
		if err := renderer.Emit(t.Context(), turn, event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Complete(t.Context(), turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded}); err != nil {
		t.Fatal(err)
	}
	msg, progress := progressMessage(t, service)
	if msg.Content != "First answer." || len(progress.Items) != 0 {
		t.Fatalf("answer=%q progress=%+v", msg.Content, progress.Items)
	}
}

func TestProgressRetainsUnidentifiedCommentaryAtCompletion(t *testing.T) {
	renderer, service, turn := progressFixture(t)
	for i, event := range []agentengine.TurnEvent{
		{Kind: agentengine.TurnEventTextDelta, Phase: "commentary", Text: "Checking."},
		{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", Text: "Answer."},
	} {
		event.Sequence = uint64(i + 1)
		if err := renderer.Emit(t.Context(), turn, event); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.Complete(t.Context(), turn, agentengine.TurnResult{
		Status: agentengine.TurnSucceeded, Output: "Answer.",
		RuntimeText: []agentengine.TurnEvent{{Kind: agentengine.TurnEventTextDelta, ItemID: "answer", Phase: "final_answer", Text: "Answer.", TextSnapshot: true}},
	}); err != nil {
		t.Fatal(err)
	}
	msg, progress := progressMessage(t, service)
	if msg.Content != "Answer." || len(progress.Items) != 1 || progress.Items[0].Text != "Checking." {
		t.Fatalf("answer=%q progress=%+v", msg.Content, progress.Items)
	}
}
