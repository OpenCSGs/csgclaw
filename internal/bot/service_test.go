package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/app/runtimewiring"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	"csgclaw/internal/hub"
	"csgclaw/internal/im"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/sandboxgateway"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/sandboxtest"
)

var (
	fakeBotRuntimeStateMu sync.RWMutex
	fakeBotRuntimeStates  = make(map[string]agentruntime.Info)
)

func init() {
	_ = agent.TestOnlySetResponsesAPIProbe(func(context.Context, string, string, string, map[string]string) error {
		return nil
	})
}

type fakeBotAgentRuntime struct {
	kind string
}

func (f fakeBotAgentRuntime) Kind() string {
	return f.kind
}

func (f fakeBotAgentRuntime) New(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
	return agentruntime.Handle{
		RuntimeID: spec.RuntimeID,
		HandleID:  fmt.Sprintf("%s-%s", f.kind, spec.AgentName),
	}, nil
}

func (f fakeBotAgentRuntime) Provision(_ context.Context, req agentruntime.ProvisionRequest) error {
	if strings.TrimSpace(req.WorkspaceOverlay) == "" {
		return nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	workspaceRoot := filepath.Join(homeDir, config.AppDirName, "agents", req.AgentName, "workspace")
	return sandboxgateway.OverlayWorkspaceTree(req.WorkspaceOverlay, workspaceRoot)
}

func (f fakeBotAgentRuntime) Start(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateRunning, nil
}

func (f fakeBotAgentRuntime) Stop(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateStopped, nil
}

func (f fakeBotAgentRuntime) Delete(context.Context, agentruntime.Handle) error {
	return nil
}

func (f fakeBotAgentRuntime) State(_ context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	return fakeBotRuntimeInfoForHandle(h).State, nil
}

func (f fakeBotAgentRuntime) Info(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
	return fakeBotRuntimeInfoForHandle(h), nil
}

func (f fakeBotAgentRuntime) EnsureGatewayConfig(agentName, botID, modelID string) error {
	return nil
}

func (f fakeBotAgentRuntime) ProjectsGuestPath() string {
	return "/projects"
}

func (f fakeBotAgentRuntime) CreateGatewayBox(_ context.Context, _ sandbox.Runtime, _ string, name, _ string, _ agentruntime.Profile) (sandbox.Instance, sandbox.Info, error) {
	info := sandbox.Info{
		ID:        "box-" + name,
		Name:      name,
		State:     sandbox.StateRunning,
		CreatedAt: time.Now().UTC(),
	}
	return sandboxtest.NewInstance(info), info, nil
}

func (f fakeBotAgentRuntime) GatewayCreateSpec(image, name, _ string, _ agentruntime.Profile) (sandbox.CreateSpec, error) {
	return sandbox.CreateSpec{Image: image, Name: name}, nil
}

func fakeBotRuntimeInfoForHandle(h agentruntime.Handle) agentruntime.Info {
	fakeBotRuntimeStateMu.RLock()
	defer fakeBotRuntimeStateMu.RUnlock()
	if info, ok := fakeBotRuntimeStates[strings.TrimSpace(h.HandleID)]; ok {
		return info
	}
	if info, ok := fakeBotRuntimeStates[strings.TrimSpace(h.RuntimeID)]; ok {
		return info
	}
	return agentruntime.Info{
		HandleID:  strings.TrimSpace(h.HandleID),
		State:     agentruntime.StateRunning,
		CreatedAt: time.Now().UTC(),
	}
}

func resetFakeBotRuntimeStates() {
	fakeBotRuntimeStateMu.Lock()
	defer fakeBotRuntimeStateMu.Unlock()
	clear(fakeBotRuntimeStates)
}

func setFakeBotRuntimeState(runtimeID, handleID string, state sandbox.State) {
	info := agentruntime.Info{
		HandleID:  strings.TrimSpace(handleID),
		State:     agentruntime.State(state),
		CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
	}
	fakeBotRuntimeStateMu.Lock()
	defer fakeBotRuntimeStateMu.Unlock()
	if runtimeID = strings.TrimSpace(runtimeID); runtimeID != "" {
		fakeBotRuntimeStates[runtimeID] = info
	}
	if handleID = strings.TrimSpace(handleID); handleID != "" {
		fakeBotRuntimeStates[handleID] = info
	}
}

func testRuntimeIDForAgentName(agentName string) string {
	agentName = strings.TrimSpace(agentName)
	if strings.EqualFold(agentName, agent.ManagerName) {
		return "rt-" + agent.ManagerUserID
	}
	if agentName == "" {
		return ""
	}
	return "rt-u-" + agentName
}

func init() {
	agent.TestOnlySetDefaultServiceOption(func(s *agent.Service) error {
		if err := runtimewiring.WithPicoClawSandboxRuntime(nil)(s); err != nil {
			return err
		}
		return runtimewiring.WithOpenClawSandboxRuntime()(s)
	})
}

func testAgentModelConfig() config.ModelConfig {
	return config.ModelConfig{
		Provider: config.ProviderLLMAPI,
		BaseURL:  "http://127.0.0.1:4000",
		APIKey:   "sk-test",
		ModelID:  "default-model",
	}
}

func TestServiceListReturnsAllWhenChannelEmpty(t *testing.T) {
	svc := mustNewBotService(t, []Bot{
		{
			ID:        "bot-csgclaw",
			Name:      "CSGClaw Bot",
			Role:      string(RoleWorker),
			Channel:   string(ChannelCSGClaw),
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:        "bot-feishu",
			Name:      "Feishu Bot",
			Role:      string(RoleWorker),
			Channel:   string(ChannelFeishu),
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})

	got, err := svc.List("", "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List() len = %d, want 2", len(got))
	}
	if got[0].ID != "bot-csgclaw" || got[1].ID != "bot-feishu" {
		t.Fatalf("List() = %+v, want all bots in store order", got)
	}
}

func TestServiceListFiltersByNormalizedChannel(t *testing.T) {
	svc := mustNewBotService(t, []Bot{
		{
			ID:        "bot-csgclaw",
			Name:      "CSGClaw Bot",
			Role:      string(RoleWorker),
			Channel:   string(ChannelCSGClaw),
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:        "bot-feishu",
			Name:      "Feishu Bot",
			Role:      string(RoleWorker),
			Channel:   string(ChannelFeishu),
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})

	got, err := svc.List(" FEISHU ", "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "bot-feishu" {
		t.Fatalf("List(FEISHU) = %+v, want only bot-feishu", got)
	}
}

func TestServiceListFiltersByNormalizedRole(t *testing.T) {
	svc := mustNewBotService(t, []Bot{
		{
			ID:        "bot-manager",
			Name:      "Manager Bot",
			Role:      string(RoleManager),
			Channel:   string(ChannelCSGClaw),
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:        "bot-worker",
			Name:      "Worker Bot",
			Role:      string(RoleWorker),
			Channel:   string(ChannelCSGClaw),
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})

	got, err := svc.List("", " WORKER ")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "bot-worker" {
		t.Fatalf("List(role=WORKER) = %+v, want only bot-worker", got)
	}
}

func TestServiceListFiltersByChannelAndRole(t *testing.T) {
	svc := mustNewBotService(t, []Bot{
		{
			ID:        "bot-csgclaw-manager",
			Name:      "CSGClaw Manager",
			Role:      string(RoleManager),
			Channel:   string(ChannelCSGClaw),
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:        "bot-feishu-manager",
			Name:      "Feishu Manager",
			Role:      string(RoleManager),
			Channel:   string(ChannelFeishu),
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:        "bot-feishu-worker",
			Name:      "Feishu Worker",
			Role:      string(RoleWorker),
			Channel:   string(ChannelFeishu),
			CreatedAt: time.Date(2026, 4, 12, 11, 0, 0, 0, time.UTC),
		},
	})

	got, err := svc.List("feishu", "manager")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "bot-feishu-manager" {
		t.Fatalf("List(feishu, manager) = %+v, want only bot-feishu-manager", got)
	}
}

func TestServiceListFeishuIncludesConfiguredUnavailableBots(t *testing.T) {
	store, err := NewMemoryStore([]Bot{
		{
			ID:        "u-worker",
			Name:      "Worker",
			Role:      string(RoleWorker),
			Channel:   string(ChannelFeishu),
			AgentID:   "u-worker",
			UserID:    "ou_worker",
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	feishuSvc := feishu.NewServiceWithBotOpenIDResolver(
		map[string]feishu.AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
			"u-worker":  {AppID: "cli_worker", AppSecret: "worker-secret"},
		},
		func(_ context.Context, app feishu.AppConfig) (feishu.BotInfo, error) {
			switch app.AppID {
			case "cli_manager":
				return feishu.BotInfo{OpenID: "ou_manager", AppName: "Manager Bot"}, nil
			case "cli_worker":
				return feishu.BotInfo{OpenID: "ou_worker", AppName: "Worker Bot"}, nil
			default:
				t.Fatalf("unexpected app_id %q", app.AppID)
				return feishu.BotInfo{}, nil
			}
		},
	)
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:           "u-worker",
			Name:         "worker",
			Role:         agent.RoleWorker,
			CreatedAt:    time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
			AgentProfile: agent.AgentProfile{ModelID: "default-model"},
		},
	})
	svc, err := NewServiceWithDependencies(store, agentSvc, nil, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}
	mustSetAgentRuntimeState(t, "worker", "box-worker", sandbox.StateRunning)

	got, err := svc.List(string(ChannelFeishu), "")
	if err != nil {
		t.Fatalf("List(feishu) error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List(feishu) = %+v, want stored bot plus configured manager", got)
	}
	if got[0].ID != "u-worker" || !got[0].Available {
		t.Fatalf("stored bot = %+v, want available u-worker", got[0])
	}
	if got[1].ID != "u-manager" || got[1].Name != "Manager Bot" || got[1].Role != string(RoleManager) || got[1].AgentID != "" || got[1].UserID != "ou_manager" || got[1].Available {
		t.Fatalf("configured bot = %+v, want unavailable manager with configured id and open_id", got[1])
	}
}

func TestServiceListFeishuConfiguredBotUsesMatchingAgent(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:           "u-manager",
			Name:         "manager",
			Role:         agent.RoleManager,
			CreatedAt:    time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
			AgentProfile: agent.AgentProfile{ModelID: "default-model"},
		},
	})
	feishuSvc := feishu.NewServiceWithBotOpenIDResolver(
		map[string]feishu.AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
		},
		func(_ context.Context, app feishu.AppConfig) (feishu.BotInfo, error) {
			if app.AppID != "cli_manager" {
				t.Fatalf("unexpected app_id %q", app.AppID)
			}
			return feishu.BotInfo{OpenID: "ou_manager", AppName: "Manager Bot"}, nil
		},
	)
	svc, err := NewServiceWithDependencies(store, agentSvc, nil, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}
	mustSetAgentRuntimeState(t, "manager", "box-manager", sandbox.StateRunning)

	got, err := svc.List(string(ChannelFeishu), "")
	if err != nil {
		t.Fatalf("List(feishu) error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List(feishu) = %+v, want configured manager", got)
	}
	if got[0].ID != "u-manager" || got[0].AgentID != "u-manager" || !got[0].Available {
		t.Fatalf("configured bot = %+v, want configured id u-manager bound to available u-manager agent", got[0])
	}
}

func TestServiceListStoredBotUnavailableWhenAgentNotRunning(t *testing.T) {
	store, err := NewMemoryStore([]Bot{
		{
			ID:        "u-worker",
			Name:      "Worker",
			Role:      string(RoleWorker),
			Channel:   string(ChannelCSGClaw),
			AgentID:   "u-worker",
			UserID:    "u-worker",
			Available: true,
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:           "u-worker",
			Name:         "worker",
			Role:         agent.RoleWorker,
			CreatedAt:    time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
			AgentProfile: agent.AgentProfile{ModelID: "default-model"},
		},
	})
	svc, err := NewServiceWithDependencies(store, agentSvc, nil)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}
	mustSetAgentRuntimeState(t, "worker", "box-worker", sandbox.StateStopped)

	got, err := svc.List(string(ChannelCSGClaw), "")
	if err != nil {
		t.Fatalf("List(csgclaw) error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List(csgclaw) = %+v, want one stored bot", got)
	}
	if got[0].AgentID != "u-worker" || got[0].Available {
		t.Fatalf("stored bot = %+v, want unavailable bot bound to stopped agent", got[0])
	}
}

func TestServiceListWithoutFiltersRefreshesAvailability(t *testing.T) {
	store, err := NewMemoryStore([]Bot{
		{
			ID:        "u-worker",
			Name:      "Worker",
			Role:      string(RoleWorker),
			Channel:   string(ChannelCSGClaw),
			AgentID:   "u-worker",
			UserID:    "u-worker",
			Available: true,
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:           "u-worker",
			Name:         "worker",
			Role:         agent.RoleWorker,
			CreatedAt:    time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
			AgentProfile: agent.AgentProfile{ModelID: "default-model"},
		},
	})
	svc, err := NewServiceWithDependencies(store, agentSvc, nil)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}
	mustSetAgentRuntimeState(t, "worker", "box-worker", sandbox.StateStopped)

	got, err := svc.List("", "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List() = %+v, want one stored bot", got)
	}
	if got[0].Available {
		t.Fatalf("stored bot = %+v, want unavailable bot in unfiltered list", got[0])
	}
}

func TestServiceListFeishuConfiguredBotsRespectRoleFilter(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	feishuSvc := feishu.NewServiceWithBotOpenIDResolver(
		map[string]feishu.AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
			"u-worker":  {AppID: "cli_worker", AppSecret: "worker-secret"},
		},
		func(_ context.Context, app feishu.AppConfig) (feishu.BotInfo, error) {
			switch app.AppID {
			case "cli_manager":
				return feishu.BotInfo{OpenID: "ou_manager", AppName: "Manager Bot"}, nil
			case "cli_worker":
				return feishu.BotInfo{OpenID: "ou_worker", AppName: "Worker Bot"}, nil
			default:
				t.Fatalf("unexpected app_id %q", app.AppID)
				return feishu.BotInfo{}, nil
			}
		},
	)
	svc, err := NewServiceWithDependencies(store, nil, nil, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.List(string(ChannelFeishu), string(RoleManager))
	if err != nil {
		t.Fatalf("List(feishu, manager) error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "u-manager" || got[0].Name != "Manager Bot" || got[0].UserID != "ou_manager" || got[0].AgentID != "" || got[0].Available {
		t.Fatalf("List(feishu, manager) = %+v, want unavailable configured manager only", got)
	}
}

func TestServiceListRejectsInvalidChannel(t *testing.T) {
	svc := mustNewBotService(t, nil)

	_, err := svc.List("slack", "")
	if err == nil || !strings.Contains(err.Error(), "channel must be one of") {
		t.Fatalf("List(slack) error = %v, want channel validation error", err)
	}
}

func TestServiceListRejectsInvalidRole(t *testing.T) {
	svc := mustNewBotService(t, nil)

	_, err := svc.List("", "agent")
	if err == nil || !strings.Contains(err.Error(), "role must be one of") {
		t.Fatalf("List(role=agent) error = %v, want role validation error", err)
	}
}

func TestServiceDeleteRemovesBotForChannel(t *testing.T) {
	svc := mustNewBotService(t, []Bot{
		{
			ID:        "u-alice",
			Name:      "Alice",
			Role:      string(RoleWorker),
			Channel:   string(ChannelCSGClaw),
			AgentID:   "u-alice",
			UserID:    "u-alice",
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
		{
			ID:        "u-alice",
			Name:      "Alice",
			Role:      string(RoleWorker),
			Channel:   string(ChannelFeishu),
			AgentID:   "u-alice",
			UserID:    "u-alice",
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})

	if err := svc.Delete(context.Background(), "feishu", "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	feishuBots, err := svc.List("feishu", "")
	if err != nil {
		t.Fatalf("List(feishu) error = %v", err)
	}
	if len(feishuBots) != 0 {
		t.Fatalf("List(feishu) = %+v, want deleted", feishuBots)
	}
	csgclawBots, err := svc.List("csgclaw", "")
	if err != nil {
		t.Fatalf("List(csgclaw) error = %v", err)
	}
	if len(csgclawBots) != 1 || csgclawBots[0].ID != "u-alice" {
		t.Fatalf("List(csgclaw) = %+v, want retained u-alice", csgclawBots)
	}
}

func TestServiceDeleteRemovesCSGClawUser(t *testing.T) {
	store, err := NewMemoryStore([]Bot{
		{
			ID:        "u-alice",
			Name:      "Alice",
			Role:      string(RoleWorker),
			Channel:   string(ChannelCSGClaw),
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
	svc, err := NewServiceWithDependencies(store, nil, imSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	if err := svc.Delete(context.Background(), "csgclaw", "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	bots, err := svc.List("csgclaw", "")
	if err != nil {
		t.Fatalf("List(csgclaw) error = %v", err)
	}
	if len(bots) != 0 {
		t.Fatalf("List(csgclaw) = %+v, want deleted", bots)
	}
	if _, ok := imSvc.User("u-alice"); ok {
		t.Fatal("User(u-alice) ok = true, want false after bot delete")
	}
	if _, ok := imSvc.Room("room-1"); ok {
		t.Fatal("Room(room-1) ok = true, want false after removing DM user")
	}
}

func TestServiceDeleteRemovesBackingAgentWhenLastReference(t *testing.T) {
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:        "u-alice",
			Name:      "alice",
			Role:      agent.RoleWorker,
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
	})
	store, err := NewMemoryStore([]Bot{
		{
			ID:        "u-alice",
			Name:      "Alice",
			Role:      string(RoleWorker),
			Channel:   string(ChannelFeishu),
			AgentID:   "u-alice",
			UserID:    "u-alice",
			CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, nil)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	if err := svc.Delete(context.Background(), "feishu", "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := agentSvc.Agent("u-alice"); ok {
		t.Fatal("Agent() ok = true, want false after deleting last bot reference")
	}
}

func TestNewServiceRequiresStore(t *testing.T) {
	_, err := NewService(nil)
	if err == nil || !strings.Contains(err.Error(), "bot store is required") {
		t.Fatalf("NewService(nil) error = %v, want store required", err)
	}
}

func TestServiceCreateCSGClawWorkerCreatesAgentUserAndBot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))
	resetFakeBotRuntimeStates()
	t.Cleanup(resetFakeBotRuntimeStates)
	restoreDefault := agent.TestOnlySetDefaultServiceOption(func(s *agent.Service) error {
		if err := agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindPicoClawSandbox})(s); err != nil {
			return err
		}
		if err := agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindOpenClawSandbox})(s); err != nil {
			return err
		}
		return agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindCodex})(s)
	})
	t.Cleanup(restoreDefault)

	agentSvc, err := agent.NewService(testAgentModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}
	imSvc := im.NewService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, imSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}
	bus := im.NewBus()
	events, cancel := bus.Subscribe()
	defer cancel()
	svc.SetIMBus(bus)

	got, err := svc.Create(context.Background(), CreateRequest{
		Name:        "alice",
		Description: "test lead",
		Image:       "agent-image:1",
		Role:        string(RoleWorker),
		Channel:     string(ChannelCSGClaw),
		RuntimeKind: agent.RuntimeKindCodex,
		AgentProfile: &apitypes.CreateAgentProfile{
			Provider:        agent.ProviderCSGHubLite,
			ModelID:         "glm-4.5",
			ReasoningEffort: "high",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID != "u-alice" || got.AgentID != "u-alice" || got.UserID != "u-alice" {
		t.Fatalf("Create() = %+v, want u-alice bot/agent/user IDs", got)
	}
	if got.Role != string(RoleWorker) || got.Channel != string(ChannelCSGClaw) {
		t.Fatalf("Create() = %+v, want worker csgclaw", got)
	}
	if got.Description != "test lead" {
		t.Fatalf("Create().Description = %q, want test lead", got.Description)
	}
	createdAgent, ok := agentSvc.Agent("u-alice")
	if !ok {
		t.Fatal("agent u-alice not created")
	}
	if createdAgent.RuntimeKind != agent.RuntimeKindCodex {
		t.Fatalf("agent.RuntimeKind = %q, want %q", createdAgent.RuntimeKind, agent.RuntimeKindCodex)
	}
	if createdAgent.Image != "agent-image:1" {
		t.Fatalf("agent.Image = %q, want agent-image:1", createdAgent.Image)
	}
	if createdAgent.AgentProfile.Provider != agent.ProviderCSGHubLite || createdAgent.AgentProfile.ModelID != "glm-4.5" {
		t.Fatalf("agent profile = %s/%s, want csghub_lite/glm-4.5", createdAgent.AgentProfile.Provider, createdAgent.AgentProfile.ModelID)
	}
	if createdAgent.AgentProfile.ReasoningEffort != "high" {
		t.Fatalf("agent reasoning = %q, want high", createdAgent.AgentProfile.ReasoningEffort)
	}
	users := imSvc.ListUsers()
	if !containsUser(users, "u-alice") {
		t.Fatalf("users = %+v, want u-alice", users)
	}
	rooms := imSvc.ListRooms()
	if len(rooms) != 1 || !containsMember(rooms[0].Members, "u-admin") || !containsMember(rooms[0].Members, "u-alice") {
		t.Fatalf("rooms = %+v, want one bootstrap room with admin and u-alice", rooms)
	}
	first := mustReceiveEventWithin(t, events, time.Second)
	if first.Type != im.EventTypeUserCreated || first.User == nil || first.User.ID != "u-alice" {
		t.Fatalf("first event = %+v, want user_created for u-alice", first)
	}
	second := mustReceiveEventWithin(t, events, time.Second)
	if second.Type != im.EventTypeRoomCreated || second.Room == nil {
		t.Fatalf("second event = %+v, want room_created with room payload", second)
	}
	third := mustReceiveEventWithin(t, events, 2*time.Second)
	if third.Type != im.EventTypeMessageCreated || third.Message == nil {
		t.Fatalf("third event = %+v, want bootstrap message", third)
	}
	listed, err := svc.List(string(ChannelCSGClaw), "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "u-alice" {
		t.Fatalf("List(csgclaw) = %+v, want u-alice", listed)
	}
	if listed[0].Description != "test lead" {
		t.Fatalf("List(csgclaw)[0].Description = %q, want test lead", listed[0].Description)
	}
}

func TestServiceCreateFeishuWorkerCreatesAgentUserAndBot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))
	resetFakeBotRuntimeStates()
	t.Cleanup(resetFakeBotRuntimeStates)
	restoreDefault := agent.TestOnlySetDefaultServiceOption(func(s *agent.Service) error {
		if err := agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindPicoClawSandbox})(s); err != nil {
			return err
		}
		if err := agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindOpenClawSandbox})(s); err != nil {
			return err
		}
		return agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindCodex})(s)
	})
	t.Cleanup(restoreDefault)

	agentSvc, err := agent.NewService(testAgentModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}
	feishuSvc := feishu.NewService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, nil, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.Create(context.Background(), CreateRequest{
		Name:        "alice",
		Description: "test lead",
		Role:        string(RoleWorker),
		Channel:     string(ChannelFeishu),
		RuntimeKind: agent.RuntimeKindCodex,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID != "u-alice" || got.AgentID != "u-alice" || got.UserID != "u-alice" {
		t.Fatalf("Create() = %+v, want u-alice bot/agent/user IDs", got)
	}
	if got.Role != string(RoleWorker) || got.Channel != string(ChannelFeishu) {
		t.Fatalf("Create() = %+v, want worker feishu", got)
	}
	if _, ok := agentSvc.Agent("u-alice"); !ok {
		t.Fatal("agent u-alice not created")
	}
	if !containsUser(feishuSvc.ListUsers(), "u-alice") {
		t.Fatalf("feishu users = %+v, want u-alice", feishuSvc.ListUsers())
	}
	listed, err := svc.List(string(ChannelFeishu), "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "u-alice" {
		t.Fatalf("List(feishu) = %+v, want u-alice", listed)
	}
}

func TestServiceCreateWorkerReusesAgentAcrossChannels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	provider := sandboxtest.NewProvider()
	t.Cleanup(agent.TestOnlySetSandboxProvider(provider))
	resetFakeBotRuntimeStates()
	t.Cleanup(resetFakeBotRuntimeStates)
	restoreDefault := agent.TestOnlySetDefaultServiceOption(func(s *agent.Service) error {
		if err := agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindPicoClawSandbox})(s); err != nil {
			return err
		}
		if err := agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindOpenClawSandbox})(s); err != nil {
			return err
		}
		return agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindCodex})(s)
	})
	t.Cleanup(restoreDefault)

	agentSvc, err := agent.NewService(testAgentModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}
	imSvc := im.NewService()
	feishuSvc := feishu.NewService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, imSvc, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	csgclawBot, err := svc.Create(context.Background(), CreateRequest{
		Name:        "alice",
		Role:        string(RoleWorker),
		Channel:     string(ChannelCSGClaw),
		RuntimeKind: agent.RuntimeKindCodex,
	})
	if err != nil {
		t.Fatalf("Create(csgclaw worker) error = %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	feishuBot, err := svc.Create(context.Background(), CreateRequest{
		Name:        "alice",
		Role:        string(RoleWorker),
		Channel:     string(ChannelFeishu),
		RuntimeKind: agent.RuntimeKindCodex,
	})
	if err != nil {
		t.Fatalf("Create(feishu worker) error = %v", err)
	}
	if csgclawBot.ID != "u-alice" || feishuBot.ID != "u-alice" || csgclawBot.AgentID != feishuBot.AgentID {
		t.Fatalf("created bots = %+v / %+v, want shared u-alice agent", csgclawBot, feishuBot)
	}
	if !feishuBot.CreatedAt.After(csgclawBot.CreatedAt) {
		t.Fatalf("created_at = %v / %v, want feishu bot time after csgclaw channel user time", csgclawBot.CreatedAt, feishuBot.CreatedAt)
	}
	if !containsUser(imSvc.ListUsers(), "u-alice") {
		t.Fatalf("im users = %+v, want u-alice", imSvc.ListUsers())
	}
	if !containsUser(feishuSvc.ListUsers(), "u-alice") {
		t.Fatalf("feishu users = %+v, want u-alice", feishuSvc.ListUsers())
	}
	all, err := svc.List("", "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List() = %+v, want two channel bindings", all)
	}
	if _, ok := agentSvc.Agent("u-alice"); !ok {
		t.Fatal("Agent(u-alice) ok = false, want shared backing agent")
	}
}

func TestServiceCreateWorkerRejectsDuplicateNameInSameChannel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	provider := sandboxtest.NewProvider()
	t.Cleanup(agent.TestOnlySetSandboxProvider(provider))
	resetFakeBotRuntimeStates()
	t.Cleanup(resetFakeBotRuntimeStates)
	restoreDefault := agent.TestOnlySetDefaultServiceOption(func(s *agent.Service) error {
		if err := agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindPicoClawSandbox})(s); err != nil {
			return err
		}
		if err := agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindOpenClawSandbox})(s); err != nil {
			return err
		}
		return agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindCodex})(s)
	})
	t.Cleanup(restoreDefault)

	agentSvc, err := agent.NewService(testAgentModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}
	imSvc := im.NewService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, imSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	if _, err := svc.Create(context.Background(), CreateRequest{
		Name:        "alice",
		Role:        string(RoleWorker),
		Channel:     string(ChannelCSGClaw),
		RuntimeKind: agent.RuntimeKindCodex,
	}); err != nil {
		t.Fatalf("first Create(worker) error = %v", err)
	}

	_, err = svc.Create(context.Background(), CreateRequest{
		Name:        "alice",
		Role:        string(RoleWorker),
		Channel:     string(ChannelCSGClaw),
		RuntimeKind: agent.RuntimeKindCodex,
	})
	if err == nil || !strings.Contains(err.Error(), `bot name "alice" already exists in channel "csgclaw"`) {
		t.Fatalf("second Create(worker) error = %v, want duplicate name error", err)
	}
}

func TestServiceCreateWorkerUsesFromTemplateWorkspace(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	resetFakeBotRuntimeStates()
	t.Cleanup(resetFakeBotRuntimeStates)
	restoreDefault := agent.TestOnlySetDefaultServiceOption(func(s *agent.Service) error {
		return agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindPicoClawSandbox})(s)
	})
	t.Cleanup(restoreDefault)

	hubSvc := mustNewBotLocalTemplateHubService(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		RuntimeKind: agent.RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
	})
	agentSvc, err := agent.NewService(
		testAgentModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		agent.WithHubService(hubSvc),
	)
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}
	imSvc := im.NewService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, imSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	if _, err := svc.Create(context.Background(), CreateRequest{
		Name:         "alice",
		Role:         string(RoleWorker),
		Channel:      string(ChannelCSGClaw),
		FromTemplate: "local/frontend-worker",
	}); err != nil {
		t.Fatalf("Create(worker) error = %v", err)
	}

	skillPath := filepath.Join(homeDir, config.AppDirName, "agents", "alice", "workspace", "skills", "custom", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("template skill missing after bot create: %v", err)
	}
}

func TestServiceCreateCSGClawManagerBindsBootstrappedAgent(t *testing.T) {
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:           agent.ManagerUserID,
			Name:         agent.ManagerName,
			Role:         agent.RoleManager,
			CreatedAt:    time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
			AgentProfile: agent.AgentProfile{ModelID: "default-model"},
		},
	})
	imSvc := im.NewService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, imSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.Create(context.Background(), CreateRequest{
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelCSGClaw),
	})
	if err != nil {
		t.Fatalf("Create(manager) error = %v", err)
	}
	if got.ID != agent.ManagerUserID || got.AgentID != agent.ManagerUserID || got.UserID != agent.ManagerUserID {
		t.Fatalf("Create(manager) = %+v, want u-manager IDs", got)
	}
	if got.Role != string(RoleManager) || got.Channel != string(ChannelCSGClaw) {
		t.Fatalf("Create(manager) = %+v, want manager csgclaw", got)
	}
	if !containsUser(imSvc.ListUsers(), agent.ManagerUserID) {
		t.Fatalf("users = %+v, want u-manager", imSvc.ListUsers())
	}
}

func TestServiceCreateManagerBindsSameAgentAcrossChannels(t *testing.T) {
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:           agent.ManagerUserID,
			Name:         agent.ManagerName,
			Role:         agent.RoleManager,
			CreatedAt:    time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
			AgentProfile: agent.AgentProfile{ModelID: "default-model"},
		},
	})
	imSvc := im.NewService()
	feishuSvc := feishu.NewService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, imSvc, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	csgclawBot, err := svc.Create(context.Background(), CreateRequest{
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelCSGClaw),
	})
	if err != nil {
		t.Fatalf("Create(csgclaw manager) error = %v", err)
	}
	feishuBot, err := svc.Create(context.Background(), CreateRequest{
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelFeishu),
	})
	if err != nil {
		t.Fatalf("Create(feishu manager) error = %v", err)
	}
	if csgclawBot.ID != agent.ManagerUserID || feishuBot.ID != agent.ManagerUserID || csgclawBot.AgentID != feishuBot.AgentID {
		t.Fatalf("created managers = %+v / %+v, want shared manager agent", csgclawBot, feishuBot)
	}
	all, err := svc.List("", "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List() = %+v, want two channel bindings", all)
	}
}

func TestServiceCreateCSGClawManagerReusesExistingBotAndRestoresMissingUser(t *testing.T) {
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			Role:      agent.RoleManager,
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
	})
	imSvc := im.NewService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, imSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	if _, err := svc.Create(context.Background(), CreateRequest{
		ID:      agent.ManagerUserID,
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelCSGClaw),
	}); err != nil {
		t.Fatalf("first Create(manager) error = %v", err)
	}
	if err := imSvc.DeleteUser(agent.ManagerUserID); err != nil {
		t.Fatalf("DeleteUser(manager) error = %v", err)
	}
	if containsUser(imSvc.ListUsers(), agent.ManagerUserID) {
		t.Fatalf("users = %+v, want u-manager removed before second create", imSvc.ListUsers())
	}

	got, err := svc.Create(context.Background(), CreateRequest{
		ID:      agent.ManagerUserID,
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelCSGClaw),
	})
	if err != nil {
		t.Fatalf("second Create(manager) error = %v", err)
	}
	if got.ID != agent.ManagerUserID || got.UserID != agent.ManagerUserID {
		t.Fatalf("second Create(manager) = %+v, want u-manager", got)
	}
	if !containsUser(imSvc.ListUsers(), agent.ManagerUserID) {
		t.Fatalf("users = %+v, want u-manager restored", imSvc.ListUsers())
	}
}

func TestServiceCreateFeishuManagerEnsuresExistingUser(t *testing.T) {
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			Role:      agent.RoleManager,
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
	})
	feishuSvc := feishu.NewService()
	if _, err := feishuSvc.CreateUser(feishu.CreateUserRequest{ID: agent.ManagerUserID, Name: "manager"}); err != nil {
		t.Fatalf("CreateUser(manager) error = %v", err)
	}
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, nil, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.Create(context.Background(), CreateRequest{
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelFeishu),
	})
	if err != nil {
		t.Fatalf("Create(feishu manager) error = %v", err)
	}
	if got.ID != agent.ManagerUserID || got.UserID != agent.ManagerUserID || got.Channel != string(ChannelFeishu) {
		t.Fatalf("Create(feishu manager) = %+v, want u-manager feishu", got)
	}
}

func TestServiceCreateFeishuManagerUsesConfiguredOpenID(t *testing.T) {
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			Role:      agent.RoleManager,
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
		},
	})
	feishuSvc := feishu.NewServiceWithBotOpenIDResolver(
		map[string]feishu.AppConfig{
			"u-manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
		},
		func(_ context.Context, app feishu.AppConfig) (feishu.BotInfo, error) {
			if got, want := app.AppID, "cli_manager"; got != want {
				t.Fatalf("resolve app_id = %q, want %q", got, want)
			}
			return feishu.BotInfo{OpenID: "ou_manager", AppName: "Manager Bot"}, nil
		},
	)
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, nil, feishuSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.Create(context.Background(), CreateRequest{
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelFeishu),
	})
	if err != nil {
		t.Fatalf("Create(feishu manager) error = %v", err)
	}
	if got.ID != agent.ManagerUserID || got.AgentID != agent.ManagerUserID || got.UserID != "ou_manager" || got.Channel != string(ChannelFeishu) {
		t.Fatalf("Create(feishu manager) = %+v, want u-manager agent with ou_manager user", got)
	}

	bots, err := svc.List(string(ChannelFeishu), "")
	if err != nil {
		t.Fatalf("List(feishu) error = %v", err)
	}
	if len(bots) != 1 || bots[0].UserID != "ou_manager" {
		t.Fatalf("List(feishu) = %+v, want manager user_id ou_manager", bots)
	}
}

func TestServiceCreateManagerBootstrapsMissingAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	agentSvc := mustNewSeededAgentService(t, nil)
	imSvc := im.NewService()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, imSvc)
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	got, err := svc.Create(context.Background(), CreateRequest{
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelCSGClaw),
	})
	if err != nil {
		t.Fatalf("Create(manager) error = %v", err)
	}
	if got.ID != agent.ManagerUserID || got.AgentID != agent.ManagerUserID || got.UserID != agent.ManagerUserID {
		t.Fatalf("Create(manager) = %+v, want u-manager IDs", got)
	}
	if _, ok := agentSvc.Agent(agent.ManagerUserID); !ok {
		t.Fatal("manager agent was not bootstrapped")
	}
}

func TestServiceCreateRejectsUnsupportedCombination(t *testing.T) {
	svc := mustNewBotService(t, nil)

	_, err := svc.Create(context.Background(), CreateRequest{
		ID:      "custom-manager",
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelCSGClaw),
	})
	if err == nil || !strings.Contains(err.Error(), "agent service is required") {
		t.Fatalf("Create(manager without agent service) error = %v, want agent service error", err)
	}

	_, err = svc.Create(context.Background(), CreateRequest{
		Name:    "alice",
		Role:    string(RoleWorker),
		Channel: "slack",
	})
	if err == nil || !strings.Contains(err.Error(), "channel must be one of") {
		t.Fatalf("Create(feishu) error = %v, want unsupported channel", err)
	}
}

func TestServiceCreateManagerRejectsCustomID(t *testing.T) {
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{ID: agent.ManagerUserID, Name: agent.ManagerName, Role: agent.RoleManager},
	})
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewServiceWithDependencies(store, agentSvc, im.NewService())
	if err != nil {
		t.Fatalf("NewServiceWithDependencies() error = %v", err)
	}

	_, err = svc.Create(context.Background(), CreateRequest{
		ID:      "bot-manager",
		Name:    "manager",
		Role:    string(RoleManager),
		Channel: string(ChannelCSGClaw),
	})
	if err == nil || !strings.Contains(err.Error(), `manager bot id must be "u-manager"`) {
		t.Fatalf("Create(manager custom ID) error = %v, want manager ID error", err)
	}
}

func containsUser(users []im.User, id string) bool {
	for _, user := range users {
		if user.ID == id {
			return true
		}
	}
	return false
}

func containsMember(members []string, id string) bool {
	for _, member := range members {
		if member == id {
			return true
		}
	}
	return false
}

func mustReceiveEventWithin(t *testing.T, events <-chan im.Event, timeout time.Duration) im.Event {
	t.Helper()
	select {
	case evt := <-events:
		return evt
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for IM event after %s", timeout)
		return im.Event{}
	}
}

func mustNewBotService(t *testing.T, bots []Bot) *Service {
	t.Helper()
	store, err := NewMemoryStore(bots)
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	svc, err := NewService(store)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}

func mustNewBotLocalTemplateHubService(t *testing.T, id string, item hub.Template) *hub.Service {
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

func mustNewSeededAgentService(t *testing.T, agents []agent.Agent) *agent.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(agent.TestOnlySetSandboxProvider(sandboxtest.NewProvider()))
	resetFakeBotRuntimeStates()
	t.Cleanup(resetFakeBotRuntimeStates)
	restoreDefault := agent.TestOnlySetDefaultServiceOption(func(s *agent.Service) error {
		if err := agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindPicoClawSandbox})(s); err != nil {
			return err
		}
		if err := agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindOpenClawSandbox})(s); err != nil {
			return err
		}
		return agent.WithRuntime(fakeBotAgentRuntime{kind: agent.RuntimeKindCodex})(s)
	})
	t.Cleanup(restoreDefault)

	if agents == nil {
		agents = []agent.Agent{}
	}
	for i := range agents {
		if strings.TrimSpace(agents[i].RuntimeKind) == "" {
			agents[i].RuntimeKind = agent.RuntimeKindPicoClawSandbox
		}
	}
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(map[string]any{"agents": agents})
	if err != nil {
		t.Fatalf("marshal agents: %v", err)
	}
	if err := os.WriteFile(statePath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write agents: %v", err)
	}
	svc, err := agent.NewService(testAgentModelConfig(), config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}
	return svc
}

func mustSetAgentRuntimeState(t *testing.T, agentName, boxID string, state sandbox.State) {
	t.Helper()
	setFakeBotRuntimeState(testRuntimeIDForAgentName(agentName), boxID, state)
}
