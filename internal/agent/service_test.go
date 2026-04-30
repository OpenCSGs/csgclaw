package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"csgclaw/internal/config"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/boxlitecli"
)

type fakeRuntime struct{}

func (f *fakeRuntime) Create(context.Context, sandbox.CreateSpec) (sandbox.Instance, error) {
	return &fakeInstance{}, nil
}

func (f *fakeRuntime) Get(context.Context, string) (sandbox.Instance, error) {
	return &fakeInstance{}, nil
}

func (f *fakeRuntime) Remove(context.Context, string, sandbox.RemoveOptions) error {
	return nil
}

func (f *fakeRuntime) Close() error {
	return nil
}

type fakeInstance struct{}

func (f *fakeInstance) Start(context.Context) error {
	return nil
}

func (f *fakeInstance) Stop(context.Context, sandbox.StopOptions) error {
	return nil
}

func (f *fakeInstance) Info(context.Context) (sandbox.Info, error) {
	return sandbox.Info{}, nil
}

func (f *fakeInstance) Run(context.Context, sandbox.CommandSpec) (sandbox.CommandResult, error) {
	return sandbox.CommandResult{}, nil
}

func (f *fakeInstance) Close() error {
	return nil
}

type fakeInfoInstance struct {
	fakeInstance
	info sandbox.Info
}

func (f *fakeInfoInstance) Info(context.Context) (sandbox.Info, error) {
	return f.info, nil
}

type cancelOnWrite struct {
	writer io.Writer
	cancel context.CancelFunc
}

func (w cancelOnWrite) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if n > 0 && w.cancel != nil {
		w.cancel()
	}
	return n, err
}

type agentBoxliteCLIRunner struct {
	requests []boxlitecli.CommandRequest
	boxes    map[string]agentBoxliteCLIBox
}

type agentBoxliteCLIBox struct {
	ID     string
	Name   string
	Status string
}

func newAgentBoxliteCLIRunner() *agentBoxliteCLIRunner {
	return &agentBoxliteCLIRunner{boxes: make(map[string]agentBoxliteCLIBox)}
}

func (r *agentBoxliteCLIRunner) Run(_ context.Context, req boxlitecli.CommandRequest) (boxlitecli.CommandResult, error) {
	r.requests = append(r.requests, req)
	if len(req.Args) < 3 {
		return boxlitecli.CommandResult{ExitCode: 1, Stderr: []byte("missing command")}, fmt.Errorf("exit status 1")
	}

	switch req.Args[2] {
	case "inspect":
		idOrName := req.Args[len(req.Args)-1]
		box, ok := r.boxes[idOrName]
		if !ok {
			return boxlitecli.CommandResult{
				ExitCode: 1,
				Stderr:   []byte("Error: no such box: " + idOrName),
			}, fmt.Errorf("exit status 1")
		}
		stdout := fmt.Sprintf(`[{"Id":%q,"Name":%q,"Created":"2026-04-18T07:31:25Z","Status":%q}]`, box.ID, box.Name, box.Status)
		return boxlitecli.CommandResult{Stdout: []byte(stdout)}, nil
	case "run":
		name := valueAfter(req.Args, "--name")
		if name == "" {
			name = "box"
		}
		id := "box-" + name
		box := agentBoxliteCLIBox{ID: id, Name: name, Status: "running"}
		r.boxes[id] = box
		r.boxes[name] = box
		return boxlitecli.CommandResult{Stdout: []byte(id + "\n")}, nil
	case "start":
		idOrName := req.Args[len(req.Args)-1]
		box, ok := r.boxes[idOrName]
		if !ok {
			return boxlitecli.CommandResult{ExitCode: 1, Stderr: []byte("Error: no such box: " + idOrName)}, fmt.Errorf("exit status 1")
		}
		box.Status = "running"
		r.boxes[box.ID] = box
		r.boxes[box.Name] = box
		return boxlitecli.CommandResult{}, nil
	case "exec":
		if len(req.Args) > 6 && req.Args[5] == "tail" && req.Stdout != nil {
			_, _ = req.Stdout.Write([]byte("gateway line\n"))
		}
		return boxlitecli.CommandResult{}, nil
	case "rm":
		idOrName := req.Args[len(req.Args)-1]
		box, ok := r.boxes[idOrName]
		if !ok {
			return boxlitecli.CommandResult{ExitCode: 1, Stderr: []byte("Error: no such box: " + idOrName)}, fmt.Errorf("exit status 1")
		}
		delete(r.boxes, box.ID)
		delete(r.boxes, box.Name)
		return boxlitecli.CommandResult{}, nil
	default:
		return boxlitecli.CommandResult{ExitCode: 1, Stderr: []byte("unsupported command")}, fmt.Errorf("exit status 1")
	}
}

func valueAfter(args []string, key string) string {
	for idx := 0; idx < len(args)-1; idx++ {
		if args[idx] == key {
			return args[idx+1]
		}
	}
	return ""
}

func countBoxliteCLICommand(requests []boxlitecli.CommandRequest, command string) int {
	var count int
	for _, req := range requests {
		if len(req.Args) > 2 && req.Args[2] == command {
			count++
		}
	}
	return count
}

func hasBoxliteCLIExec(requests []boxlitecli.CommandRequest, values ...string) bool {
	for _, req := range requests {
		if len(req.Args) > 5 && req.Args[2] == "exec" && containsSubsequence(req.Args[5:], values) {
			return true
		}
	}
	return false
}

func hasBoxliteCLICommandArgs(requests []boxlitecli.CommandRequest, command string, values ...string) bool {
	for _, req := range requests {
		if len(req.Args) > 2 && req.Args[2] == command && containsSubsequence(req.Args[3:], values) {
			return true
		}
	}
	return false
}

func containsSubsequence(args []string, values []string) bool {
	if len(values) == 0 {
		return true
	}
	for idx := 0; idx <= len(args)-len(values); idx++ {
		matched := true
		for valueIdx, value := range values {
			if args[idx+valueIdx] != value {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func containsAny(args []string, values ...string) bool {
	for _, arg := range args {
		for _, value := range values {
			if arg == value {
				return true
			}
		}
	}
	return false
}

func requestArgs(requests []boxlitecli.CommandRequest) [][]string {
	out := make([][]string, 0, len(requests))
	for _, req := range requests {
		out = append(out, req.Args)
	}
	return out
}

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

	_, err = svc.CreateWorker(context.Background(), CreateAgentSpec{
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

	_, err = svc.CreateWorker(context.Background(), CreateAgentSpec{
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

func TestCreateRejectsDuplicateAgentIDWithoutReplace(t *testing.T) {
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	svc.agents["u-alice"] = Agent{
		ID:        "u-alice",
		Name:      "alice",
		Status:    "active",
		CreatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		Role:      RoleWorker,
	}

	_, err = svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:   "u-alice",
			Name: "alice-v2",
			Role: RoleWorker,
		},
	})
	if err == nil {
		t.Fatal("Create() duplicate error = nil, want duplicate-id error")
	}
	if !strings.Contains(err.Error(), `agent id "u-alice" already exists`) {
		t.Fatalf("Create() duplicate error = %q, want duplicate-id error", err)
	}
}

func TestCreateWorkerRejectsInvalidRuntime(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return nil, nil },
		nil,
	)
	defer ResetTestHooks()

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateWorker(context.Background(), CreateAgentSpec{Name: "alice"})
	if err == nil {
		t.Fatal("CreateWorker() error = nil, want invalid runtime error")
	}
	if !strings.Contains(err.Error(), "invalid sandbox runtime") {
		t.Fatalf("CreateWorker() error = %q, want invalid runtime error", err)
	}
}

func TestBoxLiteCLIProviderGatewayLifecycle(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	orig := localIPv4Resolver
	localIPv4Resolver = func() string { return "10.0.0.8" }
	defer func() { localIPv4Resolver = orig }()

	runner := newAgentBoxliteCLIRunner()
	provider := boxlitecli.NewProvider(boxlitecli.WithRunner(runner))
	statePath := filepath.Join(homeDir, "agents.json")
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"picoclaw:latest",
		statePath,
		WithSandboxProvider(provider),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	manager, err := svc.EnsureManager(context.Background(), false)
	if err != nil {
		t.Fatalf("EnsureManager() error = %v", err)
	}
	if manager.BoxID != "box-manager" || manager.Status != string(sandbox.StateRunning) {
		t.Fatalf("EnsureManager() = %+v, want running box-manager", manager)
	}

	worker, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		ID:   "u-alice",
		Name: "alice",
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if worker.BoxID != "box-alice" || worker.Status != string(sandbox.StateRunning) {
		t.Fatalf("CreateWorker() = %+v, want running box-alice", worker)
	}

	logPath, err := agentGatewayLogPath("alice")
	if err != nil {
		t.Fatalf("agentGatewayLogPath() error = %v", err)
	}
	if err := os.WriteFile(logPath, []byte("old line\nnew line\ngateway line\n"), 0o600); err != nil {
		t.Fatalf("write gateway log: %v", err)
	}

	var logs strings.Builder
	logCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.StreamLogs(logCtx, worker.ID, true, 1, cancelOnWrite{writer: &logs, cancel: cancel}); err != nil {
		t.Fatalf("StreamLogs() error = %v", err)
	}
	if got := logs.String(); got != "gateway line\n" {
		t.Fatalf("StreamLogs() output = %q, want gateway line", got)
	}

	if err := svc.Delete(context.Background(), worker.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if got, want := countBoxliteCLICommand(runner.requests, "run"), 2; got != want {
		t.Fatalf("run command count = %d, want %d", got, want)
	}
	if got, want := countBoxliteCLICommand(runner.requests, "start"), 0; got != want {
		t.Fatalf("start command count = %d, want %d", got, want)
	}
	if !hasBoxliteCLICommandArgs(runner.requests, "run", "/bin/sh", "-c", gatewayRunCommand()) {
		t.Fatalf("boxlite-cli gateway run command not found in requests: %#v", requestArgs(runner.requests))
	}
	if hasBoxliteCLIExec(runner.requests, "tail", "-n", "1", "-f", boxGatewayLogPath) {
		t.Fatalf("boxlite-cli tail exec should not be used for mounted gateway logs: %#v", requestArgs(runner.requests))
	}
	if !hasBoxliteCLICommandArgs(runner.requests, "rm", "-f", "box-alice") {
		t.Fatalf("boxlite-cli remove command not found in requests: %#v", requestArgs(runner.requests))
	}
	for _, req := range runner.requests {
		if len(req.Args) > 2 && req.Args[2] == "run" && !containsAny(req.Args, "/bin/sh", "/usr/local/bin/picoclaw") {
			t.Fatalf("boxlite-cli run args missing gateway command: %q", req.Args)
		}
	}
}

func TestEnsureBootstrapManagerStartsAfterSingleSuccessfulDetection(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	origLocalIPv4 := localIPv4Resolver
	localIPv4Resolver = func() string { return "10.0.0.8" }
	origCSGHubLiteURL := defaultCSGHubLiteBaseURL
	defaultCSGHubLiteBaseURL = "http://127.0.0.1:1/v1"
	origListCLIProxyModels := listCLIProxyModels
	var codexDetections int
	listCLIProxyModels = func(_ context.Context, provider string) ([]string, error) {
		if provider == ProviderCodex {
			codexDetections++
			if codexDetections == 1 {
				return []string{"gpt-auto"}, nil
			}
		}
		return nil, fmt.Errorf("%s unavailable", provider)
	}
	t.Cleanup(func() {
		localIPv4Resolver = origLocalIPv4
		defaultCSGHubLiteBaseURL = origCSGHubLiteURL
		listCLIProxyModels = origListCLIProxyModels
		ResetTestHooks()
	})

	var created bool
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Service, _ context.Context, _ sandbox.Runtime, image, name, botID string, profile AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			created = true
			if image != "picoclaw:latest" || name != ManagerName || botID != ManagerUserID {
				t.Fatalf("createGatewayBox() got image=%q name=%q botID=%q", image, name, botID)
			}
			if profile.Provider != ProviderCodex || profile.ModelID != "gpt-auto" || !profile.ProfileComplete {
				t.Fatalf("createGatewayBox() profile = %+v, want complete codex gpt-auto", profile)
			}
			return &fakeInstance{}, sandbox.Info{
				ID:        "box-manager",
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		if created {
			return &fakeInfoInstance{info: sandbox.Info{
				ID:        "box-manager",
				Name:      ManagerName,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}}, nil
		}
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{ListenAddr: ":18080", AccessToken: "token"}, "picoclaw:latest", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := svc.EnsureBootstrapManager(context.Background(), false); err != nil {
		t.Fatalf("EnsureBootstrapManager() error = %v", err)
	}
	got, ok := svc.Agent(ManagerUserID)
	if !ok {
		t.Fatal("manager agent not saved")
	}
	if got.Status != string(sandbox.StateRunning) || got.ModelID != "gpt-auto" || !got.ProfileComplete {
		t.Fatalf("manager = %+v, want running with detected model", got)
	}
	if codexDetections != 1 {
		t.Fatalf("codex detections = %d, want exactly one detection before manager start", codexDetections)
	}
}

func TestCreateReplaceWorkerRecreatesExistingAgent(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	orig := localIPv4Resolver
	localIPv4Resolver = func() string { return "10.0.0.8" }
	defer func() { localIPv4Resolver = orig }()

	runner := newAgentBoxliteCLIRunner()
	provider := boxlitecli.NewProvider(boxlitecli.WithRunner(runner))
	statePath := filepath.Join(homeDir, "agents.json")
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"picoclaw:latest",
		statePath,
		WithSandboxProvider(provider),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	created, err := svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:   "u-alice",
			Name: "alice",
		},
	})
	if err != nil {
		t.Fatalf("Create() seed error = %v", err)
	}
	if created.Role != RoleWorker {
		t.Fatalf("Create() role = %q, want %q", created.Role, RoleWorker)
	}

	replaced, err := svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:   "u-alice",
			Name: "alice-v2",
		},
		Replace: true,
	})
	if err != nil {
		t.Fatalf("Create() replace error = %v", err)
	}
	if replaced.ID != "u-alice" || replaced.Name != "alice-v2" || replaced.Role != RoleWorker {
		t.Fatalf("Create() replaced = %+v, want replaced worker", replaced)
	}
	if !hasBoxliteCLICommandArgs(runner.requests, "rm", "-f", "box-alice") {
		t.Fatalf("boxlite-cli remove command not found in requests: %#v", requestArgs(runner.requests))
	}
	if !hasBoxliteCLICommandArgs(runner.requests, "run", "--name", "alice-v2") {
		t.Fatalf("boxlite-cli run command for alice-v2 not found in requests: %#v", requestArgs(runner.requests))
	}
}

func TestCreateReplaceRequiresExistingAgent(t *testing.T) {
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:   "u-missing",
			Name: "missing",
		},
		Replace: true,
	})
	if err == nil {
		t.Fatal("Create() error = nil, want missing agent error")
	}
	if !strings.Contains(err.Error(), `agent "u-missing" not found`) {
		t.Fatalf("Create() error = %q, want missing agent error", err)
	}
}

func TestCreateReplaceFieldMaskMergesExistingAgent(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	runner := newAgentBoxliteCLIRunner()
	provider := boxlitecli.NewProvider(boxlitecli.WithRunner(runner))
	statePath := filepath.Join(homeDir, "agents.json")
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"picoclaw:latest",
		statePath,
		WithSandboxProvider(provider),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if _, err := svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:          "u-alice",
			Name:        "alice",
			Description: "worker",
			Image:       "agent-image:v1",
		},
	}); err != nil {
		t.Fatalf("Create() seed error = %v", err)
	}

	replaced, err := svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:          "u-alice",
			Name:        "alice-v2",
			Description: "",
			Image:       "agent-image:v2",
		},
		Replace:   true,
		FieldMask: []string{"id", "name"},
	})
	if err != nil {
		t.Fatalf("Create() replace error = %v", err)
	}
	if replaced.ID != "u-alice" || replaced.Name != "alice-v2" {
		t.Fatalf("Create() replaced = %+v, want id u-alice name alice-v2", replaced)
	}
	if replaced.Description != "worker" {
		t.Fatalf("Create() description = %q, want preserved description", replaced.Description)
	}
	if replaced.Image != "agent-image:v1" {
		t.Fatalf("Create() image = %q, want preserved image", replaced.Image)
	}
}

func TestCreateReplaceManagerUsesRequestedImage(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	var gotImages []string
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Service, _ context.Context, _ sandbox.Runtime, image, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			gotImages = append(gotImages, image)
			return &fakeInstance{}, sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string) error {
		return nil
	}
	defer ResetTestHooks()

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:1", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if _, err := svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:   ManagerUserID,
			Name: ManagerName,
		},
	}); err != nil {
		t.Fatalf("seed Create() error = %v", err)
	}

	replaced, err := svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:    ManagerUserID,
			Name:  ManagerName,
			Image: "manager-image:2",
		},
		Replace:   true,
		FieldMask: []string{"id", "image"},
	})
	if err != nil {
		t.Fatalf("Create() replace error = %v", err)
	}
	if len(gotImages) != 2 {
		t.Fatalf("createGatewayBox() calls = %d, want 2", len(gotImages))
	}
	if gotImages[0] != "manager-image:1" || gotImages[1] != "manager-image:2" {
		t.Fatalf("createGatewayBox() images = %#v, want manager-image:1 then manager-image:2", gotImages)
	}
	if replaced.Image != "manager-image:2" {
		t.Fatalf("Create() image = %q, want requested image", replaced.Image)
	}
}

func TestCreateReplaceManagerWithoutRequestedImageUsesManagerDefault(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	var gotImages []string
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Service, _ context.Context, _ sandbox.Runtime, image, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			gotImages = append(gotImages, image)
			return &fakeInstance{}, sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string) error {
		return nil
	}
	defer ResetTestHooks()

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:1", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:        ManagerUserID,
		Name:      ManagerName,
		Image:     "old-manager-image:0",
		Role:      RoleManager,
		Status:    string(sandbox.StateRunning),
		CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
	}

	replaced, err := svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:   ManagerUserID,
			Name: ManagerName,
		},
		Replace:   true,
		FieldMask: []string{"id"},
	})
	if err != nil {
		t.Fatalf("Create() replace error = %v", err)
	}
	if len(gotImages) != 1 || gotImages[0] != "manager-image:1" {
		t.Fatalf("createGatewayBox() images = %#v, want manager-image:1", gotImages)
	}
	if replaced.Image != "manager-image:1" {
		t.Fatalf("Create() image = %q, want manager default image", replaced.Image)
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

func TestDeleteAllowsManagerAgent(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return nil, nil },
		nil,
	)
	defer ResetTestHooks()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	svc.agents[ManagerUserID] = Agent{
		ID:        ManagerUserID,
		Name:      ManagerName,
		Role:      RoleManager,
		CreatedAt: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
	}
	if err := svc.saveLocked(); err != nil {
		t.Fatalf("saveLocked() error = %v", err)
	}

	if err := svc.Delete(context.Background(), ManagerUserID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := svc.Agent(ManagerUserID); ok {
		t.Fatal("Agent() ok = true, want false after delete")
	}

	reloaded, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() reload error = %v", err)
	}
	if _, ok := reloaded.Agent(ManagerUserID); ok {
		t.Fatal("reloaded Agent() ok = true, want false after delete")
	}
}

func TestDeleteRemovesAgentFromState(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return nil, nil },
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

func TestSaveLockedPersistsLastKnownAgentStatus(t *testing.T) {
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
		BoxID:     "box-alice",
		Status:    "running",
		CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	if err := svc.saveLocked(); err != nil {
		t.Fatalf("saveLocked() error = %v", err)
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), `"status": "running"`) {
		t.Fatalf("saved state should contain last known status: %s", data)
	}
}

func TestListKeepsLastKnownStatusWhenHydrationFails(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) {
			return nil, fmt.Errorf("runtime lock")
		},
		nil,
	)
	defer ResetTestHooks()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:        "u-alice",
		Name:      "alice",
		Role:      RoleWorker,
		BoxID:     "box-alice",
		Status:    string(sandbox.StateRunning),
		CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	got := svc.List()
	if len(got) != 1 {
		t.Fatalf("List() len = %d, want 1", len(got))
	}
	if got[0].Status != string(sandbox.StateRunning) {
		t.Fatalf("List()[0].Status = %q, want running", got[0].Status)
	}
}

func TestIsSandboxRuntimeContentionRecognizesBoxLiteLockErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "failed acquire runtime lock",
			err:  fmt.Errorf("inspect boxlite cli box: Error: internal error: Failed to acquire runtime lock at /tmp/boxlite"),
			want: true,
		},
		{
			name: "runtime already using directory",
			err:  fmt.Errorf("get agent box: internal error: Another BoxliteRuntime is already using directory: /tmp/boxlite"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  fmt.Errorf("network down"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSandboxRuntimeContention(tc.err); got != tc.want {
				t.Fatalf("isSandboxRuntimeContention(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestLoadLegacyAgentWithBoxIDInfersRunningUntilHydrated(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:        "u-alice",
				Name:      "alice",
				Role:      RoleWorker,
				BoxID:     "box-alice",
				CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) {
			return nil, fmt.Errorf("runtime lock")
		},
		nil,
	)
	defer ResetTestHooks()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	got, ok := svc.Agent("u-alice")
	if !ok {
		t.Fatal("Agent() ok = false, want true")
	}
	if got.Status != string(sandbox.StateRunning) {
		t.Fatalf("Agent().Status = %q, want running fallback", got.Status)
	}
}

func TestDeleteRemovesAgentHomeDirectory(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return nil, nil },
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
		func(_ *Service, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		nil,
	)
	defer ResetTestHooks()

	var removed string
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) error {
		removed = idOrName
		return nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
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
		func(_ *Service, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		nil,
	)
	defer ResetTestHooks()

	var lookedUp []string
	var removed string
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		lookedUp = append(lookedUp, idOrName)
		if idOrName == "alice" {
			return &fakeInstance{}, nil
		}
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) error {
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
	rt := &fakeRuntime{}
	SetTestHooks(func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil }, nil)
	defer ResetTestHooks()
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string) error {
		return nil
	}
	var closeRuntimeCalls int
	testCloseRuntimeHook = func(_ *Service, home string, got sandbox.Runtime) error {
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

	runtimeHome, err := sandboxRuntimeHome("alice")
	if err != nil {
		t.Fatalf("sandboxRuntimeHome() error = %v", err)
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

func TestDeleteRetriesAgentHomeRemovalOnDirectoryNotEmpty(t *testing.T) {
	rt := &fakeRuntime{}
	SetTestHooks(func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil }, nil)
	defer ResetTestHooks()
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string) error {
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

	agentHome, err := agentHomeDir("alice")
	if err != nil {
		t.Fatalf("agentHomeDir() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(agentHome, "boxlite", "images"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(agentHome) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentHome, "boxlite", "images", "cache.txt"), []byte("cache"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(cache.txt) error = %v", err)
	}

	origRemoveAll := osRemoveAll
	var removeCalls int
	osRemoveAll = func(path string) error {
		removeCalls++
		if path == agentHome && removeCalls == 1 {
			return &os.PathError{Op: "unlinkat", Path: filepath.Join(agentHome, "boxlite", "images"), Err: syscall.ENOTEMPTY}
		}
		return os.RemoveAll(path)
	}
	defer func() {
		osRemoveAll = origRemoveAll
	}()

	if err := svc.Delete(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if removeCalls < 2 {
		t.Fatalf("osRemoveAll() calls = %d, want at least 2", removeCalls)
	}
	if _, err := os.Stat(agentHome); !os.IsNotExist(err) {
		t.Fatalf("agent home still exists after delete: err=%v", err)
	}
}

func TestCreateWorkerStoresBoxID(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return nil, nil },
		func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			return nil, sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	defer ResetTestHooks()

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{Name: "alice"})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.BoxID != "box-alice" {
		t.Fatalf("CreateWorker().BoxID = %q, want %q", got.BoxID, "box-alice")
	}
}

func TestCreateWorkerUsesRequestedImageOrManagerFallback(t *testing.T) {
	tests := []struct {
		name      string
		reqImage  string
		wantImage string
	}{
		{name: "requested image", reqImage: "worker-image:2", wantImage: "worker-image:2"},
		{name: "manager fallback", reqImage: "", wantImage: "manager-image:1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotImage string
			SetTestHooks(
				func(_ *Service, _ string) (sandbox.Runtime, error) { return nil, nil },
				func(_ *Service, _ context.Context, _ sandbox.Runtime, image, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
					gotImage = image
					return nil, sandbox.Info{
						ID:        "box-" + name,
						Name:      name,
						State:     sandbox.StateRunning,
						CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
					}, nil
				},
			)
			defer ResetTestHooks()

			svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:1", "")
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}

			got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{Name: "alice", Image: tt.reqImage})
			if err != nil {
				t.Fatalf("CreateWorker() error = %v", err)
			}
			if gotImage != tt.wantImage {
				t.Fatalf("createGatewayBox() image = %q, want %q", gotImage, tt.wantImage)
			}
			if got.Image != tt.wantImage {
				t.Fatalf("CreateWorker().Image = %q, want %q", got.Image, tt.wantImage)
			}
		})
	}
}

func TestCreateWorkerStoresResolvedProfileSnapshot(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return nil, nil },
		func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			return nil, sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
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

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		Name:    "alice",
		Profile: "remote-main",
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.Profile != "api.gpt-5.4" {
		t.Fatalf("CreateWorker().Profile = %q, want %q", got.Profile, "api.gpt-5.4")
	}
	if got.Provider != ProviderAPI {
		t.Fatalf("CreateWorker().Provider = %q, want %q", got.Provider, ProviderAPI)
	}
	if got.ModelID != "gpt-5.4" {
		t.Fatalf("CreateWorker().ModelID = %q, want %q", got.ModelID, "gpt-5.4")
	}
	if got.ReasoningEffort != "medium" {
		t.Fatalf("CreateWorker().ReasoningEffort = %q, want %q", got.ReasoningEffort, "medium")
	}
}

func TestCreateWorkerClosesBoxHandleAfterCreate(t *testing.T) {
	rt := &fakeRuntime{}
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil },
		func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			return &fakeInstance{}, sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	defer ResetTestHooks()

	var closeCalls int
	var closeRuntimeCalls int
	testCloseBoxHook = func(_ *Service, _ sandbox.Instance) error {
		closeCalls++
		return nil
	}
	testCloseRuntimeHook = func(_ *Service, _ string, got sandbox.Runtime) error {
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

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{Name: "alice"})
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

func TestStreamLogsFallsBackToSandboxTailWhenHostLogIsMissing(t *testing.T) {
	rt := &fakeRuntime{}
	SetTestHooks(func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil }, nil)
	defer ResetTestHooks()

	var gotBoxID string
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		gotBoxID = idOrName
		return &fakeInstance{}, nil
	}
	var gotName string
	var gotArgs []string
	testRunBoxCommandHook = func(_ *Service, _ context.Context, _ sandbox.Instance, name string, args []string, w io.Writer) (int, error) {
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
	if err := svc.StreamLogs(context.Background(), "u-alice", false, 50, &out); err != nil {
		t.Fatalf("StreamLogs() error = %v", err)
	}
	if gotBoxID != "box-123" {
		t.Fatalf("getBox() idOrName = %q, want %q", gotBoxID, "box-123")
	}
	if gotName != "tail" {
		t.Fatalf("runBoxCommand() name = %q, want %q", gotName, "tail")
	}
	if strings.Join(gotArgs, " ") != "-n 50 "+boxGatewayLogPath {
		t.Fatalf("runBoxCommand() args = %q", gotArgs)
	}
	if out.String() != "line-1\n" {
		t.Fatalf("output = %q, want streamed log line", out.String())
	}
}

func TestStreamLogsFollowUsesHostGatewayLogWithoutSandboxRuntime(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

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
	logPath, err := agentGatewayLogPath("alice")
	if err != nil {
		t.Fatalf("agentGatewayLogPath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("create log dir: %v", err)
	}
	if err := os.WriteFile(logPath, []byte("older\nready\n"), 0o600); err != nil {
		t.Fatalf("write gateway log: %v", err)
	}

	testEnsureRuntimeHook = func(*Service, string) (sandbox.Runtime, error) {
		t.Fatal("StreamLogs follow opened sandbox runtime; want host log streaming")
		return nil, nil
	}
	defer func() { testEnsureRuntimeHook = nil }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out strings.Builder
	if err := svc.StreamLogs(ctx, "u-alice", true, 1, cancelOnWrite{writer: &out, cancel: cancel}); err != nil {
		t.Fatalf("StreamLogs() error = %v", err)
	}
	if out.String() != "ready\n" {
		t.Fatalf("output = %q, want last host log line", out.String())
	}
}

func TestStreamLogsFallsBackToNameAndRefreshesStoredBoxID(t *testing.T) {
	rt := &fakeRuntime{}
	SetTestHooks(func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil }, nil)
	defer ResetTestHooks()

	var gotKeys []string
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		gotKeys = append(gotKeys, idOrName)
		if idOrName == "alice" {
			return &fakeInstance{}, nil
		}
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testBoxInfoHook = func(_ *Service, _ context.Context, _ sandbox.Instance) (sandbox.Info, error) {
		return sandbox.Info{ID: "box-new"}, nil
	}
	testRunBoxCommandHook = func(_ *Service, _ context.Context, _ sandbox.Instance, name string, args []string, w io.Writer) (int, error) {
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
	if len(gotKeys) < 2 || gotKeys[0] != "box-stale" || gotKeys[1] != "alice" {
		t.Fatalf("getBox() leading keys = %q, want stale box id then name fallback", gotKeys)
	}
	got, ok := svc.Agent("u-alice")
	if !ok {
		t.Fatal("Agent() missing u-alice after StreamLogs()")
	}
	if got.BoxID != "box-new" {
		t.Fatalf("Agent().BoxID = %q, want %q", got.BoxID, "box-new")
	}
}

func TestStartFallsBackToNameAndRefreshesStoredAgentState(t *testing.T) {
	rt := &fakeRuntime{}
	SetTestHooks(func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil }, nil)
	defer ResetTestHooks()

	var gotKeys []string
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		gotKeys = append(gotKeys, idOrName)
		if idOrName == "alice" {
			return &fakeInstance{}, nil
		}
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	var startCalls int
	testStartBoxHook = func(_ *Service, _ context.Context, _ sandbox.Instance) error {
		startCalls++
		return nil
	}
	testBoxInfoHook = func(_ *Service, _ context.Context, _ sandbox.Instance) (sandbox.Info, error) {
		return sandbox.Info{ID: "box-new", State: sandbox.StateRunning}, nil
	}
	defer func() {
		testGetBoxHook = nil
		testStartBoxHook = nil
		testBoxInfoHook = nil
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
		Status:    "stopped",
		CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	got, err := svc.Start(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if len(gotKeys) < 2 || gotKeys[0] != "box-stale" || gotKeys[1] != "alice" {
		t.Fatalf("getBox() leading keys = %q, want stale box id then name fallback", gotKeys)
	}
	if startCalls != 1 {
		t.Fatalf("startBox() calls = %d, want 1", startCalls)
	}
	if got.BoxID != "box-new" {
		t.Fatalf("Start().BoxID = %q, want %q", got.BoxID, "box-new")
	}
	if got.Status != "running" {
		t.Fatalf("Start().Status = %q, want %q", got.Status, "running")
	}

	reloaded, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService(reload) error = %v", err)
	}
	persisted, ok := reloaded.Agent("u-alice")
	if !ok {
		t.Fatal("reloaded Agent() missing u-alice")
	}
	if persisted.BoxID != "box-new" || persisted.Status != "running" {
		t.Fatalf("reloaded Agent() = %+v, want refreshed box id/status", persisted)
	}
}

func TestStartRefreshesCompleteWorkerGatewayConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	orig := localIPv4Resolver
	localIPv4Resolver = func() string { return "10.0.0.8" }
	defer func() { localIPv4Resolver = orig }()

	rt := &fakeRuntime{}
	SetTestHooks(func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil }, nil)
	defer ResetTestHooks()

	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		if idOrName != "box-alice" {
			t.Fatalf("getBox() idOrName = %q, want box-alice", idOrName)
		}
		return &fakeInfoInstance{info: sandbox.Info{
			ID:        "box-alice",
			Name:      "alice",
			State:     sandbox.StateRunning,
			CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
		}}, nil
	}
	testStartBoxHook = func(_ *Service, _ context.Context, _ sandbox.Instance) error {
		return nil
	}
	testBoxInfoHook = func(_ *Service, _ context.Context, box sandbox.Instance) (sandbox.Info, error) {
		return box.Info(context.Background())
	}
	defer func() {
		testGetBoxHook = nil
		testStartBoxHook = nil
		testBoxInfoHook = nil
	}()

	svc, err := NewService(testModelConfig(), config.ServerConfig{ListenAddr: ":18080", AccessToken: "token"}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:              "u-alice",
		Name:            "alice",
		Role:            RoleWorker,
		BoxID:           "box-alice",
		Status:          string(sandbox.StateRunning),
		AgentProfile:    AgentProfile{Name: "alice", Provider: ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true},
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	if _, err := svc.Start(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	configPath := filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, "alice", hostWorkspaceDir, filepath.FromSlash(hostPicoClawStateDir), hostPicoClawConfig)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(worker config) error = %v", err)
	}
	text := string(data)
	for _, want := range []string{`"bot_id": "u-alice"`, `"model_name": "gpt-5.5"`, `"api_base": "http://10.0.0.8:18080/api/bots/u-alice/llm"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("worker config missing %q in:\n%s", want, text)
		}
	}
}

func TestStartConfiguredAgentsRecreatesMissingCompleteWorkerBoxes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rt := &fakeRuntime{}
	boxes := map[string]sandbox.Info{}
	var created []string
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil },
		func(_ *Service, _ context.Context, _ sandbox.Runtime, image, name, botID string, profile AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			if image != "worker-image:1" {
				t.Fatalf("createGatewayBox() image = %q, want %q", image, "worker-image:1")
			}
			if botID != "u-alice" {
				t.Fatalf("createGatewayBox() botID = %q, want %q", botID, "u-alice")
			}
			if !profile.ProfileComplete || profile.Provider != ProviderCodex || profile.ModelID != "gpt-5.5" {
				t.Fatalf("createGatewayBox() profile = %+v, want complete codex gpt-5.5", profile)
			}
			created = append(created, name)
			info := sandbox.Info{
				ID:        "box-alice-new",
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}
			boxes[info.ID] = info
			boxes[name] = info
			return &fakeInfoInstance{info: info}, info, nil
		},
	)
	defer ResetTestHooks()

	var gotKeys []string
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		gotKeys = append(gotKeys, idOrName)
		info, ok := boxes[idOrName]
		if !ok {
			return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
		}
		return &fakeInfoInstance{info: info}, nil
	}
	testBoxInfoHook = func(_ *Service, _ context.Context, box sandbox.Instance) (sandbox.Info, error) {
		return box.Info(context.Background())
	}
	var removed []string
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) error {
		removed = append(removed, idOrName)
		return fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:1", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	completeAlice := AgentProfile{Name: "alice", Provider: ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true}
	svc.agents["u-alice"] = Agent{
		ID:              "u-alice",
		Name:            "alice",
		Role:            RoleWorker,
		Image:           "worker-image:1",
		BoxID:           "box-alice-stale",
		Status:          string(sandbox.StateRunning),
		AgentProfile:    completeAlice,
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	if err := svc.StartConfiguredAgents(context.Background()); err != nil {
		t.Fatalf("StartConfiguredAgents() error = %v", err)
	}
	if strings.Join(created, ",") != "alice" {
		t.Fatalf("created boxes = %q, want alice", created)
	}
	if strings.Join(removed, ",") != "box-alice-stale" {
		t.Fatalf("removed boxes = %q, want stale box id", removed)
	}
	if len(gotKeys) < 2 || gotKeys[0] != "box-alice-stale" || gotKeys[1] != "alice" {
		t.Fatalf("getBox() leading keys = %q, want stale box id then name", gotKeys)
	}
	got, ok := svc.Agent("u-alice")
	if !ok {
		t.Fatal("Agent() missing u-alice")
	}
	if got.BoxID != "box-alice-new" {
		t.Fatalf("Agent().BoxID = %q, want %q", got.BoxID, "box-alice-new")
	}
	if got.Status != string(sandbox.StateRunning) {
		t.Fatalf("Agent().Status = %q, want running", got.Status)
	}

	reloaded, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:1", statePath)
	if err != nil {
		t.Fatalf("NewService(reload) error = %v", err)
	}
	persisted, ok := reloaded.Agent("u-alice")
	if !ok {
		t.Fatal("reloaded Agent() missing u-alice")
	}
	if persisted.BoxID != "box-alice-new" {
		t.Fatalf("reloaded Agent().BoxID = %q, want %q", persisted.BoxID, "box-alice-new")
	}
}

func TestStartConfiguredAgentsStartsStoppedAndRecreatesRunningCompleteWorkers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rt := &fakeRuntime{}
	infos := map[string]sandbox.Info{
		"box-alice": {
			ID:        "box-alice",
			Name:      "alice",
			State:     sandbox.StateStopped,
			CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
		},
		"box-carol": {
			ID:        "box-carol",
			Name:      "carol",
			State:     sandbox.StateRunning,
			CreatedAt: time.Date(2026, 4, 1, 13, 0, 0, 0, time.UTC),
		},
	}
	var recreated []string
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil },
		func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string, name, botID string, profile AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			recreated = append(recreated, name)
			if name != "carol" || botID != "u-carol" {
				t.Fatalf("createGatewayBox() got name=%q botID=%q, want carol/u-carol", name, botID)
			}
			if !profile.ProfileComplete || profile.Provider != ProviderCodex || profile.ModelID != "gpt-5.5" {
				t.Fatalf("createGatewayBox() profile = %+v, want complete codex gpt-5.5", profile)
			}
			info := sandbox.Info{
				ID:        "box-carol-new",
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 14, 0, 0, 0, time.UTC),
			}
			infos[info.ID] = info
			infos[name] = info
			return &fakeInfoInstance{info: info}, info, nil
		},
	)
	defer ResetTestHooks()
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		info, ok := infos[idOrName]
		if !ok {
			return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
		}
		return &fakeInfoInstance{info: info}, nil
	}
	var started []string
	testStartBoxHook = func(_ *Service, _ context.Context, box sandbox.Instance) error {
		info, err := box.Info(context.Background())
		if err != nil {
			return err
		}
		started = append(started, info.ID)
		info.State = sandbox.StateRunning
		infos[info.ID] = info
		infos[info.Name] = info
		return nil
	}
	testBoxInfoHook = func(_ *Service, _ context.Context, box sandbox.Instance) (sandbox.Info, error) {
		info, err := box.Info(context.Background())
		if err != nil {
			return sandbox.Info{}, err
		}
		if current, ok := infos[info.ID]; ok {
			return current, nil
		}
		return info, nil
	}
	var removed []string
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) error {
		removed = append(removed, idOrName)
		if info, ok := infos[idOrName]; ok {
			delete(infos, info.ID)
			delete(infos, info.Name)
			return nil
		}
		return fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	completeManager := AgentProfile{Name: ManagerName, Provider: ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true}
	completeAlice := AgentProfile{Name: "alice", Provider: ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true}
	completeCarol := AgentProfile{Name: "carol", Provider: ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true}
	incompleteBob := AgentProfile{Name: "bob"}
	svc.agents[ManagerUserID] = Agent{
		ID:              ManagerUserID,
		Name:            ManagerName,
		Role:            RoleManager,
		BoxID:           "box-manager",
		AgentProfile:    completeManager,
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
	}
	svc.agents["u-alice"] = Agent{
		ID:              "u-alice",
		Name:            "alice",
		Role:            RoleWorker,
		BoxID:           "box-alice",
		AgentProfile:    completeAlice,
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}
	svc.agents["u-bob"] = Agent{
		ID:              "u-bob",
		Name:            "bob",
		Role:            RoleWorker,
		BoxID:           "box-bob",
		AgentProfile:    incompleteBob,
		ProfileComplete: false,
		CreatedAt:       time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
	}
	svc.agents["u-carol"] = Agent{
		ID:              "u-carol",
		Name:            "carol",
		Role:            RoleWorker,
		BoxID:           "box-carol",
		Status:          string(sandbox.StateRunning),
		AgentProfile:    completeCarol,
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 4, 1, 13, 0, 0, 0, time.UTC),
	}

	if err := svc.StartConfiguredAgents(context.Background()); err != nil {
		t.Fatalf("StartConfiguredAgents() error = %v", err)
	}
	if strings.Join(started, ",") != "box-alice" {
		t.Fatalf("started boxes = %q, want only box-alice", started)
	}
	if strings.Join(recreated, ",") != "carol" {
		t.Fatalf("recreated boxes = %q, want carol", recreated)
	}
	if strings.Join(removed, ",") != "box-carol" {
		t.Fatalf("removed boxes = %q, want box-carol", removed)
	}
	got, ok := svc.Agent("u-alice")
	if !ok {
		t.Fatal("Agent() missing u-alice")
	}
	if got.Status != string(sandbox.StateRunning) {
		t.Fatalf("Agent().Status = %q, want running", got.Status)
	}
	carol, ok := svc.Agent("u-carol")
	if !ok {
		t.Fatal("Agent() missing u-carol")
	}
	if carol.BoxID != "box-carol-new" {
		t.Fatalf("Agent(u-carol).BoxID = %q, want box-carol-new", carol.BoxID)
	}
	if carol.Status != string(sandbox.StateRunning) {
		t.Fatalf("Agent(u-carol).Status = %q, want running", carol.Status)
	}
}

func TestStopFallsBackToNameAndRefreshesStoredAgentState(t *testing.T) {
	rt := &fakeRuntime{}
	SetTestHooks(func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil }, nil)
	defer ResetTestHooks()

	var gotKeys []string
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		gotKeys = append(gotKeys, idOrName)
		if idOrName == "alice" {
			return &fakeInstance{}, nil
		}
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	var stopCalls int
	testStopBoxHook = func(_ *Service, _ context.Context, _ sandbox.Instance, opts sandbox.StopOptions) error {
		stopCalls++
		if opts != (sandbox.StopOptions{}) {
			t.Fatalf("Stop() opts = %+v, want zero value", opts)
		}
		return nil
	}
	testBoxInfoHook = func(_ *Service, _ context.Context, _ sandbox.Instance) (sandbox.Info, error) {
		return sandbox.Info{ID: "box-new", State: sandbox.StateStopped}, nil
	}
	defer func() {
		testGetBoxHook = nil
		testStopBoxHook = nil
		testBoxInfoHook = nil
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

	got, err := svc.Stop(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if len(gotKeys) < 2 || gotKeys[0] != "box-stale" || gotKeys[1] != "alice" {
		t.Fatalf("getBox() leading keys = %q, want stale box id then name fallback", gotKeys)
	}
	if stopCalls != 1 {
		t.Fatalf("stopBox() calls = %d, want 1", stopCalls)
	}
	if got.BoxID != "box-new" {
		t.Fatalf("Stop().BoxID = %q, want %q", got.BoxID, "box-new")
	}
	if got.Status != "stopped" {
		t.Fatalf("Stop().Status = %q, want %q", got.Status, "stopped")
	}

	reloaded, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService(reload) error = %v", err)
	}
	persisted, ok := reloaded.Agent("u-alice")
	if !ok {
		t.Fatal("reloaded Agent() missing u-alice")
	}
	if persisted.BoxID != "box-new" || persisted.Status != "stopped" {
		t.Fatalf("reloaded Agent() = %+v, want refreshed box id/status", persisted)
	}
}

func TestCreateClosesBoxHandleAfterCreate(t *testing.T) {
	rt := &fakeRuntime{}
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil },
		nil,
	)
	defer ResetTestHooks()

	var closeCalls int
	var closeRuntimeCalls int
	testCreateBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ sandbox.CreateSpec) (sandbox.Instance, error) {
		return &fakeInstance{}, nil
	}
	testCloseBoxHook = func(_ *Service, _ sandbox.Instance) error {
		closeCalls++
		return nil
	}
	testCloseRuntimeHook = func(_ *Service, _ string, got sandbox.Runtime) error {
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
		Spec: CreateAgentSpec{
			ID:    "agent-1",
			Name:  "alice",
			Image: "test-image",
			Role:  RoleAgent,
		},
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

func TestCreateUsesRequestedImageOrManagerFallback(t *testing.T) {
	tests := []struct {
		name      string
		reqImage  string
		wantImage string
	}{
		{name: "requested image", reqImage: "agent-image:2", wantImage: "agent-image:2"},
		{name: "manager fallback", reqImage: "", wantImage: "manager-image:1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &fakeRuntime{}
			var gotSpec sandbox.CreateSpec
			SetTestHooks(
				func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil },
				nil,
			)
			defer ResetTestHooks()

			testCreateBoxHook = func(_ *Service, _ context.Context, gotRT sandbox.Runtime, spec sandbox.CreateSpec) (sandbox.Instance, error) {
				if gotRT != rt {
					t.Fatalf("createBox() runtime = %p, want %p", gotRT, rt)
				}
				gotSpec = spec
				return &fakeInstance{}, nil
			}

			svc, err := NewService(
				config.ModelConfig{BaseURL: "http://127.0.0.1:4000", APIKey: "sk-test", ModelID: "model-1"},
				config.ServerConfig{},
				"manager-image:1",
				"",
			)
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}

			got, err := svc.Create(context.Background(), CreateRequest{
				Spec: CreateAgentSpec{
					ID:    "agent-1",
					Name:  "alice",
					Image: tt.reqImage,
					Role:  RoleAgent,
				},
			})
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if gotSpec.Image != tt.wantImage {
				t.Fatalf("createBox() spec.Image = %q, want %q", gotSpec.Image, tt.wantImage)
			}
			if got.Image != tt.wantImage {
				t.Fatalf("Create().Image = %q, want %q", got.Image, tt.wantImage)
			}
		})
	}
}

func TestEnsureBootstrapStateForceRecreatePrefersStoredManagerBoxID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			return &fakeInstance{}, sandbox.Info{
				ID:        "box-new",
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	defer ResetTestHooks()

	var removed string
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) error {
		removed = idOrName
		return nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	defer func() {
		testForceRemoveBoxHook = nil
	}()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:        ManagerUserID,
				Name:      ManagerName,
				Role:      RoleManager,
				BoxID:     "box-old",
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

	runtimeHome, err := sandboxRuntimeHome(ManagerName)
	if err != nil {
		t.Fatalf("sandboxRuntimeHome() error = %v", err)
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
	testEnsureRuntimeAtHomeHook = func(_ *Service, home string) (sandbox.Runtime, error) {
		ensuredHomes = append(ensuredHomes, home)
		return &fakeRuntime{}, nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string) error {
		return nil
	}
	testCreateGatewayBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
		if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
			t.Fatalf("stale manager file still exists before recreate: err=%v", err)
		}
		return &fakeInstance{}, sandbox.Info{
			ID:        "box-new",
			Name:      name,
			State:     sandbox.StateRunning,
			CreatedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
		}, nil
	}
	testCloseRuntimeHook = func(_ *Service, gotHome string, _ sandbox.Runtime) error {
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
		Agents: []persistedAgent{
			{
				ID:        ManagerUserID,
				Name:      ManagerName,
				Role:      RoleManager,
				BoxID:     "box-old",
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
	t.Setenv("HOME", t.TempDir())

	rt := &fakeRuntime{}
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil },
		func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			return &fakeInstance{}, sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	defer ResetTestHooks()

	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}

	var closeCalls int
	var closeRuntimeCalls int
	testCloseBoxHook = func(_ *Service, _ sandbox.Instance) error {
		closeCalls++
		return nil
	}
	testCloseRuntimeHook = func(_ *Service, _ string, got sandbox.Runtime) error {
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
	t.Setenv("HOME", t.TempDir())

	SetTestHooks(nil, nil)
	defer ResetTestHooks()

	primaryRT := &fakeRuntime{}
	testEnsureRuntimeAtHomeHook = func(_ *Service, home string) (sandbox.Runtime, error) {
		return primaryRT, nil
	}

	var created bool
	testCreateGatewayBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string, _ string, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
		created = true
		return nil, sandbox.Info{}, nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, rt sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		if rt == primaryRT && idOrName == "box-old" {
			return &fakeInstance{}, nil
		}
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testStartBoxHook = func(_ *Service, _ context.Context, _ sandbox.Instance) error { return nil }
	testBoxInfoHook = func(_ *Service, _ context.Context, _ sandbox.Instance) (sandbox.Info, error) {
		return sandbox.Info{
			ID:        "box-old",
			Name:      ManagerName,
			State:     sandbox.StateRunning,
			CreatedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
		}, nil
	}

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:        ManagerUserID,
				Name:      ManagerName,
				Role:      RoleManager,
				BoxID:     "box-old",
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

	got, err := sandboxRuntimeHome("alice")
	if err != nil {
		t.Fatalf("sandboxRuntimeHome() error = %v", err)
	}

	want := filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, "alice", config.RuntimeHomeDirName)
	if got != want {
		t.Fatalf("sandboxRuntimeHome() = %q, want %q", got, want)
	}
}

func TestLookupBootstrapManagerUsesPerAgentHome(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	var gotHome string
	testEnsureRuntimeAtHomeHook = func(_ *Service, homeDir string) (sandbox.Runtime, error) {
		gotHome = homeDir
		return &fakeRuntime{}, nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
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

func TestLookupBootstrapManagerUsesStoredIDWhenConfigured(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	var lookedUp []string
	testEnsureRuntimeAtHomeHook = func(_ *Service, homeDir string) (sandbox.Runtime, error) {
		if homeDir == "" {
			t.Fatalf("ensureRuntimeAtHome() homeDir = %q, want non-empty", homeDir)
		}
		return &fakeRuntime{}, nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		lookedUp = append(lookedUp, idOrName)
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	defer func() {
		testEnsureRuntimeAtHomeHook = nil
		testGetBoxHook = nil
	}()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:     ManagerUserID,
		Name:   ManagerName,
		Role:   RoleManager,
		BoxID:  "box-stale",
		Status: "running",
	}

	_, _, err = svc.lookupBootstrapManager(context.Background())
	if err != nil {
		t.Fatalf("lookupBootstrapManager() error = %v", err)
	}
	if len(lookedUp) != 2 {
		t.Fatalf("lookupBootstrapManager() called times = %d, want %d", len(lookedUp), 2)
	}
	if lookedUp[0] != "box-stale" {
		t.Fatalf("lookupBootstrapManager() first lookup = %q, want %q", lookedUp[0], "box-stale")
	}
	if lookedUp[1] != ManagerName {
		t.Fatalf("lookupBootstrapManager() second lookup = %q, want %q", lookedUp[1], ManagerName)
	}
}

func TestLookupBootstrapManagerUsesManagerNameWhenNoStoredID(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("CSGCLAW_NAME", "tenant-a")

	var lookedUp []string
	testEnsureRuntimeAtHomeHook = func(_ *Service, homeDir string) (sandbox.Runtime, error) {
		if homeDir == "" {
			t.Fatalf("ensureRuntimeAtHome() homeDir = %q, want non-empty", homeDir)
		}
		return &fakeRuntime{}, nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		lookedUp = append(lookedUp, idOrName)
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	defer func() {
		testEnsureRuntimeAtHomeHook = nil
		testGetBoxHook = nil
	}()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:     ManagerUserID,
		Name:   ManagerName,
		Role:   RoleManager,
		Status: "running",
	}

	_, _, err = svc.lookupBootstrapManager(context.Background())
	if err != nil {
		t.Fatalf("lookupBootstrapManager() error = %v", err)
	}
	if len(lookedUp) != 1 {
		t.Fatalf("lookupBootstrapManager() called times = %d, want %d", len(lookedUp), 1)
	}
	if lookedUp[0] != ManagerName {
		t.Fatalf("lookupBootstrapManager() first lookup = %q, want %q", lookedUp[0], ManagerName)
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

func TestGatewayCreateSpecBuildsSandboxSpec(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	orig := localIPv4Resolver
	localIPv4Resolver = func() string { return "10.0.0.8" }
	defer func() { localIPv4Resolver = orig }()

	svc, err := NewServiceWithChannels(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		config.ChannelsConfig{
			Feishu: map[string]config.FeishuConfig{
				"u-worker-1": {
					AppID:     "cli_worker",
					AppSecret: "worker-secret",
				},
			},
		},
		"",
		"",
	)
	if err != nil {
		t.Fatalf("NewServiceWithChannels() error = %v", err)
	}

	spec, err := svc.gatewayCreateSpec("picoclaw:latest", "alice", "u-worker-1", AgentProfile{
		Name:     "alice",
		Provider: ProviderAPI,
		ModelID:  "minimax-m2.7",
	})
	if err != nil {
		t.Fatalf("gatewayCreateSpec() error = %v", err)
	}

	if spec.Image != "picoclaw:latest" {
		t.Fatalf("gatewayCreateSpec() image = %q, want %q", spec.Image, "picoclaw:latest")
	}
	if spec.Name != "alice" {
		t.Fatalf("gatewayCreateSpec() name = %q, want %q", spec.Name, "alice")
	}
	if !spec.Detach {
		t.Fatal("gatewayCreateSpec() detach = false, want true")
	}
	if spec.AutoRemove {
		t.Fatal("gatewayCreateSpec() auto_remove = true, want false")
	}
	wantCmd := "/bin/sh -c " + gatewayRunCommand()
	if strings.Join(spec.Cmd, " ") != wantCmd {
		t.Fatalf("gatewayCreateSpec() cmd = %q, want %q", spec.Cmd, wantCmd)
	}
	if got, want := spec.Env["HOME"], "/home/picoclaw"; got != want {
		t.Fatalf("HOME env = %q, want %q", got, want)
	}
	if got, want := spec.Env["CSGCLAW_BASE_URL"], "http://10.0.0.8:18080"; got != want {
		t.Fatalf("CSGCLAW_BASE_URL = %q, want %q", got, want)
	}
	if got, want := spec.Env["CSGCLAW_LLM_BASE_URL"], "http://10.0.0.8:18080/api/bots/u-worker-1/llm"; got != want {
		t.Fatalf("CSGCLAW_LLM_BASE_URL = %q, want %q", got, want)
	}
	if got, want := spec.Env["PICOCLAW_CHANNELS_FEISHU_APP_ID"], "cli_worker"; got != want {
		t.Fatalf("PICOCLAW_CHANNELS_FEISHU_APP_ID = %q, want %q", got, want)
	}

	wantAgentHome := filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, "alice")
	wantWorkspaceRoot := filepath.Join(wantAgentHome, hostWorkspaceDir)
	wantConfigRoot := filepath.Join(wantWorkspaceRoot, filepath.FromSlash(hostPicoClawStateDir))
	wantProjectsRoot := filepath.Join(homeDir, config.AppDirName, hostProjectsDir)
	if len(spec.Mounts) != 2 {
		t.Fatalf("gatewayCreateSpec() mounts = %+v, want 2 mounts", spec.Mounts)
	}
	if spec.Mounts[0].HostPath != wantWorkspaceRoot || spec.Mounts[0].GuestPath != boxWorkspaceDir {
		t.Fatalf("workspace mount = %+v, want host %q guest %q", spec.Mounts[0], wantWorkspaceRoot, boxWorkspaceDir)
	}
	if spec.Mounts[1].HostPath != wantProjectsRoot || spec.Mounts[1].GuestPath != boxProjectsDir {
		t.Fatalf("projects mount = %+v, want host %q guest %q", spec.Mounts[1], wantProjectsRoot, boxProjectsDir)
	}
	if _, err := os.Stat(filepath.Join(wantConfigRoot, hostPicoClawConfig)); err != nil {
		t.Fatalf("worker PicoClaw config was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantWorkspaceRoot, "AGENT.md")); err != nil {
		t.Fatalf("worker workspace was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantAgentHome, hostPicoClawDir)); !os.IsNotExist(err) {
		t.Fatalf("picoclaw host dir stat error = %v, want not exist", err)
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

func TestResolveManagerBaseURLPrefersAdvertiseBaseURL(t *testing.T) {
	orig := localIPv4Resolver
	localIPv4Resolver = func() string {
		t.Fatal("local IPv4 resolver should not be called when advertise_base_url is set")
		return "10.0.0.8"
	}
	t.Cleanup(func() {
		localIPv4Resolver = orig
	})

	got := resolveManagerBaseURL(config.ServerConfig{
		ListenAddr:       "0.0.0.0:19090",
		AdvertiseBaseURL: "http://127.0.0.1:18080",
	})

	want := "http://127.0.0.1:18080"
	if got != want {
		t.Fatalf("resolveManagerBaseURL() = %q, want %q", got, want)
	}
}
