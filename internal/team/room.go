package team

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// EventRoomID returns the IM room that should receive projections for a task-scoped event.
func EventRoomID(meta TeamMeta, task *TeamTask) string {
	if task != nil {
		if roomID := strings.TrimSpace(task.RoomID); roomID != "" {
			return roomID
		}
	}
	return strings.TrimSpace(meta.RoomID)
}

// TaskExecutionRoomTitle formats a human-readable execution room title with task id.
func TaskExecutionRoomTitle(task TeamTask) string {
	taskID := strings.TrimSpace(task.ID)
	if taskID == "" {
		taskID = "task"
	}
	title := strings.TrimSpace(task.Title)
	if title == "" {
		return fmt.Sprintf("[%s]", taskID)
	}
	if utf8.RuneCountInString(title) > 48 {
		title = string([]rune(title)[:45]) + "..."
	}
	return fmt.Sprintf("[%s] %s", taskID, title)
}

// ExecutionRoomBound reports whether the task already has a dedicated execution room.
func ExecutionRoomBound(task TeamTask, meta TeamMeta) bool {
	roomID := strings.TrimSpace(task.RoomID)
	teamRoom := strings.TrimSpace(meta.RoomID)
	return roomID != "" && roomID != teamRoom
}
