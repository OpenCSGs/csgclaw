package csgclaw

import (
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
)

func TestNewServiceWithNilIMReturnsNil(t *testing.T) {
	if got := NewService(nil); got != nil {
		t.Fatalf("NewService(nil) = %#v, want nil", got)
	}
}

func TestServiceUsesBotIDsAsIMUserIDs(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "manager",
		Users: []im.User{
			{ID: "manager", Name: "manager", Handle: "manager", Role: "manager"},
			{ID: "u-alice", Name: "alice", Handle: "alice", Role: "worker"},
			{ID: "u-bob", Name: "bob", Handle: "bob", Role: "worker"},
		},
	})
	svc := NewService(imSvc)

	room, err := svc.CreateRoom(apitypes.CreateRoomRequest{
		Title:     "Ops",
		CreatorID: " manager ",
		MemberIDs: []string{
			" u-alice ",
		},
	})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	assertMembers(t, room.Members, "manager", "u-alice")

	room, err = svc.AddRoomMembers(apitypes.AddRoomMembersRequest{
		RoomID:    room.ID,
		InviterID: " manager ",
		UserIDs: []string{
			" u-bob ",
		},
	})
	if err != nil {
		t.Fatalf("AddRoomMembers() error = %v", err)
	}
	assertMembers(t, room.Members, "manager", "u-alice", "u-bob")

	message, err := svc.SendMessage(apitypes.CreateMessageRequest{
		RoomID:    room.ID,
		SenderID:  " manager ",
		MentionID: " u-alice ",
		Content:   "hello",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if message.SenderID != "manager" {
		t.Fatalf("SenderID = %q, want %q", message.SenderID, "manager")
	}
	if !strings.Contains(message.Content, "u-alice") {
		t.Fatalf("Content = %q, want mention tag for u-alice", message.Content)
	}

	messages, err := svc.ListMessages(room.ID)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("messages len = %d, want room-created + member-added + user message", len(messages))
	}

	members, err := svc.ListRoomMembers(room.ID)
	if err != nil {
		t.Fatalf("ListRoomMembers() error = %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("members len = %d, want 3", len(members))
	}

	if err := svc.DeleteRoom(room.ID); err != nil {
		t.Fatalf("DeleteRoom() error = %v", err)
	}
	if _, err := svc.ListMessages(room.ID); err == nil {
		t.Fatal("ListMessages() error = nil, want room not found after DeleteRoom")
	}
}

func TestServiceNormalizesCanonicalSlashCommand(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "manager",
		Users: []im.User{
			{ID: "manager", Name: "manager", Handle: "manager", Role: "manager"},
			{ID: "u-alice", Name: "alice", Handle: "alice", Role: "worker"},
		},
		Rooms: []im.Room{{ID: "room-1", Title: "Direct", Members: []string{"manager", "u-alice"}}},
	})
	svc := NewService(imSvc)

	message, err := svc.SendMessage(apitypes.CreateMessageRequest{
		RoomID:   "room-1",
		SenderID: "manager",
		Content:  ` <slash-command arg="skill-creator" name="use-skill"/> create one `,
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	want := `<slash-command name="use-skill" arg="skill-creator"></slash-command> create one`
	if message.Content != want {
		t.Fatalf("Content = %q, want canonical XML %q", message.Content, want)
	}
}

func TestServiceKeepsLegacySlashTextAsPlainContent(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "manager",
		Users:         []im.User{{ID: "manager", Name: "manager", Handle: "manager", Role: "manager"}},
		Rooms:         []im.Room{{ID: "room-1", Title: "Direct", Members: []string{"manager"}}},
	})
	svc := NewService(imSvc)

	message, err := svc.SendMessage(apitypes.CreateMessageRequest{
		RoomID:   "room-1",
		SenderID: "manager",
		Content:  `/skill-creator create one`,
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if message.Content != `/skill-creator create one` {
		t.Fatalf("Content = %q, want legacy slash text kept as plain content", message.Content)
	}
}

func assertMembers(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("members = %#v, want %#v", got, want)
	}
	for _, id := range want {
		found := false
		for _, memberID := range got {
			if memberID == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("members = %#v, want member %q", got, id)
		}
	}
}
