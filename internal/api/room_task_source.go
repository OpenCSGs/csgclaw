package api

import "csgclaw/internal/taskmeta"

// Stops refer to the exact work lease's source message, not just an agent.
func (h *Handler) roomTaskIDForSource(roomID string, value any) string {
	source, _ := value.(string)
	if source == "" || h.roomTaskSvc == nil {
		return ""
	}
	if id := h.roomTaskSvc.SourceTaskID(roomID, source); id != "" {
		return id
	}
	if room, ok := h.im.Room(roomID); ok {
		for _, m := range room.Messages {
			if m.ID == source {
				return taskmeta.ID(m.Metadata)
			}
		}
	}
	return ""
}

func (h *Handler) roomTaskAttemptForSource(roomID string, value any) int {
	source, _ := value.(string)
	if source == "" || h.roomTaskSvc == nil {
		return 0
	}
	if attempt := h.roomTaskSvc.SourceAttempt(roomID, source); attempt > 0 {
		return attempt
	}
	if room, ok := h.im.Room(roomID); ok {
		for _, m := range room.Messages {
			if m.ID == source {
				return taskmeta.Attempt(m.Metadata)
			}
		}
	}
	return 0
}
