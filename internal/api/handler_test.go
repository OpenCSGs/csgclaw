package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/app/runtimewiring"
	"csgclaw/internal/bot"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	"csgclaw/internal/hub"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/picoclawsandbox"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/sandboxtest"
)

type fakeCompatRuntime struct {
	kind  string
	new   func(context.Context, agentruntime.Spec) (agentruntime.Handle, error)
	start func(context.Context, agentruntime.Handle) (agentruntime.State, error)
	stop  func(context.Context, agentruntime.Handle) (agentruntime.State, error)
	del   func(context.Context, agentruntime.Handle) error
	info  func(context.Context, agentruntime.Handle) (agentruntime.Info, error)
}

func init() {
	_ = agent.TestOnlySetDefaultServiceOption(func(s *agent.Service) error {
		if err := runtimewiring.WithPicoClawSandboxRuntime(nil)(s); err != nil {
			return err
		}
		return runtimewiring.WithOpenClawSandboxRuntime(nil)(s)
	})
	_ = agent.TestOnlySetResponsesAPIProbe(func(context.Context, string, string, string, map[string]string) error {
		return nil
	})
}

func testFeishuBotInfoResolver(t *testing.T, openIDsByAppID map[string]string) func(context.Context, feishu.AppConfig) (feishu.BotInfo, error) {
	t.Helper()
	return func(_ context.Context, app feishu.AppConfig) (feishu.BotInfo, error) {
		openID, ok := openIDsByAppID[app.AppID]
		if !ok {
			t.Fatalf("unexpected Feishu app_id %q", app.AppID)
		}
		return feishu.BotInfo{OpenID: openID}, nil
	}
}

func (f fakeCompatRuntime) Kind() string {
	if strings.TrimSpace(f.kind) != "" {
		return strings.TrimSpace(f.kind)
	}
	return agent.RuntimeKindPicoClawSandbox
}

func (f fakeCompatRuntime) New(ctx context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
	if f.new != nil {
		return f.new(ctx, spec)
	}
	return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "box-" + spec.AgentName}, nil
}

func (f fakeCompatRuntime) Start(ctx context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	if f.start != nil {
		return f.start(ctx, h)
	}
	return agentruntime.StateRunning, nil
}

func (f fakeCompatRuntime) Stop(ctx context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	if f.stop != nil {
		return f.stop(ctx, h)
	}
	return agentruntime.StateStopped, nil
}

func (f fakeCompatRuntime) Delete(ctx context.Context, h agentruntime.Handle) error {
	if f.del != nil {
		return f.del(ctx, h)
	}
	return nil
}

func (f fakeCompatRuntime) State(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateRunning, nil
}

func (f fakeCompatRuntime) Info(ctx context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
	if f.info != nil {
		return f.info(ctx, h)
	}
	return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
}

func (f fakeCompatRuntime) EnsureGatewayConfig(string, string, string) error {
	return nil
}

func (f fakeCompatRuntime) ProjectsGuestPath() string {
	return ""
}

type fakeCodexBridgeController struct {
	ensureCalls []agent.Agent
	stopCalls   []string
	ensureErr   error
}

func (f *fakeCodexBridgeController) EnsureAgent(_ context.Context, a agent.Agent) error {
	f.ensureCalls = append(f.ensureCalls, a)
	return f.ensureErr
}

func (f *fakeCodexBridgeController) StopAgent(agentID string) {
	f.stopCalls = append(f.stopCalls, agentID)
}

func TestParseBotCompatibilityPath(t *testing.T) {
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
		{path: "/api/bots/u-manager/llm/responses", wantBotID: "u-manager", wantAction: "llm/responses", wantOK: true},
		{path: "/api/bots/u-manager/llm/v1/responses", wantBotID: "u-manager", wantAction: "llm/v1/responses", wantOK: true},
		{path: "/api/bots/u-manager", wantOK: false},
		{path: "/api/bots//events", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			gotBotID, gotAction, gotOK := parseBotCompatibilityPath(tt.path)
			if gotBotID != tt.wantBotID || gotAction != tt.wantAction || gotOK != tt.wantOK {
				t.Fatalf("parseBotCompatibilityPath(%q) = (%q, %q, %v), want (%q, %q, %v)", tt.path, gotBotID, gotAction, gotOK, tt.wantBotID, tt.wantAction, tt.wantOK)
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
	srv := &Handler{feishu: feishu.NewService()}

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

func TestHandleVersion(t *testing.T) {
	srv := &Handler{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got apitypes.VersionResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Version == "" {
		t.Fatal("version is empty")
	}
}

func TestHandleVersionMethodNotAllowed(t *testing.T) {
	srv := &Handler{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/version", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
}

func TestBootstrapConfigViewUsesServerUpgradeVisibility(t *testing.T) {
	tests := []struct {
		name        string
		configValue bool
		showUpgrade bool
	}{
		{name: "shown", configValue: true, showUpgrade: true},
		{name: "hidden", configValue: false, showUpgrade: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bootstrapConfigView(context.Background(), config.Config{
				Server: config.ServerConfig{ShowUpgrade: tt.configValue},
			}, nil)

			if got.ShowUpgrade != tt.showUpgrade {
				t.Fatalf("ShowUpgrade = %t, want %t", got.ShowUpgrade, tt.showUpgrade)
			}
		})
	}
}

func TestHandleFeishuRoomsMembers(t *testing.T) {
	feishuSvc := feishu.NewServiceWithCreateChatAndAddMembers(
		map[string]feishu.AppConfig{
			"u-manager": {AppID: "manager-app-id", AppSecret: "app-secret", AdminOpenID: "ou_admin"},
			"fsu-alice": {AppID: "alice-app-id", AppSecret: "alice-secret"},
		},
		func(_ context.Context, _ feishu.AppConfig, req feishu.CreateChatRequest) (feishu.CreateChatResponse, error) {
			return feishu.CreateChatResponse{
				ChatID:      "oc_alpha",
				Name:        req.Title,
				Description: req.Description,
			}, nil
		},
		func(context.Context, feishu.AppConfig, feishu.AddChatMembersRequest) error { return nil },
		func(context.Context, feishu.AppConfig, map[string]feishu.AppConfig, string) ([]im.User, error) {
			return []im.User{
				{ID: "fsu-admin", Name: "Admin"},
				{ID: "fsu-alice", Name: "Alice"},
			}, nil
		},
	)
	feishuSvc.SetBotOpenIDResolver(testFeishuBotInfoResolver(t, map[string]string{
		"manager-app-id": "fsu-admin",
		"alice-app-id":   "fsu-alice",
	}))
	if _, err := feishuSvc.CreateUser(feishu.CreateUserRequest{ID: "fsu-admin", Name: "Admin"}); err != nil {
		t.Fatalf("CreateUser(admin) error = %v", err)
	}
	if _, err := feishuSvc.CreateUser(feishu.CreateUserRequest{ID: "fsu-alice", Name: "Alice"}); err != nil {
		t.Fatalf("CreateUser(alice) error = %v", err)
	}
	srv := &Handler{feishu: feishuSvc}

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
	if members[0].ID != "u-manager" || members[1].ID != "fsu-alice" {
		t.Fatalf("members = %+v, want bot ids", members)
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

func TestHandleBotsListUsesChannelPath(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/bots", nil)
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
		t.Fatalf("bots = %+v, want only csgclaw bot", got)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/bots", nil)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/bots?role=worker", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/unknown/bots", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleBotsListRejectsInvalidRole(t *testing.T) {
	srv := &Handler{botSvc: mustNewBotService(t, nil)}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/bots?role=agent", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleBotsListRequiresService(t *testing.T) {
	srv := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/bots", nil)
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/bots", strings.NewReader(`{"name":"alice","description":"test lead","image":"agent-image:1","role":"worker","runtime_kind":"picoclaw_sandbox","agent_profile":{"provider":"csghub_lite","model_id":"glm-4.5","reasoning_effort":"high"}}`))
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
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/bots", nil))
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
	var agents []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&agents); err != nil {
		t.Fatalf("decode agents response: %v", err)
	}
	if len(agents) != 1 || agents[0]["id"] != "u-alice" {
		t.Fatalf("agents = %+v, want u-alice", agents)
	}
	if agents[0]["image"] != "agent-image:1" {
		t.Fatalf("agents[0].image = %#v, want agent-image:1", agents[0]["image"])
	}
	profile, ok := agents[0]["agent_profile"].(map[string]any)
	if !ok || profile["provider"] != agent.ProviderCSGHubLite || profile["model_id"] != "glm-4.5" {
		t.Fatalf("agent_profile = %#v, want csghub_lite/glm-4.5", agents[0]["agent_profile"])
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

func TestHandleBotsCreateCodexWorkerEnsuresCodexBridge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	agentSvc, err := agent.NewService(
		config.ModelConfig{
			Provider: config.ProviderLLMAPI,
			BaseURL:  "http://127.0.0.1:4000",
			APIKey:   "sk-test",
			ModelID:  "model-1",
		},
		config.ServerConfig{}, "manager-image:test", "",
		agent.WithRuntime(fakeCompatRuntime{
			kind: agent.RuntimeKindCodex,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-" + spec.AgentName}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	imSvc := im.NewService()
	bus := im.NewBus()
	store, err := bot.NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("bot.NewMemoryStore() error = %v", err)
	}
	botSvc, err := bot.NewServiceWithDependencies(store, agentSvc, imSvc)
	if err != nil {
		t.Fatalf("bot.NewServiceWithDependencies() error = %v", err)
	}
	bridge := &fakeCodexBridgeController{}
	agentSvc.SetLifecycleObserver(bridge)
	srv := &Handler{
		svc:    agentSvc,
		botSvc: botSvc,
		im:     imSvc,
		imBus:  bus,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/bots", strings.NewReader(`{"name":"alice","role":"worker","runtime_kind":"codex"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(bridge.ensureCalls) != 1 {
		t.Fatalf("EnsureAgent() calls = %d, want 1", len(bridge.ensureCalls))
	}
	if bridge.ensureCalls[0].ID != "u-alice" || bridge.ensureCalls[0].RuntimeKind != agent.RuntimeKindCodex {
		t.Fatalf("EnsureAgent() got %+v, want codex worker u-alice", bridge.ensureCalls[0])
	}
}

func TestHandleBotsCreateFeishuWorker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	agentSvc, _ := mustNewSeededServiceWithPath(t, nil)
	feishuSvc := feishu.NewService()
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/bots", strings.NewReader(`{"name":"alice","image":"agent-image:1","role":"worker","runtime_kind":"picoclaw_sandbox"}`))
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
	srv.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/bots", nil))
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

func TestHandleBotsCreateRejectsDuplicateWorkerNameInSameChannel(t *testing.T) {
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

	first := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/bots", strings.NewReader(`{"name":"alice","image":"agent-image:1","role":"worker","runtime_kind":"picoclaw_sandbox"}`))
	firstRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("first status = %d, want %d; body=%s", firstRec.Code, http.StatusCreated, firstRec.Body.String())
	}

	second := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/bots", strings.NewReader(`{"name":"alice","image":"agent-image:1","role":"worker","runtime_kind":"picoclaw_sandbox"}`))
	secondRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusBadRequest {
		t.Fatalf("second status = %d, want %d; body=%s", secondRec.Code, http.StatusBadRequest, secondRec.Body.String())
	}
	if !strings.Contains(secondRec.Body.String(), `bot name "alice" already exists in channel "csgclaw"`) {
		t.Fatalf("second body = %q, want duplicate name error", secondRec.Body.String())
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/bots", strings.NewReader(`{"name":"manager","role":"manager"}`))
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/bots", strings.NewReader(`{"name":"manager","role":"manager"}`))
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
	req := httptest.NewRequest(http.MethodPut, "/api/v1/channels/csgclaw/bots", strings.NewReader(`{}`))
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
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/feishu/bots/u-alice", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	bots, err := srv.botSvc.List(string(bot.ChannelCSGClaw), "", "")
	if err != nil {
		t.Fatalf("List(csgclaw) error = %v", err)
	}
	if len(bots) != 1 || bots[0].ID != "u-alice" {
		t.Fatalf("csgclaw bots = %+v, want retained u-alice", bots)
	}
	bots, err = srv.botSvc.List(string(bot.ChannelFeishu), "", "")
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
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/csgclaw/bots/u-alice", nil)
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
	bots, err := botSvc.List(string(bot.ChannelCSGClaw), "", "")
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
	svc, err := agent.NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath, agent.WithSandboxProvider(provider))
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
	if got, want := profile["api_key_preview"], "secr..."; got != want {
		t.Fatalf("agent_profile api_key_preview = %#v, want %q", got, want)
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
	if got["description"] != "new role" {
		t.Fatalf("agent = %#v, want updated description", got)
	}
	profile, ok := got["agent_profile"].(map[string]any)
	if !ok || profile["env_restart_required"] != true || profile["model_id"] != "new-model" {
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
	if strings.Join(gotArgs, " ") != "-n 80 "+picoclawsandbox.BoxGatewayLogPath {
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
		state := sandbox.StateStopped
		if startCalls > 0 {
			state = sandbox.StateRunning
		}
		return sandbox.Info{ID: "box-new", Name: "alice", State: state}, nil
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

func TestHandleAgentStartEnsuresCodexBridge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	statePath := filepath.Join(t.TempDir(), "agents.json")

	agentSvc, err := agent.NewService(
		config.ModelConfig{
			Provider: config.ProviderLLMAPI,
			BaseURL:  "http://127.0.0.1:4000",
			APIKey:   "sk-test",
			ModelID:  "model-1",
		},
		config.ServerConfig{}, "manager-image:test", statePath,
		agent.WithRuntime(fakeCompatRuntime{
			kind: agent.RuntimeKindCodex,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-" + spec.AgentName}, nil
			},
			start: func(_ context.Context, _ agentruntime.Handle) (agentruntime.State, error) {
				return agentruntime.StateRunning, nil
			},
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	created, err := agentSvc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			Name:        "alice",
			Role:        agent.RoleWorker,
			RuntimeKind: agent.RuntimeKindCodex,
			AgentProfile: agent.AgentProfile{
				ProfileComplete: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := agentSvc.Stop(context.Background(), created.ID); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	bridge := &fakeCodexBridgeController{}
	agentSvc.SetLifecycleObserver(bridge)
	srv := &Handler{svc: agentSvc}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+created.ID+"/start", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(bridge.ensureCalls) != 1 {
		t.Fatalf("EnsureAgent() calls = %d, want 1", len(bridge.ensureCalls))
	}
	if bridge.ensureCalls[0].ID != created.ID || bridge.ensureCalls[0].RuntimeKind != agent.RuntimeKindCodex {
		t.Fatalf("EnsureAgent() got %+v, want codex worker %q", bridge.ensureCalls[0], created.ID)
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

func TestHandleAgentStopStopsCodexBridge(t *testing.T) {
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	agentSvc, statePath := mustNewSeededServiceWithPath(t, []agent.Agent{
		{ID: "u-alice", Name: "alice", BoxID: "box-old", Role: agent.RoleWorker, Status: "running", CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	})
	if err := writeSeededAgents(statePath, []agent.Agent{
		{ID: "u-alice", Name: "alice", BoxID: "box-new", Role: agent.RoleWorker, Status: "running", CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}

	agent.TestOnlySetGetBoxHook(func(_ *agent.Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		return sandboxtest.NewInstance(sandbox.Info{ID: idOrName, Name: "alice", State: sandbox.StateRunning}), nil
	})
	agent.TestOnlySetStopBoxHook(func(_ *agent.Service, _ context.Context, _ sandbox.Instance, _ sandbox.StopOptions) error {
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

	bridge := &fakeCodexBridgeController{}
	agentSvc.SetLifecycleObserver(bridge)
	srv := &Handler{svc: agentSvc}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/u-alice/stop", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(bridge.stopCalls) != 1 || bridge.stopCalls[0] != "u-alice" {
		t.Fatalf("StopAgent() calls = %v, want [u-alice]", bridge.stopCalls)
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

func TestHandleAgentsCreateDoesNotProvisionIMUser(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc := mustNewService(t)
	srv := &Handler{
		svc: svc,
		im:  im.NewService(),
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
	if _, ok := srv.im.User("u-alice"); ok {
		t.Fatal("User(u-alice) ok = true, want false after agent create")
	}
	if rooms := srv.im.ListRooms(); len(rooms) != 0 {
		t.Fatalf("rooms = %+v, want no IM rooms after agent create", rooms)
	}
}

func TestHandleAgentsCreateWorkerUsesRequestedRuntimeKind(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc, err := agent.NewService(
		config.ModelConfig{
			Provider: config.ProviderLLMAPI,
			BaseURL:  "http://127.0.0.1:4000",
			APIKey:   "sk-test",
			ModelID:  "model-1",
		},
		config.ServerConfig{}, "manager-image:test", "",
		agent.WithRuntime(fakeCompatRuntime{
			kind: agent.RuntimeKindCodex,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-" + spec.AgentName}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	srv := &Handler{
		svc: svc,
		im:  im.NewService(),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"name":"alice","role":"worker","runtime_kind":"codex"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.RuntimeKind != agent.RuntimeKindCodex {
		t.Fatalf("agent runtime kind = %q, want %q", got.RuntimeKind, agent.RuntimeKindCodex)
	}
	if got.BoxID != "codex-alice" {
		t.Fatalf("agent BoxID = %q, want %q", got.BoxID, "codex-alice")
	}
}

func TestHandleAgentsCreateCodexWorkerEnsuresCodexBridge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc, err := agent.NewService(
		config.ModelConfig{
			Provider: config.ProviderLLMAPI,
			BaseURL:  "http://127.0.0.1:4000",
			APIKey:   "sk-test",
			ModelID:  "model-1",
		},
		config.ServerConfig{}, "manager-image:test", "",
		agent.WithRuntime(fakeCompatRuntime{
			kind: agent.RuntimeKindCodex,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-" + spec.AgentName}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	bridge := &fakeCodexBridgeController{}
	svc.SetLifecycleObserver(bridge)
	srv := &Handler{
		svc: svc,
		im:  im.NewService(),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"name":"alice","role":"worker","runtime_kind":"codex","agent_profile":{"profile_complete":true}}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(bridge.ensureCalls) != 1 {
		t.Fatalf("EnsureAgent() calls = %d, want 1", len(bridge.ensureCalls))
	}
	if bridge.ensureCalls[0].ID != "u-alice" || bridge.ensureCalls[0].RuntimeKind != agent.RuntimeKindCodex {
		t.Fatalf("EnsureAgent() got %+v, want codex worker u-alice", bridge.ensureCalls[0])
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
	if got.RuntimeKind != agent.RuntimeKindPicoClawSandbox {
		t.Fatalf("agent runtime kind = %q, want %q", got.RuntimeKind, agent.RuntimeKindPicoClawSandbox)
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

func TestAgentCreateRequestFromAPIIncludesFromTemplate(t *testing.T) {
	got := agentCreateRequestFromAPI(apitypes.CreateAgentRequest{
		Name:         "alice",
		RuntimeKind:  agent.RuntimeKindCodex,
		FromTemplate: "builtin/frontend-alice",
		Profile:      "codex-fast",
	})

	if got.Spec.Name != "alice" {
		t.Fatalf("Spec.Name = %q, want %q", got.Spec.Name, "alice")
	}
	if got.Spec.RuntimeKind != agent.RuntimeKindCodex {
		t.Fatalf("Spec.RuntimeKind = %q, want %q", got.Spec.RuntimeKind, agent.RuntimeKindCodex)
	}
	if got.Spec.FromTemplate != "builtin/frontend-alice" {
		t.Fatalf("Spec.FromTemplate = %q, want %q", got.Spec.FromTemplate, "builtin/frontend-alice")
	}
	if got.Spec.Profile != "codex-fast" {
		t.Fatalf("Spec.Profile = %q, want %q", got.Spec.Profile, "codex-fast")
	}
}

func TestHandleHubTemplatesListsAggregatedTemplates(t *testing.T) {
	hubSvc := mustNewLocalTemplateHubService(t, "review-bot", hub.Template{
		ID:          "review-bot",
		Name:        "review-bot",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: agent.RuntimeKindCodex,
	})
	srv := &Handler{}
	srv.SetHubService(hubSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []apitypes.HubTemplate
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("templates = %#v, want non-empty result", got)
	}
	if got[0].Source.Name == "" || got[0].Source.Kind == "" {
		t.Fatalf("template source = %+v, want populated source", got[0].Source)
	}
}

func TestHandleHubTemplateByRegistryNameReturnsTemplate(t *testing.T) {
	hubSvc := mustNewLocalTemplateHubService(t, "review-bot", hub.Template{
		ID:          "review-bot",
		Name:        "review-bot",
		Description: "code review helper",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: agent.RuntimeKindCodex,
	})
	srv := &Handler{}
	srv.SetHubService(hubSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates/local/review-bot", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got apitypes.HubTemplate
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "local/review-bot" {
		t.Fatalf("template id = %q, want %q", got.ID, "local/review-bot")
	}
	if got.Source.Name != "local" || got.Source.Kind != "local" {
		t.Fatalf("template source = %+v, want local/local", got.Source)
	}
	if len(got.Workspace.Entries) == 0 {
		t.Fatalf("workspace entries = %#v, want non-empty result", got.Workspace.Entries)
	}
	paths := make([]string, 0, len(got.Workspace.Entries))
	for _, entry := range got.Workspace.Entries {
		paths = append(paths, entry.Path)
	}
	if !slices.Contains(paths, "USER.md") || !slices.Contains(paths, "skills/custom/SKILL.md") {
		t.Fatalf("workspace paths = %#v, want USER.md and skills/custom/SKILL.md", paths)
	}
	if got, want := slices.Index(paths, "skills"), 1; got != want {
		t.Fatalf("workspace entry index for %q = %d, want %d; paths=%#v", "skills", got, want, paths)
	}
	if got, want := slices.Index(paths, "skills/custom"), 2; got != want {
		t.Fatalf("workspace entry index for %q = %d, want %d; paths=%#v", "skills/custom", got, want, paths)
	}
	if got, want := slices.Index(paths, "skills/custom/SKILL.md"), 3; got != want {
		t.Fatalf("workspace entry index for %q = %d, want %d; paths=%#v", "skills/custom/SKILL.md", got, want, paths)
	}
}

func TestHandleHubTemplateWorkspaceFileByRegistryNameReturnsContent(t *testing.T) {
	hubSvc := mustNewLocalTemplateHubService(t, "review-bot", hub.Template{
		ID:          "review-bot",
		Name:        "review-bot",
		Description: "code review helper",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: agent.RuntimeKindCodex,
	})
	srv := &Handler{}
	srv.SetHubService(hubSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates/local/review-bot/workspace/file?path=skills/custom/SKILL.md", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got apitypes.HubTemplateWorkspaceFile
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Path != "skills/custom/SKILL.md" {
		t.Fatalf("path = %q, want %q", got.Path, "skills/custom/SKILL.md")
	}
	if strings.TrimSpace(got.Content) != "# Custom" {
		t.Fatalf("content = %q, want %q", strings.TrimSpace(got.Content), "# Custom")
	}
}

func TestHandleAgentWorkspaceFileReturnsContent(t *testing.T) {
	svc := mustNewService(t)
	created, err := svc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			Name:        "alice",
			Role:        agent.RoleWorker,
			RuntimeKind: agent.RuntimeKindPicoClawSandbox,
			Image:       "worker-image:test",
			AgentProfile: agent.AgentProfile{
				ProfileComplete: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	workspaceRoot, err := agent.WorkspaceRoot(created.Name, created.RuntimeKind)
	if err != nil {
		t.Fatalf("WorkspaceRoot() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "skills", "custom"), 0o755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "skills", "custom", "SKILL.md"), []byte("# Custom\n"), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+created.ID+"/workspace/file?path=skills/custom/SKILL.md", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got apitypes.WorkspaceFile
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Path != "skills/custom/SKILL.md" {
		t.Fatalf("path = %q, want %q", got.Path, "skills/custom/SKILL.md")
	}
	if strings.TrimSpace(got.Content) != "# Custom" {
		t.Fatalf("content = %q, want %q", strings.TrimSpace(got.Content), "# Custom")
	}
}

func TestHandleHubTemplateWithoutWorkspaceOmitsEntriesAndFilePreview(t *testing.T) {
	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "review-bot", hub.Template{
		ID:          "review-bot",
		Name:        "review-bot",
		Description: "code review helper",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: agent.RuntimeKindCodex,
	})
	srv := &Handler{}
	srv.SetHubService(hubSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates/local/review-bot", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var detail apitypes.HubTemplate
	if err := json.NewDecoder(rec.Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if len(detail.Workspace.Entries) != 0 {
		t.Fatalf("workspace entries = %#v, want empty", detail.Workspace.Entries)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates/local/review-bot/workspace/file?path=USER.md", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("file status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "hub workspace is not available") {
		t.Fatalf("file response body = %q, want unavailable workspace error", rec.Body.String())
	}
}

func TestHandleHubTemplatesPublishesAgentSnapshot(t *testing.T) {
	svc := mustNewService(t)
	created, err := svc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			ID:          "u-alice",
			Name:        "alice",
			Description: "review worker",
			RuntimeKind: agent.RuntimeKindPicoClawSandbox,
			Image:       "worker-image:1",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	spec, err := svc.HubPublishSpec(created.ID)
	if err != nil {
		t.Fatalf("HubPublishSpec() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(spec.WorkspaceRef.Path, "PLAYBOOK.md"), []byte("published workspace\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(PLAYBOOK.md) error = %v", err)
	}

	registryRoot := t.TempDir()
	hubSvc, err := hub.NewService(config.HubConfig{
		DefaultRegistry:        "local",
		DefaultPublishRegistry: "local",
		Registries: []config.HubRegistryConfig{
			{Name: "local", Kind: hub.RegistryKindLocal, Path: registryRoot, Enabled: true},
		},
	}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatalf("hub.NewService() error = %v", err)
	}

	srv := &Handler{svc: svc}
	srv.SetHubService(hubSvc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/hub/templates", strings.NewReader(`{"agent_id":"u-alice","registry":"local"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got apitypes.HubTemplate
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "local/alice" {
		t.Fatalf("template id = %q, want %q", got.ID, "local/alice")
	}
	if got.Role != hub.TemplateRoleWorker {
		t.Fatalf("template role = %q, want %q", got.Role, hub.TemplateRoleWorker)
	}
	if got.Source.Name != "local" || got.Source.Kind != "local" {
		t.Fatalf("template source = %+v, want local/local", got.Source)
	}
	publishedWorkspace := filepath.Join(registryRoot, "templates", "alice", "workspace", "PLAYBOOK.md")
	if data, err := os.ReadFile(publishedWorkspace); err != nil {
		t.Fatalf("ReadFile(PLAYBOOK.md) error = %v", err)
	} else if strings.TrimSpace(string(data)) != "published workspace" {
		t.Fatalf("PLAYBOOK.md = %q, want %q", strings.TrimSpace(string(data)), "published workspace")
	}
}

func TestHandleHubTemplatesPublishesAgentSnapshotToDefaultRegistryWhenOmitted(t *testing.T) {
	svc := mustNewService(t)
	created, err := svc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			ID:          "u-alice",
			Name:        "alice",
			Description: "review worker",
			RuntimeKind: agent.RuntimeKindPicoClawSandbox,
			Image:       "worker-image:1",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	spec, err := svc.HubPublishSpec(created.ID)
	if err != nil {
		t.Fatalf("HubPublishSpec() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(spec.WorkspaceRef.Path, "PLAYBOOK.md"), []byte("published workspace\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(PLAYBOOK.md) error = %v", err)
	}

	registryRoot := t.TempDir()
	hubSvc, err := hub.NewService(config.HubConfig{
		DefaultRegistry:        "local",
		DefaultPublishRegistry: "local",
		Registries: []config.HubRegistryConfig{
			{Name: "local", Kind: hub.RegistryKindLocal, Path: registryRoot, Enabled: true},
		},
	}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatalf("hub.NewService() error = %v", err)
	}

	srv := &Handler{svc: svc}
	srv.SetHubService(hubSvc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/hub/templates", strings.NewReader(`{"agent_id":"u-alice"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got apitypes.HubTemplate
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "local/alice" {
		t.Fatalf("template id = %q, want %q", got.ID, "local/alice")
	}
	if got.Role != hub.TemplateRoleWorker {
		t.Fatalf("template role = %q, want %q", got.Role, hub.TemplateRoleWorker)
	}
	if got.Source.Name != "local" || got.Source.Kind != "local" {
		t.Fatalf("template source = %+v, want local/local", got.Source)
	}
}

func mustNewLocalTemplateHubService(t *testing.T, id string, item hub.Template) *hub.Service {
	t.Helper()

	registryRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "USER.md"), []byte("template user\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(USER.md) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "skills", "custom"), 0o755); err != nil {
		t.Fatalf("MkdirAll(skill dir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "skills", "custom", "SKILL.md"), []byte("# Custom\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	store := hub.NewLocalStore(registryRoot)
	if _, err := store.Publish(context.Background(), hub.PublishSpec{
		ID:           id,
		Name:         item.Name,
		Description:  item.Description,
		Role:         item.Role,
		RuntimeKind:  item.RuntimeKind,
		Image:        item.Image,
		WorkspaceRef: hub.WorkspaceRef{Kind: hub.WorkspaceKindDir, Path: workspaceRoot},
		UpdatedAt:    time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	svc, err := hub.NewService(config.HubConfig{
		DefaultRegistry: "local",
		Registries: []config.HubRegistryConfig{
			{Name: "local", Kind: hub.RegistryKindLocal, Path: registryRoot, Enabled: true},
		},
	}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatalf("hub.NewService() error = %v", err)
	}
	return svc
}

func mustNewLocalTemplateHubServiceWithoutWorkspace(t *testing.T, id string, item hub.Template) *hub.Service {
	t.Helper()

	registryRoot := t.TempDir()
	store := hub.NewLocalStore(registryRoot)
	if _, err := store.Publish(context.Background(), hub.PublishSpec{
		ID:          id,
		Name:        item.Name,
		Description: item.Description,
		Role:        item.Role,
		RuntimeKind: item.RuntimeKind,
		Image:       item.Image,
		UpdatedAt:   time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	svc, err := hub.NewService(config.HubConfig{
		DefaultRegistry: "local",
		Registries: []config.HubRegistryConfig{
			{Name: "local", Kind: hub.RegistryKindLocal, Path: registryRoot, Enabled: true},
		},
	}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatalf("hub.NewService() error = %v", err)
	}
	return svc
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

	bots, err := botSvc.List(string(bot.ChannelCSGClaw), string(bot.RoleWorker), "")
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

func TestHandleThreadRoutesAndMessageFiltering(t *testing.T) {
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
					Messages: []im.Message{
						{ID: "msg-1", SenderID: "u-admin", Content: "before", CreatedAt: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)},
						{ID: "msg-root", SenderID: "u-admin", Content: "root", CreatedAt: time.Date(2026, 5, 20, 9, 1, 0, 0, time.UTC)},
						{ID: "msg-2", SenderID: "u-manager", Content: "after", CreatedAt: time.Date(2026, 5, 20, 9, 2, 0, 0, time.UTC)},
					},
				},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/room-1/threads", strings.NewReader(`{"root_message_id":"msg-root"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start thread status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var started im.ThreadView
	if err := json.NewDecoder(rec.Body).Decode(&started); err != nil {
		t.Fatalf("decode start thread response: %v", err)
	}
	if started.Root.ID != "msg-root" || len(started.Context) != 3 {
		t.Fatalf("started thread = %+v, want root with context", started)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"room_id":"room-1","sender_id":"u-manager","content":"thread reply","relates_to":{"rel_type":"m.thread","event_id":"msg-root"}}`))
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("reply status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var reply im.Message
	if err := json.NewDecoder(rec.Body).Decode(&reply); err != nil {
		t.Fatalf("decode reply response: %v", err)
	}
	if reply.RelatesTo == nil || reply.RelatesTo.EventID != "msg-root" {
		t.Fatalf("reply.RelatesTo = %+v, want thread root", reply.RelatesTo)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/messages?room_id=room-1", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var timeline []im.Message
	if err := json.NewDecoder(rec.Body).Decode(&timeline); err != nil {
		t.Fatalf("decode timeline: %v", err)
	}
	if containsAPIMessageID(timeline, reply.ID) {
		t.Fatalf("timeline = %+v, want thread reply hidden by default", timeline)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/rooms", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rooms status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var rooms []im.Room
	if err := json.NewDecoder(rec.Body).Decode(&rooms); err != nil {
		t.Fatalf("decode rooms: %v", err)
	}
	if len(rooms) != 1 || containsAPIMessageID(rooms[0].Messages, reply.ID) {
		t.Fatalf("rooms = %+v, want thread reply hidden by default", rooms)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/bootstrap", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var bootstrap struct {
		Rooms []im.Room `json:"rooms"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&bootstrap); err != nil {
		t.Fatalf("decode bootstrap: %v", err)
	}
	if len(bootstrap.Rooms) != 1 || containsAPIMessageID(bootstrap.Rooms[0].Messages, reply.ID) {
		t.Fatalf("bootstrap rooms = %+v, want thread reply hidden by default", bootstrap.Rooms)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/messages?room_id=room-1&include_thread_replies=true", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list include status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var fullTimeline []im.Message
	if err := json.NewDecoder(rec.Body).Decode(&fullTimeline); err != nil {
		t.Fatalf("decode full timeline: %v", err)
	}
	if !containsAPIMessageID(fullTimeline, reply.ID) {
		t.Fatalf("full timeline = %+v, want thread reply included", fullTimeline)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/rooms/room-1/threads/msg-root", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get thread status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var gotThread im.ThreadView
	if err := json.NewDecoder(rec.Body).Decode(&gotThread); err != nil {
		t.Fatalf("decode get thread: %v", err)
	}
	if len(gotThread.Replies) != 1 || gotThread.Replies[0].ID != reply.ID {
		t.Fatalf("thread replies = %+v, want reply %s", gotThread.Replies, reply.ID)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/rooms/room-1/relations/msg-root/m.thread", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("relations status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var relations im.ThreadRelationsResponse
	if err := json.NewDecoder(rec.Body).Decode(&relations); err != nil {
		t.Fatalf("decode relations: %v", err)
	}
	if len(relations.Chunk) != 1 || relations.Chunk[0].ID != reply.ID {
		t.Fatalf("relations = %+v, want reply %s", relations, reply.ID)
	}
}

func TestHandleThreadEventsPublishCreatedAndUpdated(t *testing.T) {
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()
	srv := &Handler{
		im: im.NewServiceFromBootstrapWithBus(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Handle: "admin"},
				{ID: "u-manager", Name: "manager", Handle: "manager"},
			},
			Rooms: []im.Room{
				{
					ID:       "room-1",
					Title:    "Room One",
					Members:  []string{"u-admin", "u-manager"},
					Messages: []im.Message{{ID: "msg-root", SenderID: "u-admin", Content: "root", CreatedAt: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)}},
				},
			},
		}, bus),
		imBus: bus,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/room-1/threads", strings.NewReader(`{"root_message_id":"msg-root"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start thread status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	created := mustReceiveIMEvent(t, events)
	if created.Type != im.EventTypeThreadCreated || created.Thread == nil || created.Thread.Root.ID != "msg-root" {
		t.Fatalf("created event = %+v, want thread.created for msg-root", created)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"room_id":"room-1","sender_id":"u-manager","content":"thread reply","relates_to":{"rel_type":"m.thread","event_id":"msg-root"}}`))
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("reply status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	messageCreated := mustReceiveIMEvent(t, events)
	if messageCreated.Type != im.EventTypeMessageCreated || messageCreated.Message == nil || messageCreated.Message.RelatesTo == nil {
		t.Fatalf("message event = %+v, want threaded message.created", messageCreated)
	}
	threadUpdated := mustReceiveIMEvent(t, events)
	if threadUpdated.Type != im.EventTypeThreadUpdated || threadUpdated.Thread == nil || threadUpdated.Thread.Summary.ReplyCount != 1 {
		t.Fatalf("thread updated event = %+v, want one reply", threadUpdated)
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
	feishuSvc := feishu.NewServiceWithSendMessage(
		map[string]feishu.AppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"}},
		func(_ context.Context, _ feishu.AppConfig, req feishu.SendMessageRequest) (feishu.SendMessageResponse, error) {
			if req.ChatID != "oc_alpha" || req.Content != "hello" {
				t.Fatalf("send request = %+v, want chat/content", req)
			}
			return feishu.SendMessageResponse{MessageID: "om_1", SenderOpenID: "ou_manager"}, nil
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
	feishuSvc := feishu.NewServiceWithCreateChatAndListRoomMessages(
		map[string]feishu.AppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
		func(_ context.Context, _ feishu.AppConfig, req feishu.CreateChatRequest) (feishu.CreateChatResponse, error) {
			return feishu.CreateChatResponse{ChatID: "oc_alpha", Name: req.Title}, nil
		},
		func(_ context.Context, _ feishu.AppConfig, roomID string) ([]im.Message, error) {
			if roomID != "oc_alpha" {
				t.Fatalf("room_id = %q, want oc_alpha", roomID)
			}
			return []im.Message{{ID: "om_1", SenderID: "ou_manager", Content: "hello", CreatedAt: time.Unix(1, 0).UTC()}}, nil
		},
	)
	feishuSvc.SetBotOpenIDResolver(testFeishuBotInfoResolver(t, map[string]string{"cli_manager": "ou_manager"}))
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
	if len(got) != 1 || got[0].ID != "om_1" || got[0].SenderID != "u-manager" {
		t.Fatalf("messages = %+v, want listed feishu messages with bot ids", got)
	}
}

func TestHandleFeishuEventsStreamsMessageBusEvents(t *testing.T) {
	feishuSvc := feishu.NewService()
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
	feishuSvc.MessageBus().Publish(feishu.MessageEvent{
		Type:   feishu.MessageEventTypeMessageCreated,
		RoomID: "oc_ignored",
		Message: &im.Message{
			ID:       "om_ignored",
			SenderID: "ou_manager",
			Content:  "hello @worker",
			Mentions: []im.Mention{{ID: "u-worker"}},
		},
	})
	feishuSvc.MessageBus().Publish(feishu.MessageEvent{
		Type:         feishu.MessageEventTypeMessageCreated,
		RoomID:       "oc_alpha",
		SenderBotID:  "u-worker",
		MentionBotID: "u-manager",
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
	if !strings.Contains(body, `"sender_bot_id":"u-worker"`) || !strings.Contains(body, `"mention_bot_id":"u-manager"`) {
		t.Fatalf("body = %q, want bot id bridge metadata", body)
	}
	if strings.Contains(body, "om_ignored") || strings.Contains(body, "oc_ignored") {
		t.Fatalf("body = %q, want only u-manager events", body)
	}
	if !strings.Contains(body, `"id":"om_1"`) {
		t.Fatalf("body = %q, want message id", body)
	}
}

func TestHandleFeishuEventsSendsHeartbeat(t *testing.T) {
	oldInterval := sseHeartbeatInterval
	sseHeartbeatInterval = 5 * time.Millisecond
	t.Cleanup(func() {
		sseHeartbeatInterval = oldInterval
	})

	feishuSvc := feishu.NewService()
	srv := &Handler{feishu: feishuSvc, serverAccessToken: "secret"}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/bots/u-manager/events", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.Routes().ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	if body := rec.Body.String(); !strings.Contains(body, ": ping\n\n") {
		t.Fatalf("body = %q, want heartbeat ping", body)
	}
}

func TestHandleFeishuEventsRequiresAuthorization(t *testing.T) {
	srv := &Handler{
		feishu:            feishu.NewService(),
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(ctx)
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

func TestHandleRoomsPostUsesCsgclawChannelAdapter(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", Handle: "admin"},
				{ID: "u-alice", Name: "Alice", Handle: "alice"},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"title":"Launch","creator_id":" u-admin ","member_ids":[" u-alice "]}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got im.Room
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !containsMember(got.Members, "u-admin") || !containsMember(got.Members, "u-alice") {
		t.Fatalf("members = %+v, want trimmed bot IDs", got.Members)
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
		feishu: feishu.NewService(),
	}
	if _, err := srv.feishu.CreateUser(feishu.CreateUserRequest{ID: "ou-alice", Name: "Alice"}); err != nil {
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
		feishu: feishu.NewServiceWithDeleteChat(
			map[string]feishu.AppConfig{"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
			func(_ context.Context, _ feishu.AppConfig, roomID string) error {
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

func TestHandleBotCompatibilityRoutesRequireAuthorization(t *testing.T) {
	srv := &Handler{
		im:                im.NewService(),
		botBridge:         im.NewBotBridge("secret"),
		serverAccessToken: "secret",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/bots/u-manager/messages/send", strings.NewReader(`{"room_id":"room-1","text":"hello"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleBotCompatibilityRoutesRequireAuthorizationWhenServerAccessTokenEmpty(t *testing.T) {
	srv := &Handler{
		botBridge: im.NewBotBridge("secret"),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/bots/u-manager/messages/send", strings.NewReader(`{"room_id":"room-1","text":"hello"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleBotCompatibilityRoutesSkipAuthorizationWhenNoAuth(t *testing.T) {
	srv := &Handler{
		botBridge:    im.NewBotBridge("secret"),
		serverNoAuth: true,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/bots/u-manager/messages/send", strings.NewReader(`{"room_id":"room-1","text":"hello"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("status = %d, want non-unauthorized when no_auth is true", rec.Code)
	}
}

func TestHandleBotSendMessageRequiresIMService(t *testing.T) {
	srv := &Handler{
		botBridge:    im.NewBotBridge(""),
		serverNoAuth: true,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/bots/u-manager/messages/send", strings.NewReader(`{"room_id":"room-1","text":"hello"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func TestHandleBotSendMessageDoesNotInferRecentThreadScope(t *testing.T) {
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
					{ID: "msg-root", SenderID: "u-manager", Content: "How can I help?", CreatedAt: now},
				},
			},
		},
	})
	if _, _, err := imSvc.StartThread(im.StartThreadRequest{RoomID: "room-1", RootMessageID: "msg-root"}); err != nil {
		t.Fatalf("StartThread() error = %v", err)
	}
	inbound, err := imSvc.CreateMessage(im.CreateMessageRequest{
		RoomID:   "room-1",
		SenderID: "u-admin",
		Content:  "thread question",
		RelatesTo: &im.MessageRelation{
			RelType: im.RelationTypeThread,
			EventID: "msg-root",
		},
	})
	if err != nil {
		t.Fatalf("CreateMessage(thread question) error = %v", err)
	}
	room, ok := imSvc.Room("room-1")
	if !ok {
		t.Fatal("Room(room-1) = false, want room")
	}
	sender, ok := imSvc.User("u-admin")
	if !ok {
		t.Fatal("User(u-admin) = false, want user")
	}
	bridge := im.NewBotBridge("")
	events, cancel := bridge.Subscribe("u-manager")
	defer cancel()
	bridge.PublishMessageEvent(room, sender, inbound)
	select {
	case evt := <-events:
		if evt.ThreadRootID != "msg-root" {
			t.Fatalf("bot event ThreadRootID = %q, want msg-root", evt.ThreadRootID)
		}
		bridge.Ack("u-manager", evt.MessageID)
	case <-time.After(time.Second):
		t.Fatal("PublishMessageEvent() timed out waiting for threaded event")
	}

	srv := &Handler{im: imSvc, botBridge: bridge, serverNoAuth: true}
	req := httptest.NewRequest(http.MethodPost, "/api/bots/u-manager/messages/send", strings.NewReader(`{"room_id":"room-1","text":"thread answer"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var sent struct {
		MessageID string `json:"message_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&sent); err != nil {
		t.Fatalf("decode send response: %v", err)
	}
	messages, err := imSvc.ListMessagesWithOptions("room-1", im.ListMessagesOptions{IncludeThreadReplies: true})
	if err != nil {
		t.Fatalf("ListMessagesWithOptions() error = %v", err)
	}
	var reply im.Message
	for _, message := range messages {
		if message.ID == sent.MessageID {
			reply = message
			break
		}
	}
	if reply.ID == "" {
		t.Fatalf("sent message %q not found in room messages", sent.MessageID)
	}
	if reply.RelatesTo != nil {
		t.Fatalf("reply.RelatesTo = %+v, want nil when bot send omits explicit thread/topic", reply.RelatesTo)
	}
}

func TestHandleBotSendMessageAcceptsPicoClawThreadContext(t *testing.T) {
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
					{ID: "msg-root", SenderID: "u-manager", Content: "How can I help?", CreatedAt: now},
				},
			},
		},
	})
	if _, _, err := imSvc.StartThread(im.StartThreadRequest{RoomID: "room-1", RootMessageID: "msg-root"}); err != nil {
		t.Fatalf("StartThread() error = %v", err)
	}

	srv := &Handler{im: imSvc, botBridge: im.NewBotBridge(""), serverNoAuth: true}
	req := httptest.NewRequest(http.MethodPost, "/api/bots/u-manager/messages/send", strings.NewReader(`{
		"chat_id": "room-1",
		"content": "direct PicoClaw thread answer",
		"context": {
			"channel": "csgclaw",
			"chat_id": "room-1",
			"chat_type": "direct",
			"topic_id": "msg-root"
		}
	}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var sent struct {
		MessageID string `json:"message_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&sent); err != nil {
		t.Fatalf("decode send response: %v", err)
	}
	messages, err := imSvc.ListMessagesWithOptions("room-1", im.ListMessagesOptions{IncludeThreadReplies: true})
	if err != nil {
		t.Fatalf("ListMessagesWithOptions() error = %v", err)
	}
	var reply im.Message
	for _, message := range messages {
		if message.ID == sent.MessageID {
			reply = message
			break
		}
	}
	if reply.ID == "" {
		t.Fatalf("sent message %q not found in room messages", sent.MessageID)
	}
	if reply.Content != "direct PicoClaw thread answer" {
		t.Fatalf("reply.Content = %q, want direct PicoClaw thread answer", reply.Content)
	}
	if reply.RelatesTo == nil || reply.RelatesTo.RelType != im.RelationTypeThread || reply.RelatesTo.EventID != "msg-root" {
		t.Fatalf("reply.RelatesTo = %+v, want m.thread -> msg-root", reply.RelatesTo)
	}
}

func TestPublishBotEventQueuesUntilBotSubscribes(t *testing.T) {
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
	bridge := im.NewBotBridge("")
	srv := &Handler{im: imSvc, botBridge: bridge}

	sender, ok := imSvc.User("u-admin")
	if !ok {
		t.Fatal("missing sender")
	}
	room, ok := imSvc.Room("room-1")
	if !ok || len(room.Messages) != 1 {
		t.Fatalf("room = %+v, want one message", room)
	}

	srv.PublishBotEvent(im.Event{
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
		t.Fatal("Subscribe() timed out waiting for queued bot event")
	}
}

func TestPublishBotEventReensuresRunningWorkerLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	started := make(chan string, 1)
	recreated := make(chan string, 1)
	restoreDefault := agent.TestOnlySetDefaultServiceOption(func(s *agent.Service) error {
		return agent.WithRuntime(fakeCompatRuntime{
			info: func(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: "box-stale", State: agentruntime.StateRunning}, nil
			},
			start: func(_ context.Context, h agentruntime.Handle) (agentruntime.State, error) {
				started <- h.HandleID
				return agentruntime.StateRunning, nil
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				recreated <- spec.AgentName
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "box-" + spec.AgentName}, nil
			},
		})(s)
	})
	t.Cleanup(restoreDefault)

	statePath := filepath.Join(t.TempDir(), "agents.json")
	if err := writeSeededAgents(statePath, []agent.Agent{
		{
			ID:              "u-worker",
			Name:            "worker",
			Role:            agent.RoleWorker,
			RuntimeID:       "rt-u-worker",
			BoxID:           "box-stale",
			Status:          string(agentruntime.StateRunning),
			AgentProfile:    agent.AgentProfile{Name: "worker", Provider: agent.ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true},
			ProfileComplete: true,
			CreatedAt:       time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}
	svc, err := agent.NewService(
		config.ModelConfig{ModelID: "gpt-5.5"},
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "token"}, "manager-image:test", statePath,
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

	srv := &Handler{svc: svc, im: imSvc, botBridge: im.NewBotBridge("")}
	srv.PublishBotEvent(im.Event{
		Type:    im.EventTypeMessageCreated,
		RoomID:  "room-1",
		Sender:  &sender,
		Message: &room.Messages[0],
	})

	select {
	case handleID := <-started:
		if handleID != "box-stale" {
			t.Fatalf("started running worker handle %q, want %q", handleID, "box-stale")
		}
	case name := <-recreated:
		t.Fatalf("recreated running worker %q, want start-based lifecycle recovery", name)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for running worker lifecycle recovery")
	}
}

func TestPublishBotEventStartsStoppedWorker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	started := make(chan string, 1)
	restoreDefault := agent.TestOnlySetDefaultServiceOption(func(s *agent.Service) error {
		return agent.WithRuntime(fakeCompatRuntime{
			info: func(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: "box-stale", State: agentruntime.StateStopped}, nil
			},
			start: func(_ context.Context, h agentruntime.Handle) (agentruntime.State, error) {
				started <- h.HandleID
				return agentruntime.StateRunning, nil
			},
		})(s)
	})
	t.Cleanup(restoreDefault)

	statePath := filepath.Join(t.TempDir(), "agents.json")
	if err := writeSeededAgents(statePath, []agent.Agent{
		{
			ID:              "u-worker",
			Name:            "worker",
			Role:            agent.RoleWorker,
			RuntimeID:       "rt-u-worker",
			BoxID:           "box-stale",
			Status:          string(agentruntime.StateStopped),
			AgentProfile:    agent.AgentProfile{Name: "worker", Provider: agent.ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true},
			ProfileComplete: true,
			CreatedAt:       time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}
	svc, err := agent.NewService(
		config.ModelConfig{ModelID: "gpt-5.5"},
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "token"}, "manager-image:test", statePath,
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

	srv := &Handler{svc: svc, im: imSvc, botBridge: im.NewBotBridge("")}
	srv.PublishBotEvent(im.Event{
		Type:    im.EventTypeMessageCreated,
		RoomID:  "room-1",
		Sender:  &sender,
		Message: &room.Messages[0],
	})

	select {
	case handleID := <-started:
		if handleID != "box-stale" {
			t.Fatalf("started handle = %q, want %q", handleID, "box-stale")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for missed worker start")
	}
}

func TestHandleBotEventsRequeuesWhenSSEWriteFails(t *testing.T) {
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
	bridge := im.NewBotBridge("")
	room, ok := imSvc.Room("room-1")
	if !ok {
		t.Fatal("Room(room-1) = false, want room")
	}
	sender, ok := imSvc.User("u-admin")
	if !ok {
		t.Fatal("User(u-admin) = false, want user")
	}
	bridge.PublishMessageEvent(room, sender, room.Messages[0])

	srv := &Handler{im: imSvc, botBridge: bridge}
	req := httptest.NewRequest(http.MethodGet, "/api/bots/u-manager/events", nil)
	srv.handleBotEvents(&failingBotEventWriter{header: make(http.Header)}, req, "u-manager")

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

type failingBotEventWriter struct {
	header http.Header
}

func (w *failingBotEventWriter) Header() http.Header {
	return w.header
}

func (w *failingBotEventWriter) Write(data []byte) (int, error) {
	text := string(data)
	if strings.HasPrefix(text, "id: ") || strings.HasPrefix(text, "event: message") {
		return 0, errors.New("broken stream")
	}
	return len(data), nil
}

func (w *failingBotEventWriter) WriteHeader(int) {}

func (w *failingBotEventWriter) Flush() {}

func TestReplayRecentBotMessagesReplaysUnansweredHumanMessage(t *testing.T) {
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
	bridge := im.NewBotBridge("")
	events, cancel := bridge.Subscribe("u-manager")
	defer cancel()

	srv := &Handler{im: imSvc, botBridge: bridge}
	srv.replayRecentBotMessages("u-manager", "")

	select {
	case evt := <-events:
		if evt.MessageID != "msg-missed" || evt.Text != "please reply" {
			t.Fatalf("replayed event = %+v, want msg-missed please reply", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("replayRecentBotMessages() timed out waiting for event")
	}
}

func TestReplayRecentBotMessagesSkipsAnsweredMessage(t *testing.T) {
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
	bridge := im.NewBotBridge("")
	events, cancel := bridge.Subscribe("u-manager")
	defer cancel()

	srv := &Handler{im: imSvc, botBridge: bridge}
	srv.replayRecentBotMessages("u-manager", "")

	select {
	case evt := <-events:
		t.Fatalf("replayed event = %+v, want no replay for answered message", evt)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestReplayRecentBotMessagesDoesNotDuplicateDeliveredMessage(t *testing.T) {
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
	bridge := im.NewBotBridge("")
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

	srv := &Handler{im: imSvc, botBridge: bridge}
	srv.replayRecentBotMessages("u-manager", "")

	select {
	case evt := <-events:
		t.Fatalf("replayed event = %+v, want no duplicate for delivered message", evt)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestReplayRecentBotMessagesHonorsLastEventID(t *testing.T) {
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
	bridge := im.NewBotBridge("")
	events, cancel := bridge.Subscribe("u-manager")
	defer cancel()

	srv := &Handler{im: imSvc, botBridge: bridge}
	srv.replayRecentBotMessages("u-manager", "msg-seen")

	select {
	case evt := <-events:
		if evt.MessageID != "msg-new" || evt.Text != "new after reconnect" {
			t.Fatalf("replayed event = %+v, want msg-new new after reconnect", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("replayRecentBotMessages() timed out waiting for event")
	}

	select {
	case evt := <-events:
		t.Fatalf("extra replayed event = %+v, want only messages after Last-Event-ID", evt)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHandleBotLLMModelsReturnsBridgeCatalog(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	agents := []agent.Agent{
		{
			ID:          agent.ManagerUserID,
			Name:        agent.ManagerName,
			Role:        agent.RoleManager,
			RuntimeKind: agent.RuntimeKindCodex,
			Profile:     config.DefaultLLMProfile,
			AgentProfile: agent.AgentProfile{
				Provider:        agent.ProviderAPI,
				BaseURL:         "http://127.0.0.1:4000",
				APIKey:          "sk-test",
				ModelID:         "gpt-5.4",
				ReasoningEffort: agent.DefaultReasoningEffort,
				ProfileComplete: true,
			},
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
	}), config.ServerConfig{}, "manager-image:test", statePath,
		agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}),
	)
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
		botBridge:         im.NewBotBridge("secret"),
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

func TestHandleBotLLMModelsLegacyRouteReturnsBridgeCatalog(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	agents := []agent.Agent{
		{
			ID:          agent.ManagerUserID,
			Name:        agent.ManagerName,
			Role:        agent.RoleManager,
			RuntimeKind: agent.RuntimeKindCodex,
			Profile:     config.DefaultLLMProfile,
			AgentProfile: agent.AgentProfile{
				Provider:        agent.ProviderAPI,
				BaseURL:         "http://127.0.0.1:4000",
				APIKey:          "sk-test",
				ModelID:         "gpt-5.4",
				ReasoningEffort: agent.DefaultReasoningEffort,
				ProfileComplete: true,
			},
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
	}), config.ServerConfig{}, "manager-image:test", statePath,
		agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}),
	)
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
		botBridge:         im.NewBotBridge("secret"),
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

	hubSvc, err := hub.NewService(config.HubConfig{}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatalf("hub.NewService() error = %v", err)
	}

	svc, err := agent.NewService(config.ModelConfig{
		Provider: config.ProviderLLMAPI,
		BaseURL:  "http://127.0.0.1:4000",
		APIKey:   "sk-test",
		ModelID:  "model-1",
	}, config.ServerConfig{}, "manager-image:test", "",
		agent.WithHubService(hubSvc),
		agent.WithBootstrapDefaultTemplates(config.BootstrapConfig{
			DefaultManagerTemplate: config.DefaultBootstrapManagerTemplate,
			DefaultWorkerTemplate:  config.DefaultBootstrapWorkerTemplate,
		}),
	)
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

	hubSvc, err := hub.NewService(config.HubConfig{}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatalf("hub.NewService() error = %v", err)
	}

	svc, err := agent.NewService(config.ModelConfig{
		Provider: config.ProviderLLMAPI,
		BaseURL:  "http://127.0.0.1:4000",
		APIKey:   "sk-test",
		ModelID:  "model-1",
	}, config.ServerConfig{}, "manager-image:test", statePath,
		agent.WithHubService(hubSvc),
		agent.WithBootstrapDefaultTemplates(config.BootstrapConfig{
			DefaultManagerTemplate: config.DefaultBootstrapManagerTemplate,
			DefaultWorkerTemplate:  config.DefaultBootstrapWorkerTemplate,
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc, statePath
}

func writeSeededAgents(statePath string, agents []agent.Agent) error {
	agents = normalizeSeededAgents(agents)
	data, err := json.Marshal(map[string]any{
		"agents": agents,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(statePath, append(data, '\n'), 0o600)
}

func normalizeSeededAgents(agents []agent.Agent) []agent.Agent {
	out := make([]agent.Agent, len(agents))
	for i, item := range agents {
		out[i] = item
		if strings.TrimSpace(out[i].RuntimeKind) != "" {
			continue
		}
		switch strings.TrimSpace(out[i].Role) {
		case agent.RoleManager, agent.RoleWorker:
			out[i].RuntimeKind = agent.RuntimeKindPicoClawSandbox
			if strings.TrimSpace(out[i].Image) == "" {
				out[i].Image = "manager-image:test"
			}
		case agent.RoleAgent:
			out[i].RuntimeKind = agent.RuntimeKindCodex
		}
	}
	return out
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

func containsAPIMessageID(messages []im.Message, want string) bool {
	for _, message := range messages {
		if message.ID == want {
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
