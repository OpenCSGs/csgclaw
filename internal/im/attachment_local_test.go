package im

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalAttachmentCopyPreservesNameAndOriginal(t *testing.T) {
	svc, err := NewServiceFromPath(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	room, err := svc.CreateRoom(CreateRoomRequest{Title: "Files", CreatorID: "user-admin"})
	if err != nil {
		t.Fatal(err)
	}
	message, err := svc.CreateMessage(CreateMessageRequest{RoomID: room.ID, SenderID: "user-admin", Attachments: []MessageAttachmentUpload{{Name: "same name.json", Data: []byte(`{"original":true}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	id := message.Attachments[0].ID
	original, err := svc.RoomAttachmentFile(room.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	local, err := svc.LocalRoomAttachmentFile(room.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	if local.Path == original.Path || filepath.Base(local.Path) != "same name.json" {
		t.Fatalf("copy: %+v", local)
	}
	if err := os.WriteFile(local.Path, []byte("edited"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(original.Path)
	if err != nil || !bytes.Equal(data, []byte(`{"original":true}`)) {
		t.Fatalf("original changed: %s %v", data, err)
	}
	again, err := svc.LocalRoomAttachmentFile(room.ID, id)
	if err != nil || again.Path != local.Path {
		t.Fatalf("reopen: %+v %v", again, err)
	}
	data, err = os.ReadFile(again.Path)
	if err != nil || string(data) != "edited" {
		t.Fatalf("edit lost: %s %v", data, err)
	}
	other, err := svc.CreateRoom(CreateRoomRequest{Title: "Other", CreatorID: "user-admin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LocalRoomAttachmentFile(other.ID, id); err == nil {
		t.Fatal("cross-room copy allowed")
	}
	if err := svc.DeleteRoomAttachment(room.ID, id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(local.Path); !os.IsNotExist(err) {
		t.Fatalf("copy not removed: %v", err)
	}
	if _, err := svc.LocalRoomAttachmentFile(room.ID, id); err == nil {
		t.Fatal("deleted attachment reopened")
	}
}
