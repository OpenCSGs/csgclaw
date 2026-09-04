package agentengine

import (
	"context"
	"strings"
	"testing"

	"csgclaw/internal/activity"
)

func TestStructuredQuestionPreservesReadableOutput(t *testing.T) {
	adapter := &codexRuntimeAdapter{}
	var events []TurnEvent
	var output strings.Builder
	var files []*OutputFile
	text := "## 交互式输出演示 - 第 1/3 步\n\n请选择工作流分支。"
	result := adapter.handleEvent(context.Background(), TurnRequest{}, EventSinkFunc(func(_ context.Context, event TurnEvent) error {
		events = append(events, event)
		return nil
	}), activity.RuntimeEvent{
		Kind: activity.RuntimeEventStructuredOutput,
		Text: text,
		Payload: activity.StructuredOutputArtifact{
			RequestUserInput: &activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{ID: "demo_kind", Header: "演示类型", Question: "请选择工作流分支。"}}},
			ResourceLinks:    []activity.ResourceLink{{Type: "resource_link", Name: "docs", URI: "https://example.com/docs"}},
		},
	}, &output, &files)
	if result != nil || len(events) != 2 {
		t.Fatalf("result = %+v, events = %+v", result, events)
	}
	if events[0].Output.Kind != OutputItemRequestUserInput || events[0].Text != text {
		t.Fatalf("question event = %+v, want readable output preserved", events[0])
	}
	if events[1].Output.Kind != OutputItemResourceLink || events[1].Text != "" {
		t.Fatalf("link event = %+v, want link without duplicate text", events[1])
	}
}
