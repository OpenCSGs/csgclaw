package im

import (
	"errors"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
)

func TestRoomMessageDispatchAndMentionAliases(t *testing.T) {
	s := NewService()
	_, direct, err := s.EnsureAgentUser(EnsureAgentUserRequest{ID: "agent-dev", Name: "dev", Role: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	for _, roomType := range []apitypes.RoomType{apitypes.RoomTypeOnDemand, apitypes.RoomTypeFree} {
		room, err := s.CreateRoom(CreateRoomRequest{Title: "Mention policy", CreatorID: "admin", MemberIDs: []string{"manager", "dev"}, Type: roomType})
		if err != nil {
			t.Fatal(err)
		}
		for _, alias := range []string{"pt-dev", "user-dev", "agent-dev", "u-dev"} {
			content := `<at user_id="` + alias + `">Developer</at> Please reply`
			_, err := s.CreateMessage(CreateMessageRequest{RoomID: room.ID, SenderID: "manager", MentionID: alias, Content: content})
			if roomType == apitypes.RoomTypeOnDemand {
				if !errors.Is(err, ErrRoomTaskDispatchRequired) {
					t.Fatalf("%s accepted untracked mention: %v", alias, err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			// Humans can still address room members, and a mention_id must not
			// duplicate a matching body tag even when its ID/name uses an alias.
			msg, err := s.CreateMessage(CreateMessageRequest{RoomID: room.ID, SenderID: "admin", MentionID: alias, Content: content})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(msg.Content, "<at ") != 1 || len(msg.Mentions) != 1 || msg.Mentions[0].ID != "user-dev" {
				t.Fatalf("duplicate/incorrect mention: %+v", msg)
			}
		}
		if _, err := s.CreateMessage(CreateMessageRequest{RoomID: room.ID, SenderID: "manager", Content: "Status update"}); err != nil {
			t.Fatal(err)
		}
		msg, err := s.CreateMessage(CreateMessageRequest{RoomID: room.ID, SenderID: "admin", MentionID: "pt-dev", Content: "@dev Please reply"})
		if err != nil || msg.Content != `<at user_id="user-dev">dev</at> Please reply` {
			t.Fatalf("plain mention duplicated: %+v, %v", msg, err)
		}
	}
	if _, err := s.CreateMessage(CreateMessageRequest{RoomID: direct.ID, SenderID: "admin", MentionID: "pt-dev", Content: "Please reply"}); err != nil {
		t.Fatal(err)
	}
}
