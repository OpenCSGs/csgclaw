// Package presentation renders Agent Engine events into Feishu Markdown or
// CardKit payloads without importing an external bridge SDK.
package presentation

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"csgclaw/internal/agentengine"
)

type Mode string

const (
	ModeMarkdown Mode = "markdown"
	ModeCard     Mode = "card"
)

const (
	streamFlushInterval = 400 * time.Millisecond
	maxTextBytes        = 20 << 10
	maxReasoningBytes   = 1536
	maxRichWireBytes    = 28 << 10
	maxToolSummaryRunes = 80
)

func NormalizeMode(value string) Mode {
	if strings.EqualFold(strings.TrimSpace(value), string(ModeCard)) {
		return ModeCard
	}
	return ModeMarkdown
}

type Rendered struct {
	Mode          Mode
	Markdown      string
	MarkdownParts []string
	Card          map[string]any
}

type runStatus string

const (
	statusRunning   runStatus = "running"
	statusSucceeded runStatus = "succeeded"
	statusFailed    runStatus = "failed"
	statusCanceled  runStatus = "canceled"
)

type toolState struct {
	ID       string
	Name     string
	Input    any
	Output   string
	Finished bool
	Failed   bool
}

type Progress struct {
	mode             Mode
	status           runStatus
	text             string
	reasoning        string
	errorMessage     string
	tools            []*toolState
	toolByID         map[string]*toolState
	lastFlush        time.Time
	maxMarkdownParts int
}

func NewProgress(mode Mode, _, _, _ string) *Progress {
	return &Progress{
		mode:     NormalizeMode(string(mode)),
		status:   statusRunning,
		toolByID: make(map[string]*toolState),
	}
}

func Initial(mode Mode, turnID, scope, threadID string) Rendered {
	return NewProgress(mode, turnID, scope, threadID).render()
}

func (p *Progress) Observe(event agentengine.TurnEvent) (Rendered, bool) {
	if p == nil {
		return Rendered{}, false
	}
	immediate := false
	switch event.Kind {
	case agentengine.TurnEventTextDelta:
		if event.Phase == "commentary" || event.TextSnapshot {
			return Rendered{}, false
		}
		if event.Text == "" {
			return Rendered{}, false
		}
		p.text += event.Text
	case agentengine.TurnEventThoughtDelta:
		if event.Thought == "" {
			return Rendered{}, false
		}
		p.reasoning += event.Thought
	case agentengine.TurnEventToolCallStart, agentengine.TurnEventToolCallUpdate:
		if !p.observeTool(event.Tool) {
			return Rendered{}, false
		}
		immediate = true
	default:
		return Rendered{}, false
	}
	p.compact()
	now := time.Now()
	flush := immediate || p.lastFlush.IsZero() || now.Sub(p.lastFlush) >= streamFlushInterval
	if !flush {
		return Rendered{}, false
	}
	p.lastFlush = now
	return p.render(), true
}

func (p *Progress) observeTool(activity *agentengine.ToolActivity) bool {
	if activity == nil || strings.TrimSpace(activity.ID) == "" {
		return false
	}
	id := strings.TrimSpace(activity.ID)
	tool := p.toolByID[id]
	if tool == nil {
		tool = &toolState{
			ID:    id,
			Name:  toolDisplayName(p.mode, activity),
			Input: decodedToolSummary(activity.InputSummary),
		}
		p.toolByID[id] = tool
		p.tools = append(p.tools, tool)
		return true
	}
	status := strings.ToLower(strings.TrimSpace(activity.Status))
	if !terminalToolStatus(status) {
		return false
	}
	tool.Finished = true
	tool.Failed = status == "failed" || status == "error" || status == "canceled" || status == "cancelled"
	tool.Output = strings.TrimSpace(activity.OutputSummary)
	return true
}

func (p *Progress) Finalize(result agentengine.TurnResult) Rendered {
	if p == nil {
		return Terminal(ModeMarkdown, result)
	}
	if strings.TrimSpace(p.text) == "" && result.Status == agentengine.TurnSucceeded {
		p.text = result.Output
	}
	switch result.Status {
	case agentengine.TurnCanceled:
		p.status = statusCanceled
	case agentengine.TurnFailed:
		p.status = statusFailed
		p.errorMessage = "Agent execution failed"
		if result.Error != nil && strings.TrimSpace(result.Error.Message) != "" {
			p.errorMessage = strings.TrimSpace(result.Error.Message)
		}
	default:
		p.status = statusSucceeded
	}
	p.compact()
	rendered := p.render()
	if rendered.Mode == ModeMarkdown {
		for len(rendered.MarkdownParts) < p.maxMarkdownParts {
			rendered.MarkdownParts = append(rendered.MarkdownParts, "_（内容已结束）_")
		}
	}
	return rendered
}

func Terminal(mode Mode, result agentengine.TurnResult) Rendered {
	return NewProgress(mode, "", "", "").Finalize(result)
}

func toolDisplayName(mode Mode, tool *agentengine.ToolActivity) string {
	kind := strings.ToLower(strings.TrimSpace(tool.Kind))
	if mode == ModeMarkdown {
		if kind == "exec_command" {
			return "command_execution"
		}
		if kind != "" {
			return kind
		}
		if title := strings.TrimSpace(tool.Title); title != "" {
			return title
		}
		return "tool"
	}
	switch kind {
	case "exec_command":
		return "Bash"
	case "patch_apply":
		return "Edit"
	case "web_search":
		return "WebSearch"
	}
	if title := strings.TrimSpace(tool.Title); title != "" {
		return title
	}
	if kind != "" {
		return kind
	}
	return "Tool"
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

func terminalToolStatus(status string) bool {
	switch status {
	case "completed", "succeeded", "failed", "error", "canceled", "cancelled":
		return true
	default:
		return false
	}
}

func (p *Progress) compact() {
	p.text = tailUTF8(p.text, maxTextBytes)
	p.reasoning = tailUTF8(p.reasoning, maxReasoningBytes)
}

func tailUTF8(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	start := len(value) - limit
	for start < len(value) && !utf8.RuneStart(value[start]) {
		start++
	}
	return "…" + value[start:]
}

func (p *Progress) render() Rendered {
	if p.mode == ModeMarkdown {
		parts := splitMarkdownWire(p.renderMarkdown())
		if len(parts) > p.maxMarkdownParts {
			p.maxMarkdownParts = len(parts)
		}
		return Rendered{Mode: ModeMarkdown, Markdown: parts[0], MarkdownParts: parts}
	}
	card := p.renderCard()
	fitCardWire(card)
	return Rendered{Mode: ModeCard, Card: card}
}

func (p *Progress) renderMarkdown() string {
	var sections []string
	if reasoning := strings.TrimSpace(p.reasoning); reasoning != "" {
		sections = append(sections, "> "+strings.ReplaceAll(reasoning, "\n", "\n> "))
	}
	toolLines := make([]string, 0, len(p.tools))
	for _, tool := range p.tools {
		icon := "⏳"
		if tool.Finished {
			icon = "✅"
			if tool.Failed {
				icon = "❌"
			}
		}
		line := "> " + icon + " **" + tool.Name + "**"
		if summary := markdownToolInputSummary(tool.Input); summary != "" {
			line += " — " + summary
		}
		toolLines = append(toolLines, line)
	}
	if len(toolLines) > 0 {
		sections = append(sections, strings.Join(toolLines, "\n"))
	}
	if text := strings.TrimSpace(p.text); text != "" {
		sections = append(sections, text)
	}
	if p.status == statusFailed && p.errorMessage != "" {
		sections = append(sections, "❌ "+p.errorMessage)
	}
	if p.status == statusCanceled {
		sections = append(sections, "_已取消_")
	}
	if p.status == statusRunning {
		footer := "_正在思考…_"
		if p.text != "" || len(p.tools) > 0 {
			footer = "_正在输出…_"
		}
		sections = append(sections, footer)
	}
	return strings.Join(sections, "\n\n")
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
	if len(runes) <= maxToolSummaryRunes {
		return summary
	}
	return string(runes[:maxToolSummaryRunes]) + "…"
}

func (p *Progress) renderCard() map[string]any {
	elements := make([]any, 0, len(p.tools)+4)
	if reasoning := strings.TrimSpace(p.reasoning); reasoning != "" {
		elements = append(elements, markdownElement("> "+strings.ReplaceAll(reasoning, "\n", "\n> ")))
	}
	finished := 0
	for _, tool := range p.tools {
		if tool.Finished {
			finished++
		}
		icon, state := "⏳", "运行中"
		if tool.Finished {
			icon, state = "✅", "已完成"
			if tool.Failed {
				icon, state = "❌", "失败"
			}
		}
		content := toolInputSummary(tool.Input)
		if tool.Output != "" {
			if content != "" {
				content += "\n\n"
			}
			content += tool.Output
		}
		elements = append(elements, map[string]any{
			"tag":      "collapsible_panel",
			"expanded": !tool.Finished,
			"header": map[string]any{
				"title": map[string]any{"tag": "plain_text", "content": icon + " " + tool.Name + "（" + state + "）"},
			},
			"elements": []any{markdownElement(content)},
		})
	}
	if finished >= 3 {
		elements = append([]any{map[string]any{
			"tag":      "collapsible_panel",
			"expanded": false,
			"header": map[string]any{
				"title": map[string]any{"tag": "plain_text", "content": fmt.Sprintf("%d 个工具调用（已结束）", finished)},
			},
			"elements": []any{},
		}}, elements...)
	}
	if text := strings.TrimSpace(p.text); text != "" {
		elements = append(elements, markdownElement(text))
	}
	if p.status == statusFailed && p.errorMessage != "" {
		elements = append(elements, markdownElement("❌ "+p.errorMessage))
	}
	if p.status == statusCanceled {
		elements = append(elements, markdownElement("_已取消_"))
	}
	if p.status == statusRunning {
		statusText := "🧠 正在思考…"
		if p.text != "" || len(p.tools) > 0 {
			statusText = "⏳ 正在输出…"
		}
		elements = append(elements, map[string]any{
			"tag":       "markdown",
			"content":   statusText,
			"text_size": "notation",
		})
		// Card JSON 2.0 treats button as a body element and declares callbacks
		// through behaviors. action/actions and a top-level value belong to the
		// legacy card schema and cause Feishu to reject a schema 2.0 card.
		elements = append(elements, map[string]any{
			"tag":  "button",
			"text": map[string]any{"tag": "plain_text", "content": "⏹ 终止"},
			"type": "danger",
			"behaviors": []any{map[string]any{
				"type":  "callback",
				"value": map[string]any{"cmd": "stop"},
			}},
		})
	}
	return map[string]any{
		"schema": "2.0",
		"config": map[string]any{
			"streaming_mode": p.status == statusRunning,
			"update_multi":   true,
		},
		"body": map[string]any{"elements": elements},
	}
}

func markdownElement(content string) map[string]any {
	return map[string]any{"tag": "markdown", "content": content}
}

func splitMarkdownWire(markdown string) []string {
	if markdown == "" || markdownWireBytes(markdown) <= maxRichWireBytes {
		return []string{markdown}
	}
	parts := make([]string, 0, len(markdown)/maxRichWireBytes+1)
	for markdown != "" {
		end := markdownWireChunkEnd(markdown)
		parts = append(parts, markdown[:end])
		markdown = markdown[end:]
	}
	return parts
}

func markdownWireChunkEnd(markdown string) int {
	if markdownWireBytes(markdown) <= maxRichWireBytes {
		return len(markdown)
	}
	boundaries := make([]int, 0, len(markdown)+1)
	for index := range markdown {
		if index > 0 {
			boundaries = append(boundaries, index)
		}
	}
	if len(boundaries) == 0 || boundaries[len(boundaries)-1] != len(markdown) {
		boundaries = append(boundaries, len(markdown))
	}
	low, high, best := 0, len(boundaries)-1, 0
	for low <= high {
		middle := low + (high-low)/2
		end := boundaries[middle]
		if markdownWireBytes(markdown[:end]) <= maxRichWireBytes {
			best = end
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	if best > 0 {
		return best
	}
	_, size := utf8.DecodeRuneInString(markdown)
	return size
}

func markdownWireBytes(markdown string) int {
	content, err := json.Marshal(map[string]any{
		"zh_cn": map[string]any{"content": [][]map[string]string{{{"tag": "md", "text": markdown}}}},
	})
	if err != nil {
		return maxRichWireBytes + 1
	}
	wire, err := json.Marshal(map[string]any{
		"receive_id": strings.Repeat("x", 128),
		"msg_type":   "post",
		"content":    string(content),
		"uuid":       strings.Repeat("x", 36),
	})
	if err != nil {
		return maxRichWireBytes + 1
	}
	return len(wire)
}

func fitCardWire(card map[string]any) {
	for cardWireBytes(card) > maxRichWireBytes {
		container, key, content := longestCardContent(card)
		if container == nil || utf8.RuneCountInString(content) <= 1 {
			return
		}
		runes := []rune(content)
		container[key] = string(runes[:max(1, len(runes)/2)]) + "…"
	}
}

func longestCardContent(value any) (map[string]any, string, string) {
	var bestContainer map[string]any
	bestKey, bestValue := "", ""
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, item := range typed {
				if key == "content" {
					if content, ok := item.(string); ok && len(content) > len(bestValue) {
						bestContainer, bestKey, bestValue = typed, key, content
					}
				}
				visit(item)
			}
		case []any:
			for _, item := range typed {
				visit(item)
			}
		}
	}
	visit(value)
	return bestContainer, bestKey, bestValue
}

func cardWireBytes(card map[string]any) int {
	content, err := json.Marshal(card)
	if err != nil {
		return maxRichWireBytes + 1
	}
	wire, err := json.Marshal(map[string]any{
		"receive_id": strings.Repeat("x", 128),
		"msg_type":   "interactive",
		"content":    string(content),
		"uuid":       strings.Repeat("x", 36),
	})
	if err != nil {
		return maxRichWireBytes + 1
	}
	return len(wire)
}
