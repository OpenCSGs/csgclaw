package presentation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompletionRetryUsesCallbackWithoutChatCommand(t *testing.T) {
	for _, card := range []map[string]any{COTCompletionFailureCard()} {
		raw, _ := json.Marshal(card)
		text := string(raw)
		if !strings.Contains(text, `"type":"callback"`) || !strings.Contains(text, `"operation":"cancel"`) || strings.Contains(text, "/stop") {
			t.Fatalf("control=%s", text)
		}
	}
}
