package api

import (
	"context"
	"csgclaw/internal/agentengine"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agent "csgclaw/internal/agentengine/agents"
)

func TestConnectingFirstProviderConfiguresManager(t *testing.T) {
	for _, action := range []string{"create", "check", "login"} {
		t.Run(action, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			configPath := filepath.Join(t.TempDir(), "config.toml")
			writeMinimalAPIConfig(t, configPath)
			providerID := "fresh-models"
			if action == "login" {
				providerID = agent.ModelProviderIDOpenCSG
			}
			defer stubModelProviderCheck(func(context.Context, agent.ModelProviderCheckInput) agent.ModelProviderCheckResult {
				return agent.ModelProviderCheckResult{ID: providerID, Status: agent.ModelProviderStatusConnected, Models: []string{"chat", "auto"}, LastCheckedAt: "2026-11-02T00:00:00Z"}
			})()
			defer stubAuthCallback(func(*http.Request, string) (string, error) { return "http://127.0.0.1:18080/", nil })()
			handler, runtime := newManagerOnboardingHandler(t, configPath, agent.Agent{
				ID: agent.ManagerUserID, Name: agent.ManagerName, Role: agent.RoleManager,
				RuntimeKind: agent.RuntimeKindCodex, Status: "profile_incomplete", DesiredState: agent.DesiredStateRunning,
				AgentProfile: agent.AgentProfile{Name: agent.ManagerName, Provider: agent.ProviderCSGHubLite},
			})
			routes := handler.Routes()
			create := func(connect bool) *httptest.ResponseRecorder {
				body := `{"id":"fresh-models","display_name":"Fresh models","base_url":"http://127.0.0.1:1/v1","api_key":"fixture-key","connect":` + map[bool]string{true: "true", false: "false"}[connect] + `}`
				rec := httptest.NewRecorder()
				routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/model-providers", strings.NewReader(body)))
				if rec.Code != http.StatusCreated {
					t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
				}
				return rec
			}
			switch action {
			case "create":
				create(true)
			case "check":
				create(false)
				rec := httptest.NewRecorder()
				routes.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/model-providers/fresh-models/check", strings.NewReader(`{"connect":true}`)))
				if rec.Code != http.StatusOK {
					t.Fatalf("check: %d %s", rec.Code, rec.Body.String())
				}
			case "login":
				rec := httptest.NewRecorder()
				routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback", nil))
				if rec.Code != http.StatusFound {
					t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
				}
			}
			manager, ok := handler.svc.Agent(agent.ManagerUserID)
			if !ok || !manager.ProfileComplete || manager.AgentProfile.ModelProviderID != providerID || manager.Status != "running" {
				t.Fatalf("manager did not come online: complete=%v provider=%s model=%s status=%s", manager.ProfileComplete, manager.AgentProfile.ModelProviderID, manager.AgentProfile.ModelID, manager.Status)
			}
			if action == "login" && manager.AgentProfile.ModelID != "auto" {
				t.Fatalf("OpenCSG model = %s, want auto", manager.AgentProfile.ModelID)
			}
			if manager.AgentProfile.APIKey != "" {
				t.Fatal("manager persisted inline provider secret")
			}
			starts := runtime.starts.Load()
			if starts == 0 {
				t.Fatal("manager runtime was never started")
			}
			cfg, err := config.Load(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := handler.saveModelProvidersConfig(context.Background(), configPath, cfg); err != nil {
				t.Fatal(err)
			}
			if runtime.starts.Load() != starts {
				t.Fatal("provider refresh restarted configured manager")
			}
		})
	}
}

// Observe the lifecycle rather than reporting a stopped process after Start.
type managerOnboardingRuntime struct {
	modelProviderFakeRuntime
	running atomic.Bool
	starts  atomic.Int32
}

func (r *managerOnboardingRuntime) Start(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	r.running.Store(true)
	r.starts.Add(1)
	return agentruntime.StateRunning, nil
}
func (r *managerOnboardingRuntime) Stop(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	r.running.Store(false)
	return agentruntime.StateStopped, nil
}
func (r *managerOnboardingRuntime) Info(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
	state := agentruntime.StateStopped
	if r.running.Load() {
		state = agentruntime.StateRunning
	}
	return agentruntime.Info{State: state}, nil
}
func newManagerOnboardingHandler(t *testing.T, configPath string, manager agent.Agent) (*Handler, *managerOnboardingRuntime) {
	t.Helper()
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "agents.json")
	data, err := json.Marshal(map[string]any{"agents": []agent.Agent{manager}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	runtime := &managerOnboardingRuntime{modelProviderFakeRuntime: modelProviderFakeRuntime{kind: agent.RuntimeKindCodex}}
	svc, err := agent.NewControllerWithLLM(cfg.Models, cfg.Server, "manager:test", statePath, agent.WithRuntime(runtime))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.SetDesiredState(manager.ID, manager.DesiredState, false); err != nil {
		t.Fatal(err)
	}
	h := &Handler{svc: svc, agentEngine: agentengine.New(svc), workspace: svc.Workspace(), agentModels: svc.Models(), agentRuntime: svc}
	h.SetConfigPath(configPath)
	return h, runtime
}

func TestProviderUpdatesPreserveManagerConfigurationAndIntent(t *testing.T) {
	for _, scenario := range []string{"unconnected", "image_only", "configured", "stopped", "archived"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			configPath := filepath.Join(t.TempDir(), "config.toml")
			writeMinimalAPIConfig(t, configPath)
			manager := agent.Agent{ID: agent.ManagerUserID, Name: agent.ManagerName, Role: agent.RoleManager, RuntimeKind: agent.RuntimeKindCodex, DesiredState: agent.DesiredStateRunning, Status: "profile_incomplete", AgentProfile: agent.AgentProfile{Name: agent.ManagerName, Provider: agent.ProviderCSGHubLite}}
			switch scenario {
			case "configured":
				manager.AgentProfile = agent.AgentProfile{Name: agent.ManagerName, Provider: agent.ProviderAPI, ModelProviderID: "chosen", ModelID: "chosen-model", ProfileComplete: true}
				manager.ProfileComplete = true
			case "stopped":
				manager.DesiredState = agent.DesiredStateStopped
			case "archived":
				now := time.Now()
				manager.ArchivedAt = &now
			}
			handler, runtime := newManagerOnboardingHandler(t, configPath, manager)
			cfg, err := config.Load(configPath)
			if err != nil {
				t.Fatal(err)
			}
			provider := config.ProviderConfig{Status: agent.ModelProviderStatusConnected, Models: []string{"chat", "image"}, ImageModels: []string{"image"}}
			if scenario == "unconnected" {
				provider.Status = agent.ModelProviderStatusFailed
			}
			if scenario == "image_only" {
				provider.Models = []string{"image"}
			}
			cfg.Models.Providers = map[string]config.ProviderConfig{"new-provider": provider}
			if err := handler.saveModelProvidersConfig(context.Background(), configPath, cfg); err != nil {
				t.Fatal(err)
			}
			got, _ := handler.svc.Agent(agent.ManagerUserID)
			if got.AgentProfile.ModelID != manager.AgentProfile.ModelID || got.DesiredState != manager.DesiredState || runtime.starts.Load() != 0 {
				t.Fatalf("provider update changed manager configuration or intent: model=%s desired=%s starts=%d", got.AgentProfile.ModelID, got.DesiredState, runtime.starts.Load())
			}
		})
	}
}
