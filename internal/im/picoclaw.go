package im

import (
	"encoding/json"
	"fmt"
	"sync"
)

type PicoClawBridge struct {
	mu          sync.Mutex
	subscribers map[string]map[chan PicoClawEvent]struct{}
	pending     map[string][]PicoClawEvent
	seen        map[string]map[string]struct{}
}

const maxPendingPicoClawEventsPerBot = 64

type PicoClawEvent struct {
	MessageID string         `json:"message_id"`
	RoomID    string         `json:"room_id"`
	ChatType  string         `json:"chat_type"`
	Sender    PicoClawSender `json:"sender"`
	Text      string         `json:"text"`
	Timestamp string         `json:"timestamp"`
	Mentions  []string       `json:"mentions,omitempty"`
}

type PicoClawSender struct {
	ID          string `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

type PicoClawSendMessageRequest struct {
	RoomID string `json:"room_id"`
	Text   string `json:"text"`
}

func NewPicoClawBridge(string) *PicoClawBridge {
	return &PicoClawBridge{
		subscribers: make(map[string]map[chan PicoClawEvent]struct{}),
		pending:     make(map[string][]PicoClawEvent),
		seen:        make(map[string]map[string]struct{}),
	}
}

func (b *PicoClawBridge) Subscribe(botID string) (<-chan PicoClawEvent, func()) {
	ch := make(chan PicoClawEvent, 16)

	b.mu.Lock()
	if b.subscribers[botID] == nil {
		b.subscribers[botID] = make(map[chan PicoClawEvent]struct{})
	}
	b.subscribers[botID][ch] = struct{}{}
	pending := append([]PicoClawEvent(nil), b.pending[botID]...)
	delete(b.pending, botID)
	b.mu.Unlock()

	var unsent []PicoClawEvent
	var sent []PicoClawEvent
	for _, evt := range pending {
		select {
		case ch <- evt:
			sent = append(sent, evt)
		default:
			unsent = append(unsent, evt)
		}
	}
	if len(sent) > 0 || len(unsent) > 0 {
		b.mu.Lock()
		for _, evt := range sent {
			b.markSeenLocked(botID, evt.MessageID)
		}
		for _, evt := range unsent {
			b.addPendingLocked(botID, evt)
		}
		b.mu.Unlock()
	}

	cancel := func() {
		b.mu.Lock()
		if subs, ok := b.subscribers[botID]; ok {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(b.subscribers, botID)
			}
		}
		b.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}

func (b *PicoClawBridge) SubscriberCount(botID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subscribers[botID])
}

func (b *PicoClawBridge) PublishMessageEvent(room Room, sender User, message Message) []string {
	var missed []string
	for _, botID := range room.Members {
		if !shouldNotifyBot(room, message, botID) {
			continue
		}
		if !b.EnqueueMessageEvent(room, sender, message, botID) {
			missed = append(missed, botID)
		}
	}
	return missed
}

func (b *PicoClawBridge) EnqueueMessageEvent(room Room, sender User, message Message, botID string) bool {
	if !shouldNotifyBot(room, message, botID) {
		return true
	}
	return b.enqueue(botID, messageEventForBot(room, sender, message, botID))
}

func (b *PicoClawBridge) enqueue(botID string, evt PicoClawEvent) bool {
	b.mu.Lock()
	if b.hasSeenLocked(botID, evt.MessageID) {
		b.mu.Unlock()
		return true
	}
	subs := make([]chan PicoClawEvent, 0, len(b.subscribers[botID]))
	for ch := range b.subscribers[botID] {
		subs = append(subs, ch)
	}
	if len(subs) == 0 {
		b.addPendingLocked(botID, evt)
		b.mu.Unlock()
		return false
	}
	b.mu.Unlock()

	sent := false
	for _, ch := range subs {
		select {
		case ch <- evt:
			sent = true
		default:
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if sent {
		b.markSeenLocked(botID, evt.MessageID)
		return true
	}
	b.addPendingLocked(botID, evt)
	return false
}

func (b *PicoClawBridge) addPendingLocked(botID string, evt PicoClawEvent) {
	if b.hasSeenLocked(botID, evt.MessageID) {
		return
	}
	pending := append(b.pending[botID], evt)
	if len(pending) > maxPendingPicoClawEventsPerBot {
		pending = pending[len(pending)-maxPendingPicoClawEventsPerBot:]
	}
	b.pending[botID] = pending
}

func (b *PicoClawBridge) hasSeenLocked(botID, messageID string) bool {
	if messageID == "" {
		return false
	}
	_, ok := b.seen[botID][messageID]
	return ok
}

func (b *PicoClawBridge) markSeenLocked(botID, messageID string) {
	if messageID == "" {
		return
	}
	if b.seen[botID] == nil {
		b.seen[botID] = make(map[string]struct{})
	}
	b.seen[botID][messageID] = struct{}{}
}

func messageEventForBot(room Room, sender User, message Message, botID string) PicoClawEvent {
	return PicoClawEvent{
		MessageID: message.ID,
		RoomID:    room.ID,
		ChatType:  chatTypeForRoom(room),
		Sender: PicoClawSender{
			ID:          sender.ID,
			Username:    sender.Handle,
			DisplayName: sender.Name,
		},
		Text:      message.Content,
		Timestamp: fmt.Sprintf("%d", message.CreatedAt.UnixMilli()),
		Mentions:  mentionsForBot(message.Mentions, botID),
	}
}

func (e PicoClawEvent) MarshalJSONLine() ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func shouldNotifyBot(room Room, message Message, botID string) bool {
	if message.SenderID == botID {
		return false
	}
	if !containsUserIDInRoom(room, botID) {
		return false
	}
	return true
}

func mentionsForBot(mentions []Mention, botID string) []string {
	if len(mentions) == 0 {
		return nil
	}
	result := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		if mention.ID == botID {
			result = append(result, mention.ID)
		}
	}
	return result
}

func chatTypeForRoom(room Room) string {
	if room.IsDirect {
		return "direct"
	}
	return "group"
}
