package bot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	boxlite "github.com/RussellLuo/boxlite/sdks/go"

	"csgclaw/internal/agent"
	"csgclaw/internal/channel"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
)

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

	got, err := svc.List("")
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

	got, err := svc.List(" FEISHU ")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "bot-feishu" {
		t.Fatalf("List(FEISHU) = %+v, want only bot-feishu", got)
	}
}

func TestServiceListRejectsInvalidChannel(t *testing.T) {
	svc := mustNewBotService(t, nil)

	_, err := svc.List("slack")
	if err == nil || !strings.Contains(err.Error(), "channel must be one of") {
		t.Fatalf("List(slack) error = %v, want channel validation error", err)
	}
}

func TestNewServiceRequiresStore(t *testing.T) {
	_, err := NewService(nil)
	if err == nil || !strings.Contains(err.Error(), "bot store is required") {
		t.Fatalf("NewService(nil) error = %v, want store required", err)
	}
}

func TestServiceCreateCSGClawWorkerCreatesAgentUserAndBot(t *testing.T) {
	agent.SetTestHooks(
		func(_ *agent.Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		func(_ *agent.Service, _ context.Context, _ *boxlite.Runtime, _ string, name, botID string, _ config.ModelConfig) (*boxlite.Box, *boxlite.BoxInfo, error) {
			if name != "alice" {
				t.Fatalf("create gateway name = %q, want alice", name)
			}
			if botID != "u-alice" {
				t.Fatalf("create gateway botID = %q, want u-alice", botID)
			}
			return nil, &boxlite.BoxInfo{
				ID:        "box-alice",
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
				Name:      name,
				Image:     "test-image",
			}, nil
		},
	)
	defer agent.ResetTestHooks()

	agentSvc, err := agent.NewService(config.ModelConfig{ModelID: "default-model"}, config.ServerConfig{}, "", "")
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

	got, err := svc.Create(context.Background(), CreateRequest{
		Name:        "alice",
		Description: "test lead",
		Role:        string(RoleWorker),
		Channel:     string(ChannelCSGClaw),
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
	if _, ok := agentSvc.Agent("u-alice"); !ok {
		t.Fatal("agent u-alice not created")
	}
	users := imSvc.ListUsers()
	if !containsUser(users, "u-alice") {
		t.Fatalf("users = %+v, want u-alice", users)
	}
	listed, err := svc.List(string(ChannelCSGClaw))
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "u-alice" {
		t.Fatalf("List(csgclaw) = %+v, want u-alice", listed)
	}
}

func TestServiceCreateFeishuWorkerCreatesAgentUserAndBot(t *testing.T) {
	agent.SetTestHooks(
		func(_ *agent.Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		func(_ *agent.Service, _ context.Context, _ *boxlite.Runtime, _ string, name, botID string, _ config.ModelConfig) (*boxlite.Box, *boxlite.BoxInfo, error) {
			if name != "alice" {
				t.Fatalf("create gateway name = %q, want alice", name)
			}
			if botID != "u-alice" {
				t.Fatalf("create gateway botID = %q, want u-alice", botID)
			}
			return nil, &boxlite.BoxInfo{
				ID:        "box-alice",
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC),
				Name:      name,
				Image:     "test-image",
			}, nil
		},
	)
	defer agent.ResetTestHooks()

	agentSvc, err := agent.NewService(config.ModelConfig{ModelID: "default-model"}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}
	feishuSvc := channel.NewFeishuService()
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
	listed, err := svc.List(string(ChannelFeishu))
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "u-alice" {
		t.Fatalf("List(feishu) = %+v, want u-alice", listed)
	}
}

func TestServiceCreateCSGClawManagerBindsBootstrappedAgent(t *testing.T) {
	agentSvc := mustNewSeededAgentService(t, []agent.Agent{
		{
			ID:        agent.ManagerUserID,
			Name:      agent.ManagerName,
			Role:      agent.RoleManager,
			CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
			ModelID:   "default-model",
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
	if err := imSvc.KickUser(agent.ManagerUserID); err != nil {
		t.Fatalf("KickUser(manager) error = %v", err)
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
	feishuSvc := channel.NewFeishuService()
	if _, err := feishuSvc.CreateUser(channel.FeishuCreateUserRequest{ID: agent.ManagerUserID, Name: "manager"}); err != nil {
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

func TestServiceCreateManagerBootstrapsMissingAgent(t *testing.T) {
	agent.SetTestHooks(
		func(_ *agent.Service, _ string) (*boxlite.Runtime, error) { return &boxlite.Runtime{}, nil },
		func(_ *agent.Service, _ context.Context, _ *boxlite.Runtime, _ string, name, botID string, _ config.ModelConfig) (*boxlite.Box, *boxlite.BoxInfo, error) {
			if name != agent.ManagerName {
				t.Fatalf("create gateway name = %q, want manager", name)
			}
			if botID != agent.ManagerUserID {
				t.Fatalf("create gateway botID = %q, want u-manager", botID)
			}
			return &boxlite.Box{}, &boxlite.BoxInfo{
				ID:        "box-manager",
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC),
				Name:      name,
				Image:     "test-image",
			}, nil
		},
	)
	defer agent.ResetTestHooks()
	agent.TestOnlySetGetBoxHook(func(_ *agent.Service, _ context.Context, _ *boxlite.Runtime, _ string) (*boxlite.Box, error) {
		return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing"}
	})

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

func mustNewSeededAgentService(t *testing.T, agents []agent.Agent) *agent.Service {
	t.Helper()

	if agents == nil {
		agents = []agent.Agent{}
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
	svc, err := agent.NewService(config.ModelConfig{ModelID: "default-model"}, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("agent.NewService() error = %v", err)
	}
	return svc
}
