package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

type liveSession struct {
	mu                   sync.Mutex
	session              *Session
	conn                 *acp.ClientSideConnection
	client               *sessionClient
	cmd                  *exec.Cmd
	stderr               *os.File
	done                 chan struct{}
	conversationSessions map[string]string
}

func (s *liveSession) sessionIDs() []string {
	if s == nil || s.session == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var ids []string
	add := func(sessionID string) {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			return
		}
		if _, ok := seen[sessionID]; ok {
			return
		}
		seen[sessionID] = struct{}{}
		ids = append(ids, sessionID)
	}
	add(s.session.SessionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sessionID := range s.conversationSessions {
		add(sessionID)
	}
	return ids
}

func (m *acpManager) Start(ctx context.Context, spec SessionSpec) (*Session, error) {
	spec.RuntimeID = strings.TrimSpace(spec.RuntimeID)
	if spec.RuntimeID == "" {
		return nil, fmt.Errorf("runtime id is required")
	}

	m.mu.RLock()
	current := m.sessions[spec.RuntimeID]
	m.mu.RUnlock()
	if current != nil && current.session != nil && processAlive(current.session.ProcessID) {
		cloned := *current.session
		return &cloned, nil
	}

	stderrFile, err := m.deps.OpenFile(spec.StderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open stderr log %s: %w", spec.StderrPath, err)
	}

	cmd := exec.CommandContext(ctx, spec.BinaryPath)
	cmd.Dir = spec.WorkspaceDir
	cmd.Env = buildSessionEnv(spec)
	cmd.Stderr = stderrFile
	cmd.SysProcAttr = newSessionSysProcAttr()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = stderrFile.Close()
		return nil, fmt.Errorf("codex stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stderrFile.Close()
		return nil, fmt.Errorf("codex stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stderrFile.Close()
		return nil, fmt.Errorf("start codex-acp: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(stderrFile, &slog.HandlerOptions{}))
	client := &sessionClient{
		runtimeID:    spec.RuntimeID,
		eventSink:    m.deps.EventSink,
		logger:       logger,
		workspaceDir: spec.WorkspaceDir,
		homeDir:      spec.HomeDir,
		codexHomeDir: spec.CodexHomeDir,
		baseEnv:      buildSessionEnv(spec),
		mkdirAll:     os.MkdirAll,
		readFile:     m.deps.ReadFile,
		writeFile:    m.deps.WriteFile,
		terminals:    make(map[string]*managedTerminal),
	}
	conn := acp.NewClientSideConnection(client, stdin, stdout)
	conn.SetLogger(logger)

	initResp, err := conn.Initialize(ctx, acp.InitializeRequest{
		ProtocolVersion: acp.ProtocolVersionNumber,
		ClientCapabilities: acp.ClientCapabilities{
			Fs: acp.FileSystemCapabilities{
				ReadTextFile:  true,
				WriteTextFile: true,
			},
			Terminal: true,
		},
	})
	if err != nil {
		_ = cmd.Process.Kill()
		_ = stderrFile.Close()
		return nil, fmt.Errorf("initialize codex-acp: %w", err)
	}
	newSession, err := conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        spec.WorkspaceDir,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		_ = cmd.Process.Kill()
		_ = stderrFile.Close()
		return nil, fmt.Errorf("create codex session: %w", err)
	}
	now := time.Now().UTC()
	client.setSessionID(string(newSession.SessionId))
	session := &Session{
		RuntimeID:         spec.RuntimeID,
		AgentID:           spec.AgentID,
		AgentName:         spec.AgentName,
		SessionID:         string(newSession.SessionId),
		BinaryPath:        spec.BinaryPath,
		RuntimeDir:        spec.RuntimeDir,
		WorkspaceDir:      spec.WorkspaceDir,
		HomeDir:           spec.HomeDir,
		CodexHomeDir:      spec.CodexHomeDir,
		StderrPath:        spec.StderrPath,
		ProcessID:         cmd.Process.Pid,
		CreatedAt:         now,
		StartedAt:         now,
		AgentCapabilities: initResp.AgentCapabilities,
	}

	live := &liveSession{
		session:              session,
		conn:                 conn,
		client:               client,
		cmd:                  cmd,
		stderr:               stderrFile,
		done:                 make(chan struct{}),
		conversationSessions: make(map[string]string),
	}
	m.mu.Lock()
	m.sessions[spec.RuntimeID] = live
	m.mu.Unlock()

	go m.waitSession(spec.RuntimeID, live)

	cloned := *session
	return &cloned, nil
}

func (m *acpManager) Stop(ctx context.Context, handle SessionHandle) error {
	runtimeID := strings.TrimSpace(handle.RuntimeID)
	m.mu.RLock()
	live := m.sessions[runtimeID]
	m.mu.RUnlock()
	if live == nil {
		return os.ErrNotExist
	}

	if live.conn != nil && live.session != nil {
		for _, sessionID := range live.sessionIDs() {
			_, _ = live.conn.CloseSession(ctx, acp.CloseSessionRequest{
				SessionId: acp.SessionId(sessionID),
			})
		}
	}
	if live.client != nil {
		live.client.shutdownTerminals()
	}
	if live.cmd != nil && live.cmd.Process != nil {
		_ = stopProcess(live.cmd.Process.Pid)
	}

	select {
	case <-live.done:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(3 * time.Second):
	}
	return nil
}

func (m *acpManager) Session(handle SessionHandle) (*Session, error) {
	runtimeID := strings.TrimSpace(handle.RuntimeID)
	m.mu.RLock()
	live := m.sessions[runtimeID]
	m.mu.RUnlock()
	if live == nil || live.session == nil {
		return nil, os.ErrNotExist
	}
	cloned := *live.session
	return &cloned, nil
}

func (m *acpManager) Prompt(ctx context.Context, handle SessionHandle, req acp.PromptRequest) (acp.PromptResponse, error) {
	runtimeID := strings.TrimSpace(handle.RuntimeID)
	m.mu.RLock()
	live := m.sessions[runtimeID]
	m.mu.RUnlock()
	if live == nil || live.conn == nil || live.session == nil {
		return acp.PromptResponse{}, os.ErrNotExist
	}
	if req.SessionId == "" {
		req.SessionId = acp.SessionId(live.session.SessionID)
	}
	sessionID := strings.TrimSpace(string(req.SessionId))
	resp, err := live.conn.Prompt(ctx, req)
	if err != nil {
		if m.deps.EventSink != nil {
			m.deps.EventSink.Publish(promptFailedEvent(runtimeID, sessionID, err))
		}
		return acp.PromptResponse{}, err
	}
	if m.deps.EventSink != nil {
		m.deps.EventSink.Publish(promptCompletedEvent(runtimeID, sessionID, resp))
	}
	return resp, nil
}

func (m *acpManager) EnsureSession(ctx context.Context, handle SessionHandle, conversationKey string) (string, error) {
	runtimeID := strings.TrimSpace(handle.RuntimeID)
	m.mu.RLock()
	live := m.sessions[runtimeID]
	m.mu.RUnlock()
	if live == nil || live.conn == nil || live.session == nil {
		return "", os.ErrNotExist
	}

	conversationKey = strings.TrimSpace(conversationKey)
	if conversationKey == "" {
		return strings.TrimSpace(live.session.SessionID), nil
	}

	live.mu.Lock()
	defer live.mu.Unlock()
	if sessionID := strings.TrimSpace(live.conversationSessions[conversationKey]); sessionID != "" {
		return sessionID, nil
	}

	newSession, err := live.conn.NewSession(ctx, acp.NewSessionRequest{
		Cwd:        live.session.WorkspaceDir,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		return "", err
	}
	sessionID := strings.TrimSpace(string(newSession.SessionId))
	if sessionID == "" {
		return "", fmt.Errorf("create codex conversation session: empty session id")
	}
	live.conversationSessions[conversationKey] = sessionID
	return sessionID, nil
}

func (m *acpManager) waitSession(runtimeID string, live *liveSession) {
	err := live.cmd.Wait()
	exitCode := 0
	if err != nil {
		exitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	if live.session != nil {
		live.session.ProcessID = 0
		if m.deps.OnExit != nil {
			m.deps.OnExit(live.session, exitCode)
		}
	}
	if live.client != nil {
		live.client.shutdownTerminals()
	}
	if live.stderr != nil {
		_ = live.stderr.Close()
	}
	m.mu.Lock()
	delete(m.sessions, runtimeID)
	m.mu.Unlock()
	close(live.done)
}

func buildSessionEnv(spec SessionSpec) []string {
	spec.Profile = spec.Profile.Normalized()
	envMap := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		envMap[key] = value
	}
	envMap["HOME"] = spec.HomeDir
	envMap["CODEX_HOME"] = spec.CodexHomeDir
	// if baseURL := spec.Profile.BaseURL; baseURL != "" {
	// 	envMap["OPENAI_BASE_URL"] = baseURL
	// }
	if apiKey := spec.Profile.APIKey; apiKey != "" {
		envMap["OPENAI_API_KEY"] = apiKey
	}
	// if modelID := spec.Profile.ModelID; modelID != "" {
	// 	envMap["OPENAI_MODEL"] = modelID
	// }
	for key, value := range spec.Profile.Env {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if isReservedSessionEnvKey(key) {
			continue
		}
		envMap[key] = value
	}
	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		keys = append(keys, key)
	}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+envMap[key])
	}
	return out
}

func isReservedSessionEnvKey(key string) bool {
	switch strings.ToUpper(strings.TrimSpace(key)) {
	case "HOME", "CODEX_HOME", "OPENAI_BASE_URL", "OPENAI_API_KEY", "OPENAI_MODEL":
		return true
	default:
		return false
	}
}

func formatSessionConfigOptionsDebug(options []acp.SessionConfigOption) string {
	if len(options) == 0 {
		return "codex-acp new session config options: []\n"
	}

	lines := []string{"codex-acp new session config options:"}
	for i, option := range options {
		if option.Select != nil {
			lines = append(lines, fmt.Sprintf("  [%d] select id=%q name=%q current=%q", i, option.Select.Id, option.Select.Name, option.Select.CurrentValue))
			for _, value := range flattenSelectOptions(option.Select.Options) {
				lines = append(lines, fmt.Sprintf("       - %q => %q", value.Name, value.Value))
			}
			continue
		}
		if option.Boolean != nil {
			lines = append(lines, fmt.Sprintf("  [%d] boolean id=%q name=%q current=%t", i, option.Boolean.Id, option.Boolean.Name, option.Boolean.CurrentValue))
			continue
		}
		lines = append(lines, fmt.Sprintf("  [%d] <unknown option variant>", i))
	}
	return strings.Join(lines, "\n") + "\n"
}

func flattenSelectOptions(options acp.SessionConfigSelectOptions) []acp.SessionConfigSelectOption {
	if options.Ungrouped != nil {
		out := make([]acp.SessionConfigSelectOption, len(*options.Ungrouped))
		copy(out, *options.Ungrouped)
		return out
	}
	if options.Grouped != nil {
		var out []acp.SessionConfigSelectOption
		for _, group := range *options.Grouped {
			out = append(out, group.Options...)
		}
		return out
	}
	return nil
}

func formatSessionModelsDebug(models *acp.SessionModelState) string {
	if models == nil {
		return "codex-acp new session models: <nil>\n"
	}

	type modelLine struct {
		ModelID     acp.ModelId `json:"model_id"`
		Name        string      `json:"name"`
		Description *string     `json:"description,omitempty"`
	}
	payload := struct {
		CurrentModelID  acp.ModelId `json:"current_model_id"`
		AvailableModels []modelLine `json:"available_models"`
	}{
		CurrentModelID:  models.CurrentModelId,
		AvailableModels: make([]modelLine, 0, len(models.AvailableModels)),
	}
	for _, model := range models.AvailableModels {
		payload.AvailableModels = append(payload.AvailableModels, modelLine{
			ModelID:     model.ModelId,
			Name:        model.Name,
			Description: model.Description,
		})
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Sprintf("codex-acp new session models: marshal error: %v\n", err)
	}
	return "codex-acp new session models:\n" + string(data) + "\n"
}

type sessionClient struct {
	runtimeID    string
	sessionID    string
	eventSink    SessionEventSink
	logger       *slog.Logger
	workspaceDir string
	homeDir      string
	codexHomeDir string
	baseEnv      []string
	mkdirAll     func(string, os.FileMode) error
	readFile     func(string) ([]byte, error)
	writeFile    func(string, []byte, os.FileMode) error

	mu         sync.Mutex
	nextTermID int
	terminals  map[string]*managedTerminal
}

func (c *sessionClient) RequestPermission(_ context.Context, params acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	c.publish(permissionRequestEvent(c.runtimeID, params))

	option := choosePermissionOption(params.Options)
	if option != nil {
		c.publish(permissionDecisionEvent(c.runtimeID, params, option))
		return acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{
				Selected: &acp.RequestPermissionOutcomeSelected{OptionId: option.OptionId},
			},
		}, nil
	}
	c.publish(permissionDecisionEvent(c.runtimeID, params, nil))
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{
			Cancelled: &acp.RequestPermissionOutcomeCancelled{},
		},
	}, nil
}

func (c *sessionClient) SessionUpdate(_ context.Context, params acp.SessionNotification) error {
	c.publish(eventFromSessionUpdate(c.runtimeID, params))
	return nil
}

func (c *sessionClient) WriteTextFile(_ context.Context, params acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	if !filepath.IsAbs(params.Path) {
		return acp.WriteTextFileResponse{}, fmt.Errorf("path must be absolute: %s", params.Path)
	}
	if !c.pathAllowed(params.Path) {
		return acp.WriteTextFileResponse{}, fmt.Errorf("path is outside allowed roots: %s", params.Path)
	}
	if err := c.mkdirAll(filepath.Dir(params.Path), 0o755); err != nil {
		return acp.WriteTextFileResponse{}, err
	}
	if err := c.writeFile(params.Path, []byte(params.Content), 0o644); err != nil {
		return acp.WriteTextFileResponse{}, err
	}
	return acp.WriteTextFileResponse{}, nil
}

func (c *sessionClient) ReadTextFile(_ context.Context, params acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	if !filepath.IsAbs(params.Path) {
		return acp.ReadTextFileResponse{}, fmt.Errorf("path must be absolute: %s", params.Path)
	}
	if !c.pathAllowed(params.Path) {
		return acp.ReadTextFileResponse{}, fmt.Errorf("path is outside allowed roots: %s", params.Path)
	}
	data, err := c.readFile(params.Path)
	if err != nil {
		return acp.ReadTextFileResponse{}, err
	}
	content := string(data)
	if params.Line == nil && params.Limit == nil {
		return acp.ReadTextFileResponse{Content: content}, nil
	}
	lines := strings.Split(content, "\n")
	start := 0
	if params.Line != nil && *params.Line > 0 {
		start = min(max(*params.Line-1, 0), len(lines))
	}
	end := len(lines)
	if params.Limit != nil && *params.Limit > 0 && start+*params.Limit < end {
		end = start + *params.Limit
	}
	return acp.ReadTextFileResponse{Content: strings.Join(lines[start:end], "\n")}, nil
}

func (c *sessionClient) CreateTerminal(_ context.Context, params acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	command := strings.TrimSpace(params.Command)
	if command == "" {
		return acp.CreateTerminalResponse{}, fmt.Errorf("terminal command is required")
	}
	cwd, err := resolveTerminalCWD(c.workspaceDir, []string{c.workspaceDir, c.homeDir, c.codexHomeDir}, params.Cwd)
	if err != nil {
		return acp.CreateTerminalResponse{}, err
	}
	term := newManagedTerminal(intValue(params.OutputByteLimit, defaultTerminalOutputLimit))
	cmd := exec.Command(command, params.Args...)
	cmd.Dir = cwd
	cmd.Env = buildTerminalEnv(c.baseEnv, params.Env)
	cmd.Stdout = term
	cmd.Stderr = term
	if err := cmd.Start(); err != nil {
		return acp.CreateTerminalResponse{}, err
	}
	term.cmd = cmd

	c.mu.Lock()
	c.nextTermID++
	terminalID := fmt.Sprintf("term-%d", c.nextTermID)
	c.terminals[terminalID] = term
	c.mu.Unlock()

	go func() {
		term.setExit(cmd.Wait())
	}()

	return acp.CreateTerminalResponse{TerminalId: terminalID}, nil
}

func (c *sessionClient) TerminalOutput(_ context.Context, params acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	term, err := c.terminal(params.TerminalId)
	if err != nil {
		return acp.TerminalOutputResponse{}, err
	}
	output, truncated, exitStatus := term.snapshot()
	return acp.TerminalOutputResponse{
		Output:     output,
		Truncated:  truncated,
		ExitStatus: exitStatus,
	}, nil
}

func (c *sessionClient) ReleaseTerminal(_ context.Context, params acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	term, err := c.terminal(params.TerminalId)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return acp.ReleaseTerminalResponse{}, nil
		}
		return acp.ReleaseTerminalResponse{}, err
	}
	select {
	case <-term.done:
		c.mu.Lock()
		delete(c.terminals, params.TerminalId)
		c.mu.Unlock()
	default:
	}
	return acp.ReleaseTerminalResponse{}, nil
}

func (c *sessionClient) WaitForTerminalExit(ctx context.Context, params acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	term, err := c.terminal(params.TerminalId)
	if err != nil {
		return acp.WaitForTerminalExitResponse{}, err
	}
	return waitTerminal(ctx, term)
}

func (c *sessionClient) KillTerminal(_ context.Context, params acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	term, err := c.terminal(params.TerminalId)
	if err != nil {
		return acp.KillTerminalResponse{}, err
	}
	if err := killManagedProcess(term.cmd); err != nil {
		return acp.KillTerminalResponse{}, err
	}
	return acp.KillTerminalResponse{}, nil
}

func (c *sessionClient) setSessionID(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionID = strings.TrimSpace(sessionID)
}

func (c *sessionClient) terminal(id string) (*managedTerminal, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	term := c.terminals[strings.TrimSpace(id)]
	if term == nil {
		return nil, os.ErrNotExist
	}
	return term, nil
}

func (c *sessionClient) shutdownTerminals() {
	c.mu.Lock()
	terminals := make([]*managedTerminal, 0, len(c.terminals))
	for _, term := range c.terminals {
		terminals = append(terminals, term)
	}
	c.terminals = make(map[string]*managedTerminal)
	c.mu.Unlock()
	for _, term := range terminals {
		_ = killManagedProcess(term.cmd)
	}
}

func (c *sessionClient) publish(event SessionEvent) {
	if c.eventSink != nil {
		c.eventSink.Publish(event)
	}
	if c.logger != nil {
		attrs := []any{"runtime_id", event.RuntimeID, "session_id", event.SessionID, "kind", event.Kind}
		if event.ToolCallID != "" {
			attrs = append(attrs, "tool_call_id", event.ToolCallID)
		}
		if event.ToolStatus != "" {
			attrs = append(attrs, "tool_status", event.ToolStatus)
		}
		if event.StopReason != "" {
			attrs = append(attrs, "stop_reason", event.StopReason)
		}
		if event.Error != "" {
			attrs = append(attrs, "error", event.Error)
			c.logger.Warn("codex session event", attrs...)
			return
		}
		c.logger.Debug("codex session event", attrs...)
	}
}

func (c *sessionClient) pathAllowed(path string) bool {
	return pathAllowed(path, c.workspaceDir, c.homeDir, c.codexHomeDir)
}

func intValue(v *int, fallback int) int {
	if v == nil || *v <= 0 {
		return fallback
	}
	return *v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var _ io.Writer
