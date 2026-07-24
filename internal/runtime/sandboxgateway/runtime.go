package sandboxgateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/hostuser"
)

var ErrPreparedGatewayProvisionNotAvailable = errors.New("gateway provision is not available")

type AgentRef struct {
	ID           string
	Name         string
	RuntimeID    string
	BoxID        string
	Instructions string
}

type WorkspaceLayout struct {
	MountHostPath      string
	MountGuestPath     string
	WorkspaceHostPath  string
	WorkspaceGuestPath string
	ExtraMounts        []sandbox.Mount
}

type Dependencies struct {
	RuntimeKind    string
	FeishuProvider feishu.AgentCredentialProvider

	SandboxProviderName func() string
	SandboxToolsDir     func() (string, error)
	AgentHome           func(agentID string) (string, error)
	EnsureRuntime       func(agentID string) (sandbox.Runtime, error)
	RuntimeHome         func(agentID string) (string, error)
	CloseRuntime        func(homeDir string, rt sandbox.Runtime) error
	ResolveBox          func(ctx context.Context, rt sandbox.Runtime, got AgentRef) (sandbox.Instance, string, error)
	CreateGatewayBox    func(ctx context.Context, rt sandbox.Runtime, image, name, botID string, profile agentruntime.Profile) (sandbox.Instance, sandbox.Info, error)
	CreateBox           func(ctx context.Context, rt sandbox.Runtime, spec sandbox.CreateSpec) (sandbox.Instance, error)
	StartBox            func(ctx context.Context, box sandbox.Instance) error
	StopBox             func(ctx context.Context, box sandbox.Instance, opts sandbox.StopOptions) error
	BoxInfo             func(ctx context.Context, box sandbox.Instance) (sandbox.Info, error)
	ForceRemoveBox      func(ctx context.Context, rt sandbox.Runtime, idOrName string) error
	CloseBox            func(box sandbox.Instance) error
	RunBoxCommand       func(ctx context.Context, box sandbox.Instance, name string, args []string, w io.Writer) (int, error)

	ResolveAgent       func(h agentruntime.Handle) (AgentRef, error)
	SyncHandle         func(h agentruntime.Handle) error
	BuildRuntimeEnv    func(baseURL, accessToken, participantID, agentID, llmBaseURL, modelID string, feishuProvider feishu.AgentCredentialProvider) map[string]string
	AddProfileEnv      func(envVars map[string]string, profileEnv map[string]string)
	HomeEnv            string
	MountGuestPath     string
	WorkspaceGuestPath string
	ProjectsGuestPath  string
	GatewayLogPath     string
	GatewayCommand     func() string
	ReadinessProbe     GatewayReadinessProbe
	StreamLogs         func(ctx context.Context, agentID string, follow bool, lines int, w io.Writer) error
}

const sandboxToolsGuestDir = "/opt/csgclaw/bin"

type GatewayReadinessProbe struct {
	Name     string
	Args     []string
	Timeout  time.Duration
	Interval time.Duration
}

const defaultGatewayReadinessTimeout = 5 * time.Minute

type Runtime struct {
	mu       sync.RWMutex
	deps     Dependencies
	prepared map[string]PreparedGatewayProvision
}

var (
	_ agentruntime.Runtime     = (*Runtime)(nil)
	_ agentruntime.LogStreamer = (*Runtime)(nil)
)

func New(deps Dependencies) *Runtime {
	return &Runtime{
		deps:     deps,
		prepared: make(map[string]PreparedGatewayProvision),
	}
}

func (r *Runtime) SetFeishuProvider(provider feishu.AgentCredentialProvider) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deps.FeishuProvider = provider
}

// CurrentFeishuProvider returns the provider used when provisioning or creating gateway boxes.
func (r *Runtime) CurrentFeishuProvider() feishu.AgentCredentialProvider {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.deps.FeishuProvider
}

func (r *Runtime) Kind() string {
	if kind := strings.TrimSpace(r.deps.RuntimeKind); kind != "" {
		return kind
	}
	return "sandbox_gateway"
}

func (r *Runtime) Layout(string) agentruntime.Layout {
	return agentruntime.Layout{}
}

func (r *Runtime) New(ctx context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
	agentName := strings.TrimSpace(spec.AgentName)
	agentID := strings.TrimSpace(spec.AgentID)
	if agentName == "" || agentID == "" {
		return agentruntime.Handle{}, fmt.Errorf("runtime agent name and id are required")
	}

	rt, runtimeHome, err := r.openSandboxRuntime(agentID)
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
		if err := r.waitForGatewayReady(ctx, box); err != nil {
			return agentruntime.StateUnknown, err
		}
		return info.State, nil
	}
	if err := r.deps.StartBox(ctx, box); err != nil {
		return agentruntime.StateUnknown, err
	}
	if err := r.waitForGatewayReady(ctx, box); err != nil {
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

	rt, runtimeHome, err := r.openSandboxRuntime(got.ID)
	if err != nil {
		return err
	}
	defer func() {
		_ = r.deps.CloseRuntime(runtimeHome, rt)
	}()

	boxIDOrName := ""
	box, resolvedKey, err := r.deps.ResolveBox(ctx, rt, got)
	if err == nil {
		if stopErr := r.deps.StopBox(ctx, box, sandbox.StopOptions{}); stopErr != nil && !sandbox.IsNotFound(stopErr) {
			_ = r.deps.CloseBox(box)
			return stopErr
		}
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
		boxIDOrName = agentruntime.SandboxNameForAgentID(got.ID)
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
	box, release, err := r.openBoxForHandle(ctx, h)
	if err != nil {
		return agentruntime.Info{}, err
	}
	defer release()
	info, err := r.infoForBox(ctx, h, box)
	if err != nil {
		return agentruntime.Info{}, err
	}
	if info.State == agentruntime.StateRunning {
		if err := r.checkGatewayReady(ctx, box); err != nil {
			return agentruntime.Info{}, err
		}
	}
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
		if removeErr := r.forceRemoveCreatedGatewayBox(ctx, rt, sandbox.Info{Name: spec.Name}, spec.Name); removeErr != nil {
			return nil, sandbox.Info{}, fmt.Errorf("read gateway box info: %w; cleanup gateway box: %v", err, removeErr)
		}
		return nil, sandbox.Info{}, fmt.Errorf("read gateway box info: %w", err)
	}
	if err := r.waitForGatewayReady(ctx, box); err != nil {
		_ = r.deps.CloseBox(box)
		if removeErr := r.forceRemoveCreatedGatewayBox(ctx, rt, info, spec.Name); removeErr != nil {
			return nil, sandbox.Info{}, fmt.Errorf("%w; cleanup gateway box: %v", err, removeErr)
		}
		return nil, sandbox.Info{}, err
	}
	return box, info, nil
}

func (r *Runtime) forceRemoveCreatedGatewayBox(ctx context.Context, rt sandbox.Runtime, info sandbox.Info, fallbackName string) error {
	if r == nil || r.deps.ForceRemoveBox == nil || rt == nil {
		return nil
	}
	target := strings.TrimSpace(info.ID)
	if target == "" {
		target = strings.TrimSpace(info.Name)
	}
	if target == "" {
		target = strings.TrimSpace(fallbackName)
	}
	if target == "" {
		return nil
	}
	if err := r.deps.ForceRemoveBox(ctx, rt, target); err != nil && !sandbox.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *Runtime) GatewayCreateSpec(image, name, botID string, profile agentruntime.Profile) (sandbox.CreateSpec, error) {
	prepared, err := r.preparedGatewayProvision(botID)
	if err != nil {
		return sandbox.CreateSpec{}, err
	}
	agentID := strings.TrimSpace(prepared.AgentID)
	if agentID == "" {
		agentID = strings.TrimSpace(botID)
	}
	participantID := strings.TrimSpace(prepared.ParticipantID)
	if participantID == "" {
		participantID = agentID
	}
	sandboxName := agentruntime.SandboxNameForAgentID(agentID)
	if sandboxName == "" {
		return sandbox.CreateSpec{}, fmt.Errorf("runtime sandbox name is required")
	}
	modelID := prepared.ModelID
	managerBaseURL := strings.TrimRight(strings.TrimSpace(prepared.ManagerBaseURL), "/")
	llmBaseURL := llmBridgeBaseURL(managerBaseURL, agentID)
	profile = prepared.Profile
	workspaceLayout := prepared.WorkspaceLayout
	projectsRoot := prepared.ProjectsRoot
	envVars := r.deps.BuildRuntimeEnv(managerBaseURL, prepared.Server.AccessToken, participantID, agentID, llmBaseURL, modelID, r.CurrentFeishuProvider())
	r.deps.AddProfileEnv(envVars, profile.Env)
	homeEnv := r.homeEnv()
	projectsGuestPath := r.projectsGuestPath()
	gatewayCommand := r.gatewayCommand()
	if homeEnv == "" {
		return sandbox.CreateSpec{}, fmt.Errorf("runtime HOME env is required")
	}
	if workspaceLayout.MountHostPath == "" {
		return sandbox.CreateSpec{}, fmt.Errorf("workspace mount host path is required")
	}
	if workspaceLayout.MountGuestPath == "" {
		return sandbox.CreateSpec{}, fmt.Errorf("workspace mount guest path is required")
	}
	if workspaceLayout.WorkspaceHostPath == "" {
		return sandbox.CreateSpec{}, fmt.Errorf("workspace host path is required")
	}
	if workspaceLayout.WorkspaceGuestPath == "" {
		return sandbox.CreateSpec{}, fmt.Errorf("workspace guest path is required")
	}
	if projectsGuestPath == "" {
		return sandbox.CreateSpec{}, fmt.Errorf("projects guest path is required")
	}
	if gatewayCommand == "" {
		return sandbox.CreateSpec{}, fmt.Errorf("gateway command is required")
	}
	toolsDir, mountTools, err := r.resolveSandboxToolsDir()
	if err != nil {
		return sandbox.CreateSpec{}, err
	}
	if mountTools {
		gatewayCommand = prependSandboxToolsToCommand(gatewayCommand)
	}
	envVars["HOME"] = homeEnv
	spec := sandbox.CreateSpec{
		Image:      image,
		Name:       sandboxName,
		Detach:     true,
		AutoRemove: false,
		Env:        envVars,
		Cmd:        []string{"/bin/sh", "-c", gatewayCommand},
	}
	if runUser, err := hostuser.RunUser(); err == nil {
		spec.RunUser = runUser
	}
	spec.Mounts = append(spec.Mounts,
		sandbox.Mount{HostPath: workspaceLayout.MountHostPath, GuestPath: workspaceLayout.MountGuestPath},
		sandbox.Mount{HostPath: projectsRoot, GuestPath: projectsGuestPath},
	)
	spec.Mounts = append(spec.Mounts, workspaceLayout.ExtraMounts...)
	if mountTools {
		spec.Mounts = append(spec.Mounts, sandbox.Mount{
			HostPath:  toolsDir,
			GuestPath: sandboxToolsGuestDir,
			ReadOnly:  true,
		})
	}
	return spec, nil
}

func prependSandboxToolsToCommand(command string) string {
	return `export PATH="` + sandboxToolsGuestDir + `${PATH:+:$PATH}"; ` + command
}

func (r *Runtime) resolveSandboxToolsDir() (string, bool, error) {
	if r == nil || r.deps.SandboxToolsDir == nil {
		return "", false, nil
	}
	provider := ""
	if r.deps.SandboxProviderName != nil {
		provider = strings.TrimSpace(r.deps.SandboxProviderName())
	}
	if provider != config.DockerProvider && provider != config.BoxLiteProvider && provider != config.CSGHubProvider {
		return "", false, nil
	}
	dir, err := r.deps.SandboxToolsDir()
	if err != nil {
		return "", false, fmt.Errorf("resolve sandbox tools directory: %w", err)
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", false, fmt.Errorf("sandbox tools directory is required")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", false, fmt.Errorf("stat sandbox tools directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return "", false, fmt.Errorf("sandbox tools path %q is not a directory", dir)
	}
	cliPath := filepath.Join(dir, "csgclaw-cli")
	cliInfo, err := os.Stat(cliPath)
	if err != nil {
		return "", false, fmt.Errorf("stat sandbox CLI %q: %w", cliPath, err)
	}
	if !cliInfo.Mode().IsRegular() {
		return "", false, fmt.Errorf("sandbox CLI %q is not a regular file", cliPath)
	}
	return dir, true, nil
}

type PreparedGatewayProvision struct {
	AgentID         string
	ParticipantID   string
	ModelID         string
	Profile         agentruntime.Profile
	WorkspaceLayout WorkspaceLayout
	ProjectsRoot    string
	ManagerBaseURL  string
	Server          config.ServerConfig
}

func FinalizePreparedGatewayProvision(req agentruntime.ProvisionRequest, workspaceLayout WorkspaceLayout) (PreparedGatewayProvision, error) {
	name := strings.TrimSpace(req.AgentName)
	agentID := strings.TrimSpace(req.AgentID)
	if name == "" || agentID == "" {
		return PreparedGatewayProvision{}, fmt.Errorf("runtime agent name and id are required")
	}
	participantID := strings.TrimSpace(req.ParticipantID)
	if participantID == "" {
		participantID = agentID
	}
	gateway := req.Gateway
	if gateway == nil {
		return PreparedGatewayProvision{}, fmt.Errorf("gateway provisioning data is required")
	}
	profile := req.Profile.Normalized()
	modelID := strings.TrimSpace(profile.ModelID)
	if modelID == "" {
		modelID = strings.TrimSpace(gateway.ModelFallback)
	}
	profile.ModelID = modelID
	workspaceLayout = normalizeWorkspaceLayout(Dependencies{}, workspaceLayout)
	if overlayRoot := strings.TrimSpace(req.WorkspaceOverlay); overlayRoot != "" {
		if err := OverlayWorkspaceTree(overlayRoot, workspaceLayout.WorkspaceHostPath); err != nil {
			return PreparedGatewayProvision{}, fmt.Errorf("overlay workspace for agent %q: %w", name, err)
		}
	}
	return PreparedGatewayProvision{
		AgentID:         agentID,
		ParticipantID:   participantID,
		ModelID:         modelID,
		Profile:         profile,
		WorkspaceLayout: workspaceLayout,
		ProjectsRoot:    strings.TrimSpace(gateway.ProjectsRoot),
		ManagerBaseURL:  strings.TrimRight(strings.TrimSpace(gateway.ManagerBaseURL), "/"),
		Server:          gateway.Server,
	}, nil
}

func (r *Runtime) RememberPreparedGatewayProvision(agentID string, prepared PreparedGatewayProvision) {
	if r == nil {
		return
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prepared[agentID] = prepared
}

func (r *Runtime) preparedGatewayProvision(agentID string) (PreparedGatewayProvision, error) {
	if r == nil {
		return PreparedGatewayProvision{}, fmt.Errorf("runtime is required")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return PreparedGatewayProvision{}, fmt.Errorf("runtime agent id is required")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	prepared, ok := r.prepared[agentID]
	if !ok {
		return PreparedGatewayProvision{}, fmt.Errorf("%w for agent %q; call Provision first", ErrPreparedGatewayProvisionNotAvailable, agentID)
	}
	return prepared, nil
}

func (r *Runtime) PreparedGatewayProvisionForHandle(h agentruntime.Handle) (PreparedGatewayProvision, error) {
	got, err := r.ResolveAgentForHandle(h)
	if err != nil {
		return PreparedGatewayProvision{}, err
	}
	return r.preparedGatewayProvision(got.ID)
}

func (r *Runtime) ResolveAgentForHandle(h agentruntime.Handle) (AgentRef, error) {
	if r == nil {
		return AgentRef{}, fmt.Errorf("runtime is required")
	}
	if r.deps.ResolveAgent == nil {
		return AgentRef{}, fmt.Errorf("runtime agent resolver is required")
	}
	return r.deps.ResolveAgent(h)
}

func (r *Runtime) AgentHomeForAgentID(agentID string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("runtime is required")
	}
	if r.deps.AgentHome == nil {
		return "", fmt.Errorf("runtime agent home resolver is required")
	}
	return r.deps.AgentHome(strings.TrimSpace(agentID))
}

func (r *Runtime) GatewayLogPath() string {
	return r.gatewayLogPath()
}

func (r *Runtime) homeEnv() string {
	return strings.TrimSpace(r.deps.HomeEnv)
}

func (r *Runtime) mountGuestPath() string {
	return strings.TrimSpace(r.deps.MountGuestPath)
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

func (r *Runtime) normalizeWorkspaceLayout(layout WorkspaceLayout) WorkspaceLayout {
	return normalizeWorkspaceLayout(r.deps, layout)
}

func normalizeWorkspaceLayout(deps Dependencies, layout WorkspaceLayout) WorkspaceLayout {
	layout.MountHostPath = strings.TrimSpace(layout.MountHostPath)
	layout.MountGuestPath = strings.TrimSpace(layout.MountGuestPath)
	layout.WorkspaceHostPath = strings.TrimSpace(layout.WorkspaceHostPath)
	layout.WorkspaceGuestPath = strings.TrimSpace(layout.WorkspaceGuestPath)
	normalizedMounts := make([]sandbox.Mount, 0, len(layout.ExtraMounts))
	for _, mount := range layout.ExtraMounts {
		mount.HostPath = strings.TrimSpace(mount.HostPath)
		mount.GuestPath = strings.TrimSpace(mount.GuestPath)
		if mount.HostPath != "" || mount.GuestPath != "" {
			normalizedMounts = append(normalizedMounts, mount)
		}
	}
	layout.ExtraMounts = normalizedMounts
	if layout.WorkspaceHostPath == "" {
		layout.WorkspaceHostPath = layout.MountHostPath
	}
	if layout.MountGuestPath == "" {
		layout.MountGuestPath = strings.TrimSpace(deps.MountGuestPath)
	}
	if layout.WorkspaceGuestPath == "" {
		layout.WorkspaceGuestPath = strings.TrimSpace(deps.WorkspaceGuestPath)
	}
	return layout
}

func (r *Runtime) openSandboxRuntime(agentID string) (sandbox.Runtime, string, error) {
	rt, err := r.deps.EnsureRuntime(agentID)
	if err != nil {
		return nil, "", err
	}
	runtimeHome, err := r.deps.RuntimeHome(agentID)
	if err != nil {
		_ = r.deps.CloseRuntime("", rt)
		return nil, "", err
	}
	return rt, runtimeHome, nil
}

func (r *Runtime) AgentHomeForHandle(h agentruntime.Handle) (string, error) {
	got, err := r.ResolveAgentForHandle(h)
	if err != nil {
		return "", err
	}
	return r.AgentHomeForAgentID(got.ID)
}

func (r *Runtime) openBoxForHandle(ctx context.Context, h agentruntime.Handle) (sandbox.Instance, func(), error) {
	got, err := r.deps.ResolveAgent(h)
	if err != nil {
		return nil, nil, err
	}
	rt, runtimeHome, err := r.openSandboxRuntime(got.ID)
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

func (r *Runtime) waitForGatewayReady(ctx context.Context, box sandbox.Instance) error {
	if r == nil || box == nil || r.deps.RunBoxCommand == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(r.sandboxProviderName()), "docker") {
		return nil
	}
	probe := r.deps.ReadinessProbe
	probe.Name = strings.TrimSpace(probe.Name)
	if probe.Name == "" {
		return nil
	}
	timeout := probe.Timeout
	if timeout <= 0 {
		timeout = defaultGatewayReadinessTimeout
	}
	interval := probe.Interval
	if interval <= 0 {
		interval = time.Second
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastErr error
	for {
		var out bytes.Buffer
		exitCode, err := r.deps.RunBoxCommand(waitCtx, box, probe.Name, probe.Args, &out)
		if err == nil && exitCode == 0 {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("%s exited with code %d: %s", probe.Name, exitCode, strings.TrimSpace(out.String()))
		}
		if stoppedErr := r.gatewayStoppedError(waitCtx, box, lastErr); stoppedErr != nil {
			return stoppedErr
		}

		select {
		case <-waitCtx.Done():
			if lastErr != nil {
				return fmt.Errorf("wait %s gateway ready: %w", r.Kind(), lastErr)
			}
			return fmt.Errorf("wait %s gateway ready exceeded deadline (%s): %w", r.Kind(), timeout, waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func (r *Runtime) checkGatewayReady(ctx context.Context, box sandbox.Instance) error {
	if r == nil || box == nil || r.deps.RunBoxCommand == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(r.sandboxProviderName()), "docker") {
		return nil
	}
	probe := r.deps.ReadinessProbe
	probe.Name = strings.TrimSpace(probe.Name)
	if probe.Name == "" {
		return nil
	}

	var out bytes.Buffer
	exitCode, err := r.deps.RunBoxCommand(ctx, box, probe.Name, probe.Args, &out)
	if err == nil && exitCode == 0 {
		return nil
	}
	if err == nil {
		err = fmt.Errorf("%s exited with code %d: %s", probe.Name, exitCode, strings.TrimSpace(out.String()))
	}
	if stoppedErr := r.gatewayStoppedError(ctx, box, err); stoppedErr != nil {
		return stoppedErr
	}
	return fmt.Errorf("check %s gateway ready: %w", r.Kind(), err)
}

func (r *Runtime) gatewayStoppedError(ctx context.Context, box sandbox.Instance, lastErr error) error {
	if r == nil || r.deps.BoxInfo == nil || box == nil {
		return nil
	}
	info, err := r.deps.BoxInfo(ctx, box)
	if err != nil {
		return nil
	}
	if info.State != sandbox.StateStopped && info.State != sandbox.StateExited {
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("wait %s gateway ready: sandbox %s before ready: %w", r.Kind(), info.State, lastErr)
	}
	return fmt.Errorf("wait %s gateway ready: sandbox %s before ready", r.Kind(), info.State)
}

func (r *Runtime) sandboxProviderName() string {
	if r == nil || r.deps.SandboxProviderName == nil {
		return ""
	}
	return strings.TrimSpace(r.deps.SandboxProviderName())
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

func llmBridgeBaseURL(managerBaseURL, agentID string) string {
	managerBaseURL = strings.TrimRight(strings.TrimSpace(managerBaseURL), "/")
	return managerBaseURL + "/api/v1/agents/" + strings.TrimSpace(agentID) + "/llm"
}
