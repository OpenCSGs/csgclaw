package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestHandleRoomsMembersListsCsgclawMembers(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "Admin", Handle: "admin", Role: "admin"},
			{ID: "u-alice", Name: "Alice", Handle: "alice", Role: "worker"},
		},
		Rooms: []im.Room{
			{ID: "room-1", Title: "Ops", Members: []string{"u-admin", "u-alice"}},
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
		t.Fatalf("members = %+v, want room members", members)
	}
}

func TestHandleRoomsMembersAddsCsgclawMember(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "Admin", Handle: "admin", Role: "admin"},
			{ID: "u-alice", Name: "Alice", Handle: "alice", Role: "worker"},
		},
		Rooms: []im.Room{
			{ID: "room-1", Title: "Ops", Members: []string{"u-admin"}},
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
	if len(room.Members) != 2 || room.Members[1] != "u-alice" {
		t.Fatalf("members = %+v, want u-admin and u-alice", room.Members)
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
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()
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
		imBus:  bus,
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
	rooms := imSvc.ListRooms()
	if len(rooms) != 1 || !containsMember(rooms[0].Members, "u-admin") || !containsMember(rooms[0].Members, "u-alice") {
		t.Fatalf("rooms = %+v, want bootstrap room with admin and u-alice", rooms)
	}
	first := mustReceiveIMEvent(t, events)
	if first.Type != im.EventTypeUserCreated || first.User == nil || first.User.ID != "u-alice" {
		t.Fatalf("first event = %+v, want user_created for u-alice", first)
	}
	second := mustReceiveIMEvent(t, events)
	if second.Type != im.EventTypeRoomCreated || second.Room == nil {
		t.Fatalf("second event = %+v, want room_created with room payload", second)
	}
	third := mustReceiveIMEventWithin(t, events, 2*time.Second)
	if third.Type != im.EventTypeMessageCreated || third.Message == nil {
		t.Fatalf("third event = %+v, want bootstrap message", third)
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

func TestHandleBotByIDDeleteRemovesCSGClawUser(t *testing.T) {
	store, err := bot.NewMemoryStore([]bot.Bot{
		{
			ID:        "u-alice",
			Name:      "Alice",
			Role:      string(bot.RoleWorker),
			Channel:   string(bot.ChannelCSGClaw),
			AgentID:   "u-alice",
			UserID:    "u-alice",
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin", Handle: "admin", IsOnline: true},
			{ID: "u-alice", Name: "Alice", Handle: "alice", IsOnline: true},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				Title:    "Alice",
				Members:  []string{"u-admin", "u-alice"},
				Messages: []im.Message{{ID: "msg-1", SenderID: "u-alice", Content: "hello"}},
			},
		},
	})
	botSvc, err := bot.NewServiceWithDependencies(store, nil, imSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}
	srv := &Handler{botSvc: botSvc}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/bots/u-alice?channel=csgclaw", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if _, ok := imSvc.User("u-alice"); ok {
		t.Fatal("User(u-alice) ok = true, want false after bot delete")
	}
	if _, ok := imSvc.Room("room-1"); ok {
		t.Fatal("Room(room-1) ok = true, want false after removing DM user")
	}
	bots, err := botSvc.List(string(bot.ChannelCSGClaw), "")
	if err != nil {
		t.Fatalf("List(csgclaw) error = %v", err)
	}
	if len(bots) != 0 {
		t.Fatalf("csgclaw bots = %+v, want deleted", bots)
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

func TestHandleAgentsListHydratesStatusFromSandboxInfo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	runtimeHome, err := agentSandboxRuntimeHomeForTest("alice")
	if err != nil {
		t.Fatalf("agentSandboxRuntimeHomeForTest() error = %v", err)
	}
	provider := sandboxtest.NewProvider()
	rt := sandboxtest.NewRuntime()
	rt.Instances["box-stored"] = sandboxtest.NewInstance(sandbox.Info{
		ID:        "box-live",
		Name:      "alice",
		State:     sandbox.StateRunning,
		CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
	})
	provider.Runtimes[runtimeHome] = rt

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	if err := writeSeededAgents(statePath, []agent.Agent{
		{ID: "u-alice", Name: "alice", BoxID: "box-stored", Role: agent.RoleWorker, Status: "stale", CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}
	svc, err := agent.NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath, agent.WithSandboxProvider(provider))
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(agents) = %d, want 1", len(got))
	}
	if got[0].Status != string(sandbox.StateRunning) || got[0].BoxID != "box-live" {
		t.Fatalf("agent = %+v, want live running status and refreshed box id", got[0])
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

func TestHandleAgentsListRedactsProfileAPIKey(t *testing.T) {
	svc := mustNewSeededService(t, []agent.Agent{
		{
			ID:   "u-alice",
			Name: "alice",
			Role: agent.RoleWorker,
			AgentProfile: agent.AgentProfile{
				Name:            "alice",
				Provider:        agent.ProviderAPI,
				BaseURL:         "https://api.example.test/v1",
				APIKey:          "secret-token",
				ModelID:         "gpt-test",
				ReasoningEffort: agent.DefaultReasoningEffort,
				ProfileComplete: true,
			},
			ProfileComplete: true,
			CreatedAt:       time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		},
	})

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-token") {
		t.Fatalf("response leaked API key: %s", rec.Body.String())
	}
	var got []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	profile, ok := got[0]["agent_profile"].(map[string]any)
	if !ok || profile["api_key_set"] != true {
		t.Fatalf("agent_profile = %#v, want api_key_set true", got[0]["agent_profile"])
	}
	if _, ok := profile["api_key"]; ok {
		t.Fatalf("agent_profile includes api_key: %#v", profile)
	}
}

func TestHandleAgentsPatchUpdatesMetadataAndProfile(t *testing.T) {
	svc := mustNewSeededService(t, []agent.Agent{
		{
			ID:   "u-alice",
			Name: "alice",
			Role: agent.RoleWorker,
			AgentProfile: agent.AgentProfile{
				Name:            "alice",
				Provider:        agent.ProviderCSGHubLite,
				ModelID:         "old-model",
				ReasoningEffort: agent.DefaultReasoningEffort,
				ProfileComplete: true,
			},
			ProfileComplete: true,
			CreatedAt:       time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		},
	})

	srv := &Handler{svc: svc}
	body := `{"description":"new role","agent_profile":{"name":"alice","provider":"csghub_lite","model_id":"new-model","env":{"A":"B"}}}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/u-alice", strings.NewReader(body))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["description"] != "new role" || got["model_id"] != "new-model" {
		t.Fatalf("agent = %#v, want updated description/model", got)
	}
	profile, ok := got["agent_profile"].(map[string]any)
	if !ok || profile["env_restart_required"] != true {
		t.Fatalf("agent_profile = %#v, want env_restart_required true", got["agent_profile"])
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-alice/logs?lines=80", nil)
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
	if strings.Join(gotArgs, " ") != "-n 80 /home/picoclaw/.picoclaw/workspace/gateway.log" {
		t.Fatalf("args = %q", gotArgs)
	}
	if rec.Body.String() != "hello\nworld\n" {
		t.Fatalf("body = %q, want streamed logs", rec.Body.String())
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

func TestHandleAgentStartStartsExistingBox(t *testing.T) {
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	agentSvc, statePath := mustNewSeededServiceWithPath(t, []agent.Agent{
		{ID: "u-alice", Name: "alice", BoxID: "box-old", Role: agent.RoleWorker, Status: "stopped", CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	})
	if err := writeSeededAgents(statePath, []agent.Agent{
		{ID: "u-alice", Name: "alice", BoxID: "box-new", Role: agent.RoleWorker, Status: "stopped", CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}

	var gotBoxID string
	var startCalls int
	agent.TestOnlySetGetBoxHook(func(_ *agent.Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		gotBoxID = idOrName
		return sandboxtest.NewInstance(sandbox.Info{ID: idOrName, Name: "alice", State: sandbox.StateStopped}), nil
	})
	agent.TestOnlySetStartBoxHook(func(_ *agent.Service, _ context.Context, _ sandbox.Instance) error {
		startCalls++
		return nil
	})
	agent.TestOnlySetBoxInfoHook(func(_ *agent.Service, _ context.Context, _ sandbox.Instance) (sandbox.Info, error) {
		return sandbox.Info{ID: "box-new", Name: "alice", State: sandbox.StateRunning}, nil
	})
	defer func() {
		agent.TestOnlySetGetBoxHook(nil)
		agent.TestOnlySetStartBoxHook(nil)
		agent.TestOnlySetBoxInfoHook(nil)
	}()

	srv := &Handler{svc: agentSvc}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/u-alice/start", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotBoxID != "box-new" {
		t.Fatalf("getBox() idOrName = %q, want %q", gotBoxID, "box-new")
	}
	if startCalls != 1 {
		t.Fatalf("startBox() calls = %d, want 1", startCalls)
	}
	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Status != "running" || got.BoxID != "box-new" {
		t.Fatalf("agent = %+v, want running box-new", got)
	}
}

func TestHandleAgentStopStopsExistingBox(t *testing.T) {
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	agentSvc, statePath := mustNewSeededServiceWithPath(t, []agent.Agent{
		{ID: "u-alice", Name: "alice", BoxID: "box-old", Role: agent.RoleWorker, Status: "running", CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	})
	if err := writeSeededAgents(statePath, []agent.Agent{
		{ID: "u-alice", Name: "alice", BoxID: "box-new", Role: agent.RoleWorker, Status: "running", CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}

	var gotBoxID string
	var stopCalls int
	agent.TestOnlySetGetBoxHook(func(_ *agent.Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		gotBoxID = idOrName
		return sandboxtest.NewInstance(sandbox.Info{ID: idOrName, Name: "alice", State: sandbox.StateRunning}), nil
	})
	agent.TestOnlySetStopBoxHook(func(_ *agent.Service, _ context.Context, _ sandbox.Instance, opts sandbox.StopOptions) error {
		stopCalls++
		if opts != (sandbox.StopOptions{}) {
			t.Fatalf("Stop() opts = %+v, want zero value", opts)
		}
		return nil
	})
	agent.TestOnlySetBoxInfoHook(func(_ *agent.Service, _ context.Context, _ sandbox.Instance) (sandbox.Info, error) {
		return sandbox.Info{ID: "box-new", Name: "alice", State: sandbox.StateStopped}, nil
	})
	defer func() {
		agent.TestOnlySetGetBoxHook(nil)
		agent.TestOnlySetStopBoxHook(nil)
		agent.TestOnlySetBoxInfoHook(nil)
	}()

	srv := &Handler{svc: agentSvc}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/u-alice/stop", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotBoxID != "box-new" {
		t.Fatalf("getBox() idOrName = %q, want %q", gotBoxID, "box-new")
	}
	if stopCalls != 1 {
		t.Fatalf("stopBox() calls = %d, want 1", stopCalls)
	}
	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Status != "stopped" || got.BoxID != "box-new" {
		t.Fatalf("agent = %+v, want stopped box-new", got)
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

func TestHandleAgentsCreateProvisionsIMDirectMessage(t *testing.T) {
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
	user, ok := srv.im.User("u-alice")
	if !ok {
		t.Fatal("User(u-alice) ok = false, want provisioned IM user after agent create")
	}
	if user.Handle != "alice" || user.Role != "worker" {
		t.Fatalf("User(u-alice) = %+v, want worker handle alice", user)
	}
	rooms := srv.im.ListRooms()
	if len(rooms) != 1 {
		t.Fatalf("rooms = %+v, want one direct room after agent create", rooms)
	}
	room := rooms[0]
	if !room.IsDirect || !roomHasMember(room, "u-admin") || !roomHasMember(room, "u-alice") {
		t.Fatalf("room = %+v, want admin/agent direct room", room)
	}

	first := mustReceiveAPIEvent(t, events)
	if first.Type != im.EventTypeUserCreated || first.User == nil || first.User.ID != "u-alice" {
		t.Fatalf("first event = %+v, want user-created for u-alice", first)
	}
	second := mustReceiveAPIEvent(t, events)
	if second.Type != im.EventTypeRoomCreated || second.Room == nil || second.Room.ID != room.ID {
		t.Fatalf("second event = %+v, want room-created for %s", second, room.ID)
	}
}

func roomHasMember(room im.Room, userID string) bool {
	for _, memberID := range room.Members {
		if memberID == userID {
			return true
		}
	}
	return false
}

func mustReceiveAPIEvent(t *testing.T, events <-chan im.Event) im.Event {
	t.Helper()
	select {
	case evt := <-events:
		return evt
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for IM event")
		return im.Event{}
	}
}

func TestHandleAgentsCreateManagerUsesBootstrapManager(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc := mustNewService(t)
	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"id":"manager","name":"manager"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != agent.ManagerUserID || got.Name != agent.ManagerName || got.Role != agent.RoleManager {
		t.Fatalf("agent = %+v, want bootstrapped manager", got)
	}
}

func TestHandleAgentsCreateReplaceUsesUnifiedServiceEntry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc := mustNewService(t)
	if _, err := svc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			ID:   "u-alice",
			Name: "alice",
			Role: agent.RoleWorker,
		},
	}); err != nil {
		t.Fatalf("seed Create() error = %v", err)
	}

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"id":"u-alice","name":"alice-v2","role":"worker","replace":true}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "u-alice" || got.Name != "alice-v2" || got.Role != agent.RoleWorker {
		t.Fatalf("agent = %+v, want replaced worker", got)
	}
}

func TestHandleAgentsCreateReplaceFieldMaskMergesInService(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc := mustNewService(t)
	if _, err := svc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			ID:          "u-alice",
			Name:        "alice",
			Description: "worker",
			Image:       "agent-image:v1",
			Role:        agent.RoleWorker,
		},
	}); err != nil {
		t.Fatalf("seed Create() error = %v", err)
	}

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"id":"u-alice","name":"alice-v2","description":"","image":"agent-image:v2","replace":true,"field_mask":["id","name"]}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "u-alice" || got.Name != "alice-v2" || got.Description != "worker" || got.Image != "agent-image:v1" {
		t.Fatalf("agent = %+v, want masked replace preserving unmasked fields", got)
	}
}

func TestHandleAgentsCreateReplaceManagerUsesUnifiedServiceEntry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc := mustNewService(t)
	if _, err := svc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			ID:   agent.ManagerUserID,
			Name: agent.ManagerName,
		},
	}); err != nil {
		t.Fatalf("seed Create() error = %v", err)
	}

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"id":"u-manager","name":"manager","replace":true}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != agent.ManagerUserID || got.Name != agent.ManagerName || got.Role != agent.RoleManager {
		t.Fatalf("agent = %+v, want replaced manager", got)
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

func TestHandleBootstrapReloadsIMStateBeforeResponse(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	initial := im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin", Handle: "admin", Role: "admin"},
			{ID: "u-manager", Name: "manager", Handle: "manager", Role: "manager"},
		},
	}
	if err := im.SaveBootstrap(statePath, initial); err != nil {
		t.Fatalf("SaveBootstrap() initial error = %v", err)
	}

	imSvc, err := im.NewServiceFromPath(statePath)
	if err != nil {
		t.Fatalf("NewServiceFromPath() error = %v", err)
	}

	updated := initial
	updated.Rooms = []im.Room{{
		ID:       "room-1",
		Title:    "admin & manager",
		IsDirect: true,
		Members:  []string{"u-admin", "u-manager"},
	}}
	if err := im.SaveBootstrap(statePath, updated); err != nil {
		t.Fatalf("SaveBootstrap() updated error = %v", err)
	}

	srv := &Handler{im: imSvc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bootstrap", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got struct {
		Rooms []im.Room `json:"rooms"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Rooms) != 1 || got.Rooms[0].ID != "room-1" {
		t.Fatalf("rooms = %+v, want reloaded room-1", got.Rooms)
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
					ID:      "room-1",
					Title:   "Room One",
					Members: []string{"u-admin"},
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
	if !containsMember(got.Members, "u-manager") {
		t.Fatalf("members = %+v, want u-manager to be invited", got.Members)
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
					ID:       "room-1",
					Title:    "Room One",
					Members:  []string{"u-admin"},
					Messages: []im.Message{{ID: "msg-1", SenderID: "u-admin", Content: "hello"}},
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
	if room, ok := srv.im.Room("room-1"); !ok || !containsMember(room.Members, "u-alice") {
		t.Fatalf("room members = %+v, want agent joined", room.Members)
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

func TestHandleRoomsReturnsConversationList(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-alice", Name: "Alice", Handle: "alice"},
			},
			Rooms: []im.Room{
				{
					ID:      "room-1",
					Title:   "Room One",
					Members: []string{"u-admin", "u-alice"},
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

func TestHandleRoomsReloadsIMStateBeforeList(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	initial := im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin", Handle: "admin", Role: "admin"},
			{ID: "u-manager", Name: "manager", Handle: "manager", Role: "manager"},
		},
	}
	if err := im.SaveBootstrap(statePath, initial); err != nil {
		t.Fatalf("SaveBootstrap() initial error = %v", err)
	}

	imSvc, err := im.NewServiceFromPath(statePath)
	if err != nil {
		t.Fatalf("NewServiceFromPath() error = %v", err)
	}

	updated := initial
	updated.Rooms = []im.Room{{
		ID:       "room-1",
		Title:    "admin & manager",
		IsDirect: true,
		Members:  []string{"u-admin", "u-manager"},
	}}
	if err := im.SaveBootstrap(statePath, updated); err != nil {
		t.Fatalf("SaveBootstrap() updated error = %v", err)
	}

	srv := &Handler{im: imSvc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []im.Conversation
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "room-1" {
		t.Fatalf("rooms = %+v, want reloaded room-1", got)
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

func TestHandleUsersCreateProvisionsIMUser(t *testing.T) {
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()

	imSvc := im.NewService()
	srv := &Handler{
		im:    imSvc,
		imBus: bus,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"id":"u-alice","name":"Alice","handle":"alice","role":"worker"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got im.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "u-alice" || got.Name != "alice" || got.Handle != "alice" || got.Role != "worker" {
		t.Fatalf("user = %+v, want normalized provisioned user", got)
	}

	if _, ok := srv.im.User("u-alice"); !ok {
		t.Fatal("User(u-alice) ok = false, want true after create")
	}
	rooms := srv.im.ListRooms()
	if len(rooms) != 1 || !containsMember(rooms[0].Members, "u-admin") || !containsMember(rooms[0].Members, "u-alice") {
		t.Fatalf("rooms = %+v, want one bootstrap room with admin and u-alice", rooms)
	}

	first := mustReceiveIMEvent(t, events)
	if first.Type != im.EventTypeUserCreated || first.User == nil || first.User.ID != "u-alice" {
		t.Fatalf("first event = %+v, want user_created for u-alice", first)
	}
	second := mustReceiveIMEvent(t, events)
	if second.Type != im.EventTypeRoomCreated || second.Room == nil || second.Room.ID == "" {
		t.Fatalf("second event = %+v, want room_created for bootstrap room", second)
	}
}

func TestHandleUsersCreateWithBotServiceCreatesWorkerAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	agentSvc := mustNewService(t)
	imSvc := im.NewService()
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()

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
		imBus:  bus,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"id":"u-qa","name":"qa","handle":"qa","role":"qa"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got im.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "u-qa" || got.Name != "qa" || got.Handle != "qa" || got.Role != "worker" {
		t.Fatalf("user = %+v, want qa worker user", got)
	}

	created, ok := agentSvc.Agent("u-qa")
	if !ok {
		t.Fatal("Agent(u-qa) ok = false, want worker agent created with IM user")
	}
	if created.Name != "qa" || created.Role != agent.RoleWorker {
		t.Fatalf("agent = %+v, want qa worker", created)
	}

	bots, err := botSvc.List(string(bot.ChannelCSGClaw), string(bot.RoleWorker))
	if err != nil {
		t.Fatalf("List(worker) error = %v", err)
	}
	if len(bots) != 1 || bots[0].ID != "u-qa" || bots[0].AgentID != "u-qa" || bots[0].UserID != "u-qa" {
		t.Fatalf("bots = %+v, want one qa worker bot", bots)
	}

	first := mustReceiveIMEvent(t, events)
	if first.Type != im.EventTypeUserCreated || first.User == nil || first.User.ID != "u-qa" {
		t.Fatalf("first event = %+v, want user_created for u-qa", first)
	}
	second := mustReceiveIMEvent(t, events)
	if second.Type != im.EventTypeRoomCreated || second.Room == nil || !containsMember(second.Room.Members, "u-qa") {
		t.Fatalf("second event = %+v, want qa direct room", second)
	}
}

func TestHandleUsersCreateDefaultsHandleFromName(t *testing.T) {
	srv := &Handler{im: im.NewService()}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"id":"u-alice","name":"Alice"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got im.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Handle != "alice" {
		t.Fatalf("user.Handle = %q, want %q", got.Handle, "alice")
	}
}

func TestHandleUsersCreateRejectsMissingID(t *testing.T) {
	srv := &Handler{im: im.NewService()}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"name":"Alice","handle":"alice"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func mustReceiveIMEvent(t *testing.T, events <-chan im.Event) im.Event {
	t.Helper()
	select {
	case evt := <-events:
		return evt
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for IM event")
		return im.Event{}
	}
}

func mustReceiveIMEventWithin(t *testing.T, events <-chan im.Event, timeout time.Duration) im.Event {
	t.Helper()
	select {
	case evt := <-events:
		return evt
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for IM event after %s", timeout)
		return im.Event{}
	}
}

func TestHandleMessagesReturnsConversationMessages(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Rooms: []im.Room{
				{
					ID:      "room-1",
					Title:   "Room One",
					Members: []string{"u-admin", "u-manager"},
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
					ID:      "room-1",
					Title:   "Room One",
					Members: []string{"u-admin", "u-manager"},
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
	if len(got.Mentions) != 1 || got.Mentions[0].ID != "u-manager" || got.Mentions[0].Name != "manager" {
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
					ID:      "room-1",
					Title:   "Room One",
					Members: []string{"u-admin", "u-dev", "u-manager"},
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
	if got.Content != `<at user_id="u-dev">dev</at> hi` {
		t.Fatalf(`content = %q, want <at user_id="u-dev">dev</at> hi`, got.Content)
	}
	if len(got.Mentions) != 1 || got.Mentions[0].ID != "u-dev" || got.Mentions[0].Name != "dev" {
		t.Fatalf("mentions = %+v, want u-dev", got.Mentions)
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
			Mentions: []im.Mention{{ID: "u-worker"}},
		},
	})
	feishuSvc.MessageBus().Publish(channel.FeishuMessageEvent{
		Type:   channel.FeishuMessageEventTypeMessageCreated,
		RoomID: "oc_alpha",
		Message: &im.Message{
			ID:       "om_1",
			SenderID: "ou_manager",
			Content:  "hello @alice",
			Mentions: []im.Mention{{ID: "u-manager"}},
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"title":"Launch","description":"coordination","creator_id":"u-admin","member_ids":["u-alice","u-manager"],"locale":"en"}`))
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
	if !containsMember(got.Members, "u-admin") || !containsMember(got.Members, "u-alice") || !containsMember(got.Members, "u-manager") {
		t.Fatalf("members = %+v, want admin, alice, and manager", got.Members)
	}
}

func TestHandleUsersDeleteRemovesUser(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Handle: "admin", IsOnline: true},
				{ID: "u-alice", Name: "Alice", Handle: "alice", IsOnline: true},
			},
			Rooms: []im.Room{
				{
					ID:       "room-1",
					Title:    "Room One",
					Members:  []string{"u-admin", "u-alice"},
					Messages: []im.Message{{ID: "msg-1", SenderID: "u-alice", Content: "hello"}},
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
		t.Fatal("Room() ok = true, want false for DM after deleted user")
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
				{ID: "room-1", Title: "Room One", Members: []string{"u-admin", "u-manager"}},
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

func TestPublishPicoClawEventQueuesUntilBotSubscribes(t *testing.T) {
	now := time.Now().UTC()
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-manager", Name: "manager", Handle: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "u-manager"},
				Messages: []im.Message{
					{
						ID:        "msg-pending",
						SenderID:  "u-admin",
						Content:   "queued while disconnected",
						CreatedAt: now,
					},
				},
			},
		},
	})
	bridge := im.NewPicoClawBridge("")
	srv := &Handler{im: imSvc, picoclaw: bridge}

	sender, ok := imSvc.User("u-admin")
	if !ok {
		t.Fatal("missing sender")
	}
	room, ok := imSvc.Room("room-1")
	if !ok || len(room.Messages) != 1 {
		t.Fatalf("room = %+v, want one message", room)
	}

	srv.PublishPicoClawEvent(im.Event{
		Type:    im.EventTypeMessageCreated,
		RoomID:  "room-1",
		Sender:  &sender,
		Message: &room.Messages[0],
	})

	events, cancel := bridge.Subscribe("u-manager")
	defer cancel()

	select {
	case evt := <-events:
		if evt.MessageID != "msg-pending" || evt.Text != "queued while disconnected" {
			t.Fatalf("queued event = %+v, want msg-pending queued while disconnected", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("Subscribe() timed out waiting for queued PicoClaw event")
	}
}

func TestPublishPicoClawEventReconnectsMissedWorker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	created := make(chan sandbox.CreateSpec, 1)
	provider := &sandboxtest.Provider{
		NameValue: "fake",
		OpenFunc: func(context.Context, string) (sandbox.Runtime, error) {
			return &sandboxtest.Runtime{
				GetFunc: func(context.Context, string) (sandbox.Instance, error) {
					return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
				},
				RemoveFunc: func(context.Context, string, sandbox.RemoveOptions) error {
					return fmt.Errorf("%w: missing", sandbox.ErrNotFound)
				},
				CreateFunc: func(_ context.Context, spec sandbox.CreateSpec) (sandbox.Instance, error) {
					created <- spec
					return sandboxtest.NewInstance(sandbox.Info{
						ID:        "box-" + spec.Name,
						Name:      spec.Name,
						State:     sandbox.StateRunning,
						CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
					}), nil
				},
			}, nil
		},
	}
	t.Cleanup(agent.TestOnlySetSandboxProvider(provider))

	statePath := filepath.Join(t.TempDir(), "agents.json")
	if err := writeSeededAgents(statePath, []agent.Agent{
		{
			ID:              "u-worker",
			Name:            "worker",
			Role:            agent.RoleWorker,
			BoxID:           "box-stale",
			Status:          string(sandbox.StateRunning),
			AgentProfile:    agent.AgentProfile{Name: "worker", Provider: agent.ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true},
			ProfileComplete: true,
			CreatedAt:       time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}
	svc, err := agent.NewService(
		config.ModelConfig{ModelID: "gpt-5.5"},
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "token"},
		"",
		statePath,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-worker", Name: "worker", Handle: "worker"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				Members:  []string{"u-admin", "u-worker"},
				Messages: []im.Message{{ID: "msg-1", SenderID: "u-admin", Content: "please handle this", CreatedAt: time.Now().UTC()}},
			},
		},
	})
	sender, ok := imSvc.User("u-admin")
	if !ok {
		t.Fatal("missing sender")
	}
	room, ok := imSvc.Room("room-1")
	if !ok || len(room.Messages) != 1 {
		t.Fatalf("room = %+v, want one message", room)
	}

	srv := &Handler{svc: svc, im: imSvc, picoclaw: im.NewPicoClawBridge("")}
	srv.PublishPicoClawEvent(im.Event{
		Type:    im.EventTypeMessageCreated,
		RoomID:  "room-1",
		Sender:  &sender,
		Message: &room.Messages[0],
	})

	select {
	case spec := <-created:
		if spec.Name != "worker" {
			t.Fatalf("recreated spec.Name = %q, want worker", spec.Name)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for missed worker reconnect")
	}
}

func TestHandlePicoClawEventsRequeuesWhenSSEWriteFails(t *testing.T) {
	now := time.Now().UTC()
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-manager", Name: "manager", Handle: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "u-manager"},
				Messages: []im.Message{
					{
						ID:        "msg-retry",
						SenderID:  "u-admin",
						Content:   "retry after broken stream",
						CreatedAt: now,
					},
				},
			},
		},
	})
	bridge := im.NewPicoClawBridge("")
	room, ok := imSvc.Room("room-1")
	if !ok {
		t.Fatal("Room(room-1) = false, want room")
	}
	sender, ok := imSvc.User("u-admin")
	if !ok {
		t.Fatal("User(u-admin) = false, want user")
	}
	bridge.PublishMessageEvent(room, sender, room.Messages[0])

	srv := &Handler{im: imSvc, picoclaw: bridge}
	req := httptest.NewRequest(http.MethodGet, "/api/bots/u-manager/events", nil)
	srv.handlePicoClawEvents(&failingPicoClawEventWriter{header: make(http.Header)}, req, "u-manager")

	events, cancel := bridge.Subscribe("u-manager")
	defer cancel()
	select {
	case evt := <-events:
		if evt.MessageID != "msg-retry" || evt.Text != "retry after broken stream" {
			t.Fatalf("requeued event = %+v, want msg-retry retry after broken stream", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("Subscribe() timed out waiting for requeued event")
	}
}

type failingPicoClawEventWriter struct {
	header http.Header
}

func (w *failingPicoClawEventWriter) Header() http.Header {
	return w.header
}

func (w *failingPicoClawEventWriter) Write(data []byte) (int, error) {
	text := string(data)
	if strings.HasPrefix(text, "id: ") || strings.HasPrefix(text, "event: message") {
		return 0, errors.New("broken stream")
	}
	return len(data), nil
}

func (w *failingPicoClawEventWriter) WriteHeader(int) {}

func (w *failingPicoClawEventWriter) Flush() {}

func TestReplayRecentPicoClawMessagesReplaysUnansweredHumanMessage(t *testing.T) {
	now := time.Now().UTC()
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-manager", Name: "manager", Handle: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "u-manager"},
				Messages: []im.Message{
					{
						ID:        "msg-missed",
						SenderID:  "u-admin",
						Content:   "please reply",
						CreatedAt: now,
					},
				},
			},
		},
	})
	bridge := im.NewPicoClawBridge("")
	events, cancel := bridge.Subscribe("u-manager")
	defer cancel()

	srv := &Handler{im: imSvc, picoclaw: bridge}
	srv.replayRecentPicoClawMessages("u-manager", "")

	select {
	case evt := <-events:
		if evt.MessageID != "msg-missed" || evt.Text != "please reply" {
			t.Fatalf("replayed event = %+v, want msg-missed please reply", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("replayRecentPicoClawMessages() timed out waiting for event")
	}
}

func TestReplayRecentPicoClawMessagesSkipsAnsweredMessage(t *testing.T) {
	now := time.Now().UTC()
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-manager", Name: "manager", Handle: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "u-manager"},
				Messages: []im.Message{
					{
						ID:        "msg-answered",
						SenderID:  "u-admin",
						Content:   "please reply",
						CreatedAt: now,
					},
					{
						ID:        "msg-reply",
						SenderID:  "u-manager",
						Content:   "done",
						CreatedAt: now.Add(time.Second),
					},
				},
			},
		},
	})
	bridge := im.NewPicoClawBridge("")
	events, cancel := bridge.Subscribe("u-manager")
	defer cancel()

	srv := &Handler{im: imSvc, picoclaw: bridge}
	srv.replayRecentPicoClawMessages("u-manager", "")

	select {
	case evt := <-events:
		t.Fatalf("replayed event = %+v, want no replay for answered message", evt)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestReplayRecentPicoClawMessagesDoesNotDuplicateDeliveredMessage(t *testing.T) {
	now := time.Now().UTC()
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-manager", Name: "manager", Handle: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "u-manager"},
				Messages: []im.Message{
					{
						ID:        "msg-delivered",
						SenderID:  "u-admin",
						Content:   "please reply",
						CreatedAt: now,
					},
				},
			},
		},
	})
	bridge := im.NewPicoClawBridge("")
	events, cancel := bridge.Subscribe("u-manager")
	defer cancel()

	room, ok := imSvc.Room("room-1")
	if !ok {
		t.Fatal("Room(room-1) = false, want room")
	}
	sender, ok := imSvc.User("u-admin")
	if !ok {
		t.Fatal("User(u-admin) = false, want user")
	}
	bridge.PublishMessageEvent(room, sender, room.Messages[0])

	select {
	case evt := <-events:
		if evt.MessageID != "msg-delivered" {
			t.Fatalf("live event = %+v, want msg-delivered", evt)
		}
		bridge.Ack("u-manager", evt.MessageID)
	case <-time.After(time.Second):
		t.Fatal("PublishMessageEvent() timed out waiting for event")
	}

	srv := &Handler{im: imSvc, picoclaw: bridge}
	srv.replayRecentPicoClawMessages("u-manager", "")

	select {
	case evt := <-events:
		t.Fatalf("replayed event = %+v, want no duplicate for delivered message", evt)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestReplayRecentPicoClawMessagesHonorsLastEventID(t *testing.T) {
	now := time.Now().UTC()
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin", Handle: "admin"},
			{ID: "u-manager", Name: "manager", Handle: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "u-manager"},
				Messages: []im.Message{
					{
						ID:        "msg-seen",
						SenderID:  "u-admin",
						Content:   "already delivered",
						CreatedAt: now,
					},
					{
						ID:        "msg-new",
						SenderID:  "u-admin",
						Content:   "new after reconnect",
						CreatedAt: now.Add(time.Second),
					},
				},
			},
		},
	})
	bridge := im.NewPicoClawBridge("")
	events, cancel := bridge.Subscribe("u-manager")
	defer cancel()

	srv := &Handler{im: imSvc, picoclaw: bridge}
	srv.replayRecentPicoClawMessages("u-manager", "msg-seen")

	select {
	case evt := <-events:
		if evt.MessageID != "msg-new" || evt.Text != "new after reconnect" {
			t.Fatalf("replayed event = %+v, want msg-new new after reconnect", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("replayRecentPicoClawMessages() timed out waiting for event")
	}

	select {
	case evt := <-events:
		t.Fatalf("extra replayed event = %+v, want only messages after Last-Event-ID", evt)
	case <-time.After(50 * time.Millisecond):
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

func agentSandboxRuntimeHomeForTest(agentName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, config.AppDirName, "agents", agentName, config.RuntimeHomeDirName), nil
}

func containsMember(members []string, want string) bool {
	for _, member := range members {
		if member == want {
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
