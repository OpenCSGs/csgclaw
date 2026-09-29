package delivery

import (
	"context"
	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/im"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func progressFixture(t *testing.T) (*TranscriptRenderer, *im.Service, channel.TurnContext) {
	t.Helper()
	service := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: "user", Users: []im.User{{ID: "user", Name: "User"}, {ID: "agent", Name: "Agent"}}, Rooms: []im.Room{{ID: "room", Title: "Room", Members: []string{"user", "agent"}}}})
	store, err := NewIMTranscriptStore(service, fixedParticipantResolver{item: apitypes.Participant{ID: "participant", ChannelUserRef: "agent"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return NewTranscriptRenderer(store), service, channel.TurnContext{AgentID: "agent", RoomID: "room", ParticipantID: "participant", TurnID: "turn", ConversationKey: "room", Locale: "en"}
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
