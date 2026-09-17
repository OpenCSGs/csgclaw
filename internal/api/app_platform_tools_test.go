package api

import (
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"testing"
)

func TestAppMCPRoomCreationPreservesHumanAndMembership(t *testing.T) {
	h := newAppTaskTestHandler(t)
	sourceRoom, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "request", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeOnDemand})
	if err != nil {
		t.Fatal(err)
	}
	source, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: sourceRoom.ID, SenderID: "admin", Content: "Create a room for the requested work"})
	if err != nil {
		t.Fatal(err)
	}
	h.registerAppRoomTools(agent.ManagerUserID)
	h.registerAppRoomTools("agent-dev")
	manager := taskMCPClient(t, h, agent.ManagerUserID)
	worker := taskMCPClient(t, h, "agent-dev")
	var created im.Room
	callTaskTool(t, manager, "room_create", map[string]any{"title": "created", "source_room_id": sourceRoom.ID, "source_message_id": source.ID, "member_ids": []any{"dev"}}, &created)
	if len(created.Messages) == 0 || !h.participantBridgeTargetForRoomMember("admin").matches(created.Messages[0].SenderID) {
		t.Fatal("room creation event did not preserve the requester")
	}
	if !h.agentPlatformRoom(agent.ManagerUserID, created.ID) || !h.agentPlatformRoom("agent-dev", created.ID) {
		t.Fatal("requested members were not preserved")
	}
	denyTaskTool(t, worker, "room_create", map[string]any{"title": "denied", "source_room_id": sourceRoom.ID, "source_message_id": source.ID, "member_ids": []any{"dev"}})
	denyTaskTool(t, manager, "room_create", map[string]any{"title": "forged", "source_room_id": sourceRoom.ID, "source_message_id": "missing", "member_ids": []any{"dev"}})
	callTaskTool(t, worker, "message_send", map[string]any{"room_id": created.ID, "content": "from worker"}, nil)
	denyTaskTool(t, worker, "message_send", map[string]any{"room_id": created.ID, "content": "spoofed", "sender_id": "admin"})
}
