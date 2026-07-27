package codexmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"csgclaw/internal/agent"
	csgclawchannel "csgclaw/internal/channel/csgclaw"
	"csgclaw/internal/channel/feishu"
	"csgclaw/internal/channelbridge/codexbridge"
	agentruntime "csgclaw/internal/runtime"
	runtimecodex "csgclaw/internal/runtime/codex"
	"csgclaw/internal/worklease"
)

type Manager interface {
	Start(context.Context) error
	EnsureAgent(context.Context, agent.Agent) error
	StopAgent(string)
	RefreshAgentChannel(context.Context, agent.Agent, string) error
	Close()
}

type AgentLister interface {
	List() []agent.Agent
}

type RuntimeProvider interface {
	Runtime(kind string) (agentruntime.Runtime, error)
}

type channelManager interface {
	Manager
	supportsAgentChannel(string) bool
}

type Options struct {
	Agents         AgentLister
	Runtimes       RuntimeProvider
	CSGClawClient  codexbridge.BotClient
	FeishuClient   codexbridge.BotClient
	FeishuProvider feishu.AgentCredentialProvider
	WorkReporter   worklease.ParticipantWorkReporter
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
	codexRuntime, events, err := resolveCodexRuntime(opts.Runtimes)
	if err != nil || codexRuntime == nil {
		return nil, err
	}

	managers := make([]Manager, 0, 2)
	if hasCSGClawManager {
		managers = append(managers, newCSGClawManager(managerDeps{
			agents:   opts.Agents,
			runtime:  codexRuntime,
			events:   events,
			client:   opts.CSGClawClient,
			reporter: opts.WorkReporter,
		}))
	}
	if hasFeishuManager {
		managers = append(managers, newFeishuManager(managerDeps{
			agents:   opts.Agents,
			runtime:  codexRuntime,
			events:   events,
			client:   opts.FeishuClient,
			provider: opts.FeishuProvider,
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
	agents   AgentLister
	runtime  *runtimecodex.Runtime
	events   *runtimecodex.EventSink
	client   codexbridge.BotClient
	provider feishu.AgentCredentialProvider
	reporter worklease.ParticipantWorkReporter
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

func (m *multiManager) RefreshAgentChannel(ctx context.Context, a agent.Agent, channel string) error {
	channel = normalizeAgentChannel(channel)
	if channel == "" {
		return fmt.Errorf("channel is required")
	}
	if m == nil {
		return nil
	}
	var outErr error
	handled := false
	for _, manager := range m.managers {
		target, ok := manager.(channelManager)
		if !ok || !target.supportsAgentChannel(channel) {
			continue
		}
		handled = true
		if err := target.RefreshAgentChannel(ctx, a, channel); err != nil {
			outErr = errors.Join(outErr, err)
		}
	}
	if !handled {
		return fmt.Errorf("channel %q bridge manager is not configured", channel)
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
	agents   AgentLister
	runtime  *runtimecodex.Runtime
	bridge   *codexbridge.Service
	ensuring ensureGate
}

func newCSGClawManager(deps managerDeps) *csgclawManager {
	bridgeOptions := []codexbridge.ServiceOption{
		codexbridge.WithUserInputBroker(deps.runtime.UserInputBroker()),
		codexbridge.WithParticipantWorkReporter(deps.reporter),
	}
	if registrar, ok := deps.reporter.(agentruntime.TurnControllerRegistrar); ok {
		bridgeOptions = append(bridgeOptions, codexbridge.WithTurnControllerRegistrar(registrar))
	}
	return &csgclawManager{
		agents:  deps.agents,
		runtime: deps.runtime,
		bridge: codexbridge.NewService(
			deps.client,
			deps.runtime.SessionManager(),
			deps.events,
			bridgeOptions...,
		),
		ensuring: newEnsureGate(),
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
		session, err := currentSession(m.runtime, a)
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
	for {
		session, err := currentSession(m.runtime, a)
		if err == nil {
			// Force a fresh bot-event subscription even when the binding is unchanged.
			// This repairs cases where the bridge worker exists but missed its initial
			// subscription window and would otherwise be treated as a no-op restart.
			m.stopAgentBridge(a)
			err = m.bridge.StartBot(ctx, bindingForAgent(a, session.SessionID))
		}
		if m.ensuring.finish(a.ID) {
			continue
		}
		return err
	}
}

func (m *csgclawManager) RefreshAgentChannel(ctx context.Context, a agent.Agent, channel string) error {
	channel = normalizeAgentChannel(channel)
	if !m.supportsAgentChannel(channel) {
		return unsupportedAgentChannelError(channel)
	}
	return m.EnsureAgent(ctx, a)
}

func (m *csgclawManager) supportsAgentChannel(channel string) bool {
	return normalizeAgentChannel(channel) == csgclawchannel.ChannelID
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
	runtime             *runtimecodex.Runtime
	bridge              *codexbridge.Service
	provider            feishu.AgentCredentialProvider
	ensuring            ensureGate
	activeParticipantMu sync.Mutex
	activeParticipants  map[string]string
}

func newFeishuManager(deps managerDeps) *feishuManager {
	return &feishuManager{
		agents:  deps.agents,
		runtime: deps.runtime,
		bridge: codexbridge.NewService(
			deps.client,
			deps.runtime.SessionManager(),
			deps.events,
			codexbridge.WithUserInputBroker(deps.runtime.UserInputBroker()),
		),
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
		session, err := currentSession(m.runtime, a)
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
	if !shouldStartCodexBridge(a) {
		m.StopAgent(a.ID)
		return nil
	}
	if !m.ensuring.begin(a.ID) {
		return nil
	}
	for {
		participantID := strings.TrimSpace(m.participantIDForAgent(a))
		var err error
		if participantID == "" {
			m.StopAgent(a.ID)
		} else {
			m.stopStaleBridgeForAgent(a.ID, participantID)
			var session *runtimecodex.Session
			session, err = currentSession(m.runtime, a)
			if err == nil {
				m.stopAgentBridgeForAgent(a.ID, participantID, a)
				binding := bindingForAgent(a, session.SessionID)
				binding.BotID = participantID
				err = m.bridge.StartBot(ctx, binding)
				if err == nil {
					m.rememberParticipant(a.ID, participantID)
				}
			}
		}
		if m.ensuring.finish(a.ID) {
			continue
		}
		return err
	}
}

func (m *feishuManager) RefreshAgentChannel(ctx context.Context, a agent.Agent, channel string) error {
	channel = normalizeAgentChannel(channel)
	if !m.supportsAgentChannel(channel) {
		return unsupportedAgentChannelError(channel)
	}
	return m.EnsureAgent(ctx, a)
}

func (m *feishuManager) supportsAgentChannel(channel string) bool {
	return normalizeAgentChannel(channel) == feishu.ChannelID
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

func currentSession(runtime *runtimecodex.Runtime, a agent.Agent) (*runtimecodex.Session, error) {
	if runtime == nil {
		return nil, fmt.Errorf("codex runtime is required")
	}
	return runtime.SessionManager().LiveSession(runtimecodex.SessionHandle{RuntimeID: strings.TrimSpace(a.RuntimeID)})
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

func normalizeAgentChannel(channel string) string {
	return strings.ToLower(strings.TrimSpace(channel))
}

func unsupportedAgentChannelError(channel string) error {
	channel = normalizeAgentChannel(channel)
	if channel == "" {
		return fmt.Errorf("channel is required")
	}
	return fmt.Errorf("channel %q is not supported by this codex bridge manager", channel)
}

type ensureGate struct {
	mu      sync.Mutex
	active  map[string]bool
	pending map[string]bool
}

func newEnsureGate() ensureGate {
	return ensureGate{active: make(map[string]bool), pending: make(map[string]bool)}
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
		if g.pending == nil {
			g.pending = make(map[string]bool)
		}
		g.pending[agentID] = true
		return false
	}
	g.active[agentID] = true
	return true
}

func (g *ensureGate) finish(agentID string) bool {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pending[agentID] {
		delete(g.pending, agentID)
		return true
	}
	delete(g.active, agentID)
	return false
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
