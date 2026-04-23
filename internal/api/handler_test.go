package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/bot"
	"csgclaw/internal/channel"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/sandboxtest"
)

func TestParsePicoClawBotPath(t *testing.T) {
	tests := []struct {
		path       string
		wantBotID  string
		wantAction string
		wantOK     bool
	}{
		{path: "/api/bots/u-manager/events", wantBotID: "u-manager", wantAction: "events", wantOK: true},
		{path: "/api/bots/u-manager/messages/send", wantBotID: "u-manager", wantAction: "messages/send", wantOK: true},
		{path: "/api/bots/u-manager/llm/models", wantBotID: "u-manager", wantAction: "llm/models", wantOK: true},
		{path: "/api/bots/u-manager/llm/v1/models", wantBotID: "u-manager", wantAction: "llm/v1/models", wantOK: true},
		{path: "/api/bots/u-manager/llm/chat/completions", wantBotID: "u-manager", wantAction: "llm/chat/completions", wantOK: true},
		{path: "/api/bots/u-manager/llm/v1/chat/completions", wantBotID: "u-manager", wantAction: "llm/v1/chat/completions", wantOK: true},
		{path: "/api/bots/u-manager", wantOK: false},
		{path: "/api/v1/bots/u-manager/events", wantOK: false},
		{path: "/api/bots//events", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			gotBotID, gotAction, gotOK := parsePicoClawBotPath(tt.path)
			if gotBotID != tt.wantBotID || gotAction != tt.wantAction || gotOK != tt.wantOK {
				t.Fatalf("parsePicoClawBotPath(%q) = (%q, %q, %v), want (%q, %q, %v)", tt.path, gotBotID, gotAction, gotOK, tt.wantBotID, tt.wantAction, tt.wantOK)
			}
		})
	}
}

func TestDeriveAgentHandle(t *testing.T) {
	tests := []struct {
		name  string
		agent agent.Agent
		want  string
	}{
		{
			name:  "plain name",
			agent: agent.Agent{Name: "Alice Smith", ID: "u-alice", Role: agent.RoleWorker},
			want:  "alice-smith",
		},
		{
			name:  "fallback to id",
			agent: agent.Agent{Name: "!!!", ID: "u-worker_01", Role: agent.RoleWorker},
			want:  "worker_01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveAgentHandle(tt.agent); got != tt.want {
				t.Fatalf("deriveAgentHandle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnsureWorkerIMStatePublishesBootstrapRoom(t *testing.T) {
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()

	srv := &Handler{
		im:    im.NewService(),
		imBus: bus,
	}

	created := agent.Agent{
		ID:          "u-alice",
		Name:        "Alice",
		Description: "test lead",
		Role:        agent.RoleWorker,
	}
	if err := srv.ensureWorkerIMState(created); err != nil {
		t.Fatalf("ensureWorkerIMState() error = %v", err)
	}

	first := mustReceiveEvent(t, events)
	if first.Type != im.EventTypeUserCreated {
		t.Fatalf("first event.Type = %q, want %q", first.Type, im.EventTypeUserCreated)
	}
	if first.User == nil || first.User.ID != "u-alice" {
		t.Fatalf("first event.User = %+v, want u-alice", first.User)
	}

	second := mustReceiveEvent(t, events)
	if second.Type != im.EventTypeRoomCreated {
		t.Fatalf("second event.Type = %q, want %q", second.Type, im.EventTypeRoomCreated)
	}
	if second.Room == nil {
		t.Fatal("second event.Room = nil, want bootstrap room")
	}
	if second.Room.Title != "alice" {
		t.Fatalf("second event.Room.Title = %q, want %q", second.Room.Title, "alice")
	}
	if !containsParticipant(second.Room.Participants, "u-admin") || !containsParticipant(second.Room.Participants, "u-alice") {
		t.Fatalf("second event.Room.Participants = %+v, want admin and worker", second.Room.Participants)
	}

	select {
	case evt := <-events:
		t.Fatalf("unexpected third event before delay: %q", evt.Type)
	case <-time.After(150 * time.Millisecond):
	}

	third := mustReceiveEventWithin(t, events, 2*time.Second)
	if third.Type != im.EventTypeMessageCreated {
		t.Fatalf("third event.Type = %q, want %q", third.Type, im.EventTypeMessageCreated)
	}
	if third.Message == nil {
		t.Fatal("third event.Message = nil, want bootstrap message")
	}
	if third.Message.SenderID != "u-admin" {
		t.Fatalf("third event.Message.SenderID = %q, want %q", third.Message.SenderID, "u-admin")
	}
	wantContent := "@Alice Write this down in your memory: your name is Alice. Your responsibility is test lead"
	if third.Message.Content != wantContent {
		t.Fatalf("third event.Message.Content = %q, want %q", third.Message.Content, wantContent)
	}
	if third.Sender == nil || third.Sender.ID != "u-admin" {
		t.Fatalf("third event.Sender = %+v, want u-admin", third.Sender)
	}
}

func TestHandleFeishuUsersCreateAndList(t *testing.T) {
	srv := &Handler{feishu: channel.NewFeishuService()}

	createReq := strings.NewReader(`{"id":"fsu-alice","name":"Alice","handle":"alice","role":"worker"}`)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/users", createReq))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/users", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []im.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "fsu-alice" || got[0].Handle != "alice" {
		t.Fatalf("users = %+v, want fsu-alice", got)
	}
}

func TestHandleFeishuRoomsMembers(t *testing.T) {
	feishu := channel.NewFeishuServiceWithCreateChatAndAddMembers(
		map[string]channel.FeishuAppConfig{
			"u-manager": {AppID: "manager-app-id", AppSecret: "app-secret", AdminOpenID: "ou_admin"},
			"fsu-alice": {AppID: "alice-app-id", AppSecret: "alice-secret"},
		},
		func(_ context.Context, _ channel.FeishuAppConfig, req channel.FeishuCreateChatRequest) (channel.FeishuCreateChatResponse, error) {
			return channel.FeishuCreateChatResponse{
				ChatID:      "oc_alpha",
				Name:        req.Title,
				Description: req.Description,
			}, nil
		},
		func(context.Context, channel.FeishuAppConfig, channel.FeishuAddChatMembersRequest) error { return nil },
		func(context.Context, channel.FeishuAppConfig, map[string]channel.FeishuAppConfig, string) ([]im.User, error) {
			return []im.User{
				{ID: "fsu-admin", Name: "Admin"},
				{ID: "fsu-alice", Name: "Alice"},
			}, nil
		},
	)
	if _, err := feishu.CreateUser(channel.FeishuCreateUserRequest{ID: "fsu-admin", Name: "Admin"}); err != nil {
		t.Fatalf("CreateUser(admin) error = %v", err)
	}
	if _, err := feishu.CreateUser(channel.FeishuCreateUserRequest{ID: "fsu-alice", Name: "Alice"}); err != nil {
		t.Fatalf("CreateUser(alice) error = %v", err)
	}
	srv := &Handler{feishu: feishu}

	createReq := strings.NewReader(`{"title":"alpha","creator_id":"fsu-admin"}`)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/rooms", createReq))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var room im.Room
	if err := json.NewDecoder(rec.Body).Decode(&room); err != nil {
		t.Fatalf("decode room: %v", err)
	}

	addReq := strings.NewReader(`{"user_ids":["fsu-alice"]}`)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/rooms/"+room.ID+"/members", addReq))
	if rec.Code != http.StatusOK {
		t.Fatalf("add status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/rooms/"+room.ID+"/members", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("members status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var members []im.User
	if err := json.NewDecoder(rec.Body).Decode(&members); err != nil {
		t.Fatalf("decode members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("members = %+v, want two users", members)
	}
}

func TestHandleRoomsMembersListsCsgclawParticipants(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "Admin", Handle: "admin", Role: "admin"},
			{ID: "u-alice", Name: "Alice", Handle: "alice", Role: "worker"},
		},
		Rooms: []im.Room{
			{ID: "room-1", Title: "Ops", Participants: []string{"u-admin", "u-alice"}},
		},
	})
	srv := &Handler{im: imSvc}

	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/rooms/room-1/members", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("members status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var members []im.User
	if err := json.NewDecoder(rec.Body).Decode(&members); err != nil {
		t.Fatalf("decode members: %v", err)
	}
	if len(members) != 2 || members[0].ID != "u-admin" || members[1].ID != "u-alice" {
		t.Fatalf("members = %+v, want room participants", members)
	}
}

func TestHandleRoomsMembersAddsCsgclawParticipant(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "Admin", Handle: "admin", Role: "admin"},
			{ID: "u-alice", Name: "Alice", Handle: "alice", Role: "worker"},
		},
		Rooms: []im.Room{
			{ID: "room-1", Title: "Ops", Participants: []string{"u-admin"}},
		},
	})
	srv := &Handler{im: imSvc}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/room-1/members", strings.NewReader(`{"inviter_id":"u-admin","user_ids":["u-alice"]}`))
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("add status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var room im.Room
	if err := json.NewDecoder(rec.Body).Decode(&room); err != nil {
		t.Fatalf("decode room: %v", err)
	}
	if len(room.Participants) != 2 || room.Participants[1] != "u-alice" {
		t.Fatalf("participants = %+v, want u-admin and u-alice", room.Participants)
	}
}

func TestHandleBotsListReturnsAllBots(t *testing.T) {
	srv := &Handler{botSvc: mustNewBotService(t, []bot.Bot{
		{
			ID:        "bot-csgclaw",
			Name:      "CSGClaw Bot",
			Role:      string(bot.RoleWorker),
			Channel:   string(bot.ChannelCSGClaw),
			AgentID:   "agent-csgclaw",
			UserID:    "user-csgclaw",
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:        "bot-feishu",
			Name:      "Feishu Bot",
			Role:      string(bot.RoleManager),
			Channel:   string(bot.ChannelFeishu),
			AgentID:   "agent-feishu",
			UserID:    "user-feishu",
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bots", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []bot.Bot
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 2 || got[0].ID != "bot-csgclaw" || got[1].ID != "bot-feishu" {
		t.Fatalf("bots = %+v, want all bots in store order", got)
	}
}

func TestHandleBotsListFiltersByChannel(t *testing.T) {
	srv := &Handler{botSvc: mustNewBotService(t, []bot.Bot{
		{
			ID:        "bot-csgclaw",
			Name:      "CSGClaw Bot",
			Role:      string(bot.RoleWorker),
			Channel:   string(bot.ChannelCSGClaw),
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:        "bot-feishu",
			Name:      "Feishu Bot",
			Role:      string(bot.RoleWorker),
			Channel:   string(bot.ChannelFeishu),
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bots?channel=csgclaw", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []bot.Bot
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "bot-csgclaw" {
		t.Fatalf("bots = %+v, want only bot-csgclaw", got)
	}
}

func TestHandleBotsListFiltersByRole(t *testing.T) {
	srv := &Handler{botSvc: mustNewBotService(t, []bot.Bot{
		{
			ID:        "bot-manager",
			Name:      "Manager Bot",
			Role:      string(bot.RoleManager),
			Channel:   string(bot.ChannelCSGClaw),
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:        "bot-worker",
			Name:      "Worker Bot",
			Role:      string(bot.RoleWorker),
			Channel:   string(bot.ChannelCSGClaw),
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bots?role=worker", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []bot.Bot
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "bot-worker" {
		t.Fatalf("bots = %+v, want only bot-worker", got)
	}
}

func TestHandleBotsListRejectsInvalidChannel(t *testing.T) {
	srv := &Handler{botSvc: mustNewBotService(t, nil)}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bots?channel=unknown", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleBotsListRejectsInvalidRole(t *testing.T) {
	srv := &Handler{botSvc: mustNewBotService(t, nil)}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bots?role=agent", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleBotsListRequiresService(t *testing.T) {
	srv := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bots", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func TestHandleBotsCreateCSGClawWorker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	agentSvc, _ := mustNewSeededServiceWithPath(t, nil)
	imSvc := im.NewService()
	store, err := bot.NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("bot.NewMemoryStore() error = %v", err)
	}
	botSvc, err := bot.NewServiceWithDependencies(store, agentSvc, imSvc)
	if err != nil {
		t.Fatalf("bot.NewServiceWithDependencies() error = %v", err)
	}
	srv := &Handler{
		svc:    agentSvc,
		botSvc: botSvc,
		im:     imSvc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bots", strings.NewReader(`{"name":"alice","description":"test lead","role":"worker","channel":"csgclaw"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created bot.Bot
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID != "u-alice" || created.AgentID != "u-alice" || created.UserID != "u-alice" {
		t.Fatalf("created bot = %+v, want u-alice IDs", created)
	}
	if created.Description != "test lead" {
		t.Fatalf("created bot description = %q, want test lead", created.Description)
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/bots?channel=csgclaw", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list bots status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var bots []bot.Bot
	if err := json.NewDecoder(rec.Body).Decode(&bots); err != nil {
		t.Fatalf("decode bots response: %v", err)
	}
	if len(bots) != 1 || bots[0].ID != "u-alice" {
		t.Fatalf("bots = %+v, want u-alice", bots)
	}
	if bots[0].Description != "test lead" {
		t.Fatalf("bots[0].Description = %q, want test lead", bots[0].Description)
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list agents status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var agents []agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&agents); err != nil {
		t.Fatalf("decode agents response: %v", err)
	}
	if len(agents) != 1 || agents[0].ID != "u-alice" {
		t.Fatalf("agents = %+v, want u-alice", agents)
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list users status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var users []im.User
	if err := json.NewDecoder(rec.Body).Decode(&users); err != nil {
		t.Fatalf("decode users response: %v", err)
	}
	if !containsUser(users, "u-alice") {
		t.Fatalf("users = %+v, want u-alice", users)
	}
}

func TestHandleBotsCreateFeishuWorker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	agentSvc, _ := mustNewSeededServiceWithPath(t, nil)
	feishuSvc := channel.NewFeishuService()
	store, err := bot.NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("bot.NewMemoryStore() error = %v", err)
	}
	botSvc, err := bot.NewServiceWithDependencies(store, agentSvc, nil, feishuSvc)
	if err != nil {
		t.Fatalf("bot.NewServiceWithDependencies() error = %v", err)
	}
	srv := &Handler{
		svc:    agentSvc,
		botSvc: botSvc,
		feishu: feishuSvc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bots", strings.NewReader(`{"name":"alice","role":"worker","channel":"feishu"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created bot.Bot
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID != "u-alice" || created.AgentID != "u-alice" || created.UserID != "u-alice" || created.Channel != "feishu" {
		t.Fatalf("created bot = %+v, want feishu u-alice IDs", created)
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/bots?channel=feishu", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list bots status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var bots []bot.Bot
	if err := json.NewDecoder(rec.Body).Decode(&bots); err != nil {
		t.Fatalf("decode bots response: %v", err)
	}
	if len(bots) != 1 || bots[0].ID != "u-alice" {
		t.Fatalf("bots = %+v, want u-alice", bots)
	}

	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/users", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list feishu users status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var users []im.User
	if err := json.NewDecoder(rec.Body).Decode(&users); err != nil {
		t.Fatalf("decode users response: %v", err)
	}
	if !containsUser(users, "u-alice") {
		t.Fatalf("feishu users = %+v, want u-alice", users)
	}
}

func TestHandleBotsCreateCSGClawManagerBindsBootstrappedAgent(t *testing.T) {
	agentSvc := mustNewSeededService(t, []agent.Agent{
		{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			Role:      agent.RoleManager,
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
	})
	imSvc := im.NewService()
	store, err := bot.NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("bot.NewMemoryStore() error = %v", err)
	}
	botSvc, err := bot.NewServiceWithDependencies(store, agentSvc, imSvc)
	if err != nil {
		t.Fatalf("bot.NewServiceWithDependencies() error = %v", err)
	}
	srv := &Handler{
		svc:    agentSvc,
		botSvc: botSvc,
		im:     imSvc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bots", strings.NewReader(`{"name":"manager","role":"manager","channel":"csgclaw"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created bot.Bot
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID != agent.ManagerUserID || created.AgentID != agent.ManagerUserID || created.UserID != agent.ManagerUserID || created.Role != string(bot.RoleManager) {
		t.Fatalf("created bot = %+v, want manager u-manager IDs", created)
	}
}

func TestHandleBotsCreateManagerBootstrapsMissingAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	agentSvc := mustNewSeededService(t, nil)
	imSvc := im.NewService()
	store, err := bot.NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("bot.NewMemoryStore() error = %v", err)
	}
	botSvc, err := bot.NewServiceWithDependencies(store, agentSvc, imSvc)
	if err != nil {
		t.Fatalf("bot.NewServiceWithDependencies() error = %v", err)
	}
	srv := &Handler{
		svc:    agentSvc,
		botSvc: botSvc,
		im:     imSvc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bots", strings.NewReader(`{"name":"manager","role":"manager","channel":"csgclaw"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created bot.Bot
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID != agent.ManagerUserID || created.AgentID != agent.ManagerUserID || created.UserID != agent.ManagerUserID {
		t.Fatalf("created bot = %+v, want u-manager IDs", created)
	}
}

func TestHandleBotsListRejectsUnsupportedMethod(t *testing.T) {
	srv := &Handler{botSvc: mustNewBotService(t, nil)}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/bots", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
}

func TestHandleBotByIDDeleteUsesChannel(t *testing.T) {
	srv := &Handler{botSvc: mustNewBotService(t, []bot.Bot{
		{
			ID:        "u-alice",
			Name:      "Alice",
			Role:      string(bot.RoleWorker),
			Channel:   string(bot.ChannelCSGClaw),
			AgentID:   "u-alice",
			UserID:    "u-alice",
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:        "u-alice",
			Name:      "Alice",
			Role:      string(bot.RoleWorker),
			Channel:   string(bot.ChannelFeishu),
			AgentID:   "u-alice",
			UserID:    "u-alice",
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/bots/u-alice?channel=feishu", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	bots, err := srv.botSvc.List(string(bot.ChannelCSGClaw), "")
	if err != nil {
		t.Fatalf("List(csgclaw) error = %v", err)
	}
	if len(bots) != 1 || bots[0].ID != "u-alice" {
		t.Fatalf("csgclaw bots = %+v, want retained u-alice", bots)
	}
	bots, err = srv.botSvc.List(string(bot.ChannelFeishu), "")
	if err != nil {
		t.Fatalf("List(feishu) error = %v", err)
	}
	if len(bots) != 0 {
		t.Fatalf("feishu bots = %+v, want deleted", bots)
	}
}

func TestHandleAgentsListReturnsUnifiedAgents(t *testing.T) {
	svc := mustNewSeededService(t, []agent.Agent{
		{ID: "u-manager", Name: "manager", Role: agent.RoleManager, CreatedAt: time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC)},
		{ID: "u-alice", Name: "alice", Role: agent.RoleWorker, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
		{ID: "agent-1", Name: "observer", Role: agent.RoleAgent, CreatedAt: time.Date(2026, 3, 28, 11, 0, 0, 0, time.UTC)},
	})

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(agents) = %d, want 3; body=%s", len(got), rec.Body.String())
	}
	if got[0].ID != "u-manager" || got[1].ID != "u-alice" || got[2].ID != "agent-1" {
		t.Fatalf("agents = %+v, want manager/worker/agent in CreatedAt order", got)
	}
}

func TestHandleAgentsGetByIDReturnsAgent(t *testing.T) {
	svc := mustNewSeededService(t, []agent.Agent{
		{ID: "u-alice", Name: "alice", Role: agent.RoleWorker, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	})

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-alice", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "u-alice" || got.Name != "alice" || got.Role != agent.RoleWorker {
		t.Fatalf("agent = %+v, want u-alice/alice/worker", got)
	}
}

func TestHandleAgentsGetByIDReloadsStateBeforeLookup(t *testing.T) {
	svc, statePath := mustNewSeededServiceWithPath(t, []agent.Agent{
		{ID: "u-alice", Name: "alice", Role: agent.RoleWorker, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	})

	if err := writeSeededAgents(statePath, []agent.Agent{
		{ID: "u-bob", Name: "bob", Role: agent.RoleWorker, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-bob", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "u-bob" || got.Name != "bob" {
		t.Fatalf("agent = %+v, want u-bob/bob", got)
	}
}

func TestHandleAgentsGetByIDNotFound(t *testing.T) {
	svc := mustNewSeededService(t, nil)

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/missing", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleAgentLogsStreamsGatewayLog(t *testing.T) {
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	agentSvc := mustNewSeededService(t, []agent.Agent{
		{ID: "u-alice", Name: "alice", BoxID: "box-123", Role: agent.RoleWorker, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	})

	var gotBoxID string
	var gotCmd string
	var gotArgs []string
	agent.TestOnlySetGetBoxHook(func(_ *agent.Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		gotBoxID = idOrName
		return sandboxtest.NewInstance(sandbox.Info{ID: idOrName, Name: "alice"}), nil
	})
	agent.TestOnlySetRunBoxCommandHook(func(_ *agent.Service, _ context.Context, _ sandbox.Instance, name string, args []string, w io.Writer) (int, error) {
		gotCmd = name
		gotArgs = append([]string(nil), args...)
		_, _ = io.WriteString(w, "hello\nworld\n")
		return 0, nil
	})
	defer func() {
		agent.TestOnlySetGetBoxHook(nil)
		agent.TestOnlySetRunBoxCommandHook(nil)
	}()

	srv := &Handler{svc: agentSvc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-alice/logs?follow=true&lines=80", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotBoxID != "box-123" {
		t.Fatalf("getBox() idOrName = %q, want %q", gotBoxID, "box-123")
	}
	if gotCmd != "tail" {
		t.Fatalf("command = %q, want %q", gotCmd, "tail")
	}
	if strings.Join(gotArgs, " ") != "-n 80 -f /home/picoclaw/.picoclaw/gateway.log" {
		t.Fatalf("args = %q", gotArgs)
	}
	if rec.Body.String() != "hello\nworld\n" {
		t.Fatalf("body = %q, want streamed logs", rec.Body.String())
	}
	if !rec.Flushed {
		t.Fatal("response was not flushed for follow=true")
	}
}

func TestHandleAgentLogsReloadsStateBeforeStreaming(t *testing.T) {
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	agentSvc, statePath := mustNewSeededServiceWithPath(t, []agent.Agent{
		{ID: "u-manager", Name: "manager", BoxID: "box-old", Role: agent.RoleManager, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	})
	if err := writeSeededAgents(statePath, []agent.Agent{
		{ID: "u-manager", Name: "manager", BoxID: "box-new", Role: agent.RoleManager, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}

	var gotBoxID string
	agent.TestOnlySetGetBoxHook(func(_ *agent.Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		gotBoxID = idOrName
		return sandboxtest.NewInstance(sandbox.Info{ID: idOrName, Name: "manager"}), nil
	})
	agent.TestOnlySetRunBoxCommandHook(func(_ *agent.Service, _ context.Context, _ sandbox.Instance, _ string, _ []string, w io.Writer) (int, error) {
		_, _ = io.WriteString(w, "line-1\n")
		return 0, nil
	})
	defer func() {
		agent.TestOnlySetGetBoxHook(nil)
		agent.TestOnlySetRunBoxCommandHook(nil)
	}()

	srv := &Handler{svc: agentSvc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-manager/logs", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotBoxID != "box-new" {
		t.Fatalf("getBox() idOrName = %q, want %q", gotBoxID, "box-new")
	}
}

func TestHandleAgentsDeleteRemovesAgent(t *testing.T) {
	svc := mustNewSeededService(t, []agent.Agent{
		{ID: "u-alice", Name: "alice", Role: agent.RoleWorker, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	})

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/u-alice", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if _, ok := svc.Agent("u-alice"); ok {
		t.Fatal("Agent() ok = true, want false after delete")
	}
}

func TestHandleAgentsDeleteNotFound(t *testing.T) {
	svc := mustNewSeededService(t, nil)

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/missing", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleAgentsCreateUsesWorkerCompatibilityFlow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc := mustNewService(t)
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()

	srv := &Handler{
		svc:   svc,
		im:    im.NewService(),
		imBus: bus,
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"name":"alice","role":"worker"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "u-alice" || got.Role != agent.RoleWorker {
		t.Fatalf("agent = %+v, want worker alias result", got)
	}

	first := mustReceiveEvent(t, events)
	second := mustReceiveEvent(t, events)
	if first.Type != im.EventTypeUserCreated || second.Type != im.EventTypeRoomCreated {
		t.Fatalf("events = [%q, %q], want user_created then room_created", first.Type, second.Type)
	}

	third := mustReceiveEventWithin(t, events, 2*time.Second)
	if third.Type != im.EventTypeMessageCreated {
		t.Fatalf("third event.Type = %q, want %q", third.Type, im.EventTypeMessageCreated)
	}
	if third.Message == nil {
		t.Fatal("third event.Message = nil, want bootstrap message")
	}
	if third.Message.SenderID != "u-admin" {
		t.Fatalf("third event.Message.SenderID = %q, want %q", third.Message.SenderID, "u-admin")
	}
}

func TestHandleBootstrapAliasReturnsIMBootstrap(t *testing.T) {
	srv := &Handler{im: im.NewService()}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bootstrap", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got struct {
		CurrentUserID string    `json:"current_user_id"`
		Users         []im.User `json:"users"`
		Rooms         []im.Room `json:"rooms"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.CurrentUserID == "" {
		t.Fatal("bootstrap current_user_id is empty")
	}
	if got.Rooms == nil {
		t.Fatal("bootstrap rooms is nil, want room-oriented DTO")
	}
}

func TestHandleRoomsInviteAliasAddsConversationMembers(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Handle: "admin"},
				{ID: "u-manager", Name: "manager", Handle: "manager"},
			},
			Rooms: []im.Room{
				{
					ID:           "room-1",
					Title:        "Room One",
					Participants: []string{"u-admin"},
				},
			},
		}),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/invite", strings.NewReader(`{"room_id":"room-1","inviter_id":"u-admin","user_ids":["u-manager"],"locale":"en"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got im.Conversation
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "room-1" {
		t.Fatalf("conversation id = %q, want %q", got.ID, "room-1")
	}
	if !containsParticipant(got.Participants, "u-manager") {
		t.Fatalf("participants = %+v, want u-manager to be invited", got.Participants)
	}
}

func TestHandleIMAgentJoinReturnsCompactSuccessPayload(t *testing.T) {
	srv := &Handler{
		svc: mustNewSeededService(t, []agent.Agent{
			{ID: "u-alice", Name: "Alice", Role: agent.RoleWorker, CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)},
		}),
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Handle: "admin"},
				{ID: "u-alice", Name: "Alice", Handle: "alice"},
			},
			Rooms: []im.Room{
				{
					ID:           "room-1",
					Title:        "Room One",
					Participants: []string{"u-admin"},
					Messages:     []im.Message{{ID: "msg-1", SenderID: "u-admin", Content: "hello"}},
				},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/im/agents/join", strings.NewReader(`{"agent_id":"u-alice","room_id":"room-1","locale":"en"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"messages"`) {
		t.Fatalf("body = %s, want compact success payload without messages", rec.Body.String())
	}

	var got struct {
		Message string `json:"message"`
		RoomID  string `json:"room_id"`
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Message != "agent joined successfully" {
		t.Fatalf("message = %q, want success message", got.Message)
	}
	if got.RoomID != "room-1" || got.AgentID != "u-alice" {
		t.Fatalf("response = %+v, want room-1/u-alice", got)
	}
	if room, ok := srv.im.Room("room-1"); !ok || !containsParticipant(room.Participants, "u-alice") {
		t.Fatalf("room participants = %+v, want agent joined", room.Participants)
	}
}

func TestHandleRoomsInviteRequiresRoomID(t *testing.T) {
	srv := &Handler{im: im.NewService()}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/invite", strings.NewReader(`{"inviter_id":"u-admin","user_ids":["u-manager"]}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleWorkersPostRemainsCreateAlias(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc := mustNewService(t)
	store, err := bot.NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("bot.NewMemoryStore() error = %v", err)
	}
	botSvc, err := bot.NewServiceWithDependencies(store, svc, nil)
	if err != nil {
		t.Fatalf("bot.NewServiceWithDependencies() error = %v", err)
	}
	srv := &Handler{
		svc:    svc,
		botSvc: botSvc,
		im:     im.NewService(),
	}
	botSvc.SetDependencies(svc, srv.im)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workers", strings.NewReader(`{"name":"bob"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "u-bob" || got.Role != agent.RoleWorker {
		t.Fatalf("agent = %+v, want worker alias result", got)
	}
	bots, err := botSvc.List(string(bot.ChannelCSGClaw), "")
	if err != nil {
		t.Fatalf("botSvc.List() error = %v", err)
	}
	if len(bots) != 1 || bots[0].ID != "u-bob" || bots[0].Role != string(bot.RoleWorker) {
		t.Fatalf("bots = %+v, want u-bob worker bot", bots)
	}
	rooms := srv.im.ListRooms()
	var workerRoom *im.Room
	for i := range rooms {
		if containsParticipant(rooms[i].Participants, "u-bob") {
			room := rooms[i]
			workerRoom = &room
			break
		}
	}
	if workerRoom == nil {
		t.Fatal("worker room = nil, want bootstrap room for u-bob")
	}
	waitForCondition(t, 2*time.Second, 20*time.Millisecond, func() bool {
		rooms := srv.im.ListRooms()
		for i := range rooms {
			if !containsParticipant(rooms[i].Participants, "u-bob") {
				continue
			}
			if len(rooms[i].Messages) < 2 {
				return false
			}
			last := rooms[i].Messages[len(rooms[i].Messages)-1]
			if last.SenderID != "u-admin" {
				return false
			}
			workerRoom = &rooms[i]
			return true
		}
		return false
	})
	if workerRoom == nil || len(workerRoom.Messages) == 0 {
		t.Fatal("worker room.Messages = empty, want bootstrap messages")
	}
	last := workerRoom.Messages[len(workerRoom.Messages)-1]
	if last.SenderID != "u-admin" {
		t.Fatalf("last message.SenderID = %q, want %q", last.SenderID, "u-admin")
	}
	wantContent := "@bob Write this down in your memory: your name is bob."
	if last.Content != wantContent {
		t.Fatalf("last message.Content = %q, want %q", last.Content, wantContent)
	}
}

func TestHandleRoomsReturnsConversationList(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-alice", Name: "Alice", Handle: "alice"},
			},
			Rooms: []im.Room{
				{
					ID:           "room-1",
					Title:        "Room One",
					Participants: []string{"u-admin", "u-alice"},
					Messages: []im.Message{{
						ID:        "msg-1",
						SenderID:  "u-admin",
						Content:   "hello",
						CreatedAt: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
					}},
				},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []im.Conversation
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "room-1" {
		t.Fatalf("rooms = %+v, want room-1", got)
	}
}

func TestHandleUsersReturnsUserList(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-zed", Name: "Zed", Handle: "zed"},
				{ID: "u-alice", Name: "Alice", Handle: "alice"},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []im.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 4 || got[0].Name != "admin" || got[1].Name != "alice" || got[2].Name != "manager" || got[3].Name != "zed" {
		t.Fatalf("users = %+v, want admin/alice/manager/zed", got)
	}
}

func TestHandleMessagesReturnsConversationMessages(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Rooms: []im.Room{
				{
					ID:           "room-1",
					Title:        "Room One",
					Participants: []string{"u-admin", "u-manager"},
					Messages: []im.Message{{
						ID:        "msg-1",
						SenderID:  "u-admin",
						Content:   "hello",
						CreatedAt: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
					}},
				},
			},
		}),
	}

	for _, path := range []string{
		"/api/v1/messages?room_id=room-1",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("path %s status = %d, want %d; body=%s", path, rec.Code, http.StatusOK, rec.Body.String())
		}
		var got []im.Message
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("path %s decode response: %v", path, err)
		}
		if len(got) != 1 || got[0].ID != "msg-1" {
			t.Fatalf("path %s messages = %+v, want msg-1", path, got)
		}
	}
}

func TestHandleMessagesRejectsInvalidQuery(t *testing.T) {
	srv := &Handler{im: im.NewService()}

	for _, path := range []string{
		"/api/v1/messages",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("path %s status = %d, want %d", path, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestHandleMessagesPostCreatesMessage(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Handle: "admin"},
				{ID: "u-manager", Name: "manager", Handle: "manager"},
			},
			Rooms: []im.Room{
				{
					ID:           "room-1",
					Title:        "Room One",
					Participants: []string{"u-admin", "u-manager"},
				},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"room_id":"room-1","sender_id":"u-admin","content":"hello @manager"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got im.Message
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.SenderID != "u-admin" || got.Content != "hello @manager" {
		t.Fatalf("message = %+v, want sender/content populated", got)
	}
	if len(got.Mentions) != 1 || got.Mentions[0] != "u-manager" {
		t.Fatalf("mentions = %+v, want u-manager", got.Mentions)
	}
}

func TestHandleMessagesPostPrefixesMentionID(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Handle: "admin"},
				{ID: "u-dev", Name: "dev", Handle: "dev"},
				{ID: "u-manager", Name: "manager", Handle: "manager"},
			},
			Rooms: []im.Room{
				{
					ID:           "room-1",
					Title:        "Room One",
					Participants: []string{"u-admin", "u-dev", "u-manager"},
				},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"room_id":"room-1","sender_id":"u-admin","content":"hi","mention_id":"u-dev"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got im.Message
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Content != "@manager hi" {
		t.Fatalf("content = %q, want @manager hi", got.Content)
	}
	if len(got.Mentions) != 1 || got.Mentions[0] != "u-manager" {
		t.Fatalf("mentions = %+v, want u-manager", got.Mentions)
	}
}

func TestHandleFeishuMessagesPostSendsMessage(t *testing.T) {
	feishuSvc := channel.NewFeishuServiceWithSendMessage(
		map[string]channel.FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"}},
		func(_ context.Context, _ channel.FeishuAppConfig, req channel.FeishuSendMessageRequest) (channel.FeishuSendMessageResponse, error) {
			if req.ChatID != "oc_alpha" || req.Content != "hello" {
				t.Fatalf("send request = %+v, want chat/content", req)
			}
			return channel.FeishuSendMessageResponse{MessageID: "om_1", SenderOpenID: "ou_manager"}, nil
		},
	)
	srv := &Handler{feishu: feishuSvc}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/messages", strings.NewReader(`{"room_id":"oc_alpha","sender_id":"u-manager","content":"hello"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got im.Message
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "om_1" || got.SenderID != "ou_manager" || got.Content != "hello" {
		t.Fatalf("message = %+v, want feishu message response", got)
	}
}

func TestHandleFeishuMessagesGetListsRoomMessages(t *testing.T) {
	feishuSvc := channel.NewFeishuServiceWithCreateChatAndListRoomMessages(
		map[string]channel.FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
		func(_ context.Context, _ channel.FeishuAppConfig, req channel.FeishuCreateChatRequest) (channel.FeishuCreateChatResponse, error) {
			return channel.FeishuCreateChatResponse{ChatID: "oc_alpha", Name: req.Title}, nil
		},
		func(_ context.Context, _ channel.FeishuAppConfig, roomID string) ([]im.Message, error) {
			if roomID != "oc_alpha" {
				t.Fatalf("room_id = %q, want oc_alpha", roomID)
			}
			return []im.Message{{ID: "om_1", SenderID: "ou_manager", Content: "hello", CreatedAt: time.Unix(1, 0).UTC()}}, nil
		},
	)
	if _, err := feishuSvc.CreateRoom(im.CreateRoomRequest{Title: "alpha", CreatorID: "u-manager"}); err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	srv := &Handler{feishu: feishuSvc}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/messages?room_id=oc_alpha", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []im.Message
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "om_1" {
		t.Fatalf("messages = %+v, want listed feishu messages", got)
	}
}

func TestHandleFeishuEventsStreamsMessageBusEvents(t *testing.T) {
	feishuSvc := channel.NewFeishuService()
	srv := &Handler{feishu: feishuSvc, serverAccessToken: "secret"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/bots/u-manager/events", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.Routes().ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	feishuSvc.MessageBus().Publish(channel.FeishuMessageEvent{
		Type:   channel.FeishuMessageEventTypeMessageCreated,
		RoomID: "oc_ignored",
		Message: &im.Message{
			ID:       "om_ignored",
			SenderID: "ou_manager",
			Content:  "hello @worker",
			Mentions: []string{"u-worker"},
		},
	})
	feishuSvc.MessageBus().Publish(channel.FeishuMessageEvent{
		Type:   channel.FeishuMessageEventTypeMessageCreated,
		RoomID: "oc_alpha",
		Message: &im.Message{
			ID:       "om_1",
			SenderID: "ou_manager",
			Content:  "hello @alice",
			Mentions: []string{"u-manager"},
		},
	})
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, `"type":"message.created"`) {
		t.Fatalf("body = %q, want message.created event", body)
	}
	if !strings.Contains(body, `"room_id":"oc_alpha"`) {
		t.Fatalf("body = %q, want room_id", body)
	}
	if strings.Contains(body, "om_ignored") || strings.Contains(body, "oc_ignored") {
		t.Fatalf("body = %q, want only u-manager events", body)
	}
	if !strings.Contains(body, `"id":"om_1"`) {
		t.Fatalf("body = %q, want message id", body)
	}
}

func TestHandleFeishuEventsRequiresAuthorization(t *testing.T) {
	srv := &Handler{
		feishu:            channel.NewFeishuService(),
		serverAccessToken: "secret",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/bots/u-manager/events", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleFeishuEventsRequiresAuthorizationWhenServerAccessTokenEmpty(t *testing.T) {
	srv := &Handler{}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/bots/u-manager/events", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleFeishuEventsSkipsAuthorizationWhenNoAuth(t *testing.T) {
	srv := &Handler{serverNoAuth: true}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/bots/u-manager/events", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("status = %d, want non-unauthorized when no_auth is true", rec.Code)
	}
}

func TestHandleMessagesPostRequiresRoomID(t *testing.T) {
	srv := &Handler{im: im.NewService()}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"sender_id":"u-admin","content":"hello"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleIMEventsExposeRoomIDOnly(t *testing.T) {
	bus := im.NewBus()
	srv := &Handler{imBus: bus}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/im/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.Routes().ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	bus.Publish(im.Event{
		Type:   im.EventTypeMessageCreated,
		RoomID: "room-1",
		Message: &im.Message{
			ID:       "msg-1",
			SenderID: "u-admin",
			Content:  "hello",
		},
		Sender: &im.User{ID: "u-admin", Name: "admin", Handle: "admin"},
	})
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, `"room_id":"room-1"`) {
		t.Fatalf("body = %q, want room_id", body)
	}
	if strings.Contains(body, `"conversation_id"`) {
		t.Fatalf("body = %q, want no conversation_id compatibility field", body)
	}
}

func TestHandleRoomsPostCreatesRoom(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Handle: "admin"},
				{ID: "u-alice", Name: "Alice", Handle: "alice"},
				{ID: "u-manager", Name: "manager", Handle: "manager"},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"title":"Launch","description":"coordination","creator_id":"u-admin","participant_ids":["u-alice","u-manager"],"locale":"en"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got im.Conversation
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Title != "Launch" {
		t.Fatalf("conversation.Title = %q, want Launch", got.Title)
	}
	if !containsParticipant(got.Participants, "u-admin") || !containsParticipant(got.Participants, "u-alice") || !containsParticipant(got.Participants, "u-manager") {
		t.Fatalf("participants = %+v, want admin, alice, and manager", got.Participants)
	}
}

func TestHandleUsersDeleteKicksUser(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Handle: "admin", IsOnline: true},
				{ID: "u-alice", Name: "Alice", Handle: "alice", IsOnline: true},
			},
			Rooms: []im.Room{
				{
					ID:           "room-1",
					Title:        "Room One",
					Participants: []string{"u-admin", "u-alice"},
					Messages:     []im.Message{{ID: "msg-1", SenderID: "u-alice", Content: "hello"}},
				},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/u-alice", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if _, ok := srv.im.User("u-alice"); ok {
		t.Fatal("User() ok = true, want false after delete")
	}
	if _, ok := srv.im.Room("room-1"); ok {
		t.Fatal("Room() ok = true, want false for DM after kicked user")
	}
}

func TestHandleUsersDeleteCurrentUserReturnsConflict(t *testing.T) {
	srv := &Handler{im: im.NewService()}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/u-admin", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleFeishuUsersDeleteRemovesUser(t *testing.T) {
	srv := &Handler{
		feishu: channel.NewFeishuService(),
	}
	if _, err := srv.feishu.CreateUser(channel.FeishuCreateUserRequest{ID: "ou-alice", Name: "Alice"}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/feishu/users/ou-alice", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if containsUser(srv.feishu.ListUsers(), "ou-alice") {
		t.Fatal("containsUser() = true, want false after delete")
	}
}

func TestHandleRoomsDeleteRemovesRoom(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Rooms: []im.Room{
				{ID: "room-1", Title: "Room One", Participants: []string{"u-admin", "u-manager"}},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/rooms/room-1", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if _, ok := srv.im.Room("room-1"); ok {
		t.Fatal("Room() ok = true, want false after delete")
	}
}

func TestHandleFeishuRoomsDeleteRemovesRoom(t *testing.T) {
	deleted := make([]string, 0, 1)
	srv := &Handler{
		feishu: channel.NewFeishuServiceWithDeleteChat(
			map[string]channel.FeishuAppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
			func(_ context.Context, _ channel.FeishuAppConfig, roomID string) error {
				deleted = append(deleted, roomID)
				return nil
			},
		),
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/feishu/rooms/oc_alpha", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if len(deleted) != 1 || deleted[0] != "oc_alpha" {
		t.Fatalf("deleted = %+v, want [oc_alpha]", deleted)
	}
}

func TestHandlePicoClawRoutesRequireAuthorization(t *testing.T) {
	srv := &Handler{
		im:                im.NewService(),
		picoclaw:          im.NewPicoClawBridge("secret"),
		serverAccessToken: "secret",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/bots/u-manager/messages/send", strings.NewReader(`{"room_id":"room-1","text":"hello"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandlePicoClawRoutesRequireAuthorizationWhenServerAccessTokenEmpty(t *testing.T) {
	srv := &Handler{
		picoclaw: im.NewPicoClawBridge("secret"),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/bots/u-manager/messages/send", strings.NewReader(`{"room_id":"room-1","text":"hello"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandlePicoClawRoutesSkipAuthorizationWhenNoAuth(t *testing.T) {
	srv := &Handler{
		picoclaw:     im.NewPicoClawBridge("secret"),
		serverNoAuth: true,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/bots/u-manager/messages/send", strings.NewReader(`{"room_id":"room-1","text":"hello"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("status = %d, want non-unauthorized when no_auth is true", rec.Code)
	}
}

func TestHandlePicoClawSendMessageRequiresIMService(t *testing.T) {
	srv := &Handler{
		picoclaw:     im.NewPicoClawBridge(""),
		serverNoAuth: true,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/bots/u-manager/messages/send", strings.NewReader(`{"room_id":"room-1","text":"hello"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func TestHandlePicoClawModelsReturnsBridgeCatalog(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	agents := []agent.Agent{
		{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			Role:      agent.RoleManager,
			Profile:   config.DefaultLLMProfile,
			Provider:  config.ProviderLLMAPI,
			ModelID:   "gpt-5.4",
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	if err := writeSeededAgents(statePath, agents); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}
	svc, err := agent.NewServiceWithLLM(config.SingleProfileLLM(config.ModelConfig{
		Provider: config.ProviderLLMAPI,
		BaseURL:  "http://127.0.0.1:4000",
		APIKey:   "sk-test",
		ModelID:  "gpt-5.4",
	}), config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewServiceWithLLM() error = %v", err)
	}
	bridge := llm.NewService(config.ModelConfig{
		Provider: config.ProviderLLMAPI,
		BaseURL:  "http://127.0.0.1:4000",
		APIKey:   "sk-test",
		ModelID:  "gpt-5.4",
	}, svc)

	srv := &Handler{
		svc:               svc,
		picoclaw:          im.NewPicoClawBridge("secret"),
		llm:               bridge,
		serverAccessToken: "secret",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/bots/u-manager/llm/v1/models", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"gpt-5.4"`) {
		t.Fatalf("body = %s, want model catalog", rec.Body.String())
	}
}

func TestHandlePicoClawModelsLegacyRouteReturnsBridgeCatalog(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	agents := []agent.Agent{
		{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			Role:      agent.RoleManager,
			Profile:   config.DefaultLLMProfile,
			Provider:  config.ProviderLLMAPI,
			ModelID:   "gpt-5.4",
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	if err := writeSeededAgents(statePath, agents); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}
	svc, err := agent.NewServiceWithLLM(config.SingleProfileLLM(config.ModelConfig{
		Provider: config.ProviderLLMAPI,
		BaseURL:  "http://127.0.0.1:4000",
		APIKey:   "sk-test",
		ModelID:  "gpt-5.4",
	}), config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewServiceWithLLM() error = %v", err)
	}
	bridge := llm.NewService(config.ModelConfig{
		Provider: config.ProviderLLMAPI,
		BaseURL:  "http://127.0.0.1:4000",
		APIKey:   "sk-test",
		ModelID:  "gpt-5.4",
	}, svc)

	srv := &Handler{
		svc:               svc,
		picoclaw:          im.NewPicoClawBridge("secret"),
		llm:               bridge,
		serverAccessToken: "secret",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/bots/u-manager/llm/models", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"gpt-5.4"`) {
		t.Fatalf("body = %s, want model catalog", rec.Body.String())
	}
}

func mustNewService(t *testing.T) *agent.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc, err := agent.NewService(config.ModelConfig{
		Provider: config.ProviderLLMAPI,
		BaseURL:  "http://127.0.0.1:4000",
		APIKey:   "sk-test",
		ModelID:  "model-1",
	}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}

func mustNewSeededService(t *testing.T, agents []agent.Agent) *agent.Service {
	t.Helper()

	svc, _ := mustNewSeededServiceWithPath(t, agents)
	return svc
}

func mustNewBotService(t *testing.T, bots []bot.Bot) *bot.Service {
	t.Helper()

	store, err := bot.NewMemoryStore(bots)
	if err != nil {
		t.Fatalf("bot.NewMemoryStore() error = %v", err)
	}
	svc, err := bot.NewService(store)
	if err != nil {
		t.Fatalf("bot.NewService() error = %v", err)
	}
	return svc
}

func mustNewSeededServiceWithPath(t *testing.T, agents []agent.Agent) (*agent.Service, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	if agents == nil {
		agents = []agent.Agent{}
	}

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	if err := writeSeededAgents(statePath, agents); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}

	svc, err := agent.NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc, statePath
}

func writeSeededAgents(statePath string, agents []agent.Agent) error {
	data, err := json.Marshal(map[string]any{
		"agents": agents,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(statePath, append(data, '\n'), 0o600)
}

func containsParticipant(participants []string, want string) bool {
	for _, participant := range participants {
		if participant == want {
			return true
		}
	}
	return false
}

func containsUser(users []im.User, want string) bool {
	for _, user := range users {
		if user.ID == want {
			return true
		}
	}
	return false
}

func mustReceiveEvent(t *testing.T, events <-chan im.Event) im.Event {
	t.Helper()

	select {
	case evt := <-events:
		return evt
	default:
		t.Fatal("expected event")
		return im.Event{}
	}
}

func mustReceiveEventWithin(t *testing.T, events <-chan im.Event, timeout time.Duration) im.Event {
	t.Helper()

	select {
	case evt := <-events:
		return evt
	case <-time.After(timeout):
		t.Fatalf("expected event within %s", timeout)
		return im.Event{}
	}
}

func waitForCondition(t *testing.T, timeout, interval time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(interval)
	}
	t.Fatalf("condition not met within %s", timeout)
}
