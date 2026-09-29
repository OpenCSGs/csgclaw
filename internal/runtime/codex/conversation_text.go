package codex

import "strings"

type conversationTextItem struct{ id, phase, text string }

// Native item completion is authoritative even when it corrects streamed text.
type conversationText struct {
	items   []conversationTextItem
	indexes map[string]int
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
}
func (b *conversationText) String() string {
	var out strings.Builder
	for _, item := range b.items {
		if item.phase != "commentary" {
			out.WriteString(item.text)
		}
	}
	return out.String()
}
