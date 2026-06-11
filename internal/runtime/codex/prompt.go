package codex

import "strings"

const StopReasonEndTurn = "end_turn"

type PromptRequest struct {
	SessionID string
	Prompt    []PromptContentBlock
	Meta      map[string]any
}

type PromptResponse struct {
	MessageID  string
	StopReason string
}

type PromptContentBlock struct {
	Text         *PromptTextBlock
	ResourceLink *PromptResourceLink
	Resource     *PromptResourceBlock
}

type PromptTextBlock struct {
	Text string
}

type PromptResourceLink struct {
	Name string
	URI  string
}

type PromptResourceBlock struct {
	Text string
}

func TextBlock(text string) PromptContentBlock {
	return PromptContentBlock{
		Text: &PromptTextBlock{Text: text},
	}
}

func textFromPromptBlock(block PromptContentBlock) string {
	switch {
	case block.Text != nil:
		return block.Text.Text
	case block.ResourceLink != nil:
		return strings.TrimSpace(block.ResourceLink.Name) + " " + strings.TrimSpace(block.ResourceLink.URI)
	case block.Resource != nil:
		return block.Resource.Text
	default:
		return ""
	}
}
