package im

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type BotBridge struct {
	mu          sync.Mutex
	subscribers map[string]map[chan BotEvent]struct{}
	pending     map[string][]BotEvent
	inflight    map[string]map[string]BotEvent
	seen        map[string]map[string]struct{}
}

const maxPendingBotEventsPerBot = 64

type BotEvent struct {
	MessageID     string            `json:"message_id"`
	RoomID        string            `json:"room_id"`
	Channel       string            `json:"channel,omitempty"`
	ChatID        string            `json:"chat_id,omitempty"`
	ChatType      string            `json:"chat_type"`
	Sender        BotSender         `json:"sender"`
	SenderID      string            `json:"sender_id,omitempty"`
	Text          string            `json:"text"`
	Timestamp     string            `json:"timestamp"`
	Mentions      []string          `json:"mentions,omitempty"`
	ThreadRootID  string            `json:"thread_root_id,omitempty"`
	ThreadContext *BotThreadContext `json:"thread_context,omitempty"`
	Context       BotMessageContext `json:"context,omitempty"`
}

type BotSender struct {
	ID          string `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

type BotMessageContext struct {
	Channel          string            `json:"channel,omitempty"`
	Account          string            `json:"account,omitempty"`
	ChatID           string            `json:"chat_id,omitempty"`
	ChatType         string            `json:"chat_type,omitempty"`
	TopicID          string            `json:"topic_id,omitempty"`
	SpaceID          string            `json:"space_id,omitempty"`
	SpaceType        string            `json:"space_type,omitempty"`
	SenderID         string            `json:"sender_id,omitempty"`
	MessageID        string            `json:"message_id,omitempty"`
	Mentioned        bool              `json:"mentioned,omitempty"`
	ReplyToMessageID string            `json:"reply_to_message_id,omitempty"`
	ReplyToSenderID  string            `json:"reply_to_sender_id,omitempty"`
	ReplyHandles     map[string]string `json:"reply_handles,omitempty"`
	Raw              map[string]string `json:"raw,omitempty"`
}

type BotThreadContext struct {
	RootMessageID string               `json:"root_message_id"`
	Context       []Message            `json:"context,omitempty"`
	Summary       ThreadContextSummary `json:"summary"`
}

type BotSendMessageRequest struct {
	RoomID       string             `json:"room_id"`
	ChatID       string             `json:"chat_id,omitempty"`
	Text         string             `json:"text"`
	Content      string             `json:"content,omitempty"`
	MessageID    string             `json:"message_id,omitempty"`
	ThreadRootID string             `json:"thread_root_id,omitempty"`
	TopicID      string             `json:"topic_id,omitempty"`
	Context      *BotMessageContext `json:"context,omitempty"`
}

func (r BotSendMessageRequest) ResolvedRoomID() string {
	if roomID := strings.TrimSpace(r.RoomID); roomID != "" {
		return roomID
	}
	if chatID := strings.TrimSpace(r.ChatID); chatID != "" {
		return chatID
	}
	if r.Context != nil {
		return strings.TrimSpace(r.Context.ChatID)
	}
	return ""
}

func (r BotSendMessageRequest) ResolvedText() string {
	if text := strings.TrimSpace(r.Text); text != "" {
		return r.Text
	}
	if content := strings.TrimSpace(r.Content); content != "" {
		return r.Content
	}
	return ""
}

func (r BotSendMessageRequest) ResolvedThreadRootID() string {
	if rootID := strings.TrimSpace(r.ThreadRootID); rootID != "" {
		return rootID
	}
	if topicID := strings.TrimSpace(r.TopicID); topicID != "" {
		return topicID
	}
	if r.Context != nil {
		return strings.TrimSpace(r.Context.TopicID)
	}
	return ""
}

func NewBotBridge(string) *BotBridge {
	return &BotBridge{
		subscribers: make(map[string]map[chan BotEvent]struct{}),
		pending:     make(map[string][]BotEvent),
		inflight:    make(map[string]map[string]BotEvent),
		seen:        make(map[string]map[string]struct{}),
	}
}

func (b *BotBridge) Subscribe(botID string) (<-chan BotEvent, func()) {
	ch := make(chan BotEvent, 16)

	b.mu.Lock()
	if b.subscribers[botID] == nil {
		b.subscribers[botID] = make(map[chan BotEvent]struct{})
	}
	b.subscribers[botID][ch] = struct{}{}
	pending := append([]BotEvent(nil), b.pending[botID]...)
	delete(b.pending, botID)

	for _, evt := range pending {
		select {
		case ch <- evt:
			b.markInflightLocked(botID, evt)
		default:
			b.addPendingLocked(botID, evt)
		}
	}
	b.mu.Unlock()

	var cancelOnce sync.Once
	cancel := func() {
		cancelOnce.Do(func() {
			b.mu.Lock()
			if subs, ok := b.subscribers[botID]; ok {
				delete(subs, ch)
				if len(subs) == 0 {
					delete(b.subscribers, botID)
				}
			}
			b.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancel
}

func (b *BotBridge) SubscriberCount(botID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subscribers[botID])
}

func (b *BotBridge) PublishMessageEvent(room Room, sender User, message Message) []string {
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

func (b *BotBridge) EnqueueMessageEvent(room Room, sender User, message Message, botID string) bool {
	if !shouldNotifyBot(room, message, botID) {
		return true
	}
	return b.enqueue(botID, messageEventForBot(room, sender, message, botID))
}

func (b *BotBridge) enqueue(botID string, evt BotEvent) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.hasSeenOrInflightLocked(botID, evt.MessageID) {
		return true
	}
	subs := b.subscribers[botID]
	if len(subs) == 0 {
		b.addPendingLocked(botID, evt)
		return false
	}

	sent := false
	for ch := range subs {
		select {
		case ch <- evt:
			sent = true
		default:
		}
	}
	if sent {
		b.markInflightLocked(botID, evt)
		return true
	}
	b.addPendingLocked(botID, evt)
	return false
}

func (b *BotBridge) Ack(botID, messageID string) {
	botID = strings.TrimSpace(botID)
	messageID = strings.TrimSpace(messageID)
	if botID == "" || messageID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if inflight := b.inflight[botID]; inflight != nil {
		delete(inflight, messageID)
		if len(inflight) == 0 {
			delete(b.inflight, botID)
		}
	}
	b.removePendingLocked(botID, messageID)
	b.markSeenLocked(botID, messageID)
}

func (b *BotBridge) Requeue(botID string, evt BotEvent) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if messageID := strings.TrimSpace(evt.MessageID); messageID != "" {
		if inflight := b.inflight[botID]; inflight != nil {
			delete(inflight, messageID)
			if len(inflight) == 0 {
				delete(b.inflight, botID)
			}
		}
	}
	b.addPendingLocked(botID, evt)
}

func (b *BotBridge) addPendingLocked(botID string, evt BotEvent) {
	if b.hasSeenOrInflightLocked(botID, evt.MessageID) || b.hasPendingLocked(botID, evt.MessageID) {
		return
	}
	pending := append(b.pending[botID], evt)
	if len(pending) > maxPendingBotEventsPerBot {
		pending = pending[len(pending)-maxPendingBotEventsPerBot:]
	}
	b.pending[botID] = pending
}

func (b *BotBridge) markInflightLocked(botID string, evt BotEvent) {
	messageID := strings.TrimSpace(evt.MessageID)
	if messageID == "" || b.hasSeenLocked(botID, messageID) {
		return
	}
	if b.inflight[botID] == nil {
		b.inflight[botID] = make(map[string]BotEvent)
	}
	b.inflight[botID][messageID] = evt
	b.removePendingLocked(botID, messageID)
}

func (b *BotBridge) hasSeenOrInflightLocked(botID, messageID string) bool {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false
	}
	if b.hasSeenLocked(botID, messageID) {
		return true
	}
	_, ok := b.inflight[botID][messageID]
	return ok
}

func (b *BotBridge) hasPendingLocked(botID, messageID string) bool {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false
	}
	for _, evt := range b.pending[botID] {
		if strings.TrimSpace(evt.MessageID) == messageID {
			return true
		}
	}
	return false
}

func (b *BotBridge) removePendingLocked(botID, messageID string) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return
	}
	pending := b.pending[botID]
	for idx := 0; idx < len(pending); {
		if strings.TrimSpace(pending[idx].MessageID) == messageID {
			pending = append(pending[:idx], pending[idx+1:]...)
			continue
		}
		idx++
	}
	if len(pending) == 0 {
		delete(b.pending, botID)
		return
	}
	b.pending[botID] = pending
}

func (b *BotBridge) hasSeenLocked(botID, messageID string) bool {
	if messageID == "" {
		return false
	}
	_, ok := b.seen[botID][messageID]
	return ok
}

func (b *BotBridge) markSeenLocked(botID, messageID string) {
	if messageID == "" {
		return
	}
	if b.seen[botID] == nil {
		b.seen[botID] = make(map[string]struct{})
	}
	b.seen[botID][messageID] = struct{}{}
}

func messageEventForBot(room Room, sender User, message Message, botID string) BotEvent {
	threadRootID := threadRootID(message)
	chatType := chatTypeForRoom(room)
	mentions := mentionsForBot(message.Mentions, botID)
	text := textForBotEvent(message, botID)
	return BotEvent{
		MessageID:    message.ID,
		RoomID:       room.ID,
		Channel:      "csgclaw",
		ChatID:       room.ID,
		ChatType:     chatType,
		ThreadRootID: threadRootID,
		Sender: BotSender{
			ID:          sender.ID,
			Username:    sender.Handle,
			DisplayName: sender.Name,
		},
		SenderID:      sender.ID,
		Text:          text,
		Timestamp:     fmt.Sprintf("%d", message.CreatedAt.UnixMilli()),
		Mentions:      mentions,
		ThreadContext: botThreadContext(room, threadRootID),
		Context: BotMessageContext{
			Channel:   "csgclaw",
			Account:   strings.TrimSpace(botID),
			ChatID:    room.ID,
			ChatType:  chatType,
			TopicID:   threadRootID,
			SenderID:  sender.ID,
			MessageID: message.ID,
			Mentioned: len(mentions) > 0,
			Raw: map[string]string{
				"room_id":        room.ID,
				"thread_root_id": threadRootID,
			},
		},
	}
}

func textForBotEvent(message Message, botID string) string {
	content := message.Content
	botID = strings.TrimSpace(botID)
	if content == "" || botID == "" || hasMentionTagForUser(content, botID) {
		return content
	}
	for _, mention := range message.Mentions {
		if strings.TrimSpace(mention.ID) == botID {
			return replaceMentionHandleWithTag(content, mention)
		}
	}
	return content
}

func hasMentionTagForUser(content, userID string) bool {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false
	}
	for _, match := range mentionTagPattern.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 && strings.TrimSpace(match[1]) == userID {
			return true
		}
	}
	return false
}

func replaceMentionHandleWithTag(content string, mention Mention) string {
	candidates := mentionHandleCandidates(mention)
	if len(candidates) == 0 {
		return content
	}
	tag := fmt.Sprintf(`<at user_id="%s">%s</at>`, strings.TrimSpace(mention.ID), mentionDisplayName(mention))
	matches := mentionPattern.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content
	}

	var out strings.Builder
	last := 0
	replaced := false
	for _, match := range matches {
		if len(match) < 6 || match[4] < 0 || match[5] < 0 {
			continue
		}
		handle := strings.ToLower(strings.TrimSpace(content[match[4]:match[5]]))
		if _, ok := candidates[handle]; !ok {
			continue
		}
		replaced = true
		out.WriteString(content[last:match[0]])
		if match[2] >= 0 && match[3] >= 0 {
			out.WriteString(content[match[2]:match[3]])
		}
		out.WriteString(tag)
		last = match[1]
	}
	if !replaced {
		return content
	}
	out.WriteString(content[last:])
	return out.String()
}

func mentionHandleCandidates(mention Mention) map[string]struct{} {
	candidates := make(map[string]struct{}, 2)
	if name := normalizeMentionHandle(mention.Name); name != "" {
		candidates[name] = struct{}{}
	}
	if idHandle := strings.TrimPrefix(strings.TrimSpace(mention.ID), "u-"); idHandle != "" {
		candidates[strings.ToLower(idHandle)] = struct{}{}
	}
	return candidates
}

func normalizeMentionHandle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "@")
	return value
}

func mentionDisplayName(mention Mention) string {
	if name := strings.TrimSpace(mention.Name); name != "" {
		return name
	}
	if id := strings.TrimSpace(mention.ID); id != "" {
		return id
	}
	return "user"
}

func botThreadContext(room Room, rootMessageID string) *BotThreadContext {
	rootMessageID = strings.TrimSpace(rootMessageID)
	if rootMessageID == "" {
		return nil
	}
	state, ok := threadStateByRoot(room.Threads, rootMessageID)
	if !ok {
		return nil
	}
	return &BotThreadContext{
		RootMessageID: rootMessageID,
		Context:       cloneMessages(state.Context),
		Summary:       state.Summary,
	}
}

func (e BotEvent) MarshalJSONLine() ([]byte, error) {
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
