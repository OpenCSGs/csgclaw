package im

import (
	"bytes"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/taskconfig"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLegacyCollaborationKeepsConfigurationAcrossRestart(t *testing.T) {
	for _, tc := range []struct {
		name          string
		configuration *taskconfig.Config
	}{
		{name: "without configuration"},
		{name: "empty configuration", configuration: &taskconfig.Config{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// In-memory creation models a room saved before shared directories existed.
			legacy := NewService()
			room, err := legacy.CreateRoom(CreateRoomRequest{Title: "Existing work", CreatorID: "user-admin", Type: apitypes.RoomTypeOnDemand, Configuration: tc.configuration})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "state.json")
			if err := SaveBootstrap(path, legacy.Bootstrap()); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				restored, err := NewServiceFromPath(path)
				if err != nil {
					t.Fatal(err)
				}
				got, ok := restored.Room(room.ID)
				if !ok || !reflect.DeepEqual(got.Configuration, tc.configuration) {
					t.Fatalf("legacy room configuration changed: %+v", got.Configuration)
				}
				if !got.IsOnDemand() || got.ManagerID != room.ManagerID || !reflect.DeepEqual(got.Members, room.Members) {
					t.Fatal("legacy collaboration routing changed")
				}
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), "workspaces")); !os.IsNotExist(err) {
				t.Fatalf("loading legacy rooms created a shared workspace: %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("loading legacy rooms rewrote saved state")
			}
		})
	}
}

func TestCollaborationOwnsWorkspaceAndGuideAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	svc, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	create := func() Room {
		room, err := svc.CreateRoom(CreateRoomRequest{Title: "Build", CreatorID: "user-admin", Type: apitypes.RoomTypeOnDemand, SourceTeamID: "team-1", CollaborationGuide: "Write docs in docs/; review before delivery"})
		if err != nil {
			t.Fatal(err)
		}
		return room
	}
	first, second := create(), create()
	if first.Configuration == nil || second.Configuration == nil || first.Configuration.WorkspaceDir == second.Configuration.WorkspaceDir {
		t.Fatal("rooms must have independent directories")
	}
	file := filepath.Join(first.Configuration.WorkspaceDir, "shared.txt")
	if err := os.WriteFile(file, []byte("collaboration output"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateRoom(first.ID, UpdateRoomRequest{Configuration: &taskconfig.Config{WorkspaceDir: second.Configuration.WorkspaceDir}}); err == nil {
		t.Fatal("changed active room root")
	}
	restored, err := NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := restored.Room(first.ID)
	if got.Configuration.WorkspaceDir != first.Configuration.WorkspaceDir || got.CollaborationGuide != first.CollaborationGuide || got.SourceTeamID != "team-1" {
		t.Fatalf("lost room settings: %+v", got)
	}
	if err := restored.DeleteRoom(first.ID); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "collaboration output" {
		t.Fatal("deleting chat must preserve shared outputs", err)
	}
}

func TestCollaborationCreationRetryKeepsOriginalTeamAndDirectory(t *testing.T) {
	for _, automatic := range []bool{true, false} {
		t.Run(fmt.Sprint("automatic=", automatic), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			svc, err := NewServiceFromPath(path)
			if err != nil {
				t.Fatal(err)
			}
			req := CreateRoomRequest{Title: "Build", CreatorID: "admin", Type: apitypes.RoomTypeOnDemand, SourceTeamID: "team-a", ClientRequestID: "retry", CollaborationGuide: "Original guide"}
			if !automatic {
				req.Configuration = &taskconfig.Config{WorkspaceDir: t.TempDir()}
			}
			first, err := svc.CreateRoom(req)
			if err != nil {
				t.Fatal(err)
			}
			// Mutating a returned value must not alter the persisted creation intent.
			*first.CreationWorkspaceDir = "corrupted"
			svc, err = NewServiceFromPath(path)
			if err != nil {
				t.Fatal(err)
			}
			req.CollaborationGuide = "Later Team guide"
			got, err := svc.CreateRoom(req)
			if err != nil || got.ID != first.ID || got.CollaborationGuide != first.CollaborationGuide {
				t.Fatalf("retry changed snapshot: %+v %v", got, err)
			}
			for _, change := range []func(*CreateRoomRequest){
				func(r *CreateRoomRequest) { r.SourceTeamID = "team-b" },
				func(r *CreateRoomRequest) { r.Configuration = &taskconfig.Config{WorkspaceDir: t.TempDir()} },
				func(r *CreateRoomRequest) {
					if automatic {
						r.Configuration = &taskconfig.Config{WorkspaceDir: first.Configuration.WorkspaceDir}
					} else {
						r.Configuration = nil
					}
				},
			} {
				other := req
				change(&other)
				if _, err := svc.CreateRoom(other); err == nil {
					t.Fatal("accepted different creation intent with the same request ID")
				}
			}
			data, err := json.Marshal(got)
			if err != nil || bytes.Contains(data, []byte("creation_workspace_dir")) || bytes.Contains(data, []byte("CreationWorkspaceDir")) {
				t.Fatalf("private metadata leaked: %s %v", data, err)
			}
		})
	}
}
