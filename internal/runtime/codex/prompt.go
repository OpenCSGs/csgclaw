package codex

import "strings"

const (
	StopReasonEndTurn     = "end_turn"
	StopReasonInterrupted = "interrupted"
)

type PromptRequest struct {
	SessionID           string
	ClientUserMessageID string
	Prompt              []PromptContentBlock
	Meta                map[string]any
	OnAccepted          func()
}

type PromptResponse struct {
	MessageID  string
	StopReason string
}

type PromptContentBlock struct {
	Text         *PromptTextBlock
	LocalImage   *PromptLocalImageBlock
	ResourceLink *PromptResourceLink
	Resource     *PromptResourceBlock
}

type PromptLocalImageBlock struct {
	Path string
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

func LocalImageBlock(path string) PromptContentBlock {
	return PromptContentBlock{LocalImage: &PromptLocalImageBlock{Path: path}}
}

func textFromPromptBlock(block PromptContentBlock) string {
	switch {
	case block.Text != nil:
		return block.Text.Text
	case block.LocalImage != nil:
		return strings.TrimSpace(block.LocalImage.Path)
	case block.ResourceLink != nil:
		return strings.TrimSpace(block.ResourceLink.Name) + " " + strings.TrimSpace(block.ResourceLink.URI)
	case block.Resource != nil:
		return block.Resource.Text
	default:
		return ""
	}
}
