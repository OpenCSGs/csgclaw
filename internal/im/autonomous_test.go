package im

import (
	"csgclaw/internal/apitypes"
	"testing"
)

func TestAutonomousRoutingDoesNotChangeLegacyFreeRooms(t *testing.T) {
	sender := User{ID: "user-dev", Role: "worker"}
	message := Message{SenderID: sender.ID, Kind: MessageKindMessage, Content: "Public result"}
	room := Room{Members: []string{"user-dev", "user-qa"}}
	if shouldNotifyParticipant(room, sender, message, "pt-qa") {
		t.Fatal("legacy room started broadcasting")
	}
	room.Type = apitypes.RoomTypeAutonomous
	if !shouldNotifyParticipant(room, sender, message, "pt-qa") {
		t.Fatal("autonomous peer not notified")
	}
	if shouldNotifyParticipant(room, sender, message, "pt-dev") {
		t.Fatal("sender notified itself")
	}
	message.Metadata = map[string]any{"csgclaw": map[string]any{"delivery_kind": "activity"}}
	if shouldNotifyParticipant(room, sender, message, "pt-qa") {
		t.Fatal("private activity woke peer")
	}
}
