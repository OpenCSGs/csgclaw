package api

import (
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/taskconfig"
	"csgclaw/internal/team"
	"path/filepath"
	"slices"
	"testing"
)

func TestCollaborationConfigurationOnlySharesDirectory(t *testing.T) {
	root := t.TempDir()
	room := apitypes.Room{Type: apitypes.RoomTypeOnDemand}
	cfg, err := validateCollaborationConfiguration(room, &taskconfig.Config{WorkspaceDir: root})
	if err != nil || cfg.WorkspaceDir != root {
		t.Fatalf("%+v %v", cfg, err)
	}
	for _, invalid := range []taskconfig.Config{{ModelID: "override"}, {ExecutionMode: "danger-full-access"}, {WorkspaceDir: "../outside"}, {WorkspaceDir: root + "/missing"}} {
		if _, err := validateCollaborationConfiguration(room, &invalid); err == nil {
			t.Fatalf("accepted %+v", invalid)
		}
	}
}

func TestRoomTakesTeamGuideSnapshot(t *testing.T) {
	svc := team.NewService()
	meta, err := svc.CreateTeam(team.CreateTeamInput{Title: "Writers", LeadAgentID: "manager", CollaborationGuide: "Review shared files before delivery"})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{teamSvc: svc}
	req := apitypes.CreateRoomRequest{Type: apitypes.RoomTypeOnDemand, SourceTeamID: meta.ID}
	if err := h.applyRoomTeamGuide(&req); err != nil {
		t.Fatal(err)
	}
	cleared := ""
	if _, err := svc.UpdateTeam(team.UpdateTeamInput{TeamID: meta.ID, CollaborationGuide: &cleared}); err != nil {
		t.Fatal(err)
	}
	if req.CollaborationGuide != meta.CollaborationGuide {
		t.Fatal("existing room guide changed")
	}
	next := apitypes.CreateRoomRequest{Type: apitypes.RoomTypeOnDemand, SourceTeamID: meta.ID}
	if err := h.applyRoomTeamGuide(&next); err != nil || next.CollaborationGuide != "" {
		t.Fatal("new room did not use cleared guide")
	}
}

func TestTeamChangesDoNotRewriteExistingCollaboration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	messages, err := im.NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"dev", "qa"} {
		if _, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: id, Name: id, Role: "worker"}); err != nil {
			t.Fatal(err)
		}
	}
	teams := team.NewService()
	meta, err := teams.CreateTeam(team.CreateTeamInput{Title: "Writers", LeadAgentID: "manager", MemberAgentIDs: []string{"dev"}, CollaborationGuide: "Keep docs in docs/"})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{im: messages, teamSvc: teams}
	req := apitypes.CreateRoomRequest{Title: "First", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeOnDemand, SourceTeamID: meta.ID, ClientRequestID: "first"}
	if err := h.applyRoomTeamGuide(&req); err != nil {
		t.Fatal(err)
	}
	first, err := messages.CreateRoom(req)
	if err != nil {
		t.Fatal(err)
	}
	changed := "Write reports in reports/"
	if _, err := teams.UpdateTeam(team.UpdateTeamInput{TeamID: meta.ID, CollaborationGuide: &changed, MemberAgentIDs: []string{"qa"}, SetMemberAgentIDs: true}); err != nil {
		t.Fatal(err)
	}
	next := req
	next.ClientRequestID = "second"
	next.MemberIDs = []string{"qa"}
	if err := h.applyRoomTeamGuide(&next); err != nil {
		t.Fatal(err)
	}
	second, err := messages.CreateRoom(next)
	if err != nil {
		t.Fatal(err)
	}
	if second.CollaborationGuide != changed || second.Configuration.WorkspaceDir == first.Configuration.WorkspaceDir {
		t.Fatal("new room did not take independent settings")
	}
	if err := teams.DeleteTeam(meta.ID); err != nil {
		t.Fatal(err)
	}
	messages, err = im.NewServiceFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	h.im = messages
	// Lost-response retry must work after source Team deletion and server restart.
	req.CollaborationGuide = "not the saved guide"
	if err := h.applyRoomTeamGuide(&req); err != nil {
		t.Fatal(err)
	}
	retry, err := messages.CreateRoom(req)
	if err != nil || retry.ID != first.ID || retry.CollaborationGuide != first.CollaborationGuide || !slices.Equal(retry.Members, first.Members) {
		t.Fatalf("old room changed: %+v, %v", retry, err)
	}
	added, err := messages.AddRoomMembers(im.AddRoomMembersRequest{RoomID: first.ID, InviterID: "admin", UserIDs: []string{"qa"}})
	if err != nil || added.CollaborationGuide != first.CollaborationGuide || added.Configuration.WorkspaceDir != first.Configuration.WorkspaceDir {
		t.Fatalf("room membership edit lost saved settings: %+v %v", added, err)
	}
	other, _ := messages.Room(second.ID)
	if !slices.Equal(other.Members, second.Members) {
		t.Fatal("membership edit affected another room")
	}
}
