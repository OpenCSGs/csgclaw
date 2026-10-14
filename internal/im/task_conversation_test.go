package im

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTaskConversationMetadataSurvivesReloadAndBootstrap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	svc, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	request := CreateRoomRequest{Title: "First task", CreatorID: "user-admin", MemberIDs: []string{"manager"}, IsDirect: true, TaskSession: true, ClientRequestID: "first"}
	first, err := svc.CreateRoom(request)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := svc.CreateRoom(request)
	if err != nil || retry.ID != first.ID {
		t.Fatalf("retry = %+v, %v", retry, err)
	}
	request.ClientRequestID = "second"
	second, err := svc.CreateRoom(request)
	if err != nil || second.ID == first.ID {
		t.Fatalf("second = %+v, %v", second, err)
	}
	title, yes := "Named task", true
	if _, err := svc.UpdateRoom(first.ID, UpdateRoomRequest{Title: &title, Pinned: &yes, Archived: &yes}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	room, ok := reloaded.Room(first.ID)
	if !ok || room.Title != title || !room.Archived || !room.Pinned || !room.TaskSession {
		t.Fatalf("metadata lost: %+v", room)
	}
	for _, room := range reloaded.ListRooms() {
		if room.ID == first.ID && room.Title != title {
			t.Fatalf("presentation replaced title: %+v", room)
		}
	}
	if _, err := reloaded.UpdateRoom(first.ID, UpdateRoomRequest{Title: new(string)}); err == nil {
		t.Fatal("empty name accepted")
	}
}

func TestTaskConversationRejectsInvalidDirectMembersAndRetry(t *testing.T) {
	svc := NewService()
	request := CreateRoomRequest{Title: "Task", CreatorID: "user-admin", IsDirect: true, TaskSession: true, ClientRequestID: "request"}
	if _, err := svc.CreateRoom(request); err == nil {
		t.Fatal("one-member direct conversation accepted")
	}
	request.MemberIDs = []string{"manager"}
	if _, err := svc.CreateRoom(request); err != nil {
		t.Fatal(err)
	}
	request.IsDirect = false
	if _, err := svc.CreateRoom(request); err == nil {
		t.Fatal("different retry accepted")
	}
}

func TestTaskConversationDeleteRetainsRoomWhenPersistenceFails(t *testing.T) {
	root := t.TempDir()
	svc, err := NewServiceFromPath(filepath.Join(root, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	room, err := svc.CreateRoom(CreateRoomRequest{Title: "Keep history", CreatorID: "user-admin", MemberIDs: []string{"manager"}, IsDirect: true, TaskSession: true})
	if err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blocked, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	svc.statePath = filepath.Join(blocked, "state.json")
	if err := svc.DeleteRoom(room.ID); err == nil {
		t.Fatal("storage failure was swallowed")
	}
	if stored, ok := svc.Room(room.ID); !ok || stored.Title != room.Title || len(stored.Messages) != len(room.Messages) {
		t.Fatalf("history lost after failed deletion: %+v", stored)
	}
}
