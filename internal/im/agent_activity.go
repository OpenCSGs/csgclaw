package im

import (
	"encoding/json"
	"strings"
)

// IsAutonomousCoordinationNotice identifies a room notice used by the agents
// to coordinate ownership. It remains readable by members through unread tools.
func IsAutonomousCoordinationNotice(message Message) bool {
	meta, _ := message.Metadata["csgclaw"].(map[string]any)
	kind, _ := meta["delivery_kind"].(string)
	return kind == "autonomous_task_update"
}

// Activity records are observable data, never conversational wake-up events,
// even when persisted with kind=message or containing an @ mention.
func IsAgentActivityMessage(message Message) bool {
	if meta, ok := message.Metadata["csgclaw"].(map[string]any); ok {
		if direct, _ := meta["agent_session_stream_direct"].(bool); direct {
			return true
		}
		if kind, _ := meta["delivery_kind"].(string); kind == "turn_stopped" {
			return true
		}
	}
	final := false
	for _, key := range []string{"codex", "csgclaw"} {
		if meta, ok := message.Metadata[key].(map[string]any); ok {
			kind, _ := meta["delivery_kind"].(string)
			switch kind {
			case "tool", "thought", "activity", "task_reported":
				return true
			case "final":
				final = true
			}
		}
	}
	// Only untyped historical records need content-based activity detection.
	if final {
		return false
	}
	content := strings.TrimSpace(message.Content)
	if strings.HasPrefix(content, "🔧 ") {
		return true
	}
	var payload struct {
		Type string `json:"type"`
	}
	return json.Unmarshal([]byte(content), &payload) == nil && payload.Type == "com.opencsg.csgclaw.agent.activity"
}
