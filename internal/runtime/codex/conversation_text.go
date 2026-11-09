package codex

import (
	"strings"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine/contract"
)

type conversationTextItem struct{ id, phase, text string }

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
		item.text += text
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

func (b *conversationText) messageSnapshots() []contract.TurnEvent {
	if len(b.items) == 0 {
		return nil
	}
	items := make([]contract.TurnEvent, 0, len(b.items))
	for _, item := range b.items {
		// Unidentified streams rely on the consumer's tool boundaries.
		if item.id == "" {
			return nil
		}
		items = append(items, contract.TurnEvent{Kind: contract.TurnEventTextDelta, ItemID: item.id, Phase: item.phase, Text: item.text, TextSnapshot: true})
	}
	return items
}

func (b *conversationText) String() string {
	var out strings.Builder
	for _, item := range b.items {
		if item.phase != "commentary" {
			out.WriteString(item.text)
		}
	}
	form := b.fallback
	if form == nil || (form.superseded && strings.TrimSpace(out.String()) != "") {
		return out.String()
	}
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
