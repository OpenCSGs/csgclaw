package im

import (
	"csgclaw/internal/modelcap"
	"errors"
	"reflect"
	"slices"
)

var ErrInvalidContextUsage = errors.New("invalid context usage")

func cloneContextUsage(values map[string]modelcap.ContextUsage) map[string]modelcap.ContextUsage {
	if values == nil {
		return nil
	}
	result := make(map[string]modelcap.ContextUsage, len(values))
	for id, usage := range values {
		result[id] = *modelcap.CloneUsage(&usage)
	}
	return result
}

// RecordContextUsage saves the latest report in room metadata without
// rewriting the transcript or retaining per-turn history.
// It is called for accepted main-conversation reports, not work heartbeats.
func (s *Service) RecordContextUsage(roomID, userID string, usage modelcap.ContextUsage) (uint64, error) {
	if !usage.Valid() {
		return 0, ErrInvalidContextUsage
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	room, ok := s.rooms[roomID]
	if !ok {
		return 0, ErrRoomNotFound
	}
	userID = s.resolveRoomUserIDLocked(userID)
	if !slices.Contains(room.Members, userID) {
		return 0, ErrInviterNotRoomMember
	}
	if previous, ok := room.ContextUsage[userID]; ok && reflect.DeepEqual(previous, usage) {
		return room.ContextUsageEpoch, nil
	}
	previous := room.ContextUsage
	room.ContextUsage = cloneContextUsage(previous)
	if room.ContextUsage == nil {
		room.ContextUsage = make(map[string]modelcap.ContextUsage)
	}
	room.ContextUsage[userID] = *modelcap.CloneUsage(&usage)
	if s.statePath != "" {
		metadata := persistedBootstrap{CurrentUserID: s.currentUserID}
		for _, user := range s.users {
			metadata.Users = append(metadata.Users, user)
		}
		for _, current := range s.rooms {
			metadata.Rooms = append(metadata.Rooms, persistedRoomFromRoom(*current))
		}
		if err := writePersistedBootstrap(s.statePath, metadata); err != nil {
			room.ContextUsage = previous
			return 0, err
		}
	}
	return room.ContextUsageEpoch, nil
}
