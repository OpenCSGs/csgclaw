package delivery

import (
	"context"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	"csgclaw/internal/roomtask"
	"testing"
)

func TestAutonomousSilenceIsScopedToAutonomousRoom(t *testing.T) {
	for _, autonomous := range []bool{true, false} {
		store := &recordingStore{}
		renderer := NewTranscriptRenderer(store)
		turn := channel.TurnContext{RoomID: "room", SourceMessageID: "message", Autonomous: autonomous}
		if err := renderer.Complete(context.Background(), turn, agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: roomtask.AutonomousSilence}); err != nil {
			t.Fatal(err)
		}
		want := 1
		if autonomous {
			want = 0
		}
		if len(store.messages) != want {
			t.Fatalf("autonomous=%v delivered %d messages, want %d", autonomous, len(store.messages), want)
		}
	}
}
