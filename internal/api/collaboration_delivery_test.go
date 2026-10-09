package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/taskcore"
)

func TestAgentChannelMessagesUseRoomAndSenderScope(t *testing.T) {
	h, alice, bob, token, _ := newAppPlatformAuthFixture(t)
	h.im = im.NewService()
	for _, a := range []struct{ id, name string }{{alice.ID, alice.Name}, {bob.ID, bob.Name}} {
		if _, _, err := h.im.EnsureAgentUser(im.EnsureAgentUserRequest{ID: a.id, Name: a.name, Role: "worker"}); err != nil {
			t.Fatal(err)
		}
	}
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Member room", CreatorID: "admin", MemberIDs: []string{alice.Name}})
	if err != nil {
		t.Fatal(err)
	}
	other, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Other room", CreatorID: "admin", MemberIDs: []string{bob.Name}})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/messages", "/api/v1/channels/csgclaw/messages", "/api/v1/channels/csgclaw/messages/"} {
		t.Run(path, func(t *testing.T) {
			for i := range 5 {
				payload := map[string]string{"room_id": room.ID, "content": "hello"}
				if i%2 == 1 {
					payload["sender_id"] = h.agentPlatformParticipant(alice.ID)
				}
				body, _ := json.Marshal(payload)
				response := appAuthRequest(t, h, http.MethodPost, path, string(body), token, nil)
				if response.Code != http.StatusCreated {
					t.Fatalf("send status=%d: %s", response.Code, response.Body.String())
				}
				var message im.Message
				if err := json.Unmarshal(response.Body.Bytes(), &message); err != nil || !h.participantBridgeTargetForRoomMember(h.agentPlatformParticipant(alice.ID)).matches(message.SenderID) {
					t.Fatalf("sender was not derived from caller: %+v, %v", message, err)
				}
				if response := appAuthRequest(t, h, http.MethodGet, path+"?room_id="+room.ID, "", token, nil); response.Code != http.StatusOK {
					t.Fatalf("list status=%d: %s", response.Code, response.Body.String())
				}
			}
			for _, payload := range []map[string]string{
				{"room_id": room.ID, "sender_id": bob.Name, "content": "spoofed"},
				{"room_id": other.ID, "content": "not a member"},
			} {
				body, _ := json.Marshal(payload)
				if response := appAuthRequest(t, h, http.MethodPost, path, string(body), token, nil); response.Code != http.StatusForbidden {
					t.Fatalf("out-of-scope send status=%d", response.Code)
				}
			}
			if response := appAuthRequest(t, h, http.MethodGet, path+"?room_id="+other.ID, "", token, nil); response.Code != http.StatusForbidden {
				t.Fatalf("out-of-scope list status=%d", response.Code)
			}
		})
	}
}

func TestAgentTaskLookupUsesRoomMembership(t *testing.T) {
	h, alice, bob, token, _ := newAppPlatformAuthFixture(t)
	h.im = im.NewService()
	for _, a := range []struct{ id, name string }{{alice.ID, alice.Name}, {bob.ID, bob.Name}} {
		if _, _, err := h.im.EnsureAgentUser(im.EnsureAgentUserRequest{ID: a.id, Name: a.name, Role: "worker"}); err != nil {
			t.Fatal(err)
		}
	}
	h.SetRoomTaskCore(taskcore.NewService())
	for _, name := range []string{alice.Name, bob.Name} {
		room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: name, CreatorID: "admin", MemberIDs: []string{name}, Type: apitypes.RoomTypeOnDemand})
		if err != nil {
			t.Fatal(err)
		}
		source, err := h.im.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Content: "Introduce yourself"})
		if err != nil {
			t.Fatal(err)
		}
		task, err := h.roomTaskSvc.Create(room.ID, source.ID, "admin", "Introductions", "")
		if err != nil {
			t.Fatal(err)
		}
		want := http.StatusForbidden
		if name == alice.Name {
			want = http.StatusOK
		}
		for range 5 {
			response := appAuthRequest(t, h, http.MethodGet, "/api/v1/tasks/"+task.ID, "", token, nil)
			if response.Code != want {
				t.Fatalf("lookup %s status=%d want=%d: %s", name, response.Code, want, response.Body.String())
			}
		}
	}
	for _, path := range []string{"/api/v1/tasks", "/api/v1/tasks/missing"} {
		if response := appAuthRequest(t, h, http.MethodGet, path, "", token, nil); response.Code != http.StatusForbidden {
			t.Fatalf("unscoped task read %s status=%d", path, response.Code)
		}
	}
}

func TestManagerMessageMentionsRespectRoomDispatchWithoutNativeMetadata(t *testing.T) {
	h := newAppTaskTestHandler(t)
	h.registerAppRoomTools("agent-manager")
	client := taskMCPClient(t, h, "agent-manager")
	for _, roomType := range []apitypes.RoomType{apitypes.RoomTypeOnDemand, apitypes.RoomTypeFree} {
		room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Room messages", CreatorID: "admin", MemberIDs: []string{"manager", "dev"}, Type: roomType})
		if err != nil {
			t.Fatal(err)
		}
		before, _ := h.im.ListMessages(room.ID)
		if roomType == apitypes.RoomTypeOnDemand {
			for _, args := range []map[string]any{
				{"room_id": room.ID, "mention_id": "pt-dev", "content": "Participate"},
				{"room_id": room.ID, "content": `<at user_id="pt-dev">dev</at> Participate`},
				{"room_id": room.ID, "content": "@dev Participate"},
			} {
				denyTaskTool(t, client, "message_send", args)
			}
			after, _ := h.im.ListMessages(room.ID)
			if len(after) != len(before) || len(h.roomTaskSvc.List(room.ID)) != 0 {
				t.Fatal("rejected ordinary mention changed room messages or tasks")
			}
			callTaskTool(t, client, "message_send", map[string]any{"room_id": room.ID, "content": "Progress announcement"}, nil)
			continue
		}
		var message im.Message
		callTaskTool(t, client, "message_send", map[string]any{"room_id": room.ID, "mention_id": "pt-dev", "content": "A room message"}, &message)
		after, _ := h.im.ListMessages(room.ID)
		if len(after) != len(before)+1 || after[len(after)-1].ID != message.ID || !strings.HasSuffix(message.Content, "A room message") || len(message.Mentions) != 1 || !h.participantBridgeTargetForRoomMember("dev").matches(message.Mentions[0].ID) {
			t.Fatalf("%s mention was not preserved as a room message: %+v", roomType, message)
		}
		if !h.participantBridgeTargetForRoomMember("manager").matches(message.SenderID) {
			t.Fatalf("sender was not derived from Manager: %+v", message)
		}
		if tasks := h.roomTaskSvc.List(room.ID); len(tasks) != 0 {
			t.Fatalf("ordinary mention created tasks: %+v", tasks)
		}
	}
}
