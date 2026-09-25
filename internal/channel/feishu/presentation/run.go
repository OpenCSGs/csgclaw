// Package presentation maps Engine output to Feishu process events and cards.
package presentation

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"csgclaw/internal/agentengine"
)

const streamFlushInterval = 600 * time.Millisecond

type Rendered struct{ Cards []map[string]any }
type Progress struct {
	text      string
	lastFlush time.Time
	parts     int
}

func NewProgress() *Progress { return &Progress{} }
func (p *Progress) Observe(event agentengine.TurnEvent) (Rendered, bool) {
	if event.Kind != agentengine.TurnEventTextDelta || event.Text == "" {
		return Rendered{}, false
	}
	p.text += event.Text
	if !p.lastFlush.IsZero() && time.Since(p.lastFlush) < streamFlushInterval {
		return Rendered{}, false
	}
	p.lastFlush = time.Now()
	cards := ReplyCards(p.text, "正在回复…")
	p.parts = max(p.parts, len(cards))
	return Rendered{Cards: cards}, true
}
func (p *Progress) Finalize(result agentengine.TurnResult) Rendered {
	text := result.Output
	if strings.TrimSpace(text) == "" {
		text = p.text
	}
	status := ""
	switch result.Status {
	case agentengine.TurnCanceled:
		status = "已取消，回复可能尚未完成。"
	case agentengine.TurnFailed:
		status = "执行失败，回复可能尚未完成。"
		if result.Error != nil {
			status += "\n" + result.Error.Message
		}
	}
	if strings.TrimSpace(text) == "" && status == "" && len(result.Interactions) == 0 {
		text = "已完成。"
	}
	if strings.TrimSpace(text) == "" && status == "" {
		return Rendered{}
	}
	cards := ReplyCards(text, status)
	for len(cards) < p.parts {
		cards = append(cards, Card("回复已结束。"))
	}
	return Rendered{Cards: cards}
}
func Terminal(result agentengine.TurnResult) Rendered { return NewProgress().Finalize(result) }
func Card(text string) map[string]any {
	return map[string]any{"schema": "2.0", "config": map[string]any{"update_multi": true}, "body": map[string]any{"elements": []any{markdownElement(text)}}}
}
func markdownElement(text string) map[string]any {
	return map[string]any{"tag": "markdown", "content": text}
}

// ReplyCards accounts for the JSON string inside the message envelope. Fenced
// code blocks remain readable when a long answer requires several cards.
func ReplyCards(text, status string) []map[string]any {
	var cards []map[string]any
	runes := []rune(text)
	fence := ""
	for len(runes) > 0 {
		prefix := ""
		if fence != "" {
			prefix = fence + "\n"
		}
		low, high, end := 1, min(len(runes), 10000), 1
		for low <= high {
			mid := (low + high) / 2
			if replyWireSize(prefix+string(runes[:mid])+"\n```\n"+status) <= 26000 {
				end = mid
				low = mid + 1
			} else {
				high = mid - 1
			}
		}
		if end < len(runes) {
			for i := end; i > end/2; i-- {
				if runes[i-1] == '\n' {
					end = i
					break
				}
			}
		}
		part := string(runes[:end])
		for _, line := range strings.Split(part, "\n") {
			trimmed := strings.TrimSpace(line)
			if fence == "" {
				if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
					fence = trimmed
				}
			} else if strings.HasPrefix(trimmed, fenceMarker(fence)) {
				fence = ""
			}
		}
		content := prefix + part
		if fence != "" {
			content += "\n" + fenceMarker(fence)
		}
		cards = append(cards, Card(content))
		runes = runes[end:]
	}
	if len(cards) == 0 {
		return []map[string]any{Card(status)}
	}
	if status != "" {
		body := cards[len(cards)-1]["body"].(map[string]any)
		body["elements"] = append(body["elements"].([]any), markdownElement(status))
	}
	return cards
}
func fenceMarker(fence string) string {
	if fence == "" {
		return ""
	}
	end := 0
	for end < len(fence) && fence[end] == fence[0] {
		end++
	}
	return fence[:end]
}
func replyWireSize(text string) int {
	content, _ := json.Marshal(Card(text))
	wire, _ := json.Marshal(map[string]any{"receive_id": strings.Repeat("x", 128), "msg_type": "interactive", "content": string(content), "uuid": strings.Repeat("x", 36)})
	return len(wire)
}

func decodedToolSummary(summary string) any {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	var value any
	if json.Unmarshal([]byte(summary), &value) == nil {
		return value
	}
	for _, key := range []string{"command", "cmd", "input"} {
		if value := truncatedJSONStringField(summary, key); value != "" {
			return map[string]any{key: value}
		}
	}
	return map[string]any{"input": summary}
}

// truncatedJSONStringField recovers a string field when the runtime's bounded
// tool summary ends before the surrounding JSON object (or string) is closed.
func truncatedJSONStringField(summary, key string) string {
	marker := `"` + key + `"`
	index := strings.Index(summary, marker)
	if index < 0 {
		return ""
	}
	rest := strings.TrimSpace(summary[index+len(marker):])
	if !strings.HasPrefix(rest, ":") {
		return ""
	}
	rest = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
	if !strings.HasPrefix(rest, `"`) {
		return ""
	}

	var value string
	decoder := json.NewDecoder(strings.NewReader(rest))
	if decoder.Decode(&value) == nil {
		return value
	}
	rest = strings.TrimSuffix(rest, "...")
	for end := len(rest); end > 1; end-- {
		if json.Unmarshal([]byte(rest[:end]+`"`), &value) == nil {
			return value
		}
	}
	return ""
}

func toolInputSummary(input any) string {
	if input == nil {
		return ""
	}
	if fields, ok := input.(map[string]any); ok {
		for _, key := range []string{"command", "cmd", "input"} {
			if value := strings.TrimSpace(fmt.Sprint(fields[key])); value != "" && value != "<nil>" {
				return value
			}
		}
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return strings.TrimSpace(fmt.Sprint(input))
	}
	return string(encoded)
}

func markdownToolInputSummary(input any) string {
	summary := strings.Join(strings.Fields(toolInputSummary(input)), " ")
	runes := []rune(summary)
	if len(runes) <= 80 {
		return summary
	}
	return string(runes[:80]) + "…"
}
