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
		CurrentUserID: "u-manager",
		Users: []im.User{
			{ID: "u-manager", Name: "manager", Handle: "manager", Role: "manager"},
			{ID: "u-alice", Name: "alice", Handle: "alice", Role: "worker"},
			{ID: "u-bob", Name: "bob", Handle: "bob", Role: "worker"},
		},
	})
	svc := NewService(imSvc)

	room, err := svc.CreateRoom(apitypes.CreateRoomRequest{
		Title:     "Ops",
		CreatorID: " u-manager ",
		MemberIDs: []string{
			" u-alice ",
		},
	})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	assertMembers(t, room.Members, "u-manager", "u-alice")

	room, err = svc.AddRoomMembers(apitypes.AddRoomMembersRequest{
		RoomID:    room.ID,
		InviterID: " u-manager ",
		UserIDs: []string{
			" u-bob ",
		},
	})
	if err != nil {
		t.Fatalf("AddRoomMembers() error = %v", err)
	}
	assertMembers(t, room.Members, "u-manager", "u-alice", "u-bob")

	message, err := svc.SendMessage(apitypes.CreateMessageRequest{
		RoomID:    room.ID,
		SenderID:  " u-manager ",
		MentionID: " u-alice ",
		Content:   "hello",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if message.SenderID != "u-manager" {
		t.Fatalf("SenderID = %q, want %q", message.SenderID, "u-manager")
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
