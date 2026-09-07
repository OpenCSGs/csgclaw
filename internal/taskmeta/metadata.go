package taskmeta

import (
	"encoding/json"
	"strconv"
	"strings"
)

const (
	IDKey      = "task_id"
	AttemptKey = "task_attempt"
)

// Set attaches task execution context without adding empty task fields to
// ordinary messages.
func Set(metadata map[string]any, taskID string, attempt int) map[string]any {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		delete(metadata, IDKey)
		delete(metadata, AttemptKey)
		return metadata
	}
	metadata[IDKey] = taskID
	if attempt > 0 {
		metadata[AttemptKey] = attempt
	} else {
		delete(metadata, AttemptKey)
	}
	return metadata
}

func ID(metadata map[string]any) string {
	value, _ := metadata[IDKey].(string)
	return strings.TrimSpace(value)
}

func Attempt(metadata map[string]any) int {
	switch value := metadata[AttemptKey].(type) {
	case int:
		return value
	case float64:
		return int(value)
	case json.Number:
		attempt, _ := strconv.Atoi(string(value))
		return attempt
	default:
		return 0
	}
}
