package codex

import (
	"bytes"
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/diagnostics"
	"csgclaw/internal/modelcap"
	"csgclaw/internal/modelprovider"
	"csgclaw/internal/modelusage"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"csgclaw/internal/activity"
	"csgclaw/internal/codexcli"
	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	runtimeinstructions "csgclaw/internal/runtime/instructions"
)

const (
	appServerTurnInterruptTimeout      = 5 * time.Second
	appServerStdoutExitGracePeriod     = 250 * time.Millisecond
	appServerStdoutExitBufferBytes     = 1 << 20
	appServerTrackedTurnLimit          = 256
	appServerPublishFileToolName       = "csgclaw_publish_file"
	appServerUploadFileToolName        = "csgclaw_upload_file"
	appServerMaxPublishedFileNameBytes = 255
	appServerMaxUploadBytes            = 50 << 20
)

const appServerDeveloperInstructions = `- Base each new turn on the latest user message. Use earlier requests and attachments as context only when they are relevant to that message.
- Do not resume unfinished work from earlier turns unless the latest user message asks to continue or clearly depends on that work. An unanswered or failed request does not authorize a retry.
- When the latest user message is only a greeting, thanks, or acknowledgement, reply directly without using tools or inspecting earlier attachments.` +
	"\n" + runtimeinstructions.ImageGenerationPromptPolicy + "\n" + runtimeinstructions.VideoGenerationPromptPolicy

var appServerStopTimeout = 3 * time.Second

var appServerCommandContext = codexcli.AppServerCommandContextWithOverrides

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
		cloned.ExtensionDigests = maps.Clone(cloned.ExtensionDigests)
		return &cloned, nil
	}

	stderrFile, err := m.deps.OpenFile(spec.StderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open stderr log %s: %w", spec.StderrPath, err)
	}

	telemetry, telemetryErr := newNativeTelemetry()
	if telemetryErr != nil {
		slog.Warn("native diagnostics unavailable", "error", telemetryErr)
	}
	ownedTelemetry := false
	defer func() {
		if !ownedTelemetry {
			telemetry.close()
		}
	}()
	environment, extensionDigests, err := buildSessionEnvironment(spec)
	if err != nil {
		_ = stderrFile.Close()
		return nil, fmt.Errorf("prepare Runtime extension environment: %w", err)
	}
	overrides := append(appServerOverrides(spec), telemetry.overrides()...)
	if len(extensionDigests) > 0 && spec.ExecutionMode != ExecutionModeReadOnly {
		// Keep managed commands independent of login scripts and shell snapshots.
		overrides = append(overrides, "--config", "allow_login_shell=false")
		for _, entry := range environment {
			key, value, _ := strings.Cut(entry, "=")
			if canonical := agentruntime.CanonicalEnvironmentKey(key); canonical == "PATH" || canonical == "LARKSUITE_CLI_CONFIG_DIR" {
				overrides = append(overrides, "--config", "shell_environment_policy.set."+key+"="+tomlQuotedKey(value))
			}
		}
	}
	cmd, err := appServerCommandContext(ctx, spec.BinaryPath, overrides)
	if err != nil {
		_ = stderrFile.Close()
		return nil, fmt.Errorf("prepare codex app-server command: %w", err)
	}
	cmd.Dir = spec.WorkspaceDir
	cmd.Env = environment
	if telemetry != nil {
		// Keep completed-turn exports timely; this does not block the turn itself.
		cmd.Env = append(cmd.Env, "OTEL_BSP_SCHEDULE_DELAY=1000")
	}

	if sysProcAttr := newSessionSysProcAttr(); sysProcAttr != nil {
		cmd.SysProcAttr = sysProcAttr
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = stderrFile.Close()
		return nil, fmt.Errorf("codex app-server stdin pipe: %w", err)
	}
	// Own the read end so cmd.Wait cannot close it before the last turn
	// notification has been decoded.
	stdout, stdoutWriter, err := os.Pipe()
	if err != nil {
		_ = stdin.Close()
		_ = stderrFile.Close()
		return nil, fmt.Errorf("codex app-server stdout pipe: %w", err)
	}
	cmd.Stdout = stdoutWriter
	workerLog, err := diagnostics.StartWithStderr(cmd, stderrFile, "codex", spec.RuntimeID)
	if err != nil {
		_ = stdoutWriter.Close()
		_ = stdout.Close()
		_ = stdin.Close()
		_ = stderrFile.Close()
		diagnostics.ReportFailure(ctx, "runtime.process_start", err, map[string]any{"runtime_id": spec.RuntimeID, "stderr_path": spec.StderrPath})
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}

	_ = stdoutWriter.Close()

	diagnosticObserver := workerLog.Observer
	diagnosticObserver.Bind(ctx)
	defer diagnosticObserver.End(ctx)
	logger := slog.New(diagnostics.NewServerLogHandler(workerLog, slog.LevelInfo, nil))
	appClient := newAppServerClient(stdin, logger)
	conversationSessions := cloneConversationSessions(spec.ConversationSessions)
	if conversationSessions == nil {
		conversationSessions = make(map[string]string)
	}
	filePublishingThreads := make(map[string]bool)
	for conversationKey := range cloneFilePublishingConversations(spec.FilePublishingConversations) {
		if threadID := strings.TrimSpace(conversationSessions[conversationKey]); threadID != "" {
			filePublishingThreads[threadID] = true
		}
	}
	live := &liveSession{
		diagnosticObserver:    diagnosticObserver,
		telemetry:             telemetry,
		mcpCatalogRevision:    spec.MCPCatalogRevision,
		cmd:                   cmd,
		stdin:                 stdin,
		stderr:                workerLog,
		done:                  make(chan struct{}),
		spec:                  spec,
		appClient:             appClient,
		conversationSessions:  conversationSessions,
		memoryCheckpointLast:  make(map[string]time.Time),
		memoryCheckpointBusy:  make(map[string]bool),
		loadedConversations:   make(map[string]bool),
		filePublishingThreads: filePublishingThreads,
		turnWaiters:           make(map[string]*appServerTurnWaiter),
		turnContexts:          make(map[string]appServerTurnContext),
		turnThreads:           make(map[string]appServerTrackedTurn),
		commandOutputs:        make(map[string]*appServerCommandOutputState),
		replayedAgentMessages: make(map[string]struct{}),
		streamedAgentMessages: make(map[string]struct{}),
		streamedAgentThreads:  make(map[string]struct{}),
		agentMessagePhases:    make(map[string]string),
		agentMessageStreams:   make(map[string]*assistantStructuredOutputStream),
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

	ownedTelemetry = true
	go m.waitAppServerSession(spec.RuntimeID, live, stdout)

	if err := m.initializeHandshake(ctx, live); err != nil {
		_ = m.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID})
		return nil, m.wrapStartupError(spec, "initialize codex app-server", err)
	}
	if spec.ExecutionMode == ExecutionModeReadOnly {
		if err := m.disableSystemSkills(ctx, live); err != nil {
			_ = m.Stop(context.Background(), SessionHandle{RuntimeID: spec.RuntimeID})
			return nil, m.wrapStartupError(spec, "disable system skills for read-only agent", err)
		}
	}

	threadID, _, err := m.startOrResumeThread(ctx, live, m.persistedThreadID(spec), false)
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
		PreserveSkills:                 spec.PreserveSkills,
		RuntimeID:                      spec.RuntimeID,
		AgentID:                        spec.AgentID,
		AgentName:                      spec.AgentName,
		SessionID:                      threadID,
		BinaryPath:                     spec.BinaryPath,
		RuntimeDir:                     spec.RuntimeDir,
		WorkspaceDir:                   spec.WorkspaceDir,
		HomeDir:                        spec.HomeDir,
		CodexHomeDir:                   spec.CodexHomeDir,
		StderrPath:                     spec.StderrPath,
		ProcessID:                      cmd.Process.Pid,
		CreatedAt:                      now,
		StartedAt:                      now,
		ConversationSessions:           cloneConversationSessions(spec.ConversationSessions),
		ExtensionDigests:               extensionDigests,
		FilePublishingConversations:    cloneFilePublishingConversations(spec.FilePublishingConversations),
		ConversationProfileFingerprint: strings.TrimSpace(spec.ConversationProfileFingerprint),
	}
	live.mu.Lock()
	live.session = session
	live.mu.Unlock()

	cloned := *session
	cloned.ExtensionDigests = maps.Clone(cloned.ExtensionDigests)
	return &cloned, nil
}

func (m *appServerManager) disableSystemSkills(ctx context.Context, live *liveSession) error {
	if live == nil || live.appClient == nil {
		return fmt.Errorf("codex app-server session is unavailable")
	}
	root := filepath.Join(live.spec.CodexHomeDir, "skills", ".system")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read system skills: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if _, err := live.appClient.request(ctx, "skills/config/write", map[string]any{"path": path, "enabled": false}); err != nil {
			return fmt.Errorf("disable system skill %q: %w", entry.Name(), err)
		}
	}
	return nil
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
			return fmt.Errorf("stop tracked app-server pid=%d: %w", live.cmd.Process.Pid, err)
		}
	}

	timeout := time.NewTimer(appServerStopTimeout)
	defer timeout.Stop()
	select {
	case <-live.done:
		live.mu.Lock()
		stopErr := live.stopErr
		live.mu.Unlock()
		if stopErr != nil {
			return fmt.Errorf("cleanup tracked app-server pid=%d: %w", live.cmd.Process.Pid, stopErr)
		}
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
	cloned.ExtensionDigests = maps.Clone(cloned.ExtensionDigests)
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
	cloned.ExtensionDigests = maps.Clone(cloned.ExtensionDigests)
	return &cloned, nil
}

func (m *appServerManager) Prompt(ctx context.Context, handle SessionHandle, req PromptRequest) (PromptResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	runtimeID := strings.TrimSpace(handle.RuntimeID)
	live, err := m.ensureLiveSession(ctx, handle)
	if err != nil {
		return PromptResponse{}, err
	}
	if m.deps.BeforePrompt != nil {
		if err := m.deps.BeforePrompt(ctx, handle); err != nil {
			return PromptResponse{}, err
		}
	}

	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(live.session.SessionID)
		req.SessionID = sessionID
	}
	promptInput, err := appServerPromptInput(req)
	if err != nil {
		m.publishAppServerEvent(promptFailedEvent(runtimeID, sessionID, err))
		return PromptResponse{}, err
	}
	spec := roomThreadSpec(ctx, live.spec)
	model := req.Model
	if _, scoped := ctx.Value(roomThreadSettingsKey{}).(roomThreadSettings); scoped && model == nil {
		selected := turnModelFromProfile(spec.Profile)
		model = &selected
	}
	selectedModel, err := live.resolveTurnModel(model)
	if err != nil {
		return PromptResponse{}, err
	}

	live.diagnosticObserver.Bind(ctx)
	defer live.diagnosticObserver.End(ctx)
	diagnostics.From(ctx).Capture("runtime.configuration", map[string]any{"runtime": "codex", "runtime_id": runtimeID, "binary": live.spec.BinaryPath, "workspace": spec.WorkspaceDir, "stderr_path": spec.StderrPath, "profile": spec.Profile, "mcp_servers": live.spec.MCPServers, "mcp_revision": live.spec.MCPCatalogRevision, "execution_mode": live.spec.ExecutionMode, "extension_digests": live.session.ExtensionDigests, "runtime_version": live.serverInfo})
	diagnostics.From(ctx).CaptureRuntimeFiles(filepath.Join(live.spec.HomeDir, "AGENTS.md"), filepath.Join(live.spec.CodexHomeDir, "AGENTS.md"), filepath.Join(spec.WorkspaceDir, "AGENTS.md"), filepath.Join(spec.WorkspaceDir, "AGENTS.override.md"), filepath.Join(live.spec.CodexHomeDir, "config.toml"))
	diagnostics.From(ctx).Capture("runtime.turn_model", selectedModel)
	diagnostics.From(ctx).RuntimeInfo("codex", selectedModel.ModelID)
	live.appClient.logInfoContext(ctx, "model request configuration", "model", selectedModel.ModelID, "runtime_id", runtimeID, "thread_id", sessionID)
	waiter, err := live.registerAppServerTurnWaiterWithModel(sessionID, selectedModel, model != nil)
	if err != nil {
		m.publishAppServerEvent(promptFailedEvent(runtimeID, sessionID, err))
		return PromptResponse{}, err
	}
	waiter.mu.Lock()
	waiter.diagnostic = diagnostics.From(ctx)
	waiter.textOnlyUserInput = req.TextOnlyUserInput
	waiter.streamReasoningSummary = req.StreamReasoningSummary
	waiter.mu.Unlock()
	defer func() { live.removeAppServerTurnWaiter(sessionID, waiter) }()
	turnCtx, cancelTurn := context.WithCancel(ctx)
	live.setAppServerTurnContext(sessionID, waiter, turnCtx)
	defer func() {
		cancelTurn()
		live.clearAppServerTurnContext(sessionID, waiter)
	}()

	params := appServerTurnStartParamsForContext(ctx, live.spec, sessionID, promptInput, req.ClientUserMessageID)
	applyTurnModelParams(params, selectedModel)
	turnStartAt := time.Now()
	live.appClient.logDebugContext(ctx, "codex app-server turn start request",
		"runtime_id", runtimeID,
		"thread_id", sessionID,
		"prompt_blocks", len(promptInput),
	)
	raw, err := live.appClient.request(ctx, "turn/start", params)
	if err != nil {
		live.appClient.logDebugContext(ctx, "codex app-server turn start failed",
			"runtime_id", runtimeID,
			"thread_id", sessionID,
			"duration", time.Since(turnStartAt),
			"error", err,
		)
		if ctx.Err() != nil {
			response, stopErr := m.stopCanceledAppServerTurn(ctx, live, waiter, "turn/start context canceled")
			m.publishAppServerEvent(promptFailedEvent(runtimeID, sessionID, stopErr))
			return response, stopErr
		}
		err = fmt.Errorf("codex turn/start failed: %w", err)
		m.publishAppServerEvent(promptFailedEvent(runtimeID, sessionID, err))
		return PromptResponse{}, err
	}
	waiter.setTurnID(appServerTurnIDFromResult(raw))
	m.publishContextUsage(runtimeID, live, sessionID, nil, "snapshot")
	live.telemetry.bind(waiter.currentTurnID(), diagnostics.From(ctx))
	defer live.telemetry.finish(diagnostics.From(ctx))
	acceptedAt := time.Now()
	diagnostics.From(ctx).Interval("runtime.accepted", "runtime", acceptedAt, acceptedAt)
	if req.OnAccepted != nil {
		req.OnAccepted()
	}
	live.trackAppServerTurn(sessionID, waiter.currentTurnID())
	live.appClient.logDebugContext(ctx, "codex app-server turn start accepted",
		"runtime_id", runtimeID,
		"thread_id", sessionID,
		"turn_id", waiter.currentTurnID(),
		"duration", time.Since(turnStartAt),
	)

	waitStartAt := time.Now()
	resp, err := m.waitAppServerTurn(ctx, live, waiter)
	if isContextWindowError(err) && ctx.Err() == nil {
		resp, err = m.recoverContext(turnCtx, live, &waiter)
	}
	if err != nil {
		live.appClient.logDebugContext(ctx, "codex app-server turn wait failed",
			"runtime_id", runtimeID,
			"thread_id", sessionID,
			"turn_id", waiter.currentTurnID(),
			"duration", time.Since(waitStartAt),
			"last_activity", waiter.currentLastActivity(),
			"error", err,
		)
		m.publishTokenUsage(runtimeID, live, sessionID, waiter.currentTurnID(), nil, "snapshot")
		m.publishAppServerEvent(promptFailedEvent(runtimeID, sessionID, err))
		return resp, err
	}
	live.appClient.logDebugContext(ctx, "codex app-server turn wait completed",
		"runtime_id", runtimeID,
		"thread_id", sessionID,
		"turn_id", waiter.currentTurnID(),
		"duration", time.Since(waitStartAt),
		"stop_reason", strings.TrimSpace(resp.StopReason),
	)
	m.publishTokenUsage(runtimeID, live, sessionID, waiter.currentTurnID(), nil, "snapshot")
	m.publishAppServerEvent(promptCompletedEvent(runtimeID, sessionID, resp))
	m.maybeCreateMemoryCheckpoint(live, sessionID)
	return resp, nil
}

func (m *appServerManager) EnsureSession(ctx context.Context, handle SessionHandle, conversationKey string) (string, error) {
	return m.ensureSession(ctx, handle, conversationKey, false)
}

func (m *appServerManager) EnsureEngineSession(ctx context.Context, handle SessionHandle, conversationKey string) (string, error) {
	return m.ensureSession(ctx, handle, conversationKey, true)
}

func (m *appServerManager) ensureSession(ctx context.Context, handle SessionHandle, conversationKey string, publishFiles bool) (string, error) {
	live, err := m.ensureLiveSession(ctx, handle)
	if err != nil {
		return "", err
	}

	conversationKey = strings.TrimSpace(conversationKey)
	if conversationKey == "" {
		return strings.TrimSpace(live.session.SessionID), nil
	}

	logicalKey := contract.LogicalRuntimeSessionKey(conversationKey)
	if err := m.removeConversationBindings(live, logicalKey, conversationKey); err != nil {
		return "", err
	}

	live.mu.Lock()
	if threadID := strings.TrimSpace(live.conversationSessions[conversationKey]); threadID != "" && live.loadedConversations[conversationKey] {
		if !publishFiles || live.filePublishingThreads[threadID] {
			live.mu.Unlock()
			return threadID, nil
		}
	}
	restoredThreadID := strings.TrimSpace(live.conversationSessions[conversationKey])
	restoredLoaded := live.loadedConversations[conversationKey]
	restoredFilePublishing := live.filePublishingThreads[restoredThreadID]
	live.mu.Unlock()
	if restoredThreadID != "" && !restoredLoaded && (!publishFiles || restoredFilePublishing) {
		live.conversationResumeMu.Lock()
		live.conversationPersistMu.Lock()

		live.mu.Lock()
		if threadID := strings.TrimSpace(live.conversationSessions[conversationKey]); threadID != "" && live.loadedConversations[conversationKey] {
			if !publishFiles || live.filePublishingThreads[threadID] {
				live.mu.Unlock()
				live.conversationPersistMu.Unlock()
				live.conversationResumeMu.Unlock()
				return threadID, nil
			}
		}
		restoredThreadID = strings.TrimSpace(live.conversationSessions[conversationKey])
		restoredLoaded = live.loadedConversations[conversationKey]
		restoredFilePublishing = live.filePublishingThreads[restoredThreadID]
		live.mu.Unlock()
		if restoredThreadID == "" || restoredLoaded || (publishFiles && !restoredFilePublishing) {
			live.conversationPersistMu.Unlock()
			live.conversationResumeMu.Unlock()
			return m.ensureSession(ctx, handle, conversationKey, publishFiles)
		}

		threadID, filePublishing, err := m.startOrResumeThread(ctx, live, restoredThreadID, restoredFilePublishing)
		if err != nil {
			live.conversationPersistMu.Unlock()
			live.conversationResumeMu.Unlock()
			return "", err
		}
		live.mu.Lock()
		previous := live.conversationSessions[conversationKey]
		previousLoaded := live.loadedConversations[conversationKey]
		previousFilePublishing := live.filePublishingThreads[previous]
		live.conversationSessions[conversationKey] = threadID
		live.loadedConversations[conversationKey] = true
		live.filePublishingThreads[threadID] = filePublishing
		conversations := cloneConversationSessions(live.conversationSessions)
		filePublishingConversations := filePublishingConversationsForThreads(conversations, live.filePublishingThreads)
		live.mu.Unlock()
		if err := m.persistConversationSessions(live, conversations, filePublishingConversations); err != nil {
			live.mu.Lock()
			live.conversationSessions[conversationKey] = previous
			live.loadedConversations[conversationKey] = previousLoaded
			live.filePublishingThreads[previous] = previousFilePublishing
			delete(live.filePublishingThreads, threadID)
			live.mu.Unlock()
			live.conversationPersistMu.Unlock()
			live.conversationResumeMu.Unlock()
			return "", err
		}
		live.conversationPersistMu.Unlock()
		live.conversationResumeMu.Unlock()
		return threadID, nil
	}

	threadID, err := m.startThread(ctx, live, publishFiles)
	if err != nil {
		return "", err
	}

	live.conversationPersistMu.Lock()
	defer live.conversationPersistMu.Unlock()
	live.mu.Lock()
	existing := strings.TrimSpace(live.conversationSessions[conversationKey])
	if existing != "" && live.loadedConversations[conversationKey] && (!publishFiles || live.filePublishingThreads[existing]) {
		live.mu.Unlock()
		return existing, nil
	}
	previousLoaded := live.loadedConversations[conversationKey]
	previousFilePublishing := live.filePublishingThreads[existing]
	live.conversationSessions[conversationKey] = threadID
	live.loadedConversations[conversationKey] = true
	live.filePublishingThreads[threadID] = publishFiles
	conversations := cloneConversationSessions(live.conversationSessions)
	filePublishingConversations := filePublishingConversationsForThreads(conversations, live.filePublishingThreads)
	live.mu.Unlock()
	if err := m.persistConversationSessions(live, conversations, filePublishingConversations); err != nil {
		live.mu.Lock()
		if live.conversationSessions[conversationKey] == threadID {
			if existing == "" {
				delete(live.conversationSessions, conversationKey)
				delete(live.loadedConversations, conversationKey)
			} else {
				live.conversationSessions[conversationKey] = existing
				live.loadedConversations[conversationKey] = previousLoaded
				live.filePublishingThreads[existing] = previousFilePublishing
			}
			delete(live.filePublishingThreads, threadID)
		}
		live.mu.Unlock()
		return "", err
	}
	return threadID, nil
}

func (m *appServerManager) ExistingEngineSession(ctx context.Context, handle SessionHandle, conversationKey string) (string, bool, error) {
	live, err := m.ensureLiveSession(ctx, handle)
	if err != nil {
		return "", false, err
	}
	conversationKey = strings.TrimSpace(conversationKey)
	live.mu.Lock()
	threadID := strings.TrimSpace(live.conversationSessions[conversationKey])
	available := threadID != ""
	live.mu.Unlock()
	if !available {
		return "", false, nil
	}
	resolved, err := m.ensureSession(ctx, handle, conversationKey, true)
	if err != nil {
		return "", false, err
	}
	return resolved, true, nil
}

func (m *appServerManager) ExistingSession(ctx context.Context, handle SessionHandle, conversationKey string) (string, bool, error) {
	live, err := m.ensureLiveSession(ctx, handle)
	if err != nil {
		return "", false, err
	}
	conversationKey = strings.TrimSpace(conversationKey)
	if conversationKey == "" {
		return "", false, fmt.Errorf("conversation key is required")
	}
	live.mu.Lock()
	threadID := strings.TrimSpace(live.conversationSessions[conversationKey])
	live.mu.Unlock()
	if threadID == "" {
		return "", false, nil
	}
	resolved, err := m.EnsureSession(ctx, handle, conversationKey)
	if err != nil {
		return "", false, err
	}
	return resolved, true, nil
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

	return m.removeConversationBindings(live, conversationKey, "")
}

func (m *appServerManager) removeConversationBindings(live *liveSession, logicalKey, keep string) error {
	live.conversationPersistMu.Lock()
	defer live.conversationPersistMu.Unlock()
	type binding struct {
		key, sessionID    string
		loaded, publishes bool
	}
	live.mu.Lock()
	var removed []binding
	for key, sessionID := range live.conversationSessions {
		if key == keep || contract.LogicalRuntimeSessionKey(key) != logicalKey {
			continue
		}
		removed = append(removed, binding{key, sessionID, live.loadedConversations[key], live.filePublishingThreads[sessionID]})
		delete(live.conversationSessions, key)
		delete(live.loadedConversations, key)
		delete(live.filePublishingThreads, sessionID)
	}
	if len(removed) == 0 {
		live.mu.Unlock()
		return nil
	}
	conversations := cloneConversationSessions(live.conversationSessions)
	filePublishing := filePublishingConversationsForThreads(conversations, live.filePublishingThreads)
	live.mu.Unlock()
	if err := m.persistConversationSessions(live, conversations, filePublishing); err != nil {
		live.mu.Lock()
		for _, item := range removed {
			live.conversationSessions[item.key] = item.sessionID
			live.loadedConversations[item.key] = item.loaded
			live.filePublishingThreads[item.sessionID] = item.publishes
		}
		live.mu.Unlock()
		return err
	}
	for _, item := range removed {
		if m.deps.Permission != nil {
			m.deps.Permission.CancelSession(live.spec.RuntimeID, item.sessionID)
		}
		if m.deps.UserInput != nil {
			m.deps.UserInput.CancelSession(live.spec.RuntimeID, item.sessionID)
		}
	}
	return nil
}

func filePublishingConversationsForThreads(conversations map[string]string, filePublishingThreads map[string]bool) map[string]bool {
	filePublishing := make(map[string]bool)
	for conversationKey, threadID := range conversations {
		if filePublishingThreads[strings.TrimSpace(threadID)] {
			filePublishing[conversationKey] = true
		}
	}
	return cloneFilePublishingConversations(filePublishing)
}

func (m *appServerManager) persistConversationSessions(
	live *liveSession,
	conversations map[string]string,
	filePublishing map[string]bool,
) error {
	if m == nil || live == nil || live.session == nil || m.deps.OnConversationSessionsChange == nil {
		return nil
	}
	return m.deps.OnConversationSessionsChange(live.session, conversations, filePublishing)
}

func (m *appServerManager) persistSessionMetadata(session *Session, save func(*Session) error) error {
	live := m.liveSession(session.RuntimeID)
	if live == nil || !live.session.StartedAt.Equal(session.StartedAt) {
		return fmt.Errorf("Codex runtime %q changed before persisting session metadata", session.RuntimeID)
	}
	live.conversationPersistMu.Lock()
	defer live.conversationPersistMu.Unlock()
	if m.liveSession(session.RuntimeID) != live {
		return fmt.Errorf("Codex runtime %q changed before persisting session metadata", session.RuntimeID)
	}
	live.mu.Lock()
	session.ConversationSessions = cloneConversationSessions(live.conversationSessions)
	session.FilePublishingConversations = filePublishingConversationsForThreads(session.ConversationSessions, live.filePublishingThreads)
	live.mu.Unlock()
	return save(session)
}

func (m *appServerManager) ensureLiveSession(ctx context.Context, handle SessionHandle) (*liveSession, error) {
	runtimeID := strings.TrimSpace(handle.RuntimeID)
	if runtimeID == "" {
		return nil, fmt.Errorf("runtime id is required")
	}
	if live := m.liveSession(runtimeID); live != nil {
		return live, nil
	}
	// A room-scoped turn acquired a lease on the process during preparation.
	// A crashed process can only be recovered by the next preparation, with
	// the base Agent settings for the default thread.
	if ctx != nil {
		if _, scoped := ctx.Value(roomThreadSettingsKey{}).(roomThreadSettings); scoped {
			return nil, os.ErrNotExist
		}
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
	serverInfo, err := live.appClient.request(ctx, "initialize", map[string]any{
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
	live.serverInfo = append(json.RawMessage(nil), serverInfo...)
	live.appClient.notify("initialized")
	return nil
}

func (m *appServerManager) startOrResumeThread(ctx context.Context, live *liveSession, threadID string, publishFiles bool) (string, bool, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID != "" {
		resumed, err := m.resumeThread(ctx, live, threadID)
		if err == nil {
			if strings.TrimSpace(resumed) != "" {
				return resumed, publishFiles, nil
			}
			return threadID, publishFiles, nil
		}
		if live.appClient != nil {
			live.appClient.logDebug("codex app-server thread resume failed; starting a new thread", "thread_id", threadID, "error", err)
		}
	}
	started, err := m.startThread(ctx, live, publishFiles)
	return started, publishFiles && err == nil, err
}

func (m *appServerManager) startThread(ctx context.Context, live *liveSession, publishFiles bool) (string, error) {
	params := roomThreadParams(ctx, appServerThreadStartParams(roomThreadSpec(ctx, live.spec), publishFiles))
	raw, err := live.appClient.request(ctx, "thread/start", params)
	if err != nil {
		return "", err
	}
	return appServerThreadIDFromResult(raw)
}

func (m *appServerManager) resumeThread(ctx context.Context, live *liveSession, threadID string) (string, error) {
	params := roomThreadParams(ctx, appServerThreadResumeParams(roomThreadSpec(ctx, live.spec), threadID))
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

func appServerThreadStartParams(spec SessionSpec, publishFiles bool) map[string]any {
	spec.Profile = spec.Profile.Normalized()
	params := map[string]any{
		"cwd":                    spec.WorkspaceDir,
		"persistExtendedHistory": true,
		"experimentalRawEvents":  false,
		"developerInstructions":  appServerDeveloperInstructions,
	}
	if publishFiles {
		// Native tools are immutable on resume; enforce the current mode in the call handler.
		params["dynamicTools"] = []map[string]any{appServerPublishFileToolSpec(), appServerGenerateImageToolSpec(), appServerGenerateVideoToolSpec(), appServerUploadFileToolSpec()}
	}
	if spec.Profile.ModelID != "" {
		params["model"] = spec.Profile.ModelID
	}
	if config := appServerThreadConfig(spec); len(config) > 0 {
		params["config"] = config
	}
	if spec.ExecutionMode == ExecutionModeReadOnly {
		params["approvalPolicy"] = "never"
		params["permissions"] = readOnlyPermissionsProfile
	}
	return params
}

func appServerUploadFileToolSpec() map[string]any {
	return map[string]any{
		"name":        appServerUploadFileToolName,
		"description": "Upload an existing Runtime workspace file to a URI issued by one of this Agent's configured MCP servers. Call the MCP upload-creation tool first, then pass its server name and upload URI here. The destination must be same-origin with that configured MCP server. Do not use curl or expose upload credentials.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":        map[string]any{"type": "string", "description": "Runtime workspace-relative path of the input file."},
				"server":      map[string]any{"type": "string", "description": "Configured MCP server name that issued the upload URI."},
				"uploadUri":   map[string]any{"type": "string", "minLength": 1, "description": "Relative or absolute same-origin upload URI returned by the MCP server."},
				"contentType": map[string]any{"type": "string", "description": "Optional file content type."},
			},
			"required": []string{"path", "server", "uploadUri"}, "additionalProperties": false,
		},
	}
}

func appServerPublishFileToolSpec() map[string]any {
	return map[string]any{
		"name":        appServerPublishFileToolName,
		"description": "Publish a generated workspace file through the active channel. When the user asks to generate, send, return, or download a file, call this tool immediately after creating it. Do not search for or use csgclaw-cli, curl, Feishu APIs, or other upload methods. The path must be relative to the workspace; do not pass a URL or an absolute path.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Workspace-relative path of the completed file.",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Optional UTF-8 delivery filename with at most 255 bytes, no directory components, and no control characters.",
				},
				"mimeType": map[string]any{
					"type":        "string",
					"description": "Optional expected MIME type. The Runtime Adapter validates or derives it from the file.",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
	}
}

func appServerThreadResumeParams(spec SessionSpec, threadID string) map[string]any {
	spec.Profile = spec.Profile.Normalized()
	params := map[string]any{
		"threadId":              strings.TrimSpace(threadID),
		"cwd":                   spec.WorkspaceDir,
		"developerInstructions": appServerDeveloperInstructions,
	}
	if spec.Profile.ModelID != "" {
		params["model"] = spec.Profile.ModelID
	}
	if config := appServerThreadConfig(spec); len(config) > 0 {
		params["config"] = config
	}
	if spec.ExecutionMode == ExecutionModeReadOnly {
		params["approvalPolicy"] = "never"
		params["permissions"] = readOnlyPermissionsProfile
	}
	return params
}

func appServerTurnStartParams(spec SessionSpec, threadID string, prompt string) map[string]any {
	return appServerTurnStartParamsWithInput(spec, threadID, []map[string]any{{"type": "text", "text": prompt}}, "")
}

func appServerTurnStartParamsForContext(ctx context.Context, spec SessionSpec, threadID string, input []map[string]any, clientUserMessageID string) map[string]any {
	params := appServerTurnStartParamsWithInput(roomThreadSpec(ctx, spec), threadID, input, clientUserMessageID)
	if room, ok := ctx.Value(roomThreadSettingsKey{}).(roomThreadSettings); ok {
		params["cwd"] = room.WorkspaceDir
	}
	return params
}

func appServerTurnStartParamsWithInput(spec SessionSpec, threadID string, input []map[string]any, clientUserMessageID string) map[string]any {
	spec.Profile = spec.Profile.Normalized()
	params := map[string]any{
		"threadId": strings.TrimSpace(threadID),
		"input":    input,
	}
	if clientUserMessageID = strings.TrimSpace(clientUserMessageID); clientUserMessageID != "" {
		params["clientUserMessageId"] = clientUserMessageID
	}
	applyTurnModelParams(params, turnModelFromProfile(spec.Profile))
	if spec.ExecutionMode == ExecutionModeReadOnly {
		params["approvalPolicy"] = "never"
		params["permissions"] = readOnlyPermissionsProfile
	}
	return params
}

func appServerOverrides(spec SessionSpec) []string {
	if spec.ExecutionMode != ExecutionModeReadOnly {
		return nil
	}
	overrides := []string{
		"--strict-config",
		"--disable", "apps",
		"--disable", "browser_use",
		"--disable", "browser_use_external",
		"--disable", "browser_use_full_cdp_access",
		"--disable", "computer_use",
		"--disable", "goals",
		"--disable", "hooks",
		"--disable", "image_generation",
		"--disable", "in_app_browser",
		"--disable", "memories",
		"--disable", "multi_agent",
		"--disable", "remote_plugin",
		"--disable", "shell_tool",
		"--disable", "skill_mcp_dependency_install",
		"--disable", "unified_exec",
		"--disable", "workspace_dependencies",
		"--config", `approval_policy="never"`,
		"--config", `default_permissions="csgclaw-read-only"`,
		"--config", `web_search="disabled"`,
		"--config", `project_doc_max_bytes=0`,
		"--config", `shell_environment_policy.inherit="none"`,
	}
	if workspaceDir := strings.TrimSpace(spec.WorkspaceDir); workspaceDir != "" {
		overrides = append(overrides,
			"--config", fmt.Sprintf(`projects={%s={trust_level="untrusted"}}`, tomlQuotedKey(workspaceDir)),
		)
	}
	return overrides
}

func (m *appServerManager) handleAppServerServerRequest(runtimeID string, live *liveSession, req appServerServerRequest) (result any, resultErr error) {
	var diagnosticParams struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
	}
	if json.Unmarshal(req.Params, &diagnosticParams) == nil && live != nil {
		if ctx, ok := live.appServerTurnContext(diagnosticParams.ThreadID, diagnosticParams.TurnID); ok {
			diagnostics.From(ctx).Capture("runtime.callback_request", map[string]any{"method": req.Method, "params": json.RawMessage(req.Params)})
			defer func() {
				diagnostics.From(ctx).Capture("runtime.callback_response", map[string]any{"method": req.Method, "result": result, "failed": resultErr != nil})
			}()

			name, owner := "callback.handle", "csgclaw"
			if strings.Contains(req.Method, "requestUserInput") {
				name, owner = "user.wait", "user"
			}
			finish := diagnostics.Measure(ctx, name, owner)
			defer finish()
		}
	}
	readOnly := live != nil && live.spec.ExecutionMode == ExecutionModeReadOnly
	switch strings.TrimSpace(req.Method) {
	case "item/commandExecution/requestApproval", "execCommandApproval":
		if readOnly {
			return map[string]any{"decision": "decline"}, nil
		}
		return map[string]any{"decision": "accept"}, nil
	case "item/fileChange/requestApproval", "applyPatchApproval":
		if readOnly {
			return map[string]any{"decision": "decline"}, nil
		}
		return map[string]any{"decision": "accept"}, nil
	case "item/permissions/requestApproval":
		if readOnly {
			return map[string]any{"permissions": map[string]any{}, "scope": "turn"}, nil
		}
		return nil, fmt.Errorf("permission escalation is not supported")
	case "mcpServer/elicitation/request":
		if readOnly {
			return map[string]any{"action": "decline", "content": nil, "_meta": nil}, nil
		}
		return map[string]any{
			"action":  "accept",
			"content": nil,
			"_meta":   nil,
		}, nil
	case "item/tool/requestUserInput":
		return m.handleAppServerUserInputRequest(runtimeID, live, req)
	case "item/tool/call":
		return m.handleAppServerDynamicToolCall(runtimeID, live, req)
	default:
		return nil, fmt.Errorf("unhandled server request: %s", strings.TrimSpace(req.Method))
	}
}

type appServerDynamicToolCallParams struct {
	ThreadID  string          `json:"threadId"`
	TurnID    string          `json:"turnId"`
	CallID    string          `json:"callId"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
}

type appServerUploadFileArgs struct {
	Path        string `json:"path"`
	Server      string `json:"server"`
	UploadURI   string `json:"uploadUri"`
	ContentType string `json:"contentType"`
}

type appServerPublishFileArgs struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	MIMEType string `json:"mimeType"`
}

func (m *appServerManager) handleAppServerDynamicToolCall(runtimeID string, live *liveSession, req appServerServerRequest) (any, error) {
	var params appServerDynamicToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, fmt.Errorf("decode dynamic tool call: %w", err)
	}
	params.ThreadID = strings.TrimSpace(params.ThreadID)
	params.TurnID = strings.TrimSpace(params.TurnID)
	if waiter := live.appServerTurnWaiter(params.ThreadID); waiter != nil && waiter.matchesTurn(params.TurnID) {
		waiter.mu.Lock()
		waiter.visibleActivity = true
		waiter.mu.Unlock()
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.Tool = strings.TrimSpace(params.Tool)
	if params.ThreadID == "" || params.TurnID == "" || params.CallID == "" {
		return nil, fmt.Errorf("dynamic tool thread ID, turn ID, and call ID are required")
	}
	if params.Tool != appServerPublishFileToolName && params.Tool != appServerUploadFileToolName && params.Tool != "csgclaw_generate_image" && params.Tool != "csgclaw_generate_video" {
		return nil, fmt.Errorf("unsupported dynamic tool %q", params.Tool)
	}
	if live == nil || !live.appServerPublishesFilesForThread(params.ThreadID) {
		return nil, fmt.Errorf("dynamic file tool references unknown or unsupported thread %q", params.ThreadID)
	}
	if params.Tool == "csgclaw_generate_image" {
		if live.spec.ExecutionMode == ExecutionModeReadOnly {
			return appServerDynamicToolResponse(false, "image generation is unavailable in read-only mode"), nil
		}
		var args struct {
			Prompt string `json:"prompt"`
		}
		if json.Unmarshal(params.Arguments, &args) != nil {
			return appServerDynamicToolResponse(false, "invalid image generation arguments"), nil
		}
		ctx, ok := live.appServerTurnContext(params.ThreadID, params.TurnID)
		if !ok {
			return appServerDynamicToolResponse(false, "image generation requires an active turn"), nil
		}
		if err := contract.GenerateImage(ctx, params.CallID, args.Prompt); err != nil {
			return appServerDynamicToolResponse(false, err.Error()), nil
		}
		return appServerDynamicToolResponse(true, "Image generated and delivered to the current conversation. Do not generate again or publish another copy. Continue responding using the original chat model."), nil
	}
	if params.Tool == "csgclaw_generate_video" {
		if live.spec.ExecutionMode == ExecutionModeReadOnly {
			return appServerDynamicToolResponse(false, "video generation is unavailable in read-only mode"), nil
		}
		var args struct {
			Prompt  string `json:"prompt"`
			Size    string `json:"size"`
			Seconds int    `json:"seconds"`
		}
		if json.Unmarshal(params.Arguments, &args) != nil {
			return appServerDynamicToolResponse(false, "invalid video generation arguments"), nil
		}
		ctx, ok := live.appServerTurnContext(params.ThreadID, params.TurnID)
		if !ok {
			return appServerDynamicToolResponse(false, "video generation requires an active turn"), nil
		}
		if err := contract.GenerateVideo(ctx, params.CallID, args.Prompt, modelprovider.VideoGenerationOptions{Size: args.Size, Seconds: args.Seconds}); err != nil {
			return appServerDynamicToolResponse(false, err.Error()), nil
		}
		return appServerDynamicToolResponse(true, "Video generation was submitted. Briefly acknowledge submission without describing a transient status such as still generating, downloading, completed, or delivered. The attached task card is the source of truth and updates automatically. Do not publish another copy."), nil
	}
	if params.Tool == appServerUploadFileToolName {
		if live.spec.ExecutionMode == ExecutionModeReadOnly {
			return appServerDynamicToolResponse(false, "file upload is unavailable in read-only mode"), nil
		}
		var args appServerUploadFileArgs
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return appServerDynamicToolResponse(false, "invalid upload arguments"), nil
		}
		uploadCtx, ok := live.appServerTurnContext(params.ThreadID, params.TurnID)
		if !ok {
			return appServerDynamicToolResponse(false, "file upload is unavailable outside an active turn"), nil
		}
		if err := uploadAppServerWorkspaceFile(uploadCtx, roomThreadSpec(uploadCtx, live.spec), args); err != nil {
			return appServerDynamicToolResponse(false, err.Error()), nil
		}
		return appServerDynamicToolResponse(true, "File uploaded successfully. Continue by calling the MCP parse tool with its file ID."), nil
	}
	var publishArgs appServerPublishFileArgs
	if err := json.Unmarshal(params.Arguments, &publishArgs); err != nil {
		return appServerDynamicToolResponse(false, "invalid publish arguments"), nil
	}
	file, err := normalizeAppServerPublishFileArgs(publishArgs)
	if err != nil {
		return appServerDynamicToolResponse(false, err.Error()), nil
	}
	if m == nil || m.deps.EventSink == nil {
		return appServerDynamicToolResponse(false, "file delivery is unavailable"), nil
	}
	ctx, ok := live.appServerTurnContext(params.ThreadID, params.TurnID)
	if !ok {
		return appServerDynamicToolResponse(false, "file publication requires an active turn"), nil
	}
	publication := &filePublication{file: file, ctx: ctx, done: make(chan error, 1)}
	m.publishAppServerEvent(SessionEvent{
		RuntimeID:  runtimeID,
		SessionID:  params.ThreadID,
		TurnID:     params.TurnID,
		Kind:       SessionEventFileOutput,
		ToolCallID: params.CallID,
		ToolKind:   appServerPublishFileToolName,
		ToolTitle:  "Publish file",
		ToolStatus: "completed",
		Payload:    publication,
	})
	select {
	case err := <-publication.done:
		if ctx.Err() != nil {
			return appServerDynamicToolResponse(false, "File publication canceled."), nil
		}
		if err != nil {
			return appServerDynamicToolResponse(false, fmt.Sprintf("File publication failed for %q: %s. Check that the file exists and is a readable regular file inside the Runtime workspace, then retry with its workspace-relative path.", file.Path, err)), nil
		}
		return appServerDynamicToolResponse(true, "File snapshotted for delivery to the current conversation."), nil
	case <-ctx.Done():
		return appServerDynamicToolResponse(false, "File publication canceled."), nil
	}
}

func uploadAppServerWorkspaceFile(ctx context.Context, spec SessionSpec, args appServerUploadFileArgs) error {
	path := strings.TrimSpace(args.Path)
	cleaned := filepath.Clean(path)
	if path == "" || cleaned == "." || filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path must stay within the Runtime workspace")
	}
	entry, ok := spec.MCPServers[strings.TrimSpace(args.Server)].(map[string]any)
	if !ok {
		return fmt.Errorf("MCP server %q is not configured for this Agent", strings.TrimSpace(args.Server))
	}
	baseText, _ := entry["url"].(string)
	base, err := url.Parse(strings.TrimSpace(baseText))
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return fmt.Errorf("configured MCP server does not have a valid HTTP URL")
	}
	uploadURI := strings.TrimSpace(args.UploadURI)
	if uploadURI == "" {
		return fmt.Errorf("upload URI is required")
	}
	target, err := base.Parse(uploadURI)
	if err != nil || target.Scheme != base.Scheme || !strings.EqualFold(target.Host, base.Host) || target.User != nil {
		return fmt.Errorf("upload URI must be same-origin with MCP server %q", strings.TrimSpace(args.Server))
	}
	return withAppServerUploadFile(spec, cleaned, func(file *os.File, size int64) error {
		request, err := newAppServerUploadRequest(ctx, target.String(), file, size)
		if err != nil {
			return fmt.Errorf("create upload request: %w", err)
		}
		if contentType := strings.TrimSpace(args.ContentType); contentType != "" {
			if _, _, err := mime.ParseMediaType(contentType); err != nil {
				return fmt.Errorf("contentType is invalid")
			}
			request.Header.Set("Content-Type", contentType)
		}
		client := &http.Client{
			Timeout: 90 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("upload redirects are not allowed")
			},
		}
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("upload file: %w", err)
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("upload endpoint returned HTTP %d", response.StatusCode)
		}
		return nil
	})
}

func newAppServerUploadRequest(ctx context.Context, target string, file *os.File, size int64) (*http.Request, error) {
	var body io.Reader = http.NoBody
	if size > 0 {
		body = io.LimitReader(file, size)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, target, body)
	if err != nil {
		return nil, err
	}
	request.ContentLength = size
	return request, nil
}

func normalizeAppServerPublishFileArgs(args appServerPublishFileArgs) (activity.RuntimeFile, error) {
	path := strings.TrimSpace(args.Path)
	cleaned := filepath.Clean(path)
	if path == "" || cleaned == "." || filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return activity.RuntimeFile{}, fmt.Errorf("path must stay within the Runtime workspace")
	}
	name := strings.TrimSpace(args.Name)
	if name == "" {
		name = filepath.Base(cleaned)
	}
	if !utf8.ValidString(name) || len([]byte(name)) > appServerMaxPublishedFileNameBytes || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") {
		return activity.RuntimeFile{}, fmt.Errorf("name must be a valid UTF-8 basename with 1 to %d bytes", appServerMaxPublishedFileNameBytes)
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return activity.RuntimeFile{}, fmt.Errorf("name must not contain control characters")
		}
	}
	mediaType := strings.TrimSpace(args.MIMEType)
	if mediaType != "" {
		parsed, _, err := mime.ParseMediaType(mediaType)
		if err != nil || strings.TrimSpace(parsed) == "" {
			return activity.RuntimeFile{}, fmt.Errorf("mimeType is invalid")
		}
		mediaType = strings.ToLower(parsed)
	}
	return activity.RuntimeFile{Path: cleaned, Name: name, MIMEType: mediaType}, nil
}

func appServerDynamicToolResponse(success bool, message string) map[string]any {
	return map[string]any{
		"contentItems": []map[string]any{{"type": "inputText", "text": strings.TrimSpace(message)}},
		"success":      success,
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
		activity: "request_user_input:pending",
		progress: true,
	})
	defer live.notifyAppServerTurn(params.ThreadID, appServerTurnResult{
		activity: "request_user_input:resolved",
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
	effort = config.NormalizeReasoningEffort(effort)
	if config.UsesModelReasoningDefault(effort) {
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

func appServerPromptInput(req PromptRequest) ([]map[string]any, error) {
	if len(req.Prompt) == 0 {
		return nil, fmt.Errorf("prompt input is required")
	}
	input := make([]map[string]any, 0, len(req.Prompt))
	appendText := func(text string) {
		// Codex concatenates separate text inputs without adding a separator.
		// Keep logical prompt blocks distinct, as appServerPromptText does,
		// while retaining the position of native image inputs.
		if len(input) > 0 && input[len(input)-1]["type"] == "text" {
			previous := input[len(input)-1]
			previous["text"] = previous["text"].(string) + "\n\n" + text
			return
		}
		input = append(input, map[string]any{"type": "text", "text": text})
	}
	for _, block := range req.Prompt {
		switch {
		case block.Text != nil:
			if strings.TrimSpace(block.Text.Text) == "" {
				return nil, fmt.Errorf("prompt text is required")
			}
			appendText(block.Text.Text)
		case block.LocalImage != nil:
			path := strings.TrimSpace(block.LocalImage.Path)
			if path == "" {
				return nil, fmt.Errorf("local image path is required")
			}
			input = append(input, map[string]any{"type": "localImage", "path": path})
		case block.ResourceLink != nil || block.Resource != nil:
			text := strings.TrimSpace(textFromPromptBlock(block))
			if text == "" {
				return nil, fmt.Errorf("prompt resource is empty")
			}
			appendText(text)
		default:
			return nil, fmt.Errorf("unsupported prompt content block for codex app-server")
		}
	}
	return input, nil
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
	model         contract.TurnModel
	modelOverride bool
	// Guarded by liveSession.mu, alongside the thread's cumulative counters.
	previousContextUsage     *modelcap.ContextUsage
	compacting               bool
	itemTurns                map[string]string // Native item IDs received with an explicit turn ID.
	missingItems             bool              // An item without a known owner was withheld.
	tokenUsage               modelusage.Usage
	pendingUsage             map[string]modelusage.Usage
	diagnostic               *diagnostics.Record
	diagnosticEnds           map[string]time.Time
	diagnosticNotes          map[string][]json.RawMessage
	diagnosticNoteBytes      int
	visibleActivity          bool
	mu                       sync.RWMutex
	accepted                 bool
	threadID                 string
	turnID                   string
	ch                       chan appServerTurnResult
	terminalResults          map[string]appServerTurnResult
	lastActivity             string
	progressSequence         uint64
	streamReasoningSummary   bool
	reasoningSummaries       map[string]*appServerReasoningSummary
	structuredOutputBoundary bool
	textOnlyUserInput        bool
}

type appServerTurnResult struct {
	storedTurn map[string]any
	success    bool
	stopReason string
	err        error
	turnID     string
	activity   string
	started    bool
	progress   bool
}

func (s *liveSession) registerAppServerTurnWaiter(threadID string) (*appServerTurnWaiter, error) {
	return s.registerAppServerTurnWaiterWithModel(threadID, turnModelFromProfile(s.spec.Profile), false)
}

func (s *liveSession) registerAppServerTurnWaiterWithModel(threadID string, selected contract.TurnModel, overridden bool) (*appServerTurnWaiter, error) {
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
	delete(s.legacyTools, threadID)
	waiter := &appServerTurnWaiter{
		model:         selected,
		modelOverride: overridden,
		threadID:      threadID,
		ch:            make(chan appServerTurnResult, 8),
		lastActivity:  "turn/start",
	}
	waiter.tokenUsage.ModelID = selected.ModelID
	if usage, ok := s.contextUsage[threadID]; ok {
		waiter.previousContextUsage = modelcap.CloneUsage(&usage)
	}
	s.turnWaiters[threadID] = waiter
	return waiter, nil
}

func (s *liveSession) removeAppServerTurnWaiter(threadID string, waiter *appServerTurnWaiter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	threadID = strings.TrimSpace(threadID)
	if s.turnWaiters[threadID] == waiter {
		waiter.mu.RLock()
		accepted := waiter.accepted
		waiter.mu.RUnlock()
		if accepted {
			if s.threadModels == nil {
				s.threadModels = make(map[string]contract.TurnModel)
			}
			s.threadModels[threadID] = waiter.model
		} else if waiter.previousContextUsage != nil {
			// Revert a pending model selection while retaining same-model usage
			// updates, including a compaction completed during context recovery.
			if usage, ok := s.contextUsage[threadID]; !ok || usage.ModelID != waiter.previousContextUsage.ModelID {
				s.contextUsage[threadID] = *waiter.previousContextUsage
			}
		} else {
			delete(s.contextUsage, threadID)
		}
		if terminal, ok := waiter.terminalResult(); ok && terminal.turnID != "" && terminal.turnID == waiter.currentTurnID() && !s.turnThreads[terminal.turnID].recovered {
			// An identified successor has finished. Later legacy items can be
			// delivered again, while the recovered turn and its IDs stay excluded.
			for id, old := range s.turnThreads {
				if old.threadID == threadID && old.recovered && !old.successorFinished {
					old.successorFinished = true
					s.turnThreads[id] = old
				}
			}
		}
		delete(s.turnWaiters, threadID)
	}
}

type appServerTurnContext struct {
	waiter *appServerTurnWaiter
	ctx    context.Context
}

func (s *liveSession) setAppServerTurnContext(threadID string, waiter *appServerTurnWaiter, ctx context.Context) {
	threadID = strings.TrimSpace(threadID)
	if s == nil || threadID == "" || waiter == nil || ctx == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.turnContexts == nil {
		s.turnContexts = make(map[string]appServerTurnContext)
	}
	s.turnContexts[threadID] = appServerTurnContext{waiter: waiter, ctx: ctx}
}

func (s *liveSession) clearAppServerTurnContext(threadID string, waiter *appServerTurnWaiter) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	threadID = strings.TrimSpace(threadID)
	active, ok := s.turnContexts[threadID]
	if ok && active.waiter == waiter {
		delete(s.turnContexts, threadID)
	}
}

func (s *liveSession) appServerTurnContext(threadID, turnID string) (context.Context, bool) {
	if s == nil || strings.TrimSpace(threadID) == "" || strings.TrimSpace(turnID) == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	active, ok := s.turnContexts[strings.TrimSpace(threadID)]
	if !ok || active.waiter == nil || active.ctx == nil || active.waiter.currentTurnID() != strings.TrimSpace(turnID) {
		return nil, false
	}
	if _, terminal := active.waiter.terminalResult(); terminal || active.ctx.Err() != nil {
		return nil, false
	}
	return active.ctx, true
}

type appServerTrackedTurn struct {
	threadID          string
	recovered         bool
	successorFinished bool
	recoveredItems    map[string]struct{}
}

func (s *liveSession) trackAppServerTurn(threadID, turnID string) {
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	if s == nil || threadID == "" || turnID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trackAppServerTurnLocked(threadID, turnID, false)
}

func (s *liveSession) trackAppServerTurnLocked(threadID, turnID string, recovered bool) {
	if s.turnThreads == nil {
		s.turnThreads = make(map[string]appServerTrackedTurn)
	}
	tracked, exists := s.turnThreads[turnID]
	if !exists {
		s.turnThreadOrder = append(s.turnThreadOrder, turnID)
	}
	tracked.threadID = threadID
	tracked.recovered = recovered || tracked.recovered
	s.turnThreads[turnID] = tracked
	for len(s.turnThreadOrder) > appServerTrackedTurnLimit {
		oldest := s.turnThreadOrder[0]
		s.turnThreadOrder = s.turnThreadOrder[1:]
		delete(s.turnThreads, oldest)
	}
}

// Resolve older notifications that omit the thread ID only when the native
// turn or an unambiguous session identifies the recipient.
func (s *liveSession) nativeNotificationThread(threadID, turnID string, allowPrimaryFallback bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nativeNotificationThreadLocked(threadID, turnID, allowPrimaryFallback)
}

func (s *liveSession) nativeNotificationThreadLocked(threadID, turnID string, allowPrimaryFallback bool) string {
	if owner := s.turnThreads[turnID].threadID; owner != "" {
		if threadID != "" && threadID != owner {
			return ""
		}
		return owner
	}
	if threadID != "" {
		return threadID
	}
	for id, waiter := range s.turnWaiters {
		if len(s.turnWaiters) == 1 || (turnID != "" && turnID == waiter.currentTurnID()) {
			return id
		}
	}
	if s.session == nil {
		return ""
	}
	id := strings.TrimSpace(s.session.SessionID)
	if allowPrimaryFallback {
		// Older rollouts can omit both IDs. Preserve their primary-thread
		// routing while there is no recovered turn with ambiguous ownership.
		for _, tracked := range s.turnThreads {
			if tracked.recovered {
				return ""
			}
		}
		return id
	}
	if len(s.turnWaiters) == 0 {
		for _, other := range s.conversationSessions {
			if strings.TrimSpace(other) != id {
				return ""
			}
		}
		return id
	}
	return ""
}

// A turn without an ID is ambiguous after history recovery: an earlier
// notification may still be in flight on this thread. Its current waiter can
// still finish from an identified notification or the native history check.
func (s *liveSession) acceptsTurnNotificationLocked(threadID, turnID string) bool {
	if turnID == "" {
		for _, tracked := range s.turnThreads {
			if tracked.threadID == threadID && tracked.recovered {
				return false
			}
		}
		return true
	}
	tracked := s.turnThreads[turnID]
	if tracked.recovered || (tracked.threadID != "" && tracked.threadID != threadID) {
		return false
	}
	waiter := s.turnWaiters[threadID]
	return waiter == nil || waiter.matchesTurn(turnID)
}

// Use an explicit item/turn association before falling back to older protocols'
// thread routing. Items from recovered history remain excluded by their ID.
func (s *liveSession) beginTurnNotification(threadID, turnID, itemID string, isItem bool) (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Only each candidate turn's own history can identify its withheld items.
	missingOwner := turnID == "" && isItem
	unowned := func() (string, string) {
		if missingOwner {
			for id, waiter := range s.turnWaiters {
				if threadID == "" || threadID == id {
					waiter.missingItems = true
				}
			}
		}
		return "", ""
	}
	if turnID == "" {
		var itemThread string
		if itemID != "" {
			for id, waiter := range s.turnWaiters {
				if threadID != "" && threadID != id {
					continue
				}
				if owner := waiter.itemTurns[itemID]; owner != "" {
					if itemThread != "" {
						return unowned() // Two active threads use the same item ID.
					}
					itemThread, turnID = id, owner
				}
			}
		}
		for _, old := range s.turnThreads {
			if !old.recovered || (threadID != "" && old.threadID != threadID) {
				continue
			}
			if _, stale := old.recoveredItems[itemID]; stale {
				return unowned()
			}
			// A sole active waiter on a different thread cannot establish
			// ownership of an unscoped record from a recovered thread.
			if threadID == "" && itemThread == "" && !old.successorFinished && s.turnWaiters[old.threadID] == nil {
				return unowned()
			}
		}
		if itemThread != "" {
			threadID = itemThread
		}
	}
	resolved := s.nativeNotificationThreadLocked(threadID, turnID, true)
	if resolved == "" {
		return unowned()
	}
	threadID = resolved
	if turnID == "" && itemID != "" && s.turnWaiters[threadID] != nil {
		for _, old := range s.turnThreads {
			if old.threadID == threadID && old.recovered && !old.successorFinished {
				return unowned()
			}
		}
	} else if !s.acceptsTurnNotificationLocked(threadID, turnID) {
		return unowned()
	}
	if waiter := s.turnWaiters[threadID]; waiter != nil {
		if itemID != "" && turnID != "" {
			if waiter.itemTurns == nil {
				waiter.itemTurns = make(map[string]string)
			}
			waiter.itemTurns[itemID] = turnID
		}
		// Invalidate a pending lookup before publishing this notification.
		waiter.mu.Lock()
		waiter.progressSequence++
		waiter.mu.Unlock()
	}
	return threadID, turnID
}

// Claim the terminal event before waking a waiter or publishing to subscribers.
// A notification can arrive after a native history check has settled this turn.
func (s *liveSession) recordNativeCompletion(threadID string, result *appServerTurnResult, quietVersion *uint64, turn map[string]any) (*appServerTurnWaiter, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.acceptsTurnNotificationLocked(threadID, result.turnID) {
		return nil, false
	}
	waiter := s.turnWaiters[threadID]
	if result.turnID == "" && waiter != nil {
		result.turnID = waiter.currentTurnID()
	}
	if quietVersion != nil || (waiter != nil && waiter.missingItems && appServerString(turn, "id") == result.turnID && appServerString(turn, "itemsView") == "full") {
		result.storedTurn = turn
	}
	if quietVersion != nil {
		if waiter == nil || !waiter.recordQuietResult(result, *quietVersion) {
			return nil, false
		}
	} else if waiter != nil && !waiter.apply(result) {
		return nil, false
	}
	if result.turnID != "" {
		s.trackAppServerTurnLocked(threadID, result.turnID, quietVersion != nil)
		if quietVersion != nil {
			old := s.turnThreads[result.turnID]
			old.recoveredItems = make(map[string]struct{})
			for id, turn := range waiter.itemTurns {
				if turn == result.turnID {
					old.recoveredItems[id] = struct{}{}
				}
			}
			items, _ := turn["items"].([]any)
			for _, value := range items {
				item, _ := value.(map[string]any)
				if id := appServerString(item, "id"); id != "" {
					old.recoveredItems[id] = struct{}{}
				}
			}
			s.turnThreads[result.turnID] = old
		}
	}
	return waiter, true
}

func (s *liveSession) notifyAppServerTurn(threadID string, result appServerTurnResult) bool {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return false
	}
	s.mu.Lock()
	waiter := s.turnWaiters[threadID]
	// A legacy final-answer item can omit its turn ID. Deliver its text,
	// but wait for an identified terminal when an older turn was recovered.
	ambiguousEnd := (result.success || result.err != nil) && result.turnID == "" && !s.acceptsTurnNotificationLocked(threadID, "")
	s.mu.Unlock()
	if ambiguousEnd {
		return true
	}
	if waiter == nil {
		return false
	}
	if !waiter.apply(&result) {
		return true
	}

	waiter.wake(result)
	return true
}

func (w *appServerTurnWaiter) wake(result appServerTurnResult) {
	select {
	case w.ch <- result:
	default:
	}
}

func (w *appServerTurnWaiter) reasoningStream(turnID string) (active, summary bool) {
	if w == nil {
		return false, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if !w.accepted || (turnID != "" && w.turnID != turnID) {
		return false, false
	}
	if _, ended := w.terminalResults[w.turnID]; ended {
		return false, false
	}
	if _, ended := w.terminalResults[""]; ended {
		return false, false
	}
	return true, w.streamReasoningSummary
}

func (s *liveSession) appServerTurnWaiter(threadID string) *appServerTurnWaiter {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.turnWaiters[strings.TrimSpace(threadID)]
}

func (w *appServerTurnWaiter) recordDiagnosticCompletion(turnID string, at time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if turnID == "" || (w.turnID != "" && w.turnID != turnID) {
		return
	}
	if w.diagnosticEnds == nil {
		w.diagnosticEnds = map[string]time.Time{}
	}
	if _, ok := w.diagnosticEnds[turnID]; !ok && (len(w.diagnosticEnds) < 8 || w.turnID == turnID) {
		w.diagnosticEnds[turnID] = at
	}
	if w.turnID == turnID {
		w.diagnostic.RuntimeEndAt(w.diagnosticEnds[turnID])
	}
}

func (w *appServerTurnWaiter) setTurnID(turnID string) {
	turnID = strings.TrimSpace(turnID)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.accepted = true
	if turnID == "" {
		return
	}
	w.turnID = turnID
	w.flushDiagnosticNotesLocked(turnID)
	w.diagnostic.RuntimeRef(w.threadID, turnID, "")
	if at, ok := w.diagnosticEnds[turnID]; ok {
		w.diagnostic.RuntimeEndAt(at)
	}
	for id := range w.terminalResults {
		if id != "" && id != turnID {
			delete(w.terminalResults, id)
		}
	}
}

func (w *appServerTurnWaiter) apply(result *appServerTurnResult) bool {
	if w == nil || result == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.applyLocked(result)
}

func (w *appServerTurnWaiter) applyLocked(result *appServerTurnResult) bool {
	result.turnID = strings.TrimSpace(result.turnID)
	if w.turnID != "" && result.turnID != "" && w.turnID != result.turnID {
		return false
	}
	// Only a start notification or the turn/start response establishes identity.
	// A previous turn can finish while the next start response is still pending.
	if result.started {
		w.accepted = true
	}
	if result.started && result.turnID != "" {
		w.turnID = result.turnID
		w.flushDiagnosticNotesLocked(result.turnID)
		w.diagnostic.RuntimeRef(w.threadID, result.turnID, "")
		if at, ok := w.diagnosticEnds[w.turnID]; ok {
			w.diagnostic.RuntimeEndAt(at)
		}
	}
	if result.success || result.err != nil {
		// Progress is coalescible, but a terminal outcome must survive a full
		// notification channel. Retain outcomes by ID until turn/start binds the
		// waiter, including completions that precede its RPC response.
		if w.terminalResults == nil {
			w.terminalResults = make(map[string]appServerTurnResult)
		}
		if _, exists := w.terminalResults[result.turnID]; !exists {
			w.terminalResults[result.turnID] = *result
		}
	}
	if result.progress || result.started {
		w.progressSequence++
	}
	if result.activity != "" {
		w.lastActivity = strings.TrimSpace(result.activity)
	}

	return true
}

func (w *appServerTurnWaiter) terminalResult() (appServerTurnResult, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if result, ok := w.terminalResults[w.turnID]; ok {
		return result, true
	}
	result, ok := w.terminalResults[""]
	return result, ok
}

func (w *appServerTurnWaiter) matchesTurn(turnID string) bool {
	expected := w.currentTurnID()
	return expected == "" || turnID == "" || expected == strings.TrimSpace(turnID)
}

func (w *appServerTurnWaiter) currentTurnID() string {
	if w == nil {
		return ""
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return strings.TrimSpace(w.turnID)
}

func (w *appServerTurnWaiter) recordQuietResult(result *appServerTurnResult, version uint64) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.progressSequence != version || w.turnID != result.turnID {
		return false
	}
	if _, ok := w.terminalResults[w.turnID]; ok {
		return false
	}
	if _, ok := w.terminalResults[""]; ok {
		return false
	}
	return w.applyLocked(result)
}

func (w *appServerTurnWaiter) progressVersion() uint64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.progressSequence
}

func (w *appServerTurnWaiter) currentLastActivity() string {
	if w == nil {
		return ""
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return strings.TrimSpace(w.lastActivity)
}

func (w *appServerTurnWaiter) markStructuredOutputBoundary() bool {
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.textOnlyUserInput || w.structuredOutputBoundary {
		return false
	}
	w.structuredOutputBoundary = true
	return true
}

func (w *appServerTurnWaiter) hasStructuredOutputBoundary() bool {
	if w == nil {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.structuredOutputBoundary
}

func (s *liveSession) completedTurnItems(ctx context.Context, waiter *appServerTurnWaiter, result appServerTurnResult) quietTurnHistory {
	if result.storedTurn != nil {
		return quietTurnItems(result.storedTurn)
	}
	s.mu.Lock()
	missing := waiter.missingItems
	s.mu.Unlock()
	if !missing || result.turnID == "" || s.appClient == nil {
		return nil
	}
	// The identified terminal has already settled the prompt. Only its items
	// are missing, and an unavailable history must not keep that prompt open.
	ctx, cancel := context.WithTimeout(ctx, nativeTurnLookupTimeout)
	defer cancel()
	return quietTurnItems(readLatestTurn(ctx, s.appClient, waiter.threadID, result.turnID))
}

func (m *appServerManager) waitAppServerTurn(ctx context.Context, live *liveSession, waiter *appServerTurnWaiter) (PromptResponse, error) {
	waitStartedAt := time.Now()
	lastActivityAt := waitStartedAt
	quiet := time.NewTimer(quietTurnInterval)
	defer quiet.Stop()
	readCtx, cancelRead := context.WithCancel(ctx)
	defer cancelRead()
	var history <-chan map[string]any
	var readVersion uint64

	finish := func(result appServerTurnResult) (PromptResponse, error) {
		cancelRead()
		recovered := live.completedTurnItems(ctx, waiter, result)
		if result.err != nil {
			return PromptResponse{}, recovered.failure(result.err)
		}
		stopReason := result.stopReason
		if stopReason == "" {
			stopReason = StopReasonEndTurn
		}
		return PromptResponse{StopReason: stopReason, history: recovered}, nil
	}
	for {
		if result, ok := waiter.terminalResult(); ok {
			return finish(result)
		}
		select {
		case result := <-waiter.ch:
			if !waiter.matchesTurn(result.turnID) {
				continue
			}
			if result.progress || result.started {
				quiet.Reset(quietTurnInterval)
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
			}
			if result.success || result.err != nil {
				return finish(result)
			}
		case <-quiet.C:
			quiet.Reset(quietTurnInterval)
			if turnID := waiter.currentTurnID(); history == nil && turnID != "" && live.appClient != nil {
				readVersion = waiter.progressVersion()
				history = readQuietTurn(readCtx, live.appClient, waiter.threadID, turnID)
			}
		case turn := <-history:
			history = nil
			if turn == nil || ctx.Err() != nil {
				continue
			}
			if m.handleRawTurnCompleted(live.spec.RuntimeID, live, waiter.threadID, map[string]any{"turn": turn}, &readVersion) {
				live.appClient.logDebugContext(ctx, "recovered Codex turn from native history", "runtime_id", live.spec.RuntimeID, "thread_id", waiter.threadID, "turn_id", waiter.currentTurnID(), "status", turn["status"])
			}
		case <-ctx.Done():
			return m.stopCanceledAppServerTurn(ctx, live, waiter, "prompt context canceled")
		case <-live.done:
			// A terminal notification can arrive just before process exit.
			if _, completed := waiter.terminalResult(); completed {
				continue
			}
			if err := ctx.Err(); err != nil {
				return PromptResponse{StopReason: StopReasonInterrupted}, err
			}
			return PromptResponse{}, fmt.Errorf("codex app-server exited during turn: runtime_id=%s thread_id=%s", live.spec.RuntimeID, waiter.threadID)
		}
	}
}

func (m *appServerManager) stopCanceledAppServerTurn(
	ctx context.Context,
	live *liveSession,
	waiter *appServerTurnWaiter,
	reason string,
) (PromptResponse, error) {
	if err := m.stopAppServerTurn(live, waiter, reason); err != nil {
		return PromptResponse{}, errors.Join(ctx.Err(), fmt.Errorf("cancel Codex turn unconfirmed; worker preserved: %w", err))
	}
	return PromptResponse{StopReason: StopReasonInterrupted}, ctx.Err()
}

func (m *appServerManager) stopAppServerTurn(live *liveSession, waiter *appServerTurnWaiter, reason string) error {
	if m.deps.UserInput != nil {
		m.deps.UserInput.CancelSession(live.spec.RuntimeID, waiter.threadID)
	}
	interruptErr := m.interruptAppServerTurn(live, waiter)
	stderrTail := m.stderrTail(live.spec, 2048)
	if live.appClient != nil {
		live.appClient.logError("codex app-server turn cancellation attempted",
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
		return nil
	}
	// A rejected or delayed interrupt does not establish that the process is
	// unhealthy. Keep worker lifetime independent of individual turn failures.
	slog.WarnContext(diagnostics.WithRecord(context.Background(), waiter.diagnostic), "codex app-server turn interrupt failed; preserving runtime",
		"runtime_id", live.spec.RuntimeID,
		"thread_id", waiter.threadID,
		"turn_id", waiter.currentTurnID(),
		"reason", reason,
		"interrupt_error", redactAppServerDiagnostic([]byte(interruptErr.Error()), live.spec),
	)
	return interruptErr
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
		// Codex uses InvalidRequest for both inactive turns and active-turn
		// ID mismatches. Only its explicit inactive-turn response confirms
		// cancellation; other rejections are reported without stopping the worker.
		var rpcErr appServerRPCError
		if errors.As(err, &rpcErr) && rpcErr.Code == appServerRPCInvalidRequest && rpcErr.Message == "no active turn to interrupt" {
			live.appClient.logInfo("codex app-server turn interrupt rejected; preserving runtime",
				"runtime_id", live.spec.RuntimeID,
				"thread_id", threadID,
				"turn_id", turnID,
				"rpc_code", rpcErr.Code,
				"rpc_message", redactAppServerDiagnostic([]byte(rpcErr.Message), live.spec),
			)
			return nil
		}
		return fmt.Errorf("interrupt codex app-server turn: %w", err)
	}

	for {
		if _, ok := waiter.terminalResult(); ok {
			return nil
		}
		select {
		case result := <-waiter.ch:
			if waiter.matchesTurn(result.turnID) && (result.err != nil || result.success) {
				return nil
			}
		case <-live.done:
			return nil
		case <-ctx.Done():
			return fmt.Errorf("wait for interrupted codex app-server turn: %w", ctx.Err())
		}
	}
}

// interruptAppServerStructuredOutputTurn makes an emitted question a
// runtime-enforced turn boundary. The request runs asynchronously because this
// method is called while the app-server stdout reader is dispatching the
// command-completion notification that revealed the control record.
func (m *appServerManager) interruptAppServerStructuredOutputTurn(live *liveSession, threadID string) {
	if live == nil || live.appClient == nil {
		return
	}
	waiter := live.appServerTurnWaiter(threadID)
	if waiter == nil || !waiter.markStructuredOutputBoundary() {
		return
	}
	turnID := waiter.currentTurnID()
	threadID = strings.TrimSpace(threadID)
	if threadID == "" || turnID == "" {
		live.appClient.logDebug("cannot interrupt structured-output turn with incomplete identity",
			"thread_id", threadID,
			"turn_id", turnID,
		)
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), appServerTurnInterruptTimeout)
		defer cancel()
		if _, err := live.appClient.request(ctx, "turn/interrupt", map[string]any{
			"threadId": threadID,
			"turnId":   turnID,
		}); err != nil {
			live.appClient.logDebug("structured-output turn interrupt failed",
				"thread_id", threadID,
				"turn_id", turnID,
				"error", err,
			)
		}
	}()
}

// Capture the remaining pipe data at process exit. Notification delivery can
// then finish at its own pace even if another process still owns a writer.
type drainingAppServerStdout struct {
	*os.File
	mu        sync.Mutex
	remaining *bytes.Reader
}

func (s *drainingAppServerStdout) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.remaining != nil {
		return s.remaining.Read(p)
	}
	return s.File.Read(p)
}

func (s *drainingAppServerStdout) drain() {
	// Closing the pipe also releases a Read already in progress at exit.
	deadline := time.AfterFunc(appServerStdoutExitGracePeriod, func() { _ = s.File.Close() })
	defer deadline.Stop()
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := io.ReadAll(io.LimitReader(s.File, appServerStdoutExitBufferBytes))
	s.remaining = bytes.NewReader(data)
	_ = s.File.Close()
}

func (m *appServerManager) readAppServerStdout(runtimeID string, live *liveSession, stdout io.Reader, exited <-chan struct{}) {
	decoder := json.NewDecoder(stdout)
	for {
		var msg appServerWireMessage
		err := decoder.Decode(&msg)
		if err == nil {
			live.appClient.handleMessage(msg)
			continue
		}
		if errors.Is(err, os.ErrClosed) {
			select {
			case <-exited:
				return // The remaining pipe data was collected after exit.
			default:
			}
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			// EOF also accompanies a normal exit. Preserve that exit status.
			select {
			case <-exited:
				return
			case <-time.After(appServerStdoutExitGracePeriod):
			}
		}
		live.appClient.closeAllPending(fmt.Errorf("codex app-server stdout closed: %w", err))
		if !errors.Is(err, io.EOF) {
			live.diagnosticObserver.Observe("stdout_decode_failed", err, nil)
		}
		live.appClient.logError("codex app-server stdout unavailable", "runtime_id", runtimeID, "error", err)
		// The process can still accept work on stdin but can no longer
		// acknowledge it. Its regular exit path will reclaim the process tree.
		if killErr := live.cmd.Process.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			live.appClient.logError("stop codex app-server after stdout failure", "runtime_id", runtimeID, "error", killErr)
		}
		return
	}
}

func (m *appServerManager) waitAppServerSession(runtimeID string, live *liveSession, stdout *os.File) {
	exited := make(chan struct{})
	stdoutDone := make(chan struct{})
	reader := &drainingAppServerStdout{File: stdout}
	go func() {
		defer close(stdoutDone)
		defer stdout.Close()
		m.readAppServerStdout(runtimeID, live, reader, exited)
	}()
	err := live.cmd.Wait()
	close(exited)
	reader.drain()
	cleanupErr := stopProcessTree(live.cmd.Process.Pid)
	<-stdoutDone
	if live.stderr != nil {
		live.stderr.Drain()
	}
	live.mu.Lock()
	live.stopErr = cleanupErr
	live.mu.Unlock()
	live.telemetry.close()
	exitCode := 0
	if err != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	if live.appClient != nil {
		live.appClient.logInfo("codex app-server process exited",
			"runtime_id", runtimeID,
			"pid", live.cmd.Process.Pid,
			"exit_code", exitCode,
		)
		live.appClient.closeAllPending(fmt.Errorf("codex app-server exited with code %d", exitCode))
	}
	live.diagnosticObserver.Exit(exitCode, err, cleanupErr)
	isCurrent := false
	if cleanupErr == nil {
		isCurrent = m.detachSession(runtimeID, live)
	}
	if live.session != nil {
		if cleanupErr == nil {
			live.session.ProcessID = 0
		}
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

func appServerGenerateImageToolSpec() map[string]any {
	return map[string]any{
		"name":        "csgclaw_generate_image",
		"description": "Generate one image from the user's requested description using this Agent's configured image generation model and deliver it to the current conversation. Use this tool whenever the user asks to create an image. Never switch the chat model or call providers with shell commands. If image_model_not_configured is returned, ask the user to configure the Image generation model in the Agent profile; do not retry automatically.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string", "description": "Faithfully express the user's image request. Resolve references using established conversation context, but do not invent subjects, styles, text exclusions, or other constraints. Expand creatively only when explicitly requested. Preserve a supplied verbatim prompt unchanged."}}, "required": []string{"prompt"}, "additionalProperties": false},
	}
}

func appServerGenerateVideoToolSpec() map[string]any {
	return map[string]any{"name": "csgclaw_generate_video", "description": "Generate one video using this Agent's configured video model and deliver it to the conversation. Omit size or seconds unless the user explicitly supplied them; CSGClaw applies the selected model's advertised defaults and validates explicit values.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string"}, "size": map[string]any{"type": "string", "description": "Optional provider-advertised size value. Do not convert it to width x height or guess a value."}, "seconds": map[string]any{"type": "integer", "minimum": 1, "description": "Optional requested duration. Omit when the user did not specify one."}}, "required": []string{"prompt"}, "additionalProperties": false}}
}

// Explicit per-thread values override settings retained by cold-resumed threads.
func appServerThreadConfig(spec SessionSpec) map[string]any {
	values := appServerReasoningConfig(spec.Profile.ReasoningEffort)
	if values == nil {
		values = make(map[string]any)
	}
	if spec.UseProfileModelContext {
		values["model_context_window"] = spec.Profile.ModelMetadata.Normalized().ContextWindow
		values["model_auto_compact_token_limit"] = spec.Profile.ModelMetadata.CompactThreshold()
	}
	values["web_search"] = "cached"
	if spec.ExecutionMode == ExecutionModeReadOnly {
		values["web_search"] = "disabled"
	}
	return values
}

func (w *appServerTurnWaiter) captureDiagnosticNotification(turnID string, value any) {
	if w == nil || turnID == "" {
		return
	}
	raw := diagnostics.Protect(value)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.turnID == turnID {
		w.diagnostic.Capture("runtime.notification", raw)
		return
	}
	if w.turnID != "" {
		return
	}
	if w.diagnosticNoteBytes+len(raw) > 8<<20 {
		w.diagnostic.Issue("early_runtime_notifications_incomplete")
		return
	}
	if w.diagnosticNotes == nil {
		w.diagnosticNotes = map[string][]json.RawMessage{}
	}
	w.diagnosticNotes[turnID] = append(w.diagnosticNotes[turnID], raw)
	w.diagnosticNoteBytes += len(raw)
}
func (w *appServerTurnWaiter) flushDiagnosticNotesLocked(turnID string) {
	for _, raw := range w.diagnosticNotes[turnID] {
		w.diagnostic.Capture("runtime.notification", raw)
	}
	w.diagnosticNotes = nil
	w.diagnosticNoteBytes = 0
}
