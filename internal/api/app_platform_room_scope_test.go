package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/channel/csgclaw/conv"
	"csgclaw/internal/im"
	"csgclaw/internal/taskcore"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func roomToolConversation(t *testing.T, roomID, taskID string) string {
	t.Helper()
	key, err := conv.ConversationKey(channel.Binding{ID: "pt-manager"}, channel.Event{RoomID: roomID, TaskID: taskID})
	if err != nil {
		t.Fatal(err)
	}
	return string(key)
}

func TestAppRoomToolScopePreservesOtherWorkflows(t *testing.T) {
	h := newAppTaskTestHandler(t)
	client := taskMCPClient(t, h, "agent-manager")
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Collaboration", CreatorID: "admin", MemberIDs: []string{"dev", "qa"}, Type: apitypes.RoomTypeOnDemand})
	if err != nil {
		t.Fatal(err)
	}
	conversation := roomToolConversation(t, room.ID, "")
	for _, target := range []string{"pt-dev", "agent-dev", "user-dev"} {
		args := map[string]any{"room_id": room.ID, "mention_id": target, "content": "Participate"}
		if err := h.scopeAppPlatformRoomConversation("agent-manager", "message_send", conversation, args); err == nil || !strings.Contains(err.Error(), "room_task_dispatch") {
			t.Fatalf("ordinary mention %s: %v", target, err)
		}
	}
	if err := h.scopeAppPlatformRoomConversation("agent-manager", "agent_task_create", conversation, map[string]any{"agent_id": "agent-dev"}); err == nil {
		t.Fatal("on-demand turn accepted a private task")
	}
	for _, args := range []map[string]any{{"room_id": room.ID, "content": "Progress"}, {"room_id": room.ID, "mention_id": "pt-admin", "content": "Question"}} {
		if err := h.scopeAppPlatformRoomConversation("agent-manager", "message_send", conversation, args); err != nil {
			t.Fatal(err)
		}
	}
	for _, roomType := range []apitypes.RoomType{apitypes.RoomTypeFree, "direct"} {
		var id string
		if roomType == "direct" {
			_, direct, err := h.im.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "agent-manager", Name: "manager", Role: "manager"})
			if err != nil {
				t.Fatal(err)
			}
			id = direct.ID
		} else {
			free, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Free", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: roomType})
			if err != nil {
				t.Fatal(err)
			}
			id = free.ID
		}
		key := roomToolConversation(t, id, "")
		args := map[string]any{"agent_id": "agent-dev", "title": "Explicit personal task"}
		if err := h.scopeAppPlatformRoomConversation("agent-manager", "agent_task_create", key, args); err != nil {
			t.Fatal(err)
		}
		var task apitypes.TeamTask
		callTaskTool(t, client, "agent_task_create", args, &task)
		if task.AssignmentType != taskcore.AssignmentTypeAgent || task.RoomID == room.ID {
			t.Fatal("personal task workflow changed", task)
		}
		mention := map[string]any{"room_id": id, "mention_id": "pt-dev"}
		if err := h.scopeAppPlatformRoomConversation("agent-manager", "message_send", key, mention); err != nil {
			t.Fatal(err)
		}
	}
	args := map[string]any{"room_id": "explicit-room"}
	if err := h.scopeAppPlatformRoomConversation("agent-manager", "room_task_get", "feishu-conversation:example", args); err != nil || args["room_id"] != "explicit-room" {
		t.Fatal("non-CSGClaw conversation changed", args, err)
	}
}

func TestAppRoomToolScopeSeparatesRoomsAndWorkerTasks(t *testing.T) {
	h := newAppTaskTestHandler(t)
	for _, title := range []string{"A", "B"} {
		room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: title, CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeOnDemand})
		if err != nil {
			t.Fatal(err)
		}
		for _, actor := range []string{"agent-manager", "agent-dev"} {
			for _, tool := range []string{"room_task_create", "room_task_dispatch", "room_task_update", "room_tasks_list", "room_tasks_retry_delivery"} {
				args := map[string]any{"room_id": "room-made-from-message-id"}
				if err := h.scopeAppPlatformRoomConversation(actor, tool, roomToolConversation(t, room.ID, "task-1"), args); err != nil || args["room_id"] != room.ID {
					t.Fatalf("%s/%s/%s: %v %v", title, actor, tool, args, err)
				}
			}
		}
	}
}

func TestAppRoomToolScopeRequiresValidNativeMetadata(t *testing.T) {
	h := newAppTaskTestHandler(t)
	client := taskMCPClient(t, h, "agent-manager")
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "room_task_create", Arguments: json.RawMessage("null"),
		Meta: mcp.Meta{"x-codex-turn-metadata": map[string]any{"thread_id": "thread", "turn_id": "turn"}},
	})
	if err != nil || !result.IsError || len(result.Content) != 1 {
		t.Fatalf("null arguments were not rejected: %v %v", result, err)
	}
	if content, ok := result.Content[0].(*mcp.TextContent); !ok || content.Text != "invalid tool arguments" {
		t.Fatalf("null arguments reached native scope resolution: %v", result.Content)
	}
	args := map[string]any{"room_id": "explicit-room"}
	request := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}}
	if err := h.scopeAppPlatformRoomTool("agent-manager", "room_task_create", request, args); err != nil || args["room_id"] != "explicit-room" {
		t.Fatal("legacy client changed", args, err)
	}
	request.Params.Meta = mcp.Meta{"x-codex-turn-metadata": map[string]any{"thread_id": "thread", "session_id": "other", "turn_id": "turn"}}
	if err := h.scopeAppPlatformRoomTool("agent-manager", "room_task_create", request, args); err == nil {
		t.Fatal("mixed native metadata accepted")
	}
	request.Params.Meta = mcp.Meta{"x-codex-turn-metadata": map[string]any{"thread_id": "thread", "turn_id": "turn"}}
	if err := h.scopeAppPlatformRoomTool("agent-manager", "agent_task_create", request, args); err == nil {
		t.Fatal("inactive native runtime accepted")
	}
}
