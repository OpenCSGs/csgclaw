package im

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"csgclaw/internal/apitypes"
)

func TestRoomAttachmentReferencesIncludeRetainedContext(t *testing.T) {
	now := time.Now()
	a := MessageAttachment{ID: "older", Name: "历史合同.DOCX", CreatedAt: now.Add(-time.Hour)}
	b := MessageAttachment{ID: "newer", Name: "历史合同.DOCX", CreatedAt: now}
	s := NewServiceFromBootstrap(Bootstrap{Rooms: []Room{{ID: "room", Members: []string{"user-admin"}, Messages: []Message{{ID: "new", SenderID: "user-admin", Attachments: []MessageAttachment{b}}}, Threads: []ThreadState{{RootMessageID: "new", Context: []Message{{ID: "old", SenderID: "user-worker", Attachments: []MessageAttachment{a}}, {ID: "new", SenderID: "user-admin", Attachments: []MessageAttachment{b}}}}}}}})
	list, err := s.ListRoomAttachments("room", apitypes.RoomAttachmentListOptions{Query: "docx"})
	if err != nil || list.Total != 2 || len(list.Items) != 2 || list.Items[0].ID != "newer" || list.Items[1].MessageID != "old" {
		t.Fatalf("retained context: %+v %v", list, err)
	}
	filtered, err := s.ListRoomAttachments("room", apitypes.RoomAttachmentListOptions{MessageID: "old"})
	if err != nil || filtered.Total != 1 || filtered.Items[0].SenderID != "user-worker" {
		t.Fatalf("source: %+v %v", filtered, err)
	}
	if _, err = s.ClearRoomMessages("room"); err != nil {
		t.Fatal(err)
	}
	list, err = s.ListRoomAttachments("room", apitypes.RoomAttachmentListOptions{})
	if err != nil || list.Total != 0 {
		t.Fatalf("clear retained context: %+v %v", list, err)
	}
	if err = s.DeleteRoom("room"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ListRoomAttachments("room", apitypes.RoomAttachmentListOptions{}); err == nil {
		t.Fatal("listed deleted room")
	}
}

func TestDeleteRoomAttachmentPreservesMessagesOtherIDsAndSavedCopies(t *testing.T) {
	state := filepath.Join(t.TempDir(), "im", "state.json")
	bus := NewBus()
	service, err := NewServiceFromPathWithBus(state, bus)
	if err != nil {
		t.Fatal(err)
	}
	room, err := service.CreateRoom(CreateRoomRequest{Title: "Files", CreatorID: "user-admin"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.CreateRoom(CreateRoomRequest{Title: "Other", CreatorID: "user-admin"})
	if err != nil {
		t.Fatal(err)
	}
	upload := func(roomID, name string, data []byte) Message {
		message, err := service.CreateMessage(CreateMessageRequest{RoomID: roomID, SenderID: "user-admin", Content: "keep this text", Attachments: []MessageAttachmentUpload{{Name: name, Data: data}}})
		if err != nil {
			t.Fatal(err)
		}
		return message
	}
	payload := []byte("shared video content")
	source := upload(room.ID, "same.mkv", payload)
	shared := upload(other.ID, "other.mkv", payload)
	sameName := upload(room.ID, "same.mkv", []byte("different video"))
	if _, _, err := service.StartThread(StartThreadRequest{RoomID: room.ID, RootMessageID: source.ID}); err != nil {
		t.Fatal(err)
	}
	events, cancel := bus.Subscribe()
	defer cancel()
	if err := service.DeleteRoomAttachment(room.ID, source.Attachments[0].ID); err != nil {
		t.Fatal(err)
	}
	event := <-events
	if event.Type != EventTypeRoomAttachmentDeleted || event.AttachmentID != source.Attachments[0].ID {
		t.Fatalf("event=%+v", event)
	}
	if _, err := service.AttachmentFile(source.Attachments[0].ID); err == nil {
		t.Fatal("deleted attachment object survives")
	}
	file, err := service.AttachmentFile(shared.Attachments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file.Path)
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("shared blob lost: %v", err)
	}
	service, err = NewServiceFromPath(state)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := service.ListRoomAttachments(room.ID, apitypes.RoomAttachmentListOptions{})
	if err != nil || listed.Total != 1 || listed.Items[0].ID != sameName.Attachments[0].ID {
		t.Fatalf("list after reload=%+v %v", listed, err)
	}
	stored, ok := service.Room(room.ID)
	if !ok {
		t.Fatal("room missing")
	}
	for _, message := range stored.Messages {
		if message.ID == source.ID && (message.Content != "keep this text" || len(message.Attachments) != 0) {
			t.Fatal("message text changed or deleted file restored")
		}
	}
	if len(stored.Threads) == 0 {
		t.Fatal("thread disappeared")
	}
	for _, thread := range stored.Threads {
		for _, message := range thread.Context {
			for _, a := range message.Attachments {
				if a.ID == source.Attachments[0].ID {
					t.Fatal("retained thread resurrected deleted attachment")
				}
			}
		}
	}
	if err := service.DeleteRoomAttachment(room.ID, source.Attachments[0].ID); !errors.Is(err, ErrRoomAttachmentNotFound) {
		t.Fatalf("repeat delete=%v", err)
	}
	if err := service.DeleteRoomAttachment(room.ID, shared.Attachments[0].ID); !errors.Is(err, ErrRoomAttachmentNotFound) {
		t.Fatalf("cross-room delete=%v", err)
	}
}
