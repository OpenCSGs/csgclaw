package api

import (
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/roomtask"
	"strings"
	"testing"
)

func TestSceneContextsCarryIdentitySenderAndRoomPolicy(t *testing.T) {
	for _, mode := range []string{"direct", "free", "autonomous"} {
		t.Run(mode, func(t *testing.T) {
			s := im.NewService()
			if _, _, err := s.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "dev", Name: "Reviewer", Role: "worker"}); err != nil {
				t.Fatal(err)
			}
			typ := apitypes.RoomType(mode)
			if mode == "direct" {
				typ = apitypes.RoomTypeFree
			}
			room, err := s.CreateRoom(im.CreateRoomRequest{Title: mode, CreatorID: "admin", MemberIDs: []string{"dev"}, Type: typ, IsDirect: mode == "direct", CollaborationGuide: "Review before delivery"})
			if err != nil {
				t.Fatal(err)
			}
			root, err := s.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: "Draft"})
			if err != nil {
				t.Fatal(err)
			}
			message, err := s.CreateMessage(im.CreateMessageRequest{
				RoomID: room.ID, SenderID: "admin", Content: "Review", MentionID: "dev",
				RelatesTo: &apitypes.MessageRelation{RelType: im.RelationTypeThread, EventID: root.ID},
			})
			if err != nil {
				t.Fatal(err)
			}
			h := &Handler{im: s}
			request := roomtask.TurnContextRequest{RoomID: room.ID, ParticipantID: "pt-dev", SourceID: message.ID}
			current, err := h.roomExecutionContext(request)
			if err != nil {
				t.Fatal(err)
			}
			projector := roomtask.NewContextProjector()
			text, err := projector.Project("conversation", current)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{`room_type="` + mode + `"`, `"name":"Reviewer"`, `"type":"human"`, `"addressed_to_you":true`, `"thread_root_id":"` + root.ID + `"`, "Review before delivery"} {
				if !strings.Contains(text, want) {
					t.Fatalf("missing %s: %s", want, text)
				}
			}
			if mode == "autonomous" {
				duplicate, err := h.roomExecutionContext(request)
				if err != nil || !duplicate.Skip {
					t.Fatal("replay admitted", duplicate, err)
				}
			}
		})
	}
}

func TestSceneEventExposesAddressedTeamDispatch(t *testing.T) {
	s := im.NewService()
	if _, _, err := s.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "dev", Name: "Developer", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	room, err := s.CreateRoom(im.CreateRoomRequest{Title: "Team execution", CreatorID: "admin", MemberIDs: []string{"dev"}})
	if err != nil {
		t.Fatal(err)
	}
	message, err := s.DeliverEvent(im.DeliverEventRequest{
		RoomID: room.ID, SenderID: "admin", MentionID: "dev", MessageID: "team:team-1:event:1",
		Event: &im.EventPayload{Key: "task_assigned"},
	})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{im: s}
	turn := h.sceneEvent(room, roomtask.TurnContextRequest{RoomID: room.ID, ParticipantID: "pt-dev", SourceID: message.ID})
	if turn["event_key"] != "task_assigned" || turn["source_message_id"] != "team:team-1:event:1" || turn["addressed_to_you"] != true {
		t.Fatalf("unexpected dispatch context: %#v", turn)
	}
}
