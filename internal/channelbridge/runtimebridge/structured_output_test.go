package runtimebridge

import (
	"strings"
	"testing"

	"csgclaw/internal/activity"
)

func TestTurnRendererBuffersQuestionAndAppendsDeduplicatedLinks(t *testing.T) {
	t.Parallel()

	renderer := NewTurnRenderer()
	renderer.ApplyText(activity.RuntimeEvent{Kind: activity.RuntimeEventTextDelta, Text: "The demo is ready."})
	renderer.ApplyStructuredOutput(activity.RuntimeEvent{
		Kind: activity.RuntimeEventStructuredOutput,
		Payload: activity.StructuredOutputArtifact{
			RequestUserInput: &activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{
				ID: "q-1", Header: "Question", Question: "Continue?",
			}}},
			ResourceLinks: []activity.ResourceLink{
				{
					Type: "resource_link", Name: "docs", Title: "Documentation", URI: "https://example.com/docs", Description: "Read this first.",
					Icons: []map[string]any{
						{"src": "javascript:alert(1)"},
						{"src": "https://example.com/docs-icon.svg"},
					},
				},
				{Type: "resource_link", Name: "docs duplicate", URI: "https://example.com/docs"},
				{Type: "resource_link", Name: "minimal", URI: "https://example.com/minimal", Icons: []map[string]any{{"src": "data:image/svg+xml,unsafe"}}},
			},
		},
	})
	renderer.ApplyStructuredOutput(activity.RuntimeEvent{
		Kind: activity.RuntimeEventStructuredOutput,
		Payload: activity.StructuredOutputArtifact{
			RequestUserInput: &activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{
				ID: "q-2", Header: "Later", Question: "This second request must be ignored",
			}}},
		},
	})

	messages := renderer.FinalMessages()
	if len(messages) != 1 || !strings.HasPrefix(messages[0], "The demo is ready.\n\nLinks\n") {
		t.Fatalf("messages = %#v", messages)
	}
	if strings.Count(messages[0], "](<https://example.com/docs>)") != 1 || !strings.HasSuffix(messages[0], "[minimal](<https://example.com/minimal>)") {
		t.Fatalf("link markdown = %q", messages[0])
	}
	if !strings.Contains(messages[0], `<img class="resource-link-icon" src="https://example.com/docs-icon.svg" alt="" aria-hidden="true"> [Documentation]`) {
		t.Fatalf("safe resource icon is missing from %q", messages[0])
	}
	if strings.Contains(messages[0], "javascript:") || strings.Contains(messages[0], "data:image") || strings.Count(messages[0], `class="resource-link-icon"`) != 1 {
		t.Fatalf("unsafe resource icon was rendered in %q", messages[0])
	}
	request := renderer.RequestUserInput()
	if request == nil || len(request.Questions) != 1 || request.Questions[0].ID != "q-1" || request.Questions[0].Question != "Continue?" {
		t.Fatalf("request = %+v", request)
	}

	request.Questions[0].Question = "mutated"
	if renderer.RequestUserInput().Questions[0].Question != "Continue?" {
		t.Fatal("RequestUserInput returned mutable renderer state")
	}
}

func TestTurnRendererDiscardsStructuredOutputAfterFailedTurn(t *testing.T) {
	t.Parallel()

	renderer := NewTurnRenderer()
	renderer.ApplyText(activity.RuntimeEvent{Kind: activity.RuntimeEventTextDelta, Text: "Partial text"})
	renderer.ApplyStructuredOutput(activity.RuntimeEvent{
		Kind: activity.RuntimeEventStructuredOutput,
		Payload: activity.StructuredOutputArtifact{
			RequestUserInput: &activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{ID: "q", Header: "Q", Question: "Q?"}}},
			ResourceLinks:    []activity.ResourceLink{{Type: "resource_link", Name: "docs", URI: "https://example.com"}},
		},
	})
	renderer.DiscardStructuredOutput()

	if renderer.RequestUserInput() != nil {
		t.Fatal("request survived failed turn")
	}
	if got := renderer.FinalMessages(); len(got) != 1 || got[0] != "Partial text" {
		t.Fatalf("messages = %#v, want ordinary text without links", got)
	}
}

func TestTurnRendererUsesStructuredCommandStdoutAsQuestionBoundaryFallback(t *testing.T) {
	t.Parallel()

	renderer := NewTurnRenderer()
	renderer.ApplyStructuredOutput(activity.RuntimeEvent{
		Kind: activity.RuntimeEventStructuredOutput,
		Text: "## Step 2\n\nChoose the next branch.",
		Payload: activity.StructuredOutputArtifact{
			RequestUserInput: &activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{ID: "next", Header: "Next", Question: "What next?"}}},
		},
	})

	if got := renderer.FinalMessages(); len(got) != 1 || got[0] != "## Step 2\n\nChoose the next branch." {
		t.Fatalf("messages = %#v, want structured command stdout fallback", got)
	}

	renderer.ApplyText(activity.RuntimeEvent{Kind: activity.RuntimeEventTextDelta, Text: "Model-authored final response."})
	if got := renderer.FinalMessages(); len(got) != 1 || got[0] != "## Step 2\n\nChoose the next branch." {
		t.Fatalf("messages = %#v, want explicit command fallback to remain authoritative", got)
	}
}

func TestTurnRendererUsesReadableDefaultForControlOnlyQuestion(t *testing.T) {
	t.Parallel()

	renderer := NewTurnRenderer()
	renderer.ApplyStructuredOutput(activity.RuntimeEvent{
		Kind: activity.RuntimeEventStructuredOutput,
		Payload: activity.StructuredOutputArtifact{
			RequestUserInput: &activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{ID: "next", Header: "Next", Question: "What next?"}}},
		},
	})

	if got := renderer.FinalMessages(); len(got) != 1 || got[0] != "Please answer the questions below." {
		t.Fatalf("messages = %#v, want default structured question fallback", got)
	}
}
