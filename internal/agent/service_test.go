package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	boxlite "github.com/RussellLuo/boxlite/sdks/go"

	"csgclaw/internal/config"
)

func testModelConfig() config.ModelConfig {
	return config.ModelConfig{
		Provider: config.ProviderLLMAPI,
		BaseURL:  "http://127.0.0.1:4000",
		APIKey:   "sk-test",
		ModelID:  "model-1",
	}
}

func TestCreateWorkerRejectsReservedManagerName(t *testing.T) {
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateWorker(context.Background(), CreateRequest{
		ID:   "worker-1",
		Name: "manager",
	})
	if err == nil {
		t.Fatal("CreateWorker() error = nil, want reserved-name error")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("CreateWorker() error = %q, want reserved-name error", err)
	}
}

func TestCreateWorkerRejectsDuplicateName(t *testing.T) {
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	svc.agents["worker-1"] = Agent{
		ID:        "worker-1",
		Name:      "alice",
		Status:    "active",
		CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		Role:      RoleWorker,
	}

	_, err = svc.CreateWorker(context.Background(), CreateRequest{
		ID:   "worker-2",
		Name: "Alice",
	})
	if err == nil {
		t.Fatal("CreateWorker() duplicate error = nil, want duplicate-name error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("CreateWorker() duplicate error = %q, want duplicate-name error", err)
	}
}

func TestCreateWorkerRejectsInvalidRuntime(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return &boxlite.Runtime{}, nil },
		nil,
	)
	defer ResetTestHooks()

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateWorker(context.Background(), CreateRequest{Name: "alice"})
	if err == nil {
		t.Fatal("CreateWorker() error = nil, want invalid runtime error")
	}
	if !strings.Contains(err.Error(), "invalid boxlite runtime") {
		t.Fatalf("CreateWorker() error = %q, want invalid runtime error", err)
	}
}

func TestRuntimeValidRejectsNilAndZeroValue(t *testing.T) {
	var nilRT *boxlite.Runtime
	if runtimeValid(nilRT) {
		t.Fatal("runtimeValid(nil) = true, want false")
	}
	if runtimeValid(&boxlite.Runtime{}) {
		t.Fatal("runtimeValid(zero runtime) = true, want false")
	}
}

func TestListWorkersFiltersUnifiedAgents(t *testing.T) {
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	svc.agents["u-manager"] = Agent{ID: "u-manager", Name: "manager", Role: RoleManager, CreatedAt: time.Date(2026, 3, 28, 9, 0, 0, 0, time.UTC)}
	svc.agents["worker-1"] = Agent{ID: "worker-1", Name: "alice", Role: RoleWorker, CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)}
	svc.agents["agent-1"] = Agent{ID: "agent-1", Name: "observer", Role: RoleAgent, CreatedAt: time.Date(2026, 3, 28, 11, 0, 0, 0, time.UTC)}

	workers := svc.ListWorkers()
	if len(workers) != 1 || workers[0].ID != "worker-1" {
		t.Fatalf("ListWorkers() = %+v, want only worker agent", workers)
	}
}

func TestLoadMigratesLegacyWorkersIntoAgents(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Workers: []legacyWorker{
			{
				ID:        "worker-1",
				Name:      "alice",
				Status:    "running",
				CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, ok := svc.Agent("worker-1")
	if !ok {
		t.Fatal("Agent() ok = false, want true")
	}
	if got.Role != RoleWorker {
		t.Fatalf("Agent().Role = %q, want %q", got.Role, RoleWorker)
	}
}

func TestDeleteRejectsManagerAgent(t *testing.T) {
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	svc.agents[ManagerUserID] = Agent{
		ID:        ManagerUserID,
		Name:      ManagerName,
		Role:      RoleManager,
		CreatedAt: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
	}

	err = svc.Delete(context.Background(), ManagerUserID)
	if err == nil {
		t.Fatal("Delete() error = nil, want reserved error")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("Delete() error = %q, want reserved error", err)
	}
}

func TestDeleteRemovesAgentFromState(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		nil,
	)
	defer ResetTestHooks()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	svc.agents["u-alice"] = Agent{
		ID:        "u-alice",
		Name:      "alice",
		Role:      RoleWorker,
		Status:    "running",
		CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}
	if err := svc.saveLocked(); err != nil {
		t.Fatalf("saveLocked() error = %v", err)
	}

	if err := svc.Delete(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := svc.Agent("u-alice"); ok {
		t.Fatal("Agent() ok = true, want false after delete")
	}

	reloaded, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() reload error = %v", err)
	}
	if _, ok := reloaded.Agent("u-alice"); ok {
		t.Fatal("reloaded Agent() ok = true, want false after delete")
	}
}

func TestDeleteRemovesAgentHomeDirectory(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		nil,
	)
	defer ResetTestHooks()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:        "u-alice",
		Name:      "alice",
		Role:      RoleWorker,
		Status:    "running",
		CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	agentHome, err := agentHomeDir("alice")
	if err != nil {
		t.Fatalf("agentHomeDir() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(agentHome, config.RuntimeHomeDirName), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(agent runtime) error = %v", err)
	}
	sharedProjects, err := ensureAgentProjectsRoot()
	if err != nil {
		t.Fatalf("ensureAgentProjectsRoot() error = %v", err)
	}

	if err := svc.Delete(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, err := os.Stat(agentHome); !os.IsNotExist(err) {
		t.Fatalf("os.Stat(agentHome) error = %v, want not exist", err)
	}
	if info, err := os.Stat(sharedProjects); err != nil {
		t.Fatalf("os.Stat(sharedProjects) error = %v", err)
	} else if !info.IsDir() {
		t.Fatalf("shared projects path is not a directory: %q", sharedProjects)
	}
}

func TestDeletePrefersBoxIDOverName(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return &boxlite.Runtime{}, nil },
		nil,
	)
	defer ResetTestHooks()

	var removed string
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, idOrName string) error {
		removed = idOrName
		return nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string) (*boxlite.Box, error) {
		return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing"}
	}
	defer func() {
		testForceRemoveBoxHook = nil
	}()

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:        "u-alice",
		Name:      "alice",
		BoxID:     "box-123",
		Role:      RoleWorker,
		Status:    "running",
		CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	if err := svc.Delete(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if removed != "box-123" {
		t.Fatalf("ForceRemove() target = %q, want %q", removed, "box-123")
	}
}

func TestDeleteFallsBackToNameWhenStoredBoxIDIsStale(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return &boxlite.Runtime{}, nil },
		nil,
	)
	defer ResetTestHooks()

	var lookedUp []string
	var removed string
	testGetBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, idOrName string) (*boxlite.Box, error) {
		lookedUp = append(lookedUp, idOrName)
		if idOrName == "alice" {
			return &boxlite.Box{}, nil
		}
		return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing"}
	}
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, idOrName string) error {
		removed = idOrName
		return nil
	}
	defer func() {
		testGetBoxHook = nil
		testForceRemoveBoxHook = nil
	}()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:        "u-alice",
		Name:      "alice",
		BoxID:     "box-stale",
		Role:      RoleWorker,
		Status:    "running",
		CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	agentHome, err := agentHomeDir("alice")
	if err != nil {
		t.Fatalf("agentHomeDir() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(agentHome, config.RuntimeHomeDirName), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(agent runtime) error = %v", err)
	}

	if err := svc.Delete(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if strings.Join(lookedUp, ",") != "box-stale,alice" {
		t.Fatalf("getBox() keys = %q, want stale box id then name fallback", lookedUp)
	}
	if removed != "alice" {
		t.Fatalf("ForceRemove() target = %q, want %q", removed, "alice")
	}
}

func TestDeleteRemovesRuntimeCacheByHomeDir(t *testing.T) {
	rt := &boxlite.Runtime{}
	SetTestHooks(func(_ *Service, _ string) (*boxlite.Runtime, error) { return rt, nil }, nil)
	defer ResetTestHooks()
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string) error {
		return nil
	}
	var closeRuntimeCalls int
	testCloseRuntimeHook = func(_ *Service, home string, got *boxlite.Runtime) error {
		if got != rt {
			t.Fatalf("closeRuntime() got runtime %p, want %p", got, rt)
		}
		if !strings.HasSuffix(home, filepath.Join("alice", config.RuntimeHomeDirName)) {
			t.Fatalf("closeRuntime() home = %q, want alice runtime home", home)
		}
		closeRuntimeCalls++
		return nil
	}
	defer func() {
		testForceRemoveBoxHook = nil
	}()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:        "u-alice",
		Name:      "alice",
		Role:      RoleWorker,
		Status:    "running",
		CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	runtimeHome, err := boxRuntimeHome("alice")
	if err != nil {
		t.Fatalf("boxRuntimeHome() error = %v", err)
	}
	svc.runtimes[runtimeHome] = rt

	if err := svc.Delete(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := svc.runtimes[runtimeHome]; ok {
		t.Fatalf("Delete() kept runtime cache for %q", runtimeHome)
	}
	if closeRuntimeCalls != 1 {
		t.Fatalf("closeRuntime() calls = %d, want %d", closeRuntimeCalls, 1)
	}
}

func TestCreateWorkerStoresBoxID(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string, name, _ string, _ config.ModelConfig) (*boxlite.Box, *boxlite.BoxInfo, error) {
			return nil, &boxlite.BoxInfo{
				ID:        "box-" + name,
				Name:      name,
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
				Image:     "test-image",
			}, nil
		},
	)
	defer ResetTestHooks()

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateRequest{Name: "alice"})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.BoxID != "box-alice" {
		t.Fatalf("CreateWorker().BoxID = %q, want %q", got.BoxID, "box-alice")
	}
}

func TestCreateWorkerStoresResolvedProfileSnapshot(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return nil, nil },
		func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string, name, _ string, _ config.ModelConfig) (*boxlite.Box, *boxlite.BoxInfo, error) {
			return nil, &boxlite.BoxInfo{
				ID:        "box-" + name,
				Name:      name,
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
				Image:     "test-image",
			}, nil
		},
	)
	defer ResetTestHooks()

	svc, err := NewServiceWithLLM(config.LLMConfig{
		DefaultProfile: "remote-main",
		Profiles: map[string]config.ModelConfig{
			"remote-main": {
				Provider:        config.ProviderLLMAPI,
				BaseURL:         "https://example.test/v1",
				APIKey:          "sk-test",
				ModelID:         "gpt-5.4",
				ReasoningEffort: "medium",
			},
		},
	}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateRequest{
		Name:    "alice",
		Profile: "remote-main",
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.Profile != "remote-main.gpt-5.4" {
		t.Fatalf("CreateWorker().Profile = %q, want %q", got.Profile, "remote-main.gpt-5.4")
	}
	if got.Provider != config.ProviderLLMAPI {
		t.Fatalf("CreateWorker().Provider = %q, want %q", got.Provider, config.ProviderLLMAPI)
	}
	if got.ModelID != "gpt-5.4" {
		t.Fatalf("CreateWorker().ModelID = %q, want %q", got.ModelID, "gpt-5.4")
	}
	if got.ReasoningEffort != "medium" {
		t.Fatalf("CreateWorker().ReasoningEffort = %q, want %q", got.ReasoningEffort, "medium")
	}
}

func TestCreateWorkerClosesBoxHandleAfterCreate(t *testing.T) {
	rt := &boxlite.Runtime{}
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return rt, nil },
		func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string, name, _ string, _ config.ModelConfig) (*boxlite.Box, *boxlite.BoxInfo, error) {
			return &boxlite.Box{}, &boxlite.BoxInfo{
				ID:        "box-" + name,
				Name:      name,
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
				Image:     "test-image",
			}, nil
		},
	)
	defer ResetTestHooks()

	var closeCalls int
	var closeRuntimeCalls int
	testCloseBoxHook = func(_ *Service, _ *boxlite.Box) error {
		closeCalls++
		return nil
	}
	testCloseRuntimeHook = func(_ *Service, _ string, got *boxlite.Runtime) error {
		if got != rt {
			t.Fatalf("closeRuntime() got runtime %p, want %p", got, rt)
		}
		closeRuntimeCalls++
		return nil
	}

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateRequest{Name: "alice"})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.BoxID != "box-alice" {
		t.Fatalf("CreateWorker().BoxID = %q, want %q", got.BoxID, "box-alice")
	}
	if closeCalls != 1 {
		t.Fatalf("closeBox() calls = %d, want %d", closeCalls, 1)
	}
	if closeRuntimeCalls != 1 {
		t.Fatalf("closeRuntime() calls = %d, want %d", closeRuntimeCalls, 1)
	}
}

func TestStreamLogsUsesStoredBoxIDAndTailArgs(t *testing.T) {
	rt := &boxlite.Runtime{}
	SetTestHooks(func(_ *Service, _ string) (*boxlite.Runtime, error) { return rt, nil }, nil)
	defer ResetTestHooks()

	var gotBoxID string
	testGetBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, idOrName string) (*boxlite.Box, error) {
		gotBoxID = idOrName
		return &boxlite.Box{}, nil
	}
	var gotName string
	var gotArgs []string
	testRunBoxCommandHook = func(_ *Service, _ context.Context, _ *boxlite.Box, name string, args []string, w io.Writer) (int, error) {
		gotName = name
		gotArgs = append([]string(nil), args...)
		_, _ = fmt.Fprint(w, "line-1\n")
		return 0, nil
	}
	defer func() {
		testGetBoxHook = nil
		testRunBoxCommandHook = nil
	}()

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:        "u-alice",
		Name:      "alice",
		BoxID:     "box-123",
		Role:      RoleWorker,
		Status:    "running",
		CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	var out strings.Builder
	if err := svc.StreamLogs(context.Background(), "u-alice", true, 50, &out); err != nil {
		t.Fatalf("StreamLogs() error = %v", err)
	}
	if gotBoxID != "box-123" {
		t.Fatalf("getBox() idOrName = %q, want %q", gotBoxID, "box-123")
	}
	if gotName != "tail" {
		t.Fatalf("runBoxCommand() name = %q, want %q", gotName, "tail")
	}
	if strings.Join(gotArgs, " ") != "-n 50 -f /home/picoclaw/.picoclaw/gateway.log" {
		t.Fatalf("runBoxCommand() args = %q", gotArgs)
	}
	if out.String() != "line-1\n" {
		t.Fatalf("output = %q, want streamed log line", out.String())
	}
}

func TestStreamLogsFallsBackToNameAndRefreshesStoredBoxID(t *testing.T) {
	rt := &boxlite.Runtime{}
	SetTestHooks(func(_ *Service, _ string) (*boxlite.Runtime, error) { return rt, nil }, nil)
	defer ResetTestHooks()

	var gotKeys []string
	testGetBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, idOrName string) (*boxlite.Box, error) {
		gotKeys = append(gotKeys, idOrName)
		if idOrName == "alice" {
			return &boxlite.Box{}, nil
		}
		return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing"}
	}
	testBoxInfoHook = func(_ *Service, _ context.Context, _ *boxlite.Box) (*boxlite.BoxInfo, error) {
		return &boxlite.BoxInfo{ID: "box-new"}, nil
	}
	testRunBoxCommandHook = func(_ *Service, _ context.Context, _ *boxlite.Box, name string, args []string, w io.Writer) (int, error) {
		_, _ = fmt.Fprint(w, "line-1\n")
		return 0, nil
	}
	defer func() {
		testGetBoxHook = nil
		testBoxInfoHook = nil
		testRunBoxCommandHook = nil
	}()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:        "u-alice",
		Name:      "alice",
		BoxID:     "box-stale",
		Role:      RoleWorker,
		Status:    "running",
		CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	var out strings.Builder
	if err := svc.StreamLogs(context.Background(), "u-alice", false, 20, &out); err != nil {
		t.Fatalf("StreamLogs() error = %v", err)
	}
	if strings.Join(gotKeys, ",") != "box-stale,alice" {
		t.Fatalf("getBox() keys = %q, want stale box id then name fallback", gotKeys)
	}
	got, ok := svc.Agent("u-alice")
	if !ok {
		t.Fatal("Agent() missing u-alice after StreamLogs()")
	}
	if got.BoxID != "box-new" {
		t.Fatalf("Agent().BoxID = %q, want %q", got.BoxID, "box-new")
	}
}

func TestCreateClosesBoxHandleAfterCreate(t *testing.T) {
	rt := &boxlite.Runtime{}
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return rt, nil },
		nil,
	)
	defer ResetTestHooks()

	var closeCalls int
	var closeRuntimeCalls int
	testCreateBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string, _ ...boxlite.BoxOption) (*boxlite.Box, error) {
		return &boxlite.Box{}, nil
	}
	testCloseBoxHook = func(_ *Service, _ *boxlite.Box) error {
		closeCalls++
		return nil
	}
	testCloseRuntimeHook = func(_ *Service, _ string, got *boxlite.Runtime) error {
		if got != rt {
			t.Fatalf("closeRuntime() got runtime %p, want %p", got, rt)
		}
		closeRuntimeCalls++
		return nil
	}

	svc, err := NewService(
		config.ModelConfig{BaseURL: "http://127.0.0.1:4000", APIKey: "sk-test", ModelID: "model-1"},
		config.ServerConfig{},
		"",
		"",
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.Create(context.Background(), CreateRequest{
		ID:    "agent-1",
		Name:  "alice",
		Image: "test-image",
		Role:  RoleAgent,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID != "agent-1" {
		t.Fatalf("Create().ID = %q, want %q", got.ID, "agent-1")
	}
	if closeCalls != 1 {
		t.Fatalf("closeBox() calls = %d, want %d", closeCalls, 1)
	}
	if closeRuntimeCalls != 1 {
		t.Fatalf("closeRuntime() calls = %d, want %d", closeRuntimeCalls, 1)
	}
}

func TestEnsureBootstrapStateForceRecreatePrefersStoredManagerBoxID(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return &boxlite.Runtime{}, nil },
		func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string, name, _ string, _ config.ModelConfig) (*boxlite.Box, *boxlite.BoxInfo, error) {
			return &boxlite.Box{}, &boxlite.BoxInfo{
				ID:        "box-new",
				Name:      name,
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
				Image:     "test-image",
			}, nil
		},
	)
	defer ResetTestHooks()

	var removed string
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, idOrName string) error {
		removed = idOrName
		return nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string) (*boxlite.Box, error) {
		return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing"}
	}
	defer func() {
		testForceRemoveBoxHook = nil
	}()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []Agent{
			{
				ID:        ManagerUserID,
				Name:      ManagerName,
				Role:      RoleManager,
				BoxID:     "box-old",
				Status:    "running",
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := EnsureBootstrapState(context.Background(), statePath, config.ServerConfig{}, config.ModelConfig{}, "", true); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}
	if removed != "box-old" {
		t.Fatalf("ForceRemove() target = %q, want %q", removed, "box-old")
	}

	reloaded, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() reload error = %v", err)
	}
	got, ok := reloaded.Agent(ManagerUserID)
	if !ok {
		t.Fatal("Agent() ok = false, want true")
	}
	if got.BoxID != "box-new" {
		t.Fatalf("Agent().BoxID = %q, want %q", got.BoxID, "box-new")
	}
}

func TestEnsureBootstrapStateForceRecreateResetsManagerHomeBeforeCreate(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	runtimeHome, err := boxRuntimeHome(ManagerName)
	if err != nil {
		t.Fatalf("boxRuntimeHome() error = %v", err)
	}
	managerHome, err := agentHomeDir(ManagerName)
	if err != nil {
		t.Fatalf("agentHomeDir() error = %v", err)
	}
	stalePath := filepath.Join(managerHome, "stale.txt")
	if err := os.MkdirAll(managerHome, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(stalePath, []byte("stale"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var ensuredHomes []string
	var closeRuntimeCalls int
	testEnsureRuntimeAtHomeHook = func(_ *Service, home string) (*boxlite.Runtime, error) {
		ensuredHomes = append(ensuredHomes, home)
		return &boxlite.Runtime{}, nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string) (*boxlite.Box, error) {
		return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing"}
	}
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string) error {
		return nil
	}
	testCreateGatewayBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string, name, _ string, _ config.ModelConfig) (*boxlite.Box, *boxlite.BoxInfo, error) {
		if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
			t.Fatalf("stale manager file still exists before recreate: err=%v", err)
		}
		return &boxlite.Box{}, &boxlite.BoxInfo{
			ID:        "box-new",
			Name:      name,
			State:     boxlite.StateRunning,
			CreatedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
			Image:     "test-image",
		}, nil
	}
	testCloseRuntimeHook = func(_ *Service, gotHome string, _ *boxlite.Runtime) error {
		closeRuntimeCalls++
		if gotHome != runtimeHome {
			t.Fatalf("closeRuntime() home = %q, want %q", gotHome, runtimeHome)
		}
		return nil
	}
	defer ResetTestHooks()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []Agent{
			{
				ID:        ManagerUserID,
				Name:      ManagerName,
				Role:      RoleManager,
				BoxID:     "box-old",
				Status:    "running",
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := EnsureBootstrapState(context.Background(), statePath, config.ServerConfig{}, config.ModelConfig{}, "", true); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}
	if got, want := len(ensuredHomes), 2; got != want {
		t.Fatalf("ensureRuntimeAtHome() calls = %d, want %d", got, want)
	}
	for _, gotHome := range ensuredHomes {
		if gotHome != runtimeHome {
			t.Fatalf("ensureRuntimeAtHome() home = %q, want %q", gotHome, runtimeHome)
		}
	}
	if closeRuntimeCalls != 2 {
		t.Fatalf("closeRuntime() calls = %d, want %d", closeRuntimeCalls, 2)
	}
}

func TestEnsureBootstrapStateClosesManagerBoxHandleAfterCreate(t *testing.T) {
	rt := &boxlite.Runtime{}
	SetTestHooks(
		func(_ *Service, _ string) (*boxlite.Runtime, error) { return rt, nil },
		func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string, name, _ string, _ config.ModelConfig) (*boxlite.Box, *boxlite.BoxInfo, error) {
			return &boxlite.Box{}, &boxlite.BoxInfo{
				ID:        "box-" + name,
				Name:      name,
				State:     boxlite.StateRunning,
				CreatedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
				Image:     "test-image",
			}, nil
		},
	)
	defer ResetTestHooks()

	testGetBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string) (*boxlite.Box, error) {
		return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing"}
	}

	var closeCalls int
	var closeRuntimeCalls int
	testCloseBoxHook = func(_ *Service, _ *boxlite.Box) error {
		closeCalls++
		return nil
	}
	testCloseRuntimeHook = func(_ *Service, _ string, got *boxlite.Runtime) error {
		if got != rt {
			t.Fatalf("closeRuntime() got runtime %p, want %p", got, rt)
		}
		closeRuntimeCalls++
		return nil
	}

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	if err := EnsureBootstrapState(context.Background(), statePath, config.ServerConfig{}, config.ModelConfig{}, "", false); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}
	if closeCalls != 1 {
		t.Fatalf("closeBox() calls = %d, want %d", closeCalls, 1)
	}
	if closeRuntimeCalls != 1 {
		t.Fatalf("closeRuntime() calls = %d, want %d", closeRuntimeCalls, 1)
	}

	reloaded, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() reload error = %v", err)
	}
	if got, want := len(reloaded.runtimes), 0; got != want {
		t.Fatalf("len(reloaded.runtimes) = %d, want %d", got, want)
	}
}

func TestEnsureBootstrapStateReusesStoredManagerBoxIDWithoutForce(t *testing.T) {
	SetTestHooks(nil, nil)
	defer ResetTestHooks()

	primaryRT := &boxlite.Runtime{}
	legacyRT := &boxlite.Runtime{}
	testEnsureRuntimeAtHomeHook = func(_ *Service, home string) (*boxlite.Runtime, error) {
		if strings.HasSuffix(home, filepath.Join(config.AppDirName, config.RuntimeHomeDirName)) {
			return primaryRT, nil
		}
		return legacyRT, nil
	}

	var created bool
	testCreateGatewayBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string, _ string, _ string, _ config.ModelConfig) (*boxlite.Box, *boxlite.BoxInfo, error) {
		created = true
		return nil, nil, nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, rt *boxlite.Runtime, idOrName string) (*boxlite.Box, error) {
		if rt == primaryRT {
			return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing in primary"}
		}
		if rt == legacyRT && idOrName == "box-old" {
			return &boxlite.Box{}, nil
		}
		return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing"}
	}
	testStartBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Box) error { return nil }
	testBoxInfoHook = func(_ *Service, _ context.Context, _ *boxlite.Box) (*boxlite.BoxInfo, error) {
		return &boxlite.BoxInfo{
			ID:        "box-old",
			Name:      ManagerName,
			State:     boxlite.StateRunning,
			CreatedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
			Image:     "test-image",
		}, nil
	}

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []Agent{
			{
				ID:        ManagerUserID,
				Name:      ManagerName,
				Role:      RoleManager,
				BoxID:     "box-old",
				Status:    "running",
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := EnsureBootstrapState(context.Background(), statePath, config.ServerConfig{}, config.ModelConfig{}, "", false); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}
	if created {
		t.Fatal("createGatewayBox() called, want existing manager box to be reused")
	}
}

func TestBoxRuntimeHomeUsesPerAgentDirectory(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	got, err := boxRuntimeHome("alice")
	if err != nil {
		t.Fatalf("boxRuntimeHome() error = %v", err)
	}

	want := filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, "alice", config.RuntimeHomeDirName)
	if got != want {
		t.Fatalf("boxRuntimeHome() = %q, want %q", got, want)
	}
}

func TestLookupBootstrapManagerUsesPerAgentHome(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	var gotHome string
	testEnsureRuntimeAtHomeHook = func(_ *Service, homeDir string) (*boxlite.Runtime, error) {
		gotHome = homeDir
		return &boxlite.Runtime{}, nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ *boxlite.Runtime, _ string) (*boxlite.Box, error) {
		return nil, &boxlite.Error{Code: boxlite.ErrNotFound, Message: "missing"}
	}
	defer func() {
		testEnsureRuntimeAtHomeHook = nil
		testGetBoxHook = nil
	}()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	rt, box, err := svc.lookupBootstrapManager(context.Background())
	if err != nil {
		t.Fatalf("lookupBootstrapManager() error = %v", err)
	}
	if box != nil {
		t.Fatalf("lookupBootstrapManager() box = %#v, want nil", box)
	}
	wantHome := filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, ManagerName, config.RuntimeHomeDirName)
	if rt == nil {
		t.Fatal("lookupBootstrapManager() runtime = nil, want non-nil")
	}
	if info, err := os.Stat(wantHome); err != nil {
		t.Fatalf("os.Stat(runtime home) error = %v", err)
	} else if !info.IsDir() {
		t.Fatalf("runtime home is not a directory: %q", wantHome)
	}
	if got, want := len(svc.runtimes), 0; got != want {
		t.Fatalf("len(svc.runtimes) = %d, want %d when runtime creation is hooked", got, want)
	}
	if got, want := gotHome, wantHome; got != want {
		t.Fatalf("resolved manager runtime home = %q, want %q", got, want)
	}
}

func TestEnsureAgentProjectsRootUsesHomeProjectsDir(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	got, err := ensureAgentProjectsRoot()
	if err != nil {
		t.Fatalf("ensureAgentProjectsRoot() error = %v", err)
	}

	want := filepath.Join(homeDir, config.AppDirName, hostProjectsDir)
	if got != want {
		t.Fatalf("ensureAgentProjectsRoot() = %q, want %q", got, want)
	}

	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("ensureAgentProjectsRoot() path is not a directory: %q", got)
	}
}

func TestGatewayStartCommandUsesTiniForNormalMode(t *testing.T) {
	entrypoint, cmd := gatewayStartCommand(false)

	if strings.Join(entrypoint, " ") != "tini" {
		t.Fatalf("gatewayStartCommand(false) entrypoint = %q, want %q", entrypoint, []string{"tini"})
	}
	if strings.Join(cmd, " ") != "-- picoclaw gateway -d" {
		t.Fatalf("gatewayStartCommand(false) cmd = %q, want %q", cmd, []string{"--", "picoclaw", "gateway", "-d"})
	}
}

func TestGatewayStartCommandKeepsDebugSleepMode(t *testing.T) {
	entrypoint, cmd := gatewayStartCommand(true)

	if strings.Join(entrypoint, " ") != "sleep" {
		t.Fatalf("gatewayStartCommand(true) entrypoint = %q, want %q", entrypoint, []string{"sleep"})
	}
	if strings.Join(cmd, " ") != "infinity" {
		t.Fatalf("gatewayStartCommand(true) cmd = %q, want %q", cmd, []string{"infinity"})
	}
}

func TestPicoclawBoxEnvVars(t *testing.T) {
	got := picoclawBoxEnvVars(
		"http://10.0.0.8:18080",
		"shared-token",
		"u-worker-1",
		"http://10.0.0.8:18080/api/bots/u-worker-1/llm",
		"minimax-m2.7",
	)

	wants := map[string]string{
		"CSGCLAW_BASE_URL":                       "http://10.0.0.8:18080",
		"CSGCLAW_ACCESS_TOKEN":                   "shared-token",
		"PICOCLAW_CHANNELS_CSGCLAW_BASE_URL":     "http://10.0.0.8:18080",
		"PICOCLAW_CHANNELS_CSGCLAW_ACCESS_TOKEN": "shared-token",
		"PICOCLAW_CHANNELS_CSGCLAW_BOT_ID":       "u-worker-1",
		"CSGCLAW_LLM_BASE_URL":                   "http://10.0.0.8:18080/api/bots/u-worker-1/llm",
		"CSGCLAW_LLM_API_KEY":                    "shared-token",
		"CSGCLAW_LLM_MODEL_ID":                   "minimax-m2.7",
		"OPENAI_BASE_URL":                        "http://10.0.0.8:18080/api/bots/u-worker-1/llm",
		"OPENAI_API_KEY":                         "shared-token",
		"OPENAI_MODEL":                           "minimax-m2.7",
		"PICOCLAW_AGENTS_DEFAULTS_MODEL_NAME":    "minimax-m2.7",
		"PICOCLAW_CUSTOM_MODEL_NAME":             "minimax-m2.7",
		"PICOCLAW_CUSTOM_MODEL_ID":               "openai/minimax-m2.7",
		"PICOCLAW_CUSTOM_MODEL_API_KEY":          "shared-token",
		"PICOCLAW_CUSTOM_MODEL_BASE_URL":         "http://10.0.0.8:18080/api/bots/u-worker-1/llm",
	}
	for key, want := range wants {
		if got[key] != want {
			t.Fatalf("%s = %q, want %q", key, got[key], want)
		}
	}
}

func TestPicoclawBoxEnvVarsPrefixesCustomModelIDForSlashNames(t *testing.T) {
	got := picoclawBoxEnvVars(
		"http://10.0.0.8:18080",
		"shared-token",
		"u-worker-1",
		"http://10.0.0.8:18080/api/bots/u-worker-1/llm",
		"Qwen/Qwen3-0.6B-GGUF",
	)

	if got["PICOCLAW_CUSTOM_MODEL_ID"] != "openai/Qwen/Qwen3-0.6B-GGUF" {
		t.Fatalf("PICOCLAW_CUSTOM_MODEL_ID = %q, want %q", got["PICOCLAW_CUSTOM_MODEL_ID"], "openai/Qwen/Qwen3-0.6B-GGUF")
	}
	if got["PICOCLAW_CUSTOM_MODEL_NAME"] != "Qwen/Qwen3-0.6B-GGUF" {
		t.Fatalf("PICOCLAW_CUSTOM_MODEL_NAME = %q, want %q", got["PICOCLAW_CUSTOM_MODEL_NAME"], "Qwen/Qwen3-0.6B-GGUF")
	}
}

func TestAddFeishuBoxEnvVarsUsesMatchingBotID(t *testing.T) {
	envVars := map[string]string{}
	addFeishuBoxEnvVars(envVars, "u-worker-1", config.ChannelsConfig{
		Feishu: map[string]config.FeishuConfig{
			"u-worker-1": {
				AppID:     "cli_worker",
				AppSecret: "worker-secret",
			},
		},
	})

	if got, want := envVars["PICOCLAW_CHANNELS_FEISHU_APP_ID"], "cli_worker"; got != want {
		t.Fatalf("PICOCLAW_CHANNELS_FEISHU_APP_ID = %q, want %q", got, want)
	}
	if got, want := envVars["PICOCLAW_CHANNELS_FEISHU_APP_SECRET"], "worker-secret"; got != want {
		t.Fatalf("PICOCLAW_CHANNELS_FEISHU_APP_SECRET = %q, want %q", got, want)
	}
}

func TestAddFeishuBoxEnvVarsRequiresExactBotIDMatch(t *testing.T) {
	envVars := map[string]string{}
	addFeishuBoxEnvVars(envVars, ManagerUserID, config.ChannelsConfig{
		Feishu: map[string]config.FeishuConfig{
			"manager": {
				AppID:     "cli_manager",
				AppSecret: "manager-secret",
			},
		},
	})

	if _, ok := envVars["PICOCLAW_CHANNELS_FEISHU_APP_ID"]; ok {
		t.Fatalf("PICOCLAW_CHANNELS_FEISHU_APP_ID was set for non-matching bot id")
	}
	if _, ok := envVars["PICOCLAW_CHANNELS_FEISHU_APP_SECRET"]; ok {
		t.Fatalf("PICOCLAW_CHANNELS_FEISHU_APP_SECRET was set for non-matching bot id")
	}
}

func TestResolveManagerBaseURLPrefersLocalIP(t *testing.T) {
	orig := localIPv4Resolver
	localIPv4Resolver = func() string { return "10.0.0.8" }
	t.Cleanup(func() {
		localIPv4Resolver = orig
	})

	got := resolveManagerBaseURL(config.ServerConfig{
		ListenAddr:       "0.0.0.0:19090",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
	})

	want := "http://10.0.0.8:19090"
	if got != want {
		t.Fatalf("resolveManagerBaseURL() = %q, want %q", got, want)
	}
}
