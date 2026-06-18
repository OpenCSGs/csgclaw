package runtimebridge

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/activity"
)

func TestTurnRendererMergesToolUpdateDeltas(t *testing.T) {
	t.Parallel()

	renderer := NewTurnRenderer()
	now := time.Now().UTC()
	start := activity.RuntimeEvent{
		RuntimeID:        "rt-1",
		SessionID:        "sess-1",
		Kind:             activity.RuntimeEventToolCallStart,
		ReceivedAt:       now,
		ToolCallID:       "tool-1",
		ToolKind:         "execute",
		ToolTitle:        "Run shell command",
		ToolStatus:       "in_progress",
		ToolInputSummary: `{"cmd":"go test ./..."}`,
	}
	startRendered, ok := renderer.RenderActivity(start, "csgclaw", "room-1", "u-runtime")
	if !ok {
		t.Fatal("tool start was not rendered")
	}

	update := activity.RuntimeEvent{
		RuntimeID:         "rt-1",
		SessionID:         "sess-1",
		Kind:              activity.RuntimeEventToolCallUpdate,
		ReceivedAt:        now.Add(time.Second),
		ToolCallID:        "tool-1",
		ToolOutputSummary: `{"output":"ok"}`,
	}
	rendered, ok := renderer.RenderActivity(update, "csgclaw", "room-1", "u-runtime")
	if !ok {
		t.Fatal("tool output delta was not rendered")
	}
	if rendered.MessageID != startRendered.MessageID {
		t.Fatalf("tool activity message id changed from %q to %q", startRendered.MessageID, rendered.MessageID)
	}
	if strings.Contains(rendered.Text, "rt-1") || strings.Contains(rendered.Text, "sess-1") || strings.Contains(rendered.Text, "tool-1") {
		t.Fatalf("rendered tool activity leaked execution identity: %s", rendered.Text)
	}

	var payload struct {
		Type    string `json:"type"`
		Version int    `json:"version"`
		EventID string `json:"event_id"`
		Content struct {
			Tool struct {
				ID            string `json:"id"`
				Title         string `json:"title"`
				Status        string `json:"status"`
				InputSummary  string `json:"input_summary"`
				OutputSummary string `json:"output_summary"`
			} `json:"tool"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(rendered.Text), &payload); err != nil {
		t.Fatalf("decode rendered activity: %v", err)
	}
	if payload.Type != AgentActivityType {
		t.Fatalf("type = %q, want %q", payload.Type, AgentActivityType)
	}
	if payload.Version != AgentActivityVersion {
		t.Fatalf("version = %d, want %d", payload.Version, AgentActivityVersion)
	}
	if payload.EventID != rendered.MessageID {
		t.Fatalf("event_id = %q, want message id %q", payload.EventID, rendered.MessageID)
	}
	if payload.Content.Tool.ID == "" || payload.Content.Tool.ID == "tool-1" {
		t.Fatalf("tool id = %q, want non-empty opaque id", payload.Content.Tool.ID)
	}
	if payload.Content.Tool.Title != "Run shell command" {
		t.Fatalf("title = %q, want prior title", payload.Content.Tool.Title)
	}
	if payload.Content.Tool.Status != "running" {
		t.Fatalf("status = %q, want running", payload.Content.Tool.Status)
	}
	if payload.Content.Tool.InputSummary == "" || payload.Content.Tool.OutputSummary == "" {
		t.Fatalf("tool summaries = input %q output %q, want both retained", payload.Content.Tool.InputSummary, payload.Content.Tool.OutputSummary)
	}

	completed := activity.RuntimeEvent{
		RuntimeID:  "rt-1",
		SessionID:  "sess-1",
		Kind:       activity.RuntimeEventToolCallUpdate,
		ReceivedAt: now.Add(2 * time.Second),
		ToolCallID: "tool-1",
		ToolStatus: "completed",
	}
	if _, ok := renderer.RenderActivity(completed, "csgclaw", "room-1", "u-runtime"); !ok {
		t.Fatal("tool completed delta was not rendered")
	}

	laterOutput := activity.RuntimeEvent{
		RuntimeID:         "rt-1",
		SessionID:         "sess-1",
		Kind:              activity.RuntimeEventToolCallUpdate,
		ReceivedAt:        now.Add(3 * time.Second),
		ToolCallID:        "tool-1",
		ToolOutputSummary: `{"output":"done"}`,
	}
	rendered, ok = renderer.RenderActivity(laterOutput, "csgclaw", "room-1", "u-runtime")
	if !ok {
		t.Fatal("post-completion output delta was not rendered")
	}
	if err := json.Unmarshal([]byte(rendered.Text), &payload); err != nil {
		t.Fatalf("decode post-completion activity: %v", err)
	}
	if payload.Content.Tool.Status != "completed" {
		t.Fatalf("post-completion status = %q, want completed", payload.Content.Tool.Status)
	}
}

func TestTurnRendererUsesCurrentTimestampForZeroReceivedAt(t *testing.T) {
	t.Parallel()

	renderer := NewTurnRenderer()
	rendered, ok := renderer.RenderActivity(activity.RuntimeEvent{
		RuntimeID:  "rt-1",
		SessionID:  "sess-1",
		Kind:       activity.RuntimeEventToolCallStart,
		ToolCallID: "tool-1",
	}, "csgclaw", "room-1", "u-runtime")
	if !ok {
		t.Fatal("tool activity was not rendered")
	}

	var payload struct {
		OriginServerTS int64 `json:"origin_server_ts"`
	}
	if err := json.Unmarshal([]byte(rendered.Text), &payload); err != nil {
		t.Fatalf("decode rendered activity: %v", err)
	}
	if payload.OriginServerTS <= 0 {
		t.Fatalf("origin_server_ts = %d, want current positive timestamp", payload.OriginServerTS)
	}
}

func TestTurnRendererRendersGenericActionActivity(t *testing.T) {
	t.Parallel()

	renderer := NewTurnRenderer()
	now := time.Now().UTC()
	rendered, ok := renderer.RenderActivity(activity.RuntimeEvent{
		RuntimeKind:  "codex",
		RuntimeID:    "rt-1",
		SessionID:    "sess-1",
		Kind:         activity.RuntimeEventActionRequest,
		ReceivedAt:   now,
		ToolCallID:   "tool-1",
		ToolTitle:    "Run shell command",
		ActionID:     "act-1",
		ActionStatus: string(activity.ActionStatusPending),
		Payload: activity.ActivitySnapshot{
			ID:          "act-1",
			Kind:        activity.ActionKindPermission,
			Title:       "Run shell command",
			Status:      activity.ActionStatusPending,
			RequestedAt: now,
			ExpiresAt:   now.Add(time.Minute),
			Options: []activity.ActionOptionSnapshot{
				{ID: "once", Kind: "allow_once", Label: "Allow once"},
			},
		},
	}, "csgclaw", "room-1", "u-runtime")
	if !ok {
		t.Fatal("action activity was not rendered")
	}
	if strings.Contains(rendered.Text, "runtime_id") || strings.Contains(rendered.Text, "session_id") || strings.Contains(rendered.Text, "tool_call_id") {
		t.Fatalf("rendered action leaked execution fields: %s", rendered.Text)
	}

	var payload struct {
		Content struct {
			MsgType string `json:"msgtype"`
			Action  struct {
				ID      string `json:"id"`
				Kind    string `json:"kind"`
				Status  string `json:"status"`
				Options []struct {
					ID string `json:"id"`
				} `json:"options"`
			} `json:"action"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(rendered.Text), &payload); err != nil {
		t.Fatalf("decode rendered action: %v", err)
	}
	if payload.Content.MsgType != AgentActionMsgType {
		t.Fatalf("msgtype = %q, want %q", payload.Content.MsgType, AgentActionMsgType)
	}
	if payload.Content.Action.ID != "act-1" || payload.Content.Action.Kind != activity.ActionKindPermission || payload.Content.Action.Status != "pending" || len(payload.Content.Action.Options) != 1 {
		t.Fatalf("action payload = %+v", payload.Content.Action)
	}
}
