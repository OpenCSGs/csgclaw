package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/codexcli"
	agentruntime "csgclaw/internal/runtime"
)

const (
	appServerStopTimeout          = 3 * time.Second
	appServerTurnInterruptTimeout = 5 * time.Second
)

var (
	appServerSemanticInactivityTimeout  = 10 * time.Minute
	appServerFirstTurnNoProgressTimeout = 5 * time.Minute
	appServerMaximumTurnDuration        = 30 * time.Minute
)

var appServerCommandContext = codexcli.AppServerCommandContext

type appServerManager struct {
	deps      managerDeps
	hydrateMu sync.Mutex
	mu        sync.RWMutex
	sessions  map[string]*liveSession
}

func newAppServerManager(deps managerDeps) *appServerManager {
	return &appServerManager{
		deps:     deps,
		sessions: make(map[string]*liveSession),
	}
}

func (m *appServerManager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	runtimeIDs := make([]string, 0, len(m.sessions))
	for runtimeID := range m.sessions {
		runtimeIDs = append(runtimeIDs, runtimeID)
	}
	m.mu.RUnlock()

	var closeErr error
	for _, runtimeID := range runtimeIDs {
		if err := m.Stop(context.Background(), SessionHandle{RuntimeID: runtimeID}); err != nil && !errors.Is(err, os.ErrNotExist) {
			closeErr = errors.Join(closeErr, fmt.Errorf("stop codex app-server runtime %q: %w", runtimeID, err))
		}
	}
	return closeErr
}

func (m *appServerManager) hasSession(runtimeID string) bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[strings.TrimSpace(runtimeID)] != nil
}

func (m *appServerManager) Start(ctx context.Context, spec SessionSpec) (*Session, error) {
	spec.RuntimeID = strings.TrimSpace(spec.RuntimeID)
	if spec.RuntimeID == "" {
		return nil, fmt.Errorf("runtime id is required")
	}
	spec.Profile = spec.Profile.Normalized()

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

	cmd, err := appServerCommandContext(ctx, spec.BinaryPath)
	if err != nil {
		_ = stderrFile.Close()
		return nil, fmt.Errorf("prepare codex app-server command: %w", err)
	}
	cmd.Dir = spec.WorkspaceDir
	cmd.Env = buildSessionEnv(spec)
	cmd.Stderr = stderrFile
	if sysProcAttr := newSessionSysProcAttr(); sysProcAttr != nil {
		cmd.SysProcAttr = sysProcAttr
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = stderrFile.Close()
		return nil, fmt.Errorf("codex app-server stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stderrFile.Close()
		return nil, fmt.Errorf("codex app-server stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stderrFile.Close()
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(stderrFile, &slog.HandlerOptions{}))
	appClient := newAppServerClient(stdin, logger)
	live := &liveSession{
		cmd:                   cmd,
		stdin:                 stdin,
		stderr:                stderrFile,
		done:                  make(chan struct{}),
		spec:                  spec,
		appClient:             appClient,
		conversationSessions:  make(map[string]string),
		turnWaiters:           make(map[string]*appServerTurnWaiter),
		replayedExecCommands:  make(map[string]struct{}),
		replayedAgentMessages: make(map[string]struct{}),
	}
	appClient.onNotification = func(note appServerNotification) {
		m.handleAppServerNotification(spec.RuntimeID, live, note)
	}
	appClient.onServerRequest = func(req appServerServerRequest) (any, error) {
		return m.handleAppServerServerRequest(spec.RuntimeID, live, req)
	}

	m.mu.Lock()
	m.sessions[spec.RuntimeID] = live
	m.mu.Unlock()

	go m.readAppServerStdout(spec.RuntimeID, live, stdout)
	go m.waitAppServerSession(spec.RuntimeID, live)

	if err := m.initializeHandshake(ctx, live); err != nil {
		_ = m.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID})
		return nil, m.wrapStartupError(spec, "initialize codex app-server", err)
	}

	threadID, err := m.startOrResumeThread(ctx, live, m.persistedThreadID(spec))
	if err != nil {
		_ = m.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID})
		return nil, m.wrapStartupError(spec, "initialize codex app-server thread", err)
	}
	if strings.TrimSpace(threadID) == "" {
		_ = m.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID})
		return nil, m.wrapStartupError(spec, "initialize codex app-server thread", fmt.Errorf("empty thread id"))
	}

	now := time.Now().UTC()
	session := &Session{
		RuntimeID:    spec.RuntimeID,
		AgentID:      spec.AgentID,
		AgentName:    spec.AgentName,
		SessionID:    threadID,
		BinaryPath:   spec.BinaryPath,
		RuntimeDir:   spec.RuntimeDir,
		WorkspaceDir: spec.WorkspaceDir,
		HomeDir:      spec.HomeDir,
		CodexHomeDir: spec.CodexHomeDir,
		StderrPath:   spec.StderrPath,
		ProcessID:    cmd.Process.Pid,
		CreatedAt:    now,
		StartedAt:    now,
	}
	live.mu.Lock()
	live.session = session
	live.mu.Unlock()

	cloned := *session
	return &cloned, nil
}

func (m *appServerManager) Stop(ctx context.Context, handle SessionHandle) error {
	runtimeID := strings.TrimSpace(handle.RuntimeID)
	m.mu.RLock()
	live := m.sessions[runtimeID]
	m.mu.RUnlock()
	if live == nil {
		return os.ErrNotExist
	}
	if m.deps.Permission != nil {
		m.deps.Permission.CancelSession(runtimeID, "")
	}
	if m.deps.UserInput != nil {
		m.deps.UserInput.CancelSession(runtimeID, "")
	}
	if live.appClient != nil {
		live.appClient.closeAllPending(fmt.Errorf("codex app-server stopping"))
	}
	if live.stdin != nil {
		_ = live.stdin.Close()
	}
	if live.cmd != nil && live.cmd.Process != nil {
		if err := stopProcess(live.cmd.Process.Pid); err != nil {
			return err
		}
	}

	timeout := time.NewTimer(appServerStopTimeout)
	defer timeout.Stop()
	select {
	case <-live.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timeout.C:
		return fmt.Errorf("codex app-server stop timeout after %s: runtime_id=%s", appServerStopTimeout, runtimeID)
	}
}

func (m *appServerManager) Session(handle SessionHandle) (*Session, error) {
	live, err := m.ensureLiveSession(context.Background(), handle)
	if err != nil {
		return nil, err
	}
	cloned := *live.session
	return &cloned, nil
}

func (m *appServerManager) LiveSession(handle SessionHandle) (*Session, error) {
	runtimeID := strings.TrimSpace(handle.RuntimeID)
	if runtimeID == "" {
		return nil, fmt.Errorf("runtime id is required")
	}
	live := m.liveSession(runtimeID)
	if live == nil {
		return nil, os.ErrNotExist
	}
	cloned := *live.session
	return &cloned, nil
}

func (m *appServerManager) Prompt(ctx context.Context, handle SessionHandle, req PromptRequest) (PromptResponse, error) {
	runtimeID := strings.TrimSpace(handle.RuntimeID)
	live, err := m.ensureLiveSession(ctx, handle)
	if err != nil {
		return PromptResponse{}, err
	}

	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(live.session.SessionID)
		req.SessionID = sessionID
	}
	promptText, err := appServerPromptText(req)
	if err != nil {
		if m.deps.EventSink != nil {
			m.deps.EventSink.Publish(promptFailedEvent(runtimeID, sessionID, err))
		}
		return PromptResponse{}, err
	}

	waiter, err := live.registerAppServerTurnWaiter(sessionID)
	if err != nil {
		if m.deps.EventSink != nil {
			m.deps.EventSink.Publish(promptFailedEvent(runtimeID, sessionID, err))
		}
		return PromptResponse{}, err
	}
	defer live.removeAppServerTurnWaiter(sessionID, waiter)

	params := appServerTurnStartParams(live.spec, sessionID, promptText)
	turnStartAt := time.Now()
	live.appClient.logDebug("codex app-server turn start request",
		"runtime_id", runtimeID,
		"thread_id", sessionID,
		"prompt_bytes", len(promptText),
	)
	raw, err := live.appClient.request(ctx, "turn/start", params)
	if err != nil {
		live.appClient.logDebug("codex app-server turn start failed",
			"runtime_id", runtimeID,
			"thread_id", sessionID,
			"duration", time.Since(turnStartAt),
			"error", err,
		)
		err = fmt.Errorf("codex turn/start failed: %w", err)
		if m.deps.EventSink != nil {
			m.deps.EventSink.Publish(promptFailedEvent(runtimeID, sessionID, err))
		}
		return PromptResponse{}, err
	}
	waiter.setTurnID(appServerTurnIDFromResult(raw))
	live.appClient.logDebug("codex app-server turn start accepted",
		"runtime_id", runtimeID,
		"thread_id", sessionID,
		"turn_id", waiter.currentTurnID(),
		"duration", time.Since(turnStartAt),
	)

	waitStartAt := time.Now()
	resp, err := m.waitAppServerTurn(ctx, live, waiter)
	if err != nil {
		live.appClient.logDebug("codex app-server turn wait failed",
			"runtime_id", runtimeID,
			"thread_id", sessionID,
			"turn_id", waiter.currentTurnID(),
			"duration", time.Since(waitStartAt),
			"last_activity", waiter.currentLastActivity(),
			"error", err,
		)
		if m.deps.EventSink != nil {
			m.deps.EventSink.Publish(promptFailedEvent(runtimeID, sessionID, err))
		}
		return PromptResponse{}, err
	}
	live.appClient.logDebug("codex app-server turn wait completed",
		"runtime_id", runtimeID,
		"thread_id", sessionID,
		"turn_id", waiter.currentTurnID(),
		"duration", time.Since(waitStartAt),
		"stop_reason", strings.TrimSpace(resp.StopReason),
	)
	if m.deps.EventSink != nil {
		m.deps.EventSink.Publish(promptCompletedEvent(runtimeID, sessionID, resp))
	}
	return resp, nil
}

func (m *appServerManager) EnsureSession(ctx context.Context, handle SessionHandle, conversationKey string) (string, error) {
	live, err := m.ensureLiveSession(ctx, handle)
	if err != nil {
		return "", err
	}

	conversationKey = strings.TrimSpace(conversationKey)
	if conversationKey == "" {
		return strings.TrimSpace(live.session.SessionID), nil
	}

	live.mu.Lock()
	if threadID := strings.TrimSpace(live.conversationSessions[conversationKey]); threadID != "" {
		live.mu.Unlock()
		return threadID, nil
	}
	live.mu.Unlock()

	threadID, err := m.startThread(ctx, live)
	if err != nil {
		return "", err
	}

	live.mu.Lock()
	defer live.mu.Unlock()
	if existing := strings.TrimSpace(live.conversationSessions[conversationKey]); existing != "" {
		return existing, nil
	}
	live.conversationSessions[conversationKey] = threadID
	return threadID, nil
}

func (m *appServerManager) ResetConversationHistory(ctx context.Context, handle SessionHandle, conversationKey string) error {
	runtimeID := strings.TrimSpace(handle.RuntimeID)
	conversationKey = strings.TrimSpace(conversationKey)
	if runtimeID == "" {
		return fmt.Errorf("runtime id is required")
	}
	if conversationKey == "" {
		return fmt.Errorf("conversation key is required")
	}

	live, err := m.ensureLiveSession(ctx, handle)
	if err != nil {
		return err
	}

	live.mu.Lock()
	sessionID := strings.TrimSpace(live.conversationSessions[conversationKey])
	delete(live.conversationSessions, conversationKey)
	live.mu.Unlock()

	if m.deps.Permission != nil && sessionID != "" {
		m.deps.Permission.CancelSession(runtimeID, sessionID)
	}
	if m.deps.UserInput != nil && sessionID != "" {
		m.deps.UserInput.CancelSession(runtimeID, sessionID)
	}
	return nil
}

func (m *appServerManager) ensureLiveSession(ctx context.Context, handle SessionHandle) (*liveSession, error) {
	runtimeID := strings.TrimSpace(handle.RuntimeID)
	if runtimeID == "" {
		return nil, fmt.Errorf("runtime id is required")
	}
	if live := m.liveSession(runtimeID); live != nil {
		return live, nil
	}
	if m.deps.HydrateSession == nil {
		return nil, os.ErrNotExist
	}
	if ctx == nil {
		ctx = context.Background()
	}

	m.hydrateMu.Lock()
	defer m.hydrateMu.Unlock()

	if live := m.liveSession(runtimeID); live != nil {
		return live, nil
	}
	if _, err := m.deps.HydrateSession(ctx, handle); err != nil {
		return nil, err
	}
	if live := m.liveSession(runtimeID); live != nil {
		return live, nil
	}
	return nil, os.ErrNotExist
}

func (m *appServerManager) liveSession(runtimeID string) *liveSession {
	runtimeID = strings.TrimSpace(runtimeID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	live := m.sessions[runtimeID]
	if live == nil || live.session == nil || live.appClient == nil {
		return nil
	}
	return live
}

// initializeHandshake performs the JSON-RPC initialize handshake required by
// the codex app-server before any thread or turn requests can be sent.
// The protocol is: (1) client sends "initialize" request with clientInfo
// and capabilities, (2) server responds with its capabilities, (3) client
// sends "initialized" notification. Without this handshake, the server
// rejects subsequent requests with "Not initialized (code=-32600)".
func (m *appServerManager) initializeHandshake(ctx context.Context, live *liveSession) error {
	_, err := live.appClient.request(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "csgclaw-agent-sdk",
			"title":   "CSGClaw Agent SDK",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	})
	if err != nil {
		return fmt.Errorf("codex app-server initialize handshake: %w", err)
	}
	live.appClient.notify("initialized")
	return nil
}

func (m *appServerManager) startOrResumeThread(ctx context.Context, live *liveSession, threadID string) (string, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID != "" {
		resumed, err := m.resumeThread(ctx, live, threadID)
		if err == nil {
			if strings.TrimSpace(resumed) != "" {
				return resumed, nil
			}
			return threadID, nil
		}
		if live.appClient != nil {
			live.appClient.logDebug("codex app-server thread resume failed; starting a new thread", "thread_id", threadID, "error", err)
		}
	}
	return m.startThread(ctx, live)
}

func (m *appServerManager) startThread(ctx context.Context, live *liveSession) (string, error) {
	params := appServerThreadStartParams(live.spec)
	raw, err := live.appClient.request(ctx, "thread/start", params)
	if err != nil {
		return "", err
	}
	return appServerThreadIDFromResult(raw)
}

func (m *appServerManager) resumeThread(ctx context.Context, live *liveSession, threadID string) (string, error) {
	params := appServerThreadResumeParams(live.spec, threadID)
	raw, err := live.appClient.request(ctx, "thread/resume", params)
	if err != nil {
		return "", err
	}
	resumed, err := appServerThreadIDFromResult(raw)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(resumed) == "" {
		return threadID, nil
	}
	return resumed, nil
}

func appServerThreadStartParams(spec SessionSpec) map[string]any {
	spec.Profile = spec.Profile.Normalized()
	params := map[string]any{
		"cwd":                    spec.WorkspaceDir,
		"persistExtendedHistory": true,
		"experimentalRawEvents":  false,
	}
	if spec.Profile.ModelID != "" {
		params["model"] = spec.Profile.ModelID
	}
	if config := appServerReasoningConfig(spec.Profile.ReasoningEffort); len(config) > 0 {
		params["config"] = config
	}
	return params
}

func appServerThreadResumeParams(spec SessionSpec, threadID string) map[string]any {
	spec.Profile = spec.Profile.Normalized()
	params := map[string]any{
		"threadId": strings.TrimSpace(threadID),
		"cwd":      spec.WorkspaceDir,
	}
	if spec.Profile.ModelID != "" {
		params["model"] = spec.Profile.ModelID
	}
	if config := appServerReasoningConfig(spec.Profile.ReasoningEffort); len(config) > 0 {
		params["config"] = config
	}
	return params
}

func appServerTurnStartParams(spec SessionSpec, threadID string, prompt string) map[string]any {
	spec.Profile = spec.Profile.Normalized()
	params := map[string]any{
		"threadId": strings.TrimSpace(threadID),
		"input": []map[string]any{
			{"type": "text", "text": prompt},
		},
	}
	if effort := strings.TrimSpace(spec.Profile.ReasoningEffort); effort != "" {
		params["effort"] = effort
	}
	return params
}

func (m *appServerManager) handleAppServerServerRequest(runtimeID string, live *liveSession, req appServerServerRequest) (any, error) {
	switch strings.TrimSpace(req.Method) {
	case "item/commandExecution/requestApproval", "execCommandApproval":
		return map[string]any{"decision": "accept"}, nil
	case "item/fileChange/requestApproval", "applyPatchApproval":
		return map[string]any{"decision": "accept"}, nil
	case "mcpServer/elicitation/request":
		return map[string]any{
			"action":  "accept",
			"content": nil,
			"_meta":   nil,
		}, nil
	case "item/tool/requestUserInput":
		return m.handleAppServerUserInputRequest(runtimeID, live, req)
	default:
		return nil, fmt.Errorf("unhandled server request: %s", strings.TrimSpace(req.Method))
	}
}

type appServerUserInputParams struct {
	ThreadID         string                       `json:"threadId"`
	TurnID           string                       `json:"turnId"`
	ItemID           string                       `json:"itemId"`
	Questions        []appServerUserInputQuestion `json:"questions"`
	AutoResolutionMS *uint64                      `json:"autoResolutionMs"`
}

type appServerUserInputQuestion struct {
	ID       string                     `json:"id"`
	Header   string                     `json:"header"`
	Question string                     `json:"question"`
	Options  []appServerUserInputOption `json:"options"`
	IsOther  bool                       `json:"isOther"`
	IsSecret bool                       `json:"isSecret"`
}

type appServerUserInputOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

func (m *appServerManager) handleAppServerUserInputRequest(runtimeID string, live *liveSession, req appServerServerRequest) (any, error) {
	if m.deps.UserInput == nil {
		return nil, fmt.Errorf("user input broker is not configured")
	}
	var params appServerUserInputParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, fmt.Errorf("decode request_user_input params: %w", err)
	}
	params.ThreadID = strings.TrimSpace(params.ThreadID)
	params.TurnID = strings.TrimSpace(params.TurnID)
	params.ItemID = strings.TrimSpace(params.ItemID)
	if params.ThreadID == "" || params.TurnID == "" || params.ItemID == "" {
		return nil, fmt.Errorf("request_user_input requires threadId, turnId, and itemId")
	}
	if live == nil || !live.appServerTracksThread(params.ThreadID) {
		return nil, fmt.Errorf("request_user_input references unknown thread %q", params.ThreadID)
	}
	questions := make([]activity.UserInputQuestionSnapshot, 0, len(params.Questions))
	for _, question := range params.Questions {
		options := make([]activity.UserInputOptionSnapshot, 0, len(question.Options))
		for _, option := range question.Options {
			options = append(options, activity.UserInputOptionSnapshot{
				Label:       option.Label,
				Description: option.Description,
			})
		}
		questions = append(questions, activity.UserInputQuestionSnapshot{
			ID:       question.ID,
			Header:   question.Header,
			Question: question.Question,
			Options:  options,
			IsOther:  question.IsOther,
			IsSecret: question.IsSecret,
		})
	}
	var autoResolve time.Duration
	if params.AutoResolutionMS != nil {
		if *params.AutoResolutionMS < 60_000 || *params.AutoResolutionMS > 240_000 {
			return nil, fmt.Errorf("autoResolutionMs must be between 60000 and 240000")
		}
		autoResolve = time.Duration(*params.AutoResolutionMS) * time.Millisecond
	}
	live.notifyAppServerTurn(params.ThreadID, appServerTurnResult{
		activity:       "request_user_input:pending",
		progress:       true,
		userInputDelta: 1,
	})
	defer live.notifyAppServerTurn(params.ThreadID, appServerTurnResult{
		activity:       "request_user_input:resolved",
		userInputDelta: -1,
	})
	decision, err := m.deps.UserInput.Request(context.Background(), PendingUserInputRequest{
		Execution: activity.ExecutionRef{
			RuntimeKind: agentruntime.KindCodex,
			RuntimeID:   runtimeID,
			SessionID:   params.ThreadID,
			TurnID:      params.TurnID,
			ToolCallID:  params.ItemID,
			ToolKind:    "request_user_input",
		},
		ServerRequestID: appServerRequestID(req.ID),
		Questions:       questions,
		RequestedAt:     time.Now().UTC(),
		AutoResolve:     autoResolve,
	})
	if err != nil {
		return nil, err
	}
	return decision.Response, nil
}

func appServerRequestID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	var number int64
	if json.Unmarshal(raw, &number) == nil {
		return fmt.Sprintf("%d", number)
	}
	return strings.TrimSpace(string(raw))
}

func appServerReasoningConfig(effort string) map[string]any {
	effort = strings.TrimSpace(effort)
	if effort == "" {
		return nil
	}
	return map[string]any{"model_reasoning_effort": effort}
}

func appServerTurnIDFromResult(raw json.RawMessage) string {
	var result struct {
		TurnID string `json:"turnId"`
		ID     string `json:"id"`
		Turn   struct {
			ID     string `json:"id"`
			TurnID string `json:"turnId"`
		} `json:"turn"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &result) != nil {
		return ""
	}
	for _, candidate := range []string{result.TurnID, result.ID, result.Turn.TurnID, result.Turn.ID} {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			return candidate
		}
	}
	return ""
}

func appServerThreadIDFromResult(raw json.RawMessage) (string, error) {
	var result struct {
		ThreadID string `json:"threadId"`
		ID       string `json:"id"`
		Thread   struct {
			ID       string `json:"id"`
			ThreadID string `json:"threadId"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode thread result: %w", err)
	}
	for _, candidate := range []string{result.ThreadID, result.ID, result.Thread.ThreadID, result.Thread.ID} {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("thread result missing thread id")
}

func appServerPromptText(req PromptRequest) (string, error) {
	if len(req.Prompt) == 0 {
		return "", fmt.Errorf("prompt text is required")
	}
	parts := make([]string, 0, len(req.Prompt))
	for _, block := range req.Prompt {
		switch {
		case block.Text != nil:
			parts = append(parts, block.Text.Text)
		case block.ResourceLink != nil || block.Resource != nil:
			if text := textFromPromptBlock(block); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		default:
			return "", fmt.Errorf("unsupported prompt content block for codex app-server")
		}
	}
	text := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if text == "" {
		return "", fmt.Errorf("prompt text is required")
	}
	return text, nil
}

func (m *appServerManager) persistedThreadID(spec SessionSpec) string {
	if m.deps.ReadFile == nil {
		return ""
	}
	path := strings.TrimSpace(spec.RuntimeDir)
	if path == "" {
		return ""
	}
	data, err := m.deps.ReadFile(path + string(os.PathSeparator) + sessionFileName)
	if err != nil {
		return ""
	}
	var meta sessionMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.SessionID)
}

type appServerTurnWaiter struct {
	mu            sync.RWMutex
	threadID      string
	turnID        string
	ch            chan appServerTurnResult
	lastActivity  string
	pendingInputs int
}

type appServerTurnResult struct {
	success               bool
	stopReason            string
	err                   error
	turnID                string
	activity              string
	started               bool
	progress              bool
	assistantActivity     bool
	userInputStateChanged bool
	waitingForUser        bool
	userInputDelta        int
}

func (s *liveSession) registerAppServerTurnWaiter(threadID string) (*appServerTurnWaiter, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, fmt.Errorf("thread id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.turnWaiters == nil {
		s.turnWaiters = make(map[string]*appServerTurnWaiter)
	}
	if s.turnWaiters[threadID] != nil {
		return nil, fmt.Errorf("codex turn already in progress for thread %s", threadID)
	}
	waiter := &appServerTurnWaiter{
		threadID:     threadID,
		ch:           make(chan appServerTurnResult, 8),
		lastActivity: "turn/start",
	}
	s.turnWaiters[threadID] = waiter
	return waiter, nil
}

func (s *liveSession) removeAppServerTurnWaiter(threadID string, waiter *appServerTurnWaiter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.turnWaiters[strings.TrimSpace(threadID)] == waiter {
		delete(s.turnWaiters, strings.TrimSpace(threadID))
	}
}

func (s *liveSession) notifyAppServerTurn(threadID string, result appServerTurnResult) bool {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return false
	}
	s.mu.Lock()
	waiter := s.turnWaiters[threadID]
	s.mu.Unlock()
	if waiter == nil {
		return false
	}
	waiter.apply(&result)
	if result.userInputStateChanged {
		select {
		case waiter.ch <- result:
		case <-s.done:
		}
		return true
	}
	select {
	case waiter.ch <- result:
	default:
	}
	return true
}

func (w *appServerTurnWaiter) setTurnID(turnID string) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.turnID = turnID
}

func (w *appServerTurnWaiter) apply(result *appServerTurnResult) {
	if w == nil || result == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if result.turnID != "" {
		w.turnID = strings.TrimSpace(result.turnID)
	}
	if result.activity != "" {
		w.lastActivity = strings.TrimSpace(result.activity)
	}
	if result.userInputDelta != 0 {
		w.pendingInputs += result.userInputDelta
		if w.pendingInputs < 0 {
			w.pendingInputs = 0
		}
		result.userInputStateChanged = true
		result.waitingForUser = w.pendingInputs > 0
	}
}

func (w *appServerTurnWaiter) currentTurnID() string {
	if w == nil {
		return ""
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return strings.TrimSpace(w.turnID)
}

func (w *appServerTurnWaiter) currentLastActivity() string {
	if w == nil {
		return ""
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return strings.TrimSpace(w.lastActivity)
}

func (m *appServerManager) waitAppServerTurn(ctx context.Context, live *liveSession, waiter *appServerTurnWaiter) (PromptResponse, error) {
	semanticTimeout := appServerSemanticInactivityTimeout
	if semanticTimeout <= 0 {
		semanticTimeout = 10 * time.Minute
	}
	noProgressTimeout := appServerFirstTurnNoProgressTimeout
	if noProgressTimeout <= 0 {
		noProgressTimeout = semanticTimeout
	}
	semanticTimer := time.NewTimer(semanticTimeout)
	semanticC := semanticTimer.C
	defer semanticTimer.Stop()
	maximumDuration := appServerMaximumTurnDuration
	var maximumTimer *time.Timer
	var maximumC <-chan time.Time
	var maximumRemaining time.Duration
	var maximumStartedAt time.Time
	if maximumDuration > 0 {
		maximumTimer = time.NewTimer(maximumDuration)
		maximumC = maximumTimer.C
		maximumRemaining = maximumDuration
		maximumStartedAt = time.Now()
		defer maximumTimer.Stop()
	}

	var noProgressTimer *time.Timer
	var noProgressC <-chan time.Time
	waitStartedAt := time.Now()
	lastActivityAt := waitStartedAt
	waitingForUser := false
	stopNoProgress := func() {
		if noProgressTimer == nil {
			return
		}
		if !noProgressTimer.Stop() {
			select {
			case <-noProgressTimer.C:
			default:
			}
		}
		noProgressC = nil
	}
	defer stopNoProgress()

	resetSemantic := func() {
		if !semanticTimer.Stop() {
			select {
			case <-semanticTimer.C:
			default:
			}
		}
		semanticTimer.Reset(semanticTimeout)
		semanticC = semanticTimer.C
	}
	pauseSemantic := func() {
		if !semanticTimer.Stop() {
			select {
			case <-semanticTimer.C:
			default:
			}
		}
		semanticC = nil
	}
	pauseMaximum := func(now time.Time) {
		if maximumTimer == nil || maximumC == nil {
			return
		}
		maximumRemaining -= now.Sub(maximumStartedAt)
		if maximumRemaining < 0 {
			maximumRemaining = 0
		}
		if !maximumTimer.Stop() {
			select {
			case <-maximumTimer.C:
			default:
			}
		}
		maximumC = nil
	}
	resumeMaximum := func(now time.Time) {
		if maximumTimer == nil || maximumC != nil {
			return
		}
		remaining := maximumRemaining
		if remaining <= 0 {
			remaining = time.Nanosecond
		}
		maximumStartedAt = now
		maximumTimer.Reset(remaining)
		maximumC = maximumTimer.C
	}

	for {
		select {
		case result := <-waiter.ch:
			if result.userInputStateChanged && result.waitingForUser != waitingForUser {
				now := time.Now()
				waitingForUser = result.waitingForUser
				if waitingForUser {
					pauseSemantic()
					pauseMaximum(now)
				} else {
					resetSemantic()
					resumeMaximum(now)
				}
			}
			if result.activity != "" {
				now := time.Now()
				if live.appClient != nil {
					live.appClient.logDebug("codex app-server turn activity",
						"runtime_id", strings.TrimSpace(live.spec.RuntimeID),
						"thread_id", strings.TrimSpace(waiter.threadID),
						"turn_id", waiter.currentTurnID(),
						"activity", strings.TrimSpace(result.activity),
						"elapsed", now.Sub(waitStartedAt),
						"since_previous_activity", now.Sub(lastActivityAt),
						"started", result.started,
						"progress", result.progress,
						"success", result.success,
					)
				}
				lastActivityAt = now
				if !waitingForUser {
					resetSemantic()
				}
			}
			if result.started && noProgressTimeout > 0 && noProgressTimer == nil {
				noProgressTimer = time.NewTimer(noProgressTimeout)
				noProgressC = noProgressTimer.C
			}
			if result.progress || result.assistantActivity {
				stopNoProgress()
			}
			if result.userInputStateChanged {
				stopNoProgress()
			}
			if result.err != nil {
				return PromptResponse{}, result.err
			}
			if result.success {
				stopReason := result.stopReason
				if stopReason == "" {
					stopReason = StopReasonEndTurn
				}
				return PromptResponse{StopReason: stopReason}, nil
			}
		case <-noProgressC:
			if live.appClient != nil {
				live.appClient.logDebug("codex app-server turn no progress timeout",
					"runtime_id", strings.TrimSpace(live.spec.RuntimeID),
					"thread_id", strings.TrimSpace(waiter.threadID),
					"turn_id", waiter.currentTurnID(),
					"timeout", noProgressTimeout,
					"last_activity", waiter.currentLastActivity(),
				)
			}
			return PromptResponse{}, m.failTimedOutAppServerTurn(live, waiter, "initial assistant activity", noProgressTimeout)
		case <-semanticC:
			if live.appClient != nil {
				live.appClient.logDebug("codex app-server turn semantic inactivity timeout",
					"runtime_id", strings.TrimSpace(live.spec.RuntimeID),
					"thread_id", strings.TrimSpace(waiter.threadID),
					"turn_id", waiter.currentTurnID(),
					"timeout", semanticTimeout,
					"last_activity", waiter.currentLastActivity(),
				)
			}
			return PromptResponse{}, m.failTimedOutAppServerTurn(live, waiter, "app-server inactivity", semanticTimeout)
		case <-maximumC:
			return PromptResponse{}, m.failTimedOutAppServerTurn(live, waiter, "maximum turn duration", maximumDuration)
		case <-ctx.Done():
			m.stopAppServerTurn(live, waiter, "prompt context canceled")
			return PromptResponse{}, ctx.Err()
		}
	}
}

func (m *appServerManager) failTimedOutAppServerTurn(live *liveSession, waiter *appServerTurnWaiter, reason string, timeout time.Duration) error {
	m.stopAppServerTurn(live, waiter, reason)
	return fmt.Errorf("Codex stopped responding after %s. The turn was canceled; try again", timeout)
}

func (m *appServerManager) stopAppServerTurn(live *liveSession, waiter *appServerTurnWaiter, reason string) {
	if m.deps.UserInput != nil {
		m.deps.UserInput.CancelSession(live.spec.RuntimeID, waiter.threadID)
	}
	interruptErr := m.interruptAppServerTurn(live, waiter)
	stderrTail := m.stderrTail(live.spec, 2048)
	if live.appClient != nil {
		live.appClient.logError("codex app-server turn stopped",
			"runtime_id", strings.TrimSpace(live.spec.RuntimeID),
			"thread_id", strings.TrimSpace(waiter.threadID),
			"turn_id", waiter.currentTurnID(),
			"reason", strings.TrimSpace(reason),
			"last_activity", waiter.currentLastActivity(),
			"interrupt_error", interruptErr,
			"stderr_tail", stderrTail,
		)
	}
	if interruptErr == nil {
		return
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), appServerStopTimeout)
	defer cancel()
	if err := m.Stop(stopCtx, SessionHandle{RuntimeID: live.spec.RuntimeID}); err != nil && live.appClient != nil {
		live.appClient.logError("stop codex app-server after turn interrupt failure",
			"runtime_id", strings.TrimSpace(live.spec.RuntimeID),
			"error", err,
		)
	}
}

func (m *appServerManager) interruptAppServerTurn(live *liveSession, waiter *appServerTurnWaiter) error {
	if live == nil || live.appClient == nil {
		return fmt.Errorf("codex app-server client is unavailable")
	}
	threadID := strings.TrimSpace(waiter.threadID)
	turnID := waiter.currentTurnID()
	if threadID == "" || turnID == "" {
		return fmt.Errorf("codex app-server turn identity is incomplete")
	}

	ctx, cancel := context.WithTimeout(context.Background(), appServerTurnInterruptTimeout)
	defer cancel()
	if _, err := live.appClient.request(ctx, "turn/interrupt", map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
	}); err != nil {
		return fmt.Errorf("interrupt codex app-server turn: %w", err)
	}

	for {
		select {
		case result := <-waiter.ch:
			if result.err != nil || result.success {
				return nil
			}
		case <-live.done:
			return nil
		case <-ctx.Done():
			return fmt.Errorf("wait for interrupted codex app-server turn: %w", ctx.Err())
		}
	}
}

func (m *appServerManager) readAppServerStdout(runtimeID string, live *liveSession, stdout io.Reader) {
	decoder := json.NewDecoder(stdout)
	for {
		var msg appServerWireMessage
		if err := decoder.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			if live.appClient != nil {
				live.appClient.logDebug("codex app-server stdout decoder stopped", "runtime_id", runtimeID, "error", err)
			}
			return
		}
		if live.appClient != nil {
			live.appClient.handleMessage(msg)
		}
	}
}

func (m *appServerManager) waitAppServerSession(runtimeID string, live *liveSession) {
	err := live.cmd.Wait()
	exitCode := 0
	if err != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	if live.appClient != nil {
		live.appClient.closeAllPending(fmt.Errorf("codex app-server exited with code %d", exitCode))
	}
	isCurrent := m.detachSession(runtimeID, live)
	if live.session != nil {
		live.session.ProcessID = 0
		if isCurrent && m.deps.OnExit != nil {
			m.deps.OnExit(live.session, exitCode)
		}
	}
	if isCurrent && m.deps.Permission != nil {
		m.deps.Permission.CancelSession(runtimeID, "")
	}
	if isCurrent && m.deps.UserInput != nil {
		m.deps.UserInput.CancelSession(runtimeID, "")
	}
	if live.stderr != nil {
		_ = live.stderr.Close()
	}
	close(live.done)
}

func (m *appServerManager) detachSession(runtimeID string, live *liveSession) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions[runtimeID] != live {
		return false
	}
	delete(m.sessions, runtimeID)
	return true
}

func (m *appServerManager) wrapStartupError(spec SessionSpec, action string, err error) error {
	tail := m.stderrTail(spec, 4096)
	if tail == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w; stderr tail: %s", action, err, tail)
}

func (m *appServerManager) stderrTail(spec SessionSpec, limit int) string {
	if limit <= 0 || m.deps.ReadFile == nil {
		return ""
	}
	data, err := m.deps.ReadFile(spec.StderrPath)
	if err != nil || len(data) == 0 {
		return ""
	}
	if len(data) > limit {
		data = data[len(data)-limit:]
	}
	return redactAppServerDiagnostic(bytes.TrimSpace(data), spec)
}

func redactAppServerDiagnostic(data []byte, spec SessionSpec) string {
	text := string(data)
	for _, secret := range []string{spec.Profile.APIKey, os.Getenv("OPENAI_API_KEY")} {
		secret = strings.TrimSpace(secret)
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "api_key") ||
			strings.Contains(lower, "apikey") ||
			strings.Contains(lower, "token") ||
			strings.Contains(lower, "secret") ||
			strings.Contains(lower, "password") {
			lines[i] = redactSecretishLine(line)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func redactSecretishLine(line string) string {
	for _, sep := range []string{"=", ":"} {
		if before, _, ok := strings.Cut(line, sep); ok {
			return strings.TrimRight(before, " \t") + sep + " [redacted]"
		}
	}
	return "[redacted]"
}
