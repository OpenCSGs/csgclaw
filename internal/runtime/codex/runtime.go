package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agent"
	"csgclaw/internal/codexmodel"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
)

const (
	hostStateDirName       = ".codex"
	configFileName         = "config.toml"
	modelCatalogFileName   = "model_catalog.json"
	runtimeFileName        = "runtime.json"
	sessionFileName        = "session.json"
	stderrLogFileName      = "stderr.log"
	workspaceDirName       = "workspace"
	homeDirName            = "home"
	logPollInterval        = 200 * time.Millisecond
	removeAllRetryAttempts = 12
	codexProxyProviderName = "proxy"
	codexModelProviderName = "codex"
)

type AgentRef struct {
	ID             string
	Name           string
	RuntimeID      string
	HandleID       string
	Instructions   string
	RuntimeOptions map[string]any
	Profile        agentruntime.Profile
}

type SessionSpec struct {
	RuntimeID    string
	AgentID      string
	AgentName    string
	BinaryPath   string
	RuntimeDir   string
	WorkspaceDir string
	HomeDir      string
	CodexHomeDir string
	StderrPath   string
	Profile      agentruntime.Profile
}

type SessionHandle struct {
	RuntimeID string
}

type Session struct {
	RuntimeID         string
	AgentID           string
	AgentName         string
	SessionID         string
	BinaryPath        string
	RuntimeDir        string
	WorkspaceDir      string
	HomeDir           string
	CodexHomeDir      string
	StderrPath        string
	ProcessID         int
	CreatedAt         time.Time
	StartedAt         time.Time
	AgentCapabilities any
}

type Manager interface {
	Start(ctx context.Context, spec SessionSpec) (*Session, error)
	Stop(ctx context.Context, handle SessionHandle) error
	Session(handle SessionHandle) (*Session, error)
	Prompt(ctx context.Context, handle SessionHandle, req PromptRequest) (PromptResponse, error)
}

type SessionEventKind = activity.RuntimeEventKind

const (
	SessionEventUserMessageDelta   = activity.RuntimeEventUserMessageDelta
	SessionEventTextDelta          = activity.RuntimeEventTextDelta
	SessionEventThoughtDelta       = activity.RuntimeEventThoughtDelta
	SessionEventToolCallStart      = activity.RuntimeEventToolCallStart
	SessionEventToolCallUpdate     = activity.RuntimeEventToolCallUpdate
	SessionEventPlanUpdate         = activity.RuntimeEventPlanUpdate
	SessionEventPermissionRequest  = activity.RuntimeEventActionRequest
	SessionEventPermissionDecision = activity.RuntimeEventActionDecision
	SessionEventPromptCompleted    = activity.RuntimeEventPromptCompleted
	SessionEventPromptFailed       = activity.RuntimeEventPromptFailed
)

type SessionEvent = activity.RuntimeEvent

type SessionEventSink = activity.RuntimeEventSink

type SessionEventSubscriber = activity.RuntimeEventSubscriber

type BinaryProvider interface {
	Ensure(ctx context.Context) (string, error)
}

type Dependencies struct {
	BinaryProvider BinaryProvider
	ResolveAgent   func(h agentruntime.Handle) (AgentRef, error)
	AgentHome      func(agentID string) (string, error)
	Manager        Manager
	EventSink      SessionEventSink
	Permission     PermissionBroker

	MkdirAll  func(string, os.FileMode) error
	ReadFile  func(string) ([]byte, error)
	WriteFile func(string, []byte, os.FileMode) error
	Stat      func(string) (os.FileInfo, error)
	RemoveAll func(string) error
	OpenFile  func(string, int, os.FileMode) (*os.File, error)
}

type Runtime struct {
	deps Dependencies
}

var (
	_ agentruntime.Runtime                     = (*Runtime)(nil)
	_ agentruntime.LogStreamer                 = (*Runtime)(nil)
	_ agentruntime.ConversationStarter         = (*Runtime)(nil)
	_ agentruntime.RuntimeOptionSchemaProvider = (*Runtime)(nil)
	_ agentruntime.RuntimeConfigController     = (*Runtime)(nil)
)

func New(deps Dependencies) *Runtime {
	return &Runtime{deps: deps}
}

func (r *Runtime) Kind() string {
	return agentruntime.KindCodex
}

func workspaceRoot(agentHome string) string {
	return filepath.Join(agentHome, filepath.FromSlash(hostStateDirName), workspaceDirName)
}

func (r *Runtime) WorkspaceRoot(agentHome string) string {
	return r.Layout(agentHome).WorkspaceRoot
}

func canonicalRuntimeAgentID(agentID string) string {
	return agent.CanonicalID(agentID)
}

func (r *Runtime) resolveWorkspaceDir(agentID string, runtimeOptions map[string]any) (string, error) {
	if r.deps.AgentHome == nil {
		return "", fmt.Errorf("agent home resolver is required")
	}
	agentHome, err := r.deps.AgentHome(canonicalRuntimeAgentID(agentID))
	if err != nil {
		return "", err
	}
	return ResolveWorkspaceDir(agentHome, runtimeOptions)
}

func (r *Runtime) Layout(agentHome string) agentruntime.Layout {
	root := filepath.Join(agentHome, filepath.FromSlash(hostStateDirName))
	return agentruntime.Layout{
		WorkspaceRoot: filepath.Join(root, workspaceDirName),
		SkillsRoot:    filepath.Join(root, homeDirName, "skills"),
		HostLogPaths:  []string{filepath.Join(root, homeDirName, stderrLogFileName)},
	}
}

func (r *Runtime) SessionManager() Manager {
	return r.sessionManager()
}

func (r *Runtime) EventSink() SessionEventSink {
	return r.deps.EventSink
}

func (r *Runtime) PermissionBroker() PermissionBroker {
	return r.permissionBroker()
}

func (r *Runtime) New(ctx context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
	spec.AgentID = canonicalRuntimeAgentID(spec.AgentID)
	if err := r.ensureRuntimeHome(spec.AgentID); err != nil {
		return agentruntime.Handle{}, err
	}
	spec.Profile = spec.Profile.Normalized()
	session, err := r.ensureSession(ctx, SessionSpec{
		RuntimeID: strings.TrimSpace(spec.RuntimeID),
		AgentID:   strings.TrimSpace(spec.AgentID),
		AgentName: strings.TrimSpace(spec.AgentName),
		Profile:   spec.Profile,
	})
	if err != nil {
		return agentruntime.Handle{}, err
	}
	return agentruntime.Handle{
		RuntimeID: strings.TrimSpace(spec.RuntimeID),
		HandleID:  strings.TrimSpace(session.SessionID),
	}, nil
}

func (r *Runtime) Start(ctx context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	if current, err := r.Info(ctx, h); err == nil && current.State == agentruntime.StateRunning {
		return current.State, nil
	}

	agentRef, err := r.resolveAgent(h)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	agentRef.Profile = agentRef.Profile.Normalized()
	session, err := r.ensureSession(ctx, SessionSpec{
		RuntimeID: strings.TrimSpace(h.RuntimeID),
		AgentID:   strings.TrimSpace(agentRef.ID),
		AgentName: strings.TrimSpace(agentRef.Name),
		Profile:   agentRef.Profile,
	})
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	if err := r.writeMetadata(sessionToRuntimeMetadata(session)); err != nil {
		return agentruntime.StateUnknown, err
	}
	return agentruntime.StateRunning, nil
}

func (r *Runtime) Stop(ctx context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	meta, err := r.readRuntimeMetadata(h.RuntimeID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return agentruntime.StateUnknown, sandbox.ErrNotFound
		}
		return agentruntime.StateUnknown, err
	}
	if err := r.sessionManager().Stop(ctx, SessionHandle{RuntimeID: strings.TrimSpace(h.RuntimeID)}); err != nil && !errors.Is(err, os.ErrNotExist) {
		return agentruntime.StateUnknown, err
	}
	if meta.ProcessID > 0 {
		if err := stopProcess(meta.ProcessID); err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ESRCH) {
			return agentruntime.StateUnknown, err
		}
	}
	meta.ProcessID = 0
	meta.State = agentruntime.StateStopped
	meta.StoppedAt = time.Now().UTC()
	if err := r.writeMetadata(meta); err != nil {
		return agentruntime.StateUnknown, err
	}
	return agentruntime.StateStopped, nil
}

func (r *Runtime) Delete(ctx context.Context, h agentruntime.Handle) error {
	runtimeID := strings.TrimSpace(h.RuntimeID)
	if runtimeID == "" {
		return fmt.Errorf("runtime id is required")
	}
	_, _ = r.Stop(ctx, h)
	dir, err := r.runtimeDirForHandle(h)
	if err != nil {
		return err
	}
	if err := r.removeAllWithRetry(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (r *Runtime) State(ctx context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	info, err := r.Info(ctx, h)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	return info.State, nil
}

func (r *Runtime) Info(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
	meta, err := r.readRuntimeMetadata(h.RuntimeID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return agentruntime.Info{}, sandbox.ErrNotFound
		}
		return agentruntime.Info{}, err
	}
	state := normalizeRuntimeState(meta.State)
	if state == agentruntime.StateRunning && !processAlive(meta.ProcessID) {
		if meta.ExitCode != 0 {
			state = agentruntime.StateFailed
		} else {
			state = agentruntime.StateExited
		}
		meta.State = state
		meta.ProcessID = 0
		_ = r.writeMetadata(meta)
	}
	return agentruntime.Info{
		HandleID:  strings.TrimSpace(meta.SessionID),
		State:     state,
		CreatedAt: meta.CreatedAt,
	}, nil
}

func (r *Runtime) StreamLogs(ctx context.Context, h agentruntime.Handle, opts agentruntime.LogOptions) error {
	logPath, err := r.stderrLogPath(h)
	if err != nil {
		return err
	}
	lines := opts.Tail
	if lines <= 0 {
		lines = 20
	}
	return streamLogFile(ctx, logPath, opts.Follow, lines, opts.Writer)
}

func (r *Runtime) NewConversation(ctx context.Context, h agentruntime.Handle, req agentruntime.ConversationStartRequest) (agentruntime.ConversationStartAction, error) {
	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" {
		return agentruntime.ConversationStartAction{}, fmt.Errorf("room id is required")
	}
	if strings.TrimSpace(h.RuntimeID) == "" {
		return agentruntime.ConversationStartAction{}, fmt.Errorf("runtime id is required")
	}
	return agentruntime.ConversationStartAction{
		Mode:    agentruntime.ConversationStartActionInternal,
		AckText: "Cleared my internal history for this conversation. The IM room messages were not cleared.",
	}, nil
}

func (r *Runtime) sessionManager() Manager {
	if r.deps.Manager != nil {
		return r.deps.Manager
	}
	manager := newAppServerManager(managerDeps{
		EventSink:  r.deps.EventSink,
		Permission: r.permissionBroker(),
		OpenFile:   r.openFile,
		WriteFile:  r.writeFile,
		ReadFile:   r.readFile,
		OnExit: func(session *Session, exitCode int) {
			if session == nil {
				return
			}
			meta := sessionToRuntimeMetadata(session)
			meta.ProcessID = 0
			meta.ExitCode = exitCode
			meta.StoppedAt = time.Now().UTC()
			if exitCode != 0 {
				meta.State = agentruntime.StateFailed
			} else {
				meta.State = agentruntime.StateExited
			}
			_ = writeJSONFile(r.writeFile, filepath.Join(session.RuntimeDir, runtimeFileName), meta)
		},
	})
	manager.deps.HydrateSession = func(ctx context.Context, handle SessionHandle) (*Session, error) {
		return r.hydratePersistedSession(ctx, manager, handle)
	}
	r.deps.Manager = manager
	return r.deps.Manager
}

func (r *Runtime) permissionBroker() PermissionBroker {
	if r.deps.Permission != nil {
		return r.deps.Permission
	}
	r.deps.Permission = NewPermissionBroker(r.deps.EventSink)
	return r.deps.Permission
}

func (r *Runtime) ensureSession(ctx context.Context, spec SessionSpec) (*Session, error) {
	runtimeID := strings.TrimSpace(spec.RuntimeID)
	if runtimeID == "" {
		return nil, fmt.Errorf("runtime id is required")
	}
	if strings.TrimSpace(spec.AgentName) == "" || strings.TrimSpace(spec.AgentID) == "" {
		return nil, fmt.Errorf("agent name and id are required")
	}
	spec.AgentID = canonicalRuntimeAgentID(spec.AgentID)
	agentRef, err := r.resolveAgent(agentruntime.Handle{RuntimeID: runtimeID})
	if err != nil {
		return nil, err
	}
	dirs, err := r.ensureRuntimeDirs(spec.AgentID)
	if err != nil {
		return nil, err
	}
	workspaceDir, err := r.resolveWorkspaceDir(spec.AgentID, agentRef.RuntimeOptions)
	if err != nil {
		return nil, err
	}
	spec.RuntimeDir = dirs.Root
	spec.WorkspaceDir = workspaceDir
	spec.HomeDir = r.hostSessionHomeDir(dirs.Home)
	spec.CodexHomeDir = dirs.CodexHome
	spec.StderrPath = dirs.StderrLog
	if err := r.mkdirAll(spec.WorkspaceDir, 0o755); err != nil {
		return nil, fmt.Errorf("create codex workspace dir %s: %w", spec.WorkspaceDir, err)
	}
	if err := r.seedCodexHomeAuth(spec.CodexHomeDir); err != nil {
		return nil, err
	}
	if err := r.seedCodexHomeConfig(spec.CodexHomeDir, spec.Profile); err != nil {
		return nil, err
	}
	if err := r.seedCodexHomeSkills(spec.CodexHomeDir); err != nil {
		return nil, err
	}
	if err := r.refreshCodexHomeAgentsFile(agentruntime.Handle{RuntimeID: runtimeID}, spec.CodexHomeDir); err != nil {
		return nil, err
	}
	if strings.TrimSpace(spec.BinaryPath) == "" {
		binaryPath, err := r.ensureBinary(ctx)
		if err != nil {
			return nil, err
		}
		spec.BinaryPath = binaryPath
	}
	if ctx == nil {
		ctx = context.Background()
	}
	session, err := r.sessionManager().Start(context.WithoutCancel(ctx), spec)
	if err != nil {
		return nil, err
	}
	if err := writeJSONFile(r.writeFile, filepath.Join(spec.RuntimeDir, sessionFileName), sessionToSessionMetadata(session)); err != nil {
		return nil, err
	}
	if err := writeJSONFile(r.writeFile, filepath.Join(spec.RuntimeDir, runtimeFileName), sessionToRuntimeMetadata(session)); err != nil {
		return nil, err
	}
	return session, nil
}

func (r *Runtime) hydratePersistedSession(ctx context.Context, manager *appServerManager, handle SessionHandle) (*Session, error) {
	if manager == nil {
		return nil, os.ErrNotExist
	}
	runtimeID := strings.TrimSpace(handle.RuntimeID)
	if runtimeID == "" {
		return nil, fmt.Errorf("runtime id is required")
	}
	meta, err := r.readRuntimeMetadata(runtimeID)
	if err != nil {
		return nil, err
	}
	if _, err := r.readSessionMetadata(runtimeID); err != nil {
		return nil, err
	}
	agentRef, err := r.resolveAgent(agentruntime.Handle{RuntimeID: runtimeID})
	if err != nil {
		return nil, err
	}
	agentID := canonicalRuntimeAgentID(firstNonEmpty(agentRef.ID, meta.AgentID))
	dirs, err := r.ensureRuntimeDirs(agentID)
	if err != nil {
		return nil, err
	}
	workspaceDir, err := r.resolveWorkspaceDir(agentID, agentRef.RuntimeOptions)
	if err != nil {
		return nil, err
	}
	if meta.ProcessID > 0 && processAlive(meta.ProcessID) {
		if err := stopProcess(meta.ProcessID); err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ESRCH) {
			return nil, err
		}
	}

	spec := SessionSpec{
		RuntimeID:    runtimeID,
		AgentID:      agentID,
		AgentName:    firstNonEmpty(agentRef.Name, meta.AgentName),
		BinaryPath:   strings.TrimSpace(meta.BinaryPath),
		RuntimeDir:   dirs.Root,
		WorkspaceDir: workspaceDir,
		HomeDir:      r.hostSessionHomeDir(dirs.Home),
		CodexHomeDir: dirs.CodexHome,
		StderrPath:   dirs.StderrLog,
		Profile:      agentRef.Profile.Normalized(),
	}
	if err := r.mkdirAll(spec.WorkspaceDir, 0o755); err != nil {
		return nil, fmt.Errorf("create codex workspace dir %s: %w", spec.WorkspaceDir, err)
	}
	if err := r.seedCodexHomeAuth(spec.CodexHomeDir); err != nil {
		return nil, err
	}
	if err := r.seedCodexHomeConfig(spec.CodexHomeDir, spec.Profile); err != nil {
		return nil, err
	}
	if err := r.seedCodexHomeSkills(spec.CodexHomeDir); err != nil {
		return nil, err
	}
	if err := r.refreshCodexHomeAgentsFile(agentruntime.Handle{RuntimeID: runtimeID}, spec.CodexHomeDir); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	session, err := manager.Start(context.WithoutCancel(ctx), spec)
	if err != nil {
		return nil, err
	}
	if !meta.CreatedAt.IsZero() {
		session.CreatedAt = meta.CreatedAt.UTC()
	}
	if err := r.writeSessionMetadata(sessionToSessionMetadata(session)); err != nil {
		return nil, err
	}
	if err := r.writeMetadata(sessionToRuntimeMetadata(session)); err != nil {
		return nil, err
	}
	return session, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (r *Runtime) seedCodexHomeAuth(runtimeCodexHome string) error {
	runtimeCodexHome = strings.TrimSpace(runtimeCodexHome)
	if runtimeCodexHome == "" {
		return fmt.Errorf("codex home dir is required")
	}

	runtimeAuthPath := filepath.Join(runtimeCodexHome, "auth.json")
	if _, err := r.readFile(runtimeAuthPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read runtime codex auth %s: %w", runtimeAuthPath, err)
	}

	hostAuthPath, err := hostCodexAuthPath()
	if err != nil {
		return nil
	}
	raw, err := os.ReadFile(hostAuthPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read host codex auth %s: %w", hostAuthPath, err)
	}
	if err := r.writeFile(runtimeAuthPath, raw, 0o600); err != nil {
		return fmt.Errorf("seed runtime codex auth %s: %w", runtimeAuthPath, err)
	}
	return nil
}

func (r *Runtime) seedCodexHomeConfig(runtimeCodexHome string, profile agentruntime.Profile) error {
	runtimeCodexHome = strings.TrimSpace(runtimeCodexHome)
	if runtimeCodexHome == "" {
		return fmt.Errorf("codex home dir is required")
	}
	configPath := filepath.Join(runtimeCodexHome, configFileName)
	profile = profile.Normalized()
	configRaw, err := r.readFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read runtime codex config %s: %w", configPath, err)
	}
	if errors.Is(err, os.ErrNotExist) {
		hostRaw, hostErr := r.hostCodexConfig()
		if hostErr == nil {
			configRaw = hostRaw
		} else if !errors.Is(hostErr, os.ErrNotExist) {
			return fmt.Errorf("read host codex config: %w", hostErr)
		}
	}

	if profile.BaseURL != "" && profile.ModelID != "" {
		if err := r.writeModelCatalog(runtimeCodexHome, profile); err != nil {
			return err
		}
	}

	rendered := configureCodexHomeConfig(string(configRaw), profile)
	if err := r.writeFile(configPath, []byte(rendered), 0o600); err != nil {
		return fmt.Errorf("write runtime codex config %s: %w", configPath, err)
	}
	return nil
}

func (r *Runtime) seedCodexHomeSkills(runtimeCodexHome string) error {
	runtimeCodexHome = strings.TrimSpace(runtimeCodexHome)
	if runtimeCodexHome == "" {
		return fmt.Errorf("codex home dir is required")
	}

	targetRoot := filepath.Join(runtimeCodexHome, "skills")
	if err := r.removeAll(targetRoot); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove runtime codex skills %s: %w", targetRoot, err)
	}

	sourceRoot, err := hostCodexSkillsPath()
	if err != nil {
		return nil
	}
	info, err := os.Stat(sourceRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat host codex skills %s: %w", sourceRoot, err)
	}
	if !info.IsDir() {
		return nil
	}

	if err := r.copyDir(sourceRoot, targetRoot); err != nil {
		return fmt.Errorf("seed runtime codex skills %s: %w", targetRoot, err)
	}
	return nil
}

func (r *Runtime) writeModelCatalog(runtimeCodexHome string, profile agentruntime.Profile) error {
	catalogPath := filepath.Join(runtimeCodexHome, modelCatalogFileName)
	body, err := json.MarshalIndent(codexmodel.Catalog(codexmodel.Profile{
		ModelID:         profile.ModelID,
		ReasoningEffort: profile.ReasoningEffort,
	}), "", "  ")
	if err != nil {
		return fmt.Errorf("encode runtime codex model catalog: %w", err)
	}
	body = append(body, '\n')
	if err := r.writeFile(catalogPath, body, 0o600); err != nil {
		return fmt.Errorf("write runtime codex model catalog %s: %w", catalogPath, err)
	}
	return nil
}

func (r *Runtime) ensureBinary(ctx context.Context) (string, error) {
	if r.deps.BinaryProvider == nil {
		return "", fmt.Errorf("codex binary provider is required")
	}
	return r.deps.BinaryProvider.Ensure(ctx)
}

func (r *Runtime) ensureRuntimeHome(agentID string) error {
	_, err := r.ensureRuntimeDirs(agentID)
	return err
}

func (r *Runtime) hostSessionHomeDir(fallback string) string {
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return home
	}
	return fallback
}

func (r *Runtime) ensureRuntimeDirs(agentID string) (runtimeDirs, error) {
	root, err := r.runtimeDirForAgent(agentID)
	if err != nil {
		return runtimeDirs{}, err
	}
	dirs := runtimeDirs{
		Root:      root,
		Workspace: filepath.Join(root, workspaceDirName),
		Home:      filepath.Join(root, homeDirName),
		CodexHome: filepath.Join(root, homeDirName),
		StderrLog: filepath.Join(root, homeDirName, stderrLogFileName),
	}
	for _, path := range []string{dirs.Root, dirs.Workspace, dirs.Home} {
		if err := r.mkdirAll(path, 0o755); err != nil {
			return runtimeDirs{}, fmt.Errorf("create codex runtime dir %s: %w", path, err)
		}
	}
	return dirs, nil
}

func (r *Runtime) runtimeDirForHandle(h agentruntime.Handle) (string, error) {
	agentRef, err := r.resolveAgent(h)
	if err != nil {
		return "", err
	}
	return r.runtimeDirForAgent(agentRef.ID)
}

func (r *Runtime) runtimeDirForAgent(agentID string) (string, error) {
	if r.deps.AgentHome == nil {
		return "", fmt.Errorf("agent home resolver is required")
	}
	agentHome, err := r.deps.AgentHome(canonicalRuntimeAgentID(agentID))
	if err != nil {
		return "", err
	}
	return filepath.Join(agentHome, filepath.FromSlash(hostStateDirName)), nil
}

func (r *Runtime) stderrLogPath(h agentruntime.Handle) (string, error) {
	agentRef, err := r.resolveAgent(h)
	if err != nil {
		return "", err
	}
	if r.deps.AgentHome == nil {
		return "", fmt.Errorf("agent home resolver is required")
	}
	agentHome, err := r.deps.AgentHome(canonicalRuntimeAgentID(agentRef.ID))
	if err != nil {
		return "", err
	}
	layout := r.Layout(agentHome)
	if len(layout.HostLogPaths) == 0 {
		return "", fmt.Errorf("codex runtime host log path is required")
	}
	return layout.HostLogPaths[0], nil
}

func (r *Runtime) resolveAgent(h agentruntime.Handle) (AgentRef, error) {
	if r.deps.ResolveAgent == nil {
		return AgentRef{}, fmt.Errorf("agent resolver is required")
	}
	agentRef, err := r.deps.ResolveAgent(h)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AgentRef{}, sandbox.ErrNotFound
		}
		return AgentRef{}, err
	}
	if strings.TrimSpace(agentRef.Name) == "" || strings.TrimSpace(agentRef.ID) == "" {
		return AgentRef{}, fmt.Errorf("resolved agent is incomplete")
	}
	agentRef.ID = canonicalRuntimeAgentID(agentRef.ID)
	return agentRef, nil
}

func (r *Runtime) RefreshCodexHomeAgentsFile(_ context.Context, h agentruntime.Handle) error {
	agentRef, err := r.resolveAgent(h)
	if err != nil {
		return err
	}
	codexHomeDir, err := r.resolveCodexHomeDir(agentRef.ID)
	if err != nil {
		return err
	}
	return r.refreshCodexHomeAgentsFile(h, codexHomeDir)
}

func (r *Runtime) resolveCodexHomeDir(agentID string) (string, error) {
	dirs, err := r.ensureRuntimeDirs(agentID)
	if err != nil {
		return "", err
	}
	return dirs.CodexHome, nil
}

func (r *Runtime) refreshCodexHomeAgentsFile(h agentruntime.Handle, codexHomeDir string) error {
	codexHomeDir = strings.TrimSpace(codexHomeDir)
	if codexHomeDir == "" {
		return fmt.Errorf("codex home dir is required")
	}
	if err := r.mkdirAll(codexHomeDir, 0o755); err != nil {
		return fmt.Errorf("create codex home dir %s: %w", codexHomeDir, err)
	}
	agentRef, err := r.resolveAgent(h)
	if err != nil {
		return err
	}
	path := filepath.Join(codexHomeDir, "AGENTS.md")
	block := agent.RenderAgentsInstructionsBlock(agentRef.Instructions)
	current, err := r.readFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read codex home AGENTS.md %s: %w", path, err)
	}
	merged := mergeAgentsInstructionsBlock(string(current), block)
	if err == nil && string(current) == merged {
		return nil
	}
	if err := r.writeFile(path, []byte(merged), 0o644); err != nil {
		return fmt.Errorf("write codex home AGENTS.md %s: %w", path, err)
	}
	return nil
}

func (r *Runtime) SyncWorkspaceAgentsFile(ctx context.Context, h agentruntime.Handle, previousRuntimeOptions map[string]any) error {
	_ = previousRuntimeOptions
	return r.RefreshCodexHomeAgentsFile(ctx, h)
}

func mergeAgentsInstructionsBlock(current, block string) string {
	start, end := agent.AgentsInstructionsBlockMarkers()
	current = strings.ReplaceAll(current, "\r\n", "\n")
	block = strings.TrimRight(strings.ReplaceAll(block, "\r\n", "\n"), "\n")

	if replaced, ok := replaceAgentsInstructionsBlock(current, start, end, block); ok {
		return replaced
	}
	if strings.TrimSpace(current) == "" {
		return block + "\n"
	}
	return joinAgentsInstructionsSections(current, block, "")
}

func replaceAgentsInstructionsBlock(current, start, end, block string) (string, bool) {
	startIdx := strings.Index(current, start)
	if startIdx < 0 {
		return "", false
	}
	endIdx := strings.Index(current[startIdx:], end)
	if endIdx < 0 {
		return joinAgentsInstructionsSections(current[:startIdx], block, ""), true
	}
	endPos := startIdx + endIdx + len(end)
	prefix := current[:startIdx]
	suffix := current[endPos:]
	return joinAgentsInstructionsSections(prefix, block, suffix), true
}

func removeAgentsInstructionsBlock(current string) (string, bool) {
	start, end := agent.AgentsInstructionsBlockMarkers()
	current = strings.ReplaceAll(current, "\r\n", "\n")
	startIdx := strings.Index(current, start)
	if startIdx < 0 {
		return "", false
	}
	endIdx := strings.Index(current[startIdx:], end)
	if endIdx < 0 {
		return joinAgentsInstructionsSections(current[:startIdx]), true
	}
	endPos := startIdx + endIdx + len(end)
	return joinAgentsInstructionsSections(current[:startIdx], current[endPos:]), true
}

func joinAgentsInstructionsSections(parts ...string) string {
	sections := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.ReplaceAll(part, "\r\n", "\n"))
		if part == "" {
			continue
		}
		sections = append(sections, part)
	}
	if len(sections) == 0 {
		return ""
	}
	return strings.Join(sections, "\n\n") + "\n"
}

func (r *Runtime) readRuntimeMetadata(runtimeID string) (runtimeMetadata, error) {
	path, err := r.runtimeMetadataPath(runtimeID)
	if err != nil {
		return runtimeMetadata{}, err
	}
	var meta runtimeMetadata
	if err := readJSONFile(r.readFile, path, &meta); err != nil {
		return runtimeMetadata{}, err
	}
	return normalizeRuntimeMetadata(meta), nil
}

func (r *Runtime) readSessionMetadata(runtimeID string) (sessionMetadata, error) {
	path, err := r.sessionMetadataPath(runtimeID)
	if err != nil {
		return sessionMetadata{}, err
	}
	var meta sessionMetadata
	if err := readJSONFile(r.readFile, path, &meta); err != nil {
		return sessionMetadata{}, err
	}
	return normalizeSessionMetadata(meta), nil
}

func (r *Runtime) writeMetadata(meta runtimeMetadata) error {
	path, err := r.runtimeMetadataPath(meta.RuntimeID)
	if err != nil {
		return err
	}
	meta = normalizeRuntimeMetadata(meta)
	return writeJSONFile(r.writeFile, path, meta)
}

func (r *Runtime) writeSessionMetadata(meta sessionMetadata) error {
	path, err := r.sessionMetadataPath(meta.RuntimeID)
	if err != nil {
		return err
	}
	meta = normalizeSessionMetadata(meta)
	return writeJSONFile(r.writeFile, path, meta)
}

func (r *Runtime) runtimeMetadataPath(runtimeID string) (string, error) {
	root, err := r.runtimeDirForHandle(agentruntime.Handle{RuntimeID: strings.TrimSpace(runtimeID)})
	if err != nil {
		return "", err
	}
	return filepath.Join(root, runtimeFileName), nil
}

func (r *Runtime) sessionMetadataPath(runtimeID string) (string, error) {
	root, err := r.runtimeDirForHandle(agentruntime.Handle{RuntimeID: strings.TrimSpace(runtimeID)})
	if err != nil {
		return "", err
	}
	return filepath.Join(root, sessionFileName), nil
}

func (r *Runtime) mkdirAll(path string, mode os.FileMode) error {
	if r.deps.MkdirAll != nil {
		return r.deps.MkdirAll(path, mode)
	}
	return os.MkdirAll(path, mode)
}

func (r *Runtime) readFile(path string) ([]byte, error) {
	if r.deps.ReadFile != nil {
		return r.deps.ReadFile(path)
	}
	return os.ReadFile(path)
}

func (r *Runtime) writeFile(path string, data []byte, mode os.FileMode) error {
	if r.deps.WriteFile != nil {
		return r.deps.WriteFile(path, data, mode)
	}
	return os.WriteFile(path, data, mode)
}

func (r *Runtime) removeAll(path string) error {
	if r.deps.RemoveAll != nil {
		return r.deps.RemoveAll(path)
	}
	return os.RemoveAll(path)
}

func (r *Runtime) removeAllWithRetry(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("path is required")
	}

	var lastErr error
	for attempt := 0; attempt < removeAllRetryAttempts; attempt++ {
		if err := r.removeAll(path); err == nil || errors.Is(err, os.ErrNotExist) {
			return nil
		} else {
			lastErr = err
			if !isRetryableRemoveAllError(err) || attempt == removeAllRetryAttempts-1 {
				return err
			}
		}
		time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
	}
	return lastErr
}

func isRetryableRemoveAllError(err error) bool {
	if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EACCES) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "directory not empty") || strings.Contains(lower, "permission denied")
}

func (r *Runtime) openFile(path string, flag int, mode os.FileMode) (*os.File, error) {
	if r.deps.OpenFile != nil {
		return r.deps.OpenFile(path, flag, mode)
	}
	return os.OpenFile(path, flag, mode)
}

func (r *Runtime) copyDir(srcRoot, dstRoot string) error {
	srcRoot = strings.TrimSpace(srcRoot)
	dstRoot = strings.TrimSpace(dstRoot)
	if srcRoot == "" || dstRoot == "" {
		return fmt.Errorf("source and destination roots are required")
	}
	if err := filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return r.mkdirAll(dstRoot, 0o755)
		}
		dstPath := filepath.Join(dstRoot, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return r.mkdirAll(dstPath, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := r.mkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		return r.writeFile(dstPath, data, info.Mode().Perm())
	}); err != nil {
		return err
	}
	return nil
}

func hostCodexAuthPath() (string, error) {
	if home := strings.TrimSpace(os.Getenv("CODEX_HOME")); home != "" {
		return filepath.Join(home, "auth.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "auth.json"), nil
}

func hostCodexSkillsPath() (string, error) {
	if home := strings.TrimSpace(os.Getenv("CODEX_HOME")); home != "" {
		return filepath.Join(home, "skills"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "skills"), nil
}

type runtimeDirs struct {
	Root      string
	Workspace string
	Home      string
	CodexHome string
	StderrLog string
}

type runtimeMetadata struct {
	RuntimeID  string             `json:"runtime_id"`
	AgentID    string             `json:"agent_id"`
	AgentName  string             `json:"agent_name"`
	BinaryPath string             `json:"binary_path"`
	SessionID  string             `json:"session_id,omitempty"`
	ProcessID  int                `json:"pid,omitempty"`
	State      agentruntime.State `json:"state,omitempty"`
	CreatedAt  time.Time          `json:"created_at,omitempty"`
	StartedAt  time.Time          `json:"started_at,omitempty"`
	StoppedAt  time.Time          `json:"stopped_at,omitempty"`
	ExitCode   int                `json:"exit_code,omitempty"`
}

type sessionMetadata struct {
	RuntimeID    string    `json:"runtime_id"`
	SessionID    string    `json:"session_id"`
	WorkspaceDir string    `json:"workspace_dir"`
	HomeDir      string    `json:"home_dir"`
	CodexHomeDir string    `json:"codex_home_dir"`
	StartedAt    time.Time `json:"started_at,omitempty"`
}

func sessionToRuntimeMetadata(session *Session) runtimeMetadata {
	return normalizeRuntimeMetadata(runtimeMetadata{
		RuntimeID:  session.RuntimeID,
		AgentID:    session.AgentID,
		AgentName:  session.AgentName,
		BinaryPath: session.BinaryPath,
		SessionID:  session.SessionID,
		ProcessID:  session.ProcessID,
		State:      agentruntime.StateRunning,
		CreatedAt:  session.CreatedAt,
		StartedAt:  session.StartedAt,
	})
}

func sessionToSessionMetadata(session *Session) sessionMetadata {
	return normalizeSessionMetadata(sessionMetadata{
		RuntimeID:    session.RuntimeID,
		SessionID:    session.SessionID,
		WorkspaceDir: session.WorkspaceDir,
		HomeDir:      session.HomeDir,
		CodexHomeDir: session.CodexHomeDir,
		StartedAt:    session.StartedAt,
	})
}

func normalizeRuntimeMetadata(meta runtimeMetadata) runtimeMetadata {
	meta.RuntimeID = strings.TrimSpace(meta.RuntimeID)
	meta.AgentID = strings.TrimSpace(meta.AgentID)
	meta.AgentName = strings.TrimSpace(meta.AgentName)
	meta.BinaryPath = strings.TrimSpace(meta.BinaryPath)
	meta.SessionID = strings.TrimSpace(meta.SessionID)
	meta.State = normalizeRuntimeState(meta.State)
	if !meta.CreatedAt.IsZero() {
		meta.CreatedAt = meta.CreatedAt.UTC()
	}
	if !meta.StartedAt.IsZero() {
		meta.StartedAt = meta.StartedAt.UTC()
	}
	if !meta.StoppedAt.IsZero() {
		meta.StoppedAt = meta.StoppedAt.UTC()
	}
	return meta
}

func normalizeSessionMetadata(meta sessionMetadata) sessionMetadata {
	meta.RuntimeID = strings.TrimSpace(meta.RuntimeID)
	meta.SessionID = strings.TrimSpace(meta.SessionID)
	meta.WorkspaceDir = strings.TrimSpace(meta.WorkspaceDir)
	meta.HomeDir = strings.TrimSpace(meta.HomeDir)
	meta.CodexHomeDir = strings.TrimSpace(meta.CodexHomeDir)
	if !meta.StartedAt.IsZero() {
		meta.StartedAt = meta.StartedAt.UTC()
	}
	return meta
}

func normalizeRuntimeState(state agentruntime.State) agentruntime.State {
	switch state {
	case agentruntime.StateCreated, agentruntime.StateRunning, agentruntime.StateStopped, agentruntime.StateExited, agentruntime.StateFailed:
		return state
	default:
		return agentruntime.StateUnknown
	}
}

func processAlive(pid int) bool {
	return processAlivePID(pid)
}

func stopProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	_ = proc.Signal(os.Interrupt)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func readJSONFile(readFile func(string) ([]byte, error), path string, dst any) error {
	data, err := readFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func writeJSONFile(writeFile func(string, []byte, os.FileMode) error, path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFile(path, data, 0o600)
}

type managerDeps struct {
	EventSink      SessionEventSink
	Permission     PermissionBroker
	OpenFile       func(string, int, os.FileMode) (*os.File, error)
	WriteFile      func(string, []byte, os.FileMode) error
	ReadFile       func(string) ([]byte, error)
	OnExit         func(*Session, int)
	HydrateSession func(context.Context, SessionHandle) (*Session, error)
}
