package im

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type ParticipantBridge struct {
	mu          sync.Mutex
	subscribers map[string]map[chan ParticipantEvent]struct{}
	pending     map[string][]ParticipantEvent
	inflight    map[string]map[string]ParticipantEvent
	seen        map[string]map[string]struct{}
}

const maxPendingParticipantEventsPerParticipant = 64

type ParticipantEvent struct {
	MessageID     string                    `json:"message_id"`
	RoomID        string                    `json:"room_id"`
	Channel       string                    `json:"channel,omitempty"`
	ChatID        string                    `json:"chat_id,omitempty"`
	ChatType      string                    `json:"chat_type"`
	Sender        ParticipantSender         `json:"sender"`
	SenderID      string                    `json:"sender_id,omitempty"`
	Text          string                    `json:"text"`
	Attachments   []MessageAttachment       `json:"attachments,omitempty"`
	Timestamp     string                    `json:"timestamp"`
	Mentions      []string                  `json:"mentions,omitempty"`
	ThreadRootID  string                    `json:"thread_root_id,omitempty"`
	ThreadContext *ParticipantThreadContext `json:"thread_context,omitempty"`
	Context       ParticipantMessageContext `json:"context,omitempty"`
}

type ParticipantSender struct {
	ID          string `json:"id"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
}

type ParticipantMessageContext struct {
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

type ParticipantThreadContext struct {
	RootMessageID string               `json:"root_message_id"`
	Context       []Message            `json:"context,omitempty"`
	Summary       ThreadContextSummary `json:"summary"`
}

type ParticipantSendMessageRequest struct {
	RoomID       string                     `json:"room_id"`
	ChatID       string                     `json:"chat_id,omitempty"`
	Text         string                     `json:"text"`
	Content      string                     `json:"content,omitempty"`
	MessageID    string                     `json:"message_id,omitempty"`
	ThreadRootID string                     `json:"thread_root_id,omitempty"`
	TopicID      string                     `json:"topic_id,omitempty"`
	Metadata     map[string]any             `json:"metadata,omitempty"`
	Context      *ParticipantMessageContext `json:"context,omitempty"`
	Attachments  []MessageAttachmentUpload  `json:"attachments,omitempty"`
}

func (r ParticipantSendMessageRequest) ResolvedRoomID() string {
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

func (r ParticipantSendMessageRequest) ResolvedText() string {
	if text := strings.TrimSpace(r.Text); text != "" {
		return r.Text
	}
	if content := strings.TrimSpace(r.Content); content != "" {
		return r.Content
	}
	return ""
}

func (r ParticipantSendMessageRequest) ResolvedThreadRootID() string {
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

func NewParticipantBridge(string) *ParticipantBridge {
	return &ParticipantBridge{
		subscribers: make(map[string]map[chan ParticipantEvent]struct{}),
		pending:     make(map[string][]ParticipantEvent),
		inflight:    make(map[string]map[string]ParticipantEvent),
		seen:        make(map[string]map[string]struct{}),
	}
}

func (b *ParticipantBridge) Subscribe(participantID string) (<-chan ParticipantEvent, func()) {
	participantID = canonicalIMParticipantID(participantID)
	ch := make(chan ParticipantEvent, 16)

	b.mu.Lock()
	if b.subscribers[participantID] == nil {
		b.subscribers[participantID] = make(map[chan ParticipantEvent]struct{})
	}
	b.subscribers[participantID][ch] = struct{}{}
	pending := append([]ParticipantEvent(nil), b.pending[participantID]...)
	delete(b.pending, participantID)

	for _, evt := range pending {
		select {
		case ch <- evt:
			b.markInflightLocked(participantID, evt)
		default:
			b.addPendingLocked(participantID, evt)
		}
	}
	b.mu.Unlock()

	var cancelOnce sync.Once
	cancel := func() {
		cancelOnce.Do(func() {
			b.mu.Lock()
			if subs, ok := b.subscribers[participantID]; ok {
				delete(subs, ch)
				if len(subs) == 0 {
					delete(b.subscribers, participantID)
				}
			}
			b.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancel
}

func (b *ParticipantBridge) SubscriberCount(participantID string) int {
	participantID = canonicalIMParticipantID(participantID)
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subscribers[participantID])
}

func (b *ParticipantBridge) PublishMessageEvent(room Room, sender User, message Message) []string {
	var missed []string
	for _, participantID := range room.Members {
		if !shouldNotifyParticipant(room, message, participantID) {
			continue
		}
		if !b.EnqueueMessageEvent(room, sender, message, participantID) {
			missed = append(missed, participantID)
		}
	}
	return missed
}

func (b *ParticipantBridge) EnqueueMessageEvent(room Room, sender User, message Message, participantID string) bool {
	participantID = canonicalIMParticipantID(participantID)
	if !shouldNotifyParticipant(room, message, participantID) {
		return true
	}
	return b.enqueue(participantID, messageEventForParticipant(room, sender, message, participantID))
}

func (b *ParticipantBridge) EnqueueMessageEventWithText(room Room, sender User, message Message, participantID string, text string) bool {
	participantID = canonicalIMParticipantID(participantID)
	if !shouldNotifyParticipant(room, message, participantID) {
		return true
	}
	evt := messageEventForParticipant(room, sender, message, participantID)
	evt.Text = participantActionTextForEvent(message, participantID, text)
	if messageMentionsParticipant(message, participantID) {
		ensureParticipantMentioned(&evt, participantID)
	}
	return b.enqueue(participantID, evt)
}

func (b *ParticipantBridge) enqueue(participantID string, evt ParticipantEvent) bool {
	participantID = canonicalIMParticipantID(participantID)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.hasSeenOrInflightLocked(participantID, evt.MessageID) {
		return true
	}
	subs := b.subscribers[participantID]
	if len(subs) == 0 {
		b.addPendingLocked(participantID, evt)
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
		b.markInflightLocked(participantID, evt)
		return true
	}
	b.addPendingLocked(participantID, evt)
	return false
}

func (b *ParticipantBridge) Ack(participantID, messageID string) {
	participantID = canonicalIMParticipantID(participantID)
	messageID = strings.TrimSpace(messageID)
	if participantID == "" || messageID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if inflight := b.inflight[participantID]; inflight != nil {
		delete(inflight, messageID)
		if len(inflight) == 0 {
			delete(b.inflight, participantID)
		}
	}
	b.removePendingLocked(participantID, messageID)
	b.markSeenLocked(participantID, messageID)
}

func (b *ParticipantBridge) Requeue(participantID string, evt ParticipantEvent) {
	participantID = canonicalIMParticipantID(participantID)
	if participantID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if messageID := strings.TrimSpace(evt.MessageID); messageID != "" {
		if inflight := b.inflight[participantID]; inflight != nil {
			delete(inflight, messageID)
			if len(inflight) == 0 {
				delete(b.inflight, participantID)
			}
		}
	}
	b.addPendingLocked(participantID, evt)
}

func (b *ParticipantBridge) addPendingLocked(participantID string, evt ParticipantEvent) {
	if b.hasSeenOrInflightLocked(participantID, evt.MessageID) || b.hasPendingLocked(participantID, evt.MessageID) {
		return
	}
	pending := append(b.pending[participantID], evt)
	if len(pending) > maxPendingParticipantEventsPerParticipant {
		pending = pending[len(pending)-maxPendingParticipantEventsPerParticipant:]
	}
	b.pending[participantID] = pending
}

func (b *ParticipantBridge) markInflightLocked(participantID string, evt ParticipantEvent) {
	messageID := strings.TrimSpace(evt.MessageID)
	if messageID == "" || b.hasSeenLocked(participantID, messageID) {
		return
	}
	if b.inflight[participantID] == nil {
		b.inflight[participantID] = make(map[string]ParticipantEvent)
	}
	b.inflight[participantID][messageID] = evt
	b.removePendingLocked(participantID, messageID)
}

func (b *ParticipantBridge) hasSeenOrInflightLocked(participantID, messageID string) bool {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false
	}
	if b.hasSeenLocked(participantID, messageID) {
		return true
	}
	_, ok := b.inflight[participantID][messageID]
	return ok
}

func (b *ParticipantBridge) hasPendingLocked(participantID, messageID string) bool {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return false
	}
	for _, evt := range b.pending[participantID] {
		if strings.TrimSpace(evt.MessageID) == messageID {
			return true
		}
	}
	return false
}

func (b *ParticipantBridge) removePendingLocked(participantID, messageID string) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return
	}
	pending := b.pending[participantID]
	for idx := 0; idx < len(pending); {
		if strings.TrimSpace(pending[idx].MessageID) == messageID {
			pending = append(pending[:idx], pending[idx+1:]...)
			continue
		}
		idx++
	}
	if len(pending) == 0 {
		delete(b.pending, participantID)
		return
	}
	b.pending[participantID] = pending
}

func (b *ParticipantBridge) hasSeenLocked(participantID, messageID string) bool {
	if messageID == "" {
		return false
	}
	_, ok := b.seen[participantID][messageID]
	return ok
}

func (b *ParticipantBridge) markSeenLocked(participantID, messageID string) {
	if messageID == "" {
		return
	}
	if b.seen[participantID] == nil {
		b.seen[participantID] = make(map[string]struct{})
	}
	b.seen[participantID][messageID] = struct{}{}
}

func messageEventForParticipant(room Room, sender User, message Message, participantID string) ParticipantEvent {
	threadRootID := threadRootID(message)
	chatType := chatTypeForRoom(room)
	mentions := mentionsForParticipant(message.Mentions, participantID)
	text := textForParticipantEvent(message, participantID)
	return ParticipantEvent{
		MessageID:    message.ID,
		RoomID:       room.ID,
		Channel:      "csgclaw",
		ChatID:       room.ID,
		ChatType:     chatType,
		ThreadRootID: threadRootID,
		Sender: ParticipantSender{
			ID:          sender.ID,
			Username:    sender.Name,
			DisplayName: sender.Name,
			Description: sender.Description,
		},
		SenderID:      sender.ID,
		Text:          text,
		Attachments:   cloneMessageAttachments(message.Attachments),
		Timestamp:     fmt.Sprintf("%d", message.CreatedAt.UnixMilli()),
		Mentions:      mentions,
		ThreadContext: participantThreadContext(room, threadRootID),
		Context: ParticipantMessageContext{
			Channel:   "csgclaw",
			Account:   strings.TrimSpace(participantID),
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

func textForParticipantEvent(message Message, participantID string) string {
	content := message.Content
	userID := userIDForParticipantID(participantID)
	if content == "" || userID == "" || HasMentionTagForUser(content, userID) {
		return content
	}
	for _, mention := range message.Mentions {
		if canonicalIMUserID(mention.ID) == userID {
			return replaceMentionNameWithTag(content, mentionForUserID(mention, userID))
		}
	}
	return content
}

func participantActionTextForEvent(message Message, participantID, text string) string {
	text = strings.TrimSpace(text)
	userID := userIDForParticipantID(participantID)
	if text == "" || userID == "" || HasMentionTagForUser(text, userID) || !messageMentionsParticipant(message, participantID) {
		return text
	}
	return text + " " + mentionTagForParticipant(message, participantID)
}

func messageMentionsParticipant(message Message, participantID string) bool {
	userID := userIDForParticipantID(participantID)
	if userID == "" {
		return false
	}
	for _, mention := range message.Mentions {
		if canonicalIMUserID(mention.ID) == userID {
			return true
		}
	}
	return HasMentionTagForUser(message.Content, userID)
}

func ensureParticipantMentioned(evt *ParticipantEvent, participantID string) {
	if evt == nil {
		return
	}
	userID := userIDForParticipantID(participantID)
	if userID == "" {
		return
	}
	for _, mention := range evt.Mentions {
		if canonicalIMUserID(mention) == userID {
			evt.Context.Mentioned = true
			return
		}
	}
	evt.Mentions = append(evt.Mentions, userID)
	evt.Context.Mentioned = true
}

func mentionTagForParticipant(message Message, participantID string) string {
	userID := userIDForParticipantID(participantID)
	for _, mention := range message.Mentions {
		if canonicalIMUserID(mention.ID) == userID {
			return fmt.Sprintf(`<at user_id="%s">%s</at>`, strings.TrimSpace(userID), mentionDisplayName(mention))
		}
	}
	return fmt.Sprintf(`<at user_id="%s">%s</at>`, strings.TrimSpace(userID), strings.TrimSpace(userID))
}

func replaceMentionNameWithTag(content string, mention Mention) string {
	candidates := mentionNameCandidates(mention)
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
		name := strings.ToLower(strings.TrimSpace(content[match[4]:match[5]]))
		if _, ok := candidates[name]; !ok {
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

func mentionNameCandidates(mention Mention) map[string]struct{} {
	candidates := make(map[string]struct{}, 2)
	if name := normalizeMentionName(mention.Name); name != "" {
		candidates[name] = struct{}{}
	}
	if idName := trimLocalIdentityPrefixes(mention.ID); idName != "" {
		candidates[strings.ToLower(idName)] = struct{}{}
	}
	return candidates
}

func normalizeMentionName(value string) string {
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

func participantThreadContext(room Room, rootMessageID string) *ParticipantThreadContext {
	rootMessageID = strings.TrimSpace(rootMessageID)
	if rootMessageID == "" {
		return nil
	}
	state, ok := threadStateByRoot(room.Threads, rootMessageID)
	if !ok {
		return nil
	}
	return &ParticipantThreadContext{
		RootMessageID: rootMessageID,
		Context:       cloneMessages(state.Context),
		Summary:       state.Summary,
	}
}

func (e ParticipantEvent) MarshalJSONLine() ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func shouldNotifyParticipant(room Room, message Message, participantID string) bool {
	userID := userIDForParticipantID(participantID)
	if canonicalIMUserID(message.SenderID) == userID {
		return false
	}
	if !containsUserIDInRoom(room, participantID) {
		return false
	}
	return true
}

func mentionsForParticipant(mentions []Mention, participantID string) []string {
	if len(mentions) == 0 {
		return nil
	}
	userID := userIDForParticipantID(participantID)
	result := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		if canonicalIMUserID(mention.ID) == userID {
			result = append(result, userID)
		}
	}
	return result
}

func mentionForUserID(mention Mention, userID string) Mention {
	mention.ID = canonicalIMUserID(userID)
	return mention
}

func chatTypeForRoom(room Room) string {
	if room.IsDirect {
		return "direct"
	}
	return "group"
}
