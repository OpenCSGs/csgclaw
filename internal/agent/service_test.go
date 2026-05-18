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

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	"csgclaw/internal/hub"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/notifier"
	"csgclaw/internal/runtime/openclawsandbox"
	"csgclaw/internal/runtime/picoclawsandbox"
	"csgclaw/internal/runtime/sandboxgateway"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/boxlitecli"
	"csgclaw/internal/sandbox/sandboxtest"
	"csgclaw/internal/templates"
)

func init() {
	testDefaultServiceOption = withTestPicoClawSandboxRuntime()
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
	provision  func(context.Context, agentruntime.ProvisionRequest) error
	new        func(context.Context, agentruntime.Spec) (agentruntime.Handle, error)
	start      func(context.Context, agentruntime.Handle) (agentruntime.State, error)
	stop       func(context.Context, agentruntime.Handle) (agentruntime.State, error)
	del        func(context.Context, agentruntime.Handle) error
	state      func(context.Context, agentruntime.Handle) (agentruntime.State, error)
	info       func(context.Context, agentruntime.Handle) (agentruntime.Info, error)
	streamLogs func(context.Context, agentruntime.Handle, agentruntime.LogOptions) error
}

func (f fakeAgentRuntime) Kind() string {
	return f.kind
}

func (f fakeAgentRuntime) Provision(ctx context.Context, req agentruntime.ProvisionRequest) error {
	if f.provision != nil {
		return f.provision(ctx, req)
	}
	return nil
}

func (f fakeAgentRuntime) New(ctx context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
	if f.new != nil {
		return f.new(ctx, spec)
	}
	return agentruntime.Handle{}, nil
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
		},
		"",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				if spec.AgentID != "u-alice" {
					t.Fatalf("Create() agent id = %q, want %q", spec.AgentID, "u-alice")
				}
				if spec.AgentName != "alice" {
					t.Fatalf("Create() agent name = %q, want %q", spec.AgentName, "alice")
				}
				if got, want := spec.Profile.BaseURL, "http://127.0.0.1:18080/api/bots/u-alice/llm"; got != want {
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

func TestCreateWorkerTriggersLifecycleObserver(t *testing.T) {
	observer := &fakeLifecycleObserver{}
	svc, err := NewService(
		config.ModelConfig{},
		config.ServerConfig{},
		"",
		"",
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
		},
		"",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				callOrder = append(callOrder, "provision")
				if req.RuntimeID != "rt-u-alice" {
					t.Fatalf("Provision() runtime id = %q, want %q", req.RuntimeID, "rt-u-alice")
				}
				if req.AgentID != "u-alice" {
					t.Fatalf("Provision() agent id = %q, want %q", req.AgentID, "u-alice")
				}
				if req.AgentName != "alice" {
					t.Fatalf("Provision() agent name = %q, want %q", req.AgentName, "alice")
				}
				if got, want := req.Profile.BaseURL, "http://127.0.0.1:18080/api/bots/u-alice/llm"; got != want {
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

func TestCreateWorkerPassesWorkspaceOverlayToProvision(t *testing.T) {
	overlayRoot := t.TempDir()
	var gotOverlay string
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"",
		"",
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

func TestRecreateTriggersLifecycleObserver(t *testing.T) {
	observer := &fakeLifecycleObserver{}
	svc, err := NewService(
		config.ModelConfig{},
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		},
		"",
		"",
		WithLifecycleObserver(observer),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			del:  func(context.Context, agentruntime.Handle) error { return nil },
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				if got, want := spec.Profile.BaseURL, "http://127.0.0.1:18080/api/bots/u-alice/llm"; got != want {
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

	got, err := svc.Recreate(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("Recreate() error = %v", err)
	}
	if got.BoxID != "codex-session-alice-new" {
		t.Fatalf("Recreate().BoxID = %q, want %q", got.BoxID, "codex-session-alice-new")
	}
	if len(observer.ensureCalls) != 1 || observer.ensureCalls[0].ID != "u-alice" {
		t.Fatalf("EnsureAgent() calls = %+v, want one call for u-alice", observer.ensureCalls)
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
		},
		"",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			del: func(context.Context, agentruntime.Handle) error {
				callOrder = append(callOrder, "delete")
				return nil
			},
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				callOrder = append(callOrder, "provision")
				if req.RuntimeID != "rt-u-alice" {
					t.Fatalf("Provision() runtime id = %q, want %q", req.RuntimeID, "rt-u-alice")
				}
				if req.AgentID != "u-alice" {
					t.Fatalf("Provision() agent id = %q, want %q", req.AgentID, "u-alice")
				}
				if req.AgentName != "alice" {
					t.Fatalf("Provision() agent name = %q, want %q", req.AgentName, "alice")
				}
				if got, want := req.Profile.BaseURL, "http://127.0.0.1:18080/api/bots/u-alice/llm"; got != want {
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
		config.ServerConfig{},
		"",
		"",
		WithLifecycleObserver(observer),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			del:  func(context.Context, agentruntime.Handle) error { return nil },
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
	svc.syncRuntimeRecordLocked(svc.agents["u-alice"])

	if err := svc.Delete(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(observer.stopCalls) != 1 || observer.stopCalls[0] != "u-alice" {
		t.Fatalf("StopAgent() calls = %v, want [u-alice]", observer.stopCalls)
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
		config.ServerConfig{},
		"",
		"",
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

func TestCreateWorkerNotifierPersistsWebhookToken(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	const wantToken = "notifier-webhook-secret-xyz"
	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		Name:        "notify-worker",
		RuntimeKind: RuntimeKindNotifier,
		RuntimeOptions: map[string]any{
			"delivery_mode": "webhook",
			"webhook_token": wantToken,
		},
		AgentProfile: AgentProfile{},
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.RuntimeKind != RuntimeKindNotifier {
		t.Fatalf("CreateWorker().RuntimeKind = %q, want %q", got.RuntimeKind, RuntimeKindNotifier)
	}
	cfg := notifier.ConfigFromAgentRuntimeOptions(got.RuntimeOptions)
	if cfg.WebhookToken != wantToken {
		t.Fatalf("in-memory webhook_token = %q, want %q", cfg.WebhookToken, wantToken)
	}

	reloaded, err := NewService(testModelConfig(), config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService(reload) error = %v", err)
	}
	got2, ok := reloaded.Agent(got.ID)
	if !ok {
		t.Fatalf("Agent(%q) after reload: ok = false", got.ID)
	}
	cfg2 := notifier.ConfigFromAgentRuntimeOptions(got2.RuntimeOptions)
	if cfg2.WebhookToken != wantToken {
		t.Fatalf("after reload webhook_token = %q, want %q", cfg2.WebhookToken, wantToken)
	}
}

func TestStopNotifierPersistsStoppedAndHydrateKeepsStopped(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		Name:           "notifier-stop-test",
		RuntimeKind:    RuntimeKindNotifier,
		RuntimeOptions: map[string]any{"delivery_mode": "webhook", "webhook_token": "tok"},
		AgentProfile:   AgentProfile{},
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	stopped, err := svc.Stop(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if stopped.Status != string(sandbox.StateStopped) {
		t.Fatalf("Stop().Status = %q, want stopped", stopped.Status)
	}
	agentNow, ok := svc.Agent(got.ID)
	if !ok {
		t.Fatal("Agent() missing after stop")
	}
	if agentNow.Status != string(sandbox.StateStopped) {
		t.Fatalf("Agent().Status after stop = %q, want stopped (notifier Runtime.Info reports running)", agentNow.Status)
	}
	reloaded, err := NewService(testModelConfig(), config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService(reload) error = %v", err)
	}
	reloadedAgent, ok := reloaded.Agent(got.ID)
	if !ok {
		t.Fatal("Agent() missing after reload")
	}
	if reloadedAgent.Status != string(sandbox.StateStopped) {
		t.Fatalf("reloaded Agent().Status = %q, want stopped", reloadedAgent.Status)
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
	if worker.RuntimeKind != RuntimeKindPicoClawSandbox {
		t.Fatalf("CreateWorker().RuntimeKind = %q, want %q", worker.RuntimeKind, RuntimeKindPicoClawSandbox)
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
	if !hasBoxliteCLICommandArgs(runner.requests, "run", "/bin/sh", "-c", picoclawsandbox.GatewayRunCommand()) {
		t.Fatalf("boxlite-cli gateway run command not found in requests: %#v", requestArgs(runner.requests))
	}
	if hasBoxliteCLIExec(runner.requests, "tail", "-n", "1", "-f", picoclawsandbox.BoxGatewayLogPath) {
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

func TestCreateReplaceManagerSwitchesRuntimeKind(t *testing.T) {
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
	if err != nil {
		t.Fatalf("Create() replace error = %v", err)
	}
	if got, want := replaced.RuntimeKind, RuntimeKindOpenClawSandbox; got != want {
		t.Fatalf("Create() runtime_kind = %q, want %q", got, want)
	}
	if got, want := replaced.Image, config.DefaultOpenClawManagerImage; got != want {
		t.Fatalf("Create() image = %q, want %q", got, want)
	}
	if got, want := svc.GatewayRuntime(), RuntimeKindOpenClawSandbox; got != want {
		t.Fatalf("GatewayRuntime() = %q, want %q", got, want)
	}
	if len(gotImages) != 2 {
		t.Fatalf("createGatewayBox() calls = %d, want 2", len(gotImages))
	}
	if got, want := gotImages[1], config.DefaultOpenClawManagerImage; got != want {
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
	t.Setenv("HOME", dir)
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

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, ok := svc.agentSnapshot("u-alice")
	if !ok {
		t.Fatal("agentSnapshot() ok = false, want true")
	}
	if got.RuntimeID != "rt-u-alice" {
		t.Fatalf("agentSnapshot().RuntimeID = %q, want %q", got.RuntimeID, "rt-u-alice")
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
	if persisted.Agents[0].RuntimeID != "rt-u-alice" {
		t.Fatalf("saved agent runtime_id = %q, want %q", persisted.Agents[0].RuntimeID, "rt-u-alice")
	}
	if persisted.Agents[0].RuntimeKind != RuntimeKindPicoClawSandbox {
		t.Fatalf("saved agent runtime_kind = %q, want %q", persisted.Agents[0].RuntimeKind, RuntimeKindPicoClawSandbox)
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

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
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

	_, err = NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
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

	_, err = NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
	if err == nil || !strings.Contains(err.Error(), "runtime kind is required") {
		t.Fatalf("NewService() error = %v, want runtime kind validation error", err)
	}
}

func TestLoadManagerRequiresCanonicalIdentity(t *testing.T) {
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

	_, err = NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
	if err == nil || !strings.Contains(err.Error(), "manager id must be") {
		t.Fatalf("NewService() error = %v, want canonical manager validation error", err)
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
	}, config.ServerConfig{}, "", "")
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

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
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

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
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

func TestStreamLogsFollowUsesHostGatewayLogWithoutSandboxRuntime(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
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
	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
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

func TestStartTriggersLifecycleObserver(t *testing.T) {
	observer := &fakeLifecycleObserver{}
	svc, err := NewService(
		config.ModelConfig{},
		config.ServerConfig{},
		"",
		"",
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
	svc.agents["u-alice"] = Agent{
		ID:              "u-alice",
		Name:            "alice",
		Role:            RoleWorker,
		RuntimeID:       "rt-u-alice",
		RuntimeKind:     RuntimeKindCodex,
		BoxID:           "box-alice",
		Status:          string(agentruntime.StateStopped),
		AgentProfile:    AgentProfile{Name: "alice", Provider: ProviderCodex, ModelID: "gpt-5.4", ProfileComplete: true},
		ProfileComplete: true,
	}

	if _, err := svc.Start(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if len(observer.ensureCalls) != 1 || observer.ensureCalls[0].ID != "u-alice" {
		t.Fatalf("EnsureAgent() calls = %+v, want one call for u-alice", observer.ensureCalls)
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
		},
		"",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				callOrder = append(callOrder, "provision")
				if req.RuntimeID != "rt-u-alice" || req.AgentID != "u-alice" || req.AgentName != "alice" {
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
	svc.agents["u-alice"] = Agent{
		ID:              "u-alice",
		Name:            "alice",
		Role:            RoleWorker,
		RuntimeID:       "rt-u-alice",
		RuntimeKind:     RuntimeKindCodex,
		BoxID:           "box-alice",
		Status:          string(agentruntime.StateStopped),
		AgentProfile:    AgentProfile{Name: "alice", Provider: ProviderAPI, BaseURL: "https://api.example/v1", APIKey: "api-key", ModelID: "gpt-4.1", ProfileComplete: true},
		ProfileComplete: true,
	}

	if _, err := svc.Start(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := strings.Join(callOrder, ","), "provision,start"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
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
	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", statePath)
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

	svc, err := NewService(testModelConfig(), config.ServerConfig{ListenAddr: ":18080", AccessToken: "token"}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:              "u-alice",
		Name:            "alice",
		Role:            RoleWorker,
		RuntimeKind:     RuntimeKindPicoClawSandbox,
		BoxID:           "box-alice",
		Status:          string(sandbox.StateRunning),
		AgentProfile:    AgentProfile{Name: "alice", Provider: ProviderCodex, ModelID: "gpt-5.5", ProfileComplete: true},
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}

	if _, err := svc.Start(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	configPath := filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, "alice", hostWorkspaceDir, filepath.FromSlash(picoclawsandbox.HostPicoClawStateDir), picoclawsandbox.HostPicoClawConfig)
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

func TestCreateWorkerFromTemplateAppliesDefaultsAndOverlaysWorkspace(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubService(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
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
			FromTemplate: "local/frontend-worker",
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

	workspaceRoot, err := agentWorkspaceRoot("alice")
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

func TestCreateOpenClawWorkerFromTemplateOverlaysOpenClawWorkspace(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubService(t, "openclaw-manager", hub.Template{
		ID:          "openclaw-manager",
		Name:        "openclaw-manager",
		Description: "openclaw manager",
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
		FromTemplate: "local/openclaw-manager",
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.RuntimeKind != RuntimeKindOpenClawSandbox {
		t.Fatalf("RuntimeKind = %q, want %q", got.RuntimeKind, RuntimeKindOpenClawSandbox)
	}

	agentHome := filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, "alice")
	openclawWorkspace := openclawsandbox.WorkspaceRoot(agentHome)
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
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local/frontend-worker"}),
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

	workspaceRoot, err := agentWorkspaceRoot("alice")
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

func TestCreateWorkerRejectsMissingDefaultTemplate(t *testing.T) {
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubService(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local/missing"}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateWorker(context.Background(), CreateAgentSpec{Name: "alice"})
	if err == nil {
		t.Fatal("CreateWorker() error = nil, want missing default template")
	}
	if !strings.Contains(err.Error(), `resolve default worker template "local/missing"`) {
		t.Fatalf("CreateWorker() error = %v, want default worker template context", err)
	}
}

func TestCreateWorkerRejectsDefaultTemplateRoleMismatch(t *testing.T) {
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubService(t, "review-manager", hub.Template{
		ID:          "review-manager",
		Name:        "review-manager",
		Description: "manager template",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "manager-image:1",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local/review-manager"}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateWorker(context.Background(), CreateAgentSpec{Name: "alice"})
	if err == nil {
		t.Fatal("CreateWorker() error = nil, want role mismatch")
	}
	if !strings.Contains(err.Error(), `default worker template "local/review-manager" points to a manager template`) {
		t.Fatalf("CreateWorker() error = %v, want worker/manager mismatch", err)
	}
}

func TestCreateWorkerSkipsDefaultTemplateRuntimeMismatch(t *testing.T) {
	hubSvc := mustNewLocalTemplateHubService(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local/frontend-worker"}),
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
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local/frontend-worker"}),
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

	workspaceRoot, err := agentWorkspaceRoot("alice")
	if err != nil {
		t.Fatalf("agentWorkspaceRoot() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "skills", "custom", "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(skills/custom/SKILL.md) error = %v, want not exist", err)
	}
}

func TestCreateWorkerNotifierSkipsDefaultSandboxTemplate(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubService(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		statePath,
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultWorkerTemplate: "local/frontend-worker"}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		Name:           "notifier-hub-skip",
		RuntimeKind:    RuntimeKindNotifier,
		RuntimeOptions: map[string]any{"delivery_mode": "webhook", "webhook_token": "tok"},
		AgentProfile:   AgentProfile{},
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.RuntimeKind != RuntimeKindNotifier {
		t.Fatalf("RuntimeKind = %q, want %q", got.RuntimeKind, RuntimeKindNotifier)
	}
	if strings.TrimSpace(got.Image) != "" {
		t.Fatalf("notifier Image = %q, want empty (no default sandbox template)", got.Image)
	}
}

func TestCreateRejectsDefaultManagerTemplateRoleMismatch(t *testing.T) {
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubService(t, "review-worker", hub.Template{
		ID:          "review-worker",
		Name:        "review-worker",
		Description: "worker template",
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "worker-image:1",
	})

	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultManagerTemplate: "local/review-worker"}),
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
	if !strings.Contains(err.Error(), `default manager template "local/review-worker" points to a worker template`) {
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

	workspaceRoot, err := agentWorkspaceRoot(created.Name)
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

func TestStartConfiguredAgentsStartsStoppedCompleteWorkersAndLeavesRunningWorkersUntouched(t *testing.T) {
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

func TestStopTriggersLifecycleObserver(t *testing.T) {
	observer := &fakeLifecycleObserver{}
	svc, err := NewService(
		config.ModelConfig{},
		config.ServerConfig{},
		"",
		"",
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
		config.ServerConfig{},
		"",
		"",
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

	if err := EnsureBootstrapState(context.Background(), statePath, config.ServerConfig{}, testModelConfig(), "", true); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}
	if removed != "box-old" {
		t.Fatalf("ForceRemove() target = %q, want %q", removed, "box-old")
	}

	reloaded, err := NewService(testModelConfig(), config.ServerConfig{}, "", statePath)
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

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	runtimeHome, err := svc.sandboxRuntimeHome(ManagerName)
	if err != nil {
		t.Fatalf("svc.sandboxRuntimeHome() error = %v", err)
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

	if err := EnsureBootstrapState(context.Background(), statePath, config.ServerConfig{}, testModelConfig(), "", true); err != nil {
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
	if err := EnsureBootstrapState(context.Background(), statePath, config.ServerConfig{}, testModelConfig(), "", false); err != nil {
		t.Fatalf("EnsureBootstrapState() error = %v", err)
	}
	if closeCalls != 1 {
		t.Fatalf("closeBox() calls = %d, want %d", closeCalls, 1)
	}
	if closeRuntimeCalls != 1 {
		t.Fatalf("closeRuntime() calls = %d, want %d", closeRuntimeCalls, 1)
	}

	reloaded, err := NewService(testModelConfig(), config.ServerConfig{}, "", statePath)
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

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.sandboxRuntimeHome("alice")
	if err != nil {
		t.Fatalf("svc.sandboxRuntimeHome() error = %v", err)
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
	testGetBoxHook = func(_ *Service, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	defer func() {
		testGetBoxHook = nil
	}()

	provider := fakeProvider{
		open: func(_ context.Context, homeDir string) (sandbox.Runtime, error) {
			gotHome = homeDir
			return &fakeRuntime{}, nil
		},
	}

	svc, err := NewService(config.ModelConfig{}, config.ServerConfig{}, "", "", WithSandboxProvider(provider))
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

	apps := map[string]feishu.AppConfig{
		"u-worker-1": {
			AppID:     "cli_worker",
			AppSecret: "worker-secret",
		},
	}
	svc, err := NewService(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"",
		"",
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
	if spec.Name != "alice" {
		t.Fatalf("gatewayCreateSpec() name = %q, want %q", spec.Name, "alice")
	}
	if !spec.Detach {
		t.Fatal("gatewayCreateSpec() detach = false, want true")
	}
	if spec.AutoRemove {
		t.Fatal("gatewayCreateSpec() auto_remove = true, want false")
	}
	wantCmd := "/bin/sh -c " + picoclawsandbox.GatewayRunCommand()
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
	wantConfigRoot := filepath.Join(wantWorkspaceRoot, filepath.FromSlash(picoclawsandbox.HostPicoClawStateDir))
	wantProjectsRoot := filepath.Join(homeDir, config.AppDirName, hostProjectsDir)
	if len(spec.Mounts) != 2 {
		t.Fatalf("gatewayCreateSpec() mounts = %+v, want 2 mounts", spec.Mounts)
	}
	if spec.Mounts[0].HostPath != wantWorkspaceRoot || spec.Mounts[0].GuestPath != picoclawsandbox.BoxWorkspaceDir {
		t.Fatalf("workspace mount = %+v, want host %q guest %q", spec.Mounts[0], wantWorkspaceRoot, picoclawsandbox.BoxWorkspaceDir)
	}
	if spec.Mounts[1].HostPath != wantProjectsRoot || spec.Mounts[1].GuestPath != picoclawsandbox.BoxProjectsDir {
		t.Fatalf("projects mount = %+v, want host %q guest %q", spec.Mounts[1], wantProjectsRoot, picoclawsandbox.BoxProjectsDir)
	}
	if _, err := os.Stat(filepath.Join(wantConfigRoot, picoclawsandbox.HostPicoClawConfig)); err != nil {
		t.Fatalf("worker PicoClaw config was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantWorkspaceRoot, "AGENT.md")); err != nil {
		t.Fatalf("worker workspace was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantAgentHome, picoclawsandbox.HostPicoClawDir)); !os.IsNotExist(err) {
		t.Fatalf("picoclaw host dir stat error = %v, want not exist", err)
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
	wantAgentHome := filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, "alice")
	wantOpenClawRoot := openclawsandbox.Root(wantAgentHome)
	if gateway.AgentHome != wantAgentHome {
		t.Fatalf("Gateway.AgentHome = %q, want %q", gateway.AgentHome, wantAgentHome)
	}
	if gateway.WorkspaceTemplate != templates.OpenClawWorkerRoot {
		t.Fatalf("Gateway.WorkspaceTemplate(worker) = %q, want %q", gateway.WorkspaceTemplate, templates.OpenClawWorkerRoot)
	}
	managerGateway, err := svc.gatewayProvisionRequest(RuntimeKindOpenClawSandbox, ManagerName, ManagerUserID)
	if err != nil {
		t.Fatalf("gatewayProvisionRequest(manager) error = %v", err)
	}
	if managerGateway.WorkspaceTemplate != templates.OpenClawManagerRoot {
		t.Fatalf("Gateway.WorkspaceTemplate(manager) = %q, want %q", managerGateway.WorkspaceTemplate, templates.OpenClawManagerRoot)
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
	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", "")
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

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", statePath)
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

	svc, err := NewService(testModelConfig(), config.ServerConfig{}, "", statePath)
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

func picoclawBoxEnvVars(baseURL, accessToken, botID, llmBaseURL, modelID string) map[string]string {
	env := bridgeLLMEnvVars(llmBaseURL, accessToken, modelID)
	picoclawModelID := picoclawBridgeModelID(modelID)
	env["CSGCLAW_BASE_URL"] = baseURL
	env["CSGCLAW_ACCESS_TOKEN"] = accessToken
	env["PICOCLAW_CHANNELS_CSGCLAW_BASE_URL"] = baseURL
	env["PICOCLAW_CHANNELS_CSGCLAW_ACCESS_TOKEN"] = accessToken
	env["PICOCLAW_CHANNELS_CSGCLAW_BOT_ID"] = botID
	env["PICOCLAW_AGENTS_DEFAULTS_MODEL_NAME"] = modelID
	env["PICOCLAW_CUSTOM_MODEL_NAME"] = modelID
	env["PICOCLAW_CUSTOM_MODEL_ID"] = picoclawModelID
	env["PICOCLAW_CUSTOM_MODEL_API_KEY"] = accessToken
	env["PICOCLAW_CUSTOM_MODEL_BASE_URL"] = llmBaseURL
	return env
}

func addFeishuBoxEnvVars(envVars map[string]string, botID string, provider feishu.BotCredentialProvider) {
	if envVars == nil {
		return
	}
	botID = strings.TrimSpace(botID)
	if botID == "" || provider == nil {
		return
	}
	app, ok := provider.BotConfig(botID)
	if !ok {
		return
	}
	envVars["PICOCLAW_CHANNELS_FEISHU_APP_ID"] = app.AppID
	envVars["PICOCLAW_CHANNELS_FEISHU_APP_SECRET"] = app.AppSecret
}

func withTestPicoClawSandboxRuntime(apps ...map[string]feishu.AppConfig) ServiceOption {
	return func(s *Service) error {
		var provider feishu.BotCredentialProvider
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
		return WithRuntime(notifier.NewAgentRuntime())(s)
	}
}

func withTestSandboxRuntimeHost(host PicoClawRuntimeHost, provider feishu.BotCredentialProvider, newRuntime func(sandboxgateway.Dependencies) agentruntime.Runtime) ServiceOption {
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
			BuildRuntimeEnv: func(baseURL, accessToken, botID, llmBaseURL, modelID string, provider feishu.BotCredentialProvider) map[string]string {
				env := picoclawBoxEnvVars(baseURL, accessToken, botID, llmBaseURL, modelID)
				addFeishuBoxEnvVars(env, botID, provider)
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
