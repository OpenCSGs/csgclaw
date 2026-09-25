package presentation

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"csgclaw/internal/agentengine"
	channel "csgclaw/internal/channel"
)

// Process keeps per-tool lifecycle state. Events are append-only and never
// contain reply text or Runtime payloads.
type Process struct {
	runID, scope   string
	tools          map[string]bool
	reasoning      bool
	reasoningIndex int
}

func NewProcess(runID, scope string) *Process {
	return &Process{runID: runID, scope: scope, tools: map[string]bool{}}
}

func processEvent(kind string, content map[string]any) channel.COTEvent {
	raw, _ := json.Marshal(content)
	return channel.COTEvent{EventType: kind, Content: string(raw), Timestamp: time.Now().UnixMilli()}
}

func (p *Process) Start() []channel.COTEvent {
	return []channel.COTEvent{processEvent("RUN_STARTED", map[string]any{"runId": p.runID, "threadId": p.scope})}
}

func (p *Process) Observe(e agentengine.TurnEvent) []channel.COTEvent {
	var events []channel.COTEvent
	if e.Kind != agentengine.TurnEventThoughtDelta {
		events = append(events, p.closeReasoning()...)
	}
	add := func(kind string, body map[string]any) { events = append(events, processEvent(kind, body)) }
	switch e.Kind {
	case agentengine.TurnEventThoughtDelta:
		if e.Thought == "" {
			break
		}
		if !p.reasoning {
			p.reasoningIndex++
		}
		id := p.reasoningID()
		if !p.reasoning {
			p.reasoning = true
			add("REASONING_START", map[string]any{"messageId": id})
			add("REASONING_MESSAGE_START", map[string]any{"messageId": id, "role": "reasoning"})
		}
		add("REASONING_MESSAGE_CONTENT", map[string]any{"messageId": id, "delta": bounded(e.Thought, 1200)})
	case agentengine.TurnEventToolCallStart, agentengine.TurnEventToolCallUpdate:
		t := e.Tool
		if t == nil || t.ID == "" {
			break
		}
		done, exists := p.tools[t.ID]
		if done {
			break
		}
		if !exists {
			name := strings.TrimSpace(t.Kind)
			if name == "" {
				name = t.Title
			}
			if name == "" {
				name = "tool"
			}
			title := name
			if summary := markdownToolInputSummary(decodedToolSummary(t.InputSummary)); summary != "" {
				title += " — " + summary
			}
			add("TOOL_CALL_START", map[string]any{"toolCallId": t.ID, "toolCallName": name, "title": title, "icon": "default"})
			if t.InputSummary != "" {
				add("TOOL_CALL_ARGS", map[string]any{"toolCallId": t.ID, "delta": bounded(t.InputSummary, 1200)})
			}
			add("TOOL_CALL_END", map[string]any{"toolCallId": t.ID})
			p.tools[t.ID] = false
		}
		switch strings.ToLower(t.Status) {
		case "completed", "succeeded", "failed", "error", "canceled", "cancelled":
			p.tools[t.ID] = true
			content := t.Status
			if t.OutputSummary != "" {
				content += "\n" + bounded(t.OutputSummary, 1200)
			}
			add("TOOL_CALL_RESULT", map[string]any{"toolCallId": t.ID, "messageId": "result-" + t.ID, "role": "tool", "content": content})
		}
	}
	return events
}

func Waiting(id string, finished bool) channel.COTEvent {
	kind := "STEP_STARTED"
	if finished {
		kind = "STEP_FINISHED"
	}
	return processEvent(kind, map[string]any{"stepId": "interaction-" + id, "stepName": "等待用户操作，请查看确认卡片"})
}

func (p *Process) Finish(status agentengine.TurnStatus) []channel.COTEvent {
	events := p.closeReasoning()
	ids := make([]string, 0, len(p.tools))
	for id := range p.tools {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if !p.tools[id] {
			events = append(events, processEvent("TOOL_CALL_RESULT", map[string]any{"toolCallId": id, "messageId": "result-" + id, "role": "tool", "content": "执行已结束，未收到工具结果"}))
		}
	}
	if status == agentengine.TurnFailed {
		return append(events, processEvent("RUN_ERROR", map[string]any{"message": "执行失败", "code": "runtime_failed"}))
	}
	result := "done"
	if status == agentengine.TurnCanceled {
		result = "canceled"
	}
	return append(events, processEvent("RUN_FINISHED", map[string]any{"runId": p.runID, "threadId": p.scope, "status": result}))
}

func bounded(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func (p *Process) reasoningID() string {
	return fmt.Sprintf("reasoning-%s-%d", p.runID, p.reasoningIndex)
}

func (p *Process) closeReasoning() []channel.COTEvent {
	if !p.reasoning {
		return nil
	}
	p.reasoning = false
	body := map[string]any{"messageId": p.reasoningID()}
	return []channel.COTEvent{processEvent("REASONING_MESSAGE_END", body), processEvent("REASONING_END", body)}
}
