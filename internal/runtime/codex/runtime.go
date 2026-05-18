package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"csgclaw/internal/codexacp"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"

	acp "github.com/coder/acp-go-sdk"
)

const (
	hostStateDirName       = ".codex"
	configFileName         = "config.toml"
	runtimeFileName        = "runtime.json"
	sessionFileName        = "session.json"
	stderrLogFileName      = "stderr.log"
	workspaceDirName       = "workspace"
	homeDirName            = "home"
	logPollInterval        = 200 * time.Millisecond
	codexProxyProviderName = "proxy"
)

type AgentRef struct {
	ID        string
	Name      string
	RuntimeID string
	HandleID  string
	Profile   agentruntime.Profile
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
	Prompt(ctx context.Context, handle SessionHandle, req acp.PromptRequest) (acp.PromptResponse, error)
}

type SessionEventKind string

const (
	SessionEventUserMessageDelta   SessionEventKind = "user_message_delta"
	SessionEventTextDelta          SessionEventKind = "text_delta"
	SessionEventThoughtDelta       SessionEventKind = "thought_delta"
	SessionEventToolCallStart      SessionEventKind = "tool_call_start"
	SessionEventToolCallUpdate     SessionEventKind = "tool_call_update"
	SessionEventPlanUpdate         SessionEventKind = "plan_update"
	SessionEventPermissionRequest  SessionEventKind = "permission_request"
	SessionEventPermissionDecision SessionEventKind = "permission_decision"
	SessionEventPromptCompleted    SessionEventKind = "prompt_completed"
	SessionEventPromptFailed       SessionEventKind = "prompt_failed"
)

type SessionEvent struct {
	RuntimeID            string
	SessionID            string
	Kind                 SessionEventKind
	ReceivedAt           time.Time
	MessageID            string
	Text                 string
	ToolCallID           string
	ToolTitle            string
	ToolStatus           string
	PermissionOptionID   string
	PermissionOptionKind string
	StopReason           string
	Error                string
	Payload              any
}

type SessionEventSink interface {
	Publish(SessionEvent)
}

type Dependencies struct {
	BinaryProvider codexacp.BinaryProvider
	ResolveAgent   func(h agentruntime.Handle) (AgentRef, error)
	AgentHome      func(agentName string) (string, error)
	Manager        Manager
	EventSink      SessionEventSink

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
	_ agentruntime.Runtime     = (*Runtime)(nil)
	_ agentruntime.LogStreamer = (*Runtime)(nil)
)

func New(deps Dependencies) *Runtime {
	return &Runtime{deps: deps}
}

func (r *Runtime) Kind() string {
	return agentruntime.KindCodex
}

func (r *Runtime) SessionManager() Manager {
	return r.sessionManager()
}

func (r *Runtime) EventSink() SessionEventSink {
	return r.deps.EventSink
}

func (r *Runtime) New(ctx context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
	if err := r.ensureRuntimeHome(spec.AgentName); err != nil {
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
	if err := r.removeAll(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
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

func (r *Runtime) sessionManager() Manager {
	if r.deps.Manager != nil {
		return r.deps.Manager
	}
	r.deps.Manager = newACPManager(acpManagerDeps{
		EventSink: r.deps.EventSink,
		OpenFile:  r.openFile,
		WriteFile: r.writeFile,
		ReadFile:  r.readFile,
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
	return r.deps.Manager
}

func (r *Runtime) ensureSession(ctx context.Context, spec SessionSpec) (*Session, error) {
	runtimeID := strings.TrimSpace(spec.RuntimeID)
	if runtimeID == "" {
		return nil, fmt.Errorf("runtime id is required")
	}
	if strings.TrimSpace(spec.AgentName) == "" || strings.TrimSpace(spec.AgentID) == "" {
		return nil, fmt.Errorf("agent name and id are required")
	}
	dirs, err := r.ensureRuntimeDirs(spec.AgentName)
	if err != nil {
		return nil, err
	}
	spec.RuntimeDir = dirs.Root
	spec.WorkspaceDir = dirs.Workspace
	spec.HomeDir = dirs.Home
	spec.CodexHomeDir = dirs.CodexHome
	spec.StderrPath = dirs.StderrLog
	if err := r.seedCodexHomeAuth(spec.CodexHomeDir); err != nil {
		return nil, err
	}
	if err := r.seedCodexHomeConfig(spec.CodexHomeDir, spec.Profile); err != nil {
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
	if hasAuth, err := r.codexHomeHasAuth(runtimeCodexHome); err != nil {
		return err
	} else if hasAuth {
		if err := r.removeAll(configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove runtime codex config %s: %w", configPath, err)
		}
		return nil
	}
	profile = profile.Normalized()
	slog.Info("codex runtime profile before writing config",
		"codex_home", runtimeCodexHome,
		"base_url", profile.BaseURL,
		"model_id", profile.ModelID,
	)
	if profile.BaseURL == "" || profile.ModelID == "" {
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "model = %s\n", strconv.Quote(profile.ModelID))
	fmt.Fprintf(&b, "model_provider = %s\n\n", strconv.Quote(codexProxyProviderName))
	fmt.Fprintf(&b, "[model_providers.%s]\n", codexProxyProviderName)
	fmt.Fprintf(&b, "name = %s\n", strconv.Quote("OpenAI using LLM proxy"))
	// fmt.Fprintf(&b, "wire_api = %s\n", strconv.Quote("chat"))
	fmt.Fprintf(&b, "base_url = %s\n", strconv.Quote(profile.BaseURL))
	if profile.APIKey != "" {
		fmt.Fprintf(&b, "env_key = %s\n", strconv.Quote("OPENAI_API_KEY"))
	}

	if err := r.writeFile(configPath, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write runtime codex config %s: %w", configPath, err)
	}
	return nil
}

func (r *Runtime) codexHomeHasAuth(runtimeCodexHome string) (bool, error) {
	runtimeAuthPath := filepath.Join(strings.TrimSpace(runtimeCodexHome), "auth.json")
	if _, err := r.readFile(runtimeAuthPath); err == nil {
		return true, nil
	} else if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else {
		return false, fmt.Errorf("read runtime codex auth %s: %w", runtimeAuthPath, err)
	}
}

func (r *Runtime) ensureBinary(ctx context.Context) (string, error) {
	if r.deps.BinaryProvider == nil {
		return "", fmt.Errorf("codex binary provider is required")
	}
	return r.deps.BinaryProvider.Ensure(ctx)
}

func (r *Runtime) ensureRuntimeHome(agentName string) error {
	_, err := r.ensureRuntimeDirs(agentName)
	return err
}

func (r *Runtime) ensureRuntimeDirs(agentName string) (runtimeDirs, error) {
	root, err := r.runtimeDirForAgent(agentName)
	if err != nil {
		return runtimeDirs{}, err
	}
	dirs := runtimeDirs{
		Root:      root,
		Workspace: filepath.Join(root, workspaceDirName),
		Home:      filepath.Join(root, homeDirName),
		CodexHome: root,
		StderrLog: filepath.Join(root, stderrLogFileName),
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
	return r.runtimeDirForAgent(agentRef.Name)
}

func (r *Runtime) runtimeDirForAgent(agentName string) (string, error) {
	if r.deps.AgentHome == nil {
		return "", fmt.Errorf("agent home resolver is required")
	}
	agentHome, err := r.deps.AgentHome(strings.TrimSpace(agentName))
	if err != nil {
		return "", err
	}
	return filepath.Join(agentHome, filepath.FromSlash(hostStateDirName)), nil
}

func (r *Runtime) stderrLogPath(h agentruntime.Handle) (string, error) {
	root, err := r.runtimeDirForHandle(h)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, stderrLogFileName), nil
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
	return agentRef, nil
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

func (r *Runtime) openFile(path string, flag int, mode os.FileMode) (*os.File, error) {
	if r.deps.OpenFile != nil {
		return r.deps.OpenFile(path, flag, mode)
	}
	return os.OpenFile(path, flag, mode)
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

type acpManagerDeps struct {
	EventSink SessionEventSink
	OpenFile  func(string, int, os.FileMode) (*os.File, error)
	WriteFile func(string, []byte, os.FileMode) error
	ReadFile  func(string) ([]byte, error)
	OnExit    func(*Session, int)
}

type acpManager struct {
	deps     acpManagerDeps
	mu       sync.RWMutex
	sessions map[string]*liveSession
}

func newACPManager(deps acpManagerDeps) *acpManager {
	return &acpManager{
		deps:     deps,
		sessions: make(map[string]*liveSession),
	}
}
