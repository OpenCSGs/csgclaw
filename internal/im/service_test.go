package im

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureWorkerUserCreatesUserAndBootstrapRoom(t *testing.T) {
	svc := NewService()

	user, room, err := svc.EnsureAgentUser(EnsureAgentUserRequest{
		ID:     "u-alice",
		Name:   "Alice",
		Handle: "alice",
		Role:   "Worker",
	})
	if err != nil {
		t.Fatalf("EnsureWorkerUser() error = %v", err)
	}
	if user.ID != "u-alice" || user.Handle != "alice" {
		t.Fatalf("EnsureWorkerUser() user = %+v, want id/handle set", user)
	}
	if room == nil {
		t.Fatal("EnsureWorkerUser() room = nil, want bootstrap room")
	}
	if !room.IsDirect {
		t.Fatalf("EnsureWorkerUser() room.IsDirect = %v, want true", room.IsDirect)
	}
	if len(room.Members) != 2 || !containsUserIDInRoom(*room, "u-admin") || !containsUserIDInRoom(*room, "u-alice") {
		t.Fatalf("EnsureWorkerUser() room members = %+v, want admin and worker", room.Members)
	}
}

func TestEnsureWorkerUserRejectsDuplicateHandle(t *testing.T) {
	svc := NewService()
	_, _, err := svc.EnsureAgentUser(EnsureAgentUserRequest{
		ID:     "u-alice",
		Name:   "Alice",
		Handle: "alice",
	})
	if err != nil {
		t.Fatalf("EnsureWorkerUser() first call error = %v", err)
	}

	_, _, err = svc.EnsureAgentUser(EnsureAgentUserRequest{
		ID:     "u-bob",
		Name:   "Bob",
		Handle: "alice",
	})
	if err == nil {
		t.Fatal("EnsureWorkerUser() duplicate handle error = nil, want error")
	}
}

func TestListMembersReturnsRoomMembers(t *testing.T) {
	svc := NewServiceFromBootstrap(Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "u-admin", Name: "Admin", Handle: "admin", Role: "admin"},
			{ID: "u-alice", Name: "Alice", Handle: "alice", Role: "worker"},
		},
		Rooms: []Room{
			{ID: "room-1", Title: "Ops", Members: []string{"u-admin", "u-alice"}},
		},
	})

	members, err := svc.ListMembers("room-1")
	if err != nil {
		t.Fatalf("ListMembers() error = %v", err)
	}
	if len(members) != 2 || members[0].ID != "u-admin" || members[1].ID != "u-alice" {
		t.Fatalf("ListMembers() = %+v, want room members in member order", members)
	}
}

func TestAddAgentToRoomSupportsRoomID(t *testing.T) {
	svc := NewService()

	_, _, err := svc.EnsureAgentUser(EnsureAgentUserRequest{
		ID:     "u-alice",
		Name:   "Alice",
		Handle: "alice",
		Role:   "Worker",
	})
	if err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}

	room, err := svc.CreateRoom(CreateRoomRequest{
		Title:     "Ops",
		CreatorID: "u-admin",
		MemberIDs: []string{"u-manager"},
	})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	updated, err := svc.AddAgentToRoom(AddAgentToConversationRequest{
		AgentID:   "u-alice",
		RoomID:    room.ID,
		InviterID: "u-admin",
	})
	if err != nil {
		t.Fatalf("AddAgentToRoom() error = %v", err)
	}
	if !containsUserIDInRoom(updated, "u-alice") {
		t.Fatalf("AddAgentToRoom() members = %+v, want agent joined", updated.Members)
	}
	last := updated.Messages[len(updated.Messages)-1]
	if last.Event == nil || last.Event.Key != "room_members_added" || last.Event.ActorID != "u-admin" {
		t.Fatalf("AddAgentToRoom() event = %+v, want structured room_members_added by u-admin", last)
	}
	if len(last.Event.TargetIDs) != 1 || last.Event.TargetIDs[0] != "u-alice" {
		t.Fatalf("AddAgentToRoom() target_ids = %+v, want [u-alice]", last.Event.TargetIDs)
	}
	if last.Content != "" {
		t.Fatalf("AddAgentToRoom() content = %q, want empty structured event content", last.Content)
	}
}

func TestCreateRoomStoresStructuredEvent(t *testing.T) {
	svc := NewService()

	room, err := svc.CreateRoom(CreateRoomRequest{
		Title:     "Ops",
		CreatorID: "u-admin",
		MemberIDs: []string{"u-manager"},
		Locale:    "en",
	})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if len(room.Messages) != 1 {
		t.Fatalf("CreateRoom() messages = %d, want 1", len(room.Messages))
	}
	if room.IsDirect {
		t.Fatalf("CreateRoom() room.IsDirect = %v, want false", room.IsDirect)
	}
	got := room.Messages[0]
	if got.Kind != MessageKindEvent || got.Event == nil || got.Event.Key != "room_created" || got.Event.ActorID != "u-admin" || got.Event.Title != "Ops" {
		t.Fatalf("CreateRoom() event = %+v, want structured room_created event", got)
	}
	if got.Content != "" {
		t.Fatalf("CreateRoom() content = %q, want empty structured event content", got.Content)
	}
}

func TestCreateMessagePrefixesMentionTag(t *testing.T) {
	svc := NewServiceFromBootstrap(Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-dev", Name: "dev", Handle: "dev"},
			{ID: "u-manager", Name: "manager", Handle: "manager"},
		},
		Rooms: []Room{
			{ID: "room-1", Title: "Ops", Members: []string{"u-admin", "u-dev", "u-manager"}},
		},
	})

	message, err := svc.CreateMessage(CreateMessageRequest{
		RoomID:    "room-1",
		SenderID:  "u-admin",
		Content:   "hi",
		MentionID: "u-dev",
	})
	if err != nil {
		t.Fatalf("CreateMessage() error = %v", err)
	}
	if message.Content != `<at user_id="u-dev">dev</at> hi` {
		t.Fatalf(`CreateMessage() content = %q, want <at user_id="u-dev">dev</at> hi`, message.Content)
	}
	if len(message.Mentions) != 1 || message.Mentions[0].ID != "u-dev" || message.Mentions[0].Name != "dev" {
		t.Fatalf("CreateMessage() mentions = %+v, want [u-dev]", message.Mentions)
	}
}

func TestCreateMessageWithMissingMentionIDFails(t *testing.T) {
	svc := NewServiceFromBootstrap(Bootstrap{
		CurrentUserID: "u-admin",
		Users:         []User{{ID: "u-admin", Name: "admin", Handle: "admin"}},
		Rooms:         []Room{{ID: "room-1", Title: "Ops", Members: []string{"u-admin"}}},
	})

	message, err := svc.CreateMessage(CreateMessageRequest{
		RoomID:    "room-1",
		SenderID:  "u-admin",
		Content:   "hi",
		MentionID: "u-missing",
	})
	if err == nil {
		t.Fatalf("CreateMessage() error = nil, want mentioned user not found")
	}
	if message.Content != "" {
		t.Fatalf("CreateMessage() content = %q, want empty on error", message.Content)
	}
}

func TestListRoomsUsersAndMessages(t *testing.T) {
	earlier := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	later := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	svc := NewServiceFromBootstrap(Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "u-zed", Name: "Zed", Handle: "zed", Role: "Worker"},
			{ID: "u-alice", Name: "Alice", Handle: "alice", Role: "Worker"},
		},
		Rooms: []Room{
			{
				ID:       "room-1",
				Title:    "First",
				Members:  []string{"u-admin", "u-alice"},
				Messages: []Message{{ID: "msg-1", SenderID: "u-admin", Content: "first", CreatedAt: earlier}},
			},
			{
				ID:       "room-2",
				Title:    "Second",
				Members:  []string{"u-admin", "u-zed"},
				Messages: []Message{{ID: "msg-2", SenderID: "u-zed", Content: "second", CreatedAt: later}},
			},
		},
	})

	rooms := svc.ListRooms()
	if len(rooms) != 2 {
		t.Fatalf("len(ListRooms()) = %d, want 2", len(rooms))
	}
	if rooms[0].ID != "room-2" || rooms[1].ID != "room-1" {
		t.Fatalf("ListRooms() order = [%s, %s], want room-2 then room-1", rooms[0].ID, rooms[1].ID)
	}

	users := svc.ListUsers()
	if len(users) != 4 {
		t.Fatalf("len(ListUsers()) = %d, want 4 including ensured admin/manager", len(users))
	}
	if users[0].Name != "admin" || users[1].Name != "alice" || users[2].Name != "manager" || users[3].Name != "zed" {
		t.Fatalf("ListUsers() order = [%s, %s, %s, %s], want admin, alice, manager, zed", users[0].Name, users[1].Name, users[2].Name, users[3].Name)
	}

	gotMessages, err := svc.ListMessages("room-1")
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if len(gotMessages) != 1 || gotMessages[0].ID != "msg-1" {
		t.Fatalf("ListMessages() = %+v, want msg-1", gotMessages)
	}

	if _, err := svc.ListMessages(""); err == nil {
		t.Fatal("ListMessages(\"\") error = nil, want error")
	}
	if _, err := svc.ListMessages("missing"); err == nil {
		t.Fatal("ListMessages(\"missing\") error = nil, want error")
	}
}

func TestDeleteRoomRemovesRoom(t *testing.T) {
	svc := NewServiceFromBootstrap(Bootstrap{
		CurrentUserID: "u-admin",
		Rooms: []Room{
			{ID: "room-1", Title: "Room One", Members: []string{"u-admin", "u-manager"}},
		},
	})

	if err := svc.DeleteRoom("room-1"); err != nil {
		t.Fatalf("DeleteRoom() error = %v", err)
	}
	if _, ok := svc.Room("room-1"); ok {
		t.Fatal("Room() ok = true, want false after delete")
	}
}

func TestDeleteUserRemovesUserFromStateConversationsAndMessages(t *testing.T) {
	svc := NewServiceFromBootstrap(Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-alice", Name: "Alice", Handle: "alice"},
			{ID: "u-bob", Name: "Bob", Handle: "bob"},
		},
		Rooms: []Room{
			{
				ID:      "room-group",
				Title:   "Group",
				Members: []string{"u-admin", "u-alice", "u-bob"},
				Messages: []Message{
					{ID: "msg-1", SenderID: "u-alice", Content: "hello"},
					{ID: "msg-2", SenderID: "u-bob", Content: "world"},
				},
			},
			{
				ID:       "room-dm",
				Title:    "Alice",
				IsDirect: true,
				Members:  []string{"u-admin", "u-alice"},
				Messages: []Message{{ID: "msg-3", SenderID: "u-alice", Content: "ping"}},
			},
		},
	})

	if err := svc.DeleteUser("u-alice"); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}
	if _, ok := svc.User("u-alice"); ok {
		t.Fatal("User() ok = true, want false after delete")
	}

	group, ok := svc.Room("room-group")
	if !ok {
		t.Fatal("Room(room-group) ok = false, want true")
	}
	if containsUserIDInRoom(group, "u-alice") {
		t.Fatalf("group members = %+v, want u-alice removed", group.Members)
	}
	if len(group.Messages) != 1 || group.Messages[0].SenderID != "u-bob" {
		t.Fatalf("group messages = %+v, want only u-bob message", group.Messages)
	}

	if _, ok := svc.Room("room-dm"); ok {
		t.Fatal("Room(room-dm) ok = true, want DM deleted after user delete")
	}
}

func TestPresentRoomKeepsTwoMemberGroupTitle(t *testing.T) {
	svc := NewServiceFromBootstrap(Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-alice", Name: "alice", Handle: "alice"},
		},
		Rooms: []Room{
			{
				ID:       "room-1",
				Title:    "incident-war-room",
				IsDirect: false,
				Members:  []string{"u-admin", "u-alice"},
			},
		},
	})

	room, ok := svc.Room("room-1")
	if !ok {
		t.Fatal("Room(room-1) ok = false, want true")
	}
	if room.Title != "incident-war-room" {
		t.Fatalf("Room(room-1).Title = %q, want incident-war-room", room.Title)
	}
}

func TestAddRoomMembersRejectsDirectRoom(t *testing.T) {
	svc := NewServiceFromBootstrap(Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-alice", Name: "alice", Handle: "alice"},
			{ID: "u-bob", Name: "bob", Handle: "bob"},
		},
		Rooms: []Room{
			{
				ID:       "room-1",
				Title:    "alice",
				IsDirect: true,
				Members:  []string{"u-admin", "u-alice"},
			},
		},
	})

	_, err := svc.AddRoomMembers(AddRoomMembersRequest{
		RoomID:    "room-1",
		InviterID: "u-admin",
		UserIDs:   []string{"u-bob"},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot add members to direct room") {
		t.Fatalf("AddRoomMembers() error = %v, want direct room error", err)
	}
}

func TestDeleteUserRejectsCurrentUser(t *testing.T) {
	svc := NewService()

	if err := svc.DeleteUser("u-admin"); err == nil {
		t.Fatal("DeleteUser(current user) error = nil, want error")
	}
}

func TestDeleteUserPublishesUserDeletedEvent(t *testing.T) {
	bus := NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()

	svc := NewServiceFromBootstrapWithBus(Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-alice", Name: "Alice", Handle: "alice"},
		},
	}, bus)

	if err := svc.DeleteUser("u-alice"); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}

	evt := mustReceiveEvent(t, events)
	if evt.Type != EventTypeUserDeleted || evt.User == nil || evt.User.ID != "u-alice" {
		t.Fatalf("event = %+v, want user_deleted for u-alice", evt)
	}
}

func TestSaveBootstrapSplitsRoomMessagesIntoSessionFiles(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	createdAt := time.Date(2026, 4, 9, 4, 31, 18, 753589000, time.UTC)

	state := Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-manager", Name: "manager", Handle: "manager"},
		},
		Rooms: []Room{
			{
				ID:      "room-1775709078753586000",
				Title:   "0409-1231",
				Members: []string{"u-admin", "u-manager"},
				Messages: []Message{
					{
						ID:        "msg-1775709078753589000",
						SenderID:  "u-admin",
						Kind:      MessageKindEvent,
						Content:   "",
						Event:     &EventPayload{Key: "room_created"},
						CreatedAt: createdAt,
					},
				},
			},
		},
	}

	if err := SaveBootstrap(statePath, state); err != nil {
		t.Fatalf("SaveBootstrap() error = %v", err)
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile(state.json) error = %v", err)
	}

	var persisted struct {
		Rooms []struct {
			ID       string `json:"id"`
			Messages string `json:"messages"`
		} `json:"rooms"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("Unmarshal(state.json) error = %v", err)
	}
	if len(persisted.Rooms) != 1 {
		t.Fatalf("len(rooms) = %d, want 1", len(persisted.Rooms))
	}
	if persisted.Rooms[0].Messages != "sessions/room-1775709078753586000.jsonl" {
		t.Fatalf("room.messages = %q, want session path", persisted.Rooms[0].Messages)
	}
	if strings.Contains(string(data), "\"sender_id\"") {
		t.Fatalf("state.json = %s, want room messages stored out of line", string(data))
	}

	sessionPath := filepath.Join(dir, "sessions", "room-1775709078753586000.jsonl")
	sessionData, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("ReadFile(session) error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(sessionData)), "\n")
	if len(lines) != 1 {
		t.Fatalf("session lines = %d, want 1", len(lines))
	}

	var message Message
	if err := json.Unmarshal([]byte(lines[0]), &message); err != nil {
		t.Fatalf("Unmarshal(session line) error = %v", err)
	}
	if message.ID != "msg-1775709078753589000" {
		t.Fatalf("message.ID = %q, want saved message", message.ID)
	}
}

func TestLoadBootstrapSupportsExternalSessionFiles(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	stateJSON := `{
  "current_user_id": "u-admin",
  "users": [
    {"id": "u-admin", "name": "admin", "handle": "admin"},
    {"id": "u-manager", "name": "manager", "handle": "manager"}
  ],
  "rooms": [
    {
      "id": "room-1",
      "title": "alpha",
      "subtitle": "",
      "members": ["u-admin", "u-manager"],
      "messages": "sessions/room-1.jsonl"
    }
  ]
}`
	if err := os.WriteFile(statePath, []byte(stateJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(state.json) error = %v", err)
	}

	sessionLine := `{"id":"msg-1","sender_id":"u-admin","kind":"message","content":"hello","created_at":"2026-04-09T04:31:18.753589Z","mentions":["u-manager"]}` + "\n"
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o755); err != nil {
		t.Fatalf("MkdirAll(sessions) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions", "room-1.jsonl"), []byte(sessionLine), 0o600); err != nil {
		t.Fatalf("WriteFile(session) error = %v", err)
	}

	state, err := LoadBootstrap(statePath)
	if err != nil {
		t.Fatalf("LoadBootstrap() error = %v", err)
	}
	if len(state.Rooms) != 1 {
		t.Fatalf("len(Rooms) = %d, want 1", len(state.Rooms))
	}
	if len(state.Rooms[0].Messages) != 1 || state.Rooms[0].Messages[0].ID != "msg-1" {
		t.Fatalf("room.Messages = %+v, want msg-1 from session file", state.Rooms[0].Messages)
	}
}

func TestLoadBootstrapRejectsLegacyInlineMessages(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	stateJSON := `{
  "current_user_id": "u-admin",
  "users": [
    {"id": "u-admin", "name": "admin", "handle": "admin"},
    {"id": "u-manager", "name": "manager", "handle": "manager"}
  ],
  "rooms": [
    {
      "id": "room-1",
      "title": "alpha",
      "subtitle": "",
      "members": ["u-admin", "u-manager"],
      "messages": [
        {"id":"msg-1","sender_id":"u-admin","kind":"message","content":"hello","created_at":"2026-04-09T04:31:18.753589Z","mentions":null}
      ]
    }
  ]
}`
	if err := os.WriteFile(statePath, []byte(stateJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(state.json) error = %v", err)
	}

	_, err := LoadBootstrap(statePath)
	if err == nil {
		t.Fatal("LoadBootstrap() error = nil, want legacy inline messages rejected")
	}
	if !strings.Contains(err.Error(), "decode im bootstrap") {
		t.Fatalf("LoadBootstrap() error = %v, want decode im bootstrap error", err)
	}
}

func TestEnsureBootstrapStateCreatesAdminManagerDMWhenOnlyGroupExists(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	state := Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-manager", Name: "manager", Handle: "manager"},
			{ID: "u-alice", Name: "alice", Handle: "alice"},
		},
		Rooms: []Room{
			{
				ID:          "room-group",
				Title:       "ops",
				IsDirect:    false,
				Description: "group room",
				Members:     []string{"u-admin", "u-manager", "u-alice"},
			},
		},
	}
	if err := SaveBootstrap(statePath, state); err != nil {
		t.Fatalf("SaveBootstrap() error = %v", err)
	}

	if err := EnsureBootstrapState(statePath); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}

	loaded, err := LoadBootstrap(statePath)
	if err != nil {
		t.Fatalf("LoadBootstrap() error = %v", err)
	}

	if len(loaded.Rooms) != 2 {
		t.Fatalf("len(Rooms) = %d, want 2", len(loaded.Rooms))
	}

	var dm *Room
	for i := range loaded.Rooms {
		room := &loaded.Rooms[i]
		if room.IsDirect && len(room.Members) == 2 && containsUserIDInRoom(*room, "u-admin") && containsUserIDInRoom(*room, "u-manager") {
			dm = room
			break
		}
	}
	if dm == nil {
		t.Fatalf("Rooms = %+v, want admin-manager DM created in addition to existing group", loaded.Rooms)
	}
	if dm.Title != "admin & manager" {
		t.Fatalf("dm.Title = %q, want admin & manager", dm.Title)
	}
}

func TestReloadRefreshesRoomsFromStateFile(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	initial := Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "u-admin", Name: "admin", Handle: "admin", Role: "admin"},
			{ID: "u-manager", Name: "manager", Handle: "manager", Role: "manager"},
		},
	}
	if err := SaveBootstrap(statePath, initial); err != nil {
		t.Fatalf("SaveBootstrap() initial error = %v", err)
	}

	svc, err := NewServiceFromPath(statePath)
	if err != nil {
		t.Fatalf("NewServiceFromPath() error = %v", err)
	}
	if rooms := svc.ListRooms(); len(rooms) != 0 {
		t.Fatalf("initial rooms = %+v, want none", rooms)
	}

	updated := Bootstrap{
		CurrentUserID: "u-admin",
		Users:         initial.Users,
		Rooms: []Room{
			{
				ID:       "room-1",
				Title:    "admin & manager",
				IsDirect: true,
				Members:  []string{"u-admin", "u-manager"},
			},
		},
	}
	if err := SaveBootstrap(statePath, updated); err != nil {
		t.Fatalf("SaveBootstrap() updated error = %v", err)
	}

	if err := svc.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	rooms := svc.ListRooms()
	if len(rooms) != 1 || rooms[0].ID != "room-1" {
		t.Fatalf("rooms after reload = %+v, want room-1", rooms)
	}
}

func TestRoomIDsForMember(t *testing.T) {
	svc := NewServiceFromBootstrap(Bootstrap{
		CurrentUserID: "u-admin",
		Users: []User{
			{ID: "u-bot", Name: "Bot", Handle: "bot", Role: "Worker"},
		},
		Rooms: []Room{
			{ID: "room-a", Title: "A", Members: []string{"u-admin", "u-bot"}},
			{ID: "room-b", Title: "B", Members: []string{"u-admin"}},
		},
	})
	ids := svc.RoomIDsForMember("u-bot")
	if len(ids) != 1 || ids[0] != "room-a" {
		t.Fatalf("RoomIDsForMember(u-bot) = %#v, want [room-a]", ids)
	}
	if len(svc.RoomIDsForMember("unknown")) != 0 {
		t.Fatal("expected empty for unknown user")
	}
}
