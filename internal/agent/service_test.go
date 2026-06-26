package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/openclawsandbox"
	"csgclaw/internal/runtime/picoclawsandbox"
	"csgclaw/internal/runtime/sandboxgateway"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/boxlitecli"
	"csgclaw/internal/sandbox/hostuser"
	"csgclaw/internal/sandbox/sandboxtest"
	hub "csgclaw/internal/template"
	templateembed "csgclaw/internal/template/embed"
)

func init() {
	testDefaultServiceOption = withTestPicoClawSandboxRuntime()
}

func TestMain(m *testing.M) {
	testHome, err := os.MkdirTemp("", "csgclaw-agent-test-home-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create test home: %v\n", err)
		os.Exit(1)
	}
	oldHome, hadHome := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", testHome); err != nil {
		fmt.Fprintf(os.Stderr, "set test HOME: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	if hadHome {
		_ = os.Setenv("HOME", oldHome)
	} else {
		_ = os.Unsetenv("HOME")
	}
	_ = os.RemoveAll(testHome)
	os.Exit(code)
}

type fakeRuntime struct{}

type fakeProvider struct {
	open func(context.Context, string) (sandbox.Runtime, error)
}

func (f fakeProvider) Name() string {
	return "fake"
}

func (f fakeProvider) Open(ctx context.Context, homeDir string) (sandbox.Runtime, error) {
	if f.open != nil {
		return f.open(ctx, homeDir)
	}
	return &fakeRuntime{}, nil
}

func (fakeProvider) ListImages(context.Context, string) ([]string, error) {
	return []string{}, nil
}

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

type fakeAgentRuntime struct {
	kind       string
	workspace  func(string) string
	skills     func(string) string
	hostLogs   func(string) []string
	validate   func(context.Context, agentruntime.RuntimeConfigSnapshot) error
	restart    func(agentruntime.RuntimeConfigChange) (bool, error)
	reconcile  func(context.Context, agentruntime.Handle, agentruntime.RuntimeConfigChange) error
	provision  func(context.Context, agentruntime.ProvisionRequest) error
	new        func(context.Context, agentruntime.Spec) (agentruntime.Handle, error)
	start      func(context.Context, agentruntime.Handle) (agentruntime.State, error)
	stop       func(context.Context, agentruntime.Handle) (agentruntime.State, error)
	del        func(context.Context, agentruntime.Handle) error
	state      func(context.Context, agentruntime.Handle) (agentruntime.State, error)
	info       func(context.Context, agentruntime.Handle) (agentruntime.Info, error)
	streamLogs func(context.Context, agentruntime.Handle, agentruntime.LogOptions) error
}

type fakeBareAgentRuntime struct {
	kind string
}

func (f fakeAgentRuntime) Kind() string {
	return f.kind
}

func (f fakeBareAgentRuntime) Kind() string {
	return f.kind
}

func (f fakeAgentRuntime) Layout(agentHome string) agentruntime.Layout {
	if f.workspace != nil {
		workspace := f.workspace(agentHome)
		layout := agentruntime.Layout{
			WorkspaceRoot: workspace,
			SkillsRoot:    filepath.Join(workspace, "skills"),
		}
		if f.skills != nil {
			layout.SkillsRoot = f.skills(agentHome)
		}
		if f.hostLogs != nil {
			layout.HostLogPaths = f.hostLogs(agentHome)
		}
		return layout
	}
	switch strings.TrimSpace(f.kind) {
	case RuntimeKindPicoClawSandbox:
		workspace := filepath.Join(picoclawsandbox.Root(agentHome), picoclawsandbox.HostWorkspaceDir)
		return agentruntime.Layout{
			WorkspaceRoot: workspace,
			SkillsRoot:    filepath.Join(workspace, "skills"),
			HostLogPaths:  []string{picoclawsandbox.HostGatewayLogPath(agentHome)},
		}
	case RuntimeKindOpenClawSandbox:
		workspace := filepath.Join(openclawsandbox.Root(agentHome), openclawsandbox.HostWorkspaceDir)
		return agentruntime.Layout{
			WorkspaceRoot: workspace,
			SkillsRoot:    filepath.Join(workspace, "skills"),
			HostLogPaths:  []string{openclawsandbox.HostGatewayLogPath(agentHome)},
		}
	case RuntimeKindCodex:
		return agentruntime.Layout{
			WorkspaceRoot: filepath.Join(agentHome, ".codex", "workspace"),
			SkillsRoot:    filepath.Join(agentHome, ".codex", "home", "skills"),
			HostLogPaths:  []string{filepath.Join(agentHome, ".codex", "home", "stderr.log")},
		}
	default:
		return agentruntime.Layout{}
	}
}

func (f fakeBareAgentRuntime) Layout(agentHome string) agentruntime.Layout {
	return fakeAgentRuntime{kind: f.kind}.Layout(agentHome)
}

func (f fakeAgentRuntime) Provision(ctx context.Context, req agentruntime.ProvisionRequest) error {
	if f.provision != nil {
		return f.provision(ctx, req)
	}
	return nil
}

func (f fakeBareAgentRuntime) New(context.Context, agentruntime.Spec) (agentruntime.Handle, error) {
	return agentruntime.Handle{}, nil
}

func (f fakeBareAgentRuntime) Start(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateRunning, nil
}

func (f fakeBareAgentRuntime) Stop(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateStopped, nil
}

func (f fakeBareAgentRuntime) Delete(context.Context, agentruntime.Handle) error {
	return nil
}

func (f fakeBareAgentRuntime) State(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateRunning, nil
}

func (f fakeBareAgentRuntime) Info(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
	return agentruntime.Info{}, nil
}

func (f fakeAgentRuntime) New(ctx context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
	if f.new != nil {
		return f.new(ctx, spec)
	}
	return agentruntime.Handle{}, nil
}

func (f fakeAgentRuntime) ValidateConfig(ctx context.Context, current agentruntime.RuntimeConfigSnapshot) error {
	if f.validate != nil {
		return f.validate(ctx, current)
	}
	return nil
}

func (f fakeAgentRuntime) RestartRequired(change agentruntime.RuntimeConfigChange) (bool, error) {
	if f.restart != nil {
		return f.restart(change)
	}
	return false, nil
}

func (f fakeAgentRuntime) ReconcileConfig(ctx context.Context, h agentruntime.Handle, change agentruntime.RuntimeConfigChange) error {
	if f.reconcile != nil {
		return f.reconcile(ctx, h, change)
	}
	return nil
}

func (f fakeAgentRuntime) Start(ctx context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	if f.start != nil {
		return f.start(ctx, h)
	}
	return agentruntime.StateRunning, nil
}

func (f fakeAgentRuntime) Stop(ctx context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	if f.stop != nil {
		return f.stop(ctx, h)
	}
	return agentruntime.StateStopped, nil
}

func (f fakeAgentRuntime) Delete(ctx context.Context, h agentruntime.Handle) error {
	if f.del != nil {
		return f.del(ctx, h)
	}
	return nil
}

func (f fakeAgentRuntime) State(ctx context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	if f.state != nil {
		return f.state(ctx, h)
	}
	return agentruntime.StateRunning, nil
}

func (f fakeAgentRuntime) Info(ctx context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
	if f.info != nil {
		return f.info(ctx, h)
	}
	return agentruntime.Info{}, nil
}

func (f fakeAgentRuntime) StreamLogs(ctx context.Context, h agentruntime.Handle, opts agentruntime.LogOptions) error {
	if f.streamLogs != nil {
		return f.streamLogs(ctx, h, opts)
	}
	return nil
}

type fakeAgentRuntimeNoLogs struct {
	kind string
	info func(context.Context, agentruntime.Handle) (agentruntime.Info, error)
}

func (f fakeAgentRuntimeNoLogs) Kind() string {
	return f.kind
}

func (f fakeAgentRuntimeNoLogs) Layout(agentHome string) agentruntime.Layout {
	switch strings.TrimSpace(f.kind) {
	case RuntimeKindPicoClawSandbox:
		workspace := filepath.Join(picoclawsandbox.Root(agentHome), picoclawsandbox.HostWorkspaceDir)
		return agentruntime.Layout{
			WorkspaceRoot: workspace,
			SkillsRoot:    filepath.Join(workspace, "skills"),
			HostLogPaths:  []string{picoclawsandbox.HostGatewayLogPath(agentHome)},
		}
	case RuntimeKindOpenClawSandbox:
		workspace := filepath.Join(openclawsandbox.Root(agentHome), openclawsandbox.HostWorkspaceDir)
		return agentruntime.Layout{
			WorkspaceRoot: workspace,
			SkillsRoot:    filepath.Join(workspace, "skills"),
			HostLogPaths:  []string{openclawsandbox.HostGatewayLogPath(agentHome)},
		}
	case RuntimeKindCodex:
		return agentruntime.Layout{
			WorkspaceRoot: filepath.Join(agentHome, ".codex", "workspace"),
			SkillsRoot:    filepath.Join(agentHome, ".codex", "home", "skills"),
			HostLogPaths:  []string{filepath.Join(agentHome, ".codex", "home", "stderr.log")},
		}
	default:
		return agentruntime.Layout{}
	}
}

func (f fakeAgentRuntimeNoLogs) New(context.Context, agentruntime.Spec) (agentruntime.Handle, error) {
	return agentruntime.Handle{}, nil
}

func (f fakeAgentRuntimeNoLogs) Start(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateRunning, nil
}

func (f fakeAgentRuntimeNoLogs) Stop(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateStopped, nil
}

func (f fakeAgentRuntimeNoLogs) Delete(context.Context, agentruntime.Handle) error {
	return nil
}

func (f fakeAgentRuntimeNoLogs) State(context.Context, agentruntime.Handle) (agentruntime.State, error) {
	return agentruntime.StateRunning, nil
}

func (f fakeAgentRuntimeNoLogs) Info(ctx context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
	if f.info != nil {
		return f.info(ctx, h)
	}
	return agentruntime.Info{}, nil
}

type fakeInfoInstance struct {
	fakeInstance
	info sandbox.Info
}

func (f *fakeInfoInstance) Info(context.Context) (sandbox.Info, error) {
	return f.info, nil
}

type fakeLifecycleObserver struct {
	ensureCalls []Agent
	stopCalls   []string
	ensureErr   error
}

func (f *fakeLifecycleObserver) EnsureAgent(_ context.Context, a Agent) error {
	f.ensureCalls = append(f.ensureCalls, a)
	return f.ensureErr
}

func (f *fakeLifecycleObserver) StopAgent(agentID string) {
	f.stopCalls = append(f.stopCalls, agentID)
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

func writeSeededAgents(path string, agents []Agent) error {
	persisted := make([]persistedAgent, 0, len(agents))
	for _, a := range agents {
		persisted = append(persisted, newPersistedAgent(a))
	}
	data, err := json.Marshal(persistedState{Agents: persisted})
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
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
	case "stop":
		idOrName := req.Args[len(req.Args)-1]
		box, ok := r.boxes[idOrName]
		if !ok {
			return boxlitecli.CommandResult{ExitCode: 1, Stderr: []byte("Error: no such box: " + idOrName)}, fmt.Errorf("exit status 1")
		}
		box.Status = "stopped"
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
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
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
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
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
		ID:          "worker-2",
		Name:        "Alice",
		RuntimeKind: RuntimeKindCodex,
	})
	if err == nil {
		t.Fatal("CreateWorker() duplicate error = nil, want duplicate-name error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("CreateWorker() duplicate error = %q, want duplicate-name error", err)
	}
}

func TestCreateRejectsDuplicateAgentIDWithoutReplace(t *testing.T) {
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	svc.agents["agent-alice"] = Agent{
		ID:        "agent-alice",
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
	if !strings.Contains(err.Error(), `agent id "agent-alice" already exists`) {
		t.Fatalf("Create() duplicate error = %q, want duplicate-id error", err)
	}
}

func TestCreateWorkerRejectsInvalidRuntime(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return nil, nil },
		nil,
	)
	defer ResetTestHooks()

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateWorker(context.Background(), CreateAgentSpec{Name: "alice"})
	if err == nil {
		t.Fatal("CreateWorker() error = nil, want missing runtime_kind error")
	}
	if !strings.Contains(err.Error(), "runtime_kind is required") {
		t.Fatalf("CreateWorker() error = %q, want missing runtime_kind error", err)
	}
}

func TestCreateWorkerUsesCodexRuntimeWhenRequested(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) {
			t.Fatal("ensureRuntime() should not be used for codex worker creation")
			return nil, nil
		},
		func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string, _ string, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			t.Fatal("createGatewayBox() should not be used for codex worker creation")
			return nil, sandbox.Info{}, nil
		},
	)
	defer ResetTestHooks()

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		}, "manager-image:test", "",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				if spec.AgentID != "agent-alice" {
					t.Fatalf("Create() agent id = %q, want %q", spec.AgentID, "agent-alice")
				}
				if spec.AgentName != "alice" {
					t.Fatalf("Create() agent name = %q, want %q", spec.AgentName, "alice")
				}
				if got, want := spec.Profile.BaseURL, "http://127.0.0.1:18080/api/v1/agents/agent-alice/llm"; got != want {
					t.Fatalf("Create() profile base url = %q, want %q", got, want)
				}
				if got, want := spec.Profile.APIKey, "shared-token"; got != want {
					t.Fatalf("Create() profile api key = %q, want %q", got, want)
				}
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-session-alice"}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeKind: RuntimeKindCodex,
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderAPI,
			BaseURL:         "https://api.example/v1",
			APIKey:          "api-key",
			ModelID:         "gpt-4.1",
			ProfileComplete: true,
		},
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.RuntimeKind != RuntimeKindCodex {
		t.Fatalf("CreateWorker().RuntimeKind = %q, want %q", got.RuntimeKind, RuntimeKindCodex)
	}
	if got.BoxID != "codex-session-alice" {
		t.Fatalf("CreateWorker().BoxID = %q, want %q", got.BoxID, "codex-session-alice")
	}
	if rt := svc.runtimeRecords[got.RuntimeID]; rt.Kind != RuntimeKindCodex {
		t.Fatalf("runtime record kind = %q, want %q", rt.Kind, RuntimeKindCodex)
	}
}

func TestCreateWorkerPersistsCodexProfileBeforeRuntimeNew(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var svc *Service
	var sawProfile bool
	var err error
	svc, err = NewService(
		testModelConfig(),
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				profile, err := svc.ResolvedAgentProfile(spec.AgentID)
				if err != nil {
					t.Fatalf("ResolvedAgentProfile(%q) during runtime New = %v", spec.AgentID, err)
				}
				if got, want := profile.ModelID, "deepseek-v4-pro"; got != want {
					t.Fatalf("ResolvedAgentProfile().ModelID = %q, want %q", got, want)
				}
				sawProfile = true
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-session-alice"}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeKind: RuntimeKindCodex,
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderAPI,
			BaseURL:         "https://api.deepseek.com",
			APIKey:          "api-key",
			ModelID:         "deepseek-v4-pro",
			ProfileComplete: true,
		},
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if !sawProfile {
		t.Fatal("runtime New did not observe persisted profile")
	}
	if got.Status != string(agentruntime.StateRunning) {
		t.Fatalf("CreateWorker().Status = %q, want running", got.Status)
	}
}

func TestCreateWorkerRemovesStartingCodexAgentWhenRuntimeNewFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			new: func(context.Context, agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{}, errors.New("boom")
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateWorker(context.Background(), CreateAgentSpec{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeKind: RuntimeKindCodex,
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderAPI,
			BaseURL:         "https://api.deepseek.com",
			APIKey:          "api-key",
			ModelID:         "deepseek-v4-pro",
			ProfileComplete: true,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "create worker box: boom") {
		t.Fatalf("CreateWorker() error = %v, want runtime New failure", err)
	}
	if _, ok := svc.Agent("u-alice"); ok {
		t.Fatal("Agent(\"u-alice\") exists after runtime New failure")
	}
}

func TestCreateWorkerPreservesProfileDefaultsWhenCodexRuntimeNewFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			new: func(context.Context, agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{}, errors.New("boom")
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.profileDefaults = normalizeProfile(AgentProfile{
		Name:            ManagerName,
		Provider:        ProviderAPI,
		BaseURL:         "https://default.example/v1",
		APIKey:          "default-key",
		ModelID:         "default-model",
		ProfileComplete: true,
	}, ManagerName, "")

	_, err = svc.CreateWorker(context.Background(), CreateAgentSpec{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeKind: RuntimeKindCodex,
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderAPI,
			BaseURL:         "https://api.deepseek.com",
			APIKey:          "api-key",
			ModelID:         "deepseek-v4-pro",
			ProfileComplete: true,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "create worker box: boom") {
		t.Fatalf("CreateWorker() error = %v, want runtime New failure", err)
	}
	if got, want := svc.profileDefaults.ModelID, "default-model"; got != want {
		t.Fatalf("profileDefaults.ModelID = %q, want %q", got, want)
	}
	if got, want := svc.profileDefaults.APIKey, "default-key"; got != want {
		t.Fatalf("profileDefaults.APIKey = %q, want preserved default key", got)
	}
}

func TestEnsureCodexResponsesAPIRetriesAfterTransientProbeError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	calls := 0
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			validate: func(_ context.Context, current agentruntime.RuntimeConfigSnapshot) error {
				calls++
				if current.Profile.BaseURL != "https://api.example/v1" {
					t.Fatalf("responses probe baseURL = %q, want api.example", current.Profile.BaseURL)
				}
				if current.Profile.ModelID != "gpt-5.5" {
					t.Fatalf("responses probe modelID = %q, want gpt-5.5", current.Profile.ModelID)
				}
				if calls == 1 {
					if current.Profile.APIKey != "bad-key" {
						t.Fatalf("first responses probe apiKey = %q, want bad-key", current.Profile.APIKey)
					}
					return errors.New("temporary outage")
				}
				if current.Profile.APIKey != "fixed-key" {
					t.Fatalf("second responses probe apiKey = %q, want fixed-key", current.Profile.APIKey)
				}
				return nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	err = svc.validateRuntimeConfig(context.Background(), RuntimeKindCodex, runtimeConfigSnapshotForAgent(AgentProfile{
		Provider:        ProviderAPI,
		BaseURL:         "https://api.example/v1",
		APIKey:          "bad-key",
		ModelID:         "gpt-5.5",
		ProfileComplete: true,
	}, nil))
	if err == nil || !strings.Contains(err.Error(), "temporary outage") {
		t.Fatalf("first validateRuntimeConfig() error = %v, want temporary outage", err)
	}

	err = svc.validateRuntimeConfig(context.Background(), RuntimeKindCodex, runtimeConfigSnapshotForAgent(AgentProfile{
		Provider:        ProviderAPI,
		BaseURL:         "https://api.example/v1",
		APIKey:          "fixed-key",
		ModelID:         "gpt-5.5",
		ProfileComplete: true,
	}, nil))
	if err != nil {
		t.Fatalf("second validateRuntimeConfig() error = %v, want retry success", err)
	}
	if calls != 2 {
		t.Fatalf("responses probe calls = %d, want 2", calls)
	}
}

func TestCreateWorkerCodexRuntimeContinuesWhenResponsesUnsupported(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			validate: func(_ context.Context, current agentruntime.RuntimeConfigSnapshot) error {
				if current.Profile.BaseURL != "https://unsupported.example/v1" {
					t.Fatalf("responses probe baseURL = %q, want upstream profile URL", current.Profile.BaseURL)
				}
				if current.Profile.APIKey != "api-key" {
					t.Fatalf("responses probe apiKey = %q, want upstream profile key", current.Profile.APIKey)
				}
				if current.Profile.ModelID != "gpt-4.1" {
					t.Fatalf("responses probe modelID = %q, want gpt-4.1", current.Profile.ModelID)
				}
				return nil
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-session-alice"}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeKind: RuntimeKindCodex,
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderAPI,
			BaseURL:         "https://unsupported.example/v1",
			APIKey:          "api-key",
			ModelID:         "gpt-4.1",
			ProfileComplete: true,
		},
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.BoxID != "codex-session-alice" {
		t.Fatalf("CreateWorker().BoxID = %q, want codex-session-alice", got.BoxID)
	}
}

func TestCreateWorkerCodexRuntimeCSGHubLiteProbeUsesDefaultEndpoint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			validate: func(_ context.Context, current agentruntime.RuntimeConfigSnapshot) error {
				if current.Profile.Provider != ProviderCSGHubLite {
					t.Fatalf("responses probe provider = %q, want %q", current.Profile.Provider, ProviderCSGHubLite)
				}
				if current.Profile.BaseURL != defaultCSGHubLiteBaseURL {
					t.Fatalf("responses probe baseURL = %q, want CSGHub Lite default %q", current.Profile.BaseURL, defaultCSGHubLiteBaseURL)
				}
				if current.Profile.APIKey != defaultCSGHubLiteAPIKey {
					t.Fatalf("responses probe apiKey = %q, want CSGHub Lite default key", current.Profile.APIKey)
				}
				return nil
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-session-alice"}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeKind: RuntimeKindCodex,
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderCSGHubLite,
			BaseURL:         "https://api.deepseek.com",
			APIKey:          "stale-key",
			ModelID:         "Qwen3-0.6B-GGUF",
			ProfileComplete: true,
		},
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.BoxID != "codex-session-alice" {
		t.Fatalf("CreateWorker().BoxID = %q, want codex-session-alice", got.BoxID)
	}
}

func TestUpdateCodexAgentProfilePatchRestartsActiveBridge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	probeCalls := 0
	observer := &fakeLifecycleObserver{}
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		},
		"manager-image:test",
		"",
		WithLifecycleObserver(observer),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			validate: func(_ context.Context, current agentruntime.RuntimeConfigSnapshot) error {
				probeCalls++
				if current.Profile.BaseURL != "https://api.deepseek.com" {
					t.Fatalf("responses probe baseURL = %q, want deepseek upstream", current.Profile.BaseURL)
				}
				if current.Profile.APIKey != "deepseek-key" {
					t.Fatalf("responses probe apiKey = %q, want updated profile key", current.Profile.APIKey)
				}
				if current.Profile.ModelID != "deepseek-v4-pro" {
					t.Fatalf("responses probe modelID = %q, want deepseek-v4-pro", current.Profile.ModelID)
				}
				return nil
			},
			restart: func(change agentruntime.RuntimeConfigChange) (bool, error) {
				return change.Previous.Profile.BaseURL != change.Current.Profile.BaseURL ||
					change.Previous.Profile.APIKey != change.Current.Profile.APIKey ||
					change.Previous.Profile.ModelID != change.Current.Profile.ModelID, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	currentProfile := AgentProfile{
		Name:            "dev",
		Provider:        ProviderAPI,
		BaseURL:         "https://api.openai.com/v1",
		APIKey:          "openai-key",
		ModelID:         "gpt-5.5",
		ProfileComplete: true,
	}
	svc.agents["u-dev"] = Agent{
		ID:              "u-dev",
		Name:            "dev",
		RuntimeID:       "rt-u-dev",
		RuntimeKind:     RuntimeKindCodex,
		BoxID:           "codex-session-old",
		Role:            RoleWorker,
		Status:          string(agentruntime.StateRunning),
		Profile:         profileSelector(currentProfile),
		AgentProfile:    currentProfile,
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}
	nextProfile := AgentProfile{
		Provider: ProviderAPI,
		BaseURL:  "https://api.deepseek.com",
		APIKey:   "deepseek-key",
		ModelID:  "deepseek-v4-pro",
	}

	updated, err := svc.Update(context.Background(), "u-dev", UpdateRequest{AgentProfile: &nextProfile})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if probeCalls != 1 {
		t.Fatalf("responses probe calls = %d, want 1", probeCalls)
	}
	if !updated.AgentProfile.EnvRestartRequired {
		t.Fatal("Update().AgentProfile.EnvRestartRequired = false, want true so running Codex bridge is refreshed")
	}
	if len(observer.stopCalls) != 1 || observer.stopCalls[0] != "u-dev" {
		t.Fatalf("StopAgent() calls = %+v, want [u-dev]", observer.stopCalls)
	}
}

func TestUpdateAgentProfileCodexRuntimeFallbackRestartsActiveBridge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	observer := &fakeLifecycleObserver{}
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		},
		"manager-image:test",
		"",
		WithLifecycleObserver(observer),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			validate: func(_ context.Context, current agentruntime.RuntimeConfigSnapshot) error {
				if current.Profile.BaseURL != "https://api.deepseek.com" {
					t.Fatalf("responses probe baseURL = %q, want deepseek upstream", current.Profile.BaseURL)
				}
				if current.Profile.APIKey != "deepseek-key" {
					t.Fatalf("responses probe apiKey = %q, want updated profile key", current.Profile.APIKey)
				}
				if current.Profile.ModelID != "deepseek-v4-pro" {
					t.Fatalf("responses probe modelID = %q, want deepseek-v4-pro", current.Profile.ModelID)
				}
				return nil
			},
			restart: func(change agentruntime.RuntimeConfigChange) (bool, error) {
				return change.Previous.Profile.BaseURL != change.Current.Profile.BaseURL ||
					change.Previous.Profile.APIKey != change.Current.Profile.APIKey ||
					change.Previous.Profile.ModelID != change.Current.Profile.ModelID, nil
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-session-dev"}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	currentProfile := AgentProfile{
		Name:            "dev",
		Provider:        ProviderAPI,
		BaseURL:         "https://api.openai.com/v1",
		APIKey:          "openai-key",
		ModelID:         "gpt-5.5",
		ProfileComplete: true,
	}
	svc.agents["u-dev"] = Agent{
		ID:              "u-dev",
		Name:            "dev",
		RuntimeID:       "rt-u-dev",
		RuntimeKind:     RuntimeKindCodex,
		BoxID:           "codex-session-old",
		Role:            RoleWorker,
		Status:          string(agentruntime.StateRunning),
		Profile:         profileSelector(currentProfile),
		AgentProfile:    currentProfile,
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}

	view, err := svc.UpdateAgentProfile("u-dev", AgentProfile{
		Provider: ProviderAPI,
		BaseURL:  "https://api.deepseek.com",
		APIKey:   "deepseek-key",
		ModelID:  "deepseek-v4-pro",
	})
	if err != nil {
		t.Fatalf("UpdateAgentProfile() error = %v", err)
	}
	if !view.EnvRestartRequired {
		t.Fatal("UpdateAgentProfile().EnvRestartRequired = false, want true so running Codex bridge is refreshed")
	}
	if len(observer.stopCalls) != 1 || observer.stopCalls[0] != "u-dev" {
		t.Fatalf("StopAgent() calls = %+v, want [u-dev]", observer.stopCalls)
	}

	started, err := svc.Start(context.Background(), "u-dev")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if started.AgentProfile.EnvRestartRequired {
		t.Fatal("Start().AgentProfile.EnvRestartRequired = true, want false after recreate")
	}
}

func TestUpdateInstructionsRefreshesCodexWorkspaceAgentsFile(t *testing.T) {
	var reconcileCalls []struct {
		handle agentruntime.Handle
		change agentruntime.RuntimeConfigChange
	}
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			reconcile: func(_ context.Context, h agentruntime.Handle, change agentruntime.RuntimeConfigChange) error {
				reconcileCalls = append(reconcileCalls, struct {
					handle agentruntime.Handle
					change agentruntime.RuntimeConfigChange
				}{handle: h, change: change})
				return nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-dev"] = Agent{
		ID:           "u-dev",
		Name:         "dev",
		RuntimeID:    "rt-u-dev",
		RuntimeKind:  RuntimeKindCodex,
		Role:         RoleWorker,
		Status:       string(agentruntime.StateStopped),
		Instructions: "old instructions",
		CreatedAt:    time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}

	nextInstructions := "write AGENTS block"
	updated, err := svc.Update(context.Background(), "u-dev", UpdateRequest{Instructions: &nextInstructions})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Instructions != nextInstructions {
		t.Fatalf("Update().Instructions = %q, want %q", updated.Instructions, nextInstructions)
	}
	if len(reconcileCalls) != 1 {
		t.Fatalf("ReconcileConfig() calls = %d, want 1", len(reconcileCalls))
	}
	if got, want := reconcileCalls[0].handle.RuntimeID, "rt-agent-dev"; got != want {
		t.Fatalf("ReconcileConfig() runtime id = %q, want %q", got, want)
	}
}

func TestUpdateInstructionsDoesNotRefreshNonCodexRuntime(t *testing.T) {
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeBareAgentRuntime{kind: RuntimeKindPicoClawSandbox}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-dev"] = Agent{
		ID:           "u-dev",
		Name:         "dev",
		RuntimeID:    "rt-u-dev",
		RuntimeKind:  RuntimeKindPicoClawSandbox,
		Role:         RoleWorker,
		Status:       string(sandbox.StateStopped),
		Instructions: "old instructions",
		CreatedAt:    time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}

	nextInstructions := "do not refresh"
	if _, err := svc.Update(context.Background(), "u-dev", UpdateRequest{Instructions: &nextInstructions}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
}

func TestUpdateRuntimeOptionsSyncsCodexWorkspaceAgentsFile(t *testing.T) {
	var reconcileCalls []struct {
		handle agentruntime.Handle
		change agentruntime.RuntimeConfigChange
	}
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			reconcile: func(_ context.Context, h agentruntime.Handle, change agentruntime.RuntimeConfigChange) error {
				reconcileCalls = append(reconcileCalls, struct {
					handle agentruntime.Handle
					change agentruntime.RuntimeConfigChange
				}{
					handle: h,
					change: change,
				})
				return nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-dev"] = Agent{
		ID:           "u-dev",
		Name:         "dev",
		RuntimeID:    "rt-u-dev",
		RuntimeKind:  RuntimeKindCodex,
		Role:         RoleWorker,
		Status:       string(agentruntime.StateStopped),
		Instructions: "keep synced",
		RuntimeOptions: map[string]any{
			"local_workspace_dir": "/tmp/old",
		},
		CreatedAt: time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}

	nextRuntimeOptions := map[string]any{"local_workspace_dir": "/tmp/new"}
	updated, err := svc.Update(context.Background(), "u-dev", UpdateRequest{RuntimeOptions: &nextRuntimeOptions})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.RuntimeOptions["local_workspace_dir"] != "/tmp/new" {
		t.Fatalf("Update().RuntimeOptions = %#v, want local_workspace_dir /tmp/new", updated.RuntimeOptions)
	}
	if len(reconcileCalls) != 1 {
		t.Fatalf("ReconcileConfig() calls = %d, want 1", len(reconcileCalls))
	}
	if got, want := reconcileCalls[0].handle.RuntimeID, "rt-agent-dev"; got != want {
		t.Fatalf("ReconcileConfig() runtime id = %q, want %q", got, want)
	}
	if got := reconcileCalls[0].change.Previous.Options["local_workspace_dir"]; got != "/tmp/old" {
		t.Fatalf("ReconcileConfig() previous runtime options = %#v, want /tmp/old", reconcileCalls[0].change.Previous.Options)
	}
}

func TestUpdateCodexLocalWorkspaceDirMarksRunningRuntimeForRestart(t *testing.T) {
	observer := &fakeLifecycleObserver{}
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithLifecycleObserver(observer),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			restart: func(change agentruntime.RuntimeConfigChange) (bool, error) {
				prev, _ := change.Previous.Options["local_workspace_dir"].(string)
				next, _ := change.Current.Options["local_workspace_dir"].(string)
				return prev != next, nil
			},
			reconcile: func(context.Context, agentruntime.Handle, agentruntime.RuntimeConfigChange) error {
				return nil
			},
			info: func(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{State: agentruntime.StateRunning}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-dev"] = Agent{
		ID:          "u-dev",
		Name:        "dev",
		RuntimeID:   "rt-u-dev",
		RuntimeKind: RuntimeKindCodex,
		Role:        RoleWorker,
		Status:      string(agentruntime.StateRunning),
		RuntimeOptions: map[string]any{
			"local_workspace_dir": "/tmp/old",
		},
		AgentProfile: AgentProfile{
			Name:            "dev",
			Provider:        ProviderCodex,
			ModelID:         "gpt-5.5",
			ProfileComplete: true,
		},
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}

	nextRuntimeOptions := map[string]any{"local_workspace_dir": "/tmp/new"}
	updated, err := svc.Update(context.Background(), "u-dev", UpdateRequest{RuntimeOptions: &nextRuntimeOptions})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !updated.AgentProfile.EnvRestartRequired {
		t.Fatal("Update().AgentProfile.EnvRestartRequired = false, want true after codex local_workspace_dir change")
	}
	if len(observer.stopCalls) != 1 || observer.stopCalls[0] != "u-dev" {
		t.Fatalf("StopAgent() calls = %+v, want [u-dev]", observer.stopCalls)
	}
}

func TestUpdateFieldMaskClearsRuntimeOptions(t *testing.T) {
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			reconcile: func(context.Context, agentruntime.Handle, agentruntime.RuntimeConfigChange) error {
				return nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-dev"] = Agent{
		ID:          "u-dev",
		Name:        "dev",
		RuntimeID:   "rt-u-dev",
		RuntimeKind: RuntimeKindCodex,
		Role:        RoleWorker,
		Status:      string(agentruntime.StateStopped),
		RuntimeOptions: map[string]any{
			"local_workspace_dir": "/tmp/old",
		},
		AgentProfile: AgentProfile{
			Name:            "dev",
			Provider:        ProviderCodex,
			ModelID:         "gpt-5.5",
			ProfileComplete: true,
		},
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}

	emptyRuntimeOptions := map[string]any{}
	updated, err := svc.Update(context.Background(), "u-dev", UpdateRequest{
		RuntimeOptions: &emptyRuntimeOptions,
		FieldMask:      []string{"runtime_options"},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if len(updated.RuntimeOptions) != 0 {
		t.Fatalf("Update().RuntimeOptions = %#v, want cleared map", updated.RuntimeOptions)
	}
}

func TestCreateWorkerTriggersLifecycleObserver(t *testing.T) {
	observer := &fakeLifecycleObserver{}
	svc, err := NewService(
		config.ModelConfig{},
		config.ServerConfig{}, "manager-image:test", "",
		WithLifecycleObserver(observer),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-session-" + spec.AgentName}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		Name:        "alice",
		RuntimeKind: RuntimeKindCodex,
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderCodex,
			ModelID:         "gpt-5.4",
			ProfileComplete: true,
		},
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if len(observer.ensureCalls) != 1 {
		t.Fatalf("EnsureAgent() calls = %d, want 1", len(observer.ensureCalls))
	}
	if observer.ensureCalls[0].ID != got.ID {
		t.Fatalf("EnsureAgent() agent id = %q, want %q", observer.ensureCalls[0].ID, got.ID)
	}
}

func TestCreateWorkerProvisionsRuntimeBeforeNew(t *testing.T) {
	var callOrder []string
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		}, "manager-image:test", "",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				callOrder = append(callOrder, "provision")
				if req.RuntimeID != "rt-agent-alice" {
					t.Fatalf("Provision() runtime id = %q, want %q", req.RuntimeID, "rt-agent-alice")
				}
				if req.AgentID != "agent-alice" {
					t.Fatalf("Provision() agent id = %q, want %q", req.AgentID, "agent-alice")
				}
				if req.AgentName != "alice" {
					t.Fatalf("Provision() agent name = %q, want %q", req.AgentName, "alice")
				}
				if got, want := req.Profile.BaseURL, "http://127.0.0.1:18080/api/v1/agents/agent-alice/llm"; got != want {
					t.Fatalf("Provision() profile base url = %q, want %q", got, want)
				}
				if got, want := req.Profile.APIKey, "shared-token"; got != want {
					t.Fatalf("Provision() profile api key = %q, want %q", got, want)
				}
				if req.WorkspaceOverlay != "" {
					t.Fatalf("Provision() workspace overlay = %q, want empty", req.WorkspaceOverlay)
				}
				return nil
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				callOrder = append(callOrder, "new")
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-session-alice"}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateWorker(context.Background(), CreateAgentSpec{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeKind: RuntimeKindCodex,
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderAPI,
			BaseURL:         "https://api.example/v1",
			APIKey:          "api-key",
			ModelID:         "gpt-4.1",
			ProfileComplete: true,
		},
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got, want := strings.Join(callOrder, ","), "provision,new"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
}

func TestCreateWorkerProvisionsParticipantIDSeparateFromAgentID(t *testing.T) {
	var gotParticipantID string
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{}, "manager-image:test", "",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindPicoClawSandbox,
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				gotParticipantID = req.ParticipantID
				return nil
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "box-qa"}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if _, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		ID:          "u-agent-hhtz4b",
		Name:        "qa",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw-worker:dev",
		AgentProfile: AgentProfile{
			Name:            "qa",
			Provider:        ProviderAPI,
			BaseURL:         "https://api.example/v1",
			APIKey:          "api-key",
			ModelID:         "gpt-5.5",
			ProfileComplete: true,
		},
	}); err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got, want := gotParticipantID, "pt-hhtz4b"; got != want {
		t.Fatalf("Provision() participant id = %q, want %q", got, want)
	}
}

func TestCreateWorkerPassesWorkspaceOverlayToProvision(t *testing.T) {
	overlayRoot := t.TempDir()
	var gotOverlay string
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{}, "manager-image:test", "",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				gotOverlay = req.WorkspaceOverlay
				return nil
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-session-alice"}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if _, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		ID:           "u-alice",
		Name:         "alice",
		RuntimeKind:  RuntimeKindCodex,
		FromTemplate: overlayRoot,
		AgentProfile: AgentProfile{Name: "alice", Provider: ProviderAPI, BaseURL: "https://api.example/v1", APIKey: "api-key", ModelID: "gpt-4.1", ProfileComplete: true},
	}); err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if gotOverlay != overlayRoot {
		t.Fatalf("Provision() workspace overlay = %q, want %q", gotOverlay, overlayRoot)
	}
}

func TestCreateWorkerInstallsDefaultSystemSkills(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string, name, botID string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			if name != "alice" || botID != "agent-alice" {
				t.Fatalf("createGatewayBox() name=%q botID=%q, want alice/agent-alice", name, botID)
			}
			info := sandbox.Info{
				ID:        "box-alice",
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}
			return &fakeInfoInstance{info: info}, info, nil
		},
	)
	t.Cleanup(ResetTestHooks)

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindOpenClawSandbox}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	_, err = svc.CreateWorker(context.Background(), CreateAgentSpec{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeKind: RuntimeKindOpenClawSandbox,
		Image:       "worker-image:test",
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderAPI,
			BaseURL:         "https://api.example/v1",
			APIKey:          "api-key",
			ModelID:         "gpt-4.1",
			ProfileComplete: true,
		},
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	skillsRoot, err := svc.agentSkillsRoot("alice", RuntimeKindOpenClawSandbox)
	if err != nil {
		t.Fatalf("agentSkillsRoot() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(skillsRoot, "skill-installer", "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile(skill-installer) error = %v", err)
	}
	if !strings.Contains(string(data), "registry skill search") {
		t.Fatalf("skill-installer content = %q, want system skill instructions", string(data))
	}
}

func TestPreserveWorkspaceSkillsDropsDefaultSystemSkills(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindOpenClawSandbox}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	skillsRoot, err := svc.agentSkillsRoot("alice", RuntimeKindOpenClawSandbox)
	if err != nil {
		t.Fatalf("agentSkillsRoot() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillsRoot, "skill-installer"), 0o755); err != nil {
		t.Fatalf("MkdirAll(skill-installer) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "skill-installer", "SKILL.md"), []byte("# Old installer\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(old installer) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillsRoot, "custom"), 0o755); err != nil {
		t.Fatalf("MkdirAll(custom) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "custom", "SKILL.md"), []byte("# Custom\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(custom) error = %v", err)
	}

	restore, cleanup, err := svc.prepareWorkspaceSkillsPreservation("alice", RuntimeKindOpenClawSandbox, RuntimeKindOpenClawSandbox, RoleWorker)
	if err != nil {
		t.Fatalf("prepareWorkspaceSkillsPreservation() error = %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	agentHome, err := agentHomeDir("alice")
	if err != nil {
		t.Fatalf("agentHomeDir() error = %v", err)
	}
	if err := os.RemoveAll(agentHome); err != nil {
		t.Fatalf("RemoveAll(agent home) error = %v", err)
	}
	if restore == nil {
		t.Fatal("restore = nil, want preservation restore")
	}
	if err := restore(); err != nil {
		t.Fatalf("restore() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillsRoot, "custom", "SKILL.md")); err != nil {
		t.Fatalf("custom skill was not restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillsRoot, "skill-installer", "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old skill-installer restored unexpectedly, err=%v", err)
	}
	if err := svc.installDefaultSystemSkills("alice", RuntimeKindOpenClawSandbox); err != nil {
		t.Fatalf("installDefaultSystemSkills() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(skillsRoot, "skill-installer", "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile(new installer) error = %v", err)
	}
	if !strings.Contains(string(data), "registry skill search") {
		t.Fatalf("new skill-installer content = %q, want system skill instructions", string(data))
	}
}

func TestRecreateTriggersLifecycleObserver(t *testing.T) {
	observer := &fakeLifecycleObserver{}
	svc, err := NewService(
		config.ModelConfig{},
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		}, "manager-image:test", "",
		WithLifecycleObserver(observer),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			del:  func(context.Context, agentruntime.Handle) error { return nil },
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				if got, want := spec.Profile.BaseURL, "http://127.0.0.1:18080/api/v1/agents/agent-alice/llm"; got != want {
					t.Fatalf("Create() profile base url = %q, want %q", got, want)
				}
				if got, want := spec.Profile.APIKey, "shared-token"; got != want {
					t.Fatalf("Create() profile api key = %q, want %q", got, want)
				}
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-session-" + spec.AgentName + "-new"}, nil
			},
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["agent-alice"] = Agent{
		ID:          "agent-alice",
		Name:        "alice",
		Avatar:      "avatar/cartoon-7.png",
		Role:        RoleWorker,
		RuntimeID:   "rt-u-alice",
		RuntimeKind: RuntimeKindCodex,
		BoxID:       "codex-session-alice-old",
		Status:      string(agentruntime.StateRunning),
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderAPI,
			BaseURL:         "https://api.example/v1",
			APIKey:          "api-key",
			ModelID:         "gpt-4.1",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	got, err := svc.Recreate(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("Recreate() error = %v", err)
	}
	if got.BoxID != "codex-session-alice-new" {
		t.Fatalf("Recreate().BoxID = %q, want %q", got.BoxID, "codex-session-alice-new")
	}
	if got.Avatar != "avatar/cartoon-7.png" {
		t.Fatalf("Recreate().Avatar = %q, want %q", got.Avatar, "avatar/cartoon-7.png")
	}
	if len(observer.ensureCalls) != 1 || observer.ensureCalls[0].ID != "agent-alice" {
		t.Fatalf("EnsureAgent() calls = %+v, want one call for agent-alice", observer.ensureCalls)
	}
}

func TestRecreateProvisionsRuntimeBeforeNew(t *testing.T) {
	var callOrder []string
	svc, err := NewService(
		config.ModelConfig{},
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		}, "manager-image:test", "",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			del: func(context.Context, agentruntime.Handle) error {
				callOrder = append(callOrder, "delete")
				return nil
			},
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				callOrder = append(callOrder, "provision")
				if req.RuntimeID != "rt-agent-alice" {
					t.Fatalf("Provision() runtime id = %q, want %q", req.RuntimeID, "rt-agent-alice")
				}
				if req.AgentID != "agent-alice" {
					t.Fatalf("Provision() agent id = %q, want %q", req.AgentID, "agent-alice")
				}
				if req.AgentName != "alice" {
					t.Fatalf("Provision() agent name = %q, want %q", req.AgentName, "alice")
				}
				if got, want := req.Profile.BaseURL, "http://127.0.0.1:18080/api/v1/agents/agent-alice/llm"; got != want {
					t.Fatalf("Provision() profile base url = %q, want %q", got, want)
				}
				if got, want := req.Profile.APIKey, "shared-token"; got != want {
					t.Fatalf("Provision() profile api key = %q, want %q", got, want)
				}
				if req.WorkspaceOverlay != "" {
					t.Fatalf("Provision() workspace overlay = %q, want empty", req.WorkspaceOverlay)
				}
				return nil
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				callOrder = append(callOrder, "new")
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-session-alice-new"}, nil
			},
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:          "u-alice",
		Name:        "alice",
		Role:        RoleWorker,
		RuntimeID:   "rt-u-alice",
		RuntimeKind: RuntimeKindCodex,
		BoxID:       "codex-session-alice-old",
		Status:      string(agentruntime.StateRunning),
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderAPI,
			BaseURL:         "https://api.example/v1",
			APIKey:          "api-key",
			ModelID:         "gpt-4.1",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	_, err = svc.Recreate(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("Recreate() error = %v", err)
	}
	if got, want := strings.Join(callOrder, ","), "delete,provision,new"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
}

func TestDeleteTriggersLifecycleObserver(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	observer := &fakeLifecycleObserver{}
	svc, err := NewService(
		config.ModelConfig{},
		config.ServerConfig{}, "manager-image:test", "",
		WithLifecycleObserver(observer),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			del:  func(context.Context, agentruntime.Handle) error { return nil },
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["agent-alice"] = Agent{
		ID:              "agent-alice",
		Name:            "alice",
		Role:            RoleWorker,
		RuntimeID:       "rt-u-alice",
		RuntimeKind:     RuntimeKindCodex,
		BoxID:           "box-alice",
		Status:          string(agentruntime.StateRunning),
		AgentProfile:    AgentProfile{Name: "alice", Provider: ProviderCodex, ModelID: "gpt-5.4", ProfileComplete: true},
		ProfileComplete: true,
	}
	svc.syncRuntimeRecordLocked(svc.agents["agent-alice"])

	if err := svc.Delete(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(observer.stopCalls) != 1 || observer.stopCalls[0] != "agent-alice" {
		t.Fatalf("StopAgent() calls = %v, want [agent-alice]", observer.stopCalls)
	}
}

func TestCreateWorkerRequiresRuntimeKindWhenTemplateDoesNotProvideIt(t *testing.T) {
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

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{}, "manager-image:test", "",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			new: func(context.Context, agentruntime.Spec) (agentruntime.Handle, error) {
				t.Fatal("codex runtime should not be used when runtime kind is unset")
				return agentruntime.Handle{}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateWorker(context.Background(), CreateAgentSpec{Name: "alice"})
	if err == nil {
		t.Fatal("CreateWorker() error = nil, want missing runtime_kind error")
	}
	if !strings.Contains(err.Error(), "runtime_kind is required") {
		t.Fatalf("CreateWorker() error = %v, want missing runtime_kind error", err)
	}
}

func TestBoxLiteProviderGatewayLifecycle(t *testing.T) {
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
	if manager.BoxID != "box-csgclaw-agent-manager" || manager.Status != string(sandbox.StateRunning) {
		t.Fatalf("EnsureManager() = %+v, want running box-csgclaw-agent-manager", manager)
	}

	worker, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "picoclaw:latest",
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if worker.BoxID != "box-csgclaw-agent-alice" || worker.Status != string(sandbox.StateRunning) {
		t.Fatalf("CreateWorker() = %+v, want running box-csgclaw-agent-alice", worker)
	}
	if worker.RuntimeKind != RuntimeKindPicoClawSandbox {
		t.Fatalf("CreateWorker().RuntimeKind = %q, want %q", worker.RuntimeKind, RuntimeKindPicoClawSandbox)
	}

	layout, err := svc.agentLayout(worker.ID, RuntimeKindPicoClawSandbox)
	if err != nil {
		t.Fatalf("svc.agentLayout() error = %v", err)
	}
	logPath := layout.HostLogPaths[0]
	if logPath == "" {
		t.Fatal("svc.agentLayout() returned empty host log path")
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(gateway log dir) error = %v", err)
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
	if !hasBoxliteCLICommandArgs(runner.requests, "run", "/bin/sh", "-c", picoclawsandbox.GatewayRunCommand()) {
		t.Fatalf("boxlite-cli gateway run command not found in requests: %#v", requestArgs(runner.requests))
	}
	if hasBoxliteCLIExec(runner.requests, "tail", "-n", "1", "-f", picoclawsandbox.BoxGatewayLogPath) {
		t.Fatalf("boxlite-cli tail exec should not be used for mounted gateway logs: %#v", requestArgs(runner.requests))
	}
	if !hasBoxliteCLICommandArgs(runner.requests, "rm", "-f", "box-csgclaw-agent-alice") {
		t.Fatalf("boxlite-cli remove command not found in requests: %#v", requestArgs(runner.requests))
	}
	for _, req := range runner.requests {
		if len(req.Args) > 2 && req.Args[2] == "run" && !containsAny(req.Args, "/bin/sh", "/usr/local/bin/picoclaw") {
			t.Fatalf("boxlite-cli run args missing gateway command: %q", req.Args)
		}
	}
}

func TestCreateWorkerWithUTF8NameUsesAgentIDSandboxName(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	orig := localIPv4Resolver
	localIPv4Resolver = func() string { return "10.0.0.8" }
	defer func() { localIPv4Resolver = orig }()

	runner := newAgentBoxliteCLIRunner()
	provider := boxlitecli.NewProvider(boxlitecli.WithRunner(runner))
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"picoclaw:latest",
		filepath.Join(homeDir, "agents.json"),
		WithSandboxProvider(provider),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	worker, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		ID:          "u-qa",
		Name:        "测试工程师",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "picoclaw:latest",
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if worker.ID != "agent-qa" || worker.Name != "测试工程师" {
		t.Fatalf("CreateWorker() identity = %q/%q, want agent-qa/测试工程师", worker.ID, worker.Name)
	}
	if worker.BoxID != "box-csgclaw-agent-qa" {
		t.Fatalf("CreateWorker().BoxID = %q, want %q", worker.BoxID, "box-csgclaw-agent-qa")
	}
	if !hasBoxliteCLICommandArgs(runner.requests, "run", "--name", "csgclaw-agent-qa") {
		t.Fatalf("boxlite-cli run command for csgclaw-agent-qa not found in requests: %#v", requestArgs(runner.requests))
	}
	if hasBoxliteCLICommandArgs(runner.requests, "run", "--name", "测试工程师") {
		t.Fatalf("boxlite-cli run command used display name: %#v", requestArgs(runner.requests))
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
	if got.Status != string(sandbox.StateRunning) || got.AgentProfile.ModelID != "gpt-auto" || !got.ProfileComplete {
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
			ID:          "u-alice",
			Name:        "alice",
			RuntimeKind: RuntimeKindPicoClawSandbox,
			Image:       "picoclaw:latest",
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
	if replaced.ID != "agent-alice" || replaced.Name != "alice-v2" || replaced.Role != RoleWorker {
		t.Fatalf("Create() replaced = %+v, want replaced worker", replaced)
	}
	if !hasBoxliteCLICommandArgs(runner.requests, "rm", "-f", "box-csgclaw-agent-alice") {
		t.Fatalf("boxlite-cli remove command not found in requests: %#v", requestArgs(runner.requests))
	}
	if !hasBoxliteCLICommandArgs(runner.requests, "run", "--name", "csgclaw-agent-alice") {
		t.Fatalf("boxlite-cli run command for csgclaw-agent-alice not found in requests: %#v", requestArgs(runner.requests))
	}
}

func TestCreateReplaceRequiresExistingAgent(t *testing.T) {
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
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
	if !strings.Contains(err.Error(), `agent "agent-missing" not found`) {
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
			ID:           "u-alice",
			Name:         "alice",
			Description:  "worker",
			Instructions: "existing instructions",
			Image:        "agent-image:v1",
			RuntimeKind:  RuntimeKindPicoClawSandbox,
		},
	}); err != nil {
		t.Fatalf("Create() seed error = %v", err)
	}

	replaced, err := svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:           "u-alice",
			Name:         "alice-v2",
			Description:  "",
			Instructions: "new instructions",
			Image:        "agent-image:v2",
		},
		Replace:   true,
		FieldMask: []string{"id", "name"},
	})
	if err != nil {
		t.Fatalf("Create() replace error = %v", err)
	}
	if replaced.ID != "agent-alice" || replaced.Name != "alice-v2" {
		t.Fatalf("Create() replaced = %+v, want id agent-alice name alice-v2", replaced)
	}
	if replaced.Description != "worker" {
		t.Fatalf("Create() description = %q, want preserved description", replaced.Description)
	}
	if replaced.Instructions != "existing instructions" {
		t.Fatalf("Create() instructions = %q, want preserved instructions", replaced.Instructions)
	}
	if replaced.Image != "agent-image:v1" {
		t.Fatalf("Create() image = %q, want preserved image", replaced.Image)
	}
}

func TestUpdateInstructionsPersistsToState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "agents.json")
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		statePath,
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	svc.agents["u-alice"] = Agent{
		ID:           "u-alice",
		Name:         "alice",
		Description:  "worker",
		Instructions: "old instructions",
		RuntimeID:    "rt-u-alice",
		RuntimeKind:  RuntimeKindCodex,
		Role:         RoleWorker,
		Status:       string(agentruntime.StateStopped),
		CreatedAt:    time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC),
	}
	svc.mu.Lock()
	svc.syncRuntimeRecordLocked(svc.agents["u-alice"])
	if err := svc.saveLocked(); err != nil {
		svc.mu.Unlock()
		t.Fatalf("saveLocked() seed error = %v", err)
	}
	svc.mu.Unlock()

	nextInstructions := "keep responses short"
	updated, err := svc.Update(context.Background(), "u-alice", UpdateRequest{Instructions: &nextInstructions})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Instructions != nextInstructions {
		t.Fatalf("Update().Instructions = %q, want %q", updated.Instructions, nextInstructions)
	}

	reloaded, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		statePath,
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() reload error = %v", err)
	}
	got, ok := reloaded.Agent("u-alice")
	if !ok {
		t.Fatal("Agent() ok = false, want true")
	}
	if got.Instructions != nextInstructions {
		t.Fatalf("reloaded Agent().Instructions = %q, want %q", got.Instructions, nextInstructions)
	}
}

func TestUpdateManagerNamePersistsToRootState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), config.AppDirName, config.StateFileName)
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		statePath,
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	svc.agents[ManagerUserID] = Agent{
		ID:          ManagerUserID,
		Name:        ManagerName,
		Description: "manager",
		RuntimeID:   runtimeIDForAgentID(ManagerUserID),
		RuntimeKind: RuntimeKindCodex,
		Role:        RoleManager,
		Status:      string(agentruntime.StateStopped),
		CreatedAt:   time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC),
	}
	svc.mu.Lock()
	svc.syncRuntimeRecordLocked(svc.agents[ManagerUserID])
	if err := svc.saveLocked(); err != nil {
		svc.mu.Unlock()
		t.Fatalf("saveLocked() seed error = %v", err)
	}
	svc.mu.Unlock()

	nextName := "管理员"
	updated, err := svc.Update(context.Background(), ManagerUserID, UpdateRequest{Name: &nextName})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != nextName {
		t.Fatalf("Update().Name = %q, want %q", updated.Name, nextName)
	}

	reloaded, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		statePath,
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() reload error = %v", err)
	}
	got, ok := reloaded.Agent(ManagerUserID)
	if !ok {
		t.Fatal("Agent() ok = false, want true")
	}
	if got.ID != ManagerUserID || got.Role != RoleManager {
		t.Fatalf("reloaded manager identity = id %q role %q, want %q %q", got.ID, got.Role, ManagerUserID, RoleManager)
	}
	if got.Name != nextName {
		t.Fatalf("reloaded Agent().Name = %q, want %q", got.Name, nextName)
	}
}

func TestCreateReplaceManagerIgnoresRequestedImageAndUsesDefault(t *testing.T) {
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
	if gotImages[0] != "manager-image:1" || gotImages[1] != "manager-image:1" {
		t.Fatalf("createGatewayBox() images = %#v, want manager-image:1 for seed and replace", gotImages)
	}
	if replaced.Image != "manager-image:1" {
		t.Fatalf("Create() image = %q, want default manager image", replaced.Image)
	}
}

func TestCreateReplaceManagerPreservesRenamedManager(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	var gotNames []string
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			gotNames = append(gotNames, name)
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

	nextName := "管理员"
	if _, err := svc.Update(context.Background(), ManagerUserID, UpdateRequest{Name: &nextName}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	replaced, err := svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID: ManagerUserID,
		},
		Replace:   true,
		FieldMask: []string{"id"},
	})
	if err != nil {
		t.Fatalf("Create() replace error = %v", err)
	}
	if replaced.Name != nextName {
		t.Fatalf("Create() replaced manager name = %q, want %q", replaced.Name, nextName)
	}
	if len(gotNames) != 2 || gotNames[1] != nextName {
		t.Fatalf("createGatewayBox() names = %#v, want recreated manager name %q", gotNames, nextName)
	}
}

func TestCreateReplaceManagerClearsEnvRestartRequired(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Service, _ context.Context, _ sandbox.Runtime, image, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			if image != "manager-image:1" {
				t.Fatalf("createGatewayBox() image = %q, want manager-image:1", image)
			}
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

	profile := AgentProfile{
		Name:               ManagerName,
		Provider:           ProviderCodex,
		ModelID:            "gpt-5.5",
		ProfileComplete:    true,
		EnvRestartRequired: true,
	}
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:1", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:              ManagerUserID,
		Name:            ManagerName,
		RuntimeID:       runtimeIDForAgentID(ManagerUserID),
		RuntimeKind:     RuntimeKindPicoClawSandbox,
		Image:           "manager-image:1",
		BoxID:           "box-manager-old",
		Role:            RoleManager,
		Status:          string(sandbox.StateRunning),
		Profile:         profileSelector(profile),
		AgentProfile:    profile,
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
	}

	replaced, err := svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:   ManagerUserID,
			Name: ManagerName,
		},
		Replace: true,
	})
	if err != nil {
		t.Fatalf("Create() replace error = %v", err)
	}
	if replaced.AgentProfile.EnvRestartRequired {
		t.Fatal("Create() replaced manager EnvRestartRequired = true, want false after successful recreate")
	}
	view, err := svc.AgentProfileView(ManagerUserID)
	if err != nil {
		t.Fatalf("AgentProfileView() error = %v", err)
	}
	if view.EnvRestartRequired {
		t.Fatal("AgentProfileView().EnvRestartRequired = true, want false after successful recreate")
	}
}

func TestCreateReplaceManagerReprovisionsWorkspaceAfterHomeRemoval(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	rt := sandboxtest.NewRuntime()
	createCalls := 0
	rt.CreateFunc = func(_ context.Context, spec sandbox.CreateSpec) (sandbox.Instance, error) {
		createCalls++
		if len(spec.Mounts) == 0 {
			return nil, fmt.Errorf("create spec mounts are empty")
		}
		if _, err := os.Stat(spec.Mounts[0].HostPath); err != nil {
			return nil, fmt.Errorf("workspace mount host path %q is not available: %w", spec.Mounts[0].HostPath, err)
		}
		info := sandbox.Info{
			ID:        "box-" + spec.Name,
			Name:      spec.Name,
			State:     sandbox.StateRunning,
			CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		}
		inst := sandboxtest.NewInstance(info)
		rt.CreateCalls = append(rt.CreateCalls, spec)
		if rt.Instances == nil {
			rt.Instances = make(map[string]*sandboxtest.Instance)
		}
		rt.Instances[info.ID] = inst
		rt.Instances[info.Name] = inst
		return inst, nil
	}

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithSandboxProvider(fakeProvider{
			open: func(context.Context, string) (sandbox.Runtime, error) {
				return rt, nil
			},
		}),
	)
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
		Replace: true,
	})
	if err != nil {
		t.Fatalf("Create() replace error = %v", err)
	}
	if createCalls != 2 {
		t.Fatalf("sandbox Create() calls = %d, want 2", createCalls)
	}
	if replaced.Image != "manager-image:1" {
		t.Fatalf("Create() image = %q, want default manager image", replaced.Image)
	}
	workspaceRoot, err := testBuiltinWorkspaceRoot(ManagerName, RuntimeKindPicoClawSandbox)
	if err != nil {
		t.Fatalf("agentWorkspaceRoot() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "AGENT.md")); err != nil {
		t.Fatalf("manager workspace was not reprovisioned after replace: %v", err)
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

func TestCreateReplaceManagerSwitchesRuntimeKindRequiresImage(t *testing.T) {
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

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
	)
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
			ID:          ManagerUserID,
			Name:        ManagerName,
			RuntimeKind: RuntimeKindOpenClawSandbox,
		},
		Replace: true,
	})
	if err == nil || !strings.Contains(err.Error(), `image is required when changing gateway runtime_kind to "openclaw_sandbox"`) {
		t.Fatalf("Create() replace error = %v, want missing image error", err)
	}
	if replaced.ID != "" || replaced.Name != "" || replaced.RuntimeKind != "" || replaced.Image != "" {
		t.Fatalf("Create() replaced = %+v, want zero-value agent fields on error", replaced)
	}
	if got, want := svc.GatewayRuntime(), RuntimeKindPicoClawSandbox; got != want {
		t.Fatalf("GatewayRuntime() = %q, want %q after failed replace", got, want)
	}
	if len(gotImages) != 1 {
		t.Fatalf("createGatewayBox() calls = %d, want 1", len(gotImages))
	}
}

func TestCreateReplaceManagerSwitchesRuntimeKindUsesEmbeddedRuntimeImage(t *testing.T) {
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

	hubSvc, err := hub.NewService(config.HubConfig{}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatalf("hub.NewService() error = %v", err)
	}
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"picoclaw-manager:old",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{
			DefaultManagerTemplate: config.DefaultBootstrapManagerTemplate,
			DefaultWorkerTemplate:  config.DefaultBootstrapWorkerTemplate,
		}),
	)
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
	avatar := "avatar/cartoon-6.png"
	if _, err := svc.Update(context.Background(), ManagerUserID, UpdateRequest{Avatar: &avatar}); err != nil {
		t.Fatalf("Update() avatar error = %v", err)
	}

	const requestedImage = "openclaw-manager:requested"
	openClawTemplate, err := hubSvc.Get(context.Background(), "builtin.openclaw-manager")
	if err != nil {
		t.Fatalf("Get(openclaw-manager) error = %v", err)
	}
	wantImage := openClawTemplate.Image
	replaced, err := svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:          ManagerUserID,
			Name:        ManagerName,
			Image:       requestedImage,
			RuntimeKind: RuntimeKindOpenClawSandbox,
		},
		Replace: true,
	})
	if err != nil {
		t.Fatalf("Create() replace error = %v", err)
	}
	if got, want := replaced.RuntimeKind, RuntimeKindOpenClawSandbox; got != want {
		t.Fatalf("Create() runtime_kind = %q, want %q", got, want)
	}
	if got, want := replaced.Image, wantImage; got != want {
		t.Fatalf("Create() image = %q, want %q", got, want)
	}
	if got, want := replaced.Avatar, avatar; got != want {
		t.Fatalf("Create() avatar = %q, want %q", got, want)
	}
	if got, want := svc.managerImage, wantImage; got != want {
		t.Fatalf("managerImage = %q, want %q", got, want)
	}
	if len(gotImages) != 2 {
		t.Fatalf("createGatewayBox() calls = %d, want 2", len(gotImages))
	}
	if got, want := gotImages[1], wantImage; got != want {
		t.Fatalf("recreate manager image = %q, want %q", got, want)
	}
}

func TestCreateReplaceManagerWithStaleSubmittedImageUsesLatestDefaultTemplate(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "picoclaw-manager", hub.Template{
		ID:          "picoclaw-manager",
		Name:        "picoclaw-manager",
		Description: "picoclaw manager",
		Role:        hub.TemplateRoleManager,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Version:     "0.2.0",
		Image:       "registry.example/picoclaw-manager:0.2.0",
	})

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

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"registry.example/picoclaw-manager:0.2.0",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultManagerTemplate: "local/picoclaw-manager"}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:          ManagerUserID,
		Name:        ManagerName,
		RuntimeID:   runtimeIDForAgentID(ManagerUserID),
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "registry.example/picoclaw-manager:0.1.0",
		BoxID:       "box-manager-old",
		Role:        RoleManager,
		Status:      string(agentruntime.StateRunning),
		CreatedAt:   time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC),
		AgentProfile: AgentProfile{
			Name:               ManagerName,
			Provider:           ProviderCodex,
			ModelID:            "gpt-5.5",
			ProfileComplete:    true,
			EnvRestartRequired: true,
		},
		ProfileComplete: true,
	}

	replaced, err := svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:          ManagerUserID,
			Name:        ManagerName,
			Image:       "registry.example/picoclaw-manager:0.1.0",
			RuntimeKind: RuntimeKindPicoClawSandbox,
		},
		Replace: true,
	})
	if err != nil {
		t.Fatalf("Create() replace error = %v", err)
	}
	if got, want := replaced.Image, "registry.example/picoclaw-manager:0.2.0"; got != want {
		t.Fatalf("Create() replace image = %q, want %q", got, want)
	}
	if len(gotImages) != 1 {
		t.Fatalf("createGatewayBox() calls = %d, want 1", len(gotImages))
	}
	if got, want := gotImages[0], "registry.example/picoclaw-manager:0.2.0"; got != want {
		t.Fatalf("recreate manager image = %q, want %q", got, want)
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

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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
	t.Setenv("HOME", dir)
	statePath := filepath.Join(dir, "agents.json")
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", statePath)
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

	reloaded, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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
	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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

	reloaded, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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
	if !strings.Contains(string(data), `"state": "running"`) {
		t.Fatalf("saved state should contain last known runtime state: %s", data)
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

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
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
				ID:          "u-alice",
				Name:        "alice",
				RuntimeKind: RuntimeKindPicoClawSandbox,
				Role:        RoleWorker,
				BoxID:       "box-alice",
				CreatedAt:   time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
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

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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

func TestLoadLegacyAgentSynthesizesRuntimeRecord(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          "u-alice",
				Name:        "alice",
				RuntimeKind: RuntimeKindPicoClawSandbox,
				Role:        RoleWorker,
				BoxID:       "box-alice",
				Status:      string(sandbox.StateRunning),
				CreatedAt:   time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, ok := svc.agentSnapshot("u-alice")
	if !ok {
		t.Fatal("agentSnapshot() ok = false, want true")
	}
	if got.RuntimeID != "rt-agent-alice" {
		t.Fatalf("agentSnapshot().RuntimeID = %q, want %q", got.RuntimeID, "rt-agent-alice")
	}
	if got.RuntimeKind != RuntimeKindPicoClawSandbox {
		t.Fatalf("agentSnapshot().RuntimeKind = %q, want %q", got.RuntimeKind, RuntimeKindPicoClawSandbox)
	}
	rt, ok := svc.runtimeRecords[got.RuntimeID]
	if !ok {
		t.Fatalf("runtimeRecords[%q] missing", got.RuntimeID)
	}
	if rt.Kind != RuntimeKindPicoClawSandbox {
		t.Fatalf("runtime kind = %q, want %q", rt.Kind, RuntimeKindPicoClawSandbox)
	}
	if rt.SandboxID != "box-alice" {
		t.Fatalf("runtime sandbox id = %q, want %q", rt.SandboxID, "box-alice")
	}

	svc.mu.Lock()
	if err := svc.saveLocked(); err != nil {
		svc.mu.Unlock()
		t.Fatalf("saveLocked() error = %v", err)
	}
	svc.mu.Unlock()

	saved, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	var persisted persistedState
	if err := json.Unmarshal(saved, &persisted); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(persisted.Runtimes) != 1 {
		t.Fatalf("saved runtimes len = %d, want 1", len(persisted.Runtimes))
	}
	if persisted.Agents[0].Runtime.ID != "" {
		t.Fatalf("saved agent runtime.id = %q, want compact empty runtime id", persisted.Agents[0].Runtime.ID)
	}
	if persisted.Agents[0].Runtime.Kind != RuntimeKindPicoClawSandbox {
		t.Fatalf("saved agent runtime.kind = %q, want %q", persisted.Agents[0].Runtime.Kind, RuntimeKindPicoClawSandbox)
	}
}

func TestLoadAgentPreservesExplicitRuntimeKind(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          "u-alice",
				Name:        "alice",
				RuntimeID:   "rt-u-alice",
				RuntimeKind: RuntimeKindCodex,
				Role:        RoleWorker,
				Status:      string(sandbox.StateRunning),
				CreatedAt:   time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	got, ok := svc.agentSnapshot("u-alice")
	if !ok {
		t.Fatal("agentSnapshot() ok = false, want true")
	}
	if got.RuntimeKind != RuntimeKindCodex {
		t.Fatalf("agentSnapshot().RuntimeKind = %q, want %q", got.RuntimeKind, RuntimeKindCodex)
	}
	if rt := svc.runtimeRecords[got.RuntimeID]; rt.Kind != RuntimeKindCodex {
		t.Fatalf("runtime record kind = %q, want %q", rt.Kind, RuntimeKindCodex)
	}
}

func TestLoadAgentRequiresRuntimeKind(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:        "u-alice",
				Name:      "alice",
				Role:      RoleWorker,
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

	_, err = NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err == nil || !strings.Contains(err.Error(), "runtime_kind is required") {
		t.Fatalf("NewService() error = %v, want runtime_kind validation error", err)
	}
}

func TestLoadRuntimeRecordRequiresRuntimeKind(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          "u-alice",
				Name:        "alice",
				RuntimeID:   "rt-u-alice",
				RuntimeKind: RuntimeKindCodex,
				Role:        RoleWorker,
				CreatedAt:   time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
			},
		},
		Runtimes: []RuntimeRecord{
			{
				ID:        "rt-u-alice",
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

	_, err = NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err == nil || !strings.Contains(err.Error(), "runtime kind is required") {
		t.Fatalf("NewService() error = %v, want runtime kind validation error", err)
	}
}

func TestLoadManagerRepairsLegacyManagerIdentity(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          "manager",
				Name:        ManagerName,
				RuntimeKind: RuntimeKindPicoClawSandbox,
				Role:        RoleManager,
				CreatedAt:   time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	got, ok := svc.Agent(ManagerUserID)
	if !ok {
		t.Fatalf("Agent(%q) ok=false", ManagerUserID)
	}
	if got.ID != ManagerUserID {
		t.Fatalf("Agent().ID = %q, want %q", got.ID, ManagerUserID)
	}
}

func TestLoadManagerPreservesDisplayName(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	managerName := "管理员"
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          ManagerUserID,
				Name:        managerName,
				RuntimeKind: RuntimeKindPicoClawSandbox,
				Role:        RoleManager,
				CreatedAt:   time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	got, ok := svc.Agent(ManagerUserID)
	if !ok {
		t.Fatalf("Agent(%q) ok=false", ManagerUserID)
	}
	if got.ID != ManagerUserID || got.Name != managerName || got.Role != RoleManager {
		t.Fatalf("manager identity = %q/%q/%q, want %q/%q/%q", got.ID, got.Name, got.Role, ManagerUserID, managerName, RoleManager)
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

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
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
	var calls []string
	testStopBoxHook = func(_ *Service, _ context.Context, _ sandbox.Instance, opts sandbox.StopOptions) error {
		calls = append(calls, "stop")
		return nil
	}
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) error {
		calls = append(calls, "remove")
		removed = idOrName
		return nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		if idOrName == "box-123" {
			return &fakeInstance{}, nil
		}
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	defer func() {
		testStopBoxHook = nil
		testForceRemoveBoxHook = nil
	}()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:          "u-alice",
		Name:        "alice",
		BoxID:       "box-123",
		Role:        RoleWorker,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Status:      "running",
		CreatedAt:   time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	if err := svc.Delete(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if removed != "box-123" {
		t.Fatalf("ForceRemove() target = %q, want %q", removed, "box-123")
	}
	if strings.Join(calls, ",") != "stop,remove" {
		t.Fatalf("Delete() sandbox calls = %q, want stop then remove", strings.Join(calls, ","))
	}
}

func TestDeleteStopsBoxBeforeRemoveOnLegacyPath(t *testing.T) {
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		nil,
	)
	defer ResetTestHooks()

	var calls []string
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		if idOrName == "alice" {
			return &fakeInstance{}, nil
		}
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testStopBoxHook = func(_ *Service, _ context.Context, _ sandbox.Instance, _ sandbox.StopOptions) error {
		calls = append(calls, "stop")
		return nil
	}
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) error {
		calls = append(calls, "remove:"+idOrName)
		return nil
	}
	defer func() {
		testGetBoxHook = nil
		testStopBoxHook = nil
		testForceRemoveBoxHook = nil
	}()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
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

	if err := svc.Delete(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if strings.Join(calls, ",") != "stop,remove:alice" {
		t.Fatalf("Delete() sandbox calls = %q, want stop then remove:alice", strings.Join(calls, ","))
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

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
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
	if strings.Join(lookedUp, ",") != "box-stale,csgclaw-agent-alice,alice" {
		t.Fatalf("getBox() keys = %q, want stale box id, stable sandbox name, then display name fallback", lookedUp)
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

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
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

	runtimeHome, err := svc.sandboxRuntimeHome("alice")
	if err != nil {
		t.Fatalf("svc.sandboxRuntimeHome() error = %v", err)
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

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
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
		if path == agentHome && removeCalls == 2 {
			return &os.PathError{Op: "unlinkat", Path: filepath.Join(agentHome, "boxlite", "images"), Err: syscall.EACCES}
		}
		return os.RemoveAll(path)
	}
	defer func() {
		osRemoveAll = origRemoveAll
	}()

	if err := svc.Delete(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if removeCalls < 3 {
		t.Fatalf("osRemoveAll() calls = %d, want at least 3", removeCalls)
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

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		Name:        "alice",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.BoxID != "box-alice" {
		t.Fatalf("CreateWorker().BoxID = %q, want %q", got.BoxID, "box-alice")
	}
}

func TestCreateWorkerUsesRequestedImageWhenGatewayRuntimeExplicit(t *testing.T) {
	tests := []struct {
		name      string
		reqImage  string
		wantImage string
		wantErr   string
	}{
		{name: "requested image", reqImage: "worker-image:2", wantImage: "worker-image:2"},
		{name: "missing image", reqImage: "", wantErr: fmt.Sprintf(`image is required for runtime_kind %q`, RuntimeKindPicoClawSandbox)},
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

			got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
				Name:        "alice",
				RuntimeKind: RuntimeKindPicoClawSandbox,
				Image:       tt.reqImage,
			})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("CreateWorker() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
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

func TestCreateWorkerRejectsMissingImageWhenGatewayRuntimeExplicit(t *testing.T) {
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

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindOpenClawSandbox}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateWorker(context.Background(), CreateAgentSpec{
		Name:        "alice",
		RuntimeKind: RuntimeKindOpenClawSandbox,
	})
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf(`image is required for runtime_kind %q`, RuntimeKindOpenClawSandbox)) {
		t.Fatalf("CreateWorker() error = %v, want missing image error", err)
	}
	if gotImage != "" {
		t.Fatalf("createGatewayBox() image = %q, want empty because create should fail before box creation", gotImage)
	}
}

func TestCreateWorkerUsesDefaultProfileSnapshotForGatewayRuntime(t *testing.T) {
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
	}, config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		Name:        "alice",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
		Profile:     "codex",
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.Profile != "api.gpt-5.4" {
		t.Fatalf("CreateWorker().Profile = %q, want %q", got.Profile, "api.gpt-5.4")
	}
	if got.AgentProfile.Provider != ProviderAPI {
		t.Fatalf("CreateWorker().AgentProfile.Provider = %q, want %q", got.AgentProfile.Provider, ProviderAPI)
	}
	if got.AgentProfile.ModelID != "gpt-5.4" {
		t.Fatalf("CreateWorker().AgentProfile.ModelID = %q, want %q", got.AgentProfile.ModelID, "gpt-5.4")
	}
	if got.AgentProfile.ReasoningEffort != "medium" {
		t.Fatalf("CreateWorker().AgentProfile.ReasoningEffort = %q, want %q", got.AgentProfile.ReasoningEffort, "medium")
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

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		Name:        "alice",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
	})
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

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:          "u-alice",
		Name:        "alice",
		BoxID:       "box-123",
		Role:        RoleWorker,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Status:      "running",
		CreatedAt:   time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
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
	if strings.Join(gotArgs, " ") != "-n 50 "+picoclawsandbox.BoxGatewayLogPath {
		t.Fatalf("runBoxCommand() args = %q", gotArgs)
	}
	if out.String() != "line-1\n" {
		t.Fatalf("output = %q, want streamed log line", out.String())
	}
}

func TestStreamLogsUsesAgentIDForHostGatewayLogWithoutSandboxRuntime(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["agent-dahym7"] = Agent{
		ID:          "agent-dahym7",
		Name:        "qa",
		BoxID:       "box-123",
		Role:        RoleWorker,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Status:      "running",
		CreatedAt:   time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}
	layout, err := testBuiltinLayout("agent-dahym7", RuntimeKindPicoClawSandbox)
	if err != nil {
		t.Fatalf("testBuiltinLayout() error = %v", err)
	}
	logPath := layout.HostLogPaths[0]
	if logPath == "" {
		t.Fatal("testBuiltinLayout() returned empty host log path")
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
	if err := svc.StreamLogs(ctx, "agent-dahym7", false, 1, cancelOnWrite{writer: &out, cancel: cancel}); err != nil {
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
	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:          "u-alice",
		Name:        "alice",
		BoxID:       "box-stale",
		Role:        RoleWorker,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Status:      "running",
		CreatedAt:   time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	var out strings.Builder
	if err := svc.StreamLogs(context.Background(), "u-alice", false, 20, &out); err != nil {
		t.Fatalf("StreamLogs() error = %v", err)
	}
	if len(gotKeys) < 3 || gotKeys[0] != "box-stale" || gotKeys[1] != "csgclaw-agent-alice" || gotKeys[2] != "alice" {
		t.Fatalf("getBox() leading keys = %q, want stale box id, stable sandbox name, then display name fallback", gotKeys)
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
	var infoCalls int
	testBoxInfoHook = func(_ *Service, _ context.Context, _ sandbox.Instance) (sandbox.Info, error) {
		infoCalls++
		state := sandbox.StateRunning
		if infoCalls <= 2 {
			state = sandbox.StateStopped
		}
		return sandbox.Info{ID: "box-new", State: state}, nil
	}
	defer func() {
		testGetBoxHook = nil
		testStartBoxHook = nil
		testBoxInfoHook = nil
	}()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		BoxID:       "box-stale",
		Role:        RoleWorker,
		Status:      "stopped",
		CreatedAt:   time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	got, err := svc.Start(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if len(gotKeys) < 3 || gotKeys[0] != "box-stale" || gotKeys[1] != "csgclaw-agent-alice" || gotKeys[2] != "alice" {
		t.Fatalf("getBox() leading keys = %q, want stale box id, stable sandbox name, then display name fallback", gotKeys)
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

	reloaded, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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

func TestStartTriggersLifecycleObserver(t *testing.T) {
	observer := &fakeLifecycleObserver{}
	svc, err := NewService(
		config.ModelConfig{},
		config.ServerConfig{}, "manager-image:test", "",
		WithLifecycleObserver(observer),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
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
	svc.agents["agent-alice"] = Agent{
		ID:              "agent-alice",
		Name:            "alice",
		Role:            RoleWorker,
		RuntimeID:       "rt-u-alice",
		RuntimeKind:     RuntimeKindCodex,
		BoxID:           "box-alice",
		Status:          string(agentruntime.StateStopped),
		AgentProfile:    AgentProfile{Name: "alice", Provider: ProviderCodex, ModelID: "gpt-5.4", ProfileComplete: true},
		ProfileComplete: true,
	}

	if _, err := svc.Start(context.Background(), "agent-alice"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if len(observer.ensureCalls) != 1 || observer.ensureCalls[0].ID != "agent-alice" {
		t.Fatalf("EnsureAgent() calls = %+v, want one call for agent-alice", observer.ensureCalls)
	}
}

func TestStartProvisionsRuntimeBeforeStart(t *testing.T) {
	var callOrder []string
	svc, err := NewService(
		config.ModelConfig{},
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		}, "manager-image:test", "",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				callOrder = append(callOrder, "provision")
				if req.RuntimeID != "rt-agent-alice" || req.AgentID != "agent-alice" || req.AgentName != "alice" {
					t.Fatalf("Provision() request = %+v, want alice runtime identity", req)
				}
				return nil
			},
			start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
				callOrder = append(callOrder, "start")
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
	svc.agents["agent-alice"] = Agent{
		ID:              "agent-alice",
		Name:            "alice",
		Role:            RoleWorker,
		RuntimeID:       "rt-u-alice",
		RuntimeKind:     RuntimeKindCodex,
		BoxID:           "box-alice",
		Status:          string(agentruntime.StateStopped),
		AgentProfile:    AgentProfile{Name: "alice", Provider: ProviderAPI, BaseURL: "https://api.example/v1", APIKey: "api-key", ModelID: "gpt-4.1", ProfileComplete: true},
		ProfileComplete: true,
	}

	if _, err := svc.Start(context.Background(), "agent-alice"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := strings.Join(callOrder, ","), "provision,start"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
}

func TestStartInstallsDefaultSystemSkillsAfterProvision(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var callOrder []string
	svc, err := NewService(
		config.ModelConfig{},
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		}, "manager-image:test", "",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindOpenClawSandbox,
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				callOrder = append(callOrder, "provision")
				if req.RuntimeID != "rt-agent-alice" || req.AgentID != "agent-alice" || req.AgentName != "alice" {
					t.Fatalf("Provision() request = %+v, want alice runtime identity", req)
				}
				if req.Gateway == nil {
					t.Fatalf("Provision() gateway = nil, want gateway request")
				}
				return nil
			},
			start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
				callOrder = append(callOrder, "start")
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
	svc.agents["agent-alice"] = Agent{
		ID:              "agent-alice",
		Name:            "alice",
		Role:            RoleWorker,
		RuntimeID:       "rt-agent-alice",
		RuntimeKind:     RuntimeKindOpenClawSandbox,
		BoxID:           "box-alice",
		Status:          string(agentruntime.StateStopped),
		AgentProfile:    AgentProfile{Name: "alice", Provider: ProviderAPI, BaseURL: "https://api.example/v1", APIKey: "api-key", ModelID: "gpt-4.1", ProfileComplete: true},
		ProfileComplete: true,
	}

	if _, err := svc.Start(context.Background(), "agent-alice"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := strings.Join(callOrder, ","), "provision,start"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
	skillsRoot, err := svc.agentSkillsRoot("alice", RuntimeKindOpenClawSandbox)
	if err != nil {
		t.Fatalf("agentSkillsRoot() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(skillsRoot, "skill-installer", "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile(skill-installer) error = %v", err)
	}
	if !strings.Contains(string(data), "registry skill search") {
		t.Fatalf("skill-installer content = %q, want system skill instructions", string(data))
	}
}

func TestStartSkipsStartBoxWhenAlreadyRunning(t *testing.T) {
	rt := &fakeRuntime{}
	SetTestHooks(func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil }, nil)
	defer ResetTestHooks()

	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
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
	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:          "u-alice",
		Name:        "alice",
		BoxID:       "box-stale",
		Role:        RoleWorker,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Status:      "running",
		CreatedAt:   time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	got, err := svc.Start(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if startCalls != 0 {
		t.Fatalf("startBox() calls = %d, want 0", startCalls)
	}
	if got.Status != "running" {
		t.Fatalf("Start().Status = %q, want running", got.Status)
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

	svc, err := NewService(testModelConfig(), config.ServerConfig{ListenAddr: ":18080", AccessToken: "token"}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["agent-alice"] = Agent{
		ID:              "agent-alice",
		Name:            "alice",
		Role:            RoleWorker,
		RuntimeKind:     RuntimeKindPicoClawSandbox,
		BoxID:           "box-alice",
		Status:          string(sandbox.StateRunning),
		AgentProfile:    AgentProfile{Name: "alice", Provider: ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true},
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	if _, err := svc.Start(context.Background(), "agent-alice"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	agentHome, err := agentHomeDir("agent-alice")
	if err != nil {
		t.Fatalf("agentHomeDir() error = %v", err)
	}
	configPath := filepath.Join(agentHome, picoclawsandbox.HostDir, picoclawsandbox.HostConfig)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(worker config) error = %v", err)
	}
	text := string(data)
	for _, want := range []string{`"participant_id": "pt-alice"`, `"model_name": "gpt-5.5"`, `"api_base": "http://10.0.0.8:18080/api/v1/agents/agent-alice/llm"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("worker config missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, `"bot_id"`) {
		t.Fatalf("worker config still emitted bot_id:\n%s", text)
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
			if botID != "agent-alice" {
				t.Fatalf("createGatewayBox() botID = %q, want %q", botID, "agent-alice")
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
		RuntimeKind:     RuntimeKindPicoClawSandbox,
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
	if len(gotKeys) < 3 || gotKeys[0] != "box-alice-stale" || gotKeys[1] != "csgclaw-agent-alice" || gotKeys[2] != "alice" {
		t.Fatalf("getBox() leading keys = %q, want stale box id, stable sandbox name, then display name fallback", gotKeys)
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

func TestCreateWorkerFromTemplateAppliesDefaultsAndOverlaysWorkspace(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubService(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			Name:         "alice",
			FromTemplate: "local.frontend-worker",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.Description != "frontend worker" {
		t.Fatalf("Description = %q, want %q", got.Description, "frontend worker")
	}
	if got.Image != "worker-image:1" {
		t.Fatalf("Image = %q, want %q", got.Image, "worker-image:1")
	}
	if got.RuntimeKind != RuntimeKindPicoClawSandbox {
		t.Fatalf("RuntimeKind = %q, want %q", got.RuntimeKind, RuntimeKindPicoClawSandbox)
	}

	workspaceRoot, err := testBuiltinWorkspaceRoot(got.ID, RuntimeKindPicoClawSandbox)
	if err != nil {
		t.Fatalf("agentWorkspaceRoot() error = %v", err)
	}
	userData, err := os.ReadFile(filepath.Join(workspaceRoot, "USER.md"))
	if err != nil {
		t.Fatalf("ReadFile(USER.md) error = %v", err)
	}
	if got := strings.TrimSpace(string(userData)); got != "template user" {
		t.Fatalf("USER.md = %q, want %q", got, "template user")
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "AGENT.md")); err != nil {
		t.Fatalf("AGENT.md missing after template overlay: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "skills", "custom", "SKILL.md")); err != nil {
		t.Fatalf("template skill missing after overlay: %v", err)
	}
}

func TestApplyTemplateEnvDefaults(t *testing.T) {
	t.Parallel()

	got := applyTemplateEnvDefaults(CreateAgentSpec{
		AgentProfile: AgentProfile{
			Env: map[string]string{"GITLAB_TOKEN": "user-token"},
		},
	}, hub.Template{
		ImageEnv: []apitypes.ImageEnvContract{
			{Name: "GITLAB_TOKEN", Required: true, Secret: true},
			{Name: "GITLAB_URL", Default: "https://gitlab.example.com"},
		},
	})
	if got.AgentProfile.Env["GITLAB_TOKEN"] != "user-token" {
		t.Fatalf("GITLAB_TOKEN = %q, want user-token", got.AgentProfile.Env["GITLAB_TOKEN"])
	}
	if got.AgentProfile.Env["GITLAB_URL"] != "https://gitlab.example.com" {
		t.Fatalf("GITLAB_URL = %q, want default url", got.AgentProfile.Env["GITLAB_URL"])
	}
}

func TestCreateWorkerFromTemplateAppliesImageEnvToSandbox(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	var capturedEnv map[string]string
	restoreHook := testCreateGatewayBoxHook
	testCreateGatewayBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _, _, _ string, profile AgentProfile) (sandbox.Instance, sandbox.Info, error) {
		capturedEnv = profile.Env
		return nil, sandbox.Info{ID: "box-gitlab", State: sandbox.StateRunning}, nil
	}
	t.Cleanup(func() { testCreateGatewayBoxHook = restoreHook })

	hubSvc := mustNewLocalTemplateHubService(t, "gitlab-assistant", hub.Template{
		ID:          "gitlab-assistant",
		Name:        "gitlab-assistant",
		Description: "GitLab assistant",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
		ImageEnv: []apitypes.ImageEnvContract{
			{Name: "GITLAB_TOKEN", Required: true, Secret: true},
			{Name: "GITLAB_URL", Default: "https://gitlab.example.com"},
		},
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			Name:         "gitlab",
			FromTemplate: "local.gitlab-assistant",
			AgentProfile: AgentProfile{
				Env: map[string]string{"GITLAB_TOKEN": "secret-token"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if capturedEnv["GITLAB_TOKEN"] != "secret-token" {
		t.Fatalf("sandbox GITLAB_TOKEN = %q, want secret-token", capturedEnv["GITLAB_TOKEN"])
	}
	if capturedEnv["GITLAB_URL"] != "https://gitlab.example.com" {
		t.Fatalf("sandbox GITLAB_URL = %q, want default url", capturedEnv["GITLAB_URL"])
	}
}

func TestCreateOpenClawWorkerFromTemplateOverlaysOpenClawWorkspace(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubService(t, "openclaw-manager", hub.Template{
		ID:          "openclaw-manager",
		Name:        "openclaw-manager",
		Description: "openclaw manager",
		Role:        hub.TemplateRoleManager,
		RuntimeKind: RuntimeKindOpenClawSandbox,
		Image:       "openclaw-image:1",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		Name:         "alice",
		RuntimeKind:  RuntimeKindOpenClawSandbox,
		FromTemplate: "local.openclaw-manager",
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.RuntimeKind != RuntimeKindOpenClawSandbox {
		t.Fatalf("RuntimeKind = %q, want %q", got.RuntimeKind, RuntimeKindOpenClawSandbox)
	}

	agentHome, err := agentHomeDir(got.ID)
	if err != nil {
		t.Fatalf("agentHomeDir() error = %v", err)
	}
	openclawWorkspace := filepath.Join(openclawsandbox.Root(agentHome), openclawsandbox.HostWorkspaceDir)
	if _, err := os.Stat(filepath.Join(openclawWorkspace, "skills", "custom", "SKILL.md")); err != nil {
		t.Fatalf("template skill missing from OpenClaw workspace after overlay: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agentHome, hostWorkspaceDir, "skills", "custom", "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("template skill should not be written to legacy workspace path for OpenClaw, stat error = %v", err)
	}
}

func TestCreateWorkerUsesConfiguredDefaultTemplate(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubService(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local.frontend-worker"}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{Name: "alice"})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.Description != "frontend worker" {
		t.Fatalf("Description = %q, want %q", got.Description, "frontend worker")
	}
	if got.Image != "worker-image:1" {
		t.Fatalf("Image = %q, want %q", got.Image, "worker-image:1")
	}
	if got.RuntimeKind != RuntimeKindPicoClawSandbox {
		t.Fatalf("RuntimeKind = %q, want %q", got.RuntimeKind, RuntimeKindPicoClawSandbox)
	}

	workspaceRoot, err := testBuiltinWorkspaceRoot(got.ID, RuntimeKindPicoClawSandbox)
	if err != nil {
		t.Fatalf("agentWorkspaceRoot() error = %v", err)
	}
	userData, err := os.ReadFile(filepath.Join(workspaceRoot, "USER.md"))
	if err != nil {
		t.Fatalf("ReadFile(USER.md) error = %v", err)
	}
	if got := strings.TrimSpace(string(userData)); got != "template user" {
		t.Fatalf("USER.md = %q, want %q", got, "template user")
	}
}

func TestAgentMarksOutdatedDefaultTemplateImageUpgradeRequired(t *testing.T) {
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Version:     "0.2.0",
		Image:       "registry.example/picoclaw-worker:0.2.0",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local/frontend-worker"}),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindPicoClawSandbox,
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeID:   "rt-u-alice",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "registry.example/picoclaw-worker:0.1.0",
		BoxID:       "box-alice",
		Role:        RoleWorker,
		Status:      string(agentruntime.StateRunning),
		CreatedAt:   time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC),
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderCodex,
			ModelID:         "gpt-5.5",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	got, ok := svc.Agent("u-alice")
	if !ok {
		t.Fatal("Agent() ok = false, want true")
	}
	if got.AgentProfile.EnvRestartRequired {
		t.Fatalf("Agent().AgentProfile.EnvRestartRequired = true, want false for image-only upgrade")
	}
	if !got.AgentProfile.ImageUpgradeRequired {
		t.Fatalf("Agent().AgentProfile.ImageUpgradeRequired = false, want true for outdated default image")
	}
}

func TestCompareSemanticVersions(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    int
		wantOK  bool
	}{
		{current: "0.1.0", latest: "0.2.0", want: -1, wantOK: true},
		{current: "v1.2.3", latest: "1.2.3", want: 0, wantOK: true},
		{current: "1.10.0", latest: "1.2.0", want: 1, wantOK: true},
		{current: "1.0.0-alpha.2", latest: "1.0.0-alpha.10", want: -1, wantOK: true},
		{current: "1.0.0", latest: "1.0.0-rc.1", want: 1, wantOK: true},
		{current: "2026.5", latest: "0.2.0", want: 0, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.current+"_vs_"+tt.latest, func(t *testing.T) {
			got, ok := compareSemanticVersions(tt.current, tt.latest)
			if ok != tt.wantOK {
				t.Fatalf("compareSemanticVersions() ok = %t, want %t", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("compareSemanticVersions() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestImageNeedsTemplateVersionUpgradeWithSharedRepositoryMigration(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  defaultAgentImage
	}{
		{
			name:    "picoclaw worker to shared image",
			current: "registry.example/opencsghq/picoclaw-worker:0.1.4",
			latest: defaultAgentImage{
				image:   "registry.example/opencsghq/picoclaw:2026.6.23",
				version: "0.1.5",
			},
		},
		{
			name:    "openclaw manager to shared image",
			current: "registry.example/opencsghq/openclaw-manager:0.1.5",
			latest: defaultAgentImage{
				image:   "registry.example/opencsghq/openclaw:20260623.23-csgclaw",
				version: "0.1.6",
			},
		},
		{
			name:    "picoclaw shared image tag bump",
			current: "registry.example/opencsghq/picoclaw:2026.6.22",
			latest: defaultAgentImage{
				image:   "registry.example/opencsghq/picoclaw:2026.6.23",
				version: "0.1.5",
			},
		},
		{
			name:    "openclaw shared image tag bump",
			current: "registry.example/opencsghq/openclaw:20260623.22-csgclaw",
			latest: defaultAgentImage{
				image:   "registry.example/opencsghq/openclaw:20260623.23-csgclaw",
				version: "0.1.6",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !imageNeedsTemplateVersionUpgrade(tt.current, tt.latest) {
				t.Fatalf("imageNeedsTemplateVersionUpgrade(%q, %#v) = false, want true", tt.current, tt.latest)
			}
		})
	}
}

func TestImageNeedsTemplateVersionUpgradeIgnoresCurrentSharedImageTag(t *testing.T) {
	latest := defaultAgentImage{
		image:   "registry.example/opencsghq/picoclaw:2026.6.23",
		version: "0.1.5",
	}
	if imageNeedsTemplateVersionUpgrade("registry.example/opencsghq/picoclaw:2026.6.23", latest) {
		t.Fatal("imageNeedsTemplateVersionUpgrade() = true, want false for current shared tag")
	}
	if imageNeedsTemplateVersionUpgrade("registry.example/opencsghq/picoclaw:2026.6.24", latest) {
		t.Fatal("imageNeedsTemplateVersionUpgrade() = true, want false for newer shared tag")
	}
}

func TestAgentMarksOutdatedManagerImageUpgradeRequiredFromDefaultTemplateVersion(t *testing.T) {
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	const (
		oldManagerImage = "registry.example/opencsghq/picoclaw-manager:0.1.0"
		newManagerImage = "registry.example/opencsghq/picoclaw-manager:0.2.0"
	)
	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "picoclaw-manager", hub.Template{
		ID:          "picoclaw-manager",
		Name:        "picoclaw-manager",
		Description: "manager",
		Role:        hub.TemplateRoleManager,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Version:     "0.2.0",
		Image:       newManagerImage,
	})
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:unused",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultManagerTemplate: "local/picoclaw-manager"}),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindPicoClawSandbox,
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:          ManagerUserID,
		Name:        ManagerName,
		RuntimeID:   runtimeIDForAgentID(ManagerUserID),
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       oldManagerImage,
		BoxID:       "box-manager",
		Role:        RoleManager,
		Status:      string(agentruntime.StateRunning),
		CreatedAt:   time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
		AgentProfile: AgentProfile{
			Name:            ManagerName,
			Provider:        ProviderCodex,
			ModelID:         "gpt-5.5",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	got, ok := svc.Agent(ManagerUserID)
	if !ok {
		t.Fatal("Agent() ok = false, want true")
	}
	if got.AgentProfile.EnvRestartRequired {
		t.Fatalf("Agent().AgentProfile.EnvRestartRequired = true, want false for image-only upgrade")
	}
	if !got.AgentProfile.ImageUpgradeRequired {
		t.Fatalf("Agent().AgentProfile.ImageUpgradeRequired = false, want true for outdated manager image")
	}
}

func TestAgentIgnoresNewerLocalImageCandidateForUpgradeRequired(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	provider := sandboxtest.NewProvider()
	provider.Images = []string{
		"opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw-manager:0.2.0",
	}
	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "picoclaw-manager", hub.Template{
		ID:          "picoclaw-manager",
		Name:        "picoclaw-manager",
		Description: "manager",
		Role:        hub.TemplateRoleManager,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Version:     "0.1.0",
		Image:       "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw-manager:0.1.0",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:unused",
		"",
		WithSandboxProvider(provider),
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultManagerTemplate: "local/picoclaw-manager"}),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindPicoClawSandbox,
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:          ManagerUserID,
		Name:        ManagerName,
		RuntimeID:   runtimeIDForAgentID(ManagerUserID),
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw-manager:0.1.0",
		BoxID:       "box-manager",
		Role:        RoleManager,
		Status:      string(agentruntime.StateRunning),
		CreatedAt:   time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC),
		AgentProfile: AgentProfile{
			Name:            ManagerName,
			Provider:        ProviderCodex,
			ModelID:         "gpt-5.5",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	got, ok := svc.Agent(ManagerUserID)
	if !ok {
		t.Fatal("Agent() ok = false, want true")
	}
	if got.AgentProfile.ImageUpgradeRequired {
		t.Fatalf("Agent().AgentProfile.ImageUpgradeRequired = true, want false when only local image list is newer")
	}
}

func TestAgentDevImageDoesNotRequireUpgrade(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	provider := sandboxtest.NewProvider()
	provider.Images = []string{
		"opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw-manager:0.2.0",
		"opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw-manager:dev",
	}
	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "picoclaw-manager", hub.Template{
		ID:          "picoclaw-manager",
		Name:        "picoclaw-manager",
		Description: "manager",
		Role:        hub.TemplateRoleManager,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Version:     "0.2.0",
		Image:       "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw-manager:0.2.0",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:unused",
		"",
		WithSandboxProvider(provider),
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultManagerTemplate: "local/picoclaw-manager"}),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindPicoClawSandbox,
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:          ManagerUserID,
		Name:        ManagerName,
		RuntimeID:   runtimeIDForAgentID(ManagerUserID),
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw-manager:dev",
		BoxID:       "box-manager",
		Role:        RoleManager,
		Status:      string(agentruntime.StateRunning),
		CreatedAt:   time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
		AgentProfile: AgentProfile{
			Name:            ManagerName,
			Provider:        ProviderCodex,
			ModelID:         "gpt-5.5",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	got, ok := svc.Agent(ManagerUserID)
	if !ok {
		t.Fatal("Agent() ok = false, want true")
	}
	if got.AgentProfile.ImageUpgradeRequired {
		t.Fatalf("Agent().AgentProfile.ImageUpgradeRequired = true, want false for dev image")
	}
}

func TestRecreateUsesLatestDefaultTemplateImageAndPreservesUserSkills(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Version:     "0.2.0",
		Image:       "registry.example/picoclaw-worker:0.2.0",
	})

	var newImage string
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local/frontend-worker"}),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindPicoClawSandbox,
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				skillPath, err := agentSkillPath(req.AgentName, RuntimeKindPicoClawSandbox, "custom-tool")
				if err != nil {
					t.Fatalf("agentSkillPath() error = %v", err)
				}
				data, readErr := os.ReadFile(skillPath)
				if readErr != nil || string(data) != "# Custom Tool\n" {
					t.Fatalf("custom skill during provision = %q, %v; want preserved skill", string(data), readErr)
				}
				return nil
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				newImage = spec.Image
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "box-alice-new"}, nil
			},
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning, CreatedAt: time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeID:   "rt-u-alice",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "registry.example/picoclaw-worker:0.1.0",
		BoxID:       "box-alice-old",
		Role:        RoleWorker,
		Status:      string(agentruntime.StateRunning),
		CreatedAt:   time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC),
		AgentProfile: AgentProfile{
			Name:               "alice",
			Provider:           ProviderCodex,
			ModelID:            "gpt-5.5",
			ProfileComplete:    true,
			EnvRestartRequired: true,
		},
		ProfileComplete: true,
	}

	skillPath, err := agentSkillPath("alice", RuntimeKindPicoClawSandbox, "custom-tool")
	if err != nil {
		t.Fatalf("agentSkillPath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(skill dir) error = %v", err)
	}
	if err := os.WriteFile(skillPath, []byte("# Custom Tool\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(skill) error = %v", err)
	}

	recreated, err := svc.Recreate(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("Recreate() error = %v", err)
	}
	if newImage != "registry.example/picoclaw-worker:0.2.0" {
		t.Fatalf("runtime New() image = %q, want latest default template image", newImage)
	}
	if recreated.Image != "registry.example/picoclaw-worker:0.2.0" {
		t.Fatalf("Recreate().Image = %q, want latest default template image", recreated.Image)
	}
	if recreated.AgentProfile.EnvRestartRequired {
		t.Fatalf("Recreate().AgentProfile.EnvRestartRequired = true, want false after recreate")
	}
	data, err := os.ReadFile(skillPath)
	if err != nil || string(data) != "# Custom Tool\n" {
		t.Fatalf("custom skill after recreate = %q, %v; want preserved skill", string(data), err)
	}
}

func TestUpgradeUsesLatestDefaultTemplateImage(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Version:     "0.2.0",
		Image:       "registry.example/picoclaw-worker:0.2.0",
	})

	var newImage string
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local/frontend-worker"}),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindPicoClawSandbox,
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
	svc.agents["u-alice"] = Agent{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeID:   "rt-u-alice",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "custom.example/alice-worker:2026.05.27",
		BoxID:       "box-alice-old",
		Role:        RoleWorker,
		Status:      string(agentruntime.StateRunning),
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderCodex,
			ModelID:         "gpt-5.5",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	recreated, err := svc.Upgrade(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}
	if newImage != "registry.example/picoclaw-worker:0.2.0" {
		t.Fatalf("runtime New() image = %q, want latest default template image", newImage)
	}
	if recreated.Image != "registry.example/picoclaw-worker:0.2.0" {
		t.Fatalf("Upgrade().Image = %q, want latest default template image", recreated.Image)
	}
}

func TestUpgradeUsesBuiltinWorkerImageForAgentRuntimeWhenDefaultWorkerTemplateDiffers(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc, err := hub.NewService(config.HubConfig{}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatalf("hub.NewService() error = %v", err)
	}
	openClawTemplate, err := hubSvc.Get(context.Background(), "builtin.openclaw-worker")
	if err != nil {
		t.Fatalf("Get(openclaw-worker) error = %v", err)
	}

	var newImage string
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "builtin.picoclaw-worker"}),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindOpenClawSandbox,
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
	svc.agents["u-alice"] = Agent{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeID:   "rt-u-alice",
		RuntimeKind: RuntimeKindOpenClawSandbox,
		Image:       "custom.example/alice-openclaw:2026.05.27",
		BoxID:       "box-alice-old",
		Role:        RoleWorker,
		Status:      string(agentruntime.StateRunning),
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderCodex,
			ModelID:         "gpt-5.5",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	recreated, err := svc.Upgrade(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}
	if newImage != openClawTemplate.Image {
		t.Fatalf("runtime New() image = %q, want builtin OpenClaw worker image %q", newImage, openClawTemplate.Image)
	}
	if recreated.Image != openClawTemplate.Image {
		t.Fatalf("Upgrade().Image = %q, want builtin OpenClaw worker image %q", recreated.Image, openClawTemplate.Image)
	}
}

func TestRecreateRefreshesBuiltInSkillsAndPreservesUserSkills(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:          ManagerUserID,
		Name:        ManagerName,
		RuntimeID:   runtimeIDForAgentID(ManagerUserID),
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "manager-image:1",
		BoxID:       "box-manager-old",
		Role:        RoleManager,
		Status:      string(agentruntime.StateRunning),
		CreatedAt:   time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC),
		AgentProfile: AgentProfile{
			Name:            ManagerName,
			Provider:        ProviderCodex,
			ModelID:         "gpt-5.5",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	workspaceRoot, err := testBuiltinWorkspaceRoot(ManagerName, RuntimeKindPicoClawSandbox)
	if err != nil {
		t.Fatalf("agentWorkspaceRoot() error = %v", err)
	}
	builtInSkillPath := filepath.Join(workspaceRoot, "skills", "agent-teams", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(builtInSkillPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(agent-teams) error = %v", err)
	}
	if err := os.WriteFile(builtInSkillPath, []byte("# Locally Edited Agent Teams\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(agent-teams) error = %v", err)
	}
	userSkillPath := filepath.Join(workspaceRoot, "skills", "impeccable", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(userSkillPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(impeccable) error = %v", err)
	}
	if err := os.WriteFile(userSkillPath, []byte("# Impeccable\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(impeccable) error = %v", err)
	}

	recreated, err := svc.Recreate(context.Background(), ManagerUserID)
	if err != nil {
		t.Fatalf("Recreate() error = %v", err)
	}
	if recreated.AgentProfile.EnvRestartRequired {
		t.Fatal("Recreate().AgentProfile.EnvRestartRequired = true, want false")
	}

	wantBuiltIn, err := templateembed.FS().Open(templateembed.WorkspacePath(templateembed.PicoClawManagerRoot) + "/skills/agent-teams/SKILL.md")
	if err != nil {
		t.Fatalf("open embedded agent-teams skill: %v", err)
	}
	defer func() {
		_ = wantBuiltIn.Close()
	}()
	wantBuiltInData, err := io.ReadAll(wantBuiltIn)
	if err != nil {
		t.Fatalf("read embedded agent-teams skill: %v", err)
	}
	gotBuiltInData, err := os.ReadFile(builtInSkillPath)
	if err != nil {
		t.Fatalf("ReadFile(agent-teams) error = %v", err)
	}
	if string(gotBuiltInData) != string(wantBuiltInData) {
		t.Fatalf("agent-teams after recreate = %q, want embedded template content", string(gotBuiltInData))
	}
	userData, err := os.ReadFile(userSkillPath)
	if err != nil || string(userData) != "# Impeccable\n" {
		t.Fatalf("impeccable after recreate = %q, %v; want preserved user skill", string(userData), err)
	}
}

func TestCreateWorkerRejectsMissingDefaultTemplate(t *testing.T) {
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubService(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local.missing"}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateWorker(context.Background(), CreateAgentSpec{Name: "alice"})
	if err == nil {
		t.Fatal("CreateWorker() error = nil, want missing default template")
	}
	if !strings.Contains(err.Error(), `resolve default worker template "local.missing"`) {
		t.Fatalf("CreateWorker() error = %v, want default worker template context", err)
	}
}

func TestCreateWorkerRejectsDefaultTemplateRoleMismatch(t *testing.T) {
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubService(t, "review-manager", hub.Template{
		ID:          "review-manager",
		Name:        "review-manager",
		Description: "manager template",
		Role:        hub.TemplateRoleManager,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "manager-image:1",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local.review-manager"}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateWorker(context.Background(), CreateAgentSpec{Name: "alice"})
	if err == nil {
		t.Fatal("CreateWorker() error = nil, want role mismatch")
	}
	if !strings.Contains(err.Error(), `default worker template "local.review-manager" points to a manager template`) {
		t.Fatalf("CreateWorker() error = %v, want worker/manager mismatch", err)
	}
}

func TestCreateWorkerSkipsDefaultTemplateRuntimeMismatch(t *testing.T) {
	hubSvc := mustNewLocalTemplateHubService(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local.frontend-worker"}),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				if spec.AgentName != "alice" {
					t.Fatalf("Create() agent name = %q, want %q", spec.AgentName, "alice")
				}
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-session-alice"}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		Name:        "alice",
		RuntimeKind: RuntimeKindCodex,
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.RuntimeKind != RuntimeKindCodex {
		t.Fatalf("CreateWorker().RuntimeKind = %q, want %q", got.RuntimeKind, RuntimeKindCodex)
	}
	if got.BoxID != "codex-session-alice" {
		t.Fatalf("CreateWorker().BoxID = %q, want %q", got.BoxID, "codex-session-alice")
	}
}

func TestCreateWorkerAppliesTemplateDefaultsWithoutWorkspace(t *testing.T) {
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local.frontend-worker"}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{Name: "alice"})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.Description != "frontend worker" {
		t.Fatalf("Description = %q, want %q", got.Description, "frontend worker")
	}
	if got.Image != "worker-image:1" {
		t.Fatalf("Image = %q, want %q", got.Image, "worker-image:1")
	}

	workspaceRoot, err := testBuiltinWorkspaceRoot(got.ID, RuntimeKindPicoClawSandbox)
	if err != nil {
		t.Fatalf("agentWorkspaceRoot() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "skills", "custom", "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(skills/custom/SKILL.md) error = %v, want not exist", err)
	}
}

func TestCreateRejectsDefaultManagerTemplateRoleMismatch(t *testing.T) {
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubService(t, "review-worker", hub.Template{
		ID:          "review-worker",
		Name:        "review-worker",
		Description: "worker template",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultManagerTemplate: "local.review-worker"}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{Name: ManagerName},
	})
	if err == nil {
		t.Fatal("Create() error = nil, want role mismatch")
	}
	if !strings.Contains(err.Error(), `default manager template "local.review-worker" points to a worker template`) {
		t.Fatalf("Create() error = %v, want manager/worker mismatch", err)
	}
}

func TestHubPublishSpecUsesAgentWorkspaceSnapshot(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:1", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	created, err := svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:          "u-alice",
			Name:        "alice",
			Description: "review worker",
			RuntimeKind: RuntimeKindPicoClawSandbox,
			Image:       "worker-image:1",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	workspaceRoot, err := testBuiltinWorkspaceRoot(created.ID, created.RuntimeKind)
	if err != nil {
		t.Fatalf("agentWorkspaceRoot() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "PLAYBOOK.md"), []byte("workspace snapshot\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(PLAYBOOK.md) error = %v", err)
	}

	spec, err := svc.HubPublishSpec(created.ID)
	if err != nil {
		t.Fatalf("HubPublishSpec() error = %v", err)
	}
	if spec.ID != "alice" || spec.Name != "alice" {
		t.Fatalf("publish identity = %q/%q, want alice/alice", spec.ID, spec.Name)
	}
	if spec.Description != "review worker" {
		t.Fatalf("Description = %q, want %q", spec.Description, "review worker")
	}
	if spec.Role != hub.TemplateRoleWorker {
		t.Fatalf("Role = %q, want %q", spec.Role, hub.TemplateRoleWorker)
	}
	if spec.RuntimeKind != RuntimeKindPicoClawSandbox {
		t.Fatalf("RuntimeKind = %q, want %q", spec.RuntimeKind, RuntimeKindPicoClawSandbox)
	}
	if spec.Image != "worker-image:1" {
		t.Fatalf("Image = %q, want %q", spec.Image, "worker-image:1")
	}
	if spec.WorkspaceRef.Kind != hub.WorkspaceKindDir {
		t.Fatalf("WorkspaceRef.Kind = %q, want %q", spec.WorkspaceRef.Kind, hub.WorkspaceKindDir)
	}
	if spec.WorkspaceRef.Path != workspaceRoot {
		t.Fatalf("WorkspaceRef.Path = %q, want %q", spec.WorkspaceRef.Path, workspaceRoot)
	}
}

func TestHubPublishSpecUsesOpenClawWorkspaceSnapshot(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:1", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	created, err := svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:          "u-alice",
			Name:        "alice",
			Description: "openclaw worker",
			RuntimeKind: RuntimeKindOpenClawSandbox,
			Image:       "openclaw-image:1",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	agentHome, err := agentHomeDir(created.ID)
	if err != nil {
		t.Fatalf("agentHomeDir() error = %v", err)
	}
	workspaceRoot := filepath.Join(openclawsandbox.Root(agentHome), openclawsandbox.HostWorkspaceDir)
	if err := os.WriteFile(filepath.Join(workspaceRoot, "PLAYBOOK.md"), []byte("openclaw workspace snapshot\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(PLAYBOOK.md) error = %v", err)
	}

	spec, err := svc.HubPublishSpec(created.ID)
	if err != nil {
		t.Fatalf("HubPublishSpec() error = %v", err)
	}
	if spec.RuntimeKind != RuntimeKindOpenClawSandbox {
		t.Fatalf("RuntimeKind = %q, want %q", spec.RuntimeKind, RuntimeKindOpenClawSandbox)
	}
	if spec.WorkspaceRef.Path != workspaceRoot {
		t.Fatalf("WorkspaceRef.Path = %q, want %q", spec.WorkspaceRef.Path, workspaceRoot)
	}
	if spec.WorkspaceRef.Path == filepath.Join(picoclawsandbox.Root(agentHome), picoclawsandbox.HostWorkspaceDir) {
		t.Fatalf("WorkspaceRef.Path = %q, want OpenClaw workspace root", spec.WorkspaceRef.Path)
	}
}

func TestStartConfiguredAgentsStartsStoppedCompleteWorkersAndLeavesRunningWorkersUntouched(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rt := &fakeRuntime{}
	infos := map[string]sandbox.Info{
		"box-alice": {
			ID:        "box-alice",
			Name:      "csgclaw-agent-alice",
			State:     sandbox.StateStopped,
			CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
		},
		"box-carol": {
			ID:        "box-carol",
			Name:      "csgclaw-agent-carol",
			State:     sandbox.StateRunning,
			CreatedAt: time.Date(2026, 4, 1, 13, 0, 0, 0, time.UTC),
		},
	}
	SetTestHooks(
		func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil },
		nil,
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
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
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
		RuntimeKind:     RuntimeKindPicoClawSandbox,
		BoxID:           "box-alice",
		AgentProfile:    completeAlice,
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}
	svc.agents["u-bob"] = Agent{
		ID:              "u-bob",
		Name:            "bob",
		Role:            RoleWorker,
		RuntimeKind:     RuntimeKindPicoClawSandbox,
		BoxID:           "box-bob",
		AgentProfile:    incompleteBob,
		ProfileComplete: false,
		CreatedAt:       time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
	}
	svc.agents["u-carol"] = Agent{
		ID:              "u-carol",
		Name:            "carol",
		Role:            RoleWorker,
		RuntimeKind:     RuntimeKindPicoClawSandbox,
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
	if carol.BoxID != "box-carol" {
		t.Fatalf("Agent(u-carol).BoxID = %q, want box-carol", carol.BoxID)
	}
	if carol.Status != string(sandbox.StateRunning) {
		t.Fatalf("Agent(u-carol).Status = %q, want running", carol.Status)
	}
}

func TestStartConfiguredAgentsRecreatesRunningLegacyNamedWorkerBox(t *testing.T) {
	for _, tt := range []struct {
		name        string
		runtimeKind string
	}{
		{name: "picoclaw", runtimeKind: RuntimeKindPicoClawSandbox},
		{name: "openclaw", runtimeKind: RuntimeKindOpenClawSandbox},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			rt := &fakeRuntime{}
			infos := map[string]sandbox.Info{
				"box-alice": {
					ID:        "box-alice",
					Name:      "alice",
					State:     sandbox.StateRunning,
					CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
				},
				"alice": {
					ID:        "box-alice",
					Name:      "alice",
					State:     sandbox.StateRunning,
					CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
				},
			}
			var created []string
			SetTestHooks(
				func(_ *Service, _ string) (sandbox.Runtime, error) { return rt, nil },
				func(_ *Service, _ context.Context, _ sandbox.Runtime, image, name, botID string, profile AgentProfile) (sandbox.Instance, sandbox.Info, error) {
					if image != "worker-image:1" {
						t.Fatalf("createGatewayBox() image = %q, want %q", image, "worker-image:1")
					}
					if name != "alice" {
						t.Fatalf("createGatewayBox() name = %q, want %q", name, "alice")
					}
					if botID != "agent-alice" {
						t.Fatalf("createGatewayBox() botID = %q, want %q", botID, "agent-alice")
					}
					if !profile.ProfileComplete || profile.Provider != ProviderCodex || profile.ModelID != "gpt-5.5" {
						t.Fatalf("createGatewayBox() profile = %+v, want complete codex gpt-5.5", profile)
					}
					created = append(created, botID)
					info := sandbox.Info{
						ID:        "box-canonical-alice",
						Name:      "csgclaw-agent-alice",
						State:     sandbox.StateRunning,
						CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
					}
					infos[info.ID] = info
					infos[info.Name] = info
					return &fakeInfoInstance{info: info}, info, nil
				},
			)
			defer ResetTestHooks()

			var gotKeys []string
			testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
				gotKeys = append(gotKeys, idOrName)
				info, ok := infos[idOrName]
				if !ok {
					return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
				}
				return &fakeInfoInstance{info: info}, nil
			}
			testBoxInfoHook = func(_ *Service, _ context.Context, box sandbox.Instance) (sandbox.Info, error) {
				return box.Info(context.Background())
			}
			var started []string
			testStartBoxHook = func(_ *Service, _ context.Context, box sandbox.Instance) error {
				info, err := box.Info(context.Background())
				if err != nil {
					return err
				}
				started = append(started, info.ID)
				return nil
			}
			var removed []string
			testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) error {
				removed = append(removed, idOrName)
				delete(infos, idOrName)
				delete(infos, "alice")
				return nil
			}

			svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}
			completeAlice := AgentProfile{Name: "alice", Provider: ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true}
			svc.agents["agent-alice"] = Agent{
				ID:              "agent-alice",
				Name:            "alice",
				Role:            RoleWorker,
				RuntimeKind:     tt.runtimeKind,
				Image:           "worker-image:1",
				BoxID:           "box-alice",
				Status:          string(sandbox.StateRunning),
				AgentProfile:    completeAlice,
				ProfileComplete: true,
				CreatedAt:       time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
			}

			if err := svc.StartConfiguredAgents(context.Background()); err != nil {
				t.Fatalf("StartConfiguredAgents() error = %v", err)
			}
			if strings.Join(created, ",") != "agent-alice" {
				t.Fatalf("created boxes = %q, want agent-alice", created)
			}
			if strings.Join(removed, ",") != "box-alice" {
				t.Fatalf("removed boxes = %q, want box-alice", removed)
			}
			if len(started) != 0 {
				t.Fatalf("startBox() calls = %q, want none because old running box is recreated", started)
			}
			if len(gotKeys) < 1 || gotKeys[0] != "box-alice" {
				t.Fatalf("getBox() leading keys = %q, want box-alice first", gotKeys)
			}
			got, ok := svc.Agent("agent-alice")
			if !ok {
				t.Fatal("Agent() missing agent-alice")
			}
			if got.BoxID != "box-canonical-alice" {
				t.Fatalf("Agent().BoxID = %q, want %q", got.BoxID, "box-canonical-alice")
			}
			if got.Status != string(sandbox.StateRunning) {
				t.Fatalf("Agent().Status = %q, want running", got.Status)
			}
		})
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
	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:          "u-alice",
		Name:        "alice",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		BoxID:       "box-stale",
		Role:        RoleWorker,
		Status:      "running",
		CreatedAt:   time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	got, err := svc.Stop(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if len(gotKeys) < 3 || gotKeys[0] != "box-stale" || gotKeys[1] != "csgclaw-agent-alice" || gotKeys[2] != "alice" {
		t.Fatalf("getBox() leading keys = %q, want stale box id, stable sandbox name, then display name fallback", gotKeys)
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

	reloaded, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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

func TestStopTriggersLifecycleObserver(t *testing.T) {
	observer := &fakeLifecycleObserver{}
	svc, err := NewService(
		config.ModelConfig{},
		config.ServerConfig{}, "manager-image:test", "",
		WithLifecycleObserver(observer),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			stop: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
				return agentruntime.StateStopped, nil
			},
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateStopped}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:              "u-alice",
		Name:            "alice",
		Role:            RoleWorker,
		RuntimeID:       "rt-u-alice",
		RuntimeKind:     RuntimeKindCodex,
		BoxID:           "box-alice",
		Status:          string(agentruntime.StateRunning),
		AgentProfile:    AgentProfile{Name: "alice", Provider: ProviderCodex, ModelID: "gpt-5.4", ProfileComplete: true},
		ProfileComplete: true,
	}

	if _, err := svc.Stop(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if len(observer.stopCalls) != 1 || observer.stopCalls[0] != "u-alice" {
		t.Fatalf("StopAgent() calls = %v, want [u-alice]", observer.stopCalls)
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
		config.ServerConfig{}, "manager-image:test", "",
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.Create(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:    "agent-1",
			Name:  "alice",
			Image: "test-image",
			Role:  RoleAgent,
		},
	})
	if err == nil || !strings.Contains(err.Error(), `role must be one of "manager" or "worker"`) {
		t.Fatalf("Create() error = %v, want invalid-role error", err)
	}
	if closeCalls != 0 {
		t.Fatalf("closeBox() calls = %d, want %d", closeCalls, 0)
	}
	if closeRuntimeCalls != 0 {
		t.Fatalf("closeRuntime() calls = %d, want %d", closeRuntimeCalls, 0)
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

			_, err = svc.Create(context.Background(), CreateRequest{
				Spec: CreateAgentSpec{
					ID:    "agent-1",
					Name:  "alice",
					Image: tt.reqImage,
					Role:  RoleAgent,
				},
			})
			if err == nil || !strings.Contains(err.Error(), `role must be one of "manager" or "worker"`) {
				t.Fatalf("Create() error = %v, want invalid-role error", err)
			}
			if gotSpec.Image != "" {
				t.Fatalf("createBox() spec.Image = %q, want empty because no box should be created", gotSpec.Image)
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
				ID:          ManagerUserID,
				Name:        ManagerName,
				RuntimeKind: RuntimeKindPicoClawSandbox,
				Role:        RoleManager,
				BoxID:       "box-old",
				CreatedAt:   time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := EnsureBootstrapState(context.Background(), statePath, config.ServerConfig{}, testModelConfig(), "manager-image:test", true); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}
	if removed != "box-old" {
		t.Fatalf("ForceRemove() target = %q, want %q", removed, "box-old")
	}

	reloaded, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", statePath)
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
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	runtimeHome, err := svc.sandboxRuntimeHome(ManagerName)
	if err != nil {
		t.Fatalf("svc.sandboxRuntimeHome() error = %v", err)
	}
	managerHome, err := svc.agentHomeDir(ManagerUserID)
	if err != nil {
		t.Fatalf("svc.agentHomeDir() error = %v", err)
	}
	stalePath := filepath.Join(managerHome, "stale.txt")
	if err := os.MkdirAll(managerHome, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(stalePath, []byte("stale"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	currentSkillPath := filepath.Join(filepath.Join(picoclawsandbox.Root(managerHome), picoclawsandbox.HostWorkspaceDir), "skills", "say-hello", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(currentSkillPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(current skill) error = %v", err)
	}
	if err := os.WriteFile(currentSkillPath, []byte("# Say Hello\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(current skill) error = %v", err)
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
		if data, err := os.ReadFile(currentSkillPath); err != nil || string(data) != "# Say Hello\n" {
			t.Fatalf("current skill before recreate = %q, %v; want preserved skill", string(data), err)
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

	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          ManagerUserID,
				Name:        ManagerName,
				RuntimeKind: RuntimeKindPicoClawSandbox,
				Role:        RoleManager,
				BoxID:       "box-old",
				CreatedAt:   time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := EnsureBootstrapState(context.Background(), statePath, config.ServerConfig{}, testModelConfig(), "manager-image:test", true); err != nil {
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
	if data, err := os.ReadFile(currentSkillPath); err != nil || string(data) != "# Say Hello\n" {
		t.Fatalf("current skill after recreate = %q, %v; want preserved skill", string(data), err)
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
	if err := EnsureBootstrapState(context.Background(), statePath, config.ServerConfig{}, testModelConfig(), "manager-image:test", false); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}
	if closeCalls != 1 {
		t.Fatalf("closeBox() calls = %d, want %d", closeCalls, 1)
	}
	if closeRuntimeCalls != 1 {
		t.Fatalf("closeRuntime() calls = %d, want %d", closeRuntimeCalls, 1)
	}

	reloaded, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", statePath)
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
			Name:      "csgclaw-agent-manager",
			State:     sandbox.StateRunning,
			CreatedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
		}, nil
	}

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          ManagerUserID,
				Name:        ManagerName,
				RuntimeKind: RuntimeKindPicoClawSandbox,
				Role:        RoleManager,
				BoxID:       "box-old",
				CreatedAt:   time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := EnsureBootstrapState(context.Background(), statePath, config.ServerConfig{}, config.ModelConfig{}, "manager-image:test", false); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}
	if created {
		t.Fatal("createGatewayBox() called, want existing manager box to be reused")
	}
}

func TestEnsureBootstrapStateRecreatesLegacyNamedManagerBoxWithoutForce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	SetTestHooks(nil, nil)
	defer ResetTestHooks()

	primaryRT := &fakeRuntime{}
	testEnsureRuntimeAtHomeHook = func(_ *Service, home string) (sandbox.Runtime, error) {
		return primaryRT, nil
	}

	var created bool
	testCreateGatewayBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string, name, botID string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
		created = true
		if name != ManagerName {
			t.Fatalf("createGatewayBox() name = %q, want %q", name, ManagerName)
		}
		if botID != ManagerUserID {
			t.Fatalf("createGatewayBox() botID = %q, want %q", botID, ManagerUserID)
		}
		info := sandbox.Info{
			ID:        "box-canonical-manager",
			Name:      "csgclaw-agent-manager",
			State:     sandbox.StateRunning,
			CreatedAt: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC),
		}
		return &fakeInfoInstance{info: info}, info, nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, rt sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		if rt == primaryRT && idOrName == "box-old" {
			return &fakeInfoInstance{info: sandbox.Info{
				ID:        "box-old",
				Name:      ManagerName,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
			}}, nil
		}
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	var removed []string
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) error {
		removed = append(removed, idOrName)
		return nil
	}
	testStartBoxHook = func(_ *Service, _ context.Context, _ sandbox.Instance) error { return nil }
	testBoxInfoHook = func(_ *Service, _ context.Context, box sandbox.Instance) (sandbox.Info, error) {
		return box.Info(context.Background())
	}

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          ManagerUserID,
				Name:        ManagerName,
				RuntimeKind: RuntimeKindPicoClawSandbox,
				Role:        RoleManager,
				BoxID:       "box-old",
				CreatedAt:   time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
				AgentProfile: AgentProfile{
					Name:            ManagerName,
					Provider:        ProviderCodex,
					ModelID:         "gpt-5.5",
					ProfileComplete: true,
				},
				ProfileComplete: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := EnsureBootstrapState(context.Background(), statePath, config.ServerConfig{}, config.ModelConfig{}, "manager-image:test", false); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}
	if !created {
		t.Fatal("createGatewayBox() was not called for legacy-named manager box")
	}
	if strings.Join(removed, ",") != "box-old" {
		t.Fatalf("removed boxes = %q, want box-old", removed)
	}

	reloaded, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() reload error = %v", err)
	}
	got, ok := reloaded.Agent(ManagerUserID)
	if !ok {
		t.Fatal("Agent() missing manager")
	}
	if got.BoxID != "box-canonical-manager" {
		t.Fatalf("Agent().BoxID = %q, want box-canonical-manager", got.BoxID)
	}
}

func TestEnsureBootstrapStateRecreatesManagerWithLegacyPicoClawBridgeConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	statePath := filepath.Join(t.TempDir(), "agents.json")
	seedSvc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	SetTestHooks(nil, nil)
	defer ResetTestHooks()

	primaryRT := &fakeRuntime{}
	testEnsureRuntimeAtHomeHook = func(_ *Service, home string) (sandbox.Runtime, error) {
		return primaryRT, nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, rt sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		if rt == primaryRT && idOrName == "box-old" {
			return &fakeInfoInstance{info: sandbox.Info{
				ID:        "box-old",
				Name:      ManagerName,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
			}}, nil
		}
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	var removed []string
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) error {
		removed = append(removed, idOrName)
		return nil
	}
	var created bool
	testCreateGatewayBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, image, name, botID string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
		created = true
		if image != "manager-image:test" || name != ManagerName || botID != ManagerUserID {
			t.Fatalf("createGatewayBox() got image=%q name=%q botID=%q", image, name, botID)
		}
		return &fakeInfoInstance{info: sandbox.Info{
				ID:        "box-new",
				Name:      ManagerName,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC),
			}}, sandbox.Info{
				ID:        "box-new",
				Name:      ManagerName,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC),
			}, nil
	}

	managerHome, err := seedSvc.agentHomeDir(ManagerUserID)
	if err != nil {
		t.Fatalf("seedSvc.agentHomeDir() error = %v", err)
	}
	configPath := filepath.Join(managerHome, picoclawsandbox.HostDir, picoclawsandbox.HostConfig)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir) error = %v", err)
	}
	legacyConfig := `{"channels":{"csgclaw":{"enabled":true,"bot_id":"u-manager"}}}`
	if err := os.WriteFile(configPath, []byte(legacyConfig), 0o600); err != nil {
		t.Fatalf("WriteFile(legacy config) error = %v", err)
	}

	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          ManagerUserID,
				Name:        ManagerName,
				RuntimeKind: RuntimeKindPicoClawSandbox,
				Role:        RoleManager,
				BoxID:       "box-old",
				CreatedAt:   time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
				AgentProfile: AgentProfile{
					Name:            ManagerName,
					Provider:        ProviderAPI,
					BaseURL:         "https://api.example/v1",
					APIKey:          "api-key",
					ModelID:         "gpt-4.1",
					ProfileComplete: true,
				},
				ProfileComplete: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	if err := EnsureBootstrapState(context.Background(), statePath, config.ServerConfig{ListenAddr: ":18080", AccessToken: "token"}, testModelConfig(), "manager-image:test", false); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}
	if !created {
		t.Fatal("createGatewayBox() was not called; legacy manager bridge config should force recreate")
	}
	if got, want := strings.Join(removed, ","), "box-old"; got != want {
		t.Fatalf("removed boxes = %q, want %q", got, want)
	}
	rendered, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(rendered config) error = %v", err)
	}
	if !strings.Contains(string(rendered), `"participant_id": "`+ManagerParticipantID+`"`) {
		t.Fatalf("rendered config missing participant_id:\n%s", rendered)
	}
	if strings.Contains(string(rendered), `"bot_id"`) {
		t.Fatalf("rendered config still contains bot_id:\n%s", rendered)
	}
}

func TestEnsureBootstrapManagerDoesNotRemoveExistingBoxWhenMigrationProvisionFails(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	SetTestHooks(nil, nil)
	defer ResetTestHooks()

	primaryRT := &fakeRuntime{}
	testEnsureRuntimeAtHomeHook = func(_ *Service, home string) (sandbox.Runtime, error) {
		return primaryRT, nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, rt sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		if rt == primaryRT && idOrName == "box-old" {
			return &fakeInfoInstance{info: sandbox.Info{
				ID:        "box-old",
				Name:      ManagerName,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
			}}, nil
		}
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	var removed []string
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) error {
		removed = append(removed, idOrName)
		return nil
	}
	testCreateGatewayBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _, _, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
		t.Fatal("createGatewayBox() called after provisioning failed")
		return nil, sandbox.Info{}, nil
	}

	managerHome, err := agentHomeDir(ManagerUserID)
	if err != nil {
		t.Fatalf("agentHomeDir() error = %v", err)
	}
	configPath := filepath.Join(managerHome, picoclawsandbox.HostDir, picoclawsandbox.HostConfig)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir) error = %v", err)
	}
	legacyConfig := `{"channels":{"csgclaw":{"enabled":true,"bot_id":"u-manager"}}}`
	if err := os.WriteFile(configPath, []byte(legacyConfig), 0o600); err != nil {
		t.Fatalf("WriteFile(legacy config) error = %v", err)
	}

	statePath := filepath.Join(t.TempDir(), "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          ManagerUserID,
				Name:        ManagerName,
				RuntimeKind: RuntimeKindPicoClawSandbox,
				Role:        RoleManager,
				BoxID:       "box-old",
				CreatedAt:   time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
				AgentProfile: AgentProfile{
					Name:            ManagerName,
					Provider:        ProviderAPI,
					BaseURL:         "https://api.example/v1",
					APIKey:          "api-key",
					ModelID:         "gpt-4.1",
					ProfileComplete: true,
				},
				ProfileComplete: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "token"},
		"manager-image:test",
		statePath,
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindPicoClawSandbox,
			provision: func(context.Context, agentruntime.ProvisionRequest) error {
				return fmt.Errorf("provision failed")
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	err = svc.EnsureBootstrapManager(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "provision failed") {
		t.Fatalf("EnsureBootstrapManager() error = %v, want provision failure", err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed boxes = %#v, want none when migration provisioning fails", removed)
	}
}

func TestEnsureBootstrapManagerUsesStoredManagerProfileWhenDefaultModelIsInvalid(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	SetTestHooks(nil, nil)
	defer ResetTestHooks()

	primaryRT := &fakeRuntime{}
	testEnsureRuntimeAtHomeHook = func(_ *Service, home string) (sandbox.Runtime, error) {
		return primaryRT, nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testCreateGatewayBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, image, name, botID string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
		if image != "manager-image:test" || name != ManagerName || botID != ManagerUserID {
			t.Fatalf("createGatewayBox() got image=%q name=%q botID=%q", image, name, botID)
		}
		return &fakeInfoInstance{info: sandbox.Info{
				ID:        "box-new",
				Name:      ManagerName,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC),
			}}, sandbox.Info{
				ID:        "box-new",
				Name:      ManagerName,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC),
			}, nil
	}

	statePath := filepath.Join(t.TempDir(), "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          ManagerUserID,
				Name:        ManagerName,
				RuntimeKind: RuntimeKindPicoClawSandbox,
				Role:        RoleManager,
				BoxID:       "box-old",
				CreatedAt:   time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
				AgentProfile: AgentProfile{
					Name:            ManagerName,
					Provider:        ProviderAPI,
					BaseURL:         "https://api.manager.example/v1",
					APIKey:          "manager-key",
					ModelID:         "manager-model",
					ProfileComplete: true,
				},
				ProfileComplete: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	invalidDefault := config.LLMConfig{
		Default: "openai.gpt-4.1",
		Providers: map[string]config.ProviderConfig{
			"openai": {
				BaseURL: "https://api.default.example/v1",
				APIKey:  "default-key",
				Models:  []string{"other-model"},
			},
		},
	}
	var provisionedModel string
	svc, err := NewServiceWithLLM(
		invalidDefault,
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "token"},
		"manager-image:test",
		statePath,
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindPicoClawSandbox,
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				provisionedModel = req.Profile.ModelID
				return nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewServiceWithLLM() error = %v", err)
	}

	if err := svc.EnsureBootstrapManager(context.Background(), false); err != nil {
		t.Fatalf("EnsureBootstrapManager() error = %v", err)
	}
	if provisionedModel != "manager-model" {
		t.Fatalf("provisioned model = %q, want stored manager profile model", provisionedModel)
	}
	got, ok := svc.Agent(ManagerUserID)
	if !ok {
		t.Fatal("Agent(manager) ok = false, want true")
	}
	if got.BoxID != "box-new" {
		t.Fatalf("manager BoxID = %q, want recreated box", got.BoxID)
	}
}

func TestBoxRuntimeHomeUsesPerAgentDirectory(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.sandboxRuntimeHome("alice")
	if err != nil {
		t.Fatalf("svc.sandboxRuntimeHome() error = %v", err)
	}

	want := filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, "agent-alice", config.RuntimeHomeDirName)
	if got != want {
		t.Fatalf("sandboxRuntimeHome() = %q, want %q", got, want)
	}
}

func TestBoxRuntimeHomeUsesServiceStateRoot(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	stateRoot := t.TempDir()
	statePath := filepath.Join(stateRoot, "agents.json")

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.sandboxRuntimeHome("alice")
	if err != nil {
		t.Fatalf("svc.sandboxRuntimeHome() error = %v", err)
	}

	want := filepath.Join(stateRoot, managerAgentsDirName, "agent-alice", config.RuntimeHomeDirName)
	if got != want {
		t.Fatalf("sandboxRuntimeHome() = %q, want %q", got, want)
	}
}

func TestLookupBootstrapManagerUsesPerAgentHome(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	var gotHome string
	var removed []string
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) error {
		removed = append(removed, idOrName)
		return fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	defer func() {
		testForceRemoveBoxHook = nil
	}()

	provider := fakeProvider{
		open: func(_ context.Context, homeDir string) (sandbox.Runtime, error) {
			gotHome = homeDir
			return &fakeRuntime{}, nil
		},
	}

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "", WithSandboxProvider(provider))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	wantHome, err := svc.sandboxRuntimeHome(ManagerName)
	if err != nil {
		t.Fatalf("svc.sandboxRuntimeHome() error = %v", err)
	}

	rt, box, err := svc.lookupBootstrapManager(context.Background())
	if err != nil {
		t.Fatalf("lookupBootstrapManager() error = %v", err)
	}
	if box != nil {
		t.Fatalf("lookupBootstrapManager() box = %#v, want nil", box)
	}
	if rt == nil {
		t.Fatal("lookupBootstrapManager() runtime = nil, want non-nil")
	}
	if info, err := os.Stat(wantHome); err != nil {
		t.Fatalf("os.Stat(runtime home) error = %v", err)
	} else if !info.IsDir() {
		t.Fatalf("runtime home is not a directory: %q", wantHome)
	}
	if got, want := len(svc.runtimes), 1; got != want {
		t.Fatalf("len(svc.runtimes) = %d, want %d", got, want)
	}
	if got, want := gotHome, wantHome; got != want {
		t.Fatalf("resolved manager runtime home = %q, want %q", got, want)
	}
	if len(removed) != 2 || removed[0] != "csgclaw-agent-manager" || removed[1] != ManagerName {
		t.Fatalf("removed stale manager boxes = %#v, want [csgclaw-agent-manager %q]", removed, ManagerName)
	}
}

func TestLookupBootstrapManagerRemovesOrphanManagerWhenNoRecord(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	var removed []string
	testEnsureRuntimeAtHomeHook = func(_ *Service, homeDir string) (sandbox.Runtime, error) {
		if homeDir == "" {
			t.Fatalf("ensureRuntimeAtHome() homeDir = %q, want non-empty", homeDir)
		}
		return &fakeRuntime{}, nil
	}
	testForceRemoveBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) error {
		removed = append(removed, idOrName)
		return nil
	}
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		t.Fatalf("lookupBootstrapManager() should not reuse orphan manager box %q", idOrName)
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	defer func() {
		testEnsureRuntimeAtHomeHook = nil
		testForceRemoveBoxHook = nil
		testGetBoxHook = nil
	}()

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	rt, box, err := svc.lookupBootstrapManager(context.Background())
	if err != nil {
		t.Fatalf("lookupBootstrapManager() error = %v", err)
	}
	if rt == nil {
		t.Fatal("lookupBootstrapManager() runtime = nil, want non-nil")
	}
	if box != nil {
		t.Fatalf("lookupBootstrapManager() box = %#v, want nil", box)
	}
	if len(removed) != 1 || removed[0] != "csgclaw-agent-manager" {
		t.Fatalf("removed stale manager boxes = %#v, want [csgclaw-agent-manager]", removed)
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

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
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
	if len(lookedUp) != 3 {
		t.Fatalf("lookupBootstrapManager() called times = %d, want %d", len(lookedUp), 3)
	}
	if lookedUp[0] != "box-stale" {
		t.Fatalf("lookupBootstrapManager() first lookup = %q, want %q", lookedUp[0], "box-stale")
	}
	if lookedUp[1] != "csgclaw-agent-manager" {
		t.Fatalf("lookupBootstrapManager() second lookup = %q, want %q", lookedUp[1], "csgclaw-agent-manager")
	}
	if lookedUp[2] != ManagerName {
		t.Fatalf("lookupBootstrapManager() third lookup = %q, want %q", lookedUp[2], ManagerName)
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

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
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
	if len(lookedUp) != 2 {
		t.Fatalf("lookupBootstrapManager() called times = %d, want %d", len(lookedUp), 2)
	}
	if lookedUp[0] != "csgclaw-agent-manager" {
		t.Fatalf("lookupBootstrapManager() first lookup = %q, want %q", lookedUp[0], "csgclaw-agent-manager")
	}
	if lookedUp[1] != ManagerName {
		t.Fatalf("lookupBootstrapManager() second lookup = %q, want %q", lookedUp[1], ManagerName)
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

	apps := map[string]feishu.AppConfig{
		"u-worker-1": {
			AppID:     "cli_worker",
			AppSecret: "worker-secret",
		},
	}
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"}, "manager-image:test", "",
		withTestPicoClawSandboxRuntime(apps),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	rt, err := svc.runtimeForKind(RuntimeKindPicoClawSandbox)
	if err != nil {
		t.Fatalf("runtimeForKind() error = %v", err)
	}
	if err := svc.provisionRuntime(context.Background(), rt, RuntimeKindPicoClawSandbox, agentruntime.ProvisionRequest{
		RuntimeID: "rt-u-worker-1",
		AgentID:   "u-worker-1",
		AgentName: "alice",
		Profile: agentruntime.Profile{
			Provider: ProviderAPI,
			ModelID:  "minimax-m2.7",
		},
	}); err != nil {
		t.Fatalf("provisionRuntime() error = %v", err)
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
	if spec.Name != "csgclaw-u-worker-1" {
		t.Fatalf("gatewayCreateSpec() name = %q, want %q", spec.Name, "csgclaw-u-worker-1")
	}
	if !spec.Detach {
		t.Fatal("gatewayCreateSpec() detach = false, want true")
	}
	if spec.AutoRemove {
		t.Fatal("gatewayCreateSpec() auto_remove = true, want false")
	}
	cmd := strings.Join(spec.Cmd, " ")
	if strings.Contains(cmd, "/csgclaw-projects") || strings.Contains(cmd, "ln -sfn") {
		t.Fatalf("gatewayCreateSpec() cmd = %q, want direct projects mount without symlink setup", spec.Cmd)
	}
	if !strings.HasSuffix(cmd, picoclawsandbox.GatewayRunCommand()) {
		t.Fatalf("gatewayCreateSpec() cmd = %q, want suffix %q", spec.Cmd, picoclawsandbox.GatewayRunCommand())
	}
	if got, want := spec.Env["HOME"], "/home/picoclaw"; got != want {
		t.Fatalf("HOME env = %q, want %q", got, want)
	}
	if got, want := spec.Env["CSGCLAW_BASE_URL"], "http://10.0.0.8:18080"; got != want {
		t.Fatalf("CSGCLAW_BASE_URL = %q, want %q", got, want)
	}
	if got, want := spec.Env["CSGCLAW_LLM_BASE_URL"], "http://10.0.0.8:18080/api/v1/agents/u-worker-1/llm"; got != want {
		t.Fatalf("CSGCLAW_LLM_BASE_URL = %q, want %q", got, want)
	}
	if got, want := spec.Env["PICOCLAW_CHANNELS_FEISHU_APP_ID"], "cli_worker"; got != want {
		t.Fatalf("PICOCLAW_CHANNELS_FEISHU_APP_ID = %q, want %q", got, want)
	}
	runUser, err := hostuser.RunUser()
	if err != nil {
		t.Skip("host uid/gid unavailable")
	}
	if spec.RunUser != runUser {
		t.Fatalf("gatewayCreateSpec() RunUser = %q, want %q", spec.RunUser, runUser)
	}

	wantAgentHome := filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, "agent-worker-1")
	wantPicoClawRoot := picoclawsandbox.Root(wantAgentHome)
	wantWorkspaceRoot := filepath.Join(picoclawsandbox.Root(wantAgentHome), picoclawsandbox.HostWorkspaceDir)
	wantConfigRoot := wantPicoClawRoot
	wantProjectsRoot := filepath.Join(homeDir, config.AppDirName, hostProjectsDir)
	if len(spec.Mounts) != 2 {
		t.Fatalf("gatewayCreateSpec() mounts = %+v, want 2 mounts", spec.Mounts)
	}
	if spec.Mounts[0].HostPath != wantPicoClawRoot || spec.Mounts[0].GuestPath != picoclawsandbox.BoxDir {
		t.Fatalf("runtime root mount = %+v, want host %q guest %q", spec.Mounts[0], wantPicoClawRoot, picoclawsandbox.BoxDir)
	}
	if spec.Mounts[1].HostPath != wantProjectsRoot || spec.Mounts[1].GuestPath != picoclawsandbox.BoxProjectsDir {
		t.Fatalf("projects mount = %+v, want host %q guest %q", spec.Mounts[1], wantProjectsRoot, picoclawsandbox.BoxProjectsDir)
	}
	if _, err := os.Stat(filepath.Join(wantConfigRoot, picoclawsandbox.HostConfig)); err != nil {
		t.Fatalf("worker PicoClaw config was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantWorkspaceRoot, "AGENT.md")); err != nil {
		t.Fatalf("worker workspace was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantAgentHome, hostWorkspaceDir)); !os.IsNotExist(err) {
		t.Fatalf("legacy workspace stat error = %v, want not exist", err)
	}
}

func TestGatewayProvisionRequestBuildsOpenClawWorkerAssets(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	orig := localIPv4Resolver
	localIPv4Resolver = func() string { return "10.0.0.8" }
	defer func() { localIPv4Resolver = orig }()

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"openclaw-csgclaw:local",
		"",
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	gateway, err := svc.gatewayProvisionRequest(RuntimeKindOpenClawSandbox, "alice", "u-worker-1")
	if err != nil {
		t.Fatalf("gatewayProvisionRequest() error = %v", err)
	}
	wantAgentHome := filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, "agent-worker-1")
	wantOpenClawRoot := openclawsandbox.Root(wantAgentHome)
	if gateway.AgentHome != wantAgentHome {
		t.Fatalf("Gateway.AgentHome = %q, want %q", gateway.AgentHome, wantAgentHome)
	}
	if gateway.WorkspaceTemplate != templateembed.OpenClawWorkerRoot {
		t.Fatalf("Gateway.WorkspaceTemplate(worker) = %q, want %q", gateway.WorkspaceTemplate, templateembed.OpenClawWorkerRoot)
	}
	managerGateway, err := svc.gatewayProvisionRequest(RuntimeKindOpenClawSandbox, ManagerName, ManagerUserID)
	if err != nil {
		t.Fatalf("gatewayProvisionRequest(manager) error = %v", err)
	}
	if managerGateway.WorkspaceTemplate != templateembed.OpenClawManagerRoot {
		t.Fatalf("Gateway.WorkspaceTemplate(manager) = %q, want %q", managerGateway.WorkspaceTemplate, templateembed.OpenClawManagerRoot)
	}
	rt := openclawsandbox.New(sandboxgateway.Dependencies{})
	if err := rt.Provision(context.Background(), agentruntime.ProvisionRequest{
		RuntimeID: "rt-u-worker-1",
		AgentID:   "u-worker-1",
		AgentName: "alice",
		Profile: agentruntime.Profile{
			BaseURL: "https://api.minimaxi.com/v1",
			APIKey:  "sk-minimax-test",
			ModelID: "MiniMax-M2.7",
		},
		Gateway: gateway,
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantOpenClawRoot, openclawsandbox.HostWorkspaceDir, "AGENTS.md")); err != nil {
		t.Fatalf("expected openclaw workspace template under openclaw root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantOpenClawRoot, openclawsandbox.HostWorkspaceDir, "MEMORY.md")); !os.IsNotExist(err) {
		t.Fatalf("MEMORY.md should not be seeded for openclaw worker, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantOpenClawRoot, openclawsandbox.HostWorkspaceDir, "AGENT.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENT.md should not be seeded for openclaw worker, stat error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(wantOpenClawRoot, openclawsandbox.HostConfig))
	if err != nil {
		t.Fatalf("ReadFile(openclaw config) error = %v", err)
	}
	cfgText := string(data)
	if strings.Contains(cfgText, "csg-skills") {
		t.Fatalf("openclaw config should not load the manager-only CSG skill pack, got:\n%s", cfgText)
	}
	if !strings.Contains(cfgText, `"security": "full"`) || !strings.Contains(cfgText, `"ask": "off"`) {
		t.Fatalf("openclaw config should disable exec approval prompts (tools.exec security=full ask=off), got:\n%s", cfgText)
	}
	if !strings.Contains(cfgText, `"verboseDefault": "on"`) {
		t.Fatalf("openclaw config should set agents.defaults.verboseDefault to on for tool stream visibility, got:\n%s", cfgText)
	}
	approvalsRaw, err := os.ReadFile(filepath.Join(wantOpenClawRoot, openclawsandbox.HostExecApproval))
	if err != nil {
		t.Fatalf("ReadFile(openclaw exec-approvals) error = %v", err)
	}
	approvalsText := string(approvalsRaw)
	if !strings.Contains(approvalsText, `"security": "full"`) || !strings.Contains(approvalsText, `"ask": "off"`) {
		t.Fatalf("openclaw exec-approvals should default security=full ask=off, got:\n%s", approvalsText)
	}
}

func TestGatewayProvisionRequestUsesDockerHostAliasForImplicitAdvertiseURL(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	origDockerHostAliasEnabled := dockerHostAliasEnabled
	origLocalIPv4Resolver := localIPv4Resolver
	dockerHostAliasEnabled = func() bool { return true }
	localIPv4Resolver = func() string {
		t.Fatal("local IPv4 resolver should not be used for Docker Desktop gateway URLs")
		return ""
	}
	defer func() {
		dockerHostAliasEnabled = origDockerHostAliasEnabled
		localIPv4Resolver = origLocalIPv4Resolver
	}()

	provider := sandboxtest.NewProvider()
	provider.NameValue = config.DockerProvider
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{ListenAddr: "0.0.0.0:18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithSandboxProvider(provider),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	gateway, err := svc.gatewayProvisionRequest(RuntimeKindPicoClawSandbox, ManagerName, ManagerUserID)
	if err != nil {
		t.Fatalf("gatewayProvisionRequest() error = %v", err)
	}
	if got, want := gateway.ManagerBaseURL, "http://host.docker.internal:18080"; got != want {
		t.Fatalf("Gateway.ManagerBaseURL = %q, want %q", got, want)
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
	appendTemplateImageEnvContracts(t, filepath.Join(registryRoot, "templates", id, "agent.toml"), item.ImageEnv)

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

func appendTemplateImageEnvContracts(t *testing.T, manifestPath string, items []apitypes.ImageEnvContract) {
	t.Helper()
	if len(items) == 0 {
		return
	}
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", manifestPath, err)
	}
	var b strings.Builder
	b.WriteString(strings.Replace(string(content), "env = []\n", "", 1))
	for _, item := range items {
		fmt.Fprintf(&b, "\n[[image.env]]\nname = %q\nrequired = %t\nsecret = %t\n", item.Name, item.Required, item.Secret)
		if item.Default != "" {
			fmt.Fprintf(&b, "default = %q\n", item.Default)
		}
		if item.Description != "" {
			fmt.Fprintf(&b, "description = %q\n", item.Description)
		}
	}
	if err := os.WriteFile(manifestPath, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", manifestPath, err)
	}
}

func testBuiltinLayout(agentName, runtimeKind string) (agentruntime.Layout, error) {
	agentHome, err := agentHomeDir(agentName)
	if err != nil {
		return agentruntime.Layout{}, err
	}
	switch strings.TrimSpace(runtimeKind) {
	case RuntimeKindPicoClawSandbox:
		workspace := filepath.Join(picoclawsandbox.Root(agentHome), picoclawsandbox.HostWorkspaceDir)
		return agentruntime.Layout{
			WorkspaceRoot: workspace,
			SkillsRoot:    filepath.Join(workspace, "skills"),
			HostLogPaths:  []string{picoclawsandbox.HostGatewayLogPath(agentHome)},
		}, nil
	case RuntimeKindOpenClawSandbox:
		workspace := filepath.Join(openclawsandbox.Root(agentHome), openclawsandbox.HostWorkspaceDir)
		return agentruntime.Layout{
			WorkspaceRoot: workspace,
			SkillsRoot:    filepath.Join(workspace, "skills"),
			HostLogPaths:  []string{openclawsandbox.HostGatewayLogPath(agentHome)},
		}, nil
	case RuntimeKindCodex:
		return agentruntime.Layout{
			WorkspaceRoot: filepath.Join(agentHome, ".codex", "workspace"),
			SkillsRoot:    filepath.Join(agentHome, ".codex", "home", "skills"),
			HostLogPaths:  []string{filepath.Join(agentHome, ".codex", "home", "stderr.log")},
		}, nil
	default:
		return agentruntime.Layout{}, fmt.Errorf("unsupported runtime_kind %q for agent workspace", runtimeKind)
	}
}

func testBuiltinWorkspaceRoot(agentName, runtimeKind string) (string, error) {
	layout, err := testBuiltinLayout(agentName, runtimeKind)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(layout.WorkspaceRoot) == "" {
		return "", fmt.Errorf("runtime %q returned empty workspace root", runtimeKind)
	}
	return layout.WorkspaceRoot, nil
}

func agentSkillPath(agentName, runtimeKind, skillName string) (string, error) {
	layout, err := testBuiltinLayout(agentName, runtimeKind)
	if err != nil {
		return "", err
	}
	return filepath.Join(layout.SkillsRoot, skillName, "SKILL.md"), nil
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

func TestWithGatewayRuntimeAcceptsOpenClawManagerRuntime(t *testing.T) {
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"picoclaw:latest",
		"",
		WithGatewayRuntime(RuntimeKindOpenClawSandbox),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer svc.Close()
	if got, want := svc.GatewayRuntime(), RuntimeKindOpenClawSandbox; got != want {
		t.Fatalf("GatewayRuntime() = %q, want %q", got, want)
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

func TestPicoclawSandboxRuntimeKind(t *testing.T) {
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := WithRuntime(fakeAgentRuntime{kind: RuntimeKindPicoClawSandbox})(svc); err != nil {
		t.Fatalf("WithRuntime() error = %v", err)
	}
	rt, err := svc.runtimeForKind(RuntimeKindPicoClawSandbox)
	if err != nil {
		t.Fatalf("runtimeForKind() error = %v", err)
	}
	if got, want := rt.Kind(), RuntimeKindPicoClawSandbox; got != want {
		t.Fatalf("runtime kind = %q, want %q", got, want)
	}
}

func TestServiceWorkspaceRootUsesRegisteredRuntimeCapability(t *testing.T) {
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	const runtimeKind = "custom_runtime"
	if err := WithRuntime(fakeAgentRuntime{
		kind: runtimeKind,
		workspace: func(agentHome string) string {
			return filepath.Join(agentHome, ".custom", "workspace")
		},
	})(svc); err != nil {
		t.Fatalf("WithRuntime() error = %v", err)
	}

	svc.mu.Lock()
	svc.agents["u-alice"] = Agent{ID: "u-alice", Name: "alice", RuntimeKind: runtimeKind}
	svc.mu.Unlock()

	got, err := svc.WorkspaceRoot("alice")
	if err != nil {
		t.Fatalf("WorkspaceRoot() error = %v", err)
	}
	agentHome, err := agentHomeDir("alice")
	if err != nil {
		t.Fatalf("agentHomeDir() error = %v", err)
	}
	want := filepath.Join(agentHome, ".custom", "workspace")
	if got != want {
		t.Fatalf("WorkspaceRoot() = %q, want %q", got, want)
	}

	skillsRoot, err := svc.SkillsRoot("alice")
	if err != nil {
		t.Fatalf("SkillsRoot() error = %v", err)
	}
	wantSkills := filepath.Join(want, "skills")
	if skillsRoot != wantSkills {
		t.Fatalf("SkillsRoot() = %q, want %q", skillsRoot, wantSkills)
	}
}

func TestRuntimeViewUsesRuntimeInfoAndReportsLogSupport(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	if err := writeSeededAgents(statePath, []Agent{
		{
			ID:          "u-alice",
			Name:        "alice",
			RuntimeID:   "rt-u-alice",
			RuntimeKind: RuntimeKindPicoClawSandbox,
			BoxID:       "box-old",
			Role:        RoleWorker,
			Status:      string(agentruntime.StateStopped),
			CreatedAt:   time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := WithRuntime(fakeAgentRuntime{
		kind: RuntimeKindPicoClawSandbox,
		info: func(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
			return agentruntime.Info{
				HandleID: "box-new",
				State:    agentruntime.StateRunning,
			}, nil
		},
		streamLogs: func(context.Context, agentruntime.Handle, agentruntime.LogOptions) error {
			return nil
		},
	})(svc); err != nil {
		t.Fatalf("WithRuntime() error = %v", err)
	}

	view, err := svc.RuntimeView(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("RuntimeView() error = %v", err)
	}
	if view.RuntimeKind != RuntimeKindPicoClawSandbox {
		t.Fatalf("RuntimeView().RuntimeKind = %q, want %q", view.RuntimeKind, RuntimeKindPicoClawSandbox)
	}
	if view.HandleID != "box-new" {
		t.Fatalf("RuntimeView().HandleID = %q, want %q", view.HandleID, "box-new")
	}
	if view.State != agentruntime.StateRunning {
		t.Fatalf("RuntimeView().State = %q, want %q", view.State, agentruntime.StateRunning)
	}
	if !view.LogsSupported {
		t.Fatal("RuntimeView().LogsSupported = false, want true")
	}
}

func TestRuntimeViewMapsRuntimeNotFoundToUnknown(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	if err := writeSeededAgents(statePath, []Agent{
		{
			ID:          "u-alice",
			Name:        "alice",
			RuntimeID:   "rt-u-alice",
			RuntimeKind: RuntimeKindPicoClawSandbox,
			BoxID:       "box-old",
			Role:        RoleWorker,
			Status:      string(agentruntime.StateRunning),
			CreatedAt:   time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := WithRuntime(fakeAgentRuntimeNoLogs{
		kind: RuntimeKindPicoClawSandbox,
		info: func(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
			return agentruntime.Info{}, fmt.Errorf("%w: missing box", sandbox.ErrNotFound)
		},
	})(svc); err != nil {
		t.Fatalf("WithRuntime() error = %v", err)
	}

	view, err := svc.RuntimeView(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("RuntimeView() error = %v", err)
	}
	if view.State != agentruntime.StateUnknown {
		t.Fatalf("RuntimeView().State = %q, want %q", view.State, agentruntime.StateUnknown)
	}
	if view.LogsSupported {
		t.Fatal("RuntimeView().LogsSupported = true, want false")
	}
}

func TestPicoclawBoxEnvVars(t *testing.T) {
	got := picoclawBoxEnvVars(
		"http://10.0.0.8:18080",
		"shared-token",
		"u-worker-1",
		"u-worker-1",
		"http://10.0.0.8:18080/api/v1/agents/u-worker-1/llm",
		"minimax-m2.7",
	)

	wants := map[string]string{
		"CSGCLAW_BASE_URL":                         "http://10.0.0.8:18080",
		"CSGCLAW_ACCESS_TOKEN":                     "shared-token",
		"PICOCLAW_CHANNELS_CSGCLAW_BASE_URL":       "http://10.0.0.8:18080",
		"PICOCLAW_CHANNELS_CSGCLAW_ACCESS_TOKEN":   "shared-token",
		"PICOCLAW_CHANNELS_CSGCLAW_PARTICIPANT_ID": "u-worker-1",
		"PICOCLAW_CHANNELS_CSGCLAW_ENABLED":        "true",
		"CSGCLAW_LLM_BASE_URL":                     "http://10.0.0.8:18080/api/v1/agents/u-worker-1/llm",
		"CSGCLAW_LLM_API_KEY":                      "shared-token",
		"CSGCLAW_LLM_MODEL_ID":                     "minimax-m2.7",
		"OPENAI_BASE_URL":                          "http://10.0.0.8:18080/api/v1/agents/u-worker-1/llm",
		"OPENAI_API_KEY":                           "shared-token",
		"OPENAI_MODEL":                             "minimax-m2.7",
		"PICOCLAW_AGENTS_DEFAULTS_MODEL_NAME":      "minimax-m2.7",
		"PICOCLAW_CUSTOM_MODEL_NAME":               "minimax-m2.7",
		"PICOCLAW_CUSTOM_MODEL_ID":                 "openai/minimax-m2.7",
		"PICOCLAW_CUSTOM_MODEL_API_KEY":            "shared-token",
		"PICOCLAW_CUSTOM_MODEL_BASE_URL":           "http://10.0.0.8:18080/api/v1/agents/u-worker-1/llm",
	}
	for key, want := range wants {
		if got[key] != want {
			t.Fatalf("%s = %q, want %q", key, got[key], want)
		}
	}
	if _, ok := got["PICOCLAW_CHANNELS_CSGCLAW_BOT_ID"]; ok {
		t.Fatalf("PICOCLAW_CHANNELS_CSGCLAW_BOT_ID should not be emitted")
	}
}

func TestPicoclawBoxEnvVarsPrefixesCustomModelIDForSlashNames(t *testing.T) {
	got := picoclawBoxEnvVars(
		"http://10.0.0.8:18080",
		"shared-token",
		"u-worker-1",
		"u-worker-1",
		"http://10.0.0.8:18080/api/v1/agents/u-worker-1/llm",
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
	addFeishuBoxEnvVars(envVars, "u-worker-1", testStaticFeishuProvider{
		apps: map[string]feishu.AppConfig{
			"u-worker-1": {AppID: "cli_worker", AppSecret: "worker-secret"},
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
	addFeishuBoxEnvVars(envVars, ManagerUserID, testStaticFeishuProvider{
		apps: map[string]feishu.AppConfig{
			"manager": {AppID: "cli_manager", AppSecret: "manager-secret"},
		},
	})

	if _, ok := envVars["PICOCLAW_CHANNELS_FEISHU_APP_ID"]; ok {
		t.Fatalf("PICOCLAW_CHANNELS_FEISHU_APP_ID was set for non-matching bot id")
	}
	if _, ok := envVars["PICOCLAW_CHANNELS_FEISHU_APP_SECRET"]; ok {
		t.Fatalf("PICOCLAW_CHANNELS_FEISHU_APP_SECRET was set for non-matching bot id")
	}
}

func picoclawBoxEnvVars(baseURL, accessToken, participantID, agentID, llmBaseURL, modelID string) map[string]string {
	env := bridgeLLMEnvVars(llmBaseURL, accessToken, modelID)
	picoclawModelID := picoclawBridgeModelID(modelID)
	env["CSGCLAW_BASE_URL"] = baseURL
	env["CSGCLAW_ACCESS_TOKEN"] = accessToken
	env["PICOCLAW_CHANNELS_CSGCLAW_BASE_URL"] = baseURL
	env["PICOCLAW_CHANNELS_CSGCLAW_ACCESS_TOKEN"] = accessToken
	env["PICOCLAW_CHANNELS_CSGCLAW_PARTICIPANT_ID"] = participantID
	env["PICOCLAW_CHANNELS_CSGCLAW_ENABLED"] = "true"
	env["PICOCLAW_AGENTS_DEFAULTS_MODEL_NAME"] = modelID
	env["PICOCLAW_CUSTOM_MODEL_NAME"] = modelID
	env["PICOCLAW_CUSTOM_MODEL_ID"] = picoclawModelID
	env["PICOCLAW_CUSTOM_MODEL_API_KEY"] = accessToken
	env["PICOCLAW_CUSTOM_MODEL_BASE_URL"] = llmBaseURL
	return env
}

func addFeishuBoxEnvVars(envVars map[string]string, botID string, provider feishu.AgentCredentialProvider) {
	if envVars == nil {
		return
	}
	botID = strings.TrimSpace(botID)
	if botID == "" || provider == nil {
		return
	}
	_, app, ok := provider.BotConfigForAgent(botID)
	if !ok {
		return
	}
	envVars["PICOCLAW_CHANNELS_FEISHU_APP_ID"] = app.AppID
	envVars["PICOCLAW_CHANNELS_FEISHU_APP_SECRET"] = app.AppSecret
}

func withTestPicoClawSandboxRuntime(apps ...map[string]feishu.AppConfig) ServiceOption {
	return func(s *Service) error {
		var provider feishu.AgentCredentialProvider
		if len(apps) > 0 && len(apps[0]) > 0 {
			provider = testStaticFeishuProvider{apps: cloneTestFeishuApps(apps[0])}
		}
		if err := withTestSandboxRuntimeHost(s.PicoClawRuntimeHost(), provider, func(deps sandboxgateway.Dependencies) agentruntime.Runtime {
			return picoclawsandbox.New(deps)
		})(s); err != nil {
			return err
		}
		if err := withTestSandboxRuntimeHost(s.OpenClawRuntimeHost(), nil, func(deps sandboxgateway.Dependencies) agentruntime.Runtime {
			return openclawsandbox.New(deps)
		})(s); err != nil {
			return err
		}
		return nil
	}
}

func withTestSandboxRuntimeHost(host PicoClawRuntimeHost, provider feishu.AgentCredentialProvider, newRuntime func(sandboxgateway.Dependencies) agentruntime.Runtime) ServiceOption {
	return func(s *Service) error {
		return WithRuntime(newRuntime(sandboxgateway.Dependencies{
			FeishuProvider: provider,
			EnsureRuntime:  host.EnsureRuntime,
			RuntimeHome:    host.RuntimeHome,
			CloseRuntime:   host.CloseRuntime,
			ResolveBox: func(ctx context.Context, rt sandbox.Runtime, got sandboxgateway.AgentRef) (sandbox.Instance, string, error) {
				return host.ResolveBox(ctx, rt, Agent{
					ID:        got.ID,
					Name:      got.Name,
					RuntimeID: got.RuntimeID,
					BoxID:     got.BoxID,
				})
			},
			CreateBox:      host.CreateBox,
			StartBox:       host.StartBox,
			StopBox:        host.StopBox,
			BoxInfo:        host.BoxInfo,
			ForceRemoveBox: host.ForceRemoveBox,
			CloseBox:       host.CloseBox,
			RunBoxCommand:  host.RunBoxCommand,
			ResolveAgent: func(h agentruntime.Handle) (sandboxgateway.AgentRef, error) {
				got, err := host.ResolveAgent(h)
				if err != nil {
					return sandboxgateway.AgentRef{}, err
				}
				return sandboxgateway.AgentRef{
					ID:        got.ID,
					Name:      got.Name,
					RuntimeID: got.RuntimeID,
					BoxID:     got.BoxID,
				}, nil
			},
			SyncHandle: host.SyncHandle,
			BuildRuntimeEnv: func(baseURL, accessToken, participantID, agentID, llmBaseURL, modelID string, provider feishu.AgentCredentialProvider) map[string]string {
				env := picoclawBoxEnvVars(baseURL, accessToken, participantID, agentID, llmBaseURL, modelID)
				addFeishuBoxEnvVars(env, agentID, provider)
				return env
			},
			AddProfileEnv: addProfileEnvVars,
			StreamLogs:    host.StreamLogs,
		}))(s)
	}
}

type testStaticFeishuProvider struct {
	apps map[string]feishu.AppConfig
}

func (p testStaticFeishuProvider) BotConfig(botID string) (feishu.AppConfig, bool) {
	app, ok := p.apps[strings.TrimSpace(botID)]
	return app, ok
}

func (p testStaticFeishuProvider) BotConfigForAgent(agentID string) (string, feishu.AppConfig, bool) {
	app, ok := p.apps[strings.TrimSpace(agentID)]
	return strings.TrimSpace(agentID), app, ok
}

func cloneTestFeishuApps(apps map[string]feishu.AppConfig) map[string]feishu.AppConfig {
	cloned := make(map[string]feishu.AppConfig, len(apps))
	for botID, app := range apps {
		cloned[strings.TrimSpace(botID)] = app
	}
	return cloned
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

func TestGatewayProfileRuntimeRestartRequiredOnModelChange(t *testing.T) {
	current := Agent{
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Name:        ManagerName,
		AgentProfile: AgentProfile{
			Name:            ManagerName,
			Provider:        ProviderAPI,
			BaseURL:         "https://api.example/v1",
			APIKey:          "api-key",
			ModelID:         "qwen3.7-max",
			ProfileComplete: true,
		},
	}
	next := normalizeProfile(AgentProfile{
		Name:            ManagerName,
		Provider:        ProviderAPI,
		BaseURL:         "https://api.example/v1",
		APIKey:          "api-key",
		ModelID:         "glm-5.1",
		ProfileComplete: true,
	}, ManagerName, "")
	if !gatewayProfileRuntimeRestartRequired(current, next) {
		t.Fatal("gatewayProfileRuntimeRestartRequired() = false, want true when gateway model changes")
	}
	if profileRestartRequired(current, next) {
		t.Fatal("profileRestartRequired() = true, want false when only gateway model settings change")
	}
}

func TestGatewayProfileRuntimeRestartNotRequiredForCodex(t *testing.T) {
	current := Agent{
		RuntimeKind: RuntimeKindCodex,
		Name:        "alice",
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderCodex,
			ModelID:         "gpt-5.4",
			ProfileComplete: true,
		},
	}
	next := normalizeProfile(AgentProfile{
		Name:            "alice",
		Provider:        ProviderCodex,
		ModelID:         "gpt-5.5",
		ProfileComplete: true,
	}, "alice", "")
	if gatewayProfileRuntimeRestartRequired(current, next) {
		t.Fatal("gatewayProfileRuntimeRestartRequired() = true, want false for codex runtime")
	}
}

func TestUpdateAgentProfileSyncsGatewayHostConfigWithoutRecreate(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	recreateCalled := false
	SetTestHooks(
		nil,
		func(_ *Service, _ context.Context, _ sandbox.Runtime, _, _, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			recreateCalled = true
			return nil, sandbox.Info{}, fmt.Errorf("unexpected recreate")
		},
	)
	defer ResetTestHooks()

	svc, err := NewService(testModelConfig(), config.ServerConfig{
		ListenAddr:  ":18080",
		AccessToken: "token",
	}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:          ManagerUserID,
		Name:        ManagerName,
		Role:        RoleManager,
		RuntimeKind: RuntimeKindPicoClawSandbox,
		BoxID:       "box-manager",
		Status:      string(sandbox.StateRunning),
		AgentProfile: AgentProfile{
			Name:            ManagerName,
			Provider:        ProviderAPI,
			BaseURL:         "https://api.example/v1",
			APIKey:          "api-key",
			ModelID:         "qwen3.7-max",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	if _, err := ensureAgentPicoClawConfigForParticipant(ManagerName, ManagerParticipantID, ManagerUserID, svc.server, config.ModelConfig{
		Provider: config.ProviderLLMAPI,
		BaseURL:  "https://api.example/v1",
		APIKey:   "api-key",
		ModelID:  "qwen3.7-max",
	}); err != nil {
		t.Fatalf("ensureAgentPicoClawConfigForParticipant() error = %v", err)
	}

	_, err = svc.UpdateAgentProfile(ManagerUserID, AgentProfile{
		Name:            ManagerName,
		Provider:        ProviderAPI,
		BaseURL:         "https://api.example/v1",
		APIKey:          "api-key",
		ModelID:         "glm-5.1",
		ProfileComplete: true,
	})
	if err != nil {
		t.Fatalf("UpdateAgentProfile() error = %v", err)
	}
	if recreateCalled {
		t.Fatal("UpdateAgentProfile() recreated gateway box, want host config sync only")
	}

	managerHome, err := agentHomeDir(ManagerUserID)
	if err != nil {
		t.Fatalf("agentHomeDir() error = %v", err)
	}
	configPath := filepath.Join(managerHome, picoclawsandbox.HostDir, picoclawsandbox.HostConfig)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(manager config) error = %v", err)
	}
	if !strings.Contains(string(data), `"model_name": "glm-5.1"`) {
		t.Fatalf("manager config missing updated model:\n%s", data)
	}
	got, ok := svc.Agent(ManagerUserID)
	if !ok {
		t.Fatal("Agent() ok = false, want true")
	}
	if got.AgentProfile.EnvRestartRequired {
		t.Fatal("Agent().AgentProfile.EnvRestartRequired = true, want false after config sync")
	}
}
