package agentmemory

import (
	"strings"
	"time"
)

const HistoryPageMessages = 32

// HistoryCursor identifies a stable position in a chronological history scan.
// Offset is a UTF-8 byte offset in the sanitized text of a partially read message.
type HistoryCursor struct {
	At        time.Time `json:"at,omitzero"`
	RoomID    string    `json:"room_id,omitempty"`
	MessageID string    `json:"message_id,omitempty"`
	Offset    int       `json:"offset,omitempty"`
}

func (c HistoryCursor) Compare(other HistoryCursor) int {
	if order := c.At.Compare(other.At); order != 0 {
		return order
	}
	if order := strings.Compare(c.RoomID, other.RoomID); order != 0 {
		return order
	}
	return strings.Compare(c.MessageID, other.MessageID)
}

func (c HistoryCursor) advances(previous HistoryCursor) bool {
	if order := c.Compare(previous); order != 0 {
		return order > 0
	}
	return previous.Offset > 0 && (c.Offset == 0 || c.Offset > previous.Offset)
}

type HistoryMessage struct {
	Message
	At time.Time
}

func (m HistoryMessage) Cursor() HistoryCursor {
	return HistoryCursor{At: m.At, RoomID: m.RoomID, MessageID: m.MessageID}
}

// historySegment bounds each model call without discarding later messages or the
// tail of a long message. The returned cursor is committed with the learned notes.
func historySegment(history []HistoryMessage, cursor HistoryCursor) ([]Message, HistoryCursor) {
	const budget = 32 << 10
	var messages []Message
	var size int
	next := cursor
	for _, item := range history {
		position := item.Cursor()
		text := cleanText(item.Text)
		offset := 0
		if position.Compare(cursor) == 0 {
			offset = cursor.Offset
		}
		if offset < 0 || offset >= len(text) || item.RoomID == "" || item.MessageID == "" {
			next = position
			continue
		}
		part := boundText(text[offset:], budget-size)
		if part == "" {
			break
		}
		messages = append(messages, Message{Source: item.Source, Text: part})
		size += len(part)
		next = position
		if offset+len(part) < len(text) {
			next.Offset = offset + len(part)
			break
		}
		if size == budget {
			break
		}
	}
	return messages, next
}
