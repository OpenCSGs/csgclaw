package im

import (
	"errors"
	"slices"
	"strings"

	"csgclaw/internal/taskmeta"
)

var ErrRoomTaskDispatchRequired = errors.New("ordinary mentions do not dispatch Workers in this on-demand room; use room_task_create, room_task_plan and room_task_dispatch, or room_task_message for an existing task")

// Check the destination room at the shared message boundary. CLI and MCP
// callers must get the same result, including runtimes without turn metadata.
// Task-scoped relays retain their existing assignment checks in the API bridge.
func (s *Service) validateRoomMessageDispatchLocked(room Room, senderID, content string, metadata map[string]any) error {
	if room.IsDirect || !room.IsOnDemand() || taskmeta.ID(metadata) != "" ||
		s.resolveRoomUserIDLocked(room.ManagerID) != senderID {
		return nil
	}
	for _, mention := range s.extractMentions(content) {
		user := s.users[mention.ID]
		if slices.Contains(room.Members, mention.ID) &&
			(strings.EqualFold(user.Role, "worker") || strings.EqualFold(user.Role, "agent")) {
			return ErrRoomTaskDispatchRequired
		}
	}
	return nil
}
