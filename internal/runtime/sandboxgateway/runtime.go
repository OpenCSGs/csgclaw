package sandboxgateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
)

type AgentRef struct {
	ID        string
	Name      string
	RuntimeID string
	BoxID     string
}

type Dependencies struct {
	RuntimeKind    string
	ModelFallback  string
	Server         config.ServerConfig
	Channels       config.ChannelsConfig
	ResolveBaseURL func(server config.ServerConfig) string

	EnsureRuntime    func(agentName string) (sandbox.Runtime, error)
	RuntimeHome      func(agentName string) (string, error)
	CloseRuntime     func(homeDir string, rt sandbox.Runtime) error
	ResolveBox       func(ctx context.Context, rt sandbox.Runtime, got AgentRef) (sandbox.Instance, string, error)
	CreateGatewayBox func(ctx context.Context, rt sandbox.Runtime, image, name, botID string, profile agentruntime.Profile) (sandbox.Instance, sandbox.Info, error)
	CreateBox        func(ctx context.Context, rt sandbox.Runtime, spec sandbox.CreateSpec) (sandbox.Instance, error)
	StartBox         func(ctx context.Context, box sandbox.Instance) error
	StopBox          func(ctx context.Context, box sandbox.Instance, opts sandbox.StopOptions) error
	BoxInfo          func(ctx context.Context, box sandbox.Instance) (sandbox.Info, error)
	ForceRemoveBox   func(ctx context.Context, rt sandbox.Runtime, idOrName string) error
	CloseBox         func(box sandbox.Instance) error
	RunBoxCommand    func(ctx context.Context, box sandbox.Instance, name string, args []string, w io.Writer) (int, error)

	ResolveAgent        func(h agentruntime.Handle) (AgentRef, error)
	SyncHandle          func(h agentruntime.Handle) error
	EnsureGatewayConfig func(agentName, botID string, profile agentruntime.Profile) error
	EnsureWorkspace     func(agentName, template string) (string, error)
	WorkspaceTemplate   func(name, botID string) string
	EnsureProjectsRoot  func() (string, error)
	BuildRuntimeEnv     func(baseURL, accessToken, botID, llmBaseURL, modelID string, channels config.ChannelsConfig) map[string]string
	AddProfileEnv       func(envVars map[string]string, profileEnv map[string]string)
	HomeEnv             string
	WorkspaceGuestPath  string
	ProjectsGuestPath   string
	GatewayLogPath      string
	GatewayCommand      func() string
	StreamLogs          func(ctx context.Context, agentID string, follow bool, lines int, w io.Writer) error
}

type Runtime struct {
	mu   sync.RWMutex
	deps Dependencies
}

var (
	_ agentruntime.Runtime     = (*Runtime)(nil)
	_ agentruntime.LogStreamer = (*Runtime)(nil)
)

func New(deps Dependencies) *Runtime {
	deps.Channels = cloneChannelsConfig(deps.Channels)
	return &Runtime{deps: deps}
}

func (r *Runtime) SetChannels(channels config.ChannelsConfig) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deps.Channels = cloneChannelsConfig(channels)
}

func (r *Runtime) channels() config.ChannelsConfig {
	if r == nil {
		return config.ChannelsConfig{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneChannelsConfig(r.deps.Channels)
}

func (r *Runtime) Kind() string {
	if kind := strings.TrimSpace(r.deps.RuntimeKind); kind != "" {
		return kind
	}
	return "sandbox_gateway"
}

func (r *Runtime) Create(ctx context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
	agentName := strings.TrimSpace(spec.AgentName)
	agentID := strings.TrimSpace(spec.AgentID)
	if agentName == "" || agentID == "" {
		return agentruntime.Handle{}, fmt.Errorf("runtime agent name and id are required")
	}

	rt, runtimeHome, err := r.openSandboxRuntime(agentName)
	if err != nil {
		return agentruntime.Handle{}, err
	}
	defer func() {
		_ = r.deps.CloseRuntime(runtimeHome, rt)
	}()

	box, info, err := r.CreateGatewayBox(ctx, rt, strings.TrimSpace(spec.Image), agentName, agentID, spec.Profile)
	if err != nil {
		return agentruntime.Handle{}, err
	}
	defer func() {
		_ = r.deps.CloseBox(box)
	}()

	handle := agentruntime.Handle{
		RuntimeID: strings.TrimSpace(spec.RuntimeID),
		HandleID:  strings.TrimSpace(info.ID),
	}
	if err := r.syncHandle(handle); err != nil {
		return agentruntime.Handle{}, err
	}
	return handle, nil
}

func (r *Runtime) Start(ctx context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	box, release, err := r.openBoxForHandle(ctx, h)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	defer release()

	info, err := r.infoForBox(ctx, h, box)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	if info.State == agentruntime.StateRunning {
		return info.State, nil
	}
	if err := r.deps.StartBox(ctx, box); err != nil {
		return agentruntime.StateUnknown, err
	}
	info, err = r.infoForBox(ctx, h, box)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	return info.State, nil
}

func (r *Runtime) Stop(ctx context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	box, release, err := r.openBoxForHandle(ctx, h)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	defer release()

	if err := r.deps.StopBox(ctx, box, sandbox.StopOptions{}); err != nil {
		return agentruntime.StateUnknown, err
	}
	info, err := r.infoForBox(ctx, h, box)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	return info.State, nil
}

func (r *Runtime) Delete(ctx context.Context, h agentruntime.Handle) error {
	got, err := r.deps.ResolveAgent(h)
	if err != nil {
		return err
	}

	rt, runtimeHome, err := r.openSandboxRuntime(got.Name)
	if err != nil {
		return err
	}
	defer func() {
		_ = r.deps.CloseRuntime(runtimeHome, rt)
	}()

	boxIDOrName := ""
	box, resolvedKey, err := r.deps.ResolveBox(ctx, rt, got)
	if err == nil {
		info, infoErr := r.infoForBox(ctx, h, box)
		_ = r.deps.CloseBox(box)
		if infoErr != nil {
			return infoErr
		}
		boxIDOrName = strings.TrimSpace(info.HandleID)
		if boxIDOrName == "" {
			boxIDOrName = strings.TrimSpace(resolvedKey)
		}
	} else if !sandbox.IsNotFound(err) {
		return err
	}
	if boxIDOrName == "" {
		boxIDOrName = strings.TrimSpace(h.HandleID)
	}
	if boxIDOrName == "" {
		boxIDOrName = strings.TrimSpace(got.BoxID)
	}
	if boxIDOrName == "" {
		boxIDOrName = strings.TrimSpace(got.Name)
	}
	if err := r.deps.ForceRemoveBox(ctx, rt, boxIDOrName); err != nil && !sandbox.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *Runtime) State(ctx context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	info, release, err := r.infoForHandle(ctx, h)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	defer release()
	return info.State, nil
}

func (r *Runtime) Info(ctx context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
	info, release, err := r.infoForHandle(ctx, h)
	if err != nil {
		return agentruntime.Info{}, err
	}
	defer release()
	return info, nil
}

func (r *Runtime) StreamLogs(ctx context.Context, h agentruntime.Handle, opts agentruntime.LogOptions) error {
	got, err := r.deps.ResolveAgent(h)
	if err != nil {
		return err
	}
	lines := opts.Tail
	if lines <= 0 {
		lines = 20
	}
	if err := r.deps.StreamLogs(ctx, got.ID, opts.Follow, lines, opts.Writer); err == nil {
		return nil
	} else if opts.Follow || !errors.Is(err, os.ErrNotExist) {
		return err
	}

	box, release, err := r.openBoxForHandle(ctx, h)
	if err != nil {
		return err
	}
	defer release()

	args := []string{"-n", fmt.Sprintf("%d", lines)}
	if opts.Follow {
		args = append(args, "-f")
	}
	logPath := r.gatewayLogPath()
	if logPath == "" {
		return fmt.Errorf("gateway log path is required")
	}
	args = append(args, logPath)
	exitCode, err := r.deps.RunBoxCommand(ctx, box, "tail", args, opts.Writer)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("tail exited with code %d", exitCode)
	}
	return nil
}

func (r *Runtime) CreateGatewayBox(ctx context.Context, rt sandbox.Runtime, image, name, botID string, profile agentruntime.Profile) (sandbox.Instance, sandbox.Info, error) {
	if r.deps.CreateGatewayBox != nil {
		return r.deps.CreateGatewayBox(ctx, rt, image, name, botID, profile)
	}
	if rt == nil {
		return nil, sandbox.Info{}, fmt.Errorf("invalid sandbox runtime")
	}
	spec, err := r.GatewayCreateSpec(image, name, botID, profile)
	if err != nil {
		return nil, sandbox.Info{}, err
	}
	box, err := r.deps.CreateBox(ctx, rt, spec)
	if err != nil {
		return nil, sandbox.Info{}, fmt.Errorf("create gateway box: %w", err)
	}
	info, err := r.deps.BoxInfo(ctx, box)
	if err != nil {
		_ = r.deps.CloseBox(box)
		return nil, sandbox.Info{}, fmt.Errorf("read gateway box info: %w", err)
	}
	return box, info, nil
}

func (r *Runtime) GatewayCreateSpec(image, name, botID string, profile agentruntime.Profile) (sandbox.CreateSpec, error) {
	modelID := strings.TrimSpace(profile.ModelID)
	if modelID == "" {
		modelID = strings.TrimSpace(r.deps.ModelFallback)
	}
	managerBaseURL := r.deps.ResolveBaseURL(r.deps.Server)
	llmBaseURL := llmBridgeBaseURL(managerBaseURL, botID)
	profile.ModelID = modelID
	if err := r.deps.EnsureGatewayConfig(name, botID, profile); err != nil {
		return sandbox.CreateSpec{}, err
	}
	hostWorkspaceRoot, err := r.deps.EnsureWorkspace(name, r.deps.WorkspaceTemplate(name, botID))
	if err != nil {
		return sandbox.CreateSpec{}, err
	}
	projectsRoot, err := r.deps.EnsureProjectsRoot()
	if err != nil {
		return sandbox.CreateSpec{}, err
	}
	envVars := r.deps.BuildRuntimeEnv(managerBaseURL, r.deps.Server.AccessToken, botID, llmBaseURL, modelID, r.channels())
	r.deps.AddProfileEnv(envVars, profile.Env)
	homeEnv := r.homeEnv()
	workspaceGuestPath := r.workspaceGuestPath()
	projectsGuestPath := r.projectsGuestPath()
	gatewayCommand := r.gatewayCommand()
	if homeEnv == "" {
		return sandbox.CreateSpec{}, fmt.Errorf("runtime HOME env is required")
	}
	if workspaceGuestPath == "" {
		return sandbox.CreateSpec{}, fmt.Errorf("workspace guest path is required")
	}
	if projectsGuestPath == "" {
		return sandbox.CreateSpec{}, fmt.Errorf("projects guest path is required")
	}
	if gatewayCommand == "" {
		return sandbox.CreateSpec{}, fmt.Errorf("gateway command is required")
	}
	envVars["HOME"] = homeEnv
	spec := sandbox.CreateSpec{
		Image:      image,
		Name:       name,
		Detach:     true,
		AutoRemove: false,
		Env:        envVars,
		Cmd:        []string{"/bin/sh", "-c", gatewayCommand},
	}
	spec.Mounts = append(spec.Mounts,
		sandbox.Mount{HostPath: hostWorkspaceRoot, GuestPath: workspaceGuestPath},
		sandbox.Mount{HostPath: projectsRoot, GuestPath: projectsGuestPath},
	)
	return spec, nil
}

func (r *Runtime) EnsureGatewayConfig(agentName, botID, modelID string) error {
	return r.deps.EnsureGatewayConfig(agentName, botID, agentruntime.Profile{ModelID: strings.TrimSpace(modelID)})
}

func (r *Runtime) ProjectsGuestPath() string {
	return r.projectsGuestPath()
}

func (r *Runtime) GatewayLogPath() string {
	return r.gatewayLogPath()
}

func (r *Runtime) homeEnv() string {
	return strings.TrimSpace(r.deps.HomeEnv)
}

func (r *Runtime) workspaceGuestPath() string {
	return strings.TrimSpace(r.deps.WorkspaceGuestPath)
}

func (r *Runtime) projectsGuestPath() string {
	return strings.TrimSpace(r.deps.ProjectsGuestPath)
}

func (r *Runtime) gatewayLogPath() string {
	return strings.TrimSpace(r.deps.GatewayLogPath)
}

func (r *Runtime) gatewayCommand() string {
	if r.deps.GatewayCommand != nil {
		return strings.TrimSpace(r.deps.GatewayCommand())
	}
	return ""
}

func (r *Runtime) openSandboxRuntime(agentName string) (sandbox.Runtime, string, error) {
	rt, err := r.deps.EnsureRuntime(agentName)
	if err != nil {
		return nil, "", err
	}
	runtimeHome, err := r.deps.RuntimeHome(agentName)
	if err != nil {
		_ = r.deps.CloseRuntime("", rt)
		return nil, "", err
	}
	return rt, runtimeHome, nil
}

func (r *Runtime) openBoxForHandle(ctx context.Context, h agentruntime.Handle) (sandbox.Instance, func(), error) {
	got, err := r.deps.ResolveAgent(h)
	if err != nil {
		return nil, nil, err
	}
	rt, runtimeHome, err := r.openSandboxRuntime(got.Name)
	if err != nil {
		return nil, nil, err
	}
	box, _, err := r.deps.ResolveBox(ctx, rt, got)
	if err != nil {
		_ = r.deps.CloseRuntime(runtimeHome, rt)
		return nil, nil, err
	}
	return box, func() {
		_ = r.deps.CloseBox(box)
		_ = r.deps.CloseRuntime(runtimeHome, rt)
	}, nil
}

func (r *Runtime) infoForHandle(ctx context.Context, h agentruntime.Handle) (agentruntime.Info, func(), error) {
	box, release, err := r.openBoxForHandle(ctx, h)
	if err != nil {
		return agentruntime.Info{}, nil, err
	}
	info, err := r.infoForBox(ctx, h, box)
	if err != nil {
		release()
		return agentruntime.Info{}, nil, err
	}
	return info, release, nil
}

func (r *Runtime) infoForBox(ctx context.Context, h agentruntime.Handle, box sandbox.Instance) (agentruntime.Info, error) {
	info, err := r.deps.BoxInfo(ctx, box)
	if err != nil {
		return agentruntime.Info{}, err
	}
	handle := agentruntime.Handle{
		RuntimeID: strings.TrimSpace(h.RuntimeID),
		HandleID:  strings.TrimSpace(info.ID),
	}
	if err := r.syncHandle(handle); err != nil {
		return agentruntime.Info{}, err
	}
	return agentruntime.Info{
		HandleID:  strings.TrimSpace(info.ID),
		State:     stateFromSandboxState(info.State),
		CreatedAt: info.CreatedAt,
	}, nil
}

func (r *Runtime) syncHandle(h agentruntime.Handle) error {
	if r.deps.SyncHandle == nil {
		return nil
	}
	return r.deps.SyncHandle(h)
}

func stateFromSandboxState(state sandbox.State) agentruntime.State {
	switch state {
	case sandbox.StateCreated:
		return agentruntime.StateCreated
	case sandbox.StateRunning:
		return agentruntime.StateRunning
	case sandbox.StateStopped:
		return agentruntime.StateStopped
	case sandbox.StateExited:
		return agentruntime.StateExited
	default:
		return agentruntime.StateUnknown
	}
}

func llmBridgeBaseURL(managerBaseURL, botID string) string {
	managerBaseURL = strings.TrimRight(strings.TrimSpace(managerBaseURL), "/")
	return managerBaseURL + "/api/bots/" + strings.TrimSpace(botID) + "/llm"
}

func cloneChannelsConfig(channels config.ChannelsConfig) config.ChannelsConfig {
	cloned := config.ChannelsConfig{
		FeishuAdminOpenID: channels.FeishuAdminOpenID,
	}
	if len(channels.Feishu) > 0 {
		cloned.Feishu = make(map[string]config.FeishuConfig, len(channels.Feishu))
		for name, feishu := range channels.Feishu {
			cloned.Feishu[name] = feishu
		}
	}
	return cloned
}
