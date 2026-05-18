package feishu

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/config"
	"csgclaw/internal/im"
)

func TestFeishuServiceDoesNotPersistState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "channels", "feishu", "state.json")
	svc := NewService()

	if _, err := svc.CreateUser(CreateUserRequest{ID: "fsu-alice", Name: "Alice"}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("state.json exists after Feishu operation; stat error = %v", err)
	}
}

func TestFeishuServiceInitializesMessageBus(t *testing.T) {
	svc := NewService()

	if svc.MessageBus() == nil {
		t.Fatal("MessageBus() = nil, want initialized bus")
	}
}

func testBotInfoResolver(t *testing.T, openIDsByAppID map[string]string) func(context.Context, AppConfig) (BotInfo, error) {
	t.Helper()
	return func(_ context.Context, app AppConfig) (BotInfo, error) {
		openID, ok := openIDsByAppID[app.AppID]
		if !ok {
			t.Fatalf("unexpected app_id %q", app.AppID)
		}
		return BotInfo{OpenID: openID}, nil
	}
}

func TestFeishuServiceKeepsNamedAppConfigs(t *testing.T) {
	svc := NewService(map[string]AppConfig{
		"manager": {
			AppID:       "cli_manager",
			AppSecret:   "manager-secret",
			AdminOpenID: "ou_admin",
		},
		"dev": {
			AppID:     "cli_dev",
			AppSecret: "dev-secret",
		},
	})

	apps := svc.AppConfigs()
	if got, want := apps["manager"].AppID, "cli_manager"; got != want {
		t.Fatalf("manager app_id = %q, want %q", got, want)
	}
	if got, want := apps["manager"].AdminOpenID, "ou_admin"; got != want {
		t.Fatalf("manager admin_open_id = %q, want %q", got, want)
	}
	if got, want := apps["dev"].AppSecret, "dev-secret"; got != want {
		t.Fatalf("dev app_secret = %q, want %q", got, want)
	}

	apps["manager"] = AppConfig{AppID: "mutated"}
	if got, want := svc.AppConfigs()["manager"].AppID, "cli_manager"; got != want {
		t.Fatalf("manager app_id after caller mutation = %q, want %q", got, want)
	}
}

func TestFeishuServiceUsesProviderForConfigOperations(t *testing.T) {
	dir := t.TempDir()
	provider, err := NewProvider(NewFileStore(filepath.Join(dir, config.ConfigFileName)))
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	svc := NewServiceWithProvider(provider)

	view, err := svc.UpdateConfig(Update{
		BotID:       "u-dev",
		AppID:       "cli_dev",
		AppSecret:   "dev-secret",
		AdminOpenID: "ou_admin",
	})
	if err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}
	if !view.Configured || !view.HasSecret || view.AdminOpenID != "ou_admin" {
		t.Fatalf("UpdateConfig() view = %+v, want configured masked entry", view)
	}

	got, err := svc.GetConfig("u-dev")
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
	if got.AppID != "cli_dev" || !got.HasSecret || got.AdminOpenID != "ou_admin" {
		t.Fatalf("GetConfig() = %+v, want saved provider config", got)
	}
}

func TestFeishuServiceReloadConfigSyncsAppsAndHookFromProvider(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(filepath.Join(dir, config.ConfigFileName))
	if err := store.Save(Snapshot{
		AdminOpenID: "ou_admin",
		Bots: map[string]AppConfig{
			"u-dev": {AppID: "cli_dev", AppSecret: "dev-secret"},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	provider, err := NewProvider(store)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	svc := NewServiceWithProvider(provider)

	if err := store.Save(Snapshot{
		AdminOpenID: "ou_new",
		Bots: map[string]AppConfig{
			"u-worker": {AppID: "cli_worker", AppSecret: "worker-secret"},
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var hookSnapshot Snapshot
	svc.SetConfigReloadHook(func(snapshot Snapshot) {
		hookSnapshot = snapshot
	})
	botIDs, err := svc.ReloadConfig()
	if err != nil {
		t.Fatalf("ReloadConfig() error = %v", err)
	}

	if got, want := strings.Join(botIDs, ","), "u-worker"; got != want {
		t.Fatalf("ReloadConfig() bot ids = %q, want %q", got, want)
	}
	apps := svc.AppConfigs()
	if _, ok := apps["u-dev"]; ok {
		t.Fatalf("AppConfigs() still contains old bot after reload: %+v", apps)
	}
	if got, want := apps["u-worker"].AdminOpenID, "ou_new"; got != want {
		t.Fatalf("reloaded admin_open_id = %q, want %q", got, want)
	}
	if got, want := hookSnapshot.Bots["u-worker"].AppID, "cli_worker"; got != want {
		t.Fatalf("reload hook snapshot = %+v, want updated snapshot", hookSnapshot)
	}
}

func TestFeishuListUsersUsesConfiguredAppsAndOpenIDs(t *testing.T) {
	svc := NewServiceWithBotOpenIDResolver(
		map[string]AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
			"u-dev":     {AppID: "cli_dev", AppSecret: "dev-secret"},
		},
		func(_ context.Context, app AppConfig) (BotInfo, error) {
			switch app.AppID {
			case "cli_manager":
				return BotInfo{OpenID: "ou_manager"}, nil
			case "cli_dev":
				return BotInfo{OpenID: "ou_dev"}, nil
			default:
				return BotInfo{}, nil
			}
		},
	)

	users := svc.ListUsers()
	if len(users) != 2 {
		t.Fatalf("len(ListUsers()) = %d, want 2", len(users))
	}
	if got, want := users[0].ID, "ou_dev"; got != want {
		t.Fatalf("users[0].ID = %q, want %q", got, want)
	}
	if got, want := users[0].Name, "u-dev"; got != want {
		t.Fatalf("users[0].Name = %q, want %q", got, want)
	}
	if got, want := users[1].ID, "ou_manager"; got != want {
		t.Fatalf("users[1].ID = %q, want %q", got, want)
	}
	if got, want := users[1].Name, "u-manager"; got != want {
		t.Fatalf("users[1].Name = %q, want %q", got, want)
	}
}

func TestFeishuResolveBotUserUsesConfiguredOpenID(t *testing.T) {
	svc := NewServiceWithBotOpenIDResolver(
		map[string]AppConfig{
			"u-alice": {AppID: "cli_alice", AppSecret: "alice-secret"},
		},
		func(_ context.Context, app AppConfig) (BotInfo, error) {
			if got, want := app.AppID, "cli_alice"; got != want {
				t.Fatalf("resolve app_id = %q, want %q", got, want)
			}
			return BotInfo{OpenID: "ou_alice"}, nil
		},
	)

	user, ok, err := svc.ResolveBotUser(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("ResolveBotUser() error = %v", err)
	}
	if !ok {
		t.Fatal("ResolveBotUser() ok = false, want true")
	}
	if got, want := user.ID, "ou_alice"; got != want {
		t.Fatalf("user.ID = %q, want %q", got, want)
	}
	if got, want := user.Name, "u-alice"; got != want {
		t.Fatalf("user.Name = %q, want %q", got, want)
	}
}

func TestFeishuEnsureUserUsesConfiguredOpenID(t *testing.T) {
	svc := NewServiceWithBotOpenIDResolver(
		map[string]AppConfig{
			"u-alice": {AppID: "cli_alice", AppSecret: "alice-secret"},
		},
		func(_ context.Context, app AppConfig) (BotInfo, error) {
			if got, want := app.AppID, "cli_alice"; got != want {
				t.Fatalf("resolve app_id = %q, want %q", got, want)
			}
			return BotInfo{OpenID: "ou_alice"}, nil
		},
	)

	user, err := svc.EnsureUser(CreateUserRequest{
		ID:     "u-alice",
		Name:   "alice",
		Handle: "alice",
		Role:   "worker",
	})
	if err != nil {
		t.Fatalf("EnsureUser() error = %v", err)
	}
	if got, want := user.ID, "ou_alice"; got != want {
		t.Fatalf("user.ID = %q, want %q", got, want)
	}
	if got, want := user.Name, "u-alice"; got != want {
		t.Fatalf("user.Name = %q, want %q", got, want)
	}
}

func TestFeishuDeleteUserRemovesUser(t *testing.T) {
	svc := NewService()

	if _, err := svc.CreateUser(CreateUserRequest{ID: "ou_alice", Name: "Alice"}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if err := svc.DeleteUser("ou_alice"); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}

	users := svc.ListUsers()
	for _, user := range users {
		if user.ID == "ou_alice" {
			t.Fatal("ListUsers() still contains deleted user")
		}
	}
}

func TestFeishuBotMembersInChatWithResolversIncludesConfiguredBots(t *testing.T) {
	apps := map[string]AppConfig{
		"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
		"u-dev":     {AppID: "cli_dev", AppSecret: "dev-secret"},
		"u-qa":      {AppID: "cli_qa", AppSecret: "qa-secret"},
	}
	seenChecks := make([]string, 0)
	members, err := feishuBotMembersInChatWithResolvers(
		context.Background(),
		apps,
		"oc_alpha",
		map[string]struct{}{"ou_existing": {}},
		func(_ context.Context, app AppConfig) (BotInfo, error) {
			switch app.AppID {
			case "cli_manager":
				return BotInfo{OpenID: "ou_manager"}, nil
			case "cli_dev":
				return BotInfo{OpenID: "ou_existing"}, nil
			case "cli_qa":
				return BotInfo{OpenID: "ou_qa"}, nil
			default:
				return BotInfo{}, nil
			}
		},
		func(_ context.Context, app AppConfig, chatID string) (bool, error) {
			if got, want := chatID, "oc_alpha"; got != want {
				t.Fatalf("chat_id = %q, want %q", got, want)
			}
			seenChecks = append(seenChecks, app.AppID)
			return app.AppID != "cli_qa", nil
		},
	)
	if err != nil {
		t.Fatalf("feishuBotMembersInChatWithResolvers() error = %v", err)
	}
	if len(seenChecks) != 3 {
		t.Fatalf("checked apps = %+v, want all configured apps", seenChecks)
	}
	if len(members) != 1 {
		t.Fatalf("members len = %d, want 1", len(members))
	}
	if got, want := members[0].ID, "u-manager"; got != want {
		t.Fatalf("member id = %q, want %q", got, want)
	}
	if got, want := members[0].Name, "u-manager"; got != want {
		t.Fatalf("member name = %q, want %q", got, want)
	}
}

func TestFeishuCreateRoomUsesConfiguredAdminOpenID(t *testing.T) {
	var gotCreatorID string
	var gotMemberAppIDs []string
	svc := NewServiceWithCreateChatAndAddMembers(
		map[string]AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"},
			"u-dev":     {AppID: "cli_dev", AppSecret: "dev-secret"},
		},
		func(_ context.Context, _ AppConfig, req CreateChatRequest) (CreateChatResponse, error) {
			gotCreatorID = req.CreatorID
			gotMemberAppIDs = append([]string(nil), req.MemberAppIDs...)
			return CreateChatResponse{ChatID: "oc_alpha", Name: req.Title, Description: req.Description}, nil
		},
		func(context.Context, AppConfig, AddChatMembersRequest) error { return nil },
	)

	if _, err := svc.CreateRoom(im.CreateRoomRequest{Title: "alpha", CreatorID: "u-manager", MemberIDs: []string{"u-dev"}}); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	if got, want := gotCreatorID, "ou_admin"; got != want {
		t.Fatalf("create chat creator_id = %q, want %q", got, want)
	}
	if len(gotMemberAppIDs) != 1 || gotMemberAppIDs[0] != "cli_dev" {
		t.Fatalf("create chat member app_ids = %+v, want [cli_dev]", gotMemberAppIDs)
	}
}

func TestFeishuCreateRoomRequiresConfiguredMemberBots(t *testing.T) {
	svc := NewServiceWithCreateChatAndAddMembers(
		map[string]AppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
		func(context.Context, AppConfig, CreateChatRequest) (CreateChatResponse, error) {
			t.Fatal("createChat should not be called for an unconfigured member bot")
			return CreateChatResponse{}, nil
		},
		func(context.Context, AppConfig, AddChatMembersRequest) error { return nil },
	)

	_, err := svc.CreateRoom(im.CreateRoomRequest{
		Title:     "alpha",
		CreatorID: "u-manager",
		MemberIDs: []string{"u-dev"},
	})
	if err == nil || !strings.Contains(err.Error(), `feishu app is not configured for bot "u-dev"`) {
		t.Fatalf("CreateRoom() error = %v, want configured bot error", err)
	}
}

func TestFeishuDeleteRoomUsesConfiguredApp(t *testing.T) {
	var gotApp AppConfig
	var gotRoomID string
	svc := NewServiceWithDeleteChat(
		map[string]AppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
		func(_ context.Context, app AppConfig, roomID string) error {
			gotApp = app
			gotRoomID = roomID
			return nil
		},
	)

	if err := svc.DeleteRoom("oc_alpha"); err != nil {
		t.Fatalf("DeleteRoom() error = %v", err)
	}
	if got, want := gotApp.AppID, "cli_manager"; got != want {
		t.Fatalf("delete app_id = %q, want %q", got, want)
	}
	if got, want := gotRoomID, "oc_alpha"; got != want {
		t.Fatalf("delete room_id = %q, want %q", got, want)
	}
}

func TestFeishuSendMessageUsesSenderAppAndStoresLocalMessage(t *testing.T) {
	var gotApp AppConfig
	var gotReq SendMessageRequest
	svc := NewServiceWithSendMessage(
		map[string]AppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"}},
		func(_ context.Context, app AppConfig, req SendMessageRequest) (SendMessageResponse, error) {
			gotApp = app
			gotReq = req
			return SendMessageResponse{MessageID: "om_1", SenderOpenID: "ou_manager"}, nil
		},
	)
	svc.rooms["oc_alpha"] = &im.Room{ID: "oc_alpha", Title: "alpha", Members: []string{"u-manager"}}

	message, err := svc.SendMessage(im.CreateMessageRequest{
		RoomID:   "oc_alpha",
		SenderID: "u-manager",
		Content:  "hello",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if gotApp.AppID != "cli_manager" {
		t.Fatalf("send app = %+v, want manager app", gotApp)
	}
	if gotReq.ChatID != "oc_alpha" || gotReq.Content != "hello" || gotReq.UUID == "" {
		t.Fatalf("send request = %+v, want chat/content/uuid", gotReq)
	}
	if message.ID != "om_1" || message.SenderID != "ou_manager" || message.Content != "hello" {
		t.Fatalf("message = %+v, want sent message", message)
	}
	if len(svc.rooms["oc_alpha"].Messages) != 1 || svc.rooms["oc_alpha"].Messages[0].ID != "om_1" {
		t.Fatalf("stored messages = %+v, want om_1", svc.rooms["oc_alpha"].Messages)
	}
}

func TestFeishuSendMessageResolvesMentionApp(t *testing.T) {
	var gotReq SendMessageRequest
	svc := NewServiceWithSendMessage(
		map[string]AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
			"u-dev":     {AppID: "cli_dev", AppSecret: "dev-secret"},
		},
		func(_ context.Context, _ AppConfig, req SendMessageRequest) (SendMessageResponse, error) {
			gotReq = req
			return SendMessageResponse{MessageID: "om_mention", SenderOpenID: "ou_manager", MentionOpenID: "ou_dev"}, nil
		},
	)
	svc.rooms["oc_alpha"] = &im.Room{ID: "oc_alpha", Title: "alpha", Members: []string{"u-manager", "u-dev"}}

	message, err := svc.SendMessage(im.CreateMessageRequest{
		RoomID:    "oc_alpha",
		SenderID:  "u-manager",
		Content:   "hello",
		MentionID: "u-dev",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if gotReq.MentionID != "u-dev" || gotReq.MentionAppConfig.AppID != "cli_dev" || gotReq.MentionAppConfig.AppSecret != "dev-secret" {
		t.Fatalf("send request = %+v, want mention app config", gotReq)
	}
	if message.SenderID != "ou_manager" {
		t.Fatalf("message sender_id = %q, want ou_manager", message.SenderID)
	}
	if len(message.Mentions) != 1 || message.Mentions[0].ID != "ou_dev" || message.Mentions[0].Name != "u-dev" {
		t.Fatalf("message mentions = %+v, want ou_dev", message.Mentions)
	}
}

func TestFeishuSendMessageWithMentionPublishesMessageEvent(t *testing.T) {
	svc := NewServiceWithSendMessage(
		map[string]AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
			"u-dev":     {AppID: "cli_dev", AppSecret: "dev-secret"},
		},
		func(_ context.Context, _ AppConfig, _ SendMessageRequest) (SendMessageResponse, error) {
			return SendMessageResponse{MessageID: "om_mention", SenderOpenID: "ou_manager", MentionOpenID: "ou_dev"}, nil
		},
	)
	svc.rooms["oc_alpha"] = &im.Room{ID: "oc_alpha", Title: "alpha", Members: []string{"u-manager", "u-dev"}}
	events, cancel := svc.MessageBus().Subscribe()
	defer cancel()

	message, err := svc.SendMessage(im.CreateMessageRequest{
		RoomID:    "oc_alpha",
		SenderID:  "u-manager",
		Content:   "hello",
		MentionID: "u-dev",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	select {
	case evt := <-events:
		if evt.Type != MessageEventTypeMessageCreated {
			t.Fatalf("event type = %q, want %q", evt.Type, MessageEventTypeMessageCreated)
		}
		if evt.RoomID != "oc_alpha" {
			t.Fatalf("event room_id = %q, want oc_alpha", evt.RoomID)
		}
		if evt.Message == nil || evt.Message.ID != message.ID {
			t.Fatalf("event message = %+v, want message %q", evt.Message, message.ID)
		}
		if evt.Message.SenderID != "ou_manager" {
			t.Fatalf("event sender_id = %q, want ou_manager", evt.Message.SenderID)
		}
		if len(evt.Message.Mentions) != 1 || evt.Message.Mentions[0].ID != "ou_dev" || evt.Message.Mentions[0].Name != "u-dev" {
			t.Fatalf("event mentions = %+v, want ou_dev", evt.Message.Mentions)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for feishu message event")
	}
}

func TestFeishuSendMessageWithoutMentionDoesNotPublishMessageEvent(t *testing.T) {
	svc := NewServiceWithSendMessage(
		map[string]AppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"}},
		func(_ context.Context, _ AppConfig, _ SendMessageRequest) (SendMessageResponse, error) {
			return SendMessageResponse{MessageID: "om_plain", SenderOpenID: "ou_manager"}, nil
		},
	)
	svc.rooms["oc_alpha"] = &im.Room{ID: "oc_alpha", Title: "alpha", Members: []string{"u-manager"}}
	events, cancel := svc.MessageBus().Subscribe()
	defer cancel()

	if _, err := svc.SendMessage(im.CreateMessageRequest{
		RoomID:   "oc_alpha",
		SenderID: "u-manager",
		Content:  "hello",
	}); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	select {
	case evt := <-events:
		t.Fatalf("unexpected event = %+v", evt)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestFeishuSendMessageRequiresMentionApp(t *testing.T) {
	svc := NewServiceWithSendMessage(
		map[string]AppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"}},
		func(context.Context, AppConfig, SendMessageRequest) (SendMessageResponse, error) {
			t.Fatal("sendMessage should not be called without mention app config")
			return SendMessageResponse{}, nil
		},
	)

	_, err := svc.SendMessage(im.CreateMessageRequest{
		RoomID:    "oc_alpha",
		SenderID:  "u-manager",
		Content:   "hello",
		MentionID: "u-dev",
	})
	if err == nil || !strings.Contains(err.Error(), `feishu app is not configured for mention "u-dev"`) {
		t.Fatalf("SendMessage() error = %v, want mention app config error", err)
	}
}

func TestFeishuCreateRoomUsesManagerAppRegardlessOfCreatorID(t *testing.T) {
	svc := NewServiceWithCreateChatAndAddMembers(
		map[string]AppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
		func(_ context.Context, app AppConfig, _ CreateChatRequest) (CreateChatResponse, error) {
			if got, want := app.AppID, "cli_manager"; got != want {
				t.Fatalf("create chat app_id = %q, want %q", got, want)
			}
			return CreateChatResponse{ChatID: "oc_alpha"}, nil
		},
		func(context.Context, AppConfig, AddChatMembersRequest) error { return nil },
	)

	room, err := svc.CreateRoom(im.CreateRoomRequest{Title: "alpha", CreatorID: "u-missing"})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	if got, want := room.ID, "oc_alpha"; got != want {
		t.Fatalf("room id = %q, want %q", got, want)
	}
}

func TestFeishuListRoomsCallsConfiguredApp(t *testing.T) {
	var gotApp AppConfig
	svc := NewServiceWithCreateChatAndAddMembers(
		map[string]AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"},
			"u-dev":     {AppID: "cli_dev", AppSecret: "dev-secret"},
		},
		func(_ context.Context, _ AppConfig, req CreateChatRequest) (CreateChatResponse, error) {
			return CreateChatResponse{ChatID: "oc_alpha", Name: req.Title, Description: req.Description}, nil
		},
		func(context.Context, AppConfig, AddChatMembersRequest) error { return nil },
	)
	svc.resolveBotInfo = testBotInfoResolver(t, map[string]string{
		"cli_manager": "ou_manager",
		"cli_dev":     "ou_dev",
	})
	svc.listChats = func(_ context.Context, app AppConfig) ([]im.Room, error) {
		gotApp = app
		return []im.Room{
			{ID: "oc_beta", Title: "beta", Members: []string{"ou_manager", "ou_external"}},
			{ID: "oc_alpha", Title: "alpha", Members: []string{"ou_manager", "ou_external"}},
		}, nil
	}

	if _, err := svc.CreateRoom(im.CreateRoomRequest{
		Title:     "alpha",
		CreatorID: "u-manager",
		MemberIDs: []string{"u-dev"},
	}); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	rooms, err := svc.ListRooms()
	if err != nil {
		t.Fatalf("ListRooms() error = %v", err)
	}

	if got, want := gotApp.AppID, "cli_manager"; got != want {
		t.Fatalf("list rooms app_id = %q, want %q", got, want)
	}
	if len(rooms) != 2 {
		t.Fatalf("rooms len = %d, want 2", len(rooms))
	}
	if got, want := rooms[0].ID, "oc_alpha"; got != want {
		t.Fatalf("first room id = %q, want %q", got, want)
	}
	if got, want := strings.Join(rooms[0].Members, ","), "u-manager,ou_external"; got != want {
		t.Fatalf("first room members = %+v, want realtime mapped members", rooms[0].Members)
	}
	if got, want := strings.Join(rooms[1].Members, ","), "u-manager,ou_external"; got != want {
		t.Fatalf("uncached room members = %+v, want mapped bot ids and unmapped ids preserved", rooms[1].Members)
	}
}

func TestFeishuListRoomMessagesFetchesAllMessagesAndUpdatesCache(t *testing.T) {
	var gotApp AppConfig
	var gotRoomID string
	fetchedAt := time.Unix(5, 0).UTC()
	svc := NewServiceWithListRoomMessages(
		map[string]AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
			"u-dev":     {AppID: "cli_dev", AppSecret: "dev-secret"},
		},
		func(_ context.Context, app AppConfig, roomID string) ([]im.Message, error) {
			gotApp = app
			gotRoomID = roomID
			return []im.Message{
				{ID: "om_1", SenderID: "ou_manager", Kind: im.MessageKindMessage, Content: "hello", CreatedAt: fetchedAt, Mentions: []im.Mention{{ID: "ou_dev", Name: "dev"}, {ID: "ou_external", Name: "External"}}},
				{ID: "om_2", SenderID: "ou_dev", Kind: im.MessageKindMessage, Content: "world", CreatedAt: fetchedAt.Add(time.Second)},
				{ID: "om_3", SenderID: "ou_external", Kind: im.MessageKindMessage, Content: "external", CreatedAt: fetchedAt.Add(2 * time.Second)},
			}, nil
		},
	)
	svc.resolveBotInfo = testBotInfoResolver(t, map[string]string{
		"cli_manager": "ou_manager",
		"cli_dev":     "ou_dev",
	})
	svc.rooms["oc_alpha"] = &im.Room{
		ID:       "oc_alpha",
		Title:    "alpha",
		Messages: []im.Message{{ID: "om_old", Content: "old"}},
	}

	messages, err := svc.ListRoomMessages("oc_alpha")
	if err != nil {
		t.Fatalf("ListRoomMessages() error = %v", err)
	}

	if gotApp.AppID != "cli_manager" {
		t.Fatalf("list messages app = %+v, want manager app", gotApp)
	}
	if gotRoomID != "oc_alpha" {
		t.Fatalf("list messages room_id = %q, want oc_alpha", gotRoomID)
	}
	if len(messages) != 3 || messages[0].ID != "om_1" || messages[1].ID != "om_2" || messages[2].ID != "om_3" {
		t.Fatalf("messages = %+v, want fetched messages", messages)
	}
	if messages[0].SenderID != "u-manager" || messages[1].SenderID != "u-dev" || messages[2].SenderID != "ou_external" {
		t.Fatalf("message senders = %+v, want bot ids with unmapped sender preserved", messages)
	}
	if len(messages[0].Mentions) != 2 || messages[0].Mentions[0].ID != "u-dev" || messages[0].Mentions[1].ID != "ou_external" {
		t.Fatalf("message mentions = %+v, want mapped bot ids and unmapped mentions preserved", messages[0].Mentions)
	}
	if len(svc.rooms["oc_alpha"].Messages) != 3 || svc.rooms["oc_alpha"].Messages[0].ID != "om_1" || svc.rooms["oc_alpha"].Messages[0].SenderID != "u-manager" {
		t.Fatalf("cached messages = %+v, want fetched messages", svc.rooms["oc_alpha"].Messages)
	}
	messages[0].ID = "mutated"
	if got, want := svc.rooms["oc_alpha"].Messages[0].ID, "om_1"; got != want {
		t.Fatalf("cached message id after caller mutation = %q, want %q", got, want)
	}
}

func TestFeishuListRoomMessagesRequestsAPIWithoutLocalRoomValidation(t *testing.T) {
	var gotRoomIDs []string
	svc := NewServiceWithListRoomMessages(
		map[string]AppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"}},
		func(_ context.Context, _ AppConfig, roomID string) ([]im.Message, error) {
			gotRoomIDs = append(gotRoomIDs, roomID)
			return []im.Message{{ID: "om_1"}}, nil
		},
	)
	svc.resolveBotInfo = testBotInfoResolver(t, map[string]string{"cli_manager": "ou_manager"})

	if _, err := svc.ListRoomMessages(" "); err != nil {
		t.Fatalf("ListRoomMessages() with blank room_id error = %v", err)
	}
	if _, err := svc.ListRoomMessages("missing"); err != nil {
		t.Fatalf("ListRoomMessages() with missing local room error = %v", err)
	}

	if len(gotRoomIDs) != 2 || gotRoomIDs[0] != " " || gotRoomIDs[1] != "missing" {
		t.Fatalf("list messages room_ids = %+v, want blank and missing room ids passed through", gotRoomIDs)
	}
}

func TestFeishuAddRoomMembersCallsConfiguredApp(t *testing.T) {
	var gotApp AppConfig
	var gotReq AddChatMembersRequest
	svc := NewServiceWithCreateChatAndAddMembers(
		map[string]AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"},
			"u-alice":   {AppID: "cli_alice", AppSecret: "alice-secret"},
		},
		func(_ context.Context, _ AppConfig, req CreateChatRequest) (CreateChatResponse, error) {
			return CreateChatResponse{ChatID: "oc_alpha", Name: req.Title, Description: req.Description}, nil
		},
		func(_ context.Context, app AppConfig, req AddChatMembersRequest) error {
			gotApp = app
			gotReq = req
			return nil
		},
	)

	if _, err := svc.CreateUser(CreateUserRequest{ID: "u-manager", Name: "Manager"}); err != nil {
		t.Fatalf("CreateUser(manager) error = %v", err)
	}
	if _, err := svc.CreateUser(CreateUserRequest{ID: "u-alice", Name: "Alice"}); err != nil {
		t.Fatalf("CreateUser(alice) error = %v", err)
	}
	if _, err := svc.CreateRoom(im.CreateRoomRequest{Title: "alpha", CreatorID: "u-manager"}); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	room, err := svc.AddRoomMembers(im.AddRoomMembersRequest{
		RoomID:  "oc_alpha",
		UserIDs: []string{"u-alice"},
	})
	if err != nil {
		t.Fatalf("AddRoomMembers() error = %v", err)
	}

	if got, want := gotApp.AppID, "cli_manager"; got != want {
		t.Fatalf("add members app_id = %q, want %q", got, want)
	}
	if got, want := gotReq.ChatID, "oc_alpha"; got != want {
		t.Fatalf("add members chat_id = %q, want %q", got, want)
	}
	if len(gotReq.MemberBotIDs) != 1 || gotReq.MemberBotIDs[0] != "u-alice" {
		t.Fatalf("add member bot ids = %+v, want [u-alice]", gotReq.MemberBotIDs)
	}
	if len(gotReq.MemberAppIDs) != 1 || gotReq.MemberAppIDs[0] != "cli_alice" {
		t.Fatalf("add members app_ids = %+v, want [cli_alice]", gotReq.MemberAppIDs)
	}
	if len(room.Members) != 2 {
		t.Fatalf("members = %+v, want two users", room.Members)
	}
}

func TestFeishuAddRoomMembersRequiresConfiguredBot(t *testing.T) {
	svc := NewServiceWithCreateChatAndAddMembers(
		map[string]AppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
		func(_ context.Context, _ AppConfig, req CreateChatRequest) (CreateChatResponse, error) {
			return CreateChatResponse{ChatID: "oc_alpha", Name: req.Title, Description: req.Description}, nil
		},
		func(_ context.Context, _ AppConfig, req AddChatMembersRequest) error {
			t.Fatalf("addChatMembers should not be called for an unconfigured bot: %+v", req)
			return nil
		},
	)

	if _, err := svc.CreateUser(CreateUserRequest{ID: "u-manager", Name: "Manager"}); err != nil {
		t.Fatalf("CreateUser(manager) error = %v", err)
	}
	if _, err := svc.CreateRoom(im.CreateRoomRequest{Title: "alpha", CreatorID: "u-manager"}); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	_, err := svc.AddRoomMembers(im.AddRoomMembersRequest{
		RoomID:  "oc_alpha",
		UserIDs: []string{"u-alice"},
	})
	if err == nil || !strings.Contains(err.Error(), `feishu app is not configured for bot "u-alice"`) {
		t.Fatalf("AddRoomMembers() error = %v, want configured bot error", err)
	}
}

func TestFeishuAddRoomMembersLetsFeishuValidateRoomID(t *testing.T) {
	var gotReq AddChatMembersRequest
	svc := NewServiceWithCreateChatAndAddMembers(
		map[string]AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"},
			"u-alice":   {AppID: "cli_alice", AppSecret: "alice-secret"},
		},
		func(context.Context, AppConfig, CreateChatRequest) (CreateChatResponse, error) {
			t.Fatal("createChat should not be called")
			return CreateChatResponse{}, nil
		},
		func(_ context.Context, _ AppConfig, req AddChatMembersRequest) error {
			gotReq = req
			return nil
		},
	)

	room, err := svc.AddRoomMembers(im.AddRoomMembersRequest{
		RoomID:    "oc_external",
		InviterID: "u-manager",
		UserIDs:   []string{"u-alice"},
	})
	if err != nil {
		t.Fatalf("AddRoomMembers() error = %v", err)
	}
	if got, want := gotReq.ChatID, "oc_external"; got != want {
		t.Fatalf("add members chat_id = %q, want %q", got, want)
	}
	if got, want := room.ID, "oc_external"; got != want {
		t.Fatalf("room id = %q, want %q", got, want)
	}
	if len(room.Members) != 1 || room.Members[0] != "u-alice" {
		t.Fatalf("members = %+v, want [u-alice]", room.Members)
	}
}

func TestFeishuListRoomMembersCallsConfiguredApp(t *testing.T) {
	var gotApp AppConfig
	var gotRoomID string
	svc := NewServiceWithCreateChatAndAddMembers(
		map[string]AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"},
			"u-alice":   {AppID: "cli_alice", AppSecret: "alice-secret"},
		},
		func(_ context.Context, _ AppConfig, req CreateChatRequest) (CreateChatResponse, error) {
			return CreateChatResponse{ChatID: "oc_alpha", Name: req.Title, Description: req.Description}, nil
		},
		func(context.Context, AppConfig, AddChatMembersRequest) error { return nil },
	)
	svc.resolveBotInfo = testBotInfoResolver(t, map[string]string{
		"cli_manager": "ou_manager",
		"cli_alice":   "ou_alice",
	})
	svc.listChatMembers = func(_ context.Context, app AppConfig, apps map[string]AppConfig, roomID string) ([]im.User, error) {
		gotApp = app
		gotRoomID = roomID
		if got, want := apps["u-manager"].AppID, "cli_manager"; got != want {
			t.Fatalf("list members apps manager app_id = %q, want %q", got, want)
		}
		return []im.User{{ID: "ou_alice", Name: "Alice"}, {ID: "ou_external", Name: "External"}}, nil
	}

	if _, err := svc.CreateUser(CreateUserRequest{ID: "u-manager", Name: "Manager"}); err != nil {
		t.Fatalf("CreateUser(manager) error = %v", err)
	}
	if _, err := svc.CreateUser(CreateUserRequest{ID: "u-alice", Name: "Alice Local", Handle: "alice-local", Role: "worker", Avatar: "AL"}); err != nil {
		t.Fatalf("CreateUser(alice) error = %v", err)
	}
	if _, err := svc.CreateRoom(im.CreateRoomRequest{Title: "alpha", CreatorID: "u-manager"}); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}

	members, err := svc.ListRoomMembers("oc_alpha")
	if err != nil {
		t.Fatalf("ListRoomMembers() error = %v", err)
	}
	if got, want := gotApp.AppID, "cli_manager"; got != want {
		t.Fatalf("list members app_id = %q, want %q", got, want)
	}
	if got, want := gotRoomID, "oc_alpha"; got != want {
		t.Fatalf("list members room_id = %q, want %q", got, want)
	}
	if len(members) != 2 {
		t.Fatalf("members len = %d, want 2", len(members))
	}
	if got, want := members[0].ID, "u-alice"; got != want {
		t.Fatalf("first member id = %q, want bot id %q", got, want)
	}
	if got, want := members[0].Name, "Alice"; got != want {
		t.Fatalf("member name = %q, want %q", got, want)
	}
	if got, want := members[0].Handle, "alice-local"; got != want {
		t.Fatalf("member handle = %q, want %q", got, want)
	}
	if got, want := members[0].Role, "worker"; got != want {
		t.Fatalf("member role = %q, want %q", got, want)
	}
	if got, want := members[1].ID, "ou_external"; got != want {
		t.Fatalf("second member id = %q, want unmapped id %q", got, want)
	}
}

func TestFeishuListRoomMembersLetsFeishuValidateExternalRoomID(t *testing.T) {
	var gotRoomID string
	svc := NewService(map[string]AppConfig{
		"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"},
		"u-alice":   {AppID: "cli_alice", AppSecret: "alice-secret"},
	})
	svc.resolveBotInfo = testBotInfoResolver(t, map[string]string{
		"cli_manager": "ou_manager",
		"cli_alice":   "ou_alice",
	})
	svc.listChatMembers = func(_ context.Context, app AppConfig, _ map[string]AppConfig, roomID string) ([]im.User, error) {
		if got, want := app.AppID, "cli_manager"; got != want {
			t.Fatalf("list members app_id = %q, want %q", got, want)
		}
		gotRoomID = roomID
		return []im.User{{ID: "ou_alice", Name: "Alice"}, {ID: "ou_external", Name: "External"}}, nil
	}

	members, err := svc.ListRoomMembers("oc_external")
	if err != nil {
		t.Fatalf("ListRoomMembers() error = %v", err)
	}
	if got, want := gotRoomID, "oc_external"; got != want {
		t.Fatalf("list members room_id = %q, want %q", got, want)
	}
	if len(members) != 2 || members[0].ID != "u-alice" || members[1].ID != "ou_external" {
		t.Fatalf("members = %+v, want u-alice and preserved ou_external", members)
	}
}
