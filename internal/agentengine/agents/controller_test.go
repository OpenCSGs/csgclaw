package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/assets"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	"csgclaw/internal/dshcli"
	"csgclaw/internal/mcp"
	"csgclaw/internal/mcpschema"
	agentruntime "csgclaw/internal/runtime"
	runtimecodex "csgclaw/internal/runtime/codex"
	runtimedsh "csgclaw/internal/runtime/dsh"
	runtimeinstructions "csgclaw/internal/runtime/instructions"
	"csgclaw/internal/runtime/sandboxgateway"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/boxlitecli"
	"csgclaw/internal/sandbox/sandboxtest"
	hub "csgclaw/internal/template"
	templateembed "csgclaw/internal/template/embed"
	"csgclaw/internal/utils"
)

func init() {
	locateCodexCLI = func() (string, error) { return "/usr/local/bin/codex", nil }
	testDefaultServiceOption = func(s *Controller) error {

		return WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-" + spec.AgentName}, nil
			},
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
			},
		})(s)
	}
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

type fakeRuntime struct {
	close func() error
}

type fakeProvider struct {
	name              string
	open              func(context.Context, string) (sandbox.Runtime, error)
	checkAvailability func(context.Context) error
}

func (f fakeProvider) Name() string {
	if f.name != "" {
		return f.name
	}
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

func (f fakeProvider) CheckAvailability(ctx context.Context) error {
	if f.checkAvailability != nil {
		return f.checkAvailability(ctx)
	}
	return nil
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
	if f != nil && f.close != nil {
		return f.close()
	}
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
	kind         string
	workspace    func(string) string
	skills       func(string) string
	hostLogs     func(string) []string
	validate     func(context.Context, agentruntime.RuntimeConfigSnapshot) error
	restart      func(agentruntime.RuntimeConfigChange) (bool, error)
	reconcile    func(context.Context, agentruntime.Handle, agentruntime.RuntimeConfigChange) error
	mcpValidate  func(context.Context, agentruntime.MCPServersSnapshot) error
	mcpRestart   func(agentruntime.MCPServersChange) (bool, error)
	mcpReconcile func(context.Context, agentruntime.Handle, agentruntime.MCPServersChange) error
	provision    func(context.Context, agentruntime.ProvisionRequest) error
	new          func(context.Context, agentruntime.Spec) (agentruntime.Handle, error)
	start        func(context.Context, agentruntime.Handle) (agentruntime.State, error)
	stop         func(context.Context, agentruntime.Handle) (agentruntime.State, error)
	del          func(context.Context, agentruntime.Handle) error
	state        func(context.Context, agentruntime.Handle) (agentruntime.State, error)
	info         func(context.Context, agentruntime.Handle) (agentruntime.Info, error)
	streamLogs   func(context.Context, agentruntime.Handle, agentruntime.LogOptions) error
}

type recreateCodexBinaryProvider struct{}

func (recreateCodexBinaryProvider) Ensure(context.Context) (string, error) {
	return "/tmp/codex", nil
}

type recreateCodexSessionManager struct {
	mu       sync.Mutex
	sessions map[string]*runtimecodex.Session
}

func (m *recreateCodexSessionManager) Start(_ context.Context, spec runtimecodex.SessionSpec) (*runtimecodex.Session, error) {
	now := time.Now().UTC()
	session := &runtimecodex.Session{
		RuntimeID:    spec.RuntimeID,
		AgentID:      spec.AgentID,
		AgentName:    spec.AgentName,
		SessionID:    "session-" + spec.AgentID,
		BinaryPath:   spec.BinaryPath,
		WorkspaceDir: spec.WorkspaceDir,
		HomeDir:      spec.HomeDir,
		CodexHomeDir: spec.CodexHomeDir,
		StderrPath:   spec.StderrPath,
		ProcessID:    os.Getpid(),
		CreatedAt:    now,
		StartedAt:    now,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions == nil {
		m.sessions = make(map[string]*runtimecodex.Session)
	}
	m.sessions[spec.RuntimeID] = session
	return session, nil
}

func (m *recreateCodexSessionManager) Stop(_ context.Context, handle runtimecodex.SessionHandle) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, handle.RuntimeID)
	return nil
}

func (m *recreateCodexSessionManager) LiveSession(handle runtimecodex.SessionHandle) (*runtimecodex.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[handle.RuntimeID]
	if !ok {
		return nil, os.ErrNotExist
	}
	return session, nil
}

func (m *recreateCodexSessionManager) Session(handle runtimecodex.SessionHandle) (*runtimecodex.Session, error) {
	return m.LiveSession(handle)
}

func (*recreateCodexSessionManager) Prompt(context.Context, runtimecodex.SessionHandle, runtimecodex.PromptRequest) (runtimecodex.PromptResponse, error) {
	return runtimecodex.PromptResponse{}, os.ErrNotExist
}

func newRecreateTestCodexRuntime(controller func() *Controller, agentID string) *runtimecodex.Runtime {
	return runtimecodex.New(runtimecodex.Dependencies{
		BinaryProvider: recreateCodexBinaryProvider{},
		AgentHome: func(agentID string) (string, error) {
			svc := controller()
			if svc == nil {
				return "", errors.New("controller is unavailable")
			}
			return svc.agentHomeDir(agentID)
		},
		ResolveAgent: func(h agentruntime.Handle) (runtimecodex.AgentRef, error) {
			svc := controller()
			if svc == nil {
				return runtimecodex.AgentRef{}, errors.New("controller is unavailable")
			}
			got, ok := svc.agentSnapshot(agentID)
			if !ok {
				return runtimecodex.AgentRef{}, fmt.Errorf("agent %q not found", agentID)
			}
			return runtimecodex.AgentRef{
				ID:             got.ID,
				Name:           got.Name,
				RuntimeID:      h.RuntimeID,
				HandleID:       got.BoxID,
				Instructions:   got.Instructions,
				RuntimeOptions: utils.CloneAnyMap(got.RuntimeOptions),
				MCPServers:     cloneMCPServers(got.MCPServers),
				Profile:        svc.runtimeProfileForAgent(got),
			}, nil
		},
		Manager: &recreateCodexSessionManager{},
	})
}

type fakeClosableAgentRuntime struct {
	fakeAgentRuntime
	close func() error
}

type fakeReadinessAgentRuntime struct {
	fakeAgentRuntime
	readiness func(context.Context, agentruntime.Handle) error
}

type fakeStartConfigAgentRuntime struct {
	fakeAgentRuntime
	startValidate func(context.Context, agentruntime.RuntimeConfigSnapshot) error
}

func (f fakeStartConfigAgentRuntime) ValidateStartConfig(ctx context.Context, current agentruntime.RuntimeConfigSnapshot) error {
	if f.startValidate != nil {
		return f.startValidate(ctx, current)
	}
	return nil
}

func (f fakeReadinessAgentRuntime) CheckReadiness(ctx context.Context, h agentruntime.Handle) error {
	if f.readiness != nil {
		return f.readiness(ctx, h)
	}
	return agentruntime.ErrReadinessNotSupported
}

func (f *fakeClosableAgentRuntime) Close() error {
	if f.close != nil {
		return f.close()
	}
	return nil
}

type fakeMCPServersListRuntime struct {
	fakeAgentRuntime
	list func(context.Context, agentruntime.Handle, agentruntime.MCPServersSnapshot) (agentruntime.MCPServersSnapshot, error)
}

func (f fakeMCPServersListRuntime) ListMCPServers(ctx context.Context, h agentruntime.Handle, current agentruntime.MCPServersSnapshot) (agentruntime.MCPServersSnapshot, error) {
	if f.list == nil {
		return agentruntime.MCPServersSnapshot{}, nil
	}
	return f.list(ctx, h, current)
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
	case RuntimeKindCodex:
		return agentruntime.Layout{
			WorkspaceRoot:    filepath.Join(agentHome, ".codex", "workspace"),
			InstructionsPath: filepath.Join(agentHome, ".codex", "home", "AGENTS.md"),
			SkillsRoot:       filepath.Join(agentHome, ".codex", "home", "skills"),
			HostLogPaths:     []string{filepath.Join(agentHome, ".codex", "home", "stderr.log")},
		}
	case RuntimeKindDSH:
		return agentruntime.Layout{
			WorkspaceRoot:    filepath.Join(agentHome, ".dsh", "workspace"),
			InstructionsPath: filepath.Join(agentHome, ".dsh", "home", "AGENTS.md"),
			SkillsRoot:       filepath.Join(agentHome, ".dsh", "home", "skills"),
			HostLogPaths:     []string{filepath.Join(agentHome, ".dsh", "stderr.log")},
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
	if f.kind == RuntimeKindCodex && req.AgentID != "" {
		return runtimecodex.New(runtimecodex.Dependencies{AgentHome: agentHomeDir}).Provision(ctx, req)
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

func (f fakeAgentRuntime) ValidateMCPServers(ctx context.Context, current agentruntime.MCPServersSnapshot) error {
	if f.mcpValidate != nil {
		return f.mcpValidate(ctx, current)
	}
	return mcpschema.ValidateMCPServers(current.Servers)
}

func (f fakeAgentRuntime) MCPServersRestartRequired(change agentruntime.MCPServersChange) (bool, error) {
	if f.mcpRestart != nil {
		return f.mcpRestart(change)
	}
	return agentruntime.MCPServersNeedsRestart(change.Previous.Servers, change.Current.Servers)
}

func (f fakeAgentRuntime) ReconcileMCPServers(ctx context.Context, h agentruntime.Handle, change agentruntime.MCPServersChange) error {
	if f.mcpReconcile != nil {
		return f.mcpReconcile(ctx, h, change)
	}
	return nil
}

func assertMCPServersHasServer(t *testing.T, cfg map[string]any, name string) {
	t.Helper()
	if _, ok := cfg[name]; !ok {
		t.Fatalf("MCPServers = %#v, want %q", cfg, name)
	}
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
	events      []string
	ensureErr   error
}

func (f *fakeLifecycleObserver) EnsureAgent(_ context.Context, a Agent) error {
	f.ensureCalls = append(f.ensureCalls, a)
	f.events = append(f.events, "ensure:"+a.ID)
	return f.ensureErr
}

func (f *fakeLifecycleObserver) StopAgent(agentID string) {
	f.stopCalls = append(f.stopCalls, agentID)
	f.events = append(f.events, "stop:"+agentID)
}

type fakeBindingActivator struct {
	refreshCalls        []bindingRefreshCall
	refreshErr          error
	refreshWhileStopped map[string]bool
}

type bindingRefreshCall struct {
	agent   Agent
	channel string
}

func (f *fakeBindingActivator) RefreshAgentChannel(_ context.Context, a Agent, channel string) error {
	f.refreshCalls = append(f.refreshCalls, bindingRefreshCall{agent: a, channel: channel})
	return f.refreshErr
}

func (f *fakeBindingActivator) CanRefreshStoppedAgentBinding(channel string) bool {
	return f.refreshWhileStopped[strings.ToLower(strings.TrimSpace(channel))]
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
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
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
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
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
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
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

	_, err = svc.CreateRecord(context.Background(), CreateRequest{
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
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return nil, nil },
		nil,
	)
	defer ResetTestHooks()

	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
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

func TestCreateWorkerRejectsUnavailableCodexRuntime(t *testing.T) {
	origLocateCodexCLI := locateCodexCLI
	locateCodexCLI = func() (string, error) { return "", fmt.Errorf("codex missing") }
	t.Cleanup(func() {
		locateCodexCLI = origLocateCodexCLI
	})
	SetTestHooks(
		func(_ *Controller, _ string) (sandbox.Runtime, error) {
			t.Fatal("ensureRuntime() should not be used when Codex provider is unavailable")
			return nil, nil
		},
		func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string, _ string, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			t.Fatal("createGatewayBox() should not be used when Codex provider is unavailable")
			return nil, sandbox.Info{}, nil
		},
	)
	defer ResetTestHooks()

	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateWorker(context.Background(), CreateAgentSpec{
		Name:        "alice",
		RuntimeKind: RuntimeKindCodex,
		Image:       "worker-image:1",
		AgentProfile: AgentProfile{
			Name:            "alice",
			ModelProviderID: ModelProviderIDCodex,
			ModelID:         "gpt-5.5",
			ProfileComplete: true,
		},
	})
	if err == nil {
		t.Fatal("CreateWorker() error = nil, want Codex CLI missing error")
	}
	if !strings.Contains(err.Error(), "codex missing") {
		t.Fatalf("CreateWorker() error = %q, want Codex provider availability error", err)
	}
}

func TestCreateWorkerUsesCodexRuntimeWhenRequested(t *testing.T) {
	SetTestHooks(
		func(_ *Controller, _ string) (sandbox.Runtime, error) {
			t.Fatal("ensureRuntime() should not be used for codex worker creation")
			return nil, nil
		},
		func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string, _ string, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			t.Fatal("createGatewayBox() should not be used for codex worker creation")
			return nil, sandbox.Info{}, nil
		},
	)
	defer ResetTestHooks()

	svc, err := NewController(
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
				if got := spec.Profile.APIKey; !strings.HasPrefix(got, "agent.") || got != spec.Profile.Env["CSGCLAW_ACCESS_TOKEN"] {
					t.Fatal("runtime must use its scoped Agent credential")
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

	var svc *Controller
	var sawProfile bool
	var err error
	svc, err = NewController(
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

func TestCreateWorkerUsesDSHRuntimeAndPersistsBeforeNew(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dshValidator := runtimedsh.New(runtimedsh.Dependencies{
		ResolveBinary: func(context.Context) (dshcli.Info, error) {
			return dshcli.Info{Path: "/opt/dsh", Version: "0.1.5-rc.2"}, nil
		},
	})
	var svc *Controller
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: "127.0.0.1:18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind:     RuntimeKindDSH,
			validate: dshValidator.ValidateConfig,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				persisted, ok := svc.Agent(spec.AgentID)
				if !ok {
					t.Fatalf("Agent(%q) was not persisted before DSH New", spec.AgentID)
				}
				if persisted.RuntimeKind != RuntimeKindDSH || persisted.RuntimeName != RuntimeNameDSH || persisted.SandboxEnabled {
					t.Fatalf("persisted DSH selection = %q/%q/%t", persisted.RuntimeKind, persisted.RuntimeName, persisted.SandboxEnabled)
				}
				if got, want := spec.Profile.BaseURL, "http://127.0.0.1:18080/api/v1/agents/agent-dsh/llm"; got != want {
					t.Fatalf("DSH profile base URL = %q, want %q", got, want)
				}
				if got := spec.Profile.APIKey; !strings.HasPrefix(got, "agent.") || got != spec.Profile.Env["CSGCLAW_ACCESS_TOKEN"] {
					t.Fatal("DSH runtime must use its scoped Agent credential")
				}
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "dsh-agent"}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewController() error = %v", err)
	}

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		ID: "agent-dsh", Name: "dsh-worker", RuntimeKind: RuntimeKindDSH,
		AgentProfile: AgentProfile{
			Name: "dsh-worker", Provider: ProviderOpenCSG,
			ModelID: "deepseek-chat", ReasoningEffort: DefaultReasoningEffort, ProfileComplete: true,
		},
	})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.RuntimeKind != RuntimeKindDSH || got.BoxID != "dsh-agent" {
		t.Fatalf("CreateWorker() = %+v", got)
	}
}

func TestCreateWorkerDSHSupportsAllModelProviderKinds(t *testing.T) {
	for _, test := range []struct {
		name     string
		selector string
		profile  AgentProfile
		llm      config.LLMConfig
	}{
		{
			name:     "OpenCSG",
			selector: ModelProviderIDOpenCSG + ".qwen3.7-plus",
			profile:  AgentProfile{ModelProviderID: ModelProviderIDOpenCSG, ModelID: "qwen3.7-plus"},
		},
		{
			name:     "CSGHub Lite",
			selector: ModelProviderIDCSGHubLite + ".Qwen3-0.6B-GGUF",
			profile:  AgentProfile{ModelProviderID: ModelProviderIDCSGHubLite, ModelID: "Qwen3-0.6B-GGUF"},
		},
		{
			name:     "Codex",
			selector: ModelProviderIDCodex + ".gpt-5.5",
			profile:  AgentProfile{ModelProviderID: ModelProviderIDCodex, ModelID: "gpt-5.5"},
		},
		{
			name:     "Claude Code",
			selector: ModelProviderIDClaude + ".claude-sonnet",
			profile:  AgentProfile{ModelProviderID: ModelProviderIDClaude, ModelID: "claude-sonnet"},
		},
		{
			name:     "custom OpenAI compatible",
			selector: "custom.test-model",
			profile:  AgentProfile{ModelProviderID: "custom", ModelID: "test-model"},
			llm: config.LLMConfig{Providers: map[string]config.ProviderConfig{
				"custom": {BaseURL: "https://api.example/v1", APIKey: "custom-key", Models: []string{"test-model"}},
			}},
		},
		{
			name: "legacy inline API",
			profile: AgentProfile{
				Provider: ProviderAPI,
				BaseURL:  "https://api.example/v1",
				APIKey:   "inline-key",
				ModelID:  "test-model",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			validator := runtimedsh.New(runtimedsh.Dependencies{
				ResolveBinary: func(context.Context) (dshcli.Info, error) {
					return dshcli.Info{Path: "/test/dsh", Version: "0.1.5-rc.2"}, nil
				},
			})
			var provisioned agentruntime.Profile
			runtimeImpl := fakeAgentRuntime{
				kind:     RuntimeKindDSH,
				validate: validator.ValidateConfig,
				provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
					provisioned = req.Profile
					if req.Profile.BaseURL != "http://127.0.0.1:18080/api/v1/agents/agent-dsh/llm" {
						return fmt.Errorf("unexpected DSH bridge URL %q", req.Profile.BaseURL)
					}
					if !strings.HasPrefix(req.Profile.APIKey, "agent.") {
						return fmt.Errorf("DSH Agent token is missing")
					}
					return nil
				},
				new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
					return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "dsh-agent"}, nil
				},
				info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
					return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
				},
			}
			svc, err := NewController(
				config.ModelConfig{},
				config.ServerConfig{ListenAddr: "127.0.0.1:18080"},
				"manager-image:test",
				"",
				WithRuntime(runtimeImpl),
			)
			if err != nil {
				t.Fatalf("NewController() error = %v", err)
			}
			if !test.llm.IsZero() {
				svc.SetLLMConfig(test.llm)
			}

			profile := test.profile
			profile.Name = "dsh-worker"
			created, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
				ID:           "agent-dsh",
				Name:         "dsh-worker",
				RuntimeKind:  RuntimeKindDSH,
				Profile:      test.selector,
				AgentProfile: profile,
			})
			if err != nil {
				t.Fatalf("CreateWorker() error = %v", err)
			}
			if created.Status != string(agentruntime.StateRunning) || provisioned.ModelID != profile.ModelID {
				t.Fatalf("CreateWorker() = %+v, provisioned profile = %+v", created, provisioned)
			}
		})
	}
}

func TestCreateWorkerRemovesStartingCodexAgentWhenRuntimeNewFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	svc, err := NewController(
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

	svc, err := NewController(
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
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "",
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

	svc, err := NewController(
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

	svc, err := NewController(
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

func TestCodexAutomaticRestartFailurePersistsStoppedAndRetriesWithoutDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	startErr := errors.New("start failed")
	deleteCalls := 0
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		filepath.Join(t.TempDir(), "agents.json"),
		WithRuntime(fakeAgentRuntime{
			kind:    RuntimeKindCodex,
			restart: func(agentruntime.RuntimeConfigChange) (bool, error) { return true, nil },
			start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
				if startErr != nil {
					return agentruntime.StateStopped, startErr
				}
				return agentruntime.StateRunning, nil
			},
			info: func(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{State: agentruntime.StateRunning}, nil
			},
			del: func(context.Context, agentruntime.Handle) error {
				deleteCalls++
				return nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	profile := AgentProfile{Name: "dev", Provider: ProviderAPI, BaseURL: "https://old.example/v1", APIKey: "key", ModelID: "old", ProfileComplete: true}
	svc.agents["u-dev"] = Agent{ID: "u-dev", Name: "dev", RuntimeID: "rt-u-dev", RuntimeKind: RuntimeKindCodex, Role: RoleWorker, Status: string(agentruntime.StateRunning), AgentProfile: profile, ProfileComplete: true}

	_, err = svc.UpdateRecord(context.Background(), "u-dev", UpdateRequest{AgentProfile: &AgentProfile{Provider: ProviderAPI, BaseURL: "https://new.example/v1", APIKey: "key", ModelID: "new"}})
	if !errors.Is(err, startErr) {
		t.Fatalf("Update() error = %v, want %v", err, startErr)
	}
	failed, ok := svc.agentSnapshot("u-dev")
	if !ok {
		t.Fatal("Agent() ok = false")
	}
	if got, want := failed.Status, string(agentruntime.StateStopped); got != want {
		t.Fatalf("failed restart status = %q, want %q", got, want)
	}
	if !failed.AgentProfile.EnvRestartRequired {
		t.Fatal("failed restart cleared EnvRestartRequired")
	}

	startErr = nil
	restarted, err := svc.Start(context.Background(), "u-dev")
	if err != nil {
		t.Fatalf("Start() retry error = %v", err)
	}
	if restarted.AgentProfile.EnvRestartRequired || restarted.Status != string(agentruntime.StateRunning) {
		t.Fatalf("Start() retry agent = %+v, want running without restart flag", restarted)
	}
	if deleteCalls != 0 {
		t.Fatalf("Delete() calls = %d, want 0 to preserve history", deleteCalls)
	}
}

func TestUpdateAgentProfileRejectsUnavailableCodexRuntime(t *testing.T) {
	origLocateCodexCLI := locateCodexCLI
	locateCodexCLI = func() (string, error) { return "", fmt.Errorf("codex missing") }
	t.Cleanup(func() {
		locateCodexCLI = origLocateCodexCLI
	})

	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	currentProfile := AgentProfile{
		Name:            "dev",
		Provider:        ProviderCSGHubLite,
		ModelID:         "qwen3",
		ProfileComplete: true,
	}
	svc.agents["u-dev"] = Agent{
		ID:              "u-dev",
		Name:            "dev",
		RuntimeID:       "rt-u-dev",
		RuntimeKind:     RuntimeKindCodex,
		Image:           "worker-image:1",
		Role:            RoleWorker,
		Status:          string(agentruntime.StateStopped),
		Profile:         profileSelector(currentProfile),
		AgentProfile:    currentProfile,
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}

	_, err = svc.UpdateAgentProfile("u-dev", AgentProfile{
		ModelProviderID: ModelProviderIDCodex,
		ModelID:         "gpt-5.5",
	})
	if err == nil {
		t.Fatal("UpdateAgentProfile() error = nil, want Codex CLI missing error")
	}
	if !strings.Contains(err.Error(), "codex missing") {
		t.Fatalf("UpdateAgentProfile() error = %q, want Codex provider availability error", err)
	}
}

func TestUpdateAgentProfileCodexRuntimeInfersCatalogProviderIDFromStoredSelector(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	llm := config.LLMConfig{
		Default: "opencsg-aigateway.MiniMax-M2.4",
		Providers: map[string]config.ProviderConfig{
			"opencsg-aigateway": {
				BaseURL: "https://aigateway.opencsg.com/v1",
				APIKey:  "gateway-key",
				Models:  []string{"MiniMax-M2.4", "MiniMax-M2.5"},
			},
		},
	}
	svc, err := NewControllerWithLLM(
		llm,
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
				if current.Profile.Provider != ProviderAPI {
					t.Fatalf("responses probe provider = %q, want %q", current.Profile.Provider, ProviderAPI)
				}
				if current.Profile.BaseURL != "https://aigateway.opencsg.com/v1" {
					t.Fatalf("responses probe baseURL = %q, want catalog base URL", current.Profile.BaseURL)
				}
				if current.Profile.APIKey != "gateway-key" {
					t.Fatalf("responses probe apiKey = %q, want catalog api key", current.Profile.APIKey)
				}
				if current.Profile.ModelID != "MiniMax-M2.5" {
					t.Fatalf("responses probe modelID = %q, want updated model", current.Profile.ModelID)
				}
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
		Profile:     "opencsg-aigateway.MiniMax-M2.4",
		AgentProfile: AgentProfile{
			Name:            "dev",
			Provider:        ProviderAPI,
			ModelID:         "MiniMax-M2.4",
			ProfileComplete: true,
		},
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC),
	}

	view, err := svc.UpdateAgentProfile("u-dev", AgentProfile{
		ModelID: "MiniMax-M2.5",
	})
	if err != nil {
		t.Fatalf("UpdateAgentProfile() error = %v", err)
	}
	if view.ModelProviderID != "opencsg-aigateway" {
		t.Fatalf("UpdateAgentProfile().ModelProviderID = %q, want opencsg-aigateway", view.ModelProviderID)
	}

	got, ok := svc.Agent("u-dev")
	if !ok {
		t.Fatal("Agent(u-dev) = missing, want updated agent")
	}
	if got.AgentProfile.ModelProviderID != "opencsg-aigateway" {
		t.Fatalf("stored model_provider_id = %q, want opencsg-aigateway", got.AgentProfile.ModelProviderID)
	}
	if got.Profile != "opencsg-aigateway.MiniMax-M2.5" {
		t.Fatalf("stored profile selector = %q, want updated provider selector", got.Profile)
	}
}

func TestUpdateInstructionsRefreshesCodexWorkspaceAgentsFile(t *testing.T) {
	var reconcileCalls []struct {
		handle agentruntime.Handle
		change agentruntime.RuntimeConfigChange
	}
	svc, err := NewController(
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
	updated, err := svc.UpdateRecord(context.Background(), "u-dev", UpdateRequest{Instructions: &nextInstructions})
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
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeBareAgentRuntime{kind: RuntimeKindCodex}),
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
		Status:       string(sandbox.StateStopped),
		Instructions: "old instructions",
		CreatedAt:    time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}

	nextInstructions := "do not refresh"
	if _, err := svc.UpdateRecord(context.Background(), "u-dev", UpdateRequest{Instructions: &nextInstructions}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
}

func TestUpdateRuntimeOptionsSyncsCodexWorkspaceAgentsFile(t *testing.T) {
	var reconcileCalls []struct {
		handle agentruntime.Handle
		change agentruntime.RuntimeConfigChange
	}
	svc, err := NewController(
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
	updated, err := svc.UpdateRecord(context.Background(), "u-dev", UpdateRequest{RuntimeOptions: &nextRuntimeOptions})
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

func TestUpdateManagerRejectsRuntimeOptions(t *testing.T) {
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:          ManagerUserID,
		Name:        ManagerName,
		RuntimeID:   runtimeIDForAgentID(ManagerUserID),
		RuntimeKind: RuntimeKindCodex,
		Role:        RoleManager,
		Status:      string(agentruntime.StateStopped),
		CreatedAt:   time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}

	nextRuntimeOptions := map[string]any{"local_workspace_dir": "/tmp/manager"}
	for name, req := range map[string]UpdateRequest{
		"value":      {RuntimeOptions: &nextRuntimeOptions},
		"field mask": {FieldMask: []string{"runtime_options"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.UpdateRecord(context.Background(), ManagerUserID, req)
			if err == nil || !strings.Contains(err.Error(), "manager runtime options are managed automatically") {
				t.Fatalf("Update() error = %v, want managed runtime options error", err)
			}
		})
	}
	manager, ok := svc.Agent(ManagerUserID)
	if !ok {
		t.Fatal("Agent(manager) ok = false, want true")
	}
	if manager.RuntimeOptions != nil {
		t.Fatalf("manager RuntimeOptions = %#v, want nil", manager.RuntimeOptions)
	}
}

func TestUpdateCodexLocalWorkspaceDirMarksRunningRuntimeForRestart(t *testing.T) {
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
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
	updated, err := svc.UpdateRecord(context.Background(), "u-dev", UpdateRequest{RuntimeOptions: &nextRuntimeOptions})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.AgentProfile.EnvRestartRequired {
		t.Fatal("Update().AgentProfile.EnvRestartRequired = true, want false after automatic Codex restart")
	}
}

func TestAddMCPServersFromHubImportsRuntimeServersOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	listCalls := 0
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeMCPServersListRuntime{
			fakeAgentRuntime: fakeAgentRuntime{kind: RuntimeKindCodex},
			list: func(context.Context, agentruntime.Handle, agentruntime.MCPServersSnapshot) (agentruntime.MCPServersSnapshot, error) {
				listCalls++
				return agentruntime.MCPServersSnapshot{Servers: map[string]any{
					"manual": map[string]any{"command": "manual-mcp"},
				}}, nil
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
		CreatedAt:   time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}
	catalogServers := map[string]any{
		"context7": map[string]any{
			"command":     "uvx",
			"args":        []any{"context7-mcp"},
			"description": "Context7 documentation tools",
		},
		"remote": map[string]any{"url": "https://mcp.example.com/mcp"},
	}

	updated, err := svc.AddMCPServersFromHub(context.Background(), "u-dev", []string{"context7"}, catalogServers)
	if err != nil {
		t.Fatalf("first AddMCPServersFromHub() error = %v", err)
	}
	for _, name := range []string{"manual", "context7"} {
		assertMCPServersHasServer(t, updated.MCPServers, name)
	}
	context7, ok := updated.MCPServers["context7"].(map[string]any)
	if !ok {
		t.Fatalf("context7 MCP server = %#v, want object", updated.MCPServers["context7"])
	}
	if _, exists := context7["description"]; exists {
		t.Fatalf("agent MCP server retained catalog description: %#v", context7)
	}
	if listCalls != 1 {
		t.Fatalf("ListMCPServers() calls = %d, want 1 for initial runtime import", listCalls)
	}

	updated, err = svc.AddMCPServersFromHub(context.Background(), "u-dev", []string{"remote"}, catalogServers)
	if err != nil {
		t.Fatalf("second AddMCPServersFromHub() error = %v", err)
	}
	for _, name := range []string{"manual", "context7", "remote"} {
		assertMCPServersHasServer(t, updated.MCPServers, name)
	}
	if listCalls != 1 {
		t.Fatalf("ListMCPServers() calls = %d, want no second runtime import", listCalls)
	}
}

func TestDeleteMCPServersImportsRuntimeServersBeforeDeleting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	listCalls := 0
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeMCPServersListRuntime{
			fakeAgentRuntime: fakeAgentRuntime{kind: RuntimeKindCodex},
			list: func(context.Context, agentruntime.Handle, agentruntime.MCPServersSnapshot) (agentruntime.MCPServersSnapshot, error) {
				listCalls++
				return agentruntime.MCPServersSnapshot{Servers: map[string]any{
					"manual": map[string]any{"command": "manual-mcp"},
					"native": map[string]any{"command": "native-mcp"},
				}}, nil
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
		CreatedAt:   time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}

	updated, err := svc.DeleteMCPServers(context.Background(), "u-dev", []string{"manual"})
	if err != nil {
		t.Fatalf("DeleteMCPServers() error = %v", err)
	}
	if _, exists := updated.MCPServers["manual"]; exists {
		t.Fatalf("DeleteMCPServers() retained deleted server: %#v", updated.MCPServers)
	}
	assertMCPServersHasServer(t, updated.MCPServers, "native")
	if listCalls != 1 {
		t.Fatalf("ListMCPServers() calls = %d, want 1 for initial runtime import", listCalls)
	}
}

func TestDeleteMCPServersCanRemoveLegacyInvalidHeaderConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
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
		CreatedAt:   time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC),
		MCPServers: map[string]any{
			"legacy": map[string]any{
				"url":     "https://mcp.example.com/mcp",
				"headers": map[string]any{"invalid header": "value"},
			},
			"context7": map[string]any{"command": "context7-mcp"},
		},
	}

	updated, err := svc.DeleteMCPServers(context.Background(), "u-dev", []string{"legacy"})
	if err != nil {
		t.Fatalf("DeleteMCPServers() error = %v", err)
	}
	if _, exists := updated.MCPServers["legacy"]; exists {
		t.Fatalf("DeleteMCPServers() retained legacy server: %#v", updated.MCPServers)
	}
	assertMCPServersHasServer(t, updated.MCPServers, "context7")
}

func TestAddMCPServersFromHubFailsWhenRuntimeMCPReadFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	readErr := errors.New("runtime config is unreadable")
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeMCPServersListRuntime{
			fakeAgentRuntime: fakeAgentRuntime{kind: RuntimeKindCodex},
			list: func(context.Context, agentruntime.Handle, agentruntime.MCPServersSnapshot) (agentruntime.MCPServersSnapshot, error) {
				return agentruntime.MCPServersSnapshot{}, readErr
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
		CreatedAt:   time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}

	_, err = svc.AddMCPServersFromHub(context.Background(), "u-dev", []string{"context7"}, map[string]any{
		"context7": map[string]any{"command": "uvx"},
	})
	if !errors.Is(err, readErr) {
		t.Fatalf("AddMCPServersFromHub() error = %v, want runtime read failure", err)
	}
	if got, ok := svc.Agent("u-dev"); !ok || got.MCPServers != nil {
		t.Fatalf("Agent().MCPServers = %#v, want no persisted MCP server changes", got.MCPServers)
	}
}

func TestAddMCPServersSerializesConcurrentNameMerges(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var validateCalls atomic.Int32
	firstValidateEntered := make(chan struct{})
	releaseFirstValidate := make(chan struct{})
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			mcpValidate: func(ctx context.Context, current agentruntime.MCPServersSnapshot) error {
				if validateCalls.Add(1) == 1 {
					close(firstValidateEntered)
					select {
					case <-releaseFirstValidate:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return mcpschema.ValidateMCPServers(current.Servers)
			},
			mcpRestart: func(agentruntime.MCPServersChange) (bool, error) {
				return false, nil
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
		CreatedAt:    time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}
	catalogServers := map[string]any{
		"context7": map[string]any{
			"command":     "uvx",
			"args":        []any{"context7-mcp"},
			"description": "Context7",
		},
		"remote": map[string]any{
			"url":       "https://mcp.example.com/mcp",
			"transport": "streamable-http",
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	errCh := make(chan error, 2)
	go func() {
		_, err := svc.AddMCPServersFromHub(ctx, "u-dev", []string{"context7"}, catalogServers)
		errCh <- err
	}()
	select {
	case <-firstValidateEntered:
	case <-ctx.Done():
		t.Fatalf("first AddMCPServersFromHub did not enter validation: %v", ctx.Err())
	}
	go func() {
		_, err := svc.AddMCPServersFromHub(ctx, "u-dev", []string{"remote"}, catalogServers)
		errCh <- err
	}()
	time.Sleep(25 * time.Millisecond)
	close(releaseFirstValidate)
	for i := 0; i < 2; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("AddMCPServersFromHub() error = %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("AddMCPServersFromHub() timed out: %v", ctx.Err())
		}
	}

	got, ok := svc.Agent("u-dev")
	if !ok {
		t.Fatal("Agent(\"u-dev\") not found")
	}
	servers, err := mcpschema.NormalizeMCPServers(got.MCPServers)
	if err != nil {
		t.Fatalf("NormalizeMCPServers() error = %v", err)
	}
	if _, ok := servers["context7"]; !ok {
		t.Fatalf("merged MCP servers = %#v, want context7", servers)
	}
	if _, ok := servers["remote"]; !ok {
		t.Fatalf("merged MCP servers = %#v, want remote", servers)
	}
}

func TestAddMCPServersSerializesWithDirectMCPServersUpdate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var validateCalls atomic.Int32
	firstValidateEntered := make(chan struct{})
	releaseFirstValidate := make(chan struct{})
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			mcpValidate: func(ctx context.Context, current agentruntime.MCPServersSnapshot) error {
				if validateCalls.Add(1) == 1 {
					close(firstValidateEntered)
					select {
					case <-releaseFirstValidate:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return mcpschema.ValidateMCPServers(current.Servers)
			},
			mcpRestart: func(agentruntime.MCPServersChange) (bool, error) {
				return false, nil
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
		CreatedAt:    time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}

	directServers := map[string]any{
		"direct": map[string]any{
			"command": "direct-mcp",
		},
	}
	catalogServers := map[string]any{
		"catalog": map[string]any{
			"command": "catalog-mcp",
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	directErr := make(chan error, 1)
	go func() {
		_, err := svc.UpdateRecord(ctx, "u-dev", UpdateRequest{MCPServers: &directServers, MCPServersSet: true})
		directErr <- err
	}()
	select {
	case <-firstValidateEntered:
	case <-ctx.Done():
		t.Fatalf("direct Update did not enter validation: %v", ctx.Err())
	}

	addErr := make(chan error, 1)
	go func() {
		_, err := svc.AddMCPServersFromHub(ctx, "u-dev", []string{"catalog"}, catalogServers)
		addErr <- err
	}()
	close(releaseFirstValidate)
	for _, errCh := range []<-chan error{directErr, addErr} {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("MCP server mutation error = %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("MCP server mutations timed out: %v", ctx.Err())
		}
	}

	got, ok := svc.Agent("u-dev")
	if !ok {
		t.Fatal("Agent(\"u-dev\") not found")
	}
	if _, ok := got.MCPServers["direct"]; !ok {
		t.Fatalf("MCPServers = %#v, want direct server from the direct update", got.MCPServers)
	}
	if _, ok := got.MCPServers["catalog"]; !ok {
		t.Fatalf("MCPServers = %#v, want catalog server from batchAdd", got.MCPServers)
	}
}

func TestAddMCPServersFromHubRestartsRunningCodexManagerWithoutDeletingHistory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	origLocateCodexCLI := locateCodexCLI
	locateCodexCLI = func() (string, error) { return "/usr/local/bin/codex", nil }
	t.Cleanup(func() {
		locateCodexCLI = origLocateCodexCLI
	})

	deleteCalls := 0
	newCalls := 0
	var provisionedMCPServers map[string]any
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "server-token"},
		"",
		filepath.Join(t.TempDir(), "agents.json"),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			del: func(context.Context, agentruntime.Handle) error {
				deleteCalls++
				return nil
			},
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				provisionedMCPServers = cloneMCPServers(req.MCPServers)
				return nil
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				newCalls++
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-manager-session-new"}, nil
			},
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	profile := AgentProfile{
		Name:            ManagerName,
		Provider:        ProviderCodex,
		ModelID:         "gpt-5.5",
		ProfileComplete: true,
	}
	svc.agents[ManagerUserID] = Agent{
		ID:              ManagerUserID,
		Name:            ManagerName,
		RuntimeID:       runtimeIDForAgentID(ManagerUserID),
		RuntimeKind:     RuntimeKindCodex,
		RuntimeName:     RuntimeNameCodex,
		BoxID:           "codex-manager-session-old",
		Role:            RoleManager,
		Status:          string(agentruntime.StateRunning),
		Profile:         profileSelector(profile),
		AgentProfile:    profile,
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC),
	}

	updated, err := svc.AddMCPServersFromHub(context.Background(), ManagerUserID, []string{"context7"}, map[string]any{
		"context7": map[string]any{"command": "uvx", "args": []any{"context7-mcp"}},
	})
	if err != nil {
		t.Fatalf("AddMCPServersFromHub() error = %v", err)
	}
	if deleteCalls != 0 || newCalls != 0 {
		t.Fatalf("manager restart calls = delete %d/new %d, want 0/0 to preserve history", deleteCalls, newCalls)
	}
	assertMCPServersHasServer(t, provisionedMCPServers, "context7")
	assertMCPServersHasServer(t, updated.MCPServers, "context7")
	if updated.AgentProfile.EnvRestartRequired {
		t.Fatal("AddMCPServersFromHub().AgentProfile.EnvRestartRequired = true, want false after successful manager restart")
	}
	if got, want := updated.BoxID, "codex-manager-session-old"; got != want {
		t.Fatalf("AddMCPServersFromHub().BoxID = %q, want %q", got, want)
	}
}

func TestUpdateMCPServersRecreateFailureKeepsRestartRequired(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{
			ListenAddr:       "127.0.0.1:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		},
		"dsh-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindDSH,
			mcpRestart: func(change agentruntime.MCPServersChange) (bool, error) {
				return !reflect.DeepEqual(change.Previous.Servers, change.Current.Servers), nil
			},
			provision: func(context.Context, agentruntime.ProvisionRequest) error {
				return fmt.Errorf("provision failed")
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	currentProfile := AgentProfile{
		Name:            "alice",
		Provider:        ProviderAPI,
		BaseURL:         "https://api.example/v1",
		APIKey:          "api-key",
		ModelID:         "qwen3.7-max",
		ProfileComplete: true,
	}
	svc.agents["u-alice"] = Agent{
		ID:              "u-alice",
		Name:            "alice",
		RuntimeID:       "rt-u-alice",
		RuntimeKind:     RuntimeKindDSH,
		Image:           "dsh-image:test",
		BoxID:           "dsh-box-old",
		Role:            RoleWorker,
		Status:          string(agentruntime.StateRunning),
		AgentProfile:    currentProfile,
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}

	nextMCPServers := map[string]any{
		"context7": map[string]any{"command": "uvx"},
	}
	_, err = svc.UpdateRecord(context.Background(), "u-alice", UpdateRequest{MCPServers: &nextMCPServers, MCPServersSet: true})
	if err == nil || !strings.Contains(err.Error(), "provision failed") {
		t.Fatalf("Update() error = %v, want provision failed", err)
	}
	got, ok := svc.Agent("u-alice")
	if !ok {
		t.Fatal("Agent() ok = false, want true")
	}
	if !got.AgentProfile.EnvRestartRequired {
		t.Fatal("Agent().AgentProfile.EnvRestartRequired = false, want true after failed recreate")
	}
	if _, ok := got.MCPServers["context7"].(map[string]any); !ok {
		t.Fatalf("Agent().MCPServers = %#v, want saved mcp config", got.MCPServers)
	}
	if got.BoxID != "dsh-box-old" {
		t.Fatalf("Agent().BoxID = %q, want old box id after failed recreate", got.BoxID)
	}
}

func TestUpdateRejectsMCPRuntimeOptions(t *testing.T) {
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-dev"] = Agent{
		ID:              "u-dev",
		Name:            "dev",
		RuntimeID:       "rt-u-dev",
		RuntimeKind:     RuntimeKindCodex,
		Image:           "codex-image:test",
		Role:            RoleWorker,
		Status:          "profile_incomplete",
		AgentProfile:    AgentProfile{Name: "dev"},
		ProfileComplete: false,
		CreatedAt:       time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}
	nextRuntimeOptions := map[string]any{
		"mcp": map[string]any{"mcpServers": map[string]any{}},
	}
	if _, err := svc.UpdateRecord(context.Background(), "u-dev", UpdateRequest{RuntimeOptions: &nextRuntimeOptions}); err == nil || !strings.Contains(err.Error(), "runtime_options.mcp is not supported") {
		t.Fatalf("Update() error = %v, want rejected MCP runtime option", err)
	}
	got, ok := svc.Agent("u-dev")
	if !ok {
		t.Fatal("Agent(u-dev) not found")
	}
	if _, ok := got.RuntimeOptions["mcp"]; ok {
		t.Fatalf("Agent().RuntimeOptions = %#v, want rejected MCP runtime option omitted", got.RuntimeOptions)
	}
	if got.MCPServers != nil {
		t.Fatalf("Agent().MCPServers = %#v, want nil", got.MCPServers)
	}
}

func TestValidateRuntimeOptionsWithoutMCPRejectsBothReservedKeys(t *testing.T) {
	for name, runtimeOptions := range map[string]map[string]any{
		"mcp":        {"mcp": map[string]any{}},
		"mcpServers": {"mcpServers": map[string]any{}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateRuntimeOptionsWithoutMCP(runtimeOptions); err == nil {
				t.Fatalf("validateRuntimeOptionsWithoutMCP(%#v) error = nil", runtimeOptions)
			}
		})
	}
}

func TestCreateRejectsMCPRuntimeOptions(t *testing.T) {
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateRecord(context.Background(), CreateRequest{Spec: CreateAgentSpec{
		Name:        "dev2",
		Role:        RoleWorker,
		RuntimeKind: RuntimeKindCodex,
		Image:       "codex-image:test",
		RuntimeOptions: map[string]any{
			"mcp": map[string]any{"mcpServers": map[string]any{}},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "runtime_options.mcp is not supported") {
		t.Fatalf("Create() error = %v, want rejected MCP runtime option", err)
	}

	created, err := svc.CreateRecord(context.Background(), CreateRequest{Spec: CreateAgentSpec{
		Name:          "dev3",
		Role:          RoleWorker,
		RuntimeKind:   RuntimeKindCodex,
		Image:         "codex-image:test",
		MCPServers:    map[string]any{},
		MCPServersSet: true,
	}})
	if err != nil {
		t.Fatalf("Create(empty MCPServers) error = %v", err)
	}
	if created.MCPServers == nil || len(created.MCPServers) != 0 {
		t.Fatalf("Create().MCPServers = %#v, want an explicit empty server map", created.MCPServers)
	}
}

func TestUpdateMCPServersClearPersistsExplicitEmptyMap(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	statePath := filepath.Join(homeDir, "agents.json")

	newService := func() *Controller {
		svc, err := NewController(
			testModelConfig(),
			config.ServerConfig{},
			"",
			statePath,
			WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
		)
		if err != nil {
			t.Fatalf("NewService() error = %v", err)
		}
		return svc
	}

	svc := newService()
	svc.agents["u-dev"] = Agent{
		ID:          "u-dev",
		Name:        "dev",
		RuntimeID:   "rt-u-dev",
		RuntimeKind: RuntimeKindCodex,
		RuntimeName: RuntimeNameCodex,
		Role:        RoleWorker,
		Status:      string(agentruntime.StateStopped),
		CreatedAt:   time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC),
		MCPServers: map[string]any{
			"context7": map[string]any{"command": "npx"},
		},
	}

	empty := map[string]any{}
	updated, err := svc.UpdateRecord(context.Background(), "u-dev", UpdateRequest{
		MCPServers:    &empty,
		MCPServersSet: true,
		FieldMask:     []string{"mcpServers"},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.MCPServers == nil || len(updated.MCPServers) != 0 {
		t.Fatalf("Update().MCPServers = %#v, want explicit empty map", updated.MCPServers)
	}

	reloaded := newService()
	got, ok := reloaded.Agent("u-dev")
	if !ok {
		t.Fatal("reloaded Agent(u-dev) not found")
	}
	if got.MCPServers == nil || len(got.MCPServers) != 0 {
		t.Fatalf("reloaded MCPServers = %#v, want explicit empty map", got.MCPServers)
	}
}

func TestReplacePreservesInheritedMCPBeforeDeletingExistingAgent(t *testing.T) {
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindDSH}),
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-dev"] = Agent{
		ID:          "u-dev",
		Name:        "dev",
		RuntimeID:   "rt-u-dev",
		RuntimeKind: RuntimeKindDSH,
		Image:       "dsh-image:test",
		BoxID:       "dsh-box-old",
		Role:        RoleWorker,
		Status:      string(agentruntime.StateStopped),
		MCPServers: map[string]any{
			"context7": map[string]any{"command": "uvx"},
		},
		AgentProfile: AgentProfile{
			Name:            "dev",
			Provider:        ProviderAPI,
			BaseURL:         "https://api.example/v1",
			APIKey:          "api-key",
			ModelID:         "qwen3.7-max",
			ProfileComplete: true,
		},
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC),
	}

	replaced, err := svc.CreateRecord(context.Background(), CreateRequest{
		Replace:   true,
		FieldMask: []string{"runtime_kind", "image"},
		Spec: CreateAgentSpec{
			ID:          "u-dev",
			RuntimeKind: RuntimeKindCodex,
			Image:       "codex-image:test",
		},
	})
	if err != nil {
		t.Fatalf("Create(replace) error = %v", err)
	}
	if replaced.RuntimeKind != RuntimeKindCodex {
		t.Fatalf("Create(replace).RuntimeKind = %q, want %q", replaced.RuntimeKind, RuntimeKindCodex)
	}
	if _, ok := replaced.MCPServers["context7"]; !ok {
		t.Fatalf("Create(replace).MCPServers = %#v, want inherited context7", replaced.MCPServers)
	}
}

func TestMergeReplaceSpecRuntimeFieldMaskPreservesMCPServers(t *testing.T) {
	existing := Agent{
		ID:             "u-dev",
		Name:           "dev",
		RuntimeKind:    RuntimeKindCodex,
		RuntimeName:    RuntimeNameCodex,
		RuntimeOptions: map[string]any{"local_workspace_dir": "/tmp/old"},
		MCPServers: map[string]any{
			"context7": map[string]any{"command": "npx"},
		},
	}
	next := CreateAgentSpec{
		ID:             existing.ID,
		RuntimeKind:    RuntimeKindCodex,
		RuntimeName:    RuntimeNameCodex,
		RuntimeOptions: map[string]any{"local_workspace_dir": "/tmp/new"},
	}

	merged, err := mergeReplaceSpec(existing, next, []string{"runtime"})
	if err != nil {
		t.Fatalf("mergeReplaceSpec() error = %v", err)
	}
	if got, want := merged.RuntimeOptions["local_workspace_dir"], "/tmp/new"; got != want {
		t.Fatalf("merged runtime option = %#v, want %q", got, want)
	}
	if !reflect.DeepEqual(merged.MCPServers, existing.MCPServers) {
		t.Fatalf("merged MCPServers = %#v, want preserved %#v", merged.MCPServers, existing.MCPServers)
	}
}

func TestUpdateFieldMaskClearsRuntimeOptions(t *testing.T) {
	svc, err := NewController(
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
	updated, err := svc.UpdateRecord(context.Background(), "u-dev", UpdateRequest{
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

func TestCreateWorkerProvisionsRuntimeBeforeNew(t *testing.T) {
	var callOrder []string
	svc, err := NewController(
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
				if got := req.Profile.APIKey; !strings.HasPrefix(got, "agent.") || got != req.Profile.Env["CSGCLAW_ACCESS_TOKEN"] {
					t.Fatal("runtime must use its scoped Agent credential")
				}
				if req.WorkspaceOverlay != "" {
					t.Fatalf("Provision() workspace overlay = %q, want empty", req.WorkspaceOverlay)
				}
				if req.Credentials["secrets/token"] != "runtime-secret" || req.InitShell != "test -f secrets/token" {
					t.Fatalf("Provision() Runtime provisioning = %#v, %q", req.Credentials, req.InitShell)
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
		ID:                 "u-alice",
		Name:               "alice",
		RuntimeKind:        RuntimeKindCodex,
		RuntimeCredentials: map[string]string{"secrets/token": "runtime-secret"},
		RuntimeInitShell:   "test -f secrets/token",
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
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{}, "manager-image:test", "",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
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
		RuntimeKind: RuntimeKindCodex,
		Image:       "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/codex-worker:dev",
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
	svc, err := NewController(
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
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string, name, botID string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			if name != "alice" || botID != "agent-worker-123" {
				t.Fatalf("createGatewayBox() name=%q botID=%q, want alice/agent-worker-123", name, botID)
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

	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindDSH}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	created, err := svc.CreateWorker(context.Background(), CreateAgentSpec{
		ID:          "agent-worker-123",
		Name:        "alice",
		RuntimeKind: RuntimeKindDSH,
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
	skillsRoot, err := svc.agentSkillsRoot(created.ID, RuntimeKindDSH)
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
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindDSH}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	skillsRoot, err := svc.agentSkillsRoot("alice", RuntimeKindDSH)
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

	preserved, err := svc.prepareWorkspaceSkillsPreservation("alice", RuntimeKindDSH, RuntimeKindDSH, RoleWorker)
	if err != nil {
		t.Fatalf("prepareWorkspaceSkillsPreservation() error = %v", err)
	}
	if preserved != nil {
		defer preserved.Cleanup()
	}
	agentHome, err := agentHomeDir("alice")
	if err != nil {
		t.Fatalf("agentHomeDir() error = %v", err)
	}
	if err := os.RemoveAll(agentHome); err != nil {
		t.Fatalf("RemoveAll(agent home) error = %v", err)
	}
	if preserved == nil {
		t.Fatal("preserved = nil, want preservation restore")
	}
	if err := preserved.Restore(); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillsRoot, "custom", "SKILL.md")); err != nil {
		t.Fatalf("custom skill was not restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillsRoot, "skill-installer", "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old skill-installer restored unexpectedly, err=%v", err)
	}
	if err := svc.installDefaultSystemSkills("agent-alice", RuntimeKindDSH); err != nil {
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

func TestPreserveWorkspaceSkillsRestoresGitRepositoryWithReadOnlyObjects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	skillsRoot, err := svc.agentSkillsRoot("agent-qa", RuntimeKindCodex)
	if err != nil {
		t.Fatalf("agentSkillsRoot() error = %v", err)
	}
	objectPath := filepath.Join(skillsRoot, ".git", "objects", "02", "b97d10dc320cdc19d01fdb97255fd3cbeb71a0")
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(objectPath, []byte("git object\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(objectPath, 0o444); err != nil {
		t.Fatal(err)
	}

	preserved, err := svc.prepareWorkspaceSkillsPreservation("agent-qa", RuntimeKindCodex, RuntimeKindCodex, RoleWorker)
	if err != nil {
		t.Fatalf("prepareWorkspaceSkillsPreservation() error = %v", err)
	}
	if preserved != nil {
		defer preserved.Cleanup()
	}
	if preserved == nil {
		t.Fatal("preserved = nil, want repository preservation")
	}
	if err := preserved.Restore(); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	data, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "git object\n"; got != want {
		t.Fatalf("restored Git object = %q, want %q", got, want)
	}
}

func TestPreserveWorkspaceSkillsUsesRenameForBackupAndRestore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	skillsRoot, err := svc.agentSkillsRoot("agent-qa", RuntimeKindCodex)
	if err != nil {
		t.Fatalf("agentSkillsRoot() error = %v", err)
	}
	customPath := filepath.Join(skillsRoot, "custom", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(customPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(customPath, []byte("# Custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat(customPath)
	if err != nil {
		t.Fatal(err)
	}

	preserved, err := svc.prepareWorkspaceSkillsPreservation("agent-qa", RuntimeKindCodex, RuntimeKindCodex, RoleWorker)
	if err != nil {
		t.Fatalf("prepareWorkspaceSkillsPreservation() error = %v", err)
	}
	if preserved == nil {
		t.Fatal("preserved = nil, want preservation")
	}
	preservedPath := filepath.Join(preserved.preservedRoot, "custom", "SKILL.md")
	preservedInfo, err := os.Stat(preservedPath)
	if err != nil {
		t.Fatalf("Stat(preserved skill) error = %v", err)
	}
	if !os.SameFile(originalInfo, preservedInfo) {
		t.Fatal("preserved skill file identity changed; want rename preservation")
	}
	if _, err := os.Stat(customPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source custom skill still exists after preservation, err=%v", err)
	}

	writeWorkspaceFileAt(t, skillsRoot, "custom/SKILL.md", "# Replacement\n", 0o644)
	writeWorkspaceFileAt(t, skillsRoot, "custom/generated.txt", "replacement\n", 0o644)
	if err := preserved.Restore(); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	restoredInfo, err := os.Stat(customPath)
	if err != nil {
		t.Fatalf("Stat(restored skill) error = %v", err)
	}
	if !os.SameFile(originalInfo, restoredInfo) {
		t.Fatal("restored skill file identity changed; want rename restoration")
	}
	generated, err := os.ReadFile(filepath.Join(skillsRoot, "custom", "generated.txt"))
	if err != nil {
		t.Fatalf("ReadFile(merged replacement skill) error = %v", err)
	}
	if got, want := string(generated), "replacement\n"; got != want {
		t.Fatalf("merged replacement skill = %q, want %q", got, want)
	}
	if err := preserved.RevertRestore(); err != nil {
		t.Fatalf("RevertRestore() error = %v", err)
	}
	replacement, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("ReadFile(reverted replacement skill) error = %v", err)
	}
	if got, want := string(replacement), "# Replacement\n"; got != want {
		t.Fatalf("reverted replacement skill = %q, want %q", got, want)
	}
	if err := preserved.Restore(); err != nil {
		t.Fatalf("second Restore() error = %v", err)
	}
	preserved.Cleanup()
	if _, err := os.Stat(preserved.tempRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preservation directory still exists after cleanup, err=%v", err)
	}
}

func TestPreserveWorkspaceSkillsMergesIntoReadOnlyDirectoryAndRestoresMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewController() error = %v", err)
	}
	skillsRoot, err := svc.agentSkillsRoot("agent-qa", RuntimeKindCodex)
	if err != nil {
		t.Fatal(err)
	}
	readOnlyDir := filepath.Join(skillsRoot, "custom", "read-only")
	writeWorkspaceFileAt(t, skillsRoot, "custom/SKILL.md", "# Custom\n", 0o644)
	writeWorkspaceFileAt(t, skillsRoot, "custom/read-only/original.txt", "original\n", 0o644)
	if err := os.Chmod(readOnlyDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnlyDir, 0o755) })

	preserved, err := svc.prepareWorkspaceSkillsPreservation("agent-qa", RuntimeKindCodex, RuntimeKindCodex, RoleWorker)
	if err != nil {
		t.Fatalf("prepareWorkspaceSkillsPreservation() error = %v", err)
	}
	if preserved == nil {
		t.Fatal("preserved = nil, want preservation")
	}
	writeWorkspaceFileAt(t, skillsRoot, "custom/SKILL.md", "# Replacement\n", 0o644)
	writeWorkspaceFileAt(t, skillsRoot, "custom/read-only/generated.txt", "generated\n", 0o644)
	if err := os.Chmod(readOnlyDir, 0o555); err != nil {
		t.Fatal(err)
	}

	if err := preserved.Restore(); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(readOnlyDir, "generated.txt")); err != nil || string(data) != "generated\n" {
		t.Fatalf("merged generated file = %q, %v", string(data), err)
	}
	if info, err := os.Stat(readOnlyDir); err != nil {
		t.Fatalf("Stat(read-only directory) error = %v", err)
	} else if info.Mode().Perm() != 0o555 {
		t.Fatalf("read-only directory mode = %v, want 0555", info.Mode().Perm())
	}

	if err := preserved.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(readOnlyDir, "generated.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement file remains after rollback, err=%v", err)
	}
	if data, err := os.ReadFile(filepath.Join(readOnlyDir, "original.txt")); err != nil || string(data) != "original\n" {
		t.Fatalf("original file after rollback = %q, %v", string(data), err)
	}
	if info, err := os.Stat(readOnlyDir); err != nil {
		t.Fatalf("Stat(read-only directory after rollback) error = %v", err)
	} else if info.Mode().Perm() != 0o555 {
		t.Fatalf("read-only directory mode after rollback = %v, want 0555", info.Mode().Perm())
	}
}

func TestNewControllerCleansInterruptedInitialWorkspaceSkillsJournal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewController() error = %v", err)
	}
	agentHome, err := svc.agentHomeDir("agent-qa")
	if err != nil {
		t.Fatal(err)
	}
	tempRoot := workspaceSkillsTransactionRoot(agentHome)
	if err := os.MkdirAll(tempRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempRoot, workspaceSkillsStateFileName+".tmp"), []byte("{\"version\":"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	); err != nil {
		t.Fatalf("NewController() recovery error = %v", err)
	}
	if _, err := os.Stat(tempRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-journal transaction directory remains, err=%v", err)
	}
}

func TestWorkspaceSkillsRecoveryRejectsUnknownStateWithoutJournal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewController() error = %v", err)
	}
	agentHome, err := svc.agentHomeDir("agent-qa")
	if err != nil {
		t.Fatal(err)
	}
	tempRoot := workspaceSkillsTransactionRoot(agentHome)
	if err := os.MkdirAll(filepath.Join(tempRoot, "preserved"), 0o700); err != nil {
		t.Fatal(err)
	}

	err = svc.recoverWorkspaceSkillsTransactionAt(tempRoot)
	if err == nil || !strings.Contains(err.Error(), "has no state file") {
		t.Fatalf("recoverWorkspaceSkillsTransactionAt() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempRoot, "preserved")); err != nil {
		t.Fatalf("unknown transaction data was removed: %v", err)
	}
}

func TestNewControllerRecoversInterruptedWorkspaceSkillsTransaction(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewController() error = %v", err)
	}
	skillsRoot, err := svc.agentSkillsRoot("agent-qa", RuntimeKindCodex)
	if err != nil {
		t.Fatal(err)
	}
	customPath := filepath.Join(skillsRoot, "custom", "SKILL.md")
	writeWorkspaceFileAt(t, skillsRoot, "custom/SKILL.md", "# Custom\n", 0o644)
	preserved, err := svc.prepareWorkspaceSkillsPreservation("agent-qa", RuntimeKindCodex, RuntimeKindCodex, RoleWorker)
	if err != nil {
		t.Fatalf("prepareWorkspaceSkillsPreservation() error = %v", err)
	}
	if preserved == nil {
		t.Fatal("preserved = nil, want preservation")
	}
	if _, err := os.Stat(customPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("custom skill still exists during preservation, err=%v", err)
	}

	if _, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	); err != nil {
		t.Fatalf("NewController() recovery error = %v", err)
	}
	if data, err := os.ReadFile(customPath); err != nil || string(data) != "# Custom\n" {
		t.Fatalf("recovered custom skill = %q, %v", string(data), err)
	}
	if _, err := os.Stat(preserved.tempRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("transaction directory remains after recovery, err=%v", err)
	}
}

func TestNewControllerRecognizesCompletedWorkspaceSkillsRollback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewController() error = %v", err)
	}
	skillsRoot, err := svc.agentSkillsRoot("agent-qa", RuntimeKindCodex)
	if err != nil {
		t.Fatal(err)
	}
	customPath := filepath.Join(skillsRoot, "custom", "SKILL.md")
	writeWorkspaceFileAt(t, skillsRoot, "custom/SKILL.md", "# Custom\n", 0o644)
	preserved, err := svc.prepareWorkspaceSkillsPreservation("agent-qa", RuntimeKindCodex, RuntimeKindCodex, RoleWorker)
	if err != nil {
		t.Fatalf("prepareWorkspaceSkillsPreservation() error = %v", err)
	}
	if preserved == nil {
		t.Fatal("preserved = nil, want preservation")
	}
	if err := os.Remove(preserved.sourceRoot); err != nil {
		t.Fatalf("remove temporary source root: %v", err)
	}
	if err := os.Rename(preserved.preservedRoot, preserved.sourceRoot); err != nil {
		t.Fatalf("finish rollback rename: %v", err)
	}

	if _, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	); err != nil {
		t.Fatalf("NewController() recovery error = %v", err)
	}
	if data, err := os.ReadFile(customPath); err != nil || string(data) != "# Custom\n" {
		t.Fatalf("custom skill after completed rollback recovery = %q, %v", string(data), err)
	}
	if _, err := os.Stat(preserved.tempRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("transaction directory remains after completed rollback recovery, err=%v", err)
	}
}

func TestNewControllerRecoversInterruptedWorkspaceSkillsRestore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewController() error = %v", err)
	}
	skillsRoot, err := svc.agentSkillsRoot("agent-qa", RuntimeKindCodex)
	if err != nil {
		t.Fatal(err)
	}
	customPath := filepath.Join(skillsRoot, "custom", "SKILL.md")
	generatedPath := filepath.Join(skillsRoot, "custom", "generated.txt")
	writeWorkspaceFileAt(t, skillsRoot, "custom/SKILL.md", "# Custom\n", 0o644)
	preserved, err := svc.prepareWorkspaceSkillsPreservation("agent-qa", RuntimeKindCodex, RuntimeKindCodex, RoleWorker)
	if err != nil {
		t.Fatalf("prepareWorkspaceSkillsPreservation() error = %v", err)
	}
	if preserved == nil {
		t.Fatal("preserved = nil, want preservation")
	}
	writeWorkspaceFileAt(t, skillsRoot, "custom/SKILL.md", "# Replacement\n", 0o644)
	writeWorkspaceFileAt(t, skillsRoot, "custom/generated.txt", "generated\n", 0o644)
	if err := preserved.Restore(); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if _, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	); err != nil {
		t.Fatalf("NewController() recovery error = %v", err)
	}
	if data, err := os.ReadFile(customPath); err != nil || string(data) != "# Custom\n" {
		t.Fatalf("recovered custom skill = %q, %v", string(data), err)
	}
	if _, err := os.Stat(generatedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement file remains after interrupted restore recovery, err=%v", err)
	}
	if _, err := os.Stat(preserved.tempRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("transaction directory remains after restore recovery, err=%v", err)
	}
}

func TestPreserveWorkspaceSkillsRejectsNestedSymlinkWithoutMovingSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	skillsRoot, err := svc.agentSkillsRoot("agent-qa", RuntimeKindCodex)
	if err != nil {
		t.Fatalf("agentSkillsRoot() error = %v", err)
	}
	customRoot := filepath.Join(skillsRoot, "custom")
	if err := os.MkdirAll(customRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(target, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(customRoot, "outside.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("Symlink() unsupported: %v", err)
	}

	preserved, err := svc.prepareWorkspaceSkillsPreservation("agent-qa", RuntimeKindCodex, RuntimeKindCodex, RoleWorker)
	if !errors.Is(err, ErrWorkspaceSymlinkDenied) {
		t.Fatalf("prepareWorkspaceSkillsPreservation() error = %v, want ErrWorkspaceSymlinkDenied", err)
	}
	if preserved != nil {
		t.Fatal("preserved != nil after symlink rejection")
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("source symlink changed after rejection: %v", err)
	}
}

func TestRecreateRestoresRenamedCodexSkillsWhenDeleteFails(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	stopCalls := 0
	startCalls := 0
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			stop: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
				stopCalls++
				return agentruntime.StateStopped, nil
			},
			start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
				startCalls++
				return agentruntime.StateRunning, nil
			},
			del: func(context.Context, agentruntime.Handle) error {
				return errors.New("delete failed")
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["agent-qa"] = Agent{
		ID: "agent-qa", Name: "qa", Role: RoleWorker,
		RuntimeID: "rt-agent-qa", RuntimeKind: RuntimeKindCodex,
		BoxID: "session-old", Status: string(agentruntime.StateRunning),
		AgentProfile: AgentProfile{
			Name: "qa", Provider: ProviderAPI, BaseURL: "https://api.example/v1",
			APIKey: "api-key", ModelID: "gpt-4.1", ProfileComplete: true,
		},
		ProfileComplete: true,
	}
	skillsRoot, err := svc.agentSkillsRoot("agent-qa", RuntimeKindCodex)
	if err != nil {
		t.Fatal(err)
	}
	customPath := filepath.Join(skillsRoot, "custom", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(customPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(customPath, []byte("# Custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.RecreateRecord(context.Background(), "agent-qa"); err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("RecreateRecord() error = %v, want delete failure", err)
	}
	data, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("ReadFile(restored custom skill) error = %v", err)
	}
	if got, want := string(data), "# Custom\n"; got != want {
		t.Fatalf("restored custom skill = %q, want %q", got, want)
	}
	if stopCalls != 1 {
		t.Fatalf("Stop() calls = %d, want 1", stopCalls)
	}
	if startCalls != 1 {
		t.Fatalf("Start() calls = %d, want 1", startCalls)
	}
}

func TestRecreateGatewayPreservationFailureKeepsOriginalRuntime(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stopCalls := 0
	startCalls := 0
	deleteCalls := 0
	newCalls := 0
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindDSH,
			stop: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
				stopCalls++
				return agentruntime.StateStopped, nil
			},
			start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
				startCalls++
				return agentruntime.StateRunning, nil
			},
			del: func(context.Context, agentruntime.Handle) error {
				deleteCalls++
				return nil
			},
			new: func(context.Context, agentruntime.Spec) (agentruntime.Handle, error) {
				newCalls++
				return agentruntime.Handle{}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewController() error = %v", err)
	}
	svc.agents["agent-qa"] = Agent{
		ID: "agent-qa", Name: "qa", Role: RoleWorker,
		RuntimeID: "rt-agent-qa", RuntimeKind: RuntimeKindDSH,
		RuntimeName: RuntimeNameDSH, SandboxEnabled: true,
		Image: "dsh:test", BoxID: "box-old", Status: string(agentruntime.StateRunning),
		AgentProfile: AgentProfile{
			Name: "qa", Provider: ProviderAPI, BaseURL: "https://api.example/v1",
			APIKey: "api-key", ModelID: "gpt-4.1", ProfileComplete: true,
		},
		ProfileComplete: true,
	}
	skillsRoot, err := svc.agentSkillsRoot("agent-qa", RuntimeKindDSH)
	if err != nil {
		t.Fatal(err)
	}
	customRoot := filepath.Join(skillsRoot, "custom")
	if err := os.MkdirAll(customRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(target, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(customRoot, "outside.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("Symlink() unsupported: %v", err)
	}

	if _, err := svc.RecreateRecord(context.Background(), "agent-qa"); !errors.Is(err, ErrWorkspaceSymlinkDenied) {
		t.Fatalf("RecreateRecord() error = %v, want ErrWorkspaceSymlinkDenied", err)
	}
	if stopCalls != 1 || startCalls != 1 {
		t.Fatalf("runtime stop/start calls = %d/%d, want 1/1", stopCalls, startCalls)
	}
	if deleteCalls != 0 || newCalls != 0 {
		t.Fatalf("runtime delete/new calls = %d/%d, want 0/0", deleteCalls, newCalls)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("custom skill changed after preservation failure: %v", err)
	}
}

func TestRecreateCodexPreservesLocallyEditedHostSkill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(runtimecodex.TestOnlySetResponsesAPIProbe(func(context.Context, string, string, string, map[string]string) error {
		return nil
	}))
	hostCodexHome := filepath.Join(t.TempDir(), "host-codex")
	t.Setenv("CODEX_HOME", hostCodexHome)
	hostSkillPath := filepath.Join(hostCodexHome, "skills", "host-custom", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(hostSkillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostSkillPath, []byte("# Host Original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	const agentID = "agent-qa"
	var svc *Controller
	runtimeImpl := newRecreateTestCodexRuntime(func() *Controller { return svc }, agentID)
	var err error
	svc, err = NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(runtimeImpl),
	)
	if err != nil {
		t.Fatalf("NewController() error = %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	svc.agents[agentID] = Agent{
		ID: agentID, Name: "qa", Role: RoleWorker,
		RuntimeID: runtimeIDForAgentID(agentID), RuntimeKind: RuntimeKindCodex,
		BoxID: "session-old", Status: string(agentruntime.StateRunning),
		AgentProfile: AgentProfile{
			Name: "qa", Provider: ProviderAPI, BaseURL: "https://api.example/v1",
			APIKey: "api-key", ModelID: "gpt-4.1", ProfileComplete: true,
		},
		ProfileComplete: true,
	}
	skillsRoot, err := svc.agentSkillsRoot(agentID, RuntimeKindCodex)
	if err != nil {
		t.Fatal(err)
	}
	agentSkillPath := filepath.Join(skillsRoot, "host-custom", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(agentSkillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentSkillPath, []byte("# Agent Edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.RecreateRecord(context.Background(), agentID); err != nil {
		t.Fatalf("RecreateRecord() error = %v", err)
	}
	data, err := os.ReadFile(agentSkillPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "# Agent Edited\n"; got != want {
		t.Fatalf("agent skill after recreate = %q, want %q", got, want)
	}
	assertDefaultSystemSkills(t, skillsRoot)
}

func TestRecreateCodexManagerPreservesLocallyEditedHostSkill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(runtimecodex.TestOnlySetResponsesAPIProbe(func(context.Context, string, string, string, map[string]string) error {
		return nil
	}))
	hostCodexHome := filepath.Join(t.TempDir(), "host-codex")
	t.Setenv("CODEX_HOME", hostCodexHome)
	hostSkillPath := filepath.Join(hostCodexHome, "skills", "host-custom", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(hostSkillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostSkillPath, []byte("# Host Original\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var svc *Controller
	runtimeImpl := newRecreateTestCodexRuntime(func() *Controller { return svc }, ManagerUserID)
	var err error
	svc, err = NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(runtimeImpl),
	)
	if err != nil {
		t.Fatalf("NewController() error = %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	svc.agents[ManagerUserID] = Agent{
		ID: ManagerUserID, Name: ManagerName, Role: RoleManager,
		RuntimeID: runtimeIDForAgentID(ManagerUserID), RuntimeKind: RuntimeKindCodex,
		BoxID: "manager-old", Status: string(agentruntime.StateRunning),
		AgentProfile: AgentProfile{
			Name: ManagerName, Provider: ProviderAPI, BaseURL: "https://api.example/v1",
			APIKey: "api-key", ModelID: "gpt-4.1", ProfileComplete: true,
		},
		ProfileComplete: true,
	}
	skillsRoot, err := svc.agentSkillsRoot(ManagerUserID, RuntimeKindCodex)
	if err != nil {
		t.Fatal(err)
	}
	agentSkillPath := filepath.Join(skillsRoot, "host-custom", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(agentSkillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentSkillPath, []byte("# Manager Edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.RecreateRecord(context.Background(), ManagerUserID); err != nil {
		t.Fatalf("RecreateRecord() error = %v", err)
	}
	data, err := os.ReadFile(agentSkillPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "# Manager Edited\n"; got != want {
		t.Fatalf("manager skill after recreate = %q, want %q", got, want)
	}
	assertDefaultSystemSkills(t, skillsRoot)
}

func TestRecreateManagerPreservesCustomSkills(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			stop: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
				return agentruntime.StateStopped, nil
			},
			start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
				return agentruntime.StateRunning, nil
			},
			del: func(context.Context, agentruntime.Handle) error {
				return nil
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "manager-new"}, nil
			},
			info: func(_ context.Context, handle agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: handle.HandleID, State: agentruntime.StateRunning}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewController() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID: ManagerUserID, Name: ManagerName, Role: RoleManager,
		RuntimeID: runtimeIDForAgentID(ManagerUserID), RuntimeKind: RuntimeKindCodex,
		BoxID: "manager-old", Status: string(agentruntime.StateRunning),
		AgentProfile: AgentProfile{
			Name: ManagerName, Provider: ProviderAPI, BaseURL: "https://api.example/v1",
			APIKey: "api-key", ModelID: "gpt-4.1", ProfileComplete: true,
		},
		ProfileComplete: true,
	}
	skillsRoot, err := svc.agentSkillsRoot(ManagerUserID, RuntimeKindCodex)
	if err != nil {
		t.Fatal(err)
	}
	customPath := filepath.Join(skillsRoot, "custom-manager", "SKILL.md")
	writeWorkspaceFileAt(t, skillsRoot, "custom-manager/SKILL.md", "# Manager Custom\n", 0o644)

	if _, err := svc.RecreateRecord(context.Background(), ManagerUserID); err != nil {
		t.Fatalf("RecreateRecord() error = %v", err)
	}
	if data, err := os.ReadFile(customPath); err != nil || string(data) != "# Manager Custom\n" {
		t.Fatalf("manager custom skill after recreate = %q, %v", string(data), err)
	}
}

func TestRecreateWaitsForConcurrentStartOfSameAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	startEntered := make(chan struct{})
	releaseStart := make(chan struct{})
	deleteEntered := make(chan struct{})
	var startOnce sync.Once
	var deleteOnce sync.Once
	rt := fakeAgentRuntime{
		kind: RuntimeKindCodex,
		start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
			startOnce.Do(func() { close(startEntered) })
			<-releaseStart
			return agentruntime.StateRunning, nil
		},
		del: func(context.Context, agentruntime.Handle) error {
			deleteOnce.Do(func() { close(deleteEntered) })
			return nil
		},
		new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
			return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "sess-new"}, nil
		},
		info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
			return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
		},
	}
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(rt),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["agent-alice"] = Agent{
		ID:          "agent-alice",
		Name:        "alice",
		Role:        RoleWorker,
		RuntimeID:   "rt-agent-alice",
		RuntimeKind: RuntimeKindCodex,
		BoxID:       "sess-old",
		Status:      string(agentruntime.StateRunning),
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderCodex,
			ModelID:         "gpt-5.5",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	startResult := make(chan error, 1)
	go func() {
		_, err := svc.Start(context.Background(), "agent-alice")
		startResult <- err
	}()
	select {
	case <-startEntered:
	case <-time.After(time.Second):
		t.Fatal("Start() did not enter runtime")
	}

	recreateResult := make(chan error, 1)
	go func() {
		_, err := svc.RecreateRecord(context.Background(), "agent-alice")
		recreateResult <- err
	}()
	select {
	case <-deleteEntered:
		t.Fatal("Recreate() deleted the runtime while Start() still owned the agent lifecycle")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseStart)
	if err := <-startResult; err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := <-recreateResult; err != nil {
		t.Fatalf("Recreate() error = %v", err)
	}
	select {
	case <-deleteEntered:
	case <-time.After(time.Second):
		t.Fatal("Recreate() did not delete the runtime after Start() completed")
	}
}

func TestRecreateProvisionsRuntimeBeforeDeleteAndNew(t *testing.T) {
	var callOrder []string
	svc, err := NewController(
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
				if got := req.Profile.APIKey; !strings.HasPrefix(got, "agent.") || got != req.Profile.Env["CSGCLAW_ACCESS_TOKEN"] {
					t.Fatal("runtime must use its scoped Agent credential")
				}
				if req.WorkspaceOverlay != "" {
					t.Fatalf("Provision() workspace overlay = %q, want empty", req.WorkspaceOverlay)
				}
				if req.Credentials["secrets/token"] != "runtime-secret" || req.InitShell != "test -f secrets/token" {
					t.Fatalf("Provision() Runtime provisioning paths = %#v, initShell = %q", sortedStringKeys(req.Credentials), req.InitShell)
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
	provisionedAgent := svc.agents["u-alice"]
	provisionedAgent.SetRuntimeProvision(map[string]string{"secrets/token": "runtime-secret"}, "test -f secrets/token")
	svc.agents["u-alice"] = provisionedAgent

	_, err = svc.RecreateRecord(context.Background(), "u-alice")
	if err != nil {
		t.Fatalf("Recreate() error = %v", err)
	}
	if got, want := strings.Join(callOrder, ","), "provision,delete,new"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
}

func TestRecreateDSHProvisionsBeforeDeleteAndNew(t *testing.T) {
	var callOrder []string
	svc, err := NewController(
		config.ModelConfig{},
		config.ServerConfig{
			ListenAddr:       "0.0.0.0:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		}, "manager-image:test", "",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindDSH,
			del: func(context.Context, agentruntime.Handle) error {
				callOrder = append(callOrder, "delete")
				return nil
			},
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				callOrder = append(callOrder, "provision")
				if got, want := req.RuntimeID, "rt-agent-alice"; got != want {
					t.Fatalf("Provision() runtime id = %q, want %q", got, want)
				}
				if got, want := req.Profile.BaseURL, "http://127.0.0.1:18080/api/v1/agents/agent-alice/llm"; got != want {
					t.Fatalf("Provision() profile base url = %q, want %q", got, want)
				}
				if got := req.Profile.APIKey; !strings.HasPrefix(got, "agent.") || got != req.Profile.Env["CSGCLAW_ACCESS_TOKEN"] {
					t.Fatal("DSH runtime must use its scoped Agent credential")
				}
				return nil
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				callOrder = append(callOrder, "new")
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "dsh-session-alice-new"}, nil
			},
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewController() error = %v", err)
	}
	svc.agents["agent-alice"] = Agent{
		ID:          "agent-alice",
		Name:        "alice",
		Role:        RoleWorker,
		RuntimeID:   "rt-agent-alice",
		RuntimeKind: RuntimeKindDSH,
		BoxID:       "dsh-session-alice-old",
		Status:      string(agentruntime.StateRunning),
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderAPI,
			BaseURL:         "https://api.example/v1",
			APIKey:          "api-key",
			ModelID:         "qwen3.7-plus",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	if _, err := svc.RecreateRecord(context.Background(), "agent-alice"); err != nil {
		t.Fatalf("Recreate() error = %v", err)
	}
	if got, want := strings.Join(callOrder, ","), "provision,delete,new"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
}

func TestCreateWorkerRequiresRuntimeKindWhenTemplateDoesNotProvideIt(t *testing.T) {
	SetTestHooks(
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return nil, nil },
		func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			return nil, sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	defer ResetTestHooks()

	svc, err := NewController(
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

func TestManagerMetadataDefaultsEmptyAvatar(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "agents.json")
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, _, _, avatar, _ := svc.managerMetadata()
	if avatar != assets.DefaultManagerAvatar {
		t.Fatalf("managerMetadata avatar = %q, want %q", avatar, assets.DefaultManagerAvatar)
	}

	svc.agents[ManagerUserID] = Agent{ID: ManagerUserID, Name: ManagerName, Role: RoleManager}
	_, _, _, avatar, _ = svc.managerMetadata()
	if avatar != assets.DefaultManagerAvatar {
		t.Fatalf("managerMetadata existing empty avatar = %q, want %q", avatar, assets.DefaultManagerAvatar)
	}

	svc.agents[ManagerUserID] = Agent{
		ID:     ManagerUserID,
		Name:   ManagerName,
		Role:   RoleManager,
		Avatar: assets.LegacyManagerAvatarSymbol,
	}
	_, _, _, avatar, _ = svc.managerMetadata()
	if avatar != assets.DefaultManagerAvatar {
		t.Fatalf("managerMetadata legacy avatar = %q, want %q", avatar, assets.DefaultManagerAvatar)
	}
}

func TestManagerMetadataPreservesCustomAvatar(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "agents.json")
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	customAvatar := "data:image/png;base64,custom"
	svc.agents[ManagerUserID] = Agent{ID: ManagerUserID, Name: ManagerName, Role: RoleManager, Avatar: customAvatar}

	_, _, _, avatar, _ := svc.managerMetadata()
	if avatar != customAvatar {
		t.Fatalf("managerMetadata avatar = %q, want custom avatar %q", avatar, customAvatar)
	}
}

func TestEnsureBootstrapManagerUsesCodexRuntimeWithoutConnectorTokenEnv(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	origLocateCodexCLI := locateCodexCLI
	locateCodexCLI = func() (string, error) { return "/usr/local/bin/codex", nil }
	t.Cleanup(func() {
		locateCodexCLI = origLocateCodexCLI
	})

	var gotSpec agentruntime.Spec
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "server-token"},
		"",
		filepath.Join(homeDir, "agents.json"),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				gotSpec = spec
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-manager-session"}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := svc.EnsureBootstrapManager(context.Background(), false); err != nil {
		t.Fatalf("EnsureBootstrapManager() error = %v", err)
	}
	manager, ok := svc.Agent(ManagerUserID)
	if !ok {
		t.Fatal("manager agent not saved")
	}
	if manager.ID != ManagerUserID || manager.Role != RoleManager {
		t.Fatalf("manager identity = %q/%q, want %q/%q", manager.ID, manager.Role, ManagerUserID, RoleManager)
	}
	if manager.RuntimeKind != RuntimeKindCodex || manager.RuntimeName != RuntimeNameCodex || manager.SandboxEnabled {
		t.Fatalf("manager runtime = kind %q name %q sandbox %t, want codex/codex/false", manager.RuntimeKind, manager.RuntimeName, manager.SandboxEnabled)
	}
	if manager.Image != "" {
		t.Fatalf("manager image = %q, want empty for codex runtime", manager.Image)
	}
	if manager.BoxID != "codex-manager-session" || manager.Status != string(agentruntime.StateRunning) {
		t.Fatalf("manager runtime state = %q/%q, want codex-manager-session/running", manager.BoxID, manager.Status)
	}
	if gotSpec.AgentID != ManagerUserID || gotSpec.Image != "" {
		t.Fatalf("runtime spec agent/image = %q/%q, want %q/empty", gotSpec.AgentID, gotSpec.Image, ManagerUserID)
	}
	if _, ok := gotSpec.Profile.Env["GITHUB_TOKEN"]; ok {
		t.Fatal("runtime profile env contains GITHUB_TOKEN")
	}
	if _, ok := manager.AgentProfile.Env["GITHUB_TOKEN"]; ok {
		t.Fatal("manager persisted profile env contains GITHUB_TOKEN")
	}
	if _, ok := manager.RuntimeOptions["GITHUB_TOKEN"]; ok {
		t.Fatal("manager runtime options contain GITHUB_TOKEN")
	}
}

func TestEnsureBootstrapManagerPreservesStoredManagerMCPServers(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	mcpServers := map[string]any{
		"context7": map[string]any{
			"command": "npx",
			"args":    []any{"-y", "@upstash/context7-mcp"},
		},
	}
	statePath := filepath.Join(homeDir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          ManagerUserID,
				Name:        ManagerName,
				RuntimeID:   runtimeIDForAgentID(ManagerUserID),
				RuntimeKind: RuntimeKindCodex,
				Role:        RoleManager,
				BoxID:       "codex-session-existing",
				MCPServers:  mcpServers,
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

	var provisionedMCPServers map[string]any
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "server-token"},
		"",
		statePath,
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				provisionedMCPServers = req.MCPServers
				return nil
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-manager-session"}, nil
			},
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := svc.EnsureBootstrapManager(context.Background(), false); err != nil {
		t.Fatalf("EnsureBootstrapManager() error = %v", err)
	}
	assertMCPServersHasServer(t, provisionedMCPServers, "context7")
	manager, ok := svc.Agent(ManagerUserID)
	if !ok {
		t.Fatal("manager agent not saved")
	}
	assertMCPServersHasServer(t, manager.MCPServers, "context7")

	reloaded, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "server-token"},
		"",
		statePath,
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService(reload) error = %v", err)
	}
	reloadedManager, ok := reloaded.Agent(ManagerUserID)
	if !ok {
		t.Fatal("reloaded manager agent not saved")
	}
	assertMCPServersHasServer(t, reloadedManager.MCPServers, "context7")
}

func TestEnsureBootstrapManagerPreservesStoredManagerMCPServersWhenCodexUnavailable(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	origLocateCodexCLI := locateCodexCLI
	locateCodexCLI = func() (string, error) { return "", fmt.Errorf("codex missing") }
	t.Cleanup(func() {
		locateCodexCLI = origLocateCodexCLI
	})

	mcpServers := map[string]any{
		"context7": map[string]any{"command": "npx"},
	}
	statePath := filepath.Join(homeDir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          ManagerUserID,
				Name:        ManagerName,
				RuntimeID:   runtimeIDForAgentID(ManagerUserID),
				RuntimeKind: RuntimeKindCodex,
				Role:        RoleManager,
				MCPServers:  mcpServers,
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

	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "server-token"},
		"",
		statePath,
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			new: func(context.Context, agentruntime.Spec) (agentruntime.Handle, error) {
				t.Fatal("codex runtime should not start when Codex CLI is missing")
				return agentruntime.Handle{}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := svc.EnsureBootstrapManager(context.Background(), false); err != nil {
		t.Fatalf("EnsureBootstrapManager() error = %v", err)
	}
	manager, ok := svc.Agent(ManagerUserID)
	if !ok {
		t.Fatal("manager agent not saved")
	}
	if manager.Status != StatusRuntimeUnavailable {
		t.Fatalf("manager status = %q, want %q", manager.Status, StatusRuntimeUnavailable)
	}
	assertMCPServersHasServer(t, manager.MCPServers, "context7")
}

func TestEnsureBootstrapManagerValidatesStoredManagerMCPServersBeforeRecreateDelete(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	statePath := filepath.Join(homeDir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          ManagerUserID,
				Name:        ManagerName,
				RuntimeID:   runtimeIDForAgentID(ManagerUserID),
				RuntimeKind: RuntimeKindCodex,
				Role:        RoleManager,
				BoxID:       "codex-session-existing",
				MCPServers: map[string]any{
					"invalid": "invalid",
				},
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
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

	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "server-token"},
		"",
		statePath,
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			del: func(context.Context, agentruntime.Handle) error {
				t.Fatal("manager runtime should not be deleted before stored MCP config validates")
				return nil
			},
			provision: func(context.Context, agentruntime.ProvisionRequest) error {
				t.Fatal("manager runtime should not be provisioned when stored MCP config is invalid")
				return nil
			},
			new: func(context.Context, agentruntime.Spec) (agentruntime.Handle, error) {
				t.Fatal("manager runtime should not be created when stored MCP config is invalid")
				return agentruntime.Handle{}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	err = svc.EnsureBootstrapManager(context.Background(), true)
	if err == nil || !strings.Contains(err.Error(), "mcpServers.invalid must be an object") {
		t.Fatalf("EnsureBootstrapManager() error = %v, want mcpServers validation error", err)
	}
}

func TestEnsureBootstrapManagerValidatesLegacyManagerMCPServersBeforeLegacyCleanup(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	ResetTestHooks()
	t.Cleanup(ResetTestHooks)
	testForceRemoveBoxHook = func(*Controller, context.Context, sandbox.Runtime, string) error {
		t.Fatal("legacy sandbox cleanup should not run before stored MCP config validates")
		return nil
	}

	statePath := filepath.Join(homeDir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          ManagerUserID,
				Name:        ManagerName,
				RuntimeID:   runtimeIDForAgentID(ManagerUserID),
				RuntimeKind: RuntimeKindCodex,
				Role:        RoleManager,
				BoxID:       "legacy-manager-box",
				MCPServers: map[string]any{
					"invalid": "invalid",
				},
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
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

	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "server-token"},
		"",
		statePath,
		WithSandboxProvider(fakeProvider{}),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			provision: func(context.Context, agentruntime.ProvisionRequest) error {
				t.Fatal("manager runtime should not be provisioned when stored MCP config is invalid")
				return nil
			},
			new: func(context.Context, agentruntime.Spec) (agentruntime.Handle, error) {
				t.Fatal("manager runtime should not be created when stored MCP config is invalid")
				return agentruntime.Handle{}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	err = svc.EnsureBootstrapManager(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "mcpServers.invalid must be an object") {
		t.Fatalf("EnsureBootstrapManager() error = %v, want mcpServers validation error", err)
	}
}

func TestEnsureBootstrapManagerPreservesLatestManagerMCPServersDuringStart(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	oldMCPServers := map[string]any{
		"old": map[string]any{"command": "npx"},
	}
	newMCPServers := map[string]any{
		"new": map[string]any{"command": "uvx"},
	}
	statePath := filepath.Join(homeDir, "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          ManagerUserID,
				Name:        ManagerName,
				RuntimeID:   runtimeIDForAgentID(ManagerUserID),
				RuntimeKind: RuntimeKindCodex,
				Role:        RoleManager,
				MCPServers:  oldMCPServers,
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

	newStarted := make(chan struct{})
	releaseNew := make(chan struct{})
	var reconcileHandles []string
	var reconciledConfigs []map[string]any
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "server-token"},
		"",
		statePath,
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				close(newStarted)
				<-releaseNew
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "codex-manager-session"}, nil
			},
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
			},
			mcpReconcile: func(_ context.Context, h agentruntime.Handle, change agentruntime.MCPServersChange) error {
				reconcileHandles = append(reconcileHandles, h.HandleID)
				reconciledConfigs = append(reconciledConfigs, utils.CloneAnyMap(change.Current.Servers))
				return nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.EnsureBootstrapManager(context.Background(), false)
	}()

	<-newStarted
	updateConfig := utils.CloneAnyMap(newMCPServers)
	updateErrCh := make(chan error, 1)
	go func() {
		_, updateErr := svc.UpdateRecord(context.Background(), ManagerUserID, UpdateRequest{
			MCPServers:    &updateConfig,
			MCPServersSet: true,
		})
		updateErrCh <- updateErr
	}()
	select {
	case updateErr := <-updateErrCh:
		t.Fatalf("Update(manager MCPServers) returned before start drained: %v", updateErr)
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseNew)

	if err := <-errCh; err != nil {
		t.Fatalf("EnsureBootstrapManager() error = %v", err)
	}
	if err := <-updateErrCh; err != nil {
		t.Fatalf("Update(manager MCPServers) error = %v", err)
	}
	manager, ok := svc.Agent(ManagerUserID)
	if !ok {
		t.Fatal("manager agent not saved")
	}
	assertMCPServersHasServer(t, manager.MCPServers, "new")
	if manager.MCPServers["old"] != nil {
		t.Fatalf("manager MCPServers = %#v, want latest config without old server", manager.MCPServers)
	}
	if len(reconciledConfigs) < 1 {
		t.Fatal("MCP reconcile was not called")
	}
	if got := reconcileHandles[len(reconcileHandles)-1]; got != "codex-manager-session" {
		t.Fatalf("last MCP reconcile handle = %q, want codex-manager-session", got)
	}
	assertMCPServersHasServer(t, reconciledConfigs[len(reconciledConfigs)-1], "new")
}

func TestEnsureBootstrapManagerRecordsMissingCodexCLIWithoutRuntimeStart(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	origLocateCodexCLI := locateCodexCLI
	locateCodexCLI = func() (string, error) { return "", fmt.Errorf("codex missing") }
	t.Cleanup(func() {
		locateCodexCLI = origLocateCodexCLI
	})

	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "server-token"},
		"",
		filepath.Join(homeDir, "agents.json"),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			new: func(context.Context, agentruntime.Spec) (agentruntime.Handle, error) {
				t.Fatal("codex runtime should not start when Codex CLI is missing")
				return agentruntime.Handle{}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := svc.EnsureBootstrapManager(context.Background(), false); err != nil {
		t.Fatalf("EnsureBootstrapManager() error = %v", err)
	}
	manager, ok := svc.Agent(ManagerUserID)
	if !ok {
		t.Fatal("manager agent not saved")
	}
	if manager.RuntimeKind != RuntimeKindCodex || manager.RuntimeName != RuntimeNameCodex || manager.SandboxEnabled {
		t.Fatalf("manager runtime = kind %q name %q sandbox %t, want codex/codex/false", manager.RuntimeKind, manager.RuntimeName, manager.SandboxEnabled)
	}
	if manager.Status != "runtime_unavailable" {
		t.Fatalf("manager status = %q, want runtime_unavailable", manager.Status)
	}
	if manager.BoxID != "" {
		t.Fatalf("manager BoxID = %q, want empty when runtime is unavailable", manager.BoxID)
	}
}

func TestEnsureManagerRejectsNonCodexRuntimeOverride(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"",
		filepath.Join(homeDir, "agents.json"),
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.ensureManager(context.Background(), true, RuntimeKindDSH)
	if err == nil {
		t.Fatal("ensureManager() error = nil, want non-codex runtime rejection")
	}
	if !strings.Contains(err.Error(), "manager runtime is fixed to codex") {
		t.Fatalf("ensureManager() error = %v, want fixed codex runtime rejection", err)
	}
}

func TestCreateManagerRejectsInlineMCPServers(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"",
		filepath.Join(homeDir, "agents.json"),
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateRecord(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:   ManagerUserID,
			Name: ManagerName,
			MCPServers: map[string]any{
				"context7": map[string]any{"command": "npx"},
			},
			MCPServersSet: true,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "manager mcpServers must be updated through the MCP servers endpoint") {
		t.Fatalf("Create(manager with mcpServers) error = %v, want manager mcpServers rejection", err)
	}
}

func TestCreateReplaceManagerRejectsInlineMCPServers(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"",
		filepath.Join(homeDir, "agents.json"),
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:          ManagerUserID,
		Name:        ManagerName,
		RuntimeID:   runtimeIDForAgentID(ManagerUserID),
		RuntimeKind: RuntimeKindCodex,
		RuntimeName: RuntimeNameCodex,
		Role:        RoleManager,
	}

	_, err = svc.CreateRecord(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID: ManagerUserID,
			MCPServers: map[string]any{
				"context7": map[string]any{"command": "npx"},
			},
			MCPServersSet: true,
		},
		Replace:   true,
		FieldMask: []string{"mcpServers"},
	})
	if err == nil || !strings.Contains(err.Error(), "manager mcpServers must be updated through the MCP servers endpoint") {
		t.Fatalf("Create(--replace manager with mcpServers) error = %v, want manager mcpServers rejection", err)
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
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Controller, _ context.Context, _ sandbox.Runtime, image, name, botID string, profile AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			created = true
			if image != "codex:latest" || name != ManagerName || botID != ManagerUserID {
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
	testGetBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
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

	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{ListenAddr: ":18080", AccessToken: "token"}, "codex:latest", "")
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

func TestCreateReplaceRequiresExistingAgent(t *testing.T) {
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateRecord(context.Background(), CreateRequest{
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

func TestUpdateInstructionsPersistsToState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "agents.json")
	svc, err := NewController(
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
	updated, err := svc.UpdateRecord(context.Background(), "u-alice", UpdateRequest{Instructions: &nextInstructions})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Instructions != nextInstructions {
		t.Fatalf("Update().Instructions = %q, want %q", updated.Instructions, nextInstructions)
	}
	if _, err := svc.SetDesiredState("u-alice", DesiredStateRunning, false); err != nil {
		t.Fatalf("SetDesiredState() error = %v", err)
	}

	reloaded, err := NewController(
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
	if got.DesiredState != DesiredStateRunning || got.Status != string(agentruntime.StateStopped) {
		t.Fatalf("reloaded desired/observed state = %q/%q, want running/stopped", got.DesiredState, got.Status)
	}
}

func TestUpdateManagerNamePersistsToRootState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), config.AppDirName, config.StateFileName)
	svc, err := NewController(
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
	updated, err := svc.UpdateRecord(context.Background(), ManagerUserID, UpdateRequest{Name: &nextName})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != nextName {
		t.Fatalf("Update().Name = %q, want %q", updated.Name, nextName)
	}

	reloaded, err := NewController(
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
	t.Skip("manager is Codex-only; sandbox manager image replacement is obsolete")
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	var gotImages []string
	SetTestHooks(
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Controller, _ context.Context, _ sandbox.Runtime, image, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			gotImages = append(gotImages, image)
			return &fakeInstance{}, sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	testGetBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testForceRemoveBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) error {
		return nil
	}
	defer ResetTestHooks()

	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:1", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if _, err := svc.CreateRecord(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:   ManagerUserID,
			Name: ManagerName,
		},
	}); err != nil {
		t.Fatalf("seed Create() error = %v", err)
	}
	replaced, err := svc.CreateRecord(context.Background(), CreateRequest{
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
	t.Skip("manager is Codex-only; sandbox manager replacement is obsolete")
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	var gotNames []string
	SetTestHooks(
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			gotNames = append(gotNames, name)
			return &fakeInstance{}, sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	testGetBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testForceRemoveBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) error {
		return nil
	}
	defer ResetTestHooks()

	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:1", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if _, err := svc.CreateRecord(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:   ManagerUserID,
			Name: ManagerName,
		},
	}); err != nil {
		t.Fatalf("seed Create() error = %v", err)
	}

	nextName := "管理员"
	if _, err := svc.UpdateRecord(context.Background(), ManagerUserID, UpdateRequest{Name: &nextName}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	replaced, err := svc.CreateRecord(context.Background(), CreateRequest{
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
	t.Skip("manager is Codex-only; sandbox manager replacement is obsolete")
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	SetTestHooks(
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Controller, _ context.Context, _ sandbox.Runtime, image, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
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
	testGetBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testForceRemoveBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) error {
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
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:1", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:              ManagerUserID,
		Name:            ManagerName,
		RuntimeID:       runtimeIDForAgentID(ManagerUserID),
		RuntimeKind:     RuntimeKindCodex,
		Image:           "manager-image:1",
		BoxID:           "box-manager-old",
		Role:            RoleManager,
		Status:          string(sandbox.StateRunning),
		Profile:         profileSelector(profile),
		AgentProfile:    profile,
		ProfileComplete: true,
		CreatedAt:       time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
	}

	replaced, err := svc.CreateRecord(context.Background(), CreateRequest{
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
	t.Skip("manager is Codex-only; sandbox manager workspace reprovisioning is obsolete")
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

	svc, err := NewController(
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
	if _, err := svc.CreateRecord(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:   ManagerUserID,
			Name: ManagerName,
		},
	}); err != nil {
		t.Fatalf("seed Create() error = %v", err)
	}

	replaced, err := svc.CreateRecord(context.Background(), CreateRequest{
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
	workspaceRoot, err := testBuiltinWorkspaceRoot(ManagerName, RuntimeKindCodex)
	if err != nil {
		t.Fatalf("agentWorkspaceRoot() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "AGENTS.md")); err != nil {
		t.Fatalf("manager workspace was not reprovisioned after replace: %v", err)
	}
}

func TestCreateReplaceManagerWithoutRequestedImageUsesManagerDefault(t *testing.T) {
	t.Skip("manager is Codex-only; sandbox manager default image replacement is obsolete")
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	var gotImages []string
	SetTestHooks(
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Controller, _ context.Context, _ sandbox.Runtime, image, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			gotImages = append(gotImages, image)
			return &fakeInstance{}, sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	testGetBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testForceRemoveBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) error {
		return nil
	}
	defer ResetTestHooks()

	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:1", "")
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

	replaced, err := svc.CreateRecord(context.Background(), CreateRequest{
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
	t.Skip("manager is Codex-only; sandbox manager runtime switching is obsolete")
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	var gotImages []string
	SetTestHooks(
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Controller, _ context.Context, _ sandbox.Runtime, image, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			gotImages = append(gotImages, image)
			return &fakeInstance{}, sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	testGetBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testForceRemoveBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) error {
		return nil
	}
	defer ResetTestHooks()

	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if _, err := svc.CreateRecord(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:   ManagerUserID,
			Name: ManagerName,
		},
	}); err != nil {
		t.Fatalf("seed Create() error = %v", err)
	}

	replaced, err := svc.CreateRecord(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:          ManagerUserID,
			Name:        ManagerName,
			RuntimeKind: RuntimeKindDSH,
		},
		Replace: true,
	})
	if err == nil || !strings.Contains(err.Error(), `image is required when changing gateway runtime_kind to "dsh"`) {
		t.Fatalf("Create() replace error = %v, want missing image error", err)
	}
	if replaced.ID != "" || replaced.Name != "" || replaced.RuntimeKind != "" || replaced.Image != "" {
		t.Fatalf("Create() replaced = %+v, want zero-value agent fields on error", replaced)
	}

	if len(gotImages) != 1 {
		t.Fatalf("createGatewayBox() calls = %d, want 1", len(gotImages))
	}
}

func TestCreateReplaceManagerSwitchesRuntimeKindUsesEmbeddedRuntimeImage(t *testing.T) {
	t.Skip("manager is Codex-only; sandbox manager runtime switching is obsolete")
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	var gotImages []string
	SetTestHooks(
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Controller, _ context.Context, _ sandbox.Runtime, image, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			gotImages = append(gotImages, image)
			return &fakeInstance{}, sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	testGetBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testForceRemoveBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) error {
		return nil
	}
	defer ResetTestHooks()

	hubSvc, err := hub.NewService(config.HubConfig{}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatalf("hub.NewService() error = %v", err)
	}
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"codex-manager:old",
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
	if _, err := svc.CreateRecord(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:   ManagerUserID,
			Name: ManagerName,
		},
	}); err != nil {
		t.Fatalf("seed Create() error = %v", err)
	}
	avatar := "avatar/cartoon-6.png"
	if _, err := svc.UpdateRecord(context.Background(), ManagerUserID, UpdateRequest{Avatar: &avatar}); err != nil {
		t.Fatalf("Update() avatar error = %v", err)
	}

	const requestedImage = "dsh-manager:requested"
	runtimeTemplate, err := hubSvc.Get(context.Background(), "builtin.manager-codex")
	if err != nil {
		t.Fatalf("Get(dsh-manager) error = %v", err)
	}
	wantImage := runtimeTemplate.Image
	replaced, err := svc.CreateRecord(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:          ManagerUserID,
			Name:        ManagerName,
			Image:       requestedImage,
			RuntimeKind: RuntimeKindDSH,
		},
		Replace: true,
	})
	if err != nil {
		t.Fatalf("Create() replace error = %v", err)
	}
	if got, want := replaced.RuntimeKind, RuntimeKindDSH; got != want {
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
	t.Skip("manager is Codex-only; sandbox manager image upgrade is obsolete")
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "codex-manager", hub.Template{
		ID:          "codex-manager",
		Name:        "codex-manager",
		Description: "codex manager",
		Role:        hub.TemplateRoleManager,
		RuntimeKind: RuntimeNameCodex,
		Version:     "0.2.0",
		Image:       "registry.example/codex-manager:0.2.0",
	})

	var gotImages []string
	SetTestHooks(
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Controller, _ context.Context, _ sandbox.Runtime, image, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			gotImages = append(gotImages, image)
			return &fakeInstance{}, sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	testGetBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testForceRemoveBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) error {
		return nil
	}
	defer ResetTestHooks()

	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"registry.example/codex-manager:0.2.0",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultManagerTemplate: "local/codex-manager"}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents[ManagerUserID] = Agent{
		ID:          ManagerUserID,
		Name:        ManagerName,
		RuntimeID:   runtimeIDForAgentID(ManagerUserID),
		RuntimeKind: RuntimeKindCodex,
		Image:       "registry.example/codex-manager:0.1.0",
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

	replaced, err := svc.CreateRecord(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:          ManagerUserID,
			Name:        ManagerName,
			Image:       "registry.example/codex-manager:0.1.0",
			RuntimeKind: RuntimeKindCodex,
		},
		Replace: true,
	})
	if err != nil {
		t.Fatalf("Create() replace error = %v", err)
	}
	if got, want := replaced.Image, "registry.example/codex-manager:0.2.0"; got != want {
		t.Fatalf("Create() replace image = %q, want %q", got, want)
	}
	if len(gotImages) != 1 {
		t.Fatalf("createGatewayBox() calls = %d, want 1", len(gotImages))
	}
	if got, want := gotImages[0], "registry.example/codex-manager:0.2.0"; got != want {
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

	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		nil,
	)
	defer ResetTestHooks()

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	statePath := filepath.Join(dir, "agents.json")
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	svc.agents[ManagerUserID] = Agent{RuntimeKind: RuntimeKindCodex,
		ID:        ManagerUserID,
		Name:      ManagerName,
		Role:      RoleManager,
		CreatedAt: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
	}
	if err := svc.saveLocked(); err != nil {
		t.Fatalf("saveLocked() error = %v", err)
	}

	if err := svc.DeleteRecord(context.Background(), ManagerUserID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := svc.Agent(ManagerUserID); ok {
		t.Fatal("Agent() ok = true, want false after delete")
	}

	reloaded, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() reload error = %v", err)
	}
	if _, ok := reloaded.Agent(ManagerUserID); ok {
		t.Fatal("reloaded Agent() ok = true, want false after delete")
	}
}

func TestDeleteRemovesAgentFromState(t *testing.T) {
	SetTestHooks(
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		nil,
	)
	defer ResetTestHooks()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	svc.agents["u-alice"] = Agent{RuntimeKind: RuntimeKindCodex,
		ID:        "u-alice",
		Name:      "alice",
		Role:      RoleWorker,
		Status:    "running",
		CreatedAt: time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
	}
	if err := svc.saveLocked(); err != nil {
		t.Fatalf("saveLocked() error = %v", err)
	}

	if err := svc.DeleteRecord(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := svc.Agent("u-alice"); ok {
		t.Fatal("Agent() ok = true, want false after delete")
	}

	reloaded, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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

	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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
		func(_ *Controller, _ string) (sandbox.Runtime, error) {
			return nil, fmt.Errorf("runtime lock")
		},
		nil,
	)
	defer ResetTestHooks()

	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
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

	got := svc.ListRecords()
	if len(got) != 1 {
		t.Fatalf("List() len = %d, want 1", len(got))
	}
	if got[0].Status != string(sandbox.StateRunning) {
		t.Fatalf("List()[0].Status = %q, want running", got[0].Status)
	}
}

func TestListContextRefreshesRuntimeStatusWithBoundedConcurrency(t *testing.T) {
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseAll()

	svc, err := NewController(
		config.ModelConfig{},
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			info: func(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
				entered <- struct{}{}
				<-release
				return agentruntime.Info{State: agentruntime.StateRunning}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	for _, id := range []string{"agent-alice", "agent-bob", "agent-carol"} {
		svc.agents[id] = Agent{
			ID:          id,
			Name:        id,
			Role:        RoleWorker,
			RuntimeKind: RuntimeKindCodex,
			Status:      string(sandbox.StateCreated),
			CreatedAt:   time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC),
		}
	}

	result := make(chan []Agent, 1)
	go func() { result <- svc.ListRecordsContext(context.Background()) }()
	for range 3 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("ListContext() did not start three independent runtime probes")
		}
	}
	releaseAll()
	got := <-result
	for _, item := range got {
		if item.Status != string(sandbox.StateRunning) {
			t.Fatalf("ListContext() status for %q = %q, want running", item.ID, item.Status)
		}
	}
}

func TestRecordRuntimeAvailabilityDiscardsObservationAfterAgentDeletion(t *testing.T) {
	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	agent := Agent{ID: "agent-deleted", BoxID: "box-deleted"}
	svc.recordRuntimeAvailability(agent, RuntimeAvailability{State: RuntimeAvailabilityReady})
	if _, ok := svc.availability[canonicalAgentID(agent.ID)]; ok {
		t.Fatal("recordRuntimeAvailability() restored an observation for a deleted agent")
	}
}

func TestRuntimeAvailabilityFromLifecycleErrorClassifiesSandboxBusy(t *testing.T) {
	availability, ok := runtimeAvailabilityFromLifecycleError(fmt.Errorf("inspect runtime: %w", sandbox.ErrBusy))
	if !ok {
		t.Fatal("runtimeAvailabilityFromLifecycleError() ok = false, want true")
	}
	if availability.State != RuntimeAvailabilityUnknown || availability.Reason != "runtime_busy" {
		t.Fatalf("runtimeAvailabilityFromLifecycleError() = %#v, want unknown/runtime_busy", availability)
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
				RuntimeKind: RuntimeKindCodex,
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
		func(_ *Controller, _ string) (sandbox.Runtime, error) {
			return nil, fmt.Errorf("runtime lock")
		},
		nil,
	)
	defer ResetTestHooks()

	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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
				RuntimeKind: RuntimeKindCodex,
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

	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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
	if got.RuntimeKind != RuntimeKindCodex {
		t.Fatalf("agentSnapshot().RuntimeKind = %q, want %q", got.RuntimeKind, RuntimeKindCodex)
	}
	rt, ok := svc.runtimeRecords[got.RuntimeID]
	if !ok {
		t.Fatalf("runtimeRecords[%q] missing", got.RuntimeID)
	}
	if rt.Kind != RuntimeKindCodex {
		t.Fatalf("runtime kind = %q, want %q", rt.Kind, RuntimeKindCodex)
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
	if persisted.Agents[0].Runtime.Kind != RuntimeKindCodex {
		t.Fatalf("saved agent runtime.kind = %q, want %q", persisted.Agents[0].Runtime.Kind, RuntimeKindCodex)
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

	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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

	_, err = NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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

	_, err = NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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
				RuntimeKind: RuntimeKindCodex,
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

	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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
				RuntimeKind: RuntimeKindCodex,
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

	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		nil,
	)
	defer ResetTestHooks()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{RuntimeKind: RuntimeKindCodex,
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

	if err := svc.DeleteRecord(context.Background(), "u-alice"); err != nil {
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

func TestDeleteReturnsAgentHomeRemovalError(t *testing.T) {
	rt := &fakeRuntime{}
	SetTestHooks(func(_ *Controller, _ string) (sandbox.Runtime, error) { return rt, nil }, nil)
	defer ResetTestHooks()
	testForceRemoveBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) error {
		return nil
	}
	defer func() {
		testForceRemoveBoxHook = nil
	}()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{RuntimeKind: RuntimeKindCodex,
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
			return &os.PathError{Op: "unlinkat", Path: filepath.Join(agentHome, ".codex", "home", "logs_2.sqlite"), Err: errors.New("The process cannot access the file because it is being used by another process.")}
		}
		return os.RemoveAll(path)
	}
	defer func() {
		osRemoveAll = origRemoveAll
	}()

	err = svc.DeleteRecord(context.Background(), "u-alice")
	if err == nil || !strings.Contains(err.Error(), "being used by another process") {
		t.Fatalf("Delete() error = %v, want locked file error", err)
	}
	if removeCalls != 1 {
		t.Fatalf("osRemoveAll() calls = %d, want 1", removeCalls)
	}
}

func TestDeleteRetriesAgentHomeRemovalWhenDirectoryIsNotEmpty(t *testing.T) {
	SetTestHooks(func(_ *Controller, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil }, nil)
	defer ResetTestHooks()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{RuntimeKind: RuntimeKindCodex, ID: "u-alice", Name: "alice", Role: RoleWorker}
	agentHome, err := agentHomeDir("alice")
	if err != nil {
		t.Fatalf("agentHomeDir() error = %v", err)
	}
	if err := os.MkdirAll(agentHome, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(agentHome) error = %v", err)
	}

	origRemoveAll := osRemoveAll
	removeCalls := 0
	osRemoveAll = func(path string) error {
		removeCalls++
		if path == agentHome && removeCalls < 3 {
			return &os.PathError{Op: "unlinkat", Path: filepath.Join(agentHome, "boxlite", "images"), Err: syscall.ENOTEMPTY}
		}
		return os.RemoveAll(path)
	}
	defer func() { osRemoveAll = origRemoveAll }()

	if err := svc.DeleteRecord(context.Background(), "u-alice"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if removeCalls != 3 {
		t.Fatalf("osRemoveAll() calls = %d, want 3", removeCalls)
	}
}

func TestCreateWorkerUsesDefaultProfileSnapshotForGatewayRuntime(t *testing.T) {
	SetTestHooks(
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return nil, nil },
		func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			return nil, sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	defer ResetTestHooks()

	svc, err := NewControllerWithLLM(config.LLMConfig{
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
		RuntimeKind: RuntimeKindCodex,
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

func TestStartProvisionsRuntimeBeforeStart(t *testing.T) {
	var callOrder []string
	svc, err := NewController(
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

func TestStartUsesRuntimeStartValidationInsteadOfOnlineConfigProbe(t *testing.T) {
	configProbeCalls := 0
	startValidationCalls := 0
	startCalls := 0
	rt := fakeStartConfigAgentRuntime{fakeAgentRuntime: fakeAgentRuntime{
		kind: RuntimeKindCodex,
		validate: func(context.Context, agentruntime.RuntimeConfigSnapshot) error {
			configProbeCalls++
			return errors.New("provider temporarily unavailable")
		},
		start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
			startCalls++
			return agentruntime.StateRunning, nil
		},
		info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
			return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
		},
	}, startValidate: func(_ context.Context, current agentruntime.RuntimeConfigSnapshot) error {
		startValidationCalls++
		if current.Profile.ModelID != "glm-5.1" {
			t.Fatalf("start validation model = %q, want glm-5.1", current.Profile.ModelID)
		}
		return nil
	}}
	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "", WithRuntime(rt))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["agent-alice"] = Agent{
		ID:              "agent-alice",
		Name:            "alice",
		Role:            RoleWorker,
		RuntimeID:       "rt-agent-alice",
		RuntimeKind:     RuntimeKindCodex,
		Status:          string(agentruntime.StateStopped),
		AgentProfile:    AgentProfile{Name: "alice", Provider: ProviderAPI, BaseURL: "https://api.example/v1", APIKey: "api-key", ModelID: "glm-5.1", ProfileComplete: true},
		ProfileComplete: true,
	}

	got, err := svc.Start(context.Background(), "agent-alice")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got.Status != string(agentruntime.StateRunning) {
		t.Fatalf("Start() status = %q, want running", got.Status)
	}
	if configProbeCalls != 0 {
		t.Fatalf("online config probe calls = %d, want 0", configProbeCalls)
	}
	if startValidationCalls != 1 || startCalls != 1 {
		t.Fatalf("start validation calls = %d, runtime start calls = %d, want 1 each", startValidationCalls, startCalls)
	}
}

func TestStartSkipsStartBoxWhenAlreadyRunning(t *testing.T) {
	rt := &fakeRuntime{}
	SetTestHooks(func(_ *Controller, _ string) (sandbox.Runtime, error) { return rt, nil }, nil)
	defer ResetTestHooks()

	testGetBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		if idOrName == "alice" {
			return &fakeInstance{}, nil
		}
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	var startCalls int
	testStartBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Instance) error {
		startCalls++
		return nil
	}
	testBoxInfoHook = func(_ *Controller, _ context.Context, _ sandbox.Instance) (sandbox.Info, error) {
		return sandbox.Info{ID: "box-new", State: sandbox.StateRunning}, nil
	}
	defer func() {
		testGetBoxHook = nil
		testStartBoxHook = nil
		testBoxInfoHook = nil
	}()

	dir := t.TempDir()
	statePath := filepath.Join(dir, "agents.json")
	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:          "u-alice",
		Name:        "alice",
		BoxID:       "box-stale",
		Role:        RoleWorker,
		RuntimeKind: RuntimeKindCodex,
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

func TestCreateWorkerFromTemplateAppliesDefaultsAndOverlaysWorkspace(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubService(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: RuntimeNameCodex,
		Image:       "worker-image:1",
	})

	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(hubSvc),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateRecord(context.Background(), CreateRequest{
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
	if got.Image != "" {
		t.Fatalf("Image = %q, want %q", got.Image, "worker-image:1")
	}
	if got.RuntimeKind != RuntimeKindCodex {
		t.Fatalf("RuntimeKind = %q, want %q", got.RuntimeKind, RuntimeKindCodex)
	}
	if server, ok := got.MCPServers["template-docs"].(map[string]any); !ok || server["command"] != "npx" {
		t.Fatalf("MCPServers = %#v, want template-docs from mcps/mcp.json", got.MCPServers)
	}

	workspaceRoot, err := testBuiltinWorkspaceRoot(got.ID, RuntimeKindCodex)
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
	if _, err := os.Stat(filepath.Join(workspaceRoot, "AGENTS.md")); err != nil {
		t.Fatalf("AGENT.md missing after template overlay: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "skills", "custom", "SKILL.md")); err != nil {
		t.Fatalf("template skill missing after overlay: %v", err)
	}
}

func TestCreateWorkerFromBuiltinDSHTemplateOverlaysDefaultWorkspace(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	hubSvc, err := hub.NewService(config.HubConfig{}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatalf("hub.NewService() error = %v", err)
	}
	overlayTarget := t.TempDir()
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{ListenAddr: "127.0.0.1:18080", AccessToken: "shared-token"},
		"manager-image:test",
		"",
		WithHubService(hubSvc),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindDSH,
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				if !strings.Contains(req.TemplateInstructions, "CSGClaw DSH Worker") {
					t.Fatalf("Provision().TemplateInstructions = %q", req.TemplateInstructions)
				}
				return sandboxgateway.OverlayWorkspaceTree(req.WorkspaceOverlay, overlayTarget)
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "dsh-agent"}, nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewController() error = %v", err)
	}

	got, err := svc.CreateRecord(context.Background(), CreateRequest{Spec: CreateAgentSpec{
		Name:         "dsh-worker",
		FromTemplate: "builtin.dsh-worker",
		AgentProfile: AgentProfile{
			Name: "dsh-worker", Provider: ProviderOpenCSG, ModelID: "deepseek-chat", ProfileComplete: true,
		},
	}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.RuntimeKind != RuntimeKindDSH || got.BoxID != "dsh-agent" {
		t.Fatalf("Create() = %+v", got)
	}
	for _, name := range []string{"HEARTBEAT.md", "IDENTITY.md", "SOUL.md", "TOOLS.md", "USER.md"} {
		if _, err := os.Stat(filepath.Join(overlayTarget, name)); err != nil {
			t.Fatalf("template overlay %s error = %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(overlayTarget, "skills", "agent-teams", "SKILL.md")); err != nil {
		t.Fatalf("template overlay agent-teams skill error = %v", err)
	}
}

type templateAvailabilityTestProber struct{}

func (templateAvailabilityTestProber) Probe(context.Context, string, map[string]any) (mcp.ProbeResult, error) {
	return mcp.ProbeResult{}, errors.New("forbidden")
}

func TestResolveTemplateCreateSpecSkipsUnavailableManagedMCP(t *testing.T) {
	hubSvc := mustNewLocalTemplateHubServiceWithMCP(t, "remote-worker", hub.Template{
		ID: "remote-worker", Name: "remote-worker", Role: hub.TemplateRoleWorker, RuntimeKind: RuntimeNameCodex,
	}, map[string]any{
		"manual": map[string]any{"command": "manual-mcp"},
		"remote": mcp.RemoteServer{ID: "remote-1", Name: "Remote Docs", URL: "https://remote.example.test/mcp"}.Config(),
	})
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:1", "", WithHubService(hubSvc))
	if err != nil {
		t.Fatalf("NewController() error = %v", err)
	}
	checker := mcp.NewService(mcp.WithServerProber(templateAvailabilityTestProber{}))
	skipped := []mcp.SkippedTemplateResource{}
	ctx := WithTemplateMCPAvailability(context.Background(), checker, &skipped)

	resolved, cleanup, err := svc.resolveTemplateCreateSpec(ctx, CreateAgentSpec{Name: "alice", FromTemplate: "local.remote-worker"})
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatalf("resolveTemplateCreateSpec() error = %v", err)
	}
	if _, ok := resolved.MCPServers["remote"]; ok {
		t.Fatalf("MCPServers retained unavailable remote: %#v", resolved.MCPServers)
	}
	if _, ok := resolved.MCPServers["manual"]; !ok {
		t.Fatalf("MCPServers removed manual server: %#v", resolved.MCPServers)
	}
	if len(skipped) != 1 || skipped[0].Type != mcp.TemplateResourceMCP || skipped[0].Name != "Remote Docs" {
		t.Fatalf("skipped resources = %#v", skipped)
	}
}

func TestResolveCodexTemplateCreateSpecSeparatesBaseFromProfileInstructions(t *testing.T) {
	hubSvc := mustNewLocalTemplateHubService(t, "codex-worker", hub.Template{
		ID:             "codex-worker",
		Name:           "codex-worker",
		Role:           hub.TemplateRoleWorker,
		RuntimeKind:    RuntimeNameCodex,
		RuntimeOptions: map[string]any{"execution_mode": "read_only"},
	})
	templateInstructions := "# Template Base\n\nKeep this content.\n\n" +
		runtimeinstructions.RenderAgentsInstructionsBlock("Prefer targeted tests.")
	if err := hubSvc.WriteWorkspaceFile(context.Background(), "local.codex-worker", "instructions/AGENTS.md", templateInstructions); err != nil {
		t.Fatalf("WriteWorkspaceFile(AGENTS.md) error = %v", err)
	}
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:1", "", WithHubService(hubSvc))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	resolved, cleanup, err := svc.resolveTemplateCreateSpec(context.Background(), CreateAgentSpec{
		Name:         "alice",
		FromTemplate: "local.codex-worker",
	})
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatalf("resolveTemplateCreateSpec() error = %v", err)
	}
	if got, want := resolved.Instructions, "Prefer targeted tests."; got != want {
		t.Fatalf("Instructions = %q, want template user instructions %q", got, want)
	}
	if got, want := resolved.RuntimeOptions["execution_mode"], "read_only"; got != want {
		t.Fatalf("RuntimeOptions[execution_mode] = %v, want %q", got, want)
	}
	if got := resolved.TemplateInstructions; got != templateInstructions {
		t.Fatalf("TemplateInstructions = %q, want complete template document %q", got, templateInstructions)
	}
	if _, err := os.Stat(filepath.Join(resolved.FromTemplate, "AGENTS.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace AGENTS.md stat error = %v, want template instructions separated from overlay", err)
	}
	if !resolved.TemplateMemorySet || resolved.TemplateMemory != "# Template memory\n" {
		t.Fatalf("TemplateMemory = %q (set=%v), want restored template memory", resolved.TemplateMemory, resolved.TemplateMemorySet)
	}
	if _, err := os.Stat(filepath.Join(resolved.FromTemplate, ".csgclaw-template-memory")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staged template memory leaked into workspace overlay, stat error = %v", err)
	}
}

func TestApplyTemplateDefaultsMergesExplicitRuntimeOptions(t *testing.T) {
	item := hub.Template{RuntimeOptions: map[string]any{"execution_mode": "read_only"}}
	got := applyTemplateDefaults(CreateAgentSpec{
		RuntimeOptions: map[string]any{"local_workspace_dir": "/tmp/project"},
	}, item)
	if got.RuntimeOptions["execution_mode"] != "read_only" || got.RuntimeOptions["local_workspace_dir"] != "/tmp/project" {
		t.Fatalf("merged RuntimeOptions = %#v", got.RuntimeOptions)
	}

	overridden := applyTemplateDefaults(CreateAgentSpec{
		RuntimeOptions: map[string]any{"execution_mode": "standard"},
	}, item)
	if overridden.RuntimeOptions["execution_mode"] != "standard" {
		t.Fatalf("explicit execution_mode = %v, want standard", overridden.RuntimeOptions["execution_mode"])
	}
}

func TestResolveDSHTemplateMemory(t *testing.T) {
	hubSvc := mustNewLocalTemplateHubService(t, "dsh-worker", hub.Template{
		ID: "dsh-worker", Name: "dsh-worker", Role: hub.TemplateRoleWorker, RuntimeKind: RuntimeKindDSH,
		RuntimeOptions: map[string]any{"permission_mode": "read-only", "memory_mode": "enabled"},
	})
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:1", "", WithHubService(hubSvc), WithRuntime(runtimedsh.New(runtimedsh.Dependencies{})))
	if err != nil {
		t.Fatal(err)
	}
	if !svc.SupportsMemory(RuntimeKindDSH) {
		t.Fatal("DSH memory capability is not exposed")
	}
	for _, enabled := range []bool{true, false} {
		mode := "enabled"
		if !enabled {
			mode = "disabled"
		}
		resolved, cleanup, err := svc.resolveTemplateCreateSpec(context.Background(), CreateAgentSpec{Name: "alice", FromTemplate: "local.dsh-worker", RuntimeOptions: map[string]any{"memory_mode": mode}})
		if cleanup != nil {
			defer cleanup()
		}
		if err != nil {
			t.Fatal(err)
		}
		if resolved.TemplateMemorySet != enabled || (enabled && resolved.TemplateMemory != "# Template memory\n") {
			t.Fatalf("memory seed = %q, set=%v", resolved.TemplateMemory, resolved.TemplateMemorySet)
		}
		if resolved.RuntimeOptions["memory_mode"] != mode || resolved.RuntimeOptions["permission_mode"] != "read-only" {
			t.Fatalf("runtime options = %#v", resolved.RuntimeOptions)
		}
		if _, err := os.Stat(filepath.Join(resolved.FromTemplate, ".csgclaw-template-memory")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("memory staging leaked into project: %v", err)
		}
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

func TestCreateFromTemplateUsesRequestHubService(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	requestHub := mustNewLocalTemplateHubService(t, "request-worker", hub.Template{
		ID:          "request-worker",
		Name:        "request-worker",
		Description: "request-scoped template",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: RuntimeNameCodex,
		Image:       "worker-image:request",
	})
	startupHub, err := hub.NewService(config.HubConfig{}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatalf("hub.NewService() error = %v", err)
	}
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:1",
		"",
		WithHubService(startupHub),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	got, err := svc.CreateRecord(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			Name:         "alice",
			FromTemplate: "local.request-worker",
		},
		HubService: requestHub,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.Description != "request-scoped template" || got.Image != "" {
		t.Fatalf("created agent = %+v, want request-scoped template defaults", got)
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
		RuntimeKind: RuntimeNameCodex,
		Image:       "worker-image:1",
	})

	svc, err := NewController(
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
	if got.Image != "" {
		t.Fatalf("Image = %q, want %q", got.Image, "worker-image:1")
	}
	if got.RuntimeKind != RuntimeKindCodex {
		t.Fatalf("RuntimeKind = %q, want %q", got.RuntimeKind, RuntimeKindCodex)
	}

	workspaceRoot, err := testBuiltinWorkspaceRoot(got.ID, RuntimeKindCodex)
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
			name:    "codex worker to shared image",
			current: "registry.example/opencsghq/codex-worker:0.1.4",
			latest: defaultAgentImage{
				image:   "registry.example/opencsghq/codex:2026.6.23",
				version: "0.1.5",
			},
		},
		{
			name:    "dsh manager to shared image",
			current: "registry.example/opencsghq/dsh-manager:0.1.5",
			latest: defaultAgentImage{
				image:   "registry.example/opencsghq/dsh:20260623.23-csgclaw",
				version: "0.1.6",
			},
		},
		{
			name:    "codex shared image tag bump",
			current: "registry.example/opencsghq/codex:2026.6.22",
			latest: defaultAgentImage{
				image:   "registry.example/opencsghq/codex:2026.6.23",
				version: "0.1.5",
			},
		},
		{
			name:    "dsh shared image tag bump",
			current: "registry.example/opencsghq/dsh:20260623.22-csgclaw",
			latest: defaultAgentImage{
				image:   "registry.example/opencsghq/dsh:20260623.23-csgclaw",
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
		image:   "registry.example/opencsghq/codex:2026.6.23",
		version: "0.1.5",
	}
	if imageNeedsTemplateVersionUpgrade("registry.example/opencsghq/codex:2026.6.23", latest) {
		t.Fatal("imageNeedsTemplateVersionUpgrade() = true, want false for current shared tag")
	}
	if imageNeedsTemplateVersionUpgrade("registry.example/opencsghq/codex:2026.6.24", latest) {
		t.Fatal("imageNeedsTemplateVersionUpgrade() = true, want false for newer shared tag")
	}
}

func TestAgentIgnoresLocalTemplatePublishedFromManagerForManagerUpgrade(t *testing.T) {
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	const (
		oldManagerImage = "registry.example/opencsghq/codex-manager:0.1.0"
		newManagerImage = "registry.example/opencsghq/codex-manager:0.2.0"
	)
	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "codex-manager", hub.Template{
		ID:          "codex-manager",
		Name:        "codex-manager",
		Description: "manager",
		Role:        hub.TemplateRoleManager,
		RuntimeKind: RuntimeNameCodex,
		Version:     "0.2.0",
		Image:       newManagerImage,
	})
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:unused",
		"",
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultManagerTemplate: "local/codex-manager"}),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
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
		RuntimeKind: RuntimeKindCodex,
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
	if got.AgentProfile.ImageUpgradeRequired {
		t.Fatalf("Agent().AgentProfile.ImageUpgradeRequired = true, want false for local worker template")
	}
}

func TestAgentIgnoresNewerLocalImageCandidateForUpgradeRequired(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	provider := sandboxtest.NewProvider()
	provider.Images = []string{
		"opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/codex-manager:0.2.0",
	}
	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "codex-manager", hub.Template{
		ID:          "codex-manager",
		Name:        "codex-manager",
		Description: "manager",
		Role:        hub.TemplateRoleManager,
		RuntimeKind: RuntimeNameCodex,
		Version:     "0.1.0",
		Image:       "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/codex-manager:0.1.0",
	})

	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:unused",
		"",
		WithSandboxProvider(provider),
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultManagerTemplate: "local/codex-manager"}),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
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
		RuntimeKind: RuntimeKindCodex,
		Image:       "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/codex-manager:0.1.0",
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
		"opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/codex-manager:0.2.0",
		"opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/codex-manager:dev",
	}
	hubSvc := mustNewLocalTemplateHubServiceWithoutWorkspace(t, "codex-manager", hub.Template{
		ID:          "codex-manager",
		Name:        "codex-manager",
		Description: "manager",
		Role:        hub.TemplateRoleManager,
		RuntimeKind: RuntimeNameCodex,
		Version:     "0.2.0",
		Image:       "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/codex-manager:0.2.0",
	})

	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:unused",
		"",
		WithSandboxProvider(provider),
		WithHubService(hubSvc),
		WithBootstrapDefaultTemplates(config.BootstrapConfig{DefaultManagerTemplate: "local/codex-manager"}),
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
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
		RuntimeKind: RuntimeKindCodex,
		Image:       "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/codex-manager:dev",
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

func TestRecreateRefreshesBuiltInSkillsAndPreservesUserSkills(t *testing.T) {
	t.Skip("manager is Codex-only; sandbox manager built-in skill refresh is obsolete")
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	svc, err := NewController(
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
		RuntimeKind: RuntimeKindCodex,
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

	workspaceRoot, err := testBuiltinWorkspaceRoot(ManagerName, RuntimeKindCodex)
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

	recreated, err := svc.RecreateRecord(context.Background(), ManagerUserID)
	if err != nil {
		t.Fatalf("Recreate() error = %v", err)
	}
	if recreated.AgentProfile.EnvRestartRequired {
		t.Fatal("Recreate().AgentProfile.EnvRestartRequired = true, want false")
	}

	wantBuiltIn, err := templateembed.FS().Open(templateembed.CodexManagerRoot + "/skills/agent-teams/SKILL.md")
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
		RuntimeKind: RuntimeNameCodex,
		Image:       "worker-image:1",
	})

	svc, err := NewController(
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

func TestCreateWorkerTreatsLocalManagerPublishAsWorkerTemplate(t *testing.T) {
	t.Cleanup(TestOnlySetSandboxProvider(sandboxtest.NewProvider()))

	hubSvc := mustNewLocalTemplateHubService(t, "review-manager", hub.Template{
		ID:          "review-manager",
		Name:        "review-manager",
		Description: "manager template",
		Role:        hub.TemplateRoleManager,
		RuntimeKind: RuntimeNameCodex,
		Image:       "manager-image:1",
	})

	svc, err := NewController(
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

	got, err := svc.CreateWorker(context.Background(), CreateAgentSpec{Name: "alice"})
	if err != nil {
		t.Fatalf("CreateWorker() error = %v", err)
	}
	if got.Role != RoleWorker || got.Description != "manager template" {
		t.Fatalf("CreateWorker() = role %q description %q, want worker using published template", got.Role, got.Description)
	}
}

func TestCreateWorkerSkipsDefaultTemplateRuntimeMismatch(t *testing.T) {
	hubSvc := mustNewLocalTemplateHubService(t, "frontend-worker", hub.Template{
		ID:          "frontend-worker",
		Name:        "frontend-worker",
		Description: "frontend worker",
		Role:        hub.TemplateRoleWorker,
		RuntimeKind: RuntimeNameCodex,
		Image:       "worker-image:1",
	})

	svc, err := NewController(
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
		RuntimeKind: RuntimeNameCodex,
		Image:       "worker-image:1",
	})

	svc, err := NewController(
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
	if got.Image != "" {
		t.Fatalf("Image = %q, want %q", got.Image, "worker-image:1")
	}

	workspaceRoot, err := testBuiltinWorkspaceRoot(got.ID, RuntimeKindCodex)
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
		RuntimeKind: RuntimeNameCodex,
		Image:       "worker-image:1",
	})

	svc, err := NewController(
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

	_, err = svc.CreateRecord(context.Background(), CreateRequest{
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

	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:1", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	created, err := svc.CreateRecord(context.Background(), CreateRequest{
		Spec: CreateAgentSpec{
			ID:          "u-alice",
			Name:        "alice",
			Description: "review worker",
			RuntimeKind: RuntimeKindCodex,
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

	spec, err := svc.HubPublishSpec(created.ID, false)
	if err != nil {
		t.Fatalf("HubPublishSpec() error = %v", err)
	}
	if spec.ID != created.ID || spec.Name != created.ID || spec.DisplayName != "alice" {
		t.Fatalf("publish identity = %q/%q, want alice/alice", spec.ID, spec.Name)
	}
	if spec.Description != "review worker" {
		t.Fatalf("Description = %q, want %q", spec.Description, "review worker")
	}
	if spec.RuntimeKind != RuntimeNameCodex {
		t.Fatalf("RuntimeKind = %q, want %q", spec.RuntimeKind, RuntimeNameCodex)
	}
	if spec.Image != "" {
		t.Fatalf("Image = %q, want %q", spec.Image, "worker-image:1")
	}
	if spec.WorkspaceRef.Kind != hub.WorkspaceKindDir {
		t.Fatalf("WorkspaceRef.Kind = %q, want %q", spec.WorkspaceRef.Kind, hub.WorkspaceKindDir)
	}
	if spec.WorkspaceRef.Path != workspaceRoot {
		t.Fatalf("WorkspaceRef.Path = %q, want %q", spec.WorkspaceRef.Path, workspaceRoot)
	}
}

func TestHubPublishSpecUsesCodexHomeAssets(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:1", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID: "u-alice", Name: "alice", Role: RoleWorker, RuntimeKind: RuntimeKindCodex,
		RuntimeOptions: map[string]any{"execution_mode": "read_only", "memory_mode": "disabled", "local_workspace_dir": "/private/project"},
		MCPServers: map[string]any{"remote": map[string]any{
			"url": "https://mcp.example.test", "headers": map[string]any{"Authorization": "secret"},
		}},
	}
	layout, err := svc.agentLayout("u-alice", RuntimeKindCodex)
	if err != nil {
		t.Fatalf("agentLayout() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(layout.SkillsRoot, "custom"), 0o755); err != nil {
		t.Fatalf("MkdirAll(skills) error = %v", err)
	}
	if err := os.WriteFile(layout.InstructionsPath, []byte("effective codex instructions\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENTS.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(layout.SkillsRoot, "custom", "SKILL.md"), []byte("custom skill\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}
	memoryPath := filepath.Join(filepath.Dir(layout.SkillsRoot), "memories", "memory_summary.md")
	if err := os.MkdirAll(filepath.Dir(memoryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(memories) error = %v", err)
	}
	if err := os.WriteFile(memoryPath, []byte("remember this\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(memory_summary.md) error = %v", err)
	}

	spec, err := svc.HubPublishSpec("u-alice", true)
	if err != nil {
		t.Fatalf("HubPublishSpec() error = %v", err)
	}
	if got, want := spec.WorkspaceRef.InstructionsPath, layout.InstructionsPath; got != want {
		t.Fatalf("InstructionsPath = %q, want %q", got, want)
	}
	if got, want := spec.WorkspaceRef.SkillsPath, layout.SkillsRoot; got != want {
		t.Fatalf("SkillsPath = %q, want %q", got, want)
	}
	if got := spec.WorkspaceRef.MemoryPath; got != "" {
		t.Fatalf("MemoryPath = %q, want empty when memory is disabled", got)
	}
	if got, want := spec.RuntimeOptions["execution_mode"], "read_only"; got != want {
		t.Fatalf("RuntimeOptions[execution_mode] = %v, want %q", got, want)
	}
	if got, want := spec.RuntimeOptions["memory_mode"], "disabled"; got != want {
		t.Fatalf("RuntimeOptions[memory_mode] = %v, want %q", got, want)
	}
	if _, exists := spec.RuntimeOptions["local_workspace_dir"]; exists {
		t.Fatalf("published runtime options leaked local_workspace_dir: %#v", spec.RuntimeOptions)
	}
	remote := spec.MCPServers["remote"].(map[string]any)
	if _, ok := remote["headers"]; ok {
		t.Fatalf("published MCP config leaked runtime headers: %#v", remote)
	}
}

func TestHubPublishSpecIncludesCodexMemoryWhenEnabled(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:1", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{ID: "u-alice", Name: "alice", Role: RoleWorker, RuntimeKind: RuntimeKindCodex}
	layout, err := svc.agentLayout("u-alice", RuntimeKindCodex)
	if err != nil {
		t.Fatalf("agentLayout() error = %v", err)
	}
	memoryPath := filepath.Join(filepath.Dir(layout.SkillsRoot), "memories", "memory_summary.md")
	if err := os.MkdirAll(filepath.Dir(memoryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(memoryPath, []byte("remember this\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := svc.HubPublishSpec("u-alice", false)
	if err != nil {
		t.Fatalf("HubPublishSpec() error = %v", err)
	}
	if got := spec.WorkspaceRef.MemoryPath; got != "" {
		t.Fatalf("MemoryPath = %q, want empty without explicit opt-in", got)
	}
	spec, err = svc.HubPublishSpec("u-alice", true)
	if err != nil {
		t.Fatalf("HubPublishSpec(include memory) error = %v", err)
	}
	if got := spec.WorkspaceRef.MemoryPath; got != memoryPath {
		t.Fatalf("MemoryPath = %q, want %q", got, memoryPath)
	}
}

func TestValidatedCodexTemplateMemoryPathRejectsPathsOutsideStagingDirectory(t *testing.T) {
	workspaceRoot := t.TempDir()
	valid := filepath.Join(workspaceRoot, ".csgclaw-template-memory", "memory_summary.md")
	if got, err := validatedCodexTemplateMemoryPath(workspaceRoot, valid); err != nil || got != valid {
		t.Fatalf("valid path = %q, error = %v", got, err)
	}
	for _, path := range []string{
		filepath.Join(t.TempDir(), ".csgclaw-template-memory", "memory_summary.md"),
		filepath.Join(workspaceRoot, "other", "memory_summary.md"),
		filepath.Join(workspaceRoot, ".csgclaw-template-memory", "other.md"),
	} {
		if _, err := validatedCodexTemplateMemoryPath(workspaceRoot, path); err == nil {
			t.Fatalf("validatedCodexTemplateMemoryPath(%q) error = nil, want rejection", path)
		}
	}
}

func TestTemplateSafeRuntimeOptionsValidatesMemoryMode(t *testing.T) {
	for _, runtimeOptions := range []map[string]any{
		{"memory_mode": "automatic"},
		{"memory_mode": true},
	} {
		_, err := templateSafeRuntimeOptions(Agent{
			ID:             "u-alice",
			Role:           RoleWorker,
			RuntimeKind:    RuntimeKindCodex,
			RuntimeOptions: runtimeOptions,
		})
		if err == nil || !strings.Contains(err.Error(), "runtime_options.memory_mode") {
			t.Fatalf("templateSafeRuntimeOptions(%#v) error = %v, want memory_mode validation error", runtimeOptions, err)
		}
	}
}

func TestTemplateSafeRuntimeOptionsPreservesDSHPermissionMode(t *testing.T) {
	got, err := templateSafeRuntimeOptions(Agent{
		ID:             "u-dsh",
		Role:           RoleWorker,
		RuntimeKind:    RuntimeKindDSH,
		RuntimeOptions: map[string]any{"permission_mode": "read-only", "executable_path": "/custom/dsh"},
	})
	if err != nil {
		t.Fatalf("templateSafeRuntimeOptions() error = %v", err)
	}
	if len(got) != 2 || got["permission_mode"] != "read-only" || got["memory_mode"] != "enabled" {
		t.Fatalf("templateSafeRuntimeOptions() = %#v, want DSH permission and memory modes", got)
	}
}

func TestTemplateWorkspaceCleanupRemovesMaterializedLocalWorkspace(t *testing.T) {
	path := t.TempDir()
	cleanup := templateWorkspaceCleanup(hub.RegistryKindLocal, hub.WorkspaceRef{
		Kind:      hub.WorkspaceKindDir,
		Path:      path,
		Temporary: true,
	})
	if cleanup == nil {
		t.Fatal("templateWorkspaceCleanup() = nil, want cleanup for materialized local workspace")
	}
	cleanup()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("os.Stat(%q) error = %v, want not exist", path, err)
	}
}

func TestStartConfiguredAgentsRestoresRunningCodexWorker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	startCalls := 0
	rt := fakeAgentRuntime{
		kind: RuntimeKindCodex,
		start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
			startCalls++
			return agentruntime.StateRunning, nil
		},
		info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
			return agentruntime.Info{
				HandleID: h.HandleID,
				State:    agentruntime.StateRunning,
			}, nil
		},
	}
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(rt),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:          "u-alice",
		Name:        "alice",
		Role:        RoleWorker,
		RuntimeKind: RuntimeKindCodex,
		RuntimeID:   "rt-u-alice",
		BoxID:       "sess-alice",
		Status:      string(agentruntime.StateRunning),
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderCodex,
			ModelID:         "gpt-5.5",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	if err := svc.StartConfiguredAgents(context.Background()); err != nil {
		t.Fatalf("StartConfiguredAgents() error = %v", err)
	}
	if startCalls != 1 {
		t.Fatalf("Codex runtime Start() calls = %d, want 1 to restore the in-memory session", startCalls)
	}
	if got := svc.withConfiguredAgentStartupStatus(svc.agents["u-alice"]); got.StartupPending {
		t.Fatal("configured startup marker remained set after the restore pass")
	}
}

func TestStartConfiguredAgentsRestoresDesiredRunningDSHWorker(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	startCalls := 0
	rt := fakeAgentRuntime{
		kind: RuntimeKindDSH,
		start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
			startCalls++
			return agentruntime.StateRunning, nil
		},
		info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
			return agentruntime.Info{
				HandleID: h.HandleID,
				State:    agentruntime.StateStopped,
			}, nil
		},
	}
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(rt),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:           "u-alice",
		Name:         "alice",
		Role:         RoleWorker,
		RuntimeKind:  RuntimeKindDSH,
		RuntimeID:    "rt-u-alice",
		BoxID:        "rt-u-alice",
		Status:       string(agentruntime.StateRunning),
		DesiredState: DesiredStateRunning,
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        "default",
			ModelID:         "qwen3.7-plus",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	if err := svc.StartConfiguredAgents(context.Background()); err != nil {
		t.Fatalf("StartConfiguredAgents() error = %v", err)
	}
	if startCalls != 1 {
		t.Fatalf("DSH runtime Start() calls = %d, want 1 when desired state remains running", startCalls)
	}
}

func TestConfiguredAgentStartupStatusMarksOnlyRestoreCandidates(t *testing.T) {
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.PrepareConfiguredAgentsStartup()

	tests := []struct {
		name  string
		agent Agent
		want  bool
	}{
		{
			name: "configured codex worker",
			agent: Agent{
				ID: "u-alice", Name: "alice", Role: RoleWorker, RuntimeKind: RuntimeKindCodex, ProfileComplete: true,
			},
			want: true,
		},
		{
			name: "configured DSH worker",
			agent: Agent{
				ID: "u-dsh", Name: "dsh", Role: RoleWorker, RuntimeKind: RuntimeKindDSH, ProfileComplete: true,
			},
			want: true,
		},
		{
			name: "configured gateway worker",
			agent: Agent{
				ID: "u-bob", Name: "bob", Role: RoleWorker, RuntimeKind: RuntimeKindCodex, ProfileComplete: true,
			},
			want: true,
		},
		{
			name: "manager",
			agent: Agent{
				ID: ManagerUserID, Name: ManagerName, Role: RoleManager, RuntimeKind: RuntimeKindCodex, ProfileComplete: true,
			},
			want: false,
		},
		{
			name: "incomplete worker",
			agent: Agent{
				ID: "u-carol", Name: "carol", Role: RoleWorker, RuntimeKind: RuntimeKindCodex,
			},
			want: false,
		},
		{
			name: "unsupported non-gateway worker",
			agent: Agent{
				ID: "u-dave", Name: "dave", Role: RoleWorker, RuntimeKind: "custom", ProfileComplete: true,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := svc.withConfiguredAgentStartupStatus(tt.agent).StartupPending; got != tt.want {
				t.Fatalf("StartupPending = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStartConfiguredAgentsLeavesStoppedCodexWorkerStopped(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	startCalls := 0
	rt := fakeAgentRuntime{
		kind: RuntimeKindCodex,
		start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
			startCalls++
			return agentruntime.StateRunning, nil
		},
		info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
			return agentruntime.Info{
				HandleID: h.HandleID,
				State:    agentruntime.StateStopped,
			}, nil
		},
	}
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(rt),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:          "u-alice",
		Name:        "alice",
		Role:        RoleWorker,
		RuntimeKind: RuntimeKindCodex,
		RuntimeID:   "rt-u-alice",
		BoxID:       "sess-alice",
		Status:      string(agentruntime.StateStopped),
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        ProviderCodex,
			ModelID:         "gpt-5.5",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	if err := svc.StartConfiguredAgents(context.Background()); err != nil {
		t.Fatalf("StartConfiguredAgents() error = %v", err)
	}
	if startCalls != 0 {
		t.Fatalf("Codex runtime Start() calls = %d, want 0 for explicitly stopped worker", startCalls)
	}
}

func TestStartConfiguredAgentsLeavesDesiredStoppedDSHWorkerStopped(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	startCalls := 0
	rt := fakeAgentRuntime{
		kind: RuntimeKindDSH,
		start: func(context.Context, agentruntime.Handle) (agentruntime.State, error) {
			startCalls++
			return agentruntime.StateRunning, nil
		},
		info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
			return agentruntime.Info{
				HandleID: h.HandleID,
				State:    agentruntime.StateExited,
			}, nil
		},
	}
	svc, err := NewController(
		testModelConfig(),
		config.ServerConfig{},
		"manager-image:test",
		"",
		WithRuntime(rt),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.agents["u-alice"] = Agent{
		ID:           "u-alice",
		Name:         "alice",
		Role:         RoleWorker,
		RuntimeKind:  RuntimeKindDSH,
		RuntimeID:    "rt-u-alice",
		BoxID:        "rt-u-alice",
		Status:       string(agentruntime.StateRunning),
		DesiredState: DesiredStateStopped,
		AgentProfile: AgentProfile{
			Name:            "alice",
			Provider:        "default",
			ModelID:         "qwen3.7-plus",
			ProfileComplete: true,
		},
		ProfileComplete: true,
	}

	if err := svc.StartConfiguredAgents(context.Background()); err != nil {
		t.Fatalf("StartConfiguredAgents() error = %v", err)
	}
	if startCalls != 0 {
		t.Fatalf("DSH runtime Start() calls = %d, want 0 when desired state is stopped", startCalls)
	}
}

func TestCreateClosesBoxHandleAfterCreate(t *testing.T) {
	rt := &fakeRuntime{}
	SetTestHooks(
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return rt, nil },
		nil,
	)
	defer ResetTestHooks()

	var closeCalls int
	var closeRuntimeCalls int
	testCreateBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ sandbox.CreateSpec) (sandbox.Instance, error) {
		return &fakeInstance{}, nil
	}
	testCloseBoxHook = func(_ *Controller, _ sandbox.Instance) error {
		closeCalls++
		return nil
	}
	testCloseRuntimeHook = func(_ *Controller, _ string, got sandbox.Runtime) error {
		if got != rt {
			t.Fatalf("closeRuntime() got runtime %p, want %p", got, rt)
		}
		closeRuntimeCalls++
		return nil
	}

	svc, err := NewController(
		config.ModelConfig{BaseURL: "http://127.0.0.1:4000", APIKey: "sk-test", ModelID: "model-1"},
		config.ServerConfig{}, "manager-image:test", "",
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = svc.CreateRecord(context.Background(), CreateRequest{
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
				func(_ *Controller, _ string) (sandbox.Runtime, error) { return rt, nil },
				nil,
			)
			defer ResetTestHooks()

			testCreateBoxHook = func(_ *Controller, _ context.Context, gotRT sandbox.Runtime, spec sandbox.CreateSpec) (sandbox.Instance, error) {
				if gotRT != rt {
					t.Fatalf("createBox() runtime = %p, want %p", gotRT, rt)
				}
				gotSpec = spec
				return &fakeInstance{}, nil
			}

			svc, err := NewController(
				config.ModelConfig{BaseURL: "http://127.0.0.1:4000", APIKey: "sk-test", ModelID: "model-1"},
				config.ServerConfig{},
				"manager-image:1",
				"",
			)
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}

			_, err = svc.CreateRecord(context.Background(), CreateRequest{
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
	t.Skip("manager is Codex-only; sandbox manager box cleanup is obsolete")
	t.Setenv("HOME", t.TempDir())

	SetTestHooks(
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return &fakeRuntime{}, nil },
		func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
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
	testForceRemoveBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, idOrName string) error {
		removed = idOrName
		return nil
	}
	testGetBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
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
				RuntimeKind: RuntimeKindCodex,
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

	reloaded, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", statePath)
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

func TestEnsureBootstrapStateClosesManagerBoxHandleAfterCreate(t *testing.T) {
	t.Skip("manager is Codex-only; sandbox manager box handles are obsolete")
	t.Setenv("HOME", t.TempDir())

	rt := &fakeRuntime{}
	SetTestHooks(
		func(_ *Controller, _ string) (sandbox.Runtime, error) { return rt, nil },
		func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string, name, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
			return &fakeInstance{}, sandbox.Info{
				ID:        "box-" + name,
				Name:      name,
				State:     sandbox.StateRunning,
				CreatedAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
			}, nil
		},
	)
	defer ResetTestHooks()

	testGetBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string) (sandbox.Instance, error) {
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}

	var closeCalls int
	var closeRuntimeCalls int
	testCloseBoxHook = func(_ *Controller, _ sandbox.Instance) error {
		closeCalls++
		return nil
	}
	testCloseRuntimeHook = func(_ *Controller, _ string, got sandbox.Runtime) error {
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

	reloaded, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() reload error = %v", err)
	}
	if got, want := len(reloaded.runtimes), 0; got != want {
		t.Fatalf("len(reloaded.runtimes) = %d, want %d", got, want)
	}
}

func TestEnsureBootstrapStateReusesStoredManagerBoxIDWithoutForce(t *testing.T) {
	t.Skip("manager is Codex-only; sandbox manager box reuse is obsolete")
	t.Setenv("HOME", t.TempDir())

	SetTestHooks(nil, nil)
	defer ResetTestHooks()

	primaryRT := &fakeRuntime{}
	testEnsureRuntimeAtHomeHook = func(_ *Controller, home string) (sandbox.Runtime, error) {
		return primaryRT, nil
	}

	var created bool
	testCreateGatewayBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, _ string, _ string, _ string, _ AgentProfile) (sandbox.Instance, sandbox.Info, error) {
		created = true
		return nil, sandbox.Info{}, nil
	}
	testGetBoxHook = func(_ *Controller, _ context.Context, rt sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		if rt == primaryRT && idOrName == "box-old" {
			return &fakeInstance{}, nil
		}
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	testStartBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Instance) error { return nil }
	testBoxInfoHook = func(_ *Controller, _ context.Context, _ sandbox.Instance) (sandbox.Info, error) {
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
				RuntimeKind: RuntimeKindCodex,
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

func TestEnsureBootstrapManagerUsesStoredManagerProfileWhenDefaultModelIsInvalid(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	statePath := filepath.Join(t.TempDir(), "agents.json")
	data, err := json.Marshal(persistedState{
		Agents: []persistedAgent{
			{
				ID:          ManagerUserID,
				Name:        ManagerName,
				RuntimeKind: RuntimeKindCodex,
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
	svc, err := NewControllerWithLLM(
		invalidDefault,
		config.ServerConfig{ListenAddr: ":18080", AccessToken: "token"},
		"manager-image:test",
		statePath,
		WithRuntime(fakeAgentRuntime{
			kind: RuntimeKindCodex,
			provision: func(_ context.Context, req agentruntime.ProvisionRequest) error {
				provisionedModel = req.Profile.ModelID
				return nil
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "box-new"}, nil
			},
			info: func(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
				return agentruntime.Info{HandleID: h.HandleID, State: agentruntime.StateRunning}, nil
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

	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
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

	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", statePath)
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
	testForceRemoveBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, idOrName string) error {
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

	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "", WithSandboxProvider(provider))
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
	testEnsureRuntimeAtHomeHook = func(_ *Controller, homeDir string) (sandbox.Runtime, error) {
		if homeDir == "" {
			t.Fatalf("ensureRuntimeAtHome() homeDir = %q, want non-empty", homeDir)
		}
		return &fakeRuntime{}, nil
	}
	testForceRemoveBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, idOrName string) error {
		removed = append(removed, idOrName)
		return nil
	}
	testGetBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		t.Fatalf("lookupBootstrapManager() should not reuse orphan manager box %q", idOrName)
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	defer func() {
		testEnsureRuntimeAtHomeHook = nil
		testForceRemoveBoxHook = nil
		testGetBoxHook = nil
	}()

	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
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
	testEnsureRuntimeAtHomeHook = func(_ *Controller, homeDir string) (sandbox.Runtime, error) {
		if homeDir == "" {
			t.Fatalf("ensureRuntimeAtHome() homeDir = %q, want non-empty", homeDir)
		}
		return &fakeRuntime{}, nil
	}
	testGetBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		lookedUp = append(lookedUp, idOrName)
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	defer func() {
		testEnsureRuntimeAtHomeHook = nil
		testGetBoxHook = nil
	}()

	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
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
	testEnsureRuntimeAtHomeHook = func(_ *Controller, homeDir string) (sandbox.Runtime, error) {
		if homeDir == "" {
			t.Fatalf("ensureRuntimeAtHome() homeDir = %q, want non-empty", homeDir)
		}
		return &fakeRuntime{}, nil
	}
	testGetBoxHook = func(_ *Controller, _ context.Context, _ sandbox.Runtime, idOrName string) (sandbox.Instance, error) {
		lookedUp = append(lookedUp, idOrName)
		return nil, fmt.Errorf("%w: missing", sandbox.ErrNotFound)
	}
	defer func() {
		testEnsureRuntimeAtHomeHook = nil
		testGetBoxHook = nil
	}()

	svc, err := NewController(config.ModelConfig{}, config.ServerConfig{}, "manager-image:test", "")
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

func mustNewLocalTemplateHubService(t *testing.T, id string, item hub.Template) *hub.Service {
	return mustNewLocalTemplateHubServiceWithMCP(t, id, item, map[string]any{
		"template-docs": map[string]any{"command": "npx", "args": []any{"-y", "template-docs"}},
	})
}

func mustNewLocalTemplateHubServiceWithMCP(t *testing.T, id string, item hub.Template, mcpServers map[string]any) *hub.Service {
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
	workspaceRef := hub.WorkspaceRef{Kind: hub.WorkspaceKindDir, Path: workspaceRoot}
	includeMemory := false
	if kind := agentruntime.RuntimeConfigForKind(item.RuntimeKind).LegacyKind(); kind == RuntimeKindCodex || kind == RuntimeKindDSH {
		memoryPath := filepath.Join(t.TempDir(), "memory_summary.md")
		if err := os.WriteFile(memoryPath, []byte("# Template memory\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(memory_summary.md) error = %v", err)
		}
		workspaceRef.MemoryPath = memoryPath
		includeMemory = true
	} else if agentruntime.RuntimeConfigForKind(item.RuntimeKind).LegacyKind() == RuntimeKindDSH {
		if err := os.WriteFile(filepath.Join(workspaceRoot, "MEMORY.md"), []byte("# DSH template memory\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(MEMORY.md) error = %v", err)
		}
		if err := os.MkdirAll(filepath.Join(workspaceRoot, "memory"), 0o755); err != nil {
			t.Fatalf("MkdirAll(memory) error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(workspaceRoot, "memory", "2026-08-28.md"), []byte("DSH dated memory\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(dated memory) error = %v", err)
		}
		includeMemory = true
	}
	if _, err := store.Publish(context.Background(), hub.PublishSpec{
		ID:             id,
		IncludeMemory:  includeMemory,
		Name:           item.Name,
		Description:    item.Description,
		RuntimeKind:    item.RuntimeKind,
		Version:        item.Version,
		Image:          item.Image,
		RuntimeOptions: item.RuntimeOptions,
		WorkspaceRef:   workspaceRef,
		MCPServers:     mcpServers,
		UpdatedAt:      time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC),
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
	}
	if err := os.WriteFile(manifestPath, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", manifestPath, err)
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

func TestServiceCloseClosesRegisteredAgentRuntimes(t *testing.T) {
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	closeCalls := 0
	rt := &fakeClosableAgentRuntime{
		fakeAgentRuntime: fakeAgentRuntime{kind: RuntimeKindCodex},
		close: func() error {
			closeCalls++
			return nil
		},
	}
	if err := WithRuntime(rt)(svc); err != nil {
		t.Fatalf("WithRuntime() error = %v", err)
	}

	if err := svc.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if closeCalls != 1 {
		t.Fatalf("registered runtime Close() calls = %d, want 1", closeCalls)
	}
}

func TestResetSandboxRuntimesClosesAndDropsCachedClients(t *testing.T) {
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	closeCalls := 0
	svc.runtimes["agent-home"] = &fakeRuntime{close: func() error {
		closeCalls++
		return nil
	}}

	if err := svc.ResetSandboxRuntimes(); err != nil {
		t.Fatalf("ResetSandboxRuntimes() error = %v", err)
	}
	if closeCalls != 1 {
		t.Fatalf("sandbox runtime Close() calls = %d, want 1", closeCalls)
	}
	if len(svc.runtimes) != 0 {
		t.Fatalf("cached runtimes = %d, want 0", len(svc.runtimes))
	}
}

func TestServiceWorkspaceRootUsesRegisteredRuntimeCapability(t *testing.T) {
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	const runtimeKind = "custom_runtime"
	infoCalls := 0
	if err := WithRuntime(fakeAgentRuntime{
		kind: runtimeKind,
		workspace: func(agentHome string) string {
			return filepath.Join(agentHome, ".custom", "workspace")
		},
		info: func(context.Context, agentruntime.Handle) (agentruntime.Info, error) {
			infoCalls++
			return agentruntime.Info{}, nil
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

	gotByID, err := svc.WorkspaceRootByID("u-alice")
	if err != nil {
		t.Fatalf("WorkspaceRootByID() error = %v", err)
	}
	if gotByID != want {
		t.Fatalf("WorkspaceRootByID() = %q, want %q", gotByID, want)
	}
	if infoCalls != 0 {
		t.Fatalf("WorkspaceRootByID() runtime Info() calls = %d, want 0", infoCalls)
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
			RuntimeKind: RuntimeKindCodex,
			BoxID:       "box-old",
			Role:        RoleWorker,
			Status:      string(agentruntime.StateStopped),
			CreatedAt:   time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}

	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := WithRuntime(fakeAgentRuntime{
		kind: RuntimeKindCodex,
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
	if view.RuntimeKind != RuntimeKindCodex {
		t.Fatalf("RuntimeView().RuntimeKind = %q, want %q", view.RuntimeKind, RuntimeKindCodex)
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
			RuntimeKind: RuntimeKindCodex,
			BoxID:       "box-old",
			Role:        RoleWorker,
			Status:      string(agentruntime.StateRunning),
			CreatedAt:   time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		},
	}); err != nil {
		t.Fatalf("writeSeededAgents() error = %v", err)
	}

	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", statePath)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := WithRuntime(fakeAgentRuntimeNoLogs{
		kind: RuntimeKindCodex,
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

func TestPlatformTemplateCreationDefersModelCheck(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, tc := range []struct {
		provider, template string
		skip               bool
	}{
		{"csghub", "local.worker", true},
		{"csghub", "", false},
		{"docker", "local.worker", false},
	} {
		t.Run(tc.provider+"/"+tc.template, func(t *testing.T) {
			calls := 0
			svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "",
				WithSandboxProvider(fakeProvider{name: tc.provider}),
				WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex, validate: func(_ context.Context, current agentruntime.RuntimeConfigSnapshot) error {
					calls++
					if current.SkipModelAvailabilityCheck != tc.skip {
						t.Fatalf("skip = %v, want %v", current.SkipModelAvailabilityCheck, tc.skip)
					}
					return nil
				}}))
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.validateCreateRuntimeConfig(context.Background(), CreateAgentSpec{FromTemplate: tc.template}, RuntimeKindCodex, agentruntime.RuntimeConfigSnapshot{}); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("validation calls = %d", calls)
			}
		})
	}
}

func TestCreatePlatformTemplateWorkerWithUnavailableModel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	hubSvc := mustNewLocalTemplateHubService(t, "codex-worker", hub.Template{ID: "codex-worker", Name: "codex-worker", Role: hub.TemplateRoleWorker, RuntimeKind: RuntimeNameCodex})
	calls := 0
	svc, err := NewController(testModelConfig(), config.ServerConfig{}, "manager-image:test", "",
		WithHubService(hubSvc), WithSandboxProvider(fakeProvider{name: "csghub"}),
		WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex,
			validate: func(_ context.Context, current agentruntime.RuntimeConfigSnapshot) error {
				calls++
				if !current.SkipModelAvailabilityCheck {
					return errors.New("model unavailable")
				}
				return nil
			},
			new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
				return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "template-worker"}, nil
			},
		}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.CreateRecord(context.Background(), CreateRequest{Spec: CreateAgentSpec{
		ID: "platform-worker", Name: "platform-worker", FromTemplate: "local.codex-worker",
		AgentProfile: AgentProfile{Provider: ProviderAPI, BaseURL: "https://unavailable.example/v1", APIKey: "test-key", ModelID: "offline-model", ProfileComplete: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.BoxID != "template-worker" || got.Status != "running" || calls != 1 {
		t.Fatalf("worker = %#v, validation calls = %d", got, calls)
	}
}

func testBuiltinLayout(agentID, runtimeKind string) (agentruntime.Layout, error) {
	home, err := agentHomeDir(agentID)
	if err != nil {
		return agentruntime.Layout{}, err
	}
	return fakeAgentRuntime{kind: runtimeKind}.Layout(home), nil
}

func TestCreateHostWorkerFromInstructionsOnlyTemplate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "AGENTS.md"), []byte("# Retained template instructions\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Empty directories must not be treated as an overlay containing files.
	if err := os.MkdirAll(filepath.Join(source, "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	registry := t.TempDir()
	if _, err := hub.NewLocalStore(registry).Publish(context.Background(), hub.PublishSpec{Name: "instructions-only", RuntimeKind: RuntimeKindCodex, WorkspaceRef: hub.WorkspaceRef{Kind: hub.WorkspaceKindDir, Path: source}, MCPServers: map[string]any{"docs": map[string]any{"url": "https://docs.example/mcp"}}}); err != nil {
		t.Fatal(err)
	}
	service, err := hub.NewService(config.HubConfig{Registries: []config.HubRegistryConfig{{Name: "local", Kind: hub.RegistryKindLocal, Path: registry, Enabled: true}}}, hub.DefaultStoreFactory)
	if err != nil {
		t.Fatal(err)
	}
	var provision agentruntime.ProvisionRequest
	controller, err := NewController(testModelConfig(), config.ServerConfig{}, "", "", WithHubService(service), WithRuntime(fakeAgentRuntime{kind: RuntimeKindCodex, provision: func(ctx context.Context, req agentruntime.ProvisionRequest) error {
		provision = req
		return runtimecodex.New(runtimecodex.Dependencies{AgentHome: agentHomeDir}).Provision(ctx, req)
	}}))
	if err != nil {
		t.Fatal(err)
	}
	created, err := controller.CreateRecord(context.Background(), CreateRequest{Spec: CreateAgentSpec{Name: "instructions-only", FromTemplate: "local.instructions-only"}})
	if err != nil {
		t.Fatal(err)
	}
	if created.RuntimeKind != RuntimeKindCodex {
		t.Fatalf("template instructions lost: %+v", created)
	}
	if provision.WorkspaceOverlay != "" || !strings.Contains(provision.TemplateInstructions, "Retained template instructions") {
		t.Fatalf("instructions-only template still used an empty overlay: %+v", provision)
	}
	home, err := agentHomeDir(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile(filepath.Join(home, ".codex", "home", "AGENTS.md"))
	if err != nil || !strings.Contains(string(doc), "Retained template instructions") {
		t.Fatal("provisioned template instructions lost")
	}
	if _, ok := provision.MCPServers["docs"]; !ok {
		t.Fatal("template MCP configuration lost")
	}
}
