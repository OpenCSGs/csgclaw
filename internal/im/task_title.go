package im

import (
	"strings"

	"csgclaw/internal/taskconfig"
)

const (
	titleSourcePlaceholder = "placeholder"
	titleSourceGenerated   = "generated"
	titleSourceManual      = "manual"
)

func initialTitleSource(req CreateRoomRequest) string {
	if req.AutoTitle && req.TaskSession && req.IsDirect && req.CloudAgent == nil {
		return titleSourcePlaceholder
	}
	return ""
}

// ClaimTaskTitle reserves the placeholder in memory while its title is generated.
func (s *Service) ClaimTaskTitle(roomID, messageID string) (string, *taskconfig.Config, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	room := s.rooms[roomID]
	if room == nil || room.TitleSource != titleSourcePlaceholder || s.taskTitleClaims[roomID] != "" {
		return "", nil, false
	}
	index := messageIndexByID(room.Messages, messageID)
	if index < 0 {
		return "", nil, false
	}
	message := room.Messages[index]
	question := strings.TrimSpace(message.Content)
	if message.SenderID != room.CreatedBy || message.Kind != MessageKindMessage ||
		(question == "" && len(message.Attachments) == 0) {
		return "", nil, false
	}

	s.taskTitleClaims[roomID] = messageID
	return question, taskconfig.Clone(room.Configuration), true
}

// ReleaseTaskTitle allows another completed turn to try after a failed attempt.
func (s *Service) ReleaseTaskTitle(roomID, messageID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.taskTitleClaims[roomID] == messageID {
		delete(s.taskTitleClaims, roomID)
	}
}

// TaskTitlePending reports whether this turn still owns the title reservation.
func (s *Service) TaskTitlePending(roomID, messageID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isPendingTaskTitleLocked(roomID, messageID)
}

func (s *Service) isPendingTaskTitleLocked(roomID, messageID string) bool {
	room := s.rooms[roomID]
	return room != nil && room.TitleSource == titleSourcePlaceholder && messageID != "" &&
		s.taskTitleClaims[roomID] == messageID && messageIndexByID(room.Messages, messageID) >= 0
}

// ApplyTaskTitle updates only the placeholder claimed by this turn.
func (s *Service) ApplyTaskTitle(roomID, messageID, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return ErrTitleRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isPendingTaskTitleLocked(roomID, messageID) {
		return nil
	}
	room := s.rooms[roomID]
	previous := *room
	room.Title = title
	room.TitleSource = titleSourceGenerated
	if err := s.saveTitleMetadataLocked(); err != nil {
		*room = previous
		return err
	}
	delete(s.taskTitleClaims, roomID)
	s.publishRoomEventLocked(EventTypeRoomUpdated, s.presentRoomLocked(*room, ""))
	return nil
}

func (s *Service) saveTitleMetadataLocked() error {
	if s.statePath == "" {
		return nil
	}
	state := persistedBootstrap{
		CurrentUserID: s.currentUserID,
		Users:         make([]User, 0, len(s.users)),
		Rooms:         make([]persistedRoom, 0, len(s.rooms)),
	}
	for _, user := range s.users {
		state.Users = append(state.Users, user)
	}
	for _, room := range s.rooms {
		state.Rooms = append(state.Rooms, persistedRoomFromRoom(*room))
	}
	return writePersistedBootstrap(s.statePath, state)
}
