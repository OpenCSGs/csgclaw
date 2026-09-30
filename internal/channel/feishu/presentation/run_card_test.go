package presentation

import (
	"encoding/json"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
)

func TestProcessToolLifecycleAndIndependentReply(t *testing.T) {
	process := NewProcess("turn", "scope")
	tool := &agentengine.ToolActivity{ID: "call", Kind: "exec_command", Status: "completed", InputSummary: "pwd", OutputSummary: "/tmp", Payload: map[string]any{"secret": "hidden"}}
	events := process.Observe(agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallUpdate, Tool: tool})
	if len(events) != 4 || events[0].EventType != "TOOL_CALL_START" || events[3].EventType != "TOOL_CALL_RESULT" {
		t.Fatalf("events=%+v", events)
	}
	if len(process.Observe(agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallUpdate, Tool: tool})) != 0 {
		t.Fatal("duplicate tool completion")
	}
	raw, _ := json.Marshal(events)
	if strings.Contains(string(raw), "hidden") {
		t.Fatal("raw payload exposed")
	}
	progress := NewProgress()
	if _, changed := progress.Observe(agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallStart, Tool: tool}); changed {
		t.Fatal("tool rendered in reply")
	}
	reply := progress.Finalize(agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "answer"})
	raw, _ = json.Marshal(reply)
	if !strings.Contains(string(raw), "answer") || strings.Contains(string(raw), "exec_command") {
		t.Fatalf("reply=%s", raw)
	}
}
func TestReplyPreservesLongUTF8Content(t *testing.T) {
	text := strings.Repeat("你好世界\n", 10000)
	cards := ReplyCards(text, "")
	var joined strings.Builder
	for _, card := range cards {
		body := card["body"].(map[string]any)
		joined.WriteString(body["elements"].([]any)[0].(map[string]any)["content"].(string))
		raw, _ := json.Marshal(card)
		if len(raw) > 28000 {
			t.Fatal("oversized card")
		}
	}
	if joined.String() != text {
		t.Fatal("reply content changed")
	}
}
func TestFinalReplyUsesAuthoritativeOutput(t *testing.T) {
	p := NewProgress()
	p.Observe(agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "partial"})
	raw, _ := json.Marshal(p.Finalize(agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "complete"}))
	if strings.Contains(string(raw), "partial") || !strings.Contains(string(raw), "complete") {
		t.Fatalf("reply=%s", raw)
	}
}

func TestLongCodeReplyKeepsFencesAndFitsWireLimit(t *testing.T) {
	text := "```go\n" + strings.Repeat("fmt.Println(\"你好\")\n", 3000) + "```"
	cards := ReplyCards(text, "正在回复…")
	if len(cards) < 2 {
		t.Fatal("expected continuation cards")
	}
	for _, card := range cards {
		content := card["body"].(map[string]any)["elements"].([]any)[0].(map[string]any)["content"].(string)
		if !strings.HasPrefix(content, "```go\n") || !strings.HasSuffix(content, "```") {
			t.Fatalf("unbalanced code card")
		}
		if replyWireSize(content) > 28000 {
			t.Fatal("card exceeds wire budget")
		}
	}
}

func TestProcessPreservesLongDeltasBeforeWireSplitting(t *testing.T) {
	text := strings.Repeat("中文😀\"\n", 2000)
	p := NewProcess("turn", "scope")
	events := p.Observe(agentengine.TurnEvent{Kind: agentengine.TurnEventThoughtDelta, Thought: text})
	events = append(events, p.Observe(agentengine.TurnEvent{Kind: agentengine.TurnEventToolCallStart, Tool: &agentengine.ToolActivity{ID: "tool", Kind: "exec", InputSummary: text}})...)
	found := 0
	for _, e := range events {
		if e.EventType != "REASONING_MESSAGE_CONTENT" && e.EventType != "TOOL_CALL_ARGS" {
			continue
		}
		var b map[string]string
		if err := json.Unmarshal([]byte(e.Content), &b); err != nil {
			t.Fatal(err)
		}
		if b["delta"] != text {
			t.Fatal("delta truncated before transport")
		}
		found++
	}
	if found != 2 {
		t.Fatal("missing deltas")
	}
}

func TestReplyIgnoresCommentaryAndTextSnapshots(t *testing.T) {
	p := NewProgress()
	for _, event := range []agentengine.TurnEvent{
		{Kind: agentengine.TurnEventTextDelta, Text: "Checking tools.", Phase: "commentary"},
		{Kind: agentengine.TurnEventTextDelta, Text: "Complete snapshot.", Phase: "final_answer", TextSnapshot: true},
	} {
		if _, changed := p.Observe(event); changed {
			t.Fatal("non-incremental reply event rendered a card")
		}
	}
	rendered, changed := p.Observe(agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "Answer.", Phase: "final_answer"})
	if !changed {
		t.Fatal("final answer delta did not render a card")
	}
	content := rendered.Cards[0]["body"].(map[string]any)["elements"].([]any)[0].(map[string]any)["content"].(string)
	if content != "Answer." {
		t.Fatalf("reply content = %q", content)
	}
	p.Observe(agentengine.TurnEvent{Kind: agentengine.TurnEventTextDelta, Text: "Answer.", Phase: "final_answer", TextSnapshot: true})
	final := p.Finalize(agentengine.TurnResult{Status: agentengine.TurnCanceled})
	content = final.Cards[0]["body"].(map[string]any)["elements"].([]any)[0].(map[string]any)["content"].(string)
	if content != "Answer." {
		t.Fatalf("canceled reply content = %q", content)
	}
}
