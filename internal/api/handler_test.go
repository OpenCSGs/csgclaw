package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
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
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/llm"
	"csgclaw/internal/participant"
	agentruntime "csgclaw/internal/runtime"
	codexruntime "csgclaw/internal/runtime/codex"
	"csgclaw/internal/runtime/openclawsandbox"
	"csgclaw/internal/runtime/picoclawsandbox"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/sandboxtest"
	skillsystem "csgclaw/internal/skill/system"
	hub "csgclaw/internal/template"
)

type fakeCompatRuntime struct {
	kind    string
	schemas []agentruntime.RuntimeOptionSchema
	new     func(context.Context, agentruntime.Spec) (agentruntime.Handle, error)
	start   func(context.Context, agentruntime.Handle) (agentruntime.State, error)
	stop    func(context.Context, agentruntime.Handle) (agentruntime.State, error)
	del     func(context.Context, agentruntime.Handle) error
	info    func(context.Context, agentruntime.Handle) (agentruntime.Info, error)
}

func init() {
	codexPath := filepath.Join(os.TempDir(), "csgclaw-test-codex")
	_ = os.WriteFile(codexPath, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	_ = os.Setenv("CSGCLAW_CODEX_PATH", codexPath)
	_ = agent.TestOnlySetDefaultServiceOption(func(s *agent.Service) error {
		if err := runtimewiring.WithPicoClawSandboxRuntime(nil)(s); err != nil {
			return err
		}
		if err := runtimewiring.WithOpenClawSandboxRuntime(nil)(s); err != nil {
			return err
		}
		return agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex})(s)
	})
	_ = codexruntime.TestOnlySetResponsesAPIProbe(func(context.Context, string, string, string, map[string]string) error {
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

func (f fakeCompatRuntime) Layout(agentHome string) agentruntime.Layout {
	switch f.Kind() {
	case agent.RuntimeKindPicoClawSandbox:
		workspace := filepath.Join(picoclawsandbox.Root(agentHome), picoclawsandbox.HostWorkspaceDir)
		return agentruntime.Layout{
			WorkspaceRoot: workspace,
			SkillsRoot:    filepath.Join(workspace, "skills"),
			HostLogPaths:  []string{picoclawsandbox.HostGatewayLogPath(agentHome)},
		}
	case agent.RuntimeKindOpenClawSandbox:
		workspace := filepath.Join(openclawsandbox.Root(agentHome), openclawsandbox.HostWorkspaceDir)
		return agentruntime.Layout{
			WorkspaceRoot: workspace,
			SkillsRoot:    filepath.Join(workspace, "skills"),
			HostLogPaths:  []string{openclawsandbox.HostGatewayLogPath(agentHome)},
		}
	case agent.RuntimeKindCodex:
		return agentruntime.Layout{
			WorkspaceRoot: filepath.Join(agentHome, ".codex", "workspace"),
			SkillsRoot:    filepath.Join(agentHome, ".codex", "home", "skills"),
			HostLogPaths:  []string{filepath.Join(agentHome, ".codex", "home", "stderr.log")},
		}
	default:
		return agentruntime.Layout{}
	}
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

func (f fakeCompatRuntime) RuntimeOptionsSchema() []agentruntime.RuntimeOptionSchema {
	return append([]agentruntime.RuntimeOptionSchema(nil), f.schemas...)
}

type fakeConversationRuntime struct {
	fakeCompatRuntime
	newConversation func(context.Context, agentruntime.Handle, agentruntime.ConversationStartRequest) (agentruntime.ConversationStartAction, error)
}

func (f fakeConversationRuntime) NewConversation(ctx context.Context, handle agentruntime.Handle, req agentruntime.ConversationStartRequest) (agentruntime.ConversationStartAction, error) {
	if f.newConversation != nil {
		return f.newConversation(ctx, handle, req)
	}
	return agentruntime.ConversationStartAction{
		Mode:         agentruntime.ConversationStartActionInternal,
		BotEventText: "",
		AckText:      "",
	}, nil
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

func TestHandleFeishuUsersCreateAndList(t *testing.T) {
	srv := &Handler{feishu: feishu.NewService()}

	createReq := strings.NewReader(`{"id":"fsu-alice","name":"Alice","role":"worker"}`)
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
	if len(got) != 1 || got[0].ID != "fsu-alice" || got[0].Name != "Alice" {
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

func TestHandleLocalDirectoryPickerReturnsSelectedPath(t *testing.T) {
	srv := &Handler{
		localDirectoryPicker: func(context.Context) (string, error) {
			return "/tmp/project", nil
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/local/directory-picker", strings.NewReader(`{}`))
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got directoryPickerResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Path != "/tmp/project" {
		t.Fatalf("path = %q, want %q", got.Path, "/tmp/project")
	}
}

func TestHandleLocalDirectoryPickerCancelReturnsNoContent(t *testing.T) {
	srv := &Handler{
		localDirectoryPicker: func(context.Context) (string, error) {
			return "", errDirectorySelectionCanceled
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/local/directory-picker", strings.NewReader(`{}`))
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestHandleLocalDirectoryPickerUnsupportedReturnsNotImplemented(t *testing.T) {
	srv := &Handler{
		localDirectoryPicker: func(context.Context) (string, error) {
			return "", errDirectoryPickerUnsupported
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/local/directory-picker", strings.NewReader(`{}`))
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotImplemented, rec.Body.String())
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
			}, nil, nil)

			if got.ShowUpgrade != tt.showUpgrade {
				t.Fatalf("ShowUpgrade = %t, want %t", got.ShowUpgrade, tt.showUpgrade)
			}
		})
	}
}

func TestBootstrapConfigViewIncludesManagerCodexRuntimeReadiness(t *testing.T) {
	origLocateCodexCLI := locateCodexCLI
	locateCodexCLI = func() (string, error) { return "/usr/local/bin/codex", nil }
	t.Cleanup(func() {
		locateCodexCLI = origLocateCodexCLI
	})

	got := bootstrapConfigView(context.Background(), config.Config{}, nil, nil)

	if got.ManagerRuntime.Name != agent.RuntimeNameCodex {
		t.Fatalf("ManagerRuntime.Name = %q, want %q", got.ManagerRuntime.Name, agent.RuntimeNameCodex)
	}
	if got.ManagerRuntime.Label != "Codex CLI" || got.ManagerRuntime.SandboxEnabled {
		t.Fatalf("ManagerRuntime label/sandbox = %q/%t, want Codex CLI/false", got.ManagerRuntime.Label, got.ManagerRuntime.SandboxEnabled)
	}
	if !got.ManagerRuntime.Installed {
		t.Fatalf("ManagerRuntime.Installed = false, want true: %+v", got.ManagerRuntime)
	}
	if got.ManagerRuntime.OS == "" || got.ManagerRuntime.DocsURL == "" {
		t.Fatalf("ManagerRuntime OS/docs_url missing: %+v", got.ManagerRuntime)
	}
}

func TestBootstrapConfigViewReportsMissingManagerCodexCLI(t *testing.T) {
	origLocateCodexCLI := locateCodexCLI
	locateCodexCLI = func() (string, error) { return "", errors.New("codex missing") }
	t.Cleanup(func() {
		locateCodexCLI = origLocateCodexCLI
	})

	got := bootstrapConfigView(context.Background(), config.Config{}, nil, nil)

	if got.ManagerRuntime.Installed {
		t.Fatalf("ManagerRuntime.Installed = true, want false: %+v", got.ManagerRuntime)
	}
	if !strings.Contains(strings.ToLower(got.ManagerRuntime.Message), "codex") {
		t.Fatalf("ManagerRuntime.Message = %q, want codex warning", got.ManagerRuntime.Message)
	}
	if got.ManagerRuntime.InstallGuidance == "" || got.ManagerRuntime.DocsURL == "" || got.ManagerRuntime.OS == "" {
		t.Fatalf("ManagerRuntime missing install metadata: %+v", got.ManagerRuntime)
	}
}

func TestHandlerBootstrapConfigIncludesRuntimeOptionSchemas(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "agents.json")
	svc, err := agent.NewService(
		config.ModelConfig{},
		config.ServerConfig{},
		"manager-image:test",
		statePath,
		agent.WithRuntime(fakeCompatRuntime{
			kind: agent.RuntimeKindCodex,
			schemas: []agentruntime.RuntimeOptionSchema{
				{
					Key:     "local_workspace_dir",
					Path:    "local_workspace_dir",
					Label:   "Local Workspace Dir",
					LabelZh: "本地工作目录",
					LabelEn: "Local Workspace Dir",
					Type:    "directory",
				},
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	srv := &Handler{svc: svc}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/bootstrap", nil)

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got bootstrapConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.RuntimeOptionSchemas[agent.RuntimeKindCodex]) != 1 {
		t.Fatalf("codex runtime option schemas = %#v, want one schema", got.RuntimeOptionSchemas)
	}
	if got.RuntimeOptionSchemas[agent.RuntimeKindCodex][0].Path != "local_workspace_dir" {
		t.Fatalf("schema path = %q, want local_workspace_dir", got.RuntimeOptionSchemas[agent.RuntimeKindCodex][0].Path)
	}
}

func TestBootstrapConfigIncludesBuiltinOpenClawRuntimeDefaultImage(t *testing.T) {
	hubSvc, err := hub.NewService(config.HubConfig{}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatalf("hub.NewService() error = %v", err)
	}

	got := bootstrapConfigView(context.Background(), config.Config{}, hubSvc, nil)

	openclawImage := got.RuntimeDefaultImages[agent.RuntimeNameOpenClaw]
	if openclawImage == "" {
		t.Fatalf("RuntimeDefaultImages[%q] = empty; defaults=%#v", agent.RuntimeNameOpenClaw, got.RuntimeDefaultImages)
	}
	if !strings.Contains(openclawImage, "/opencsghq/openclaw:") {
		t.Fatalf("RuntimeDefaultImages[%q] = %q, want builtin OpenClaw image", agent.RuntimeNameOpenClaw, openclawImage)
	}
}

func TestHandleAgentIncludesRuntimeOptionSchemas(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "agents.json")
	if err := writeSeededAgents(statePath, []agent.Agent{
		{
			ID:             "u-codex",
			Name:           "codex-worker",
			RuntimeID:      "rt-u-codex",
			RuntimeKind:    agent.RuntimeKindCodex,
			RuntimeOptions: map[string]any{"local_workspace_dir": "/tmp/project"},
			Role:           agent.RoleWorker,
			Status:         string(agentruntime.StateRunning),
			CreatedAt:      time.Date(2026, 6, 13, 8, 0, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}
	svc, err := agent.NewService(
		config.ModelConfig{},
		config.ServerConfig{},
		"manager-image:test",
		statePath,
		agent.WithRuntime(fakeCompatRuntime{
			kind: agent.RuntimeKindCodex,
			schemas: []agentruntime.RuntimeOptionSchema{
				{
					Key:           "local_workspace_dir",
					Path:          "local_workspace_dir",
					Label:         "Local Workspace Dir",
					LabelZh:       "本地工作目录",
					LabelEn:       "Local Workspace Dir",
					Description:   "Leave empty to use the default agent workspace.",
					DescriptionZh: "留空时使用默认 Agent 工作目录。",
					DescriptionEn: "Leave empty to use the default agent workspace.",
					Type:          "directory",
					Picker:        "optional",
				},
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	srv := &Handler{svc: svc}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-codex", nil)

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got agentResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.RuntimeOptions["local_workspace_dir"] != "/tmp/project" {
		t.Fatalf("runtime options = %#v, want local_workspace_dir", got.RuntimeOptions)
	}
	if len(got.RuntimeOptionSchemas) != 1 {
		t.Fatalf("runtime option schemas = %#v, want one schema", got.RuntimeOptionSchemas)
	}
	if got.RuntimeOptionSchemas[0].Type != "directory" {
		t.Fatalf("schema type = %q, want directory", got.RuntimeOptionSchemas[0].Type)
	}
	if got.RuntimeOptionSchemas[0].LabelZh != "本地工作目录" {
		t.Fatalf("schema zh label = %q, want 本地工作目录", got.RuntimeOptionSchemas[0].LabelZh)
	}
}

func TestHandleFeishuRoomsMembers(t *testing.T) {
	feishuSvc := feishu.NewServiceWithCreateChatAndAddMembers(
		map[string]feishu.AppConfig{
			"manager":   {AppID: "manager-app-id", AppSecret: "app-secret", AdminOpenID: "ou_admin"},
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
	if members[0].ID != "manager" || members[1].ID != "fsu-alice" {
		t.Fatalf("members = %+v, want bot ids", members)
	}
}

func TestHandleRoomsMembersListsCsgclawMembers(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "Admin", Role: "admin"},
			{ID: "u-alice", Name: "Alice", Role: "worker"},
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
	if len(members) != 2 || members[0].ID != "user-admin" || members[1].ID != "user-alice" {
		t.Fatalf("members = %+v, want room members", members)
	}
}

func TestHandleRoomsMembersAddsCsgclawMember(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "Admin", Role: "admin"},
			{ID: "u-alice", Name: "Alice", Role: "worker"},
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
	if len(room.Members) != 2 || room.Members[1] != "user-alice" {
		t.Fatalf("members = %+v, want user-admin and user-alice", room.Members)
	}
}

func TestHandleRoomsMembersDeletesCsgclawMember(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "Admin", Role: "admin"},
			{ID: "u-alice", Name: "Alice", Role: "worker"},
		},
		Rooms: []im.Room{
			{ID: "room-1", Title: "Ops", Members: []string{"u-admin", "u-alice"}},
		},
	})
	srv := &Handler{im: imSvc}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/rooms/room-1/members/u-alice", strings.NewReader(`{"inviter_id":"u-admin"}`))
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var room im.Room
	if err := json.NewDecoder(rec.Body).Decode(&room); err != nil {
		t.Fatalf("decode room: %v", err)
	}
	if len(room.Members) != 1 || room.Members[0] != "user-admin" {
		t.Fatalf("members = %+v, want only admin", room.Members)
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
	if got[0].ID != "agent-manager" || got[1].ID != "agent-alice" || got[2].ID != "agent-1" {
		t.Fatalf("agents = %+v, want manager/worker/agent in CreatedAt order", got)
	}
}

func TestHandleAgentDeleteRemovesBoundParticipants(t *testing.T) {
	svc := mustNewSeededService(t, []agent.Agent{
		{ID: "agent-qa", Name: "qa", Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindPicoClawSandbox},
	})
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: im.AdminUserID,
		Users: []im.User{
			{ID: im.AdminUserID, Name: "admin", Role: "admin"},
			{ID: "user-qa", Name: "qa", Role: agent.RoleWorker},
		},
	})
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{
		{
			ID:              "pt-qa",
			Channel:         participant.ChannelCSGClaw,
			Type:            participant.TypeAgent,
			Name:            "qa",
			AgentID:         "agent-qa",
			ChannelUserRef:  "user-qa",
			ChannelUserKind: participant.ChannelUserKindLocalUserID,
			LifecycleStatus: participant.LifecycleStatusActive,
			Mentionable:     true,
		},
		{
			ID:              "pt-qa-feishu",
			Channel:         participant.ChannelFeishu,
			Type:            participant.TypeAgent,
			Name:            "qa Feishu",
			AgentID:         "agent-qa",
			ChannelAppRef:   "cli_xxx",
			ChannelUserRef:  "ou_xxx",
			ChannelUserKind: participant.ChannelUserKindOpenID,
			LifecycleStatus: participant.LifecycleStatusActive,
			Mentionable:     true,
		},
	}), participant.WithAgentService(svc), participant.WithIMService(imSvc))
	srv := &Handler{svc: svc, im: imSvc, participant: participantSvc}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/agent-qa", nil)
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if _, ok := svc.Agent("agent-qa"); ok {
		t.Fatal("agent agent-qa still exists after delete")
	}
	for _, ref := range []struct {
		channel string
		id      string
	}{
		{participant.ChannelCSGClaw, "pt-qa"},
		{participant.ChannelFeishu, "pt-qa-feishu"},
	} {
		if _, ok := participantSvc.Get(ref.channel, ref.id); ok {
			t.Fatalf("participant %s:%s still exists after agent delete", ref.channel, ref.id)
		}
	}
	if _, ok := imSvc.User("user-qa"); ok {
		t.Fatal("local agent user user-qa still exists after deleting its CSGClaw participant")
	}
}

func TestHandleAgentsListExposesLinkedLocalUser(t *testing.T) {
	svc := mustNewSeededService(t, []agent.Agent{
		{ID: "agent-dahym7", Name: "qa", Role: agent.RoleWorker, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	})
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: im.AdminUserID,
		Users: []im.User{
			{ID: im.AdminUserID, Name: "admin", Role: "admin"},
			{ID: "user-dahym7", Name: "qa", Role: agent.RoleWorker, Avatar: "avatar/3D-5.png"},
		},
	})
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "pt-dahym7",
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		Name:            "qa",
		AgentID:         "agent-dahym7",
		ChannelUserRef:  "user-dahym7",
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}))

	srv := &Handler{svc: svc, im: imSvc, participant: participantSvc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents?include_participants=true", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []apitypes.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(agents) = %d, want 1; body=%s", len(got), rec.Body.String())
	}
	if got[0].UserID != "user-dahym7" || got[0].UserName != "qa" {
		t.Fatalf("agent user = %q/%q, want user-dahym7/qa; body=%s", got[0].UserID, got[0].UserName, rec.Body.String())
	}
	if len(got[0].Participants) != 1 || got[0].Participants[0].UserID != "user-dahym7" {
		t.Fatalf("participants = %+v, want linked local user", got[0].Participants)
	}
}

func TestHandleAgentsListHydratesStatusFromSandboxInfo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	runtimeHome := filepath.Join(dir, "agents", "agent-alice", config.RuntimeHomeDirName)
	provider := sandboxtest.NewProvider()
	rt := sandboxtest.NewRuntime()
	rt.Instances["box-stored"] = sandboxtest.NewInstance(sandbox.Info{
		ID:        "box-live",
		Name:      "alice",
		State:     sandbox.StateRunning,
		CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
	})
	provider.Runtimes[runtimeHome] = rt

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
	if got.ID != "agent-alice" || got.Name != "alice" || got.Role != agent.RoleWorker {
		t.Fatalf("agent = %+v, want agent-alice/alice/worker", got)
	}
}

func TestHandleAgentUpgradeUsesLatestDefaultImage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: agent.RuntimeNamePicoClaw,
		Version:     "0.2.0",
		Image:       "registry.example/picoclaw-worker:0.2.0",
	})
	statePath := filepath.Join(t.TempDir(), "agents.json")
	if err := writeSeededAgents(statePath, []agent.Agent{
		{
			ID:           "u-alice",
			Name:         "alice",
			RuntimeID:    "rt-u-alice",
			RuntimeKind:  agent.RuntimeKindPicoClawSandbox,
			Image:        "custom.example/alice-worker:2026.05.27",
			BoxID:        "box-alice-old",
			Role:         agent.RoleWorker,
			Status:       string(agentruntime.StateRunning),
			AgentProfile: agent.AgentProfile{Name: "alice", Provider: agent.ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true},
			CreatedAt:    time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}
	var newImage string
	svc, err := agent.NewService(
		config.ModelConfig{},
		config.ServerConfig{},
		"manager-image:test",
		statePath,
		agent.WithHubService(hubSvc),
		agent.WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local/frontend-worker"}),
		agent.WithRuntime(fakeCompatRuntime{
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				newImage = spec.Image
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "box-alice-new"}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/u-alice/upgrade", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if newImage != "registry.example/picoclaw-worker:0.2.0" {
		t.Fatalf("runtime New() image = %q, want latest default image", newImage)
	}
	var got agent.Agent
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Image != "registry.example/picoclaw-worker:0.2.0" {
		t.Fatalf("response Image = %q, want latest default image", got.Image)
	}
}

func TestHandleAgentsListReportsImageUpgradeRequiredByTemplateVersion(t *testing.T) {
	tests := []struct {
		name         string
		currentImage string
		latestImage  string
		version      string
		wantRequired bool
	}{
		{
			name:         "older template version requires upgrade",
			currentImage: "registry.example/picoclaw-worker:0.1.0",
			latestImage:  "registry.example/picoclaw-worker:0.2.0",
			version:      "0.2.0",
			wantRequired: true,
		},
		{
			name:         "legacy worker base image requires upgrade to template wrapper",
			currentImage: "registry.example/picoclaw:2026.5.27",
			latestImage:  "registry.example/picoclaw-worker:0.2.0",
			version:      "0.2.0",
			wantRequired: true,
		},
		{
			name:         "newer template version does not require upgrade",
			currentImage: "registry.example/picoclaw-worker:0.3.0",
			latestImage:  "registry.example/picoclaw-worker:0.2.0",
			version:      "0.2.0",
			wantRequired: false,
		},
		{
			name:         "dev image does not require upgrade",
			currentImage: "registry.example/picoclaw-worker:dev",
			latestImage:  "registry.example/picoclaw-worker:0.2.0",
			version:      "0.2.0",
			wantRequired: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			provider := sandboxtest.NewProvider()
			provider.Images = []string{"registry.example/picoclaw-worker:9.9.9"}
			hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "frontend-worker", hub.Template{
				ID:          "frontend-worker",
				Name:        "frontend-worker",
				Description: "frontend worker",
				Role:        hub.TemplateRoleWorker,
				RuntimeKind: agent.RuntimeNamePicoClaw,
				Version:     tt.version,
				Image:       tt.latestImage,
			})
			statePath := filepath.Join(t.TempDir(), "agents.json")
			if err := writeSeededAgents(statePath, []agent.Agent{
				{
					ID:           "u-alice",
					Name:         "alice",
					RuntimeID:    "rt-u-alice",
					RuntimeKind:  agent.RuntimeKindPicoClawSandbox,
					Image:        tt.currentImage,
					BoxID:        "box-alice",
					Role:         agent.RoleWorker,
					Status:       string(agentruntime.StateRunning),
					AgentProfile: agent.AgentProfile{Name: "alice", Provider: agent.ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true},
					CreatedAt:    time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC),
				},
			}); err != nil {
				t.Fatalf("writeSeededAgents() error = %v", err)
			}
			svc, err := agent.NewService(
				config.ModelConfig{},
				config.ServerConfig{},
				"manager-image:test",
				statePath,
				agent.WithSandboxProvider(provider),
				agent.WithHubService(hubSvc),
				agent.WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local/frontend-worker"}),
				agent.WithRuntime(fakeCompatRuntime{
					info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
						return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
					},
				}),
			)
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}

			srv := &Handler{svc: svc}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
			rec := httptest.NewRecorder()

			srv.Routes().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
			}
			var got []agentResponse
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("len(agents) = %d, want 1", len(got))
			}
			if got[0].AgentProfile.ImageUpgradeRequired != tt.wantRequired {
				t.Fatalf("image_upgrade_required = %t, want %t; response=%+v", got[0].AgentProfile.ImageUpgradeRequired, tt.wantRequired, got[0])
			}
		})
	}
}

func TestHandleManagerGetReportsImageUpgradeRequiredByTemplateVersion(t *testing.T) {
	tests := []struct {
		name         string
		currentImage string
		latestImage  string
		version      string
		wantRequired bool
	}{
		{
			name:         "older manager version requires upgrade",
			currentImage: "registry.example/opencsghq/picoclaw-manager:0.1.0",
			latestImage:  "registry.example/opencsghq/picoclaw-manager:0.2.0",
			version:      "0.2.0",
			wantRequired: true,
		},
		{
			name:         "legacy manager base image requires upgrade to template wrapper",
			currentImage: "registry.example/opencsghq/picoclaw:2026.5.27",
			latestImage:  "registry.example/opencsghq/picoclaw-manager:0.2.0",
			version:      "0.2.0",
			wantRequired: true,
		},
		{
			name:         "newer manager version does not require upgrade",
			currentImage: "registry.example/opencsghq/picoclaw-manager:0.3.0",
			latestImage:  "registry.example/opencsghq/picoclaw-manager:0.2.0",
			version:      "0.2.0",
			wantRequired: false,
		},
		{
			name:         "dev manager tag does not require upgrade",
			currentImage: "registry.example/opencsghq/picoclaw-manager:dev",
			latestImage:  "registry.example/opencsghq/picoclaw-manager:0.2.0",
			version:      "0.2.0",
			wantRequired: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))
			statePath := filepath.Join(t.TempDir(), "agents.json")
			if err := writeSeededAgents(statePath, []agent.Agent{
				{
					ID:           agent.ManagerUserID,
					Name:         agent.ManagerName,
					RuntimeID:    "rt-manager",
					RuntimeKind:  agent.RuntimeKindPicoClawSandbox,
					Image:        tt.currentImage,
					BoxID:        "box-manager",
					Role:         agent.RoleManager,
					Status:       string(agentruntime.StateRunning),
					AgentProfile: agent.AgentProfile{Name: agent.ManagerName, Provider: agent.ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true},
					CreatedAt:    time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
				},
			}); err != nil {
				t.Fatalf("writeSeededAgents() error = %v", err)
			}
			hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "picoclaw-manager", hub.Template{
				ID:          "picoclaw-manager",
				Name:        "picoclaw-manager",
				Description: "manager",
				Role:        hub.TemplateRoleManager,
				RuntimeKind: agent.RuntimeNamePicoClaw,
				Version:     tt.version,
				Image:       tt.latestImage,
			})
			svc, err := agent.NewService(
				config.ModelConfig{},
				config.ServerConfig{},
				"manager-image:unused",
				statePath,
				agent.WithHubService(hubSvc),
				agent.WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultManagerTemplate: "local/picoclaw-manager"}),
				agent.WithRuntime(fakeCompatRuntime{
					info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
						return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
					},
				}),
			)
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}

			srv := &Handler{svc: svc}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-manager", nil)
			rec := httptest.NewRecorder()

			srv.Routes().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
			}
			var got agentResponse
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if got.AgentProfile.ImageUpgradeRequired != tt.wantRequired {
				t.Fatalf("manager image_upgrade_required = %t, want %t; response=%+v", got.AgentProfile.ImageUpgradeRequired, tt.wantRequired, got)
			}
		})
	}
}

func TestHandleAgentUpgradeClearsOutdatedImageFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	provider := sandboxtest.NewProvider()
	provider.Images = []string{"registry.example/picoclaw-worker:0.1.0"}
	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: agent.RuntimeNamePicoClaw,
		Version:     "0.2.0",
		Image:       "registry.example/picoclaw-worker:0.2.0",
	})
	statePath := filepath.Join(t.TempDir(), "agents.json")
	if err := writeSeededAgents(statePath, []agent.Agent{
		{
			ID:           "u-alice",
			Name:         "alice",
			RuntimeID:    "rt-u-alice",
			RuntimeKind:  agent.RuntimeKindPicoClawSandbox,
			Image:        "registry.example/picoclaw-worker:0.1.0",
			BoxID:        "box-alice-old",
			Role:         agent.RoleWorker,
			Status:       string(agentruntime.StateRunning),
			AgentProfile: agent.AgentProfile{Name: "alice", Provider: agent.ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true},
			CreatedAt:    time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}
	var newImage string
	svc, err := agent.NewService(
		config.ModelConfig{},
		config.ServerConfig{},
		"manager-image:test",
		statePath,
		agent.WithSandboxProvider(provider),
		agent.WithHubService(hubSvc),
		agent.WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local/frontend-worker"}),
		agent.WithRuntime(fakeCompatRuntime{
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				newImage = spec.Image
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "box-alice-new"}, nil
			},
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning, CreatedAt: time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	srv := &Handler{svc: svc}
	beforeReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-alice", nil)
	beforeRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(beforeRec, beforeReq)
	if beforeRec.Code != http.StatusOK {
		t.Fatalf("pre-upgrade status = %d, want %d; body=%s", beforeRec.Code, http.StatusOK, beforeRec.Body.String())
	}
	var before agentResponse
	if err := json.NewDecoder(beforeRec.Body).Decode(&before); err != nil {
		t.Fatalf("decode pre-upgrade response: %v", err)
	}
	if !before.AgentProfile.ImageUpgradeRequired {
		t.Fatalf("pre-upgrade image_upgrade_required = false, want true; response=%+v", before)
	}

	upgradeReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/u-alice/upgrade", nil)
	upgradeRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(upgradeRec, upgradeReq)
	if upgradeRec.Code != http.StatusOK {
		t.Fatalf("upgrade status = %d, want %d; body=%s", upgradeRec.Code, http.StatusOK, upgradeRec.Body.String())
	}
	if newImage != "registry.example/picoclaw-worker:0.2.0" {
		t.Fatalf("runtime New() image = %q, want latest default image", newImage)
	}
	var upgraded agentResponse
	if err := json.NewDecoder(upgradeRec.Body).Decode(&upgraded); err != nil {
		t.Fatalf("decode upgrade response: %v", err)
	}
	if upgraded.Image != "registry.example/picoclaw-worker:0.2.0" {
		t.Fatalf("upgrade response Image = %q, want latest default image", upgraded.Image)
	}
	if upgraded.AgentProfile.ImageUpgradeRequired {
		t.Fatalf("upgrade response image_upgrade_required = true, want false; response=%+v", upgraded)
	}

	afterReq := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-alice", nil)
	afterRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(afterRec, afterReq)
	if afterRec.Code != http.StatusOK {
		t.Fatalf("post-upgrade status = %d, want %d; body=%s", afterRec.Code, http.StatusOK, afterRec.Body.String())
	}
	var after agentResponse
	if err := json.NewDecoder(afterRec.Body).Decode(&after); err != nil {
		t.Fatalf("decode post-upgrade response: %v", err)
	}
	if after.Image != "registry.example/picoclaw-worker:0.2.0" || after.AgentProfile.ImageUpgradeRequired {
		t.Fatalf("post-upgrade response = %+v, want latest image and no image upgrade flag", after)
	}
}

func TestHandleManagerUpgradeUsesDefaultTemplateVersionWhenLocalImageListIsStale(t *testing.T) {
	t.Skip("manager is Codex-only; sandbox manager image upgrades are obsolete")
	t.Setenv("HOME", t.TempDir())
	provider := sandboxtest.NewProvider()
	provider.Images = []string{
		"opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw-manager:0.1.0",
	}
	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "picoclaw-manager", hub.Template{
		ID:          "picoclaw-manager",
		Name:        "picoclaw-manager",
		Description: "manager",
		Role:        hub.TemplateRoleManager,
		RuntimeKind: agent.RuntimeNamePicoClaw,
		Version:     "0.2.0",
		Image:       "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw-manager:0.2.0",
	})
	statePath := filepath.Join(t.TempDir(), "agents.json")
	if err := writeSeededAgents(statePath, []agent.Agent{
		{
			ID:           agent.ManagerUserID,
			Name:         agent.ManagerName,
			RuntimeID:    "rt-manager",
			RuntimeKind:  agent.RuntimeKindPicoClawSandbox,
			Image:        "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.5.27",
			BoxID:        "box-manager-old",
			Role:         agent.RoleManager,
			Status:       string(agentruntime.StateRunning),
			AgentProfile: agent.AgentProfile{Name: agent.ManagerName, Provider: agent.ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true},
			CreatedAt:    time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}
	var newImage string
	svc, err := agent.NewService(
		config.ModelConfig{},
		config.ServerConfig{},
		"manager-image:unused",
		statePath,
		agent.WithSandboxProvider(provider),
		agent.WithHubService(hubSvc),
		agent.WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultManagerTemplate: "local/picoclaw-manager"}),
		agent.WithRuntime(fakeCompatRuntime{
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				newImage = spec.Image
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "box-manager-new"}, nil
			},
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning, CreatedAt: time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	srv := httptest.NewServer((&Handler{svc: svc}).Routes())
	defer srv.Close()
	getAgent := func(path string) agentResponse {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s error = %v", path, err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read GET %s response: %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d; body=%s", path, resp.StatusCode, http.StatusOK, string(body))
		}
		var got agentResponse
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("decode GET %s response: %v", path, err)
		}
		return got
	}

	before := getAgent("/api/v1/agents/u-manager")
	if !before.AgentProfile.ImageUpgradeRequired {
		t.Fatalf("pre-upgrade image_upgrade_required = false, want true; response=%+v", before)
	}

	resp, err := http.Post(srv.URL+"/api/v1/agents/u-manager/upgrade", "application/json", nil)
	if err != nil {
		t.Fatalf("POST upgrade error = %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read POST upgrade response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST upgrade status = %d, want %d; body=%s", resp.StatusCode, http.StatusOK, string(body))
	}
	wantImage := "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw-manager:0.2.0"
	if newImage != wantImage {
		t.Fatalf("runtime New() image = %q, want %q", newImage, wantImage)
	}
	var upgraded agentResponse
	if err := json.Unmarshal(body, &upgraded); err != nil {
		t.Fatalf("decode upgrade response: %v", err)
	}
	if upgraded.Image != wantImage || upgraded.AgentProfile.ImageUpgradeRequired {
		t.Fatalf("upgrade response = %+v, want latest template image and no image upgrade flag", upgraded)
	}

	after := getAgent("/api/v1/agents/u-manager")
	if after.Image != wantImage || after.AgentProfile.ImageUpgradeRequired {
		t.Fatalf("post-upgrade response = %+v, want latest template image and no image upgrade flag", after)
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
	profile, ok := got[0]["model_config"].(map[string]any)
	if !ok || profile["api_key_set"] != true {
		t.Fatalf("model_config = %#v, want api_key_set true", got[0]["model_config"])
	}
	if got, want := profile["api_key_preview"], "secr....oken"; got != want {
		t.Fatalf("model_config api_key_preview = %#v, want %q", got, want)
	}
	if _, ok := profile["api_key"]; ok {
		t.Fatalf("model_config includes api_key: %#v", profile)
	}
}

func TestHandleAgentsPatchUpdatesMetadataAndProfile(t *testing.T) {
	svc := mustNewSeededServiceWithOptions(t, []agent.Agent{
		{
			ID:          "u-alice",
			Name:        "alice",
			Role:        agent.RoleWorker,
			RuntimeKind: agent.RuntimeKindCodex,
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
	}, agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}))

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
	profile, ok := got["model_config"].(map[string]any)
	env, envOK := profile["env"].(map[string]any)
	if !ok || profile["model_id"] != "new-model" || !envOK || env["A"] != "B" {
		t.Fatalf("model_config = %#v, want updated model and env", got["model_config"])
	}
}

func TestHandleAgentsPatchFieldMaskClearsRuntimeOptions(t *testing.T) {
	svc := mustNewSeededServiceWithOptions(t, []agent.Agent{
		{
			ID:          "u-alice",
			Name:        "alice",
			Role:        agent.RoleWorker,
			RuntimeKind: agent.RuntimeKindCodex,
			RuntimeOptions: map[string]any{
				"local_workspace_dir": "/tmp/project",
			},
			AgentProfile: agent.AgentProfile{
				Name:            "alice",
				Provider:        agent.ProviderCodex,
				ModelID:         "gpt-5.5",
				ProfileComplete: true,
			},
			ProfileComplete: true,
			CreatedAt:       time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		},
	}, agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}))

	srv := &Handler{svc: svc}
	body := `{"runtime_options":{},"field_mask":["runtime_options"]}`
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
	runtime, runtimeOK := got["runtime"].(map[string]any)
	if !runtimeOK {
		t.Fatalf("runtime = %#v, want runtime object", got["runtime"])
	}
	if runtimeOptions, ok := runtime["options"].(map[string]any); ok && len(runtimeOptions) != 0 {
		t.Fatalf("runtime.options = %#v, want cleared map", runtimeOptions)
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
	if got.ID != "agent-bob" || got.Name != "bob" {
		t.Fatalf("agent = %+v, want agent-bob/bob", got)
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
		{ID: "u-alice", Name: "alice", BoxID: "box-old", Role: agent.RoleWorker, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	})
	if err := writeSeededAgents(statePath, []agent.Agent{
		{ID: "u-alice", Name: "alice", BoxID: "box-new", Role: agent.RoleWorker, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}

	var gotBoxID string
	agent.TestOnlySetGetBoxHook(func(_ *agent.Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		gotBoxID = idOrName
		return sandboxtest.NewInstance(sandbox.Info{ID: idOrName, Name: "alice"}), nil
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-alice/logs", nil)
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
	if len(bridge.stopCalls) != 1 || bridge.stopCalls[0] != "agent-alice" {
		t.Fatalf("StopAgent() calls = %v, want [agent-alice]", bridge.stopCalls)
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
	if !strings.HasPrefix(got.ID, "agent-") || got.Role != agent.RoleWorker {
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"name":"alice","role":"worker","runtime_name":"codex"}`))
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

func TestHandleAgentsCreateWorkerSupportsLegacyRuntimeKindRequest(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"name":"alice","role":"worker","runtime_name":"codex","agent_profile":{"profile_complete":true}}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(bridge.ensureCalls) != 1 {
		t.Fatalf("EnsureAgent() calls = %d, want 1", len(bridge.ensureCalls))
	}
	if !strings.HasPrefix(bridge.ensureCalls[0].ID, "agent-") || bridge.ensureCalls[0].RuntimeKind != agent.RuntimeKindCodex {
		t.Fatalf("EnsureAgent() got %+v, want codex worker typed agent ID", bridge.ensureCalls[0])
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

func TestHandleAgentsCreateManagerRejectsNonCodexRuntime(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc := mustNewService(t)
	srv := &Handler{svc: svc}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"id":"manager","name":"manager","runtime_kind":"openclaw_sandbox"}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "manager runtime is fixed to codex") {
		t.Fatalf("body = %q, want fixed codex runtime rejection", rec.Body.String())
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
	if got.ID != "agent-alice" || got.Name != "alice-v2" || got.Role != agent.RoleWorker {
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
	if got.ID != "agent-alice" || got.Name != "alice-v2" || got.Description != "worker" || got.Image != "agent-image:v1" {
		t.Fatalf("agent = %+v, want masked replace preserving unmasked fields", got)
	}
}

func TestAgentCreateRequestFromAPIIncludesFromTemplate(t *testing.T) {
	got := agentCreateRequestFromAPI(apitypes.CreateAgentRequest{
		Name:         "alice",
		Instructions: "follow AGENTS",
		RuntimeName:  agent.RuntimeNameCodex,
		FromTemplate: "builtin.frontend-alice",
		Profile:      "codex-fast",
	})

	if got.Spec.Name != "alice" {
		t.Fatalf("Spec.Name = %q, want %q", got.Spec.Name, "alice")
	}
	if got.Spec.RuntimeName != agent.RuntimeNameCodex || got.Spec.SandboxEnabled {
		t.Fatalf("Spec runtime = %q/%t, want %q/%t", got.Spec.RuntimeName, got.Spec.SandboxEnabled, agent.RuntimeNameCodex, false)
	}
	if got.Spec.FromTemplate != "builtin.frontend-alice" {
		t.Fatalf("Spec.FromTemplate = %q, want %q", got.Spec.FromTemplate, "builtin.frontend-alice")
	}
	if got.Spec.Profile != "codex-fast" {
		t.Fatalf("Spec.Profile = %q, want %q", got.Spec.Profile, "codex-fast")
	}
	if got.Spec.Instructions != "follow AGENTS" {
		t.Fatalf("Spec.Instructions = %q, want %q", got.Spec.Instructions, "follow AGENTS")
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

func TestHandleHubTemplateByIDReturnsTemplate(t *testing.T) {
	hubSvc := mustNewLocalTemplateHubService(t, "review-bot", hub.Template{
		ID:          "review-bot",
		Name:        "review-bot",
		Description: "code review helper",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: agent.RuntimeKindCodex,
	})
	srv := &Handler{}
	srv.SetHubService(hubSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates/local.review-bot", nil)
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got apitypes.HubTemplate
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "local.review-bot" {
		t.Fatalf("template id = %q, want %q", got.ID, "local.review-bot")
	}
	if got.Source.Name != "local" || got.Source.Kind != "local" {
		t.Fatalf("template source = %+v, want local/local", got.Source)
	}
	if len(got.Workspace.Entries) != 0 {
		t.Fatalf("workspace entries = %#v, want lazy-loaded empty result", got.Workspace.Entries)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates/local.review-bot/workspace", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("workspace status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var listing apitypes.WorkspaceListing
	if err := json.NewDecoder(rec.Body).Decode(&listing); err != nil {
		t.Fatalf("decode workspace response: %v", err)
	}
	paths := make([]string, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		paths = append(paths, entry.Path)
	}
	if !slices.Contains(paths, "USER.md") || !slices.Contains(paths, "skills") {
		t.Fatalf("workspace paths = %#v, want USER.md and skills", paths)
	}
	if slices.Contains(paths, "skills/custom") || slices.Contains(paths, "skills/custom/SKILL.md") {
		t.Fatalf("workspace paths = %#v, want top-level entries only", paths)
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/hub/templates/local.review-bot/workspace?path=skills",
		nil,
	)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("nested workspace status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	listing = apitypes.WorkspaceListing{}
	if err := json.NewDecoder(rec.Body).Decode(&listing); err != nil {
		t.Fatalf("decode nested workspace response: %v", err)
	}
	if len(listing.Entries) != 1 || listing.Entries[0].Path != "skills/custom" {
		t.Fatalf("nested workspace entries = %#v, want skills/custom only", listing.Entries)
	}
}

func TestHandleHubTemplateWorkspaceFileByIDReturnsContent(t *testing.T) {
	hubSvc := mustNewLocalTemplateHubService(t, "review-bot", hub.Template{
		ID:          "review-bot",
		Name:        "review-bot",
		Description: "code review helper",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: agent.RuntimeKindCodex,
	})
	srv := &Handler{}
	srv.SetHubService(hubSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates/local.review-bot/workspace/file?path=skills/custom/SKILL.md", nil)
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
	workspaceRoot, err := svc.WorkspaceRoot(created.Name)
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

func TestHandleAgentSkillsReturnsContentFromSkillsRoot(t *testing.T) {
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
		agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	created, err := svc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			Name:        "alice",
			Role:        agent.RoleWorker,
			RuntimeKind: agent.RuntimeKindCodex,
			Image:       "worker-image:test",
			AgentProfile: agent.AgentProfile{
				ProfileComplete: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	skillsRoot, err := svc.SkillsRoot(created.Name)
	if err != nil {
		t.Fatalf("SkillsRoot() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillsRoot, "custom"), 0o755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "custom", "SKILL.md"), []byte("# Custom\n"), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	srv := &Handler{svc: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+created.ID+"/skills", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var listing apitypes.WorkspaceListing
	if err := json.NewDecoder(rec.Body).Decode(&listing); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	paths := make([]string, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		paths = append(paths, entry.Path)
	}
	if !slices.Contains(paths, "custom") || !slices.Contains(paths, "custom/SKILL.md") {
		t.Fatalf("skills paths = %#v, want custom and custom/SKILL.md", paths)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+created.ID+"/skills/file?path=custom/SKILL.md", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("file status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var file apitypes.WorkspaceFile
	if err := json.NewDecoder(rec.Body).Decode(&file); err != nil {
		t.Fatalf("decode file response: %v", err)
	}
	if got, want := file.Path, "custom/SKILL.md"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if strings.TrimSpace(file.Content) != "# Custom" {
		t.Fatalf("content = %q, want %q", strings.TrimSpace(file.Content), "# Custom")
	}
}

func TestHandleAgentSkillsBatchAddCopiesGlobalSkillIntoAgentRuntimeRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	globalRoot := filepath.Join(home, ".csgclaw", "skills")
	if err := os.MkdirAll(filepath.Join(globalRoot, "alpha", "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll(global alpha) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalRoot, "alpha", "SKILL.md"), []byte("---\ndescription: Alpha skill\n---\n# Alpha\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(global SKILL.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalRoot, "alpha", "scripts", "run.sh"), []byte("echo alpha\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(global run.sh) error = %v", err)
	}

	srv, svc, created := newAgentSkillManagementTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+created.ID+"/skills:batchAdd", strings.NewReader(`{"names":["alpha"]}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	skillsRoot, err := svc.SkillsRoot(created.Name)
	if err != nil {
		t.Fatalf("SkillsRoot() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillsRoot, "alpha", "SKILL.md")); err != nil {
		t.Fatalf("Stat(agent SKILL.md) error = %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(skillsRoot, "alpha", "scripts", "run.sh")); err != nil {
		t.Fatalf("ReadFile(agent run.sh) error = %v", err)
	} else if strings.TrimSpace(string(data)) != "echo alpha" {
		t.Fatalf("run.sh content = %q, want %q", strings.TrimSpace(string(data)), "echo alpha")
	}

	workspaceRoot, err := svc.WorkspaceRoot(created.Name)
	if err != nil {
		t.Fatalf("WorkspaceRoot() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "skills", "alpha", "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(workspace skills alpha) error = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(globalRoot, "alpha", "SKILL.md")); err != nil {
		t.Fatalf("Stat(global SKILL.md) error = %v", err)
	}
}

func TestHandleAgentSkillsBatchAddReturnsNotFoundWhenGlobalSkillMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	srv, _, created := newAgentSkillManagementTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+created.ID+"/skills:batchAdd", strings.NewReader(`{"names":["missing"]}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleAgentSkillsBatchAddReturnsBadRequestWhenGlobalSkillInvalid(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	globalRoot := filepath.Join(home, ".csgclaw", "skills")
	if err := os.MkdirAll(filepath.Join(globalRoot, "alpha"), 0o755); err != nil {
		t.Fatalf("MkdirAll(global alpha) error = %v", err)
	}

	srv, _, created := newAgentSkillManagementTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+created.ID+"/skills:batchAdd", strings.NewReader(`{"names":["alpha"]}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleAgentSkillsBatchAddReturnsConflictWhenSkillAlreadyExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	globalRoot := filepath.Join(home, ".csgclaw", "skills")
	if err := os.MkdirAll(filepath.Join(globalRoot, "alpha"), 0o755); err != nil {
		t.Fatalf("MkdirAll(global alpha) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalRoot, "alpha", "SKILL.md"), []byte("# Alpha\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(global SKILL.md) error = %v", err)
	}

	srv, svc, created := newAgentSkillManagementTestServer(t)
	skillsRoot, err := svc.SkillsRoot(created.Name)
	if err != nil {
		t.Fatalf("SkillsRoot() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillsRoot, "alpha"), 0o755); err != nil {
		t.Fatalf("MkdirAll(agent alpha) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "alpha", "SKILL.md"), []byte("# Existing\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(agent SKILL.md) error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+created.ID+"/skills:batchAdd", strings.NewReader(`{"names":["alpha"]}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleAgentSkillDeleteRemovesOnlyAgentScopedSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	globalRoot := filepath.Join(home, ".csgclaw", "skills")
	if err := os.MkdirAll(filepath.Join(globalRoot, "alpha"), 0o755); err != nil {
		t.Fatalf("MkdirAll(global alpha) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalRoot, "alpha", "SKILL.md"), []byte("# Global Alpha\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(global SKILL.md) error = %v", err)
	}

	srv, svc, created := newAgentSkillManagementTestServer(t)
	skillsRoot, err := svc.SkillsRoot(created.Name)
	if err != nil {
		t.Fatalf("SkillsRoot() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillsRoot, "alpha"), 0o755); err != nil {
		t.Fatalf("MkdirAll(agent alpha) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "alpha", "SKILL.md"), []byte("# Agent Alpha\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(agent SKILL.md) error = %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/"+created.ID+"/skills/alpha", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(skillsRoot, "alpha", "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(agent SKILL.md) error = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(globalRoot, "alpha", "SKILL.md")); err != nil {
		t.Fatalf("Stat(global SKILL.md) error = %v", err)
	}
}

func TestHandleAgentSkillsMutationsReturnNotFoundForMissingAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	srv, _, _ := newAgentSkillManagementTestServer(t)

	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/agents/u-missing/skills:batchAdd", strings.NewReader(`{"names":["alpha"]}`))
	postRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusNotFound {
		t.Fatalf("batch add status = %d, want %d; body=%s", postRec.Code, http.StatusNotFound, postRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/u-missing/skills/alpha", nil)
	deleteRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNotFound {
		t.Fatalf("delete status = %d, want %d; body=%s", deleteRec.Code, http.StatusNotFound, deleteRec.Body.String())
	}
}

func newAgentSkillManagementTestServer(t *testing.T) (*Handler, *agent.Service, agent.Agent) {
	t.Helper()
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
		agent.WithRuntime(fakeCompatRuntime{kind: agent.RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	created, err := svc.Create(context.Background(), agent.CreateRequest{
		Spec: agent.CreateAgentSpec{
			Name:        "alice",
			Role:        agent.RoleWorker,
			RuntimeKind: agent.RuntimeKindCodex,
			Image:       "worker-image:test",
			AgentProfile: agent.AgentProfile{
				ProfileComplete: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return &Handler{svc: svc}, svc, created
}

func TestHandleSkillsListsGlobalSkillsAndBrowsesFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	skillsRoot := filepath.Join(home, ".csgclaw", "skills")
	if err := os.MkdirAll(filepath.Join(skillsRoot, "alpha", "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll(skills) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "alpha", "SKILL.md"), []byte("---\ndescription: Alpha skill\n---\n# Alpha\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(alpha SKILL.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "alpha", "scripts", "run.sh"), []byte("echo hi\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(run.sh) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillsRoot, "beta"), 0o755); err != nil {
		t.Fatalf("MkdirAll(beta) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "beta", "SKILL.md"), []byte("# Beta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(beta SKILL.md) error = %v", err)
	}

	srv := &Handler{}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var skills []skillsystem.SkillSummary
	if err := json.NewDecoder(rec.Body).Decode(&skills); err != nil {
		t.Fatalf("decode skills response: %v", err)
	}
	if len(skills) != 4 {
		t.Fatalf("len(skills) = %d, want 4", len(skills))
	}
	skillsByName := map[string]skillsystem.SkillSummary{}
	for _, item := range skills {
		skillsByName[item.Name] = item
	}
	if got := skillsByName["alpha"]; got.Description != "Alpha skill" || got.Source != skillsystem.SkillSourceLocal || got.Readonly {
		t.Fatalf("alpha skill = %+v, want local alpha with description", got)
	}
	if got := skillsByName["beta"]; got.Name != "beta" || got.Description != "" || got.Source != skillsystem.SkillSourceLocal || got.Readonly {
		t.Fatalf("beta skill = %+v, want local beta without description", got)
	}
	if got := skillsByName["skill-installer"]; got.Name != "skill-installer" || got.Source != skillsystem.SkillSourceSystem || !got.Readonly {
		t.Fatalf("skill-installer = %+v, want read-only system skill", got)
	}
	if got := skillsByName["skill-creator"]; got.Name != "skill-creator" || got.Source != skillsystem.SkillSourceSystem || !got.Readonly {
		t.Fatalf("skill-creator = %+v, want read-only system skill", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/skills/tree", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("root tree status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var listing apitypes.WorkspaceListing
	if err := json.NewDecoder(rec.Body).Decode(&listing); err != nil {
		t.Fatalf("decode root tree response: %v", err)
	}
	paths := make([]string, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		paths = append(paths, entry.Path)
	}
	if !slices.Contains(paths, "alpha/SKILL.md") || !slices.Contains(paths, "skill-installer/SKILL.md") {
		t.Fatalf("root tree paths = %#v, want local and system skill files", paths)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/skills/tree?path=.", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dot root tree status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&listing); err != nil {
		t.Fatalf("decode dot root tree response: %v", err)
	}
	paths = paths[:0]
	for _, entry := range listing.Entries {
		paths = append(paths, entry.Path)
	}
	if !slices.Contains(paths, "alpha/SKILL.md") || !slices.Contains(paths, "skill-installer/SKILL.md") {
		t.Fatalf("dot root tree paths = %#v, want local and system skill files", paths)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/skills/tree?path=alpha", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("tree status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&listing); err != nil {
		t.Fatalf("decode tree response: %v", err)
	}
	paths = paths[:0]
	for _, entry := range listing.Entries {
		paths = append(paths, entry.Path)
	}
	if !slices.Contains(paths, "alpha/SKILL.md") || !slices.Contains(paths, "alpha/scripts/run.sh") {
		t.Fatalf("tree paths = %#v, want alpha files", paths)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/skills/file?path=alpha/SKILL.md", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("file status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var file apitypes.WorkspaceFile
	if err := json.NewDecoder(rec.Body).Decode(&file); err != nil {
		t.Fatalf("decode file response: %v", err)
	}
	if got, want := file.Path, "alpha/SKILL.md"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if !strings.Contains(file.Content, "# Alpha") {
		t.Fatalf("content = %q, want preview with # Alpha", file.Content)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/skills/tree?path=skill-installer", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("system tree status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&listing); err != nil {
		t.Fatalf("decode system tree response: %v", err)
	}
	paths = paths[:0]
	for _, entry := range listing.Entries {
		paths = append(paths, entry.Path)
	}
	if !slices.Contains(paths, "skill-installer/SKILL.md") {
		t.Fatalf("system tree paths = %#v, want skill-installer/SKILL.md", paths)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/skills/file?path=skill-installer/SKILL.md", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("system file status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&file); err != nil {
		t.Fatalf("decode system file response: %v", err)
	}
	if !strings.Contains(file.Content, "registry skill search") {
		t.Fatalf("system skill content = %q, want skill-installer instructions", file.Content)
	}
}

func TestHandleSkillsMissingRootUsesEmptyOrNotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	srv := &Handler{}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var skills []skillsystem.SkillSummary
	if err := json.NewDecoder(rec.Body).Decode(&skills); err != nil {
		t.Fatalf("decode skills response: %v", err)
	}
	skillsByName := map[string]skillsystem.SkillSummary{}
	for _, item := range skills {
		skillsByName[item.Name] = item
	}
	for _, name := range []string{"skill-installer", "skill-creator"} {
		if got := skillsByName[name]; got.Name != name || got.Source != skillsystem.SkillSourceSystem || !got.Readonly {
			t.Fatalf("skills = %+v, want read-only system skill %s", skills, name)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/skills/tree", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("root tree status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var listing apitypes.WorkspaceListing
	if err := json.NewDecoder(rec.Body).Decode(&listing); err != nil {
		t.Fatalf("decode root tree response: %v", err)
	}
	paths := make([]string, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		paths = append(paths, entry.Path)
	}
	if !slices.Contains(paths, "skill-installer/SKILL.md") {
		t.Fatalf("root tree paths = %#v, want system skill file", paths)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/skills/tree?path=alpha", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("tree status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/skills/file?path=alpha/SKILL.md", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("file status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleSkillsBrowsesSystemSkillWhenLocalSystemSkillIsMalformed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillsRoot := filepath.Join(home, ".csgclaw", "skills")
	if err := os.MkdirAll(filepath.Join(skillsRoot, "skill-installer"), 0o755); err != nil {
		t.Fatalf("MkdirAll(malformed skill-installer) error = %v", err)
	}

	srv := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var skills []skillsystem.SkillSummary
	if err := json.NewDecoder(rec.Body).Decode(&skills); err != nil {
		t.Fatalf("decode skills response: %v", err)
	}
	skillsByName := map[string]skillsystem.SkillSummary{}
	for _, item := range skills {
		skillsByName[item.Name] = item
	}
	for _, name := range []string{"skill-installer", "skill-creator"} {
		if got := skillsByName[name]; got.Name != name || got.Source != skillsystem.SkillSourceSystem || !got.Readonly {
			t.Fatalf("skills = %+v, want read-only system skill %s", skills, name)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/skills/tree?path=skill-installer", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tree status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var listing apitypes.WorkspaceListing
	if err := json.NewDecoder(rec.Body).Decode(&listing); err != nil {
		t.Fatalf("decode tree response: %v", err)
	}
	paths := make([]string, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		paths = append(paths, entry.Path)
	}
	if !slices.Contains(paths, "skill-installer/SKILL.md") {
		t.Fatalf("tree paths = %#v, want system skill file", paths)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/skills/file?path=skill-installer/SKILL.md", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("file status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var file apitypes.WorkspaceFile
	if err := json.NewDecoder(rec.Body).Decode(&file); err != nil {
		t.Fatalf("decode file response: %v", err)
	}
	if !strings.Contains(file.Content, "registry skill search") {
		t.Fatalf("system skill content = %q, want skill-installer instructions", file.Content)
	}
}

func TestHandleSkillDeleteRejectsSystemSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	srv := &Handler{}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/skills/skill-installer", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("delete status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestHandleSkillDeleteAllowsLocalSkillWithSystemName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillsRoot := filepath.Join(home, ".csgclaw", "skills")
	if err := os.MkdirAll(filepath.Join(skillsRoot, "skill-installer"), 0o755); err != nil {
		t.Fatalf("MkdirAll(skill-installer) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "skill-installer", "SKILL.md"), []byte("# Local installer\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(local installer) error = %v", err)
	}

	srv := &Handler{}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/skills/skill-installer", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(skillsRoot, "skill-installer")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("local skill-installer still exists, err=%v", err)
	}
}

func TestHandleSkillUpload(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	srv := &Handler{}
	req := newSkillUploadRequest(t, "alpha.zip", map[string]string{
		"alpha/SKILL.md":       "---\ndescription: Alpha skill\n---\n# Alpha\n",
		"alpha/scripts/run.sh": "#!/bin/sh\necho hi\n",
	})
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var skill struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&skill); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if skill.Name != "alpha" || skill.Description != "Alpha skill" {
		t.Fatalf("uploaded skill = %+v, want alpha summary", skill)
	}
	if _, err := os.Stat(filepath.Join(home, ".csgclaw", "skills", "alpha", "SKILL.md")); err != nil {
		t.Fatalf("installed SKILL.md missing: %v", err)
	}
}

func TestHandleSkillInstallFromOfficialHub(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rootTreePages := 0
	officialHub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/skills/AIWizards/agent-builder/refs/dev/tree/":
			rootTreePages++
			cursor := r.URL.Query().Get("cursor")
			if r.URL.Query().Get("limit") != "500" {
				t.Errorf("root tree query = %s, want limit=500", r.URL.RawQuery)
				http.Error(w, "bad query", http.StatusBadRequest)
				return
			}
			if cursor == "" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{
						"Cursor": "next-root-page",
						"Files":  []map[string]any{},
					},
				})
				return
			}
			if cursor != "next-root-page" {
				t.Errorf("root tree cursor = %q, want next-root-page", cursor)
				http.Error(w, "bad cursor", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"Files": []map[string]any{
						{"name": "SKILL.md", "path": "SKILL.md", "type": "file"},
						{"name": "scripts", "path": "scripts", "type": "dir"},
					},
				},
			})
		case "/api/v1/skills/AIWizards/agent-builder/refs/dev/tree/scripts":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"Files": []map[string]any{
						{"name": "run.sh", "path": "scripts/run.sh", "type": "file"},
					},
				},
			})
		case "/api/v1/skills/AIWizards/agent-builder/blob/SKILL.md":
			if r.URL.Query().Get("ref") != "dev" {
				t.Errorf("blob ref = %q, want dev", r.URL.Query().Get("ref"))
				http.Error(w, "bad ref", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"content": base64.StdEncoding.EncodeToString([]byte("---\ndescription: Build agents\n---\n# Agent Builder\n")),
					"path":    "SKILL.md",
					"type":    "file",
				},
			})
		case "/api/v1/skills/AIWizards/agent-builder/blob/scripts/run.sh":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"content": base64.StdEncoding.EncodeToString([]byte("#!/bin/sh\necho ready\n")),
					"path":    "scripts/run.sh",
					"type":    "file",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer officialHub.Close()

	configPath := filepath.Join(t.TempDir(), "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"
access_token = "secret"

[models]
default = "default.model"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
models = ["model"]

[[hub.registries]]
name = "official"
kind = "remote"
url = "` + officialHub.URL + `/"
enabled = true
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	srv := &Handler{}
	srv.SetConfigPath(configPath)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/skills:install", strings.NewReader(`{
		"remote_path": "AIWizards/agent-builder",
		"ref": "dev"
	}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("install status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var skill struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&skill); err != nil {
		t.Fatalf("decode install response: %v", err)
	}
	if skill.Name != "agent-builder" || skill.Description != "Build agents" {
		t.Fatalf("installed skill = %+v, want agent-builder summary", skill)
	}
	skillRoot := filepath.Join(home, ".csgclaw", "skills", "agent-builder")
	if _, err := os.Stat(filepath.Join(skillRoot, "SKILL.md")); err != nil {
		t.Fatalf("installed SKILL.md missing: %v", err)
	}
	script, err := os.ReadFile(filepath.Join(skillRoot, "scripts", "run.sh"))
	if err != nil {
		t.Fatalf("read installed script: %v", err)
	}
	if !strings.Contains(string(script), "echo ready") {
		t.Fatalf("script = %q, want remote script content", script)
	}
	staleFile := filepath.Join(skillRoot, "stale.txt")
	if err := os.WriteFile(staleFile, []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale) error = %v", err)
	}

	replaceReq := httptest.NewRequest(http.MethodPost, "/api/v1/skills:install", strings.NewReader(`{
		"remote_path": "AIWizards/agent-builder",
		"ref": "dev",
		"replace": true
	}`))
	replaceRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(replaceRec, replaceReq)

	if replaceRec.Code != http.StatusCreated {
		t.Fatalf("replace install status = %d, want %d; body=%s", replaceRec.Code, http.StatusCreated, replaceRec.Body.String())
	}
	if _, err := os.Stat(staleFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale file still exists after replace, err=%v", err)
	}
	if rootTreePages != 4 {
		t.Fatalf("root tree pages = %d, want 4", rootTreePages)
	}
}

func TestHandleSkillUploadRejectsDuplicate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".csgclaw", "skills", "alpha"), 0o755); err != nil {
		t.Fatalf("MkdirAll(alpha) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".csgclaw", "skills", "alpha", "SKILL.md"), []byte("# Alpha\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(alpha SKILL.md) error = %v", err)
	}

	srv := &Handler{}
	req := newSkillUploadRequest(t, "alpha.zip", map[string]string{
		"alpha/SKILL.md": "# Alpha\n",
	})
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("upload status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleSkillDelete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	skillsRoot := filepath.Join(home, ".csgclaw", "skills")
	if err := os.MkdirAll(filepath.Join(skillsRoot, "alpha", "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll(alpha) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "alpha", "SKILL.md"), []byte("# Alpha\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(alpha SKILL.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "alpha", "scripts", "run.sh"), []byte("echo hi\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(run.sh) error = %v", err)
	}

	srv := &Handler{}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/skills/alpha", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(skillsRoot, "alpha")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("alpha still exists, err = %v", err)
	}
}

func TestHandleSkillUploadRejectsInvalidArchive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	srv := &Handler{}
	req := newSkillUploadRequest(t, "invalid.zip", map[string]string{
		"alpha/SKILL.md": "# Alpha\n",
		"beta/SKILL.md":  "# Beta\n",
	})
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("upload status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates/local.review-bot", nil)
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

	req = httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates/local.review-bot/workspace", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("workspace status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var listing apitypes.WorkspaceListing
	if err := json.NewDecoder(rec.Body).Decode(&listing); err != nil {
		t.Fatalf("decode workspace response: %v", err)
	}
	if len(listing.Entries) != 0 {
		t.Fatalf("workspace entries = %#v, want empty", listing.Entries)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/hub/templates/local.review-bot/workspace/file?path=USER.md", nil)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("file status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "workspace") {
		t.Fatalf("file response body = %q, want workspace error", rec.Body.String())
	}
}

func newSkillUploadRequest(t *testing.T, filename string, files map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write(mustZipBytes(t, files)); err != nil {
		t.Fatalf("Write(zip) error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/skills:upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func mustZipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return buf.Bytes()
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
	if got.ID != "local.alice" {
		t.Fatalf("template id = %q, want %q", got.ID, "local.alice")
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
	if got.ID != "local.alice" {
		t.Fatalf("template id = %q, want %q", got.ID, "local.alice")
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
		Version:      item.Version,
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
		Version:     item.Version,
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

func TestHandleAgentsCreateReplaceManagerRejectsNonCodexRuntime(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"id":"u-manager","name":"manager","replace":true,"runtime_name":"openclaw","sandbox_enabled":true}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "manager runtime is fixed to codex") {
		t.Fatalf("body = %q, want fixed codex runtime rejection", rec.Body.String())
	}
}

func TestHandleAgentsPatchManagerRejectsNonCodexRuntime(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/u-manager", strings.NewReader(`{"runtime":{"name":"openclaw","sandbox_enabled":true}}`))
	rec := httptest.NewRecorder()

	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "manager runtime is fixed to codex") {
		t.Fatalf("body = %q, want fixed codex runtime rejection", rec.Body.String())
	}
}

func TestHandleAgentsCreateReplaceManagerIgnoresImageAndUsesRuntimeTemplate(t *testing.T) {
	t.Skip("manager is Codex-only; sandbox manager image replacement is obsolete")
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
	hubSvc, err := hub.NewService(config.HubConfig{}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatalf("hub.NewService() error = %v", err)
	}
	openClawTemplate, err := hubSvc.Get(context.Background(), "builtin.openclaw-manager")
	if err != nil {
		t.Fatalf("Get(openclaw-manager) error = %v", err)
	}

	srv := httptest.NewServer((&Handler{svc: svc}).Routes())
	defer srv.Close()

	resp, err := http.Post(
		srv.URL+"/api/v1/agents",
		"application/json",
		strings.NewReader(`{"id":"u-manager","name":"manager","replace":true,"runtime_name":"openclaw","sandbox_enabled":true,"image":"client:must-not-win"}`),
	)
	if err != nil {
		t.Fatalf("POST /api/v1/agents error = %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.StatusCode, http.StatusCreated, string(body))
	}
	var got agent.Agent
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.RuntimeKind != agent.RuntimeKindOpenClawSandbox {
		t.Fatalf("runtime_kind = %q, want %q", got.RuntimeKind, agent.RuntimeKindOpenClawSandbox)
	}
	if got.Image != openClawTemplate.Image {
		t.Fatalf("image = %q, want runtime template image %q", got.Image, openClawTemplate.Image)
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
			{ID: "u-admin", Name: "admin", Role: "admin"},
			{ID: "u-manager", Name: "manager", Role: "manager"},
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
				{ID: "u-admin", Name: "admin"},
				{ID: "manager", Name: "manager"},
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/invite", strings.NewReader(`{"room_id":"room-1","inviter_id":"u-admin","user_ids":["manager"],"locale":"en"}`))
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
	if !containsMember(got.Members, "user-manager") {
		t.Fatalf("members = %+v, want manager to be invited", got.Members)
	}
}

func TestHandleRoomsInviteRequiresRoomID(t *testing.T) {
	srv := &Handler{im: im.NewService()}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/invite", strings.NewReader(`{"inviter_id":"u-admin","user_ids":["manager"]}`))
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
				{ID: "u-alice", Name: "Alice"},
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
			{ID: "u-admin", Name: "admin", Role: "admin"},
			{ID: "u-manager", Name: "manager", Role: "manager"},
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
				{ID: "u-zed", Name: "Zed"},
				{ID: "u-alice", Name: "Alice"},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/users", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []im.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 4 || got[0].Name != "admin" || got[1].Name != "Alice" || got[2].Name != "manager" || got[3].Name != "Zed" {
		t.Fatalf("users = %+v, want admin/Alice/manager/Zed", got)
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

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/users", strings.NewReader(`{"id":"u-alice","name":"Alice","role":"worker"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got im.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "user-alice" || got.Name != "Alice" || got.Role != "worker" {
		t.Fatalf("user = %+v, want normalized provisioned user", got)
	}

	if _, ok := srv.im.User("u-alice"); !ok {
		t.Fatal("User(u-alice) ok = false, want true after create")
	}
	rooms := srv.im.ListRooms()
	if len(rooms) != 1 || !containsMember(rooms[0].Members, "user-admin") || !containsMember(rooms[0].Members, "user-alice") {
		t.Fatalf("rooms = %+v, want one bootstrap room with admin and u-alice", rooms)
	}

	first := mustReceiveIMEvent(t, events)
	if first.Type != im.EventTypeUserCreated || first.User == nil || first.User.ID != "user-alice" {
		t.Fatalf("first event = %+v, want user_created for user-alice", first)
	}
	second := mustReceiveIMEvent(t, events)
	if second.Type != im.EventTypeRoomCreated || second.Room == nil || second.Room.ID == "" {
		t.Fatalf("second event = %+v, want room_created for bootstrap room", second)
	}
}

func TestHandleUsersCreateWithParticipantServiceCreatesWorkerAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))
	previousLocateCodexCLI := locateCodexCLI
	locateCodexCLI = func() (string, error) { return "", errors.New("not installed") }
	t.Cleanup(func() { locateCodexCLI = previousLocateCodexCLI })

	agentSvc := mustNewService(t)
	imSvc := im.NewService()
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()

	participantSvc := participant.NewService(
		participant.NewMemoryStore(nil),
		participant.WithAgentService(agentSvc),
		participant.WithIMService(imSvc),
	)
	srv := &Handler{
		svc:         agentSvc,
		participant: participantSvc,
		im:          imSvc,
		imBus:       bus,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/users", strings.NewReader(`{"id":"u-qa","name":"qa","role":"qa"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got im.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "user-qa" || got.Name != "qa" || got.Role != "worker" {
		t.Fatalf("user = %+v, want qa worker user", got)
	}

	created, ok := agentSvc.Agent("agent-qa")
	if !ok {
		t.Fatal("Agent(agent-qa) ok = false, want worker agent created with IM user")
	}
	if created.Name != "qa" || created.Role != agent.RoleWorker {
		t.Fatalf("agent = %+v, want qa worker", created)
	}
	if created.RuntimeKind != agent.RuntimeKindPicoClawSandbox {
		t.Fatalf("agent runtime kind = %q, want %q", created.RuntimeKind, agent.RuntimeKindPicoClawSandbox)
	}
	if strings.TrimSpace(created.Image) == "" {
		t.Fatal("agent Image = empty, want default sandbox image")
	}

	participants := participantSvc.List(participant.ListOptions{Channel: participant.ChannelCSGClaw, Type: participant.TypeAgent})
	if len(participants) != 1 || participants[0].ID != "pt-qa" || participants[0].AgentID != "agent-qa" || participants[0].ChannelUserRef != "user-qa" {
		t.Fatalf("participants = %+v, want one qa worker participant", participants)
	}

	first := mustReceiveIMEvent(t, events)
	if first.Type != im.EventTypeUserCreated || first.User == nil || first.User.ID != "user-qa" {
		t.Fatalf("first event = %+v, want user_created for user-qa", first)
	}
	second := mustReceiveIMEvent(t, events)
	if second.Type != im.EventTypeRoomCreated || second.Room == nil || !containsMember(second.Room.Members, "user-qa") {
		t.Fatalf("second event = %+v, want qa direct room", second)
	}
}

func TestHandleUsersCreateReusesExistingWorkerParticipant(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: im.AdminUserID,
		Users: []im.User{{
			ID:   im.AdminUserID,
			Name: "admin",
			Role: "admin",
		}, {
			ID:   "user-dahym7",
			Name: "qa",
			Role: "worker",
		}},
		Rooms: []im.Room{{
			ID:       "room-qa",
			Title:    "qa",
			Members:  []string{"user-admin", "user-dahym7"},
			IsDirect: true,
		}},
	})
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "pt-dahym7",
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		Name:            "qa",
		ChannelUserRef:  "user-dahym7",
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		AgentID:         "agent-dahym7",
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}))
	srv := &Handler{
		svc:         &agent.Service{},
		im:          imSvc,
		participant: participantSvc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/users", strings.NewReader(`{"id":"user-dahym7","name":"qa","role":"worker"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got im.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "user-dahym7" || got.Name != "qa" {
		t.Fatalf("user = %+v, want existing qa user", got)
	}
	rooms := imSvc.ListRooms()
	if len(rooms) != 1 || rooms[0].ID != "room-qa" || !containsMember(rooms[0].Members, "user-dahym7") {
		t.Fatalf("rooms = %+v, want existing qa DM preserved", rooms)
	}
	participants := participantSvc.List(participant.ListOptions{Channel: participant.ChannelCSGClaw, Type: participant.TypeAgent})
	if len(participants) != 1 || participants[0].ID != "pt-dahym7" {
		t.Fatalf("participants = %+v, want existing participant only", participants)
	}
}

func TestHandleUsersCreateManagerAgentIDReturnsParticipantUser(t *testing.T) {
	imSvc := im.NewService()
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              agent.ManagerParticipantID,
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		Name:            agent.ManagerName,
		ChannelUserRef:  agent.ManagerParticipantID,
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		AgentID:         agent.ManagerUserID,
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}))
	srv := &Handler{
		im:          imSvc,
		participant: participantSvc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/users", strings.NewReader(`{"id":"u-manager","name":"manager","role":"manager"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got im.User
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != im.ManagerUserID || got.Name != agent.ManagerName {
		t.Fatalf("user = %+v, want existing manager participant user", got)
	}
	if _, ok := imSvc.User(im.ManagerUserID); !ok {
		t.Fatalf("manager user %q missing after create", agent.ManagerUserID)
	}
}

func TestHandleCreateRoomResolvesManagerAgentIDToParticipantUser(t *testing.T) {
	imSvc := im.NewService()
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              agent.ManagerParticipantID,
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		Name:            agent.ManagerName,
		ChannelUserRef:  agent.ManagerParticipantID,
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		AgentID:         agent.ManagerUserID,
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}))
	srv := &Handler{
		im:          imSvc,
		participant: participantSvc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/rooms", strings.NewReader(`{"title":"manager dm","creator_id":"u-admin","member_ids":["u-manager"]}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got im.Room
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !containsMember(got.Members, im.ManagerUserID) || containsMember(got.Members, agent.ManagerUserID) {
		t.Fatalf("room members = %+v, want manager IM user only", got.Members)
	}
}

func TestHandleUsersCreateRejectsMissingID(t *testing.T) {
	srv := &Handler{im: im.NewService()}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/users", strings.NewReader(`{"name":"Alice"}`))
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
				{ID: "u-admin", Name: "admin"},
				{ID: "manager", Name: "manager"},
			},
			Rooms: []im.Room{
				{
					ID:      "room-1",
					Title:   "Room One",
					Members: []string{"u-admin", "manager"},
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
	if got.SenderID != "user-admin" || got.Content != "hello @manager" {
		t.Fatalf("message = %+v, want sender/content populated", got)
	}
	if len(got.Mentions) != 1 || got.Mentions[0].ID != "user-manager" || got.Mentions[0].Name != "manager" {
		t.Fatalf("mentions = %+v, want manager", got.Mentions)
	}
}

func TestHandleMessagesPostNormalizesCanonicalSlashCommand(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin"},
				{ID: "u-manager", Name: "manager"},
			},
			Rooms: []im.Room{{ID: "room-1", Title: "Room One", Members: []string{"u-admin", "u-manager"}}},
		}),
	}

	body := `{"room_id":"room-1","sender_id":"u-admin","content":"  <slash-command arg=\"skill-creator\" name=\"use-skill\"/>  create & review <safely>  "}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got im.Message
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := `<slash-command name="use-skill" arg="skill-creator"></slash-command> create & review <safely>`
	if got.Content != want {
		t.Fatalf("content = %q, want canonical slash command %q", got.Content, want)
	}
}

func TestHandleMessagesPostRejectsMalformedSlashCommand(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users:         []im.User{{ID: "u-admin", Name: "admin"}},
			Rooms:         []im.Room{{ID: "room-1", Title: "Room One", Members: []string{"u-admin"}}},
		}),
	}

	body := `{"room_id":"room-1","sender_id":"u-admin","content":"<slash-command name=\"\"></slash-command> body"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleMessagesPostKeepsLegacySlashTextAsPlainContent(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users:         []im.User{{ID: "u-admin", Name: "admin"}},
			Rooms:         []im.Room{{ID: "room-1", Title: "Room One", Members: []string{"u-admin"}}},
		}),
	}

	body := `{"room_id":"room-1","sender_id":"u-admin","content":"/skill-creator create a review skill"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got im.Message
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Content != "/skill-creator create a review skill" {
		t.Fatalf("content = %q, want legacy slash text kept as plain content", got.Content)
	}
}

func TestHandleThreadRoutesAndMessageFiltering(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin"},
				{ID: "manager", Name: "manager"},
			},
			Rooms: []im.Room{
				{
					ID:      "room-1",
					Title:   "Room One",
					Members: []string{"u-admin", "manager"},
					Messages: []im.Message{
						{ID: "msg-1", SenderID: "u-admin", Content: "before", CreatedAt: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)},
						{ID: "msg-root", SenderID: "u-admin", Content: "root", CreatedAt: time.Date(2026, 5, 20, 9, 1, 0, 0, time.UTC)},
						{ID: "msg-2", SenderID: "manager", Content: "after", CreatedAt: time.Date(2026, 5, 20, 9, 2, 0, 0, time.UTC)},
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

	req = httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"room_id":"room-1","sender_id":"manager","content":"thread reply","relates_to":{"rel_type":"m.thread","event_id":"msg-root"}}`))
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
				{ID: "u-admin", Name: "admin"},
				{ID: "manager", Name: "manager"},
			},
			Rooms: []im.Room{
				{
					ID:       "room-1",
					Title:    "Room One",
					Members:  []string{"u-admin", "manager"},
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

	req = httptest.NewRequest(http.MethodPost, "/api/v1/messages", strings.NewReader(`{"room_id":"room-1","sender_id":"manager","content":"thread reply","relates_to":{"rel_type":"m.thread","event_id":"msg-root"}}`))
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
				{ID: "u-admin", Name: "admin"},
				{ID: "u-dev", Name: "dev"},
				{ID: "u-manager", Name: "manager"},
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
	if got.Content != `<at user_id="user-dev">dev</at> hi` {
		t.Fatalf(`content = %q, want <at user_id="user-dev">dev</at> hi`, got.Content)
	}
	if len(got.Mentions) != 1 || got.Mentions[0].ID != "user-dev" || got.Mentions[0].Name != "dev" {
		t.Fatalf("mentions = %+v, want u-dev", got.Mentions)
	}
}

func TestHandleFeishuMessagesPostSendsMessage(t *testing.T) {
	feishuSvc := feishu.NewServiceWithSendMessage(
		map[string]feishu.AppConfig{"manager": {AppID: "cli_manager", AppSecret: "manager-secret"}},
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

func TestHandleFeishuMessagesPostKeepsCanonicalSlashCommandAsPlainMessage(t *testing.T) {
	wantContent := `<slash-command arg="skill-creator" name="use-skill"/> create & review`
	feishuSvc := feishu.NewServiceWithSendMessage(
		map[string]feishu.AppConfig{"manager": {AppID: "cli_manager", AppSecret: "manager-secret"}},
		func(_ context.Context, _ feishu.AppConfig, req feishu.SendMessageRequest) (feishu.SendMessageResponse, error) {
			if req.ChatID != "oc_alpha" || req.Content != wantContent {
				t.Fatalf("send request = %+v, want chat/content %q", req, wantContent)
			}
			return feishu.SendMessageResponse{MessageID: "om_1", SenderOpenID: "ou_manager"}, nil
		},
	)
	srv := &Handler{feishu: feishuSvc}

	body := `{"room_id":"oc_alpha","sender_id":"u-manager","content":"<slash-command arg=\"skill-creator\" name=\"use-skill\"/> create & review"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got im.Message
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Content != wantContent {
		t.Fatalf("content = %q, want %q", got.Content, wantContent)
	}
}

func TestHandleFeishuMessagesPostKeepsSlashShorthandAsPlainMessage(t *testing.T) {
	wantContent := `/skill-creator create & review`
	feishuSvc := feishu.NewServiceWithSendMessage(
		map[string]feishu.AppConfig{"manager": {AppID: "cli_manager", AppSecret: "manager-secret"}},
		func(_ context.Context, _ feishu.AppConfig, req feishu.SendMessageRequest) (feishu.SendMessageResponse, error) {
			if req.ChatID != "oc_alpha" || req.Content != wantContent {
				t.Fatalf("send request = %+v, want chat/content %q", req, wantContent)
			}
			return feishu.SendMessageResponse{MessageID: "om_1", SenderOpenID: "ou_manager"}, nil
		},
	)
	srv := &Handler{feishu: feishuSvc}

	body := `{"room_id":"oc_alpha","sender_id":"u-manager","content":"/skill-creator create & review"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/feishu/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var got im.Message
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Content != wantContent {
		t.Fatalf("content = %q, want %q", got.Content, wantContent)
	}
}

func TestHandleFeishuMessagesGetListsRoomMessages(t *testing.T) {
	feishuSvc := feishu.NewServiceWithCreateChatAndListRoomMessages(
		map[string]feishu.AppConfig{"manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
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
	if len(got) != 1 || got[0].ID != "om_1" || got[0].SenderID != "manager" {
		t.Fatalf("messages = %+v, want listed feishu messages with bot ids", got)
	}
}

func TestHandleFeishuEventsStreamsMessageBusEvents(t *testing.T) {
	feishuSvc := feishu.NewService()
	srv := &Handler{feishu: feishuSvc, serverAccessToken: "secret"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/participants/u-manager/events", nil).WithContext(ctx)
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
			Content:  "/custom do this",
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
	if !strings.Contains(body, `"content":"/custom do this"`) {
		t.Fatalf("body = %q, want original slash invocation content", body)
	}
	if strings.Contains(body, "agent_content") || strings.Contains(body, "Follow custom rules") {
		t.Fatalf("body = %q, want no hidden skill payload", body)
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/participants/u-manager/events", nil).WithContext(ctx)
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/participants/u-manager/events", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleFeishuEventsRequiresAuthorizationWhenServerAccessTokenEmpty(t *testing.T) {
	srv := &Handler{}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/participants/u-manager/events", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleFeishuEventsSkipsAuthorizationWhenNoAuth(t *testing.T) {
	srv := &Handler{serverNoAuth: true}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/feishu/participants/u-manager/events", nil)
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
		Sender: &im.User{ID: "u-admin", Name: "admin"},
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

func TestHandleIMEventsExposeTeamPayload(t *testing.T) {
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
		Type:   im.EventTypeTeamCreated,
		TeamID: "team-1",
		Team: &apitypes.Team{
			ID:    "team-1",
			Title: "Weather team",
		},
	})
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, `"type":"team.created"`) || !strings.Contains(body, `"team_id":"team-1"`) || !strings.Contains(body, `"title":"Weather team"`) {
		t.Fatalf("body = %q, want team.created payload", body)
	}
}

func TestHandleRoomsPostCreatesRoom(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin"},
				{ID: "u-alice", Name: "Alice"},
				{ID: "manager", Name: "manager"},
			},
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{"title":"Launch","description":"coordination","creator_id":"u-admin","member_ids":["u-alice","manager"],"locale":"en"}`))
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
	if !containsMember(got.Members, "user-admin") || !containsMember(got.Members, "user-alice") || !containsMember(got.Members, "user-manager") {
		t.Fatalf("members = %+v, want admin, alice, and manager", got.Members)
	}
}

func TestHandleRoomsPostUsesCsgclawChannelAdapter(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin"},
				{ID: "u-alice", Name: "Alice"},
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
	if !containsMember(got.Members, "user-admin") || !containsMember(got.Members, "user-alice") {
		t.Fatalf("members = %+v, want trimmed bot IDs", got.Members)
	}
}

func TestHandleUsersDeleteRemovesUser(t *testing.T) {
	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Users: []im.User{
				{ID: "u-admin", Name: "admin", IsOnline: true},
				{ID: "u-alice", Name: "Alice", IsOnline: true},
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

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/csgclaw/users/u-alice", nil)
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

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/channels/csgclaw/users/u-admin", nil)
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
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()

	srv := &Handler{
		im: im.NewServiceFromBootstrap(im.Bootstrap{
			CurrentUserID: "u-admin",
			Rooms: []im.Room{
				{ID: "room-1", Title: "Room One", Members: []string{"u-admin", "u-manager"}},
			},
		}),
		imBus: bus,
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
	event := mustReceiveIMEvent(t, events)
	if event.Type != im.EventTypeRoomDeleted || event.RoomID != "room-1" || event.Room == nil || event.Room.ID != "room-1" {
		t.Fatalf("event = %+v, want room.deleted for room-1", event)
	}
}

func TestHandleFeishuRoomsDeleteRemovesRoom(t *testing.T) {
	deleted := make([]string, 0, 1)
	srv := &Handler{
		feishu: feishu.NewServiceWithDeleteChat(
			map[string]feishu.AppConfig{"manager": {AppID: "cli_manager", AppSecret: "manager-secret", AdminOpenID: "ou_admin"}},
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

func TestHandleParticipantMessageRouteRequiresAuthorization(t *testing.T) {
	srv := &Handler{
		im:                im.NewService(),
		participantBridge: im.NewParticipantBridge("secret"),
		serverAccessToken: "secret",
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants/u-manager/messages", strings.NewReader(`{"room_id":"room-1","text":"hello"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleParticipantMessageRouteRequiresAuthorizationWhenServerAccessTokenEmpty(t *testing.T) {
	srv := &Handler{
		participantBridge: im.NewParticipantBridge("secret"),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants/u-manager/messages", strings.NewReader(`{"room_id":"room-1","text":"hello"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleParticipantMessageRouteSkipsAuthorizationWhenNoAuth(t *testing.T) {
	srv := &Handler{
		participantBridge: im.NewParticipantBridge("secret"),
		serverNoAuth:      true,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants/u-manager/messages", strings.NewReader(`{"room_id":"room-1","text":"hello"}`))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("status = %d, want non-unauthorized when no_auth is true", rec.Code)
	}
}

func TestHandleBotSendMessageRequiresIMService(t *testing.T) {
	srv := &Handler{
		participantBridge: im.NewParticipantBridge(""),
		serverNoAuth:      true,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants/u-manager/messages", strings.NewReader(`{"room_id":"room-1","text":"hello"}`))
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
			{ID: "u-admin", Name: "admin"},
			{ID: "manager", Name: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "manager"},
				Messages: []im.Message{
					{ID: "msg-root", SenderID: "manager", Content: "How can I help?", CreatedAt: now},
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
	bridge := im.NewParticipantBridge("")
	events, cancel := bridge.Subscribe("manager")
	defer cancel()
	bridge.PublishMessageEvent(room, sender, inbound)
	select {
	case evt := <-events:
		if evt.ThreadRootID != "msg-root" {
			t.Fatalf("bot event ThreadRootID = %q, want msg-root", evt.ThreadRootID)
		}
		bridge.Ack("manager", evt.MessageID)
	case <-time.After(time.Second):
		t.Fatal("PublishMessageEvent() timed out waiting for threaded event")
	}

	srv := &Handler{im: imSvc, participantBridge: bridge, serverNoAuth: true}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants/manager/messages", strings.NewReader(`{"room_id":"room-1","text":"thread answer"}`))
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

func TestHandleParticipantSendMessagePreservesMetadata(t *testing.T) {
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: "manager", Name: "manager"},
		},
		Rooms: []im.Room{{
			ID:       "room-1",
			IsDirect: true,
			Members:  []string{"u-admin", "manager"},
		}},
	})

	srv := &Handler{im: imSvc, participantBridge: im.NewParticipantBridge(""), serverNoAuth: true}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants/manager/messages", strings.NewReader(`{
		"room_id": "room-1",
		"text": "Read: from README.md",
		"metadata": {
			"codex": {
				"delivery_kind": "tool",
				"request_id": "msg-user",
				"source_message_id": "msg-user",
				"payload_flags": {"reasoning": true}
			},
			"openclaw": {
				"delivery_kind": "tool",
				"request_id": "msg-user",
				"source_message_id": "msg-user",
				"payload_flags": {"reasoning": true}
			}
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
	openclaw, ok := reply.Metadata["openclaw"].(map[string]any)
	if !ok {
		t.Fatalf("reply.Metadata = %#v, want openclaw object", reply.Metadata)
	}
	if openclaw["delivery_kind"] != "tool" || openclaw["request_id"] != "msg-user" {
		t.Fatalf("openclaw metadata = %#v, want tool delivery for msg-user", openclaw)
	}
	codex, ok := reply.Metadata["codex"].(map[string]any)
	if !ok || codex["delivery_kind"] != "tool" || codex["request_id"] != "msg-user" {
		t.Fatalf("codex metadata = %#v, want tool delivery for msg-user", reply.Metadata["codex"])
	}
	flags, ok := openclaw["payload_flags"].(map[string]any)
	if !ok || flags["reasoning"] != true {
		t.Fatalf("payload_flags = %#v, want reasoning=true", openclaw["payload_flags"])
	}
}

func TestHandleBotSendMessageAcceptsPicoClawThreadContext(t *testing.T) {
	now := time.Now().UTC()
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: "manager", Name: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "manager"},
				Messages: []im.Message{
					{ID: "msg-root", SenderID: "manager", Content: "How can I help?", CreatedAt: now},
				},
			},
		},
	})
	if _, _, err := imSvc.StartThread(im.StartThreadRequest{RoomID: "room-1", RootMessageID: "msg-root"}); err != nil {
		t.Fatalf("StartThread() error = %v", err)
	}

	srv := &Handler{im: imSvc, participantBridge: im.NewParticipantBridge(""), serverNoAuth: true}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants/manager/messages", strings.NewReader(`{
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

func TestHandleParticipantSendMessageReplacementRefreshesThreadRootSummary(t *testing.T) {
	now := time.Now().UTC()
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: "manager", Name: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "manager"},
				Messages: []im.Message{{ID: "msg-user", SenderID: "u-admin", Content: "run it", CreatedAt: now}},
			},
		},
	})
	srv := &Handler{im: imSvc, participantBridge: im.NewParticipantBridge(""), serverNoAuth: true}
	send := func(t *testing.T, body string) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants/manager/messages", strings.NewReader(body))
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
		return sent.MessageID
	}

	rootID := send(t, `{"room_id":"room-1","message_id":"assistant-turn-1","text":"\u200b"}`)
	send(t, `{"room_id":"room-1","message_id":"assistant-turn-1-tool-1","thread_root_id":"assistant-turn-1","text":"tool activity"}`)
	finalID := send(t, `{"room_id":"room-1","message_id":"assistant-turn-1","text":"final answer"}`)
	if rootID != "assistant-turn-1" || finalID != rootID {
		t.Fatalf("root/final message ids = %q / %q, want assistant-turn-1", rootID, finalID)
	}

	timeline, err := imSvc.ListMessages("room-1")
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	var root im.Message
	for _, message := range timeline {
		if message.ID == "assistant-turn-1-tool-1" {
			t.Fatalf("timeline = %+v, want tool reply hidden from top-level messages", timeline)
		}
		if message.ID == rootID {
			root = message
		}
	}
	if root.Content != "final answer" {
		t.Fatalf("root.Content = %q, want final answer", root.Content)
	}
	if root.Thread == nil || root.Thread.Context.RootExcerpt != "final answer" || root.Thread.ReplyCount != 1 {
		t.Fatalf("root.Thread = %+v, want refreshed summary with one reply", root.Thread)
	}
}

func TestHandleParticipantSendMessageKeepsTopLevelToolCallsSeparateFromFinalResponse(t *testing.T) {
	for _, tc := range []struct {
		name     string
		isDirect bool
	}{
		{name: "dm", isDirect: true},
		{name: "room"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
				CurrentUserID: "u-admin",
				Users: []im.User{
					{ID: "u-admin", Name: "admin"},
					{ID: "manager", Name: "manager"},
				},
				Rooms: []im.Room{
					{
						ID:       "room-1",
						IsDirect: tc.isDirect,
						Members:  []string{"u-admin", "manager"},
						Messages: []im.Message{{ID: "msg-user", SenderID: "u-admin", Content: "use some tools", CreatedAt: now}},
					},
				},
			})
			srv := &Handler{im: imSvc, participantBridge: im.NewParticipantBridge(""), serverNoAuth: true}
			send := func(t *testing.T, body string) string {
				t.Helper()
				req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/csgclaw/participants/manager/messages", strings.NewReader(body))
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
				return sent.MessageID
			}

			firstToolID := send(t, `{"room_id":"room-1","text":"🔧 `+"`list_dir`"+`\n`+"```"+`\n{\"path\":\"/workspace\"}\n`+"```"+`"}`)
			secondToolID := send(t, `{"room_id":"room-1","text":"🔧 `+"`exec`"+`\n`+"```"+`\n{\"command\":\"pwd\"}\n`+"```"+`"}`)
			finalID := send(t, `{"room_id":"room-1","text":"Used two tools."}`)
			if finalID == "" || finalID == firstToolID || finalID == secondToolID {
				t.Fatalf("message ids first=%q second=%q final=%q, want final root distinct from tool replies", firstToolID, secondToolID, finalID)
			}

			timeline, err := imSvc.ListMessages("room-1")
			if err != nil {
				t.Fatalf("ListMessages() error = %v", err)
			}
			var firstTool, secondTool, final im.Message
			for _, message := range timeline {
				switch message.ID {
				case firstToolID:
					firstTool = message
				case secondToolID:
					secondTool = message
				case finalID:
					final = message
				}
			}
			if firstTool.ID == "" || secondTool.ID == "" {
				t.Fatalf("timeline = %+v, want tool records kept as top-level activity messages", timeline)
			}
			if final.ID == "" {
				t.Fatalf("timeline = %+v, want final response %q", timeline, finalID)
			}
			if final.Content != "Used two tools." {
				t.Fatalf("final.Content = %q, want final response", final.Content)
			}
			if final.Thread != nil {
				t.Fatalf("final.Thread = %+v, want no synthetic activity thread", final.Thread)
			}
			if firstTool.RelatesTo != nil || secondTool.RelatesTo != nil {
				t.Fatalf("tool relates_to = %+v / %+v, want top-level activity messages", firstTool.RelatesTo, secondTool.RelatesTo)
			}
			for _, tool := range []im.Message{firstTool, secondTool} {
				if !strings.HasPrefix(strings.TrimSpace(tool.Content), "🔧 ") {
					t.Fatalf("tool.Content = %q, want legacy tool call", tool.Content)
				}
			}
			if _, err := imSvc.GetThread("room-1", final.ID); err == nil {
				t.Fatalf("GetThread(%q) unexpectedly succeeded; want no synthetic thread", final.ID)
			}
		})
	}
}

func TestPublishParticipantEventQueuesUntilParticipantSubscribes(t *testing.T) {
	now := time.Now().UTC()
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: "manager", Name: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "manager"},
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
	bridge := im.NewParticipantBridge("")
	srv := &Handler{im: imSvc, participantBridge: bridge}

	sender, ok := imSvc.User("u-admin")
	if !ok {
		t.Fatal("missing sender")
	}
	room, ok := imSvc.Room("room-1")
	if !ok || len(room.Messages) != 1 {
		t.Fatalf("room = %+v, want one message", room)
	}

	srv.PublishParticipantEvent(im.Event{
		Type:    im.EventTypeMessageCreated,
		RoomID:  "room-1",
		Sender:  &sender,
		Message: &room.Messages[0],
	})

	events, cancel := bridge.Subscribe("manager")
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

func TestPublishParticipantEventReensuresRunningWorkerLifecycle(t *testing.T) {
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
			{ID: "u-admin", Name: "admin"},
			{ID: "u-worker", Name: "worker"},
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

	srv := &Handler{svc: svc, im: imSvc, participantBridge: im.NewParticipantBridge("")}
	srv.PublishParticipantEvent(im.Event{
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

func TestPublishParticipantEventStartsStoppedWorker(t *testing.T) {
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
			{ID: "u-admin", Name: "admin"},
			{ID: "u-worker", Name: "worker"},
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

	srv := &Handler{svc: svc, im: imSvc, participantBridge: im.NewParticipantBridge("")}
	srv.PublishParticipantEvent(im.Event{
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
			{ID: "u-admin", Name: "admin"},
			{ID: "manager", Name: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "manager"},
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
	bridge := im.NewParticipantBridge("")
	room, ok := imSvc.Room("room-1")
	if !ok {
		t.Fatal("Room(room-1) = false, want room")
	}
	sender, ok := imSvc.User("u-admin")
	if !ok {
		t.Fatal("User(u-admin) = false, want user")
	}
	bridge.PublishMessageEvent(room, sender, room.Messages[0])

	srv := &Handler{im: imSvc, participantBridge: bridge}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/channels/csgclaw/participants/manager/events", nil)
	srv.handleParticipantEventsStream(&failingBotEventWriter{header: make(http.Header)}, req, "manager")

	events, cancel := bridge.Subscribe("manager")
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
			{ID: "u-admin", Name: "admin"},
			{ID: "manager", Name: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "manager"},
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
	bridge := im.NewParticipantBridge("")
	events, cancel := bridge.Subscribe("manager")
	defer cancel()

	srv := &Handler{im: imSvc, participantBridge: bridge}
	srv.replayRecentParticipantMessages("manager", "")

	select {
	case evt := <-events:
		if evt.MessageID != "msg-missed" || evt.Text != "please reply" {
			t.Fatalf("replayed event = %+v, want msg-missed please reply", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("replayRecentParticipantMessages() timed out waiting for event")
	}
}

func TestReplayRecentBotMessagesSkipsRoomWithoutBridgeTarget(t *testing.T) {
	now := time.Now().UTC()
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: "u-agent-hhtz4b", Name: "qa"},
			{ID: agent.ManagerParticipantID, Name: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-qa",
				IsDirect: true,
				Members:  []string{"u-admin", "u-agent-hhtz4b"},
				Messages: []im.Message{
					{
						ID:        "msg-qa",
						SenderID:  "u-admin",
						Content:   "qa only",
						CreatedAt: now,
					},
				},
			},
		},
	})
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              agent.ManagerParticipantID,
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		Name:            agent.ManagerName,
		ChannelUserRef:  agent.ManagerParticipantID,
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		AgentID:         agent.ManagerUserID,
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}))
	bridge := im.NewParticipantBridge("")
	events, cancel := bridge.Subscribe(agent.ManagerParticipantID)
	defer cancel()

	srv := &Handler{im: imSvc, participant: participantSvc, participantBridge: bridge}
	srv.replayRecentParticipantMessages(agent.ManagerParticipantID, "")

	select {
	case evt := <-events:
		t.Fatalf("replayed event = %+v, want no replay for room without manager membership", evt)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestReplayRecentBotMessagesReplaysParticipantRoomUsingChannelUserRef(t *testing.T) {
	now := time.Now().UTC()
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: "u-agent-hhtz4b", Name: "qa"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-qa",
				IsDirect: true,
				Members:  []string{"u-admin", "u-agent-hhtz4b"},
				Messages: []im.Message{
					{
						ID:        "msg-qa",
						SenderID:  "u-admin",
						Content:   "qa only",
						CreatedAt: now,
					},
				},
			},
		},
	})
	participantSvc := participant.NewService(participant.NewMemoryStore([]apitypes.Participant{{
		ID:              "agent-hhtz4b",
		Channel:         participant.ChannelCSGClaw,
		Type:            participant.TypeAgent,
		Name:            "qa",
		ChannelUserRef:  "u-agent-hhtz4b",
		ChannelUserKind: participant.ChannelUserKindLocalUserID,
		AgentID:         "u-agent-hhtz4b",
		LifecycleStatus: participant.LifecycleStatusActive,
		Mentionable:     true,
	}}))
	bridge := im.NewParticipantBridge("")
	events, cancel := bridge.Subscribe("agent-hhtz4b")
	defer cancel()

	srv := &Handler{im: imSvc, participant: participantSvc, participantBridge: bridge}
	srv.replayRecentParticipantMessages("agent-hhtz4b", "")

	select {
	case evt := <-events:
		if evt.MessageID != "msg-qa" || evt.Context.Account != "pt-hhtz4b" {
			t.Fatalf("replayed event = %+v, want participant-keyed QA replay", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("replayRecentParticipantMessages() timed out waiting for participant-keyed QA event")
	}
}

func TestReplayRecentBotMessagesUsesNewConversationFlow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	restoreDefault := agent.TestOnlySetDefaultServiceOption(func(s *agent.Service) error {
		return agent.WithRuntime(fakeConversationRuntime{
			fakeCompatRuntime: fakeCompatRuntime{kind: agent.RuntimeKindCodex},
			newConversation: func(_ context.Context, _ agentruntime.Handle, req agentruntime.ConversationStartRequest) (agentruntime.ConversationStartAction, error) {
				if strings.TrimSpace(req.Channel) != "csgclaw" {
					t.Fatalf("new conversation request channel = %q, want csgclaw", req.Channel)
				}
				if strings.TrimSpace(req.RoomID) != "room-1" {
					t.Fatalf("new conversation request room_id = %q, want room-1", req.RoomID)
				}
				return agentruntime.ConversationStartAction{
					Mode:         agentruntime.ConversationStartActionBotEvent,
					BotEventText: "ack: cleared",
				}, nil
			},
		})(s)
	})
	t.Cleanup(restoreDefault)

	statePath := filepath.Join(t.TempDir(), "agents.json")
	if err := writeSeededAgents(statePath, []agent.Agent{
		{
			ID:              "u-manager",
			Name:            "manager",
			Role:            agent.RoleManager,
			RuntimeKind:     agent.RuntimeKindCodex,
			RuntimeID:       "rt-u-manager",
			Status:          string(agentruntime.StateRunning),
			ProfileComplete: true,
			CreatedAt:       time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}
	svc, err := agent.NewService(config.ModelConfig{ModelID: "gpt-5.5"}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: "manager", Name: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "manager"},
				Messages: []im.Message{
					{
						ID:        "msg-new-convo",
						SenderID:  "u-admin",
						Content:   `<slash-command name="new" arg="conversation"></slash-command> clear all`,
						CreatedAt: time.Now().UTC(),
					},
				},
			},
		},
	})
	bridge := im.NewParticipantBridge("")
	events, cancel := bridge.Subscribe("manager")
	defer cancel()

	srv := &Handler{svc: svc, im: imSvc, participantBridge: bridge}
	srv.replayRecentParticipantMessages("manager", "")

	select {
	case evt := <-events:
		if evt.MessageID != "msg-new-convo" || evt.Text != "ack: cleared" {
			t.Fatalf("replayed event = %+v, want msg-new-convo ack: cleared", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("replayRecentParticipantMessages() timed out waiting for event")
	}
}

func TestReplayRecentBotMessagesSkipsAnsweredMessage(t *testing.T) {
	now := time.Now().UTC()
	imSvc := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "u-admin",
		Users: []im.User{
			{ID: "u-admin", Name: "admin"},
			{ID: "manager", Name: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "manager"},
				Messages: []im.Message{
					{
						ID:        "msg-answered",
						SenderID:  "u-admin",
						Content:   "please reply",
						CreatedAt: now,
					},
					{
						ID:        "msg-reply",
						SenderID:  "manager",
						Content:   "done",
						CreatedAt: now.Add(time.Second),
					},
				},
			},
		},
	})
	bridge := im.NewParticipantBridge("")
	events, cancel := bridge.Subscribe("manager")
	defer cancel()

	srv := &Handler{im: imSvc, participantBridge: bridge}
	srv.replayRecentParticipantMessages("manager", "")

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
			{ID: "u-admin", Name: "admin"},
			{ID: "manager", Name: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "manager"},
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
	bridge := im.NewParticipantBridge("")
	events, cancel := bridge.Subscribe("manager")
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
		bridge.Ack("manager", evt.MessageID)
	case <-time.After(time.Second):
		t.Fatal("PublishMessageEvent() timed out waiting for event")
	}

	srv := &Handler{im: imSvc, participantBridge: bridge}
	srv.replayRecentParticipantMessages("manager", "")

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
			{ID: "u-admin", Name: "admin"},
			{ID: "manager", Name: "manager"},
		},
		Rooms: []im.Room{
			{
				ID:       "room-1",
				IsDirect: true,
				Members:  []string{"u-admin", "manager"},
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
	bridge := im.NewParticipantBridge("")
	events, cancel := bridge.Subscribe("manager")
	defer cancel()

	srv := &Handler{im: imSvc, participantBridge: bridge}
	srv.replayRecentParticipantMessages("manager", "msg-seen")

	select {
	case evt := <-events:
		if evt.MessageID != "msg-new" || evt.Text != "new after reconnect" {
			t.Fatalf("replayed event = %+v, want msg-new new after reconnect", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("replayRecentParticipantMessages() timed out waiting for event")
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
		participantBridge: im.NewParticipantBridge("secret"),
		llm:               bridge,
		serverAccessToken: "secret",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-manager/llm/v1/models", nil)
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
		participantBridge: im.NewParticipantBridge("secret"),
		llm:               bridge,
		serverAccessToken: "secret",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/u-manager/llm/models", nil)
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

func mustNewSeededServiceWithOptions(t *testing.T, agents []agent.Agent, opts ...agent.ServiceOption) *agent.Service {
	t.Helper()

	svc, _ := mustNewSeededServiceWithPathAndOptions(t, agents, opts...)
	return svc
}

func mustNewSeededServiceWithPath(t *testing.T, agents []agent.Agent) (*agent.Service, string) {
	t.Helper()

	return mustNewSeededServiceWithPathAndOptions(t, agents)
}

func mustNewSeededServiceWithPathAndOptions(t *testing.T, agents []agent.Agent, opts ...agent.ServiceOption) (*agent.Service, string) {
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

	serviceOpts := []agent.ServiceOption{
		agent.WithHubService(hubSvc),
		agent.WithBootstrapDefaultTemplates(config.BootstrapConfig{
			DefaultManagerTemplate: config.DefaultBootstrapManagerTemplate,
			DefaultWorkerTemplate:  config.DefaultBootstrapWorkerTemplate,
		}),
	}
	serviceOpts = append(serviceOpts, opts...)

	svc, err := agent.NewService(config.ModelConfig{
		Provider: config.ProviderLLMAPI,
		BaseURL:  "http://127.0.0.1:4000",
		APIKey:   "sk-test",
		ModelID:  "model-1",
	}, config.ServerConfig{}, "manager-image:test", statePath, serviceOpts...)
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
		case agent.RoleManager:
			out[i].RuntimeKind = agent.RuntimeKindCodex
		case agent.RoleWorker:
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
