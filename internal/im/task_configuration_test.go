package im

import (
	"csgclaw/internal/taskconfig"
	"path/filepath"
	"testing"
)

func TestTaskConfigurationPersistsAndDoesNotLeakBetweenRooms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	svc, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := taskconfig.Config{ModelProviderID: "opencsg", ModelID: "one", WorkspaceDir: "/workspace/one", ReasoningEffort: "high"}
	room, err := svc.CreateRoom(CreateRoomRequest{Title: "one", CreatorID: "user-admin", MemberIDs: []string{"manager"}, IsDirect: true, TaskSession: true, Configuration: &cfg})
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.CreateRoom(CreateRoomRequest{Title: "two", CreatorID: "user-admin", MemberIDs: []string{"manager"}, IsDirect: true, TaskSession: true})
	if err != nil {
		t.Fatal(err)
	}
	cfg.ModelID = "two"
	updated, err := svc.UpdateRoom(room.ID, UpdateRoomRequest{Configuration: &cfg})
	if err != nil {
		t.Fatal(err)
	}
	updated.Configuration.ModelID = "external mutation"
	cfg.WorkspaceDir = "/workspace/changed"
	if _, err := svc.UpdateRoom(room.ID, UpdateRoomRequest{Configuration: &cfg}); err == nil {
		t.Fatal("changed locked directory")
	}
	restored, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := restored.Room(room.ID)
	second, _ := restored.Room(other.ID)
	if got.Configuration == nil || got.Configuration.ModelID != "two" || got.Configuration.WorkspaceDir != "/workspace/one" || second.Configuration != nil {
		t.Fatalf("configuration not isolated/persisted: %+v, %+v", got.Configuration, second.Configuration)
	}
}
