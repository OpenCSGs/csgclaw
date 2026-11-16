package im

import (
	"crypto/rand"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/utils"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

func cloneAutonomousState(state *apitypes.AutonomousState) *apitypes.AutonomousState {
	if state == nil {
		return nil
	}
	copy := *state
	copy.Receipts = append([]string(nil), state.Receipts...)
	if state.Cursors != nil {
		copy.Cursors = make(map[string]apitypes.AutonomousCursor, len(state.Cursors))
		for key, cursor := range state.Cursors {
			if cursor.Page != nil {
				page := *cursor.Page
				cursor.Page = &page
			}
			copy.Cursors[key] = cursor
		}
	}
	return &copy
}
func humanRole(role string) bool { return role == "admin" || role == "human" || role == "user" }
func publicAutonomousMessage(message Message) bool {
	return message.Event == nil && (message.Kind == MessageKindMessage || message.Kind == "") && !IsAgentActivityMessage(message) &&
		(strings.TrimSpace(strings.ReplaceAll(message.Content, "\u200b", "")) != "" || len(message.Attachments) > 0)
}

// AutonomousTaskSource resolves a human request in the bound conversation.
// A thread can also refer to its root message. When that happens the root
// belongs to the same task as in the main chat.
func (s *Service) AutonomousTaskSource(roomID, thread, sourceID string) (Message, User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	room, ok := s.rooms[roomID]
	if !ok || !room.IsAutonomous() {
		return Message{}, User{}, ErrRoomNotFound
	}
	for _, message := range room.Messages {
		if message.ID != sourceID || (threadRootID(message) != thread && message.ID != thread) {
			continue
		}
		sender := s.users[message.SenderID]
		if publicAutonomousMessage(message) && humanRole(sender.Role) {
			return PublicMessageView(message), sender, nil
		}
		break
	}
	return Message{}, User{}, fmt.Errorf("source_message_id must identify a human request in this conversation")
}

// PublicMessageView omits the private execution trace attached to legacy final
// replies. A public body must not expose another member's tool history.
func PublicMessageView(message Message) Message {
	message = cloneMessage(message)
	message.Metadata = utils.CloneAnyMapShallowNestedStringMaps(message.Metadata)
	message.Thread = nil
	for _, namespace := range []string{"csgclaw", "codex"} {
		if metadata, ok := message.Metadata[namespace].(map[string]any); ok {
			delete(metadata, "turn_progress")
		}
	}
	return message
}

func autonomousCursorKey(actor, thread string) string {
	return canonicalIMParticipantID(actor) + "\x00" + thread
}

// Sequences survive edits, member deletion and restart. Only public messages
// consume a sequence; private progress never becomes unread work.
func sequenceAutonomousMessages(room *Room) {
	if !room.IsAutonomous() {
		return
	}
	if room.AutonomousState == nil {
		room.AutonomousState = &apitypes.AutonomousState{}
	}
	for _, message := range room.Messages {
		if message.RoomSequence > room.AutonomousState.Sequence {
			room.AutonomousState.Sequence = message.RoomSequence
		}
	}
	for i := range room.Messages {
		if publicAutonomousMessage(room.Messages[i]) && room.Messages[i].RoomSequence == 0 {
			room.AutonomousState.Sequence++
			room.Messages[i].RoomSequence = room.AutonomousState.Sequence
		}
	}
}
func assignAutonomousSequence(room *Room, message *Message) {
	if !room.IsAutonomous() {
		return
	}
	sequenceAutonomousMessages(room)
	if publicAutonomousMessage(*message) && message.RoomSequence == 0 {
		room.AutonomousState.Sequence++
		message.RoomSequence = room.AutonomousState.Sequence
	}
}
func (s *Service) latestAutonomousHuman(room *Room) string {
	latest := ""
	for _, message := range room.Messages {
		if publicAutonomousMessage(message) && humanRole(s.users[message.SenderID].Role) {
			latest = message.ID
		}
	}
	return latest
}
func autonomousUnread(room *Room, actor, thread string, after uint64) []Message {
	messages := []Message{}
	for _, message := range room.Messages {
		if message.RoomSequence > after && publicAutonomousMessage(message) && threadRootID(message) == thread && canonicalIMUserID(message.SenderID) != canonicalIMUserID(actor) {
			messages = append(messages, message)
		}
	}
	sort.SliceStable(messages, func(i, j int) bool { return messages[i].RoomSequence < messages[j].RoomSequence })
	return messages
}

// Admission remembers the offered unread watermark, not a turn budget. Reading
// is separate: silence leaves messages unread but cannot wake the same batch again.
func (s *Service) AdmitAutonomousNotification(roomID, sourceID, participantID string) (apitypes.UnreadNotification, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[roomID]
	if !ok {
		return apitypes.UnreadNotification{}, false, ErrRoomNotFound
	}
	if !room.IsAutonomous() {
		return apitypes.UnreadNotification{}, true, nil
	}
	if !containsUserIDInRoom(*room, participantID) {
		return apitypes.UnreadNotification{}, false, nil
	}
	previous := cloneAutonomousState(room.AutonomousState)
	sequenceAutonomousMessages(room)
	var source *Message
	for i := range room.Messages {
		if room.Messages[i].ID == sourceID {
			source = &room.Messages[i]
			break
		}
	}
	if source == nil || !publicAutonomousMessage(*source) || canonicalIMUserID(source.SenderID) == canonicalIMUserID(participantID) {
		return apitypes.UnreadNotification{}, false, nil
	}
	state := room.AutonomousState
	resumed := false
	latestHuman := s.latestAutonomousHuman(room)
	if state.SourceID != latestHuman {
		resumed = state.Paused
		state.SourceID = latestHuman
		state.Paused = false
	}
	if state.Paused {
		return apitypes.UnreadNotification{}, false, nil
	}
	thread := threadRootID(*source)
	key := autonomousCursorKey(participantID, thread)
	if state.Cursors == nil {
		state.Cursors = map[string]apitypes.AutonomousCursor{}
	}
	cursor, exists := state.Cursors[key]
	if !exists {
		// A new member starts at the first notification it actually receives. The
		// ingress buffer retains that earliest source while coalescing a burst.
		cursor.ReadThrough = source.RoomSequence - 1
		// Old releases persisted per-message receipts. Preserve their replay guard.
		for _, receipt := range state.Receipts {
			for _, message := range room.Messages {
				if receipt == message.ID+":"+canonicalIMParticipantID(participantID) && threadRootID(message) == thread && message.RoomSequence > cursor.NotifiedThrough {
					cursor.NotifiedThrough = message.RoomSequence
				}
			}
		}
	}
	unread := autonomousUnread(room, participantID, thread, cursor.ReadThrough)
	if len(unread) == 0 || unread[len(unread)-1].RoomSequence <= cursor.NotifiedThrough {
		if resumed {
			if err := s.saveRoomLocked(*room); err != nil {
				room.AutonomousState = previous
				return apitypes.UnreadNotification{}, false, err
			}
			s.publishRoomEventLocked(EventTypeRoomUpdated, s.presentRoomLocked(*room, room.Locale))
		}
		return apitypes.UnreadNotification{}, false, nil
	}
	cursor.NotifiedThrough = unread[len(unread)-1].RoomSequence
	state.Cursors[key] = cursor
	if err := s.saveRoomLocked(*room); err != nil {
		room.AutonomousState = previous
		return apitypes.UnreadNotification{}, false, fmt.Errorf("persist unread notification: %w", err)
	}
	if resumed {
		s.publishRoomEventLocked(EventTypeRoomUpdated, s.presentRoomLocked(*room, room.Locale))
	}
	return apitypes.UnreadNotification{RoomID: roomID, ParticipantID: canonicalIMParticipantID(participantID), ThreadRootID: thread, Count: len(unread), Through: cursor.NotifiedThrough}, true, nil
}

func (s *Service) SetAutonomousTaskThreadResolver(resolve func(roomID, sourceID string) string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autonomousTaskThread = resolve
}

// AutonomousTaskThread finds the topic of the specific request that triggered a
// turn. Call the task store outside the IM lock.
func (s *Service) AutonomousTaskThread(roomID, sourceID string) string {
	if sourceID == "" {
		return ""
	}
	s.mu.RLock()
	resolve := s.autonomousTaskThread
	room := s.rooms[roomID]
	autonomous := room != nil && room.IsAutonomous()
	s.mu.RUnlock()
	if !autonomous || resolve == nil {
		return ""
	}
	return resolve(roomID, sourceID)
}

// HasAutonomousReply checks whether the participant already published the same
// answer to this room and thread during the current turn.
func (s *Service) HasAutonomousReply(roomID, participantID, thread, excludeID, content string, startedAt time.Time) bool {
	content = strings.TrimSpace(content)
	if content == "" || startedAt.IsZero() {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	room := s.rooms[roomID]
	if room == nil {
		return false
	}
	senderID := canonicalIMUserID(participantID)
	for _, message := range room.Messages {
		if !message.CreatedAt.Before(startedAt) && message.ID != excludeID && publicAutonomousMessage(message) &&
			canonicalIMUserID(message.SenderID) == senderID && threadRootID(message) == thread &&
			strings.TrimSpace(message.Content) == content {
			return true
		}
	}
	return false
}

func (s *Service) unreadRoomLocked(roomID, participantID, thread string) (*Room, error) {
	room, ok := s.rooms[roomID]
	if !ok {
		return nil, ErrRoomNotFound
	}
	if !room.IsAutonomous() {
		return nil, fmt.Errorf("unread consumption requires an autonomous room")
	}
	if !containsUserIDInRoom(*room, participantID) {
		return nil, fmt.Errorf("participant is not a room member")
	}
	if thread != "" {
		found := false
		for _, message := range room.Messages {
			if message.ID == thread && threadRootID(message) == "" {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("thread root not found")
		}
	}
	return room, nil
}

// PendingAutonomousNotification snapshots new unread work without advancing
// either cursor. A notice accepted by a running model is not a read receipt.
func (s *Service) PendingAutonomousNotification(roomID, participantID, thread string, after uint64) (apitypes.UnreadNotification, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	room, err := s.unreadRoomLocked(roomID, participantID, thread)
	if err != nil || room.AutonomousState == nil || room.AutonomousState.Paused {
		return apitypes.UnreadNotification{}, false
	}
	// A related task topic can receive messages before its first turn is admitted.
	cursor := room.AutonomousState.Cursors[autonomousCursorKey(participantID, thread)]
	unread := autonomousUnread(room, participantID, thread, cursor.ReadThrough)
	if len(unread) == 0 {
		return apitypes.UnreadNotification{}, false
	}
	through := unread[len(unread)-1].RoomSequence
	if through <= max(after, cursor.NotifiedThrough) {
		return apitypes.UnreadNotification{}, false
	}
	return apitypes.UnreadNotification{
		RoomID: roomID, ParticipantID: canonicalIMParticipantID(participantID), ThreadRootID: thread,
		Count: len(unread), Through: through,
	}, true
}

// PrepareUnread never advances the read cursor. CLI stdout must succeed before
// AcknowledgeUnread; tool clients likewise acknowledge only a received page.
func (s *Service) PrepareUnread(roomID, participantID, thread string, limit int, peek bool) (apitypes.UnreadPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, err := s.unreadRoomLocked(roomID, participantID, thread)
	if err != nil {
		return apitypes.UnreadPage{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	previous := cloneAutonomousState(room.AutonomousState)
	sequenceAutonomousMessages(room)
	state := room.AutonomousState
	key := autonomousCursorKey(participantID, thread)
	cursor := state.Cursors[key]
	unread := autonomousUnread(room, participantID, thread, cursor.ReadThrough)
	page := apitypes.UnreadPage{UnreadNotification: apitypes.UnreadNotification{RoomID: roomID, ParticipantID: canonicalIMParticipantID(participantID), ThreadRootID: thread, Count: len(unread), Through: cursor.ReadThrough}, Messages: []apitypes.UnreadMessage{}}
	present := func(message Message) apitypes.UnreadMessage {
		user := s.users[message.SenderID]
		return apitypes.UnreadMessage{Message: PublicMessageView(s.presentMessageLocked(*room, message, "")), Sender: apitypes.UnreadSender{ID: message.SenderID, Name: user.Name, Role: user.Role}}
	}
	if thread != "" {
		for _, message := range room.Messages {
			if message.ID == thread {
				root := present(message)
				page.Root = &root
				break
			}
		}
	}
	existing := !peek && cursor.Page != nil && time.Now().Before(cursor.Page.ExpiresAt)
	bytes := 0
	for _, message := range unread {
		if existing {
			if message.RoomSequence > cursor.Page.Through {
				break
			}
		} else if len(page.Messages) >= limit || (len(page.Messages) > 0 && bytes >= 512*1024) {
			break
		}
		visible := present(message)
		raw, _ := json.Marshal(visible)
		bytes += len(raw)
		page.Messages = append(page.Messages, visible)
		page.Through = message.RoomSequence
	}
	page.HasMore = len(page.Messages) < len(unread)
	if peek || (len(page.Messages) == 0 && !existing) {
		return page, nil
	}
	if existing {
		page.Token = cursor.Page.Token
		page.Through = cursor.Page.Through
	} else {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return apitypes.UnreadPage{}, err
		}
		page.Token = hex.EncodeToString(nonce[:])
		cursor.Page = &apitypes.UnreadPageReceipt{Token: page.Token, Through: page.Through, ExpiresAt: time.Now().Add(5 * time.Minute)}
	}
	if state.Cursors == nil {
		state.Cursors = map[string]apitypes.AutonomousCursor{}
	}
	state.Cursors[key] = cursor
	if err := s.saveRoomLocked(*room); err != nil {
		room.AutonomousState = previous
		return apitypes.UnreadPage{}, err
	}
	return page, nil
}
func (s *Service) AcknowledgeUnread(request apitypes.UnreadAck) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, err := s.unreadRoomLocked(request.RoomID, request.ParticipantID, request.ThreadRootID)
	if err != nil {
		return err
	}
	if room.AutonomousState == nil || request.Token == "" {
		return fmt.Errorf("unread page token is required")
	}
	key := autonomousCursorKey(request.ParticipantID, request.ThreadRootID)
	cursor := room.AutonomousState.Cursors[key]
	if cursor.LastAck == request.Token {
		return nil
	}
	if cursor.Page == nil || cursor.Page.Token != request.Token || time.Now().After(cursor.Page.ExpiresAt) {
		return fmt.Errorf("unread page expired or token does not match; read again")
	}
	previous := cloneAutonomousState(room.AutonomousState)
	if cursor.Page.Through > cursor.ReadThrough {
		cursor.ReadThrough = cursor.Page.Through
	}
	cursor.LastAck = request.Token
	cursor.Page = nil
	room.AutonomousState.Cursors[key] = cursor
	if err := s.saveRoomLocked(*room); err != nil {
		room.AutonomousState = previous
		return err
	}
	return nil
}

// AutonomousPauseWatermark captures the latest human message before a stop
// request can block on the runtime.
func (s *Service) AutonomousPauseWatermark(roomID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	room := s.rooms[roomID]
	if room == nil || !room.IsAutonomous() {
		return "", false
	}
	return s.latestAutonomousHuman(room), true
}

// PauseAutonomousRoom applies a successful stop only if no new human message
// arrived while the runtime was canceling the turn.
func (s *Service) PauseAutonomousRoom(roomID, sourceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[roomID]
	if !ok {
		return ErrRoomNotFound
	}
	if !room.IsAutonomous() || s.latestAutonomousHuman(room) != sourceID {
		return nil
	}
	previous := cloneAutonomousState(room.AutonomousState)
	sequenceAutonomousMessages(room)
	room.AutonomousState.SourceID = sourceID
	room.AutonomousState.Paused = true
	if err := s.saveRoomLocked(*room); err != nil {
		room.AutonomousState = previous
		return err
	}
	s.publishRoomEventLocked(EventTypeRoomUpdated, s.presentRoomLocked(*room, room.Locale))
	return nil
}
