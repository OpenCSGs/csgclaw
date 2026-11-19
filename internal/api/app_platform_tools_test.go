package api

import (
	"context"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
)

func TestDSHAppMCPListsConnectorToolsWithoutCodexFileTools(t *testing.T) {
	h := newAppTaskTestHandler(t)
	records := h.svc.(appTaskTestRecords)
	worker := records.records["agent-dev"]
	worker.RuntimeKind = agent.RuntimeKindDSH
	records.records[worker.ID] = worker
	h.appPlatformAgents = make(map[string]string)
	h.registerAppPlatformTools(worker.ID)
	client := taskMCPClient(t, h, worker.ID)
	listed, err := client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]bool{}
	for _, tool := range listed.Tools {
		tools[tool.Name] = true
	}
	if !tools["apps_list"] || tools["csgclaw_publish_file"] || tools["csgclaw_upload_file"] {
		t.Fatalf("DSH App MCP tools = %#v", tools)
	}
}

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

func TestAppMCPRoomCreationModesAndDelivery(t *testing.T) {
	h := newAppTaskTestHandler(t)
	_, sourceRoom, err := h.im.EnsureAgentUser(im.EnsureAgentUserRequest{ID: agent.ManagerUserID, Name: "manager", Role: agent.RoleManager})
	if err != nil {
		t.Fatal(err)
	}
	source, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: sourceRoom.ID, SenderID: "admin", Content: "Create a collaboration room"})
	if err != nil {
		t.Fatal(err)
	}
	h.registerAppRoomTools(agent.ManagerUserID)
	manager := taskMCPClient(t, h, agent.ManagerUserID)
	for _, tc := range []struct {
		name, mode  string
		omit        bool
		wantType    apitypes.RoomType
		wantManager bool
		wantWorker  bool
		members     []any
	}{
		{name: "omitted", omit: true, wantType: apitypes.RoomTypeOnDemand, wantManager: true},
		{name: "empty", wantType: apitypes.RoomTypeOnDemand, wantManager: true},
		{name: "moderated", mode: "on_demand", wantType: apitypes.RoomTypeOnDemand, wantManager: true},
		{name: "roundtable", mode: "autonomous", wantType: apitypes.RoomTypeAutonomous, wantManager: true, wantWorker: true},
		{name: "requester and manager remain a group", omit: true, wantType: apitypes.RoomTypeOnDemand, wantManager: true, members: []any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			members := tc.members
			if members == nil {
				members = []any{"dev"}
			}
			args := map[string]any{"title": tc.name, "source_room_id": sourceRoom.ID, "source_message_id": source.ID, "member_ids": members}
			if !tc.omit {
				args["type"] = tc.mode
			}
			var created im.Room
			callTaskTool(t, manager, "room_create", args, &created)
			if created.IsDirect || created.TaskSession || created.Type != tc.wantType {
				t.Fatalf("expected %s collaboration space, got %+v", tc.wantType, created)
			}
			if len(members) == 0 && len(created.Members) != 2 {
				t.Fatalf("expected requester and Manager only, got %v", created.Members)
			}
			if created.IsOnDemand() && !h.participantBridgeTargetForRoomMember(created.ManagerID).matches(agent.ManagerUserID) {
				t.Fatalf("room manager = %q", created.ManagerID)
			}
			stored, ok := h.im.Room(created.ID)
			if !ok || stored.Type != tc.wantType || stored.ManagerID != created.ManagerID {
				t.Fatalf("stored mode differs from creation result: %+v", stored)
			}
			message, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: created.ID, SenderID: "admin", Content: "Please start the requested work"})
			if err != nil {
				t.Fatal(err)
			}
			sender, _ := h.im.User(message.SenderID)
			for _, target := range []struct {
				id   string
				want bool
			}{{agent.ManagerUserID, tc.wantManager}, {"agent-dev", tc.wantWorker}} {
				bridgeTarget := h.participantBridgeTargetForBridgeID(target.id)
				events, cancel := h.participantBridge.Subscribe(bridgeTarget.bridgeID)
				defer cancel()
				if !h.enqueueParticipantMessageEventForBridgeTarget(stored, sender, message, bridgeTarget, "") {
					t.Fatalf("failed to enqueue for %s", target.id)
				}
				select {
				case event := <-events:
					if !target.want || event.MessageID != message.ID || event.RoomManager != (created.IsOnDemand() && target.id == agent.ManagerUserID) {
						t.Fatalf("unexpected delivery to %s: %+v", target.id, event)
					}
				default:
					if target.want {
						t.Fatalf("human message without @ did not reach %s", target.id)
					}
				}
			}
		})
	}
	for _, mode := range []string{"free", "unsupported"} {
		before := len(h.im.ListRooms())
		denyTaskTool(t, manager, "room_create", map[string]any{"title": "invalid", "source_room_id": sourceRoom.ID, "source_message_id": source.ID, "member_ids": []any{"dev"}, "type": mode})
		if len(h.im.ListRooms()) != before {
			t.Fatalf("unsupported room mode %q created a room", mode)
		}
	}
}
