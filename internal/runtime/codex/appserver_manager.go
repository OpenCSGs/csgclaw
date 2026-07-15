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

	"csgclaw/internal/codexcli"
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
		fallbackCompleted:     make(map[string]struct{}),
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
		"turn_id", strings.TrimSpace(waiter.turnID),
		"duration", time.Since(turnStartAt),
	)

	waitStartAt := time.Now()
	resp, err := m.waitAppServerTurn(ctx, live, waiter)
	if err != nil {
		live.appClient.logDebug("codex app-server turn wait failed",
			"runtime_id", runtimeID,
			"thread_id", sessionID,
			"turn_id", strings.TrimSpace(waiter.turnID),
			"duration", time.Since(waitStartAt),
			"last_activity", strings.TrimSpace(waiter.lastActivity),
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
		"turn_id", strings.TrimSpace(waiter.turnID),
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

func (m *appServerManager) handleAppServerServerRequest(_ string, _ *liveSession, req appServerServerRequest) (any, error) {
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
	default:
		return nil, fmt.Errorf("unhandled server request: %s", strings.TrimSpace(req.Method))
	}
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
	threadID      string
	turnID        string
	ch            chan appServerTurnResult
	started       bool
	progress      bool
	lastActivity  string
	lastUpdatedAt time.Time
}

type appServerTurnResult struct {
	success           bool
	stopReason        string
	err               error
	turnID            string
	activity          string
	started           bool
	progress          bool
	assistantActivity bool
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
		threadID:      threadID,
		ch:            make(chan appServerTurnResult, 8),
		lastActivity:  "turn/start",
		lastUpdatedAt: time.Now().UTC(),
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
	if waiter != nil {
		if result.turnID != "" {
			waiter.turnID = result.turnID
		}
		if result.activity != "" {
			waiter.lastActivity = result.activity
			waiter.lastUpdatedAt = time.Now().UTC()
		}
		if result.started {
			waiter.started = true
		}
		if result.progress {
			waiter.progress = true
		}
	}
	s.mu.Unlock()
	if waiter == nil {
		return false
	}
	select {
	case waiter.ch <- result:
	default:
	}
	return true
}

func (s *liveSession) markFallbackCompleted(threadID string) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallbackCompleted == nil {
		s.fallbackCompleted = make(map[string]struct{})
	}
	s.fallbackCompleted[threadID] = struct{}{}
}

func (s *liveSession) consumeFallbackCompleted(threadID string) bool {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.fallbackCompleted[threadID]; !ok {
		return false
	}
	delete(s.fallbackCompleted, threadID)
	return true
}

func (w *appServerTurnWaiter) setTurnID(turnID string) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return
	}
	w.turnID = turnID
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
	defer semanticTimer.Stop()
	maximumDuration := appServerMaximumTurnDuration
	var maximumTimer *time.Timer
	var maximumC <-chan time.Time
	if maximumDuration > 0 {
		maximumTimer = time.NewTimer(maximumDuration)
		maximumC = maximumTimer.C
		defer maximumTimer.Stop()
	}

	var noProgressTimer *time.Timer
	var noProgressC <-chan time.Time
	waitStartedAt := time.Now()
	lastActivityAt := waitStartedAt
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
	}

	for {
		select {
		case result := <-waiter.ch:
			if result.activity != "" {
				now := time.Now()
				if live.appClient != nil {
					live.appClient.logDebug("codex app-server turn activity",
						"runtime_id", strings.TrimSpace(live.spec.RuntimeID),
						"thread_id", strings.TrimSpace(waiter.threadID),
						"turn_id", strings.TrimSpace(waiter.turnID),
						"activity", strings.TrimSpace(result.activity),
						"elapsed", now.Sub(waitStartedAt),
						"since_previous_activity", now.Sub(lastActivityAt),
						"started", result.started,
						"progress", result.progress,
						"success", result.success,
					)
				}
				lastActivityAt = now
				resetSemantic()
			}
			if result.started && noProgressTimeout > 0 && noProgressTimer == nil {
				noProgressTimer = time.NewTimer(noProgressTimeout)
				noProgressC = noProgressTimer.C
			}
			if result.progress || result.assistantActivity {
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
					"turn_id", strings.TrimSpace(waiter.turnID),
					"timeout", noProgressTimeout,
					"last_activity", strings.TrimSpace(waiter.lastActivity),
				)
			}
			return PromptResponse{}, m.failTimedOutAppServerTurn(live, waiter, "initial assistant activity", noProgressTimeout)
		case <-semanticTimer.C:
			if live.appClient != nil {
				live.appClient.logDebug("codex app-server turn semantic inactivity timeout",
					"runtime_id", strings.TrimSpace(live.spec.RuntimeID),
					"thread_id", strings.TrimSpace(waiter.threadID),
					"turn_id", strings.TrimSpace(waiter.turnID),
					"timeout", semanticTimeout,
					"last_activity", strings.TrimSpace(waiter.lastActivity),
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
	interruptErr := m.interruptAppServerTurn(live, waiter)
	stderrTail := m.stderrTail(live.spec, 2048)
	if live.appClient != nil {
		live.appClient.logError("codex app-server turn stopped",
			"runtime_id", strings.TrimSpace(live.spec.RuntimeID),
			"thread_id", strings.TrimSpace(waiter.threadID),
			"turn_id", strings.TrimSpace(waiter.turnID),
			"reason", strings.TrimSpace(reason),
			"last_activity", strings.TrimSpace(waiter.lastActivity),
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
	turnID := strings.TrimSpace(waiter.turnID)
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
	if live.session != nil {
		live.session.ProcessID = 0
		if m.deps.OnExit != nil {
			m.deps.OnExit(live.session, exitCode)
		}
	}
	if m.deps.Permission != nil {
		m.deps.Permission.CancelSession(runtimeID, "")
	}
	if live.stderr != nil {
		_ = live.stderr.Close()
	}
	m.mu.Lock()
	delete(m.sessions, runtimeID)
	m.mu.Unlock()
	close(live.done)
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
