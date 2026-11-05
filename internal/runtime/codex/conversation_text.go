package codex

import (
	"context"
	"errors"
	"strings"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine/contract"
)

type conversationTextItem struct {
	id, phase, text string
	visibleText     string // Plain text already emitted to append-only streams.
}

type conversationQuestionFallback struct {
	questions            []activity.RequestUserInputQuestion
	sourceID, sourceText string
	superseded           bool
}

// Native item completion is authoritative even when it corrects streamed text.
type conversationText struct {
	fallback *conversationQuestionFallback
	items    []conversationTextItem
	indexes  map[string]int
}

func (b *conversationText) apply(id, phase, text string, snapshot bool) {
	if id == "" && phase == "commentary" {
		return
	}
	if b.indexes == nil {
		b.indexes = map[string]int{}
	}
	index, ok := b.indexes[id]
	if !ok {
		index = len(b.items)
		b.indexes[id] = index
		b.items = append(b.items, conversationTextItem{id: id})
	}
	item := &b.items[index]
	if phase != "" {
		item.phase = phase
	}
	if snapshot {
		item.text = text
	} else {
		previous := item.text
		item.text += text
		if phase != "commentary" && text != "" {
			if item.visibleText == previous {
				item.visibleText = item.text
			} else {
				item.visibleText += text
			}
		}
	}
	if form := b.fallback; form != nil && item.phase != "commentary" && strings.TrimSpace(text) != "" {
		// Legacy assistant messages can omit their ID. Their cleaned text also
		// accompanies the structured record.
		sameSource := id == form.sourceID
		if id == "" {
			sameSource = strings.TrimSpace(text) == form.sourceText
		}
		if !sameSource {
			form.superseded = true
		}
	}
}

func (b *conversationText) messageText() string {
	var out strings.Builder
	for _, item := range b.items {
		if item.phase != "commentary" {
			out.WriteString(item.text)
		}
	}
	return out.String()
}

func (b *conversationText) String() string {
	text := b.messageText()
	form := b.fallback
	if form == nil || (form.superseded && strings.TrimSpace(text) != "") {
		return text
	}
	var out strings.Builder
	out.WriteString(text)
	for _, question := range form.questions {
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		if question.IsSecret {
			out.WriteString("A private answer requires a secure question form.")
			continue
		}
		out.WriteString(question.Question)
		for _, option := range question.Options {
			out.WriteString("\n- " + option.Label)
		}
	}
	return out.String()
}

type conversationOutput struct {
	conversationText
	tools map[string]bool // Native tool ID to whether its result was delivered.
}

func (o *conversationOutput) observeTool(kind contract.TurnEventKind, tool *contract.ToolActivity) {
	id, status := strings.TrimSpace(tool.ID), tool.Status
	if _, started := o.tools[id]; !started && kind == contract.TurnEventToolCallUpdate && strings.TrimSpace(tool.Kind) == "" && strings.TrimSpace(tool.Title) == "" {
		// A result without a known tool name or start cannot open a public tool card.
		return
	}
	finished := activity.TerminalToolStatus(status) != "" || (kind == contract.TurnEventToolCallUpdate && status == "")
	if id == "" || (!finished && kind != contract.TurnEventToolCallStart) {
		return
	}
	if o.tools == nil {
		o.tools = map[string]bool{}
	}
	o.tools[id] = o.tools[id] || finished
}

func (o *conversationOutput) restoreFailure(ctx context.Context, sink contract.EventSink, err error) error {
	var failure *quietTurnFailure
	if errors.As(err, &failure) {
		return o.restore(ctx, sink, failure.history)
	}
	return nil
}

func (o *conversationOutput) restore(ctx context.Context, sink contract.EventSink, history quietTurnHistory) error {
	if history == nil {
		return nil
	}
	var native conversationText
	var form *conversationQuestionFallback
	if o.fallback != nil {
		saved := *o.fallback
		form = &saved
	}
	for _, event := range history {
		if event.Kind == contract.TurnEventTextDelta {
			phase := event.Phase
			if index, seen := o.indexes[event.ItemID]; seen && phase == "" {
				phase = o.items[index].phase
			}
			native.apply(event.ItemID, phase, event.Text, true)
		}
		// A text-only question remains part of the answer until a later
		// assistant message supersedes it. History also includes earlier items.
		if form != nil {
			sourceID := event.ItemID
			if event.Tool != nil {
				sourceID = event.Tool.ID
			}
			if sourceID != "" && sourceID == form.sourceID ||
				form.sourceID == "" && form.sourceText != "" && strings.TrimSpace(event.Text) == form.sourceText {
				native.fallback = form
			}
		}
	}
	textOK, legacyPrefix := o.matchHistory(&native)
	lastVisible := -1
	for i := range native.items {
		item := &native.items[i]
		if n, ok := legacyPrefix[item.id]; ok {
			item.visibleText = item.text[:n]
		} else if index, seen := o.indexes[item.id]; seen {
			item.visibleText = o.items[index].visibleText
		}
		if item.visibleText != "" {
			lastVisible = i
		}
	}
	for _, event := range history {
		if event.Tool != nil {
			finished, started := o.tools[event.Tool.ID]
			if finished || (started && event.Kind == contract.TurnEventToolCallStart) {
				continue
			}
			o.observeTool(event.Kind, event.Tool)
		} else {
			if !textOK {
				continue
			}
			index := native.indexes[event.ItemID]
			item := &native.items[index]
			event.Phase = item.phase
			if n, seen := legacyPrefix[event.ItemID]; seen {
				event.Text = event.Text[n:]
				if event.Text == "" || event.Phase == "commentary" {
					continue
				}
			} else if !o.missingText(&event) {
				continue
			}
			if event.Phase != "commentary" && event.Text != "" {
				if index < lastVisible {
					// Earlier items can still be corrected by ID. Appending their
					// suffix after text from a later item would reverse their order.
					event.TextSnapshot = true
					event.Text = item.text
				} else if !event.TextSnapshot {
					item.visibleText += event.Text
				}
			}
		}
		if err := emitTurnEvent(ctx, sink, event); err != nil {
			return err
		}
	}
	if textOK {
		native.fallback = form
		o.conversationText = native
	}
	return nil
}

func (o *conversationOutput) matchHistory(native *conversationText) (bool, map[string]int) {
	for _, item := range o.items {
		if _, ok := native.indexes[item.id]; !ok && item.text != "" && item.phase != "commentary" {
			// Legacy events can omit both the native item ID and its phase.
			// Prefer a match against the normal response body. Include matching
			// commentary if its text was already sent as unphased output.
			previous := o.messageText()
			if ok, prefix := native.matchLegacyPrefix(previous, false); ok {
				return true, prefix
			}
			if item.id == "" {
				return native.matchLegacyPrefix(previous, true)
			}
			return false, nil
		}
	}
	return true, nil
}

func (b *conversationText) matchLegacyPrefix(remaining string, includeCommentary bool) (bool, map[string]int) {
	prefix := make(map[string]int)
	for _, item := range b.items {
		n := min(len(remaining), len(item.text))
		match := remaining[:n] == item.text[:n]
		if item.phase == "commentary" && (!includeCommentary || !match) {
			continue
		}
		if !match {
			return false, nil
		}
		prefix[item.id] = n
		remaining = remaining[n:]
		if remaining == "" {
			return true, prefix
		}
	}
	return false, nil
}

func (o *conversationOutput) missingText(event *contract.TurnEvent) bool {
	index, seen := o.indexes[event.ItemID]
	if !seen {
		return event.Phase != "commentary"
	}
	previous := o.items[index]
	if previous.phase == "commentary" && event.Phase != "commentary" {
		// Unphased deltas can already be visible when an item is later
		// classified as commentary. Promote only its remaining text.
		if strings.HasPrefix(event.Text, previous.visibleText) {
			event.Text = strings.TrimPrefix(event.Text, previous.visibleText)
		} else {
			event.TextSnapshot = true
		}
		return true
	}
	if strings.HasPrefix(event.Text, previous.text) {
		event.Text = strings.TrimPrefix(event.Text, previous.text)
		return event.Text != "" || event.Phase != previous.phase
	}
	event.TextSnapshot = true
	return true
}
