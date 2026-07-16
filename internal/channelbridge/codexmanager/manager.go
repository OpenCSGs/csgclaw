package codexmanager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"csgclaw/internal/agent"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/channelbridge/codexbridge"
	agentruntime "csgclaw/internal/runtime"
	runtimecodex "csgclaw/internal/runtime/codex"
)

type Manager interface {
	Start(context.Context) error
	EnsureAgent(context.Context, agent.Agent) error
	StopAgent(string)
	Close()
}

type AgentLister interface {
	List() []agent.Agent
}

type RuntimeProvider interface {
	Runtime(kind string) (agentruntime.Runtime, error)
}

type AgentRestarter interface {
	Stop(context.Context, string) (agent.Agent, error)
	Start(context.Context, string) (agent.Agent, error)
}

type Options struct {
	Agents         AgentLister
	Runtimes       RuntimeProvider
	Restarter      AgentRestarter
	CSGClawClient  codexbridge.BotClient
	FeishuClient   codexbridge.BotClient
	FeishuProvider feishu.AgentCredentialProvider
}

func New(opts Options) (Manager, error) {
	if opts.Agents == nil || opts.Runtimes == nil {
		return nil, nil
	}
	hasCSGClawManager := opts.CSGClawClient != nil
	hasFeishuManager := opts.FeishuClient != nil && opts.FeishuProvider != nil
	if !hasCSGClawManager && !hasFeishuManager {
		return nil, nil
	}
	if opts.Restarter == nil {
		return nil, fmt.Errorf("codex bridge agent restarter is required")
	}
	codexRuntime, events, err := resolveCodexRuntime(opts.Runtimes)
	if err != nil || codexRuntime == nil {
		return nil, err
	}

	managers := make([]Manager, 0, 2)
	if hasCSGClawManager {
		managers = append(managers, newCSGClawManager(managerDeps{
			agents:    opts.Agents,
			restarter: opts.Restarter,
			runtime:   codexRuntime,
			events:    events,
			client:    opts.CSGClawClient,
		}))
	}
	if hasFeishuManager {
		managers = append(managers, newFeishuManager(managerDeps{
			agents:    opts.Agents,
			restarter: opts.Restarter,
			runtime:   codexRuntime,
			events:    events,
			client:    opts.FeishuClient,
			provider:  opts.FeishuProvider,
		}))
	}

	if len(managers) == 0 {
		return nil, nil
	}
	if len(managers) == 1 {
		return managers[0], nil
	}
	return &multiManager{managers: managers}, nil
}

func resolveCodexRuntime(provider RuntimeProvider) (*runtimecodex.Runtime, *runtimecodex.EventSink, error) {
	if provider == nil {
		return nil, nil, nil
	}
	rt, err := provider.Runtime(agentruntime.KindCodex)
	if err != nil {
		return nil, nil, nil
	}
	codexRuntime, ok := rt.(*runtimecodex.Runtime)
	if !ok {
		return nil, nil, fmt.Errorf("runtime %q has unexpected type %T", agentruntime.KindCodex, rt)
	}
	events, ok := codexRuntime.EventSink().(*runtimecodex.EventSink)
	if !ok || events == nil {
		return nil, nil, fmt.Errorf("runtime %q is missing codex event sink", agentruntime.KindCodex)
	}
	return codexRuntime, events, nil
}

type managerDeps struct {
	agents    AgentLister
	restarter AgentRestarter
	runtime   *runtimecodex.Runtime
	events    *runtimecodex.EventSink
	client    codexbridge.BotClient
	provider  feishu.AgentCredentialProvider
}

type multiManager struct {
	managers []Manager
}

func (m *multiManager) Start(ctx context.Context) error {
	if m == nil {
		return nil
	}
	var outErr error
	for _, manager := range m.managers {
		if manager == nil {
			continue
		}
		if err := manager.Start(ctx); err != nil {
			outErr = errors.Join(outErr, err)
		}
	}
	return outErr
}

func (m *multiManager) EnsureAgent(ctx context.Context, a agent.Agent) error {
	if m == nil {
		return nil
	}
	var outErr error
	for _, manager := range m.managers {
		if manager == nil {
			continue
		}
		if err := manager.EnsureAgent(ctx, a); err != nil {
			outErr = errors.Join(outErr, err)
		}
	}
	return outErr
}

func (m *multiManager) StopAgent(agentID string) {
	if m == nil {
		return
	}
	for _, manager := range m.managers {
		if manager != nil {
			manager.StopAgent(agentID)
		}
	}
}

func (m *multiManager) Close() {
	if m == nil {
		return
	}
	for _, manager := range m.managers {
		if manager != nil {
			manager.Close()
		}
	}
}

func (m *multiManager) PermissionDecider() runtimecodex.PermissionDecider {
	if m == nil {
		return nil
	}
	for _, manager := range m.managers {
		decider, ok := manager.(interface {
			PermissionDecider() runtimecodex.PermissionDecider
		})
		if !ok {
			continue
		}
		if decider.PermissionDecider() != nil {
			return decider.PermissionDecider()
		}
	}
	return nil
}

func (m *multiManager) UserInputResponder() runtimecodex.UserInputBroker {
	if m == nil {
		return nil
	}
	for _, manager := range m.managers {
		responder, ok := manager.(interface {
			UserInputResponder() runtimecodex.UserInputBroker
		})
		if ok && responder.UserInputResponder() != nil {
			return responder.UserInputResponder()
		}
	}
	return nil
}

type csgclawManager struct {
	agents    AgentLister
	restarter AgentRestarter
	runtime   *runtimecodex.Runtime
	bridge    *codexbridge.Service
	ensuring  ensureGate
}

func newCSGClawManager(deps managerDeps) *csgclawManager {
	return &csgclawManager{
		agents:    deps.agents,
		restarter: deps.restarter,
		runtime:   deps.runtime,
		bridge:    codexbridge.NewService(deps.client, deps.runtime.SessionManager(), deps.events, deps.runtime.UserInputBroker()),
		ensuring:  newEnsureGate(),
	}
}

func (m *csgclawManager) Start(ctx context.Context) error {
	if m == nil || m.agents == nil || m.runtime == nil || m.bridge == nil {
		return nil
	}
	agents := m.agents.List()
	var startErr error
	for _, a := range agents {
		if !shouldRestoreCodexBridgeOnStartup(a) {
			continue
		}
		session, err := ensureSession(ctx, m.restarter, m.runtime, "csgclaw", a)
		if err != nil {
			startErr = errors.Join(startErr, fmt.Errorf("%s: %w", a.Name, err))
			continue
		}
		if err := m.bridge.StartBot(ctx, bindingForAgent(a, session.SessionID)); err != nil {
			startErr = errors.Join(startErr, fmt.Errorf("%s: %w", a.Name, err))
		}
	}
	return startErr
}

func (m *csgclawManager) EnsureAgent(ctx context.Context, a agent.Agent) error {
	if m == nil || m.runtime == nil || m.bridge == nil {
		return nil
	}
	if !shouldStartCodexBridge(a) {
		m.StopAgent(a.ID)
		return nil
	}
	if !m.ensuring.begin(a.ID) {
		return nil
	}
	defer m.ensuring.finish(a.ID)
	session, err := ensureSession(ctx, m.restarter, m.runtime, "csgclaw", a)
	if err != nil {
		return err
	}
	// Force a fresh bot-event subscription even when the binding is unchanged.
	// This repairs cases where the bridge worker exists but missed its initial
	// subscription window and would otherwise be treated as a no-op restart.
	m.stopAgentBridge(a)
	return m.bridge.StartBot(ctx, bindingForAgent(a, session.SessionID))
}

func (m *csgclawManager) StopAgent(agentID string) {
	if m == nil || m.bridge == nil {
		return
	}
	stopBotIDs(m.bridge, strings.TrimSpace(agentID), agent.ParticipantIDForAgent("", agentID))
}

func (m *csgclawManager) stopAgentBridge(a agent.Agent) {
	if m == nil || m.bridge == nil {
		return
	}
	stopBotIDs(m.bridge, strings.TrimSpace(a.ID), agent.ParticipantIDForAgent(a.Name, a.ID))
}

func (m *csgclawManager) Close() {
	if m == nil || m.bridge == nil {
		return
	}
	m.bridge.Close()
}

func (m *csgclawManager) PermissionDecider() runtimecodex.PermissionDecider {
	if m == nil || m.runtime == nil {
		return nil
	}
	return m.runtime.PermissionBroker()
}

func (m *csgclawManager) UserInputResponder() runtimecodex.UserInputBroker {
	if m == nil || m.runtime == nil {
		return nil
	}
	return m.runtime.UserInputBroker()
}

type feishuManager struct {
	agents              AgentLister
	restarter           AgentRestarter
	runtime             *runtimecodex.Runtime
	bridge              *codexbridge.Service
	provider            feishu.AgentCredentialProvider
	ensuring            ensureGate
	activeParticipantMu sync.Mutex
	activeParticipants  map[string]string
}

func newFeishuManager(deps managerDeps) *feishuManager {
	return &feishuManager{
		agents:             deps.agents,
		restarter:          deps.restarter,
		runtime:            deps.runtime,
		bridge:             codexbridge.NewService(deps.client, deps.runtime.SessionManager(), deps.events, deps.runtime.UserInputBroker()),
		provider:           deps.provider,
		ensuring:           newEnsureGate(),
		activeParticipants: make(map[string]string),
	}
}

func (m *feishuManager) Start(ctx context.Context) error {
	if m == nil || m.agents == nil || m.runtime == nil || m.bridge == nil {
		return nil
	}
	agents := m.agents.List()
	var startErr error
	for _, a := range agents {
		if !m.shouldStartForAgent(a) {
			continue
		}
		session, err := ensureSession(ctx, m.restarter, m.runtime, "feishu", a)
		if err != nil {
			startErr = errors.Join(startErr, fmt.Errorf("%s: %w", a.Name, err))
			continue
		}
		participantID := strings.TrimSpace(m.participantIDForAgent(a))
		if participantID == "" {
			startErr = errors.Join(startErr, fmt.Errorf("%s: feishu participant not configured", a.Name))
			continue
		}
		binding := bindingForAgent(a, session.SessionID)
		binding.BotID = participantID
		if err := m.bridge.StartBot(ctx, binding); err != nil {
			startErr = errors.Join(startErr, fmt.Errorf("%s: %w", a.Name, err))
			continue
		}
		m.rememberParticipant(a.ID, participantID)
	}
	return startErr
}

func (m *feishuManager) EnsureAgent(ctx context.Context, a agent.Agent) error {
	if m == nil || m.runtime == nil || m.bridge == nil {
		return nil
	}
	participantID := strings.TrimSpace(m.participantIDForAgent(a))
	if !shouldStartCodexBridge(a) || participantID == "" {
		m.StopAgent(a.ID)
		return nil
	}
	if !m.ensuring.begin(a.ID) {
		return nil
	}
	defer m.ensuring.finish(a.ID)

	m.stopStaleBridgeForAgent(a.ID, participantID)
	session, err := ensureSession(ctx, m.restarter, m.runtime, "feishu", a)
	if err != nil {
		return err
	}
	m.stopAgentBridgeForAgent(a.ID, participantID, a)
	binding := bindingForAgent(a, session.SessionID)
	binding.BotID = participantID
	if err := m.bridge.StartBot(ctx, binding); err != nil {
		return err
	}
	m.rememberParticipant(a.ID, participantID)
	return nil
}

func (m *feishuManager) shouldStartForAgent(a agent.Agent) bool {
	if !shouldStartCodexBridge(a) {
		return false
	}
	return strings.TrimSpace(m.participantIDForAgent(a)) != ""
}

func (m *feishuManager) StopAgent(agentID string) {
	if m == nil || m.bridge == nil {
		return
	}
	m.stopAgentBridgeForAgent(agentID, "", agent.Agent{ID: agentID})
}

func (m *feishuManager) stopStaleBridgeForAgent(agentID string, participantID string) {
	if m == nil || m.bridge == nil {
		return
	}
	staleParticipant := m.clearStaleParticipant(agentID, participantID)
	stopBotIDs(m.bridge, staleParticipant)
}

func (m *feishuManager) stopAgentBridgeForAgent(agentID string, participantID string, a agent.Agent) {
	activeParticipant := m.clearActiveParticipant(agentID)
	stopBotIDs(
		m.bridge,
		activeParticipant,
		strings.TrimSpace(participantID),
		strings.TrimSpace(m.participantIDForAgent(a)),
		strings.TrimSpace(agentID),
		agent.ParticipantIDForAgent("", agentID),
	)
}

func (m *feishuManager) Close() {
	if m == nil || m.bridge == nil {
		return
	}
	m.bridge.Close()
}

func (m *feishuManager) PermissionDecider() runtimecodex.PermissionDecider {
	if m == nil || m.runtime == nil {
		return nil
	}
	return m.runtime.PermissionBroker()
}

func (m *feishuManager) UserInputResponder() runtimecodex.UserInputBroker {
	if m == nil || m.runtime == nil {
		return nil
	}
	return m.runtime.UserInputBroker()
}

func (m *feishuManager) participantIDForAgent(a agent.Agent) string {
	agentID := strings.TrimSpace(a.ID)
	if agentID == "" || m == nil || m.provider == nil {
		return ""
	}
	participantID, _, ok := m.provider.BotConfigForAgent(agentID)
	if !ok {
		return ""
	}
	return strings.TrimSpace(participantID)
}

func (m *feishuManager) rememberParticipant(agentID, participantID string) {
	agentID = strings.TrimSpace(agentID)
	participantID = strings.TrimSpace(participantID)
	if agentID == "" || participantID == "" {
		return
	}
	m.activeParticipantMu.Lock()
	defer m.activeParticipantMu.Unlock()
	if m.activeParticipants == nil {
		m.activeParticipants = make(map[string]string)
	}
	m.activeParticipants[agentID] = participantID
}

func (m *feishuManager) clearActiveParticipant(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}
	m.activeParticipantMu.Lock()
	defer m.activeParticipantMu.Unlock()
	participantID := strings.TrimSpace(m.activeParticipants[agentID])
	delete(m.activeParticipants, agentID)
	return participantID
}

func (m *feishuManager) clearStaleParticipant(agentID, participantID string) string {
	agentID = strings.TrimSpace(agentID)
	participantID = strings.TrimSpace(participantID)
	if agentID == "" || participantID == "" {
		return ""
	}
	m.activeParticipantMu.Lock()
	defer m.activeParticipantMu.Unlock()
	activeParticipant := strings.TrimSpace(m.activeParticipants[agentID])
	if activeParticipant == "" || activeParticipant == participantID {
		return ""
	}
	delete(m.activeParticipants, agentID)
	return activeParticipant
}

func ensureSession(ctx context.Context, restarter AgentRestarter, runtime *runtimecodex.Runtime, channel string, a agent.Agent) (*runtimecodex.Session, error) {
	if runtime == nil {
		return nil, fmt.Errorf("codex runtime is required")
	}
	handle := runtimecodex.SessionHandle{RuntimeID: strings.TrimSpace(a.RuntimeID)}
	session, err := runtime.SessionManager().Session(handle)
	if err == nil {
		slog.Debug(channel+" codex bridge session found",
			"agent_id", strings.TrimSpace(a.ID),
			"agent_name", strings.TrimSpace(a.Name),
			"runtime_id", strings.TrimSpace(a.RuntimeID),
			"session_id", strings.TrimSpace(session.SessionID),
		)
		return session, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if restarter == nil {
		return nil, fmt.Errorf("%s codex bridge session missing and agent restarter is not configured", channel)
	}

	slog.Warn(channel+" codex bridge session missing; restarting agent runtime",
		"agent_id", strings.TrimSpace(a.ID),
		"agent_name", strings.TrimSpace(a.Name),
		"runtime_id", strings.TrimSpace(a.RuntimeID),
	)
	if _, stopErr := restarter.Stop(ctx, a.ID); stopErr != nil && !strings.Contains(stopErr.Error(), "not found") {
		return nil, stopErr
	}
	updated, startErr := restarter.Start(ctx, a.ID)
	if startErr != nil {
		return nil, startErr
	}
	session, err = runtime.SessionManager().Session(runtimecodex.SessionHandle{RuntimeID: strings.TrimSpace(updated.RuntimeID)})
	if err != nil {
		return nil, err
	}
	slog.Debug(channel+" codex bridge session restored",
		"agent_id", strings.TrimSpace(updated.ID),
		"agent_name", strings.TrimSpace(updated.Name),
		"runtime_id", strings.TrimSpace(updated.RuntimeID),
		"session_id", strings.TrimSpace(session.SessionID),
	)
	return session, nil
}

func bindingForAgent(a agent.Agent, sessionID string) codexbridge.Binding {
	return codexbridge.Binding{
		BotID:     agent.ParticipantIDForAgent(a.Name, a.ID),
		RuntimeID: strings.TrimSpace(a.RuntimeID),
		SessionID: strings.TrimSpace(sessionID),
	}
}

func shouldStartCodexBridge(a agent.Agent) bool {
	if !isCodexBridgeRole(a.Role) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(a.RuntimeKind), agent.RuntimeKindCodex) {
		return false
	}
	if !(a.ProfileComplete || a.AgentProfile.ProfileComplete) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(a.Status), string(agentruntime.StateRunning))
}

func shouldRestoreCodexBridgeOnStartup(a agent.Agent) bool {
	if !isCodexBridgeRole(a.Role) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(a.RuntimeKind), agent.RuntimeKindCodex) {
		return false
	}
	if !(a.ProfileComplete || a.AgentProfile.ProfileComplete) {
		return false
	}
	return !strings.EqualFold(strings.TrimSpace(a.Status), string(agentruntime.StateStopped))
}

func isCodexBridgeRole(role string) bool {
	role = strings.TrimSpace(role)
	return strings.EqualFold(role, agent.RoleWorker) || strings.EqualFold(role, agent.RoleManager)
}

type ensureGate struct {
	mu     sync.Mutex
	active map[string]bool
}

func newEnsureGate() ensureGate {
	return ensureGate{active: make(map[string]bool)}
}

func (g *ensureGate) begin(agentID string) bool {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active == nil {
		g.active = make(map[string]bool)
	}
	if g.active[agentID] {
		return false
	}
	g.active[agentID] = true
	return true
}

func (g *ensureGate) finish(agentID string) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.active, agentID)
}

func stopBotIDs(bridge *codexbridge.Service, ids ...string) {
	if bridge == nil {
		return
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		bridge.StopBot(id)
	}
}
