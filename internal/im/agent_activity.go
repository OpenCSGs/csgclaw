package im

import (
	"encoding/json"
	"strings"
)

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
	for _, key := range []string{"codex", "openclaw", "csgclaw"} {
		if meta, ok := message.Metadata[key].(map[string]any); ok {
			if kind, _ := meta["delivery_kind"].(string); kind == "tool" || kind == "thought" || kind == "activity" || kind == "task_reported" {
				return true
			}
		}
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
