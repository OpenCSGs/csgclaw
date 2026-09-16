package im

import (
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
