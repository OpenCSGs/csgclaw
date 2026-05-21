package codexbridge

import (
	"fmt"
	"strings"

	runtimecodex "csgclaw/internal/runtime/codex"

	acp "github.com/coder/acp-go-sdk"
)

type turnRenderer struct {
	text        strings.Builder
	toolTitles  map[string]string
	toolStates  map[string]string
	promptError string
}

func newTurnRenderer() *turnRenderer {
	return &turnRenderer{
		toolTitles: make(map[string]string),
		toolStates: make(map[string]string),
	}
}

func (r *turnRenderer) Apply(event runtimecodex.SessionEvent) []string {
	if r == nil {
		return nil
	}

	switch event.Kind {
	case runtimecodex.SessionEventTextDelta:
		if event.Text != "" {
			_, _ = r.text.WriteString(event.Text)
		}
	case runtimecodex.SessionEventToolCallStart:
		title := r.displayToolTitle(event)
		r.toolTitles[event.ToolCallID] = title
		r.toolStates[event.ToolCallID] = normalizedToolStatus(event.ToolStatus)
		return []string{fmt.Sprintf("🔧 Running tool: %s", title)}
	case runtimecodex.SessionEventToolCallUpdate:
		title := r.displayToolTitle(event)
		if strings.TrimSpace(event.ToolTitle) != "" && event.ToolCallID != "" {
			r.toolTitles[event.ToolCallID] = strings.TrimSpace(event.ToolTitle)
		}
		status := normalizedToolStatus(event.ToolStatus)
		if status == "" {
			return nil
		}
		if event.ToolCallID != "" && r.toolStates[event.ToolCallID] == status {
			return nil
		}
		if event.ToolCallID != "" {
			r.toolStates[event.ToolCallID] = status
		}
		switch status {
		case "completed":
			return []string{formatToolUpdate("Tool completed", title, displayToolOutput(event))}
		case "failed":
			return []string{formatToolUpdate("Tool failed", title, displayToolOutput(event))}
		}
	case runtimecodex.SessionEventPromptFailed:
		r.promptError = strings.TrimSpace(event.Error)
	}
	return nil
}

func (r *turnRenderer) FinalMessages() []string {
	if r == nil {
		return nil
	}
	var messages []string
	if text := strings.TrimSpace(r.text.String()); text != "" {
		messages = append(messages, text)
	}
	if r.promptError != "" {
		messages = append(messages, fmt.Sprintf("Codex runtime error: %s", r.promptError))
	}
	return messages
}

func (r *turnRenderer) displayToolTitle(event runtimecodex.SessionEvent) string {
	title := strings.TrimSpace(event.ToolTitle)
	if title == "" && event.ToolCallID != "" {
		title = strings.TrimSpace(r.toolTitles[event.ToolCallID])
	}
	if title == "" {
		title = "tool"
	}
	return title
}

func formatToolUpdate(label, title, output string) string {
	text := fmt.Sprintf("🔧 %s: %s", label, title)
	if strings.TrimSpace(output) == "" {
		return text
	}
	return fmt.Sprintf("%s\n\n%s", text, strings.TrimSpace(output))
}

func displayToolOutput(event runtimecodex.SessionEvent) string {
	switch payload := event.Payload.(type) {
	case acp.SessionToolCallUpdate:
		return displayRawOutput(payload.RawOutput)
	case *acp.SessionToolCallUpdate:
		if payload != nil {
			return displayRawOutput(payload.RawOutput)
		}
	}
	return ""
}

func displayRawOutput(output any) string {
	switch value := output.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func normalizedToolStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}
