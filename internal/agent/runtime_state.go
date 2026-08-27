package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
)

// RuntimeAvailabilityState describes the most recent application-level
// readiness observation. It is separate from Agent.Status, which remains the
// durable runtime lifecycle state.
type RuntimeAvailabilityState string

const (
	RuntimeAvailabilityUnknown       RuntimeAvailabilityState = "unknown"
	RuntimeAvailabilityReady         RuntimeAvailabilityState = "ready"
	RuntimeAvailabilityDegraded      RuntimeAvailabilityState = "degraded"
	RuntimeAvailabilityNotApplicable RuntimeAvailabilityState = "not_applicable"

	// runtimeAvailabilityMaxAge bounds how long a transient readiness result may
	// influence the roster or gateway-warning UI without a new probe.
	runtimeAvailabilityMaxAge = 5 * time.Second
)

const (
	DesiredStateRunning = "running"
	DesiredStateStopped = "stopped"
)

// SetDesiredState persists the lifecycle intent owned by Agent Engine without
// conflating it with the Runtime's observed status.
func (s *Service) SetDesiredState(id, desired string, touch bool) (Agent, error) {
	id = strings.TrimSpace(id)
	desired = strings.ToLower(strings.TrimSpace(desired))
	if desired != DesiredStateRunning && desired != DesiredStateStopped {
		return Agent{}, fmt.Errorf("invalid agent desired state %q", desired)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, key, ok := s.agentByIDLocked(id)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	if current.DesiredState == desired && !touch {
		return *cloneAgent(&current), nil
	}
	previous := current
	current.DesiredState = desired
	current.UpdatedAt = time.Now().UTC()
	s.agents[key] = current
	if err := s.saveLocked(); err != nil {
		s.agents[key] = previous
		return Agent{}, err
	}
	return *cloneAgent(&current), nil
}

var runtimeAvailabilityProbeTimeout = 2 * time.Second

// RuntimeAvailability is intentionally ephemeral. CheckedAt lets clients
// distinguish a fresh readiness result from a lifecycle-only listing.
type RuntimeAvailability struct {
	State     RuntimeAvailabilityState
	CheckedAt time.Time
	ExpiresAt time.Time
	Reason    string
	handleID  string
}

func cloneRuntimeAvailability(src *RuntimeAvailability) *RuntimeAvailability {
	if src == nil {
		return nil
	}
	out := *src
	return &out
}

type RuntimeView struct {
	AgentID       string
	AgentName     string
	RuntimeID     string
	RuntimeKind   string
	HandleID      string
	State         agentruntime.State
	LogsSupported bool
}

type gatewayBoxFactory interface {
	CreateGatewayBox(ctx context.Context, rt sandbox.Runtime, image, name, botID string, profile agentruntime.Profile) (sandbox.Instance, sandbox.Info, error)
	GatewayCreateSpec(image, name, botID string, profile agentruntime.Profile) (sandbox.CreateSpec, error)
}

type PicoClawRuntimeHost struct {
	SandboxProviderName   func() string
	EnsureRuntime         func(agentID string) (sandbox.Runtime, error)
	AgentHome             func(agentID string) (string, error)
	RuntimeHome           func(agentID string) (string, error)
	CloseRuntime          func(homeDir string, rt sandbox.Runtime) error
	ResolveBox            func(ctx context.Context, rt sandbox.Runtime, got Agent) (sandbox.Instance, string, error)
	CreateBox             func(ctx context.Context, rt sandbox.Runtime, spec sandbox.CreateSpec) (sandbox.Instance, error)
	StartBox              func(ctx context.Context, box sandbox.Instance) error
	StopBox               func(ctx context.Context, box sandbox.Instance, opts sandbox.StopOptions) error
	BoxInfo               func(ctx context.Context, box sandbox.Instance) (sandbox.Info, error)
	ForceRemoveBox        func(ctx context.Context, rt sandbox.Runtime, idOrName string) error
	CloseBox              func(box sandbox.Instance) error
	RunBoxCommand         func(ctx context.Context, box sandbox.Instance, name string, args []string, w io.Writer) (int, error)
	ResolveAgent          func(h agentruntime.Handle) (Agent, error)
	ResolveRuntimeProfile func(h agentruntime.Handle) (agentruntime.Profile, error)
	SyncHandle            func(h agentruntime.Handle) error
	StreamLogs            func(ctx context.Context, agentID string, follow bool, lines int, w io.Writer) error
}

func (s *Service) PicoClawRuntimeHost() PicoClawRuntimeHost {
	return PicoClawRuntimeHost{
		SandboxProviderName: s.sandboxProviderName,
		EnsureRuntime:       s.ensureRuntime,
		AgentHome:           s.agentHomeDir,
		RuntimeHome:         s.sandboxRuntimeHome,
		CloseRuntime:        s.closeRuntime,
		ResolveBox: func(ctx context.Context, rt sandbox.Runtime, got Agent) (sandbox.Instance, string, error) {
			return s.resolveAgentBox(ctx, rt, got)
		},
		CreateBox:      s.createBox,
		StartBox:       s.startBox,
		StopBox:        s.stopBox,
		BoxInfo:        s.boxInfo,
		ForceRemoveBox: s.forceRemoveBox,
		CloseBox:       s.closeBox,
		RunBoxCommand:  s.runBoxCommand,
		ResolveAgent:   s.gatewayRuntimeAgent,
		ResolveRuntimeProfile: func(h agentruntime.Handle) (agentruntime.Profile, error) {
			got, err := s.gatewayRuntimeAgent(h)
			if err != nil {
				return agentruntime.Profile{}, err
			}
			return s.runtimeProfileForAgent(got), nil
		},
		SyncHandle: s.syncRuntimeHandle,
		StreamLogs: s.streamRuntimeHostLogs,
	}
}

func (s *Service) OpenClawRuntimeHost() PicoClawRuntimeHost {
	return s.PicoClawRuntimeHost()
}

func (s *Service) sandboxProviderName() string {
	if s == nil || s.sandbox == nil {
		return ""
	}
	return strings.TrimSpace(s.sandbox.Name())
}

// SandboxProviderName returns the configured provider backing sandboxed agent runtimes.
func (s *Service) SandboxProviderName() string {
	return s.sandboxProviderName()
}

func (s *Service) setGatewayWorkPhase(p uint32) {
	if s == nil {
		return
	}
	s.gatewayWorkPhase.Store(p)
}

func (s *Service) runtimeForKind(kind string) (agentruntime.Runtime, error) {
	if s == nil {
		return nil, fmt.Errorf("agent service is required")
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return nil, fmt.Errorf("runtime kind is required")
	}
	if resolved := agentruntime.RuntimeConfigForKind(kind).LegacyKind(); resolved != "" {
		kind = resolved
	}
	rt := s.runtimeRegistry[kind]
	if rt == nil {
		return nil, fmt.Errorf("runtime kind %q is not registered", kind)
	}
	return rt, nil
}

func (s *Service) gatewayRuntimeKind() string {
	if s == nil {
		return RuntimeKindPicoClawSandbox
	}
	if kind := runtimeKindForGatewayRuntime(s.gatewayRuntime); kind != "" {
		return kind
	}
	return RuntimeKindPicoClawSandbox
}

func (s *Service) runtimeProfileForAgent(a Agent) agentruntime.Profile {
	return s.runtimeProfileForAgentWithProfile(a, a.AgentProfile)
}

func (s *Service) runtimeProfileForAgentWithProfile(a Agent, profile AgentProfile) agentruntime.Profile {
	return s.runtimeProfileForKind(strings.TrimSpace(a.RuntimeKind), a.ID, a.Name, a.Description, profile)
}

func (s *Service) runtimeProfileForKind(runtimeKind, agentID, fallbackName, fallbackDescription string, profile AgentProfile) agentruntime.Profile {
	profile = normalizeProfile(profile, fallbackName, fallbackDescription)
	profile = s.hydrateProfileFromCatalog(profile)
	baseURL := profileBaseURL(profile)
	apiKey := profileAPIKey(profile)
	env := normalizeStringMap(profile.Env)

	if runtimeKind == RuntimeKindCodex {
		managerBaseURL := config.ResolveLocalBaseURL(s.server)
		if managerBaseURL != "" {
			baseURL = llmBridgeBaseURL(managerBaseURL, agentID)
			if env == nil {
				env = make(map[string]string)
			}
			env["CSGCLAW_BASE_URL"] = managerBaseURL
		}
		if token := strings.TrimSpace(s.server.AccessToken); token != "" {
			apiKey = token
			if env == nil {
				env = make(map[string]string)
			}
			env["CSGCLAW_ACCESS_TOKEN"] = token
		}
		if canonicalAgentID(agentID) == ManagerUserID {
			if capability := s.connectorCapability(agentID); capability != "" {
				if env == nil {
					env = make(map[string]string)
				}
				env[ConnectorCapabilityEnv] = capability
			}
		}
	}

	return (agentruntime.Profile{
		Provider:        profile.Provider,
		BaseURL:         baseURL,
		APIKey:          apiKey,
		ModelID:         profile.ModelID,
		ReasoningEffort: profile.ReasoningEffort,
		Env:             env,
	}).Normalized()
}

func (s *Service) Runtime(kind string) (agentruntime.Runtime, error) {
	return s.runtimeForKind(kind)
}

// Inspect returns an agent with its durable lifecycle state refreshed and, for
// runtimes that support it, a fresh readiness observation. It is intentionally
// separate from Agent and ListContext so identity and registry callers do not
// accidentally turn a metadata read into a gateway health probe.
func (s *Service) Inspect(ctx context.Context, id string) (Agent, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	a, ok := s.agentSnapshot(id)
	if !ok {
		return Agent{}, false
	}
	a = s.withRuntimeImageMigrationStatus(ctx, s.hydrateAgentStatus(ctx, a))
	return s.withConfiguredAgentStartupStatus(s.refreshRuntimeAvailability(ctx, a)), true
}

func (s *Service) WorkspaceRoot(agentName string) (string, error) {
	got, ok := s.agentSnapshotByName(agentName)
	if !ok {
		return "", fmt.Errorf("agent %q not found", strings.TrimSpace(agentName))
	}
	return s.agentWorkspaceRoot(got.ID, got.RuntimeKind)
}

func (s *Service) WorkspaceRootByID(agentID string) (string, error) {
	got, ok := s.agentSnapshot(agentID)
	if !ok {
		return "", fmt.Errorf("agent %q not found", strings.TrimSpace(agentID))
	}
	return s.agentWorkspaceRoot(got.ID, got.RuntimeKind)
}

func (s *Service) SkillsRoot(agentName string) (string, error) {
	got, ok := s.agentSnapshotByName(agentName)
	if !ok {
		return "", fmt.Errorf("agent %q not found", strings.TrimSpace(agentName))
	}
	return s.agentSkillsRoot(got.ID, got.RuntimeKind)
}

func runtimeHandleForAgent(a Agent) agentruntime.Handle {
	return agentruntime.Handle{
		RuntimeID: normalizeRuntimeID(a.RuntimeID, a.ID),
		HandleID:  strings.TrimSpace(a.BoxID),
	}
}

func (s *Service) gatewayBoxFactory() (gatewayBoxFactory, error) {
	rt, err := s.runtimeForKind(s.gatewayRuntimeKind())
	if err != nil {
		return nil, err
	}
	factory, ok := rt.(gatewayBoxFactory)
	if !ok {
		return nil, fmt.Errorf("runtime %q does not support gateway box creation", rt.Kind())
	}
	return factory, nil
}

func (s *Service) syncRuntimeHandle(h agentruntime.Handle) error {
	runtimeID := normalizeRuntimeID(h.RuntimeID, "")
	runtimeAliases := runtimeIDLookupAliases(runtimeID)
	handleID := strings.TrimSpace(h.HandleID)
	if runtimeID == "" || handleID == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rt, ok := s.runtimeRecords[runtimeID]
	if ok && strings.TrimSpace(rt.SandboxID) != handleID {
		rt.SandboxID = handleID
		s.runtimeRecords[runtimeID] = normalizeRuntimeRecord(rt)
	}

	changed := false
	for agentID, current := range s.agents {
		if !slices.Contains(runtimeAliases, normalizeRuntimeID(current.RuntimeID, current.ID)) && !slices.Contains(runtimeAliases, strings.TrimSpace(current.RuntimeID)) {
			continue
		}
		if strings.TrimSpace(current.BoxID) == handleID {
			continue
		}
		current.BoxID = handleID
		s.agents[agentID] = current
		s.clearRuntimeAvailabilityLocked(current.ID)
		s.syncRuntimeRecordLocked(current)
		changed = true
	}
	if !changed && ok {
		return nil
	}
	return s.saveLocked()
}

func (s *Service) runtimeInfo(ctx context.Context, rt agentruntime.Runtime, h agentruntime.Handle) (agentruntime.Info, error) {
	if rt == nil {
		return agentruntime.Info{}, fmt.Errorf("runtime is required")
	}
	return rt.Info(ctx, h)
}

func (s *Service) RuntimeView(ctx context.Context, id string) (RuntimeView, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return RuntimeView{}, fmt.Errorf("agent id is required")
	}

	got, ok := s.agentSnapshot(id)
	if !ok {
		return RuntimeView{}, fmt.Errorf("agent %q not found", id)
	}
	runtimeImpl, err := s.runtimeForKind(strings.TrimSpace(got.RuntimeKind))
	if err != nil {
		return RuntimeView{}, err
	}

	view := RuntimeView{
		AgentID:       got.ID,
		AgentName:     got.Name,
		RuntimeID:     normalizeRuntimeID(got.RuntimeID, got.ID),
		RuntimeKind:   runtimeImpl.Kind(),
		HandleID:      strings.TrimSpace(got.BoxID),
		State:         agentruntime.State(strings.TrimSpace(got.Status)),
		LogsSupported: supportsRuntimeLogs(runtimeImpl),
	}

	info, err := s.runtimeInfo(ctx, runtimeImpl, runtimeHandleForAgent(got))
	if err != nil {
		if sandbox.IsNotFound(err) {
			view.State = agentruntime.StateUnknown
			return view, nil
		}
		return RuntimeView{}, err
	}
	if handleID := strings.TrimSpace(info.HandleID); handleID != "" {
		view.HandleID = handleID
	}
	if info.State != "" {
		view.State = info.State
	}
	return view, nil
}

func (s *Service) withRuntimeAvailability(a Agent) Agent {
	if !isGatewayRuntimeKind(strings.TrimSpace(a.RuntimeKind)) ||
		!strings.EqualFold(strings.TrimSpace(a.Status), string(agentruntime.StateRunning)) {
		a.Availability = &RuntimeAvailability{State: RuntimeAvailabilityNotApplicable}
		return a
	}

	s.mu.RLock()
	availability, ok := s.availability[canonicalAgentID(a.ID)]
	s.mu.RUnlock()
	if !ok {
		a.Availability = &RuntimeAvailability{State: RuntimeAvailabilityUnknown}
		return a
	}
	if availability.handleID != strings.TrimSpace(a.BoxID) {
		a.Availability = &RuntimeAvailability{State: RuntimeAvailabilityUnknown}
		return a
	}
	a.Availability = cloneRuntimeAvailability(&availability)
	return a
}

func (s *Service) recordRuntimeAvailability(a Agent, availability RuntimeAvailability) RuntimeAvailability {
	if s == nil {
		return availability
	}
	agentID := canonicalAgentID(a.ID)
	if agentID == "" {
		return availability
	}
	availability.State = RuntimeAvailabilityState(strings.TrimSpace(string(availability.State)))
	availability.Reason = strings.TrimSpace(availability.Reason)
	availability.handleID = strings.TrimSpace(a.BoxID)
	if availability.CheckedAt.IsZero() {
		availability.CheckedAt = time.Now().UTC()
	} else {
		availability.CheckedAt = availability.CheckedAt.UTC()
	}
	if availability.ExpiresAt.IsZero() {
		availability.ExpiresAt = availability.CheckedAt.Add(runtimeAvailabilityMaxAge)
	} else {
		availability.ExpiresAt = availability.ExpiresAt.UTC()
	}
	s.mu.Lock()
	current, _, exists := s.agentByIDLocked(agentID)
	if !exists || strings.TrimSpace(current.BoxID) != availability.handleID {
		s.mu.Unlock()
		return availability
	}
	s.availability[agentID] = availability
	s.mu.Unlock()
	return availability
}

// clearRuntimeAvailabilityLocked drops the observation associated with an
// agent whose runtime instance is being replaced or removed. The caller must
// hold s.mu for writing.
func (s *Service) clearRuntimeAvailabilityLocked(agentID string) {
	if s == nil {
		return
	}
	delete(s.availability, canonicalAgentID(agentID))
}

func (s *Service) refreshRuntimeAvailability(ctx context.Context, a Agent) Agent {
	a = s.withRuntimeAvailability(a)
	if a.Availability == nil || a.Availability.State == RuntimeAvailabilityNotApplicable {
		return a
	}

	runtimeImpl, err := s.runtimeForKind(strings.TrimSpace(a.RuntimeKind))
	if err != nil {
		availability := RuntimeAvailability{
			State:     RuntimeAvailabilityUnknown,
			CheckedAt: time.Now().UTC(),
			Reason:    "runtime_unavailable",
		}
		availability = s.recordRuntimeAvailability(a, availability)
		a.Availability = cloneRuntimeAvailability(&availability)
		return a
	}
	checker, ok := runtimeImpl.(agentruntime.ReadinessChecker)
	if !ok {
		availability := RuntimeAvailability{
			State:     RuntimeAvailabilityNotApplicable,
			CheckedAt: time.Now().UTC(),
		}
		availability = s.recordRuntimeAvailability(a, availability)
		a.Availability = cloneRuntimeAvailability(&availability)
		return a
	}

	err = checker.CheckReadiness(ctx, runtimeHandleForAgent(a))
	availability := runtimeAvailabilityFromReadinessError(err)
	if err != nil && !errors.Is(err, agentruntime.ErrReadinessNotSupported) {
		slog.Debug("agent readiness probe failed",
			"agent_id", strings.TrimSpace(a.ID),
			"agent_name", strings.TrimSpace(a.Name),
			"error", err,
		)
	}
	availability = s.recordRuntimeAvailability(a, availability)
	a.Availability = cloneRuntimeAvailability(&availability)
	return a
}

func (s *Service) refreshRuntimeAvailabilityWithTimeout(ctx context.Context, a Agent) Agent {
	if ctx == nil {
		ctx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(ctx, runtimeAvailabilityProbeTimeout)
	defer cancel()
	return s.refreshRuntimeAvailability(probeCtx, a)
}

func (s *Service) refreshExpiredRuntimeAvailability(_ context.Context, a Agent) Agent {
	if !s.shouldRefreshRuntimeAvailability(a, time.Now()) {
		return a
	}
	runtimeImpl, err := s.runtimeForKind(strings.TrimSpace(a.RuntimeKind))
	if err != nil {
		return a
	}
	if _, ok := runtimeImpl.(agentruntime.ReadinessChecker); !ok {
		return a
	}
	s.scheduleRuntimeAvailabilityRefresh(a)
	return a
}

func (s *Service) scheduleRuntimeAvailabilityRefresh(a Agent) {
	if s == nil {
		return
	}
	agentID := canonicalAgentID(a.ID)
	s.mu.Lock()
	if s.availabilityProbes == nil {
		s.availabilityProbes = make(map[string]struct{})
	}
	if _, running := s.availabilityProbes[agentID]; running {
		s.mu.Unlock()
		return
	}
	s.availabilityProbes[agentID] = struct{}{}
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.availabilityProbes, agentID)
			s.mu.Unlock()
		}()
		_ = s.refreshRuntimeAvailabilityWithTimeout(context.Background(), a)
	}()
}

// primeUnknownGatewayRuntimeAvailability seeds the ephemeral readiness cache
// after startup without adding readiness probes to the ordinary roster path.
func (s *Service) primeUnknownGatewayRuntimeAvailability(ctx context.Context, agents []Agent) {
	if s == nil || len(agents) == 0 {
		return
	}

	candidates := make([]Agent, 0, len(agents))
	for _, a := range agents {
		availability := s.withRuntimeAvailability(a).Availability
		if availability != nil && availability.State == RuntimeAvailabilityUnknown {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		return
	}

	workers := min(agentListRuntimeProbeConcurrency, len(candidates))
	indices := make(chan int)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for idx := range indices {
				_ = s.refreshRuntimeAvailabilityWithTimeout(ctx, candidates[idx])
			}
		}()
	}
	for idx := range candidates {
		indices <- idx
	}
	close(indices)
	group.Wait()
}

func (s *Service) shouldRefreshRuntimeAvailability(a Agent, now time.Time) bool {
	if s == nil ||
		!isGatewayRuntimeKind(strings.TrimSpace(a.RuntimeKind)) ||
		!strings.EqualFold(strings.TrimSpace(a.Status), string(agentruntime.StateRunning)) {
		return false
	}
	s.mu.RLock()
	availability, ok := s.availability[canonicalAgentID(a.ID)]
	s.mu.RUnlock()
	if !ok || availability.handleID != strings.TrimSpace(a.BoxID) {
		return true
	}
	switch availability.State {
	case RuntimeAvailabilityReady, RuntimeAvailabilityDegraded, RuntimeAvailabilityUnknown:
	default:
		return false
	}
	return availability.ExpiresAt.IsZero() || !now.Before(availability.ExpiresAt)
}

func runtimeAvailabilityFromReadinessError(err error) RuntimeAvailability {
	availability := RuntimeAvailability{CheckedAt: time.Now().UTC()}
	switch {
	case err == nil:
		availability.State = RuntimeAvailabilityReady
	case errors.Is(err, agentruntime.ErrReadinessNotSupported):
		availability.State = RuntimeAvailabilityNotApplicable
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		availability.State = RuntimeAvailabilityUnknown
		availability.Reason = "probe_timed_out"
	case sandbox.IsUnavailable(err):
		availability.State = RuntimeAvailabilityDegraded
		availability.Reason = "control_plane_unavailable"
	case sandbox.IsNotFound(err):
		availability.State = RuntimeAvailabilityUnknown
		availability.Reason = "runtime_not_found"
	default:
		availability.State = RuntimeAvailabilityDegraded
		availability.Reason = "readiness_failed"
	}
	return availability
}

func (s *Service) recordRuntimeAvailabilityFromLifecycleError(a Agent, err error) {
	if s == nil || !isGatewayRuntimeKind(strings.TrimSpace(a.RuntimeKind)) || err == nil {
		return
	}
	availability, ok := runtimeAvailabilityFromLifecycleError(err)
	if !ok {
		return
	}
	_ = s.recordRuntimeAvailability(a, availability)
}

func runtimeAvailabilityFromLifecycleError(err error) (RuntimeAvailability, bool) {
	switch {
	case sandbox.IsUnavailable(err):
		return RuntimeAvailability{State: RuntimeAvailabilityDegraded, Reason: "control_plane_unavailable"}, true
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return RuntimeAvailability{State: RuntimeAvailabilityUnknown, Reason: "probe_timed_out"}, true
	case sandbox.IsBusy(err):
		return RuntimeAvailability{State: RuntimeAvailabilityUnknown, Reason: "runtime_busy"}, true
	case sandbox.IsNotFound(err):
		return RuntimeAvailability{State: RuntimeAvailabilityUnknown, Reason: "runtime_not_found"}, true
	default:
		return RuntimeAvailability{}, false
	}
}

func (s *Service) streamRuntimeHostLogs(ctx context.Context, agentID string, follow bool, lines int, w io.Writer) error {
	got, ok := s.agentSnapshot(agentID)
	if !ok {
		return fmt.Errorf("agent %q not found", strings.TrimSpace(agentID))
	}
	layout, err := s.agentLayout(got.ID, got.RuntimeKind)
	if err != nil {
		return err
	}
	if len(layout.HostLogPaths) == 0 {
		return os.ErrNotExist
	}
	return streamHostGatewayLogPaths(ctx, layout.HostLogPaths, follow, lines, w)
}

func (s *Service) updateRuntimeState(id string, info agentruntime.Info) (Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, key, ok := s.agentByIDLocked(id)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", strings.TrimSpace(id))
	}
	current.RuntimeID = normalizeRuntimeID(current.RuntimeID, current.ID)
	if handleID := strings.TrimSpace(info.HandleID); handleID != "" {
		current.BoxID = handleID
	}
	if info.State != "" {
		current.Status = string(info.State)
	}
	if current.CreatedAt.IsZero() && !info.CreatedAt.IsZero() {
		current.CreatedAt = info.CreatedAt.UTC()
	}
	delete(s.agents, key)
	s.agents[current.ID] = current
	s.clearRuntimeAvailabilityLocked(current.ID)
	s.syncRuntimeRecordLocked(current)
	if err := s.saveLocked(); err != nil {
		return Agent{}, err
	}
	return *cloneAgent(&current), nil
}

func supportsRuntimeLogs(rt agentruntime.Runtime) bool {
	if rt == nil {
		return false
	}
	_, ok := rt.(agentruntime.LogStreamer)
	return ok
}

func (s *Service) createGatewayBox(ctx context.Context, rt sandbox.Runtime, image, name, botID string, profile AgentProfile) (sandbox.Instance, sandbox.Info, error) {
	if testCreateGatewayBoxHook != nil {
		return testCreateGatewayBoxHook(s, ctx, rt, image, name, botID, profile)
	}
	factory, err := s.gatewayBoxFactory()
	if err != nil {
		return nil, sandbox.Info{}, err
	}
	s.setGatewayWorkPhase(gatewayBoxPhaseCreating)
	defer s.setGatewayWorkPhase(gatewayBoxPhaseIdle)
	return factory.CreateGatewayBox(ctx, rt, image, name, botID, runtimeProfileFromAgent(profile))
}

func (s *Service) forceRemoveBox(ctx context.Context, rt sandbox.Runtime, idOrName string) error {
	if testForceRemoveBoxHook != nil {
		return testForceRemoveBoxHook(s, ctx, rt, idOrName)
	}
	if rt == nil {
		return fmt.Errorf("invalid sandbox runtime")
	}
	return rt.Remove(ctx, idOrName, sandbox.RemoveOptions{Force: true})
}

func (s *Service) gatewayCreateSpec(image, name, botID string, profile AgentProfile) (sandbox.CreateSpec, error) {
	factory, err := s.gatewayBoxFactory()
	if err != nil {
		return sandbox.CreateSpec{}, err
	}
	return factory.GatewayCreateSpec(image, name, botID, runtimeProfileFromAgent(profile))
}

func addProfileEnvVars(envVars map[string]string, profileEnv map[string]string) {
	if len(profileEnv) == 0 {
		return
	}
	for key, value := range profileEnv {
		key = strings.TrimSpace(key)
		if key == "" || isReservedSandboxEnvKey(key) {
			continue
		}
		envVars[key] = value
	}
}

func isReservedSandboxEnvKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	if upper == "HOME" || upper == "OPENAI_BASE_URL" || upper == "OPENAI_API_KEY" || upper == "OPENAI_MODEL" {
		return true
	}
	return strings.HasPrefix(upper, "CSGCLAW_") || strings.HasPrefix(upper, "PICOCLAW_")
}

func gatewayStartCommand(debug bool) ([]string, []string) {
	if debug {
		return []string{"sleep"}, []string{"infinity"}
	}
	return []string{"tini"}, []string{"--", "picoclaw", "gateway", "-d"}
}

func ensureAgentProjectsRoot() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve host home dir: %w", err)
	}
	hostProjectsRoot := filepath.Join(homeDir, config.AppDirName, hostProjectsDir)
	if err := os.MkdirAll(hostProjectsRoot, 0o755); err != nil {
		return "", fmt.Errorf("create host projects dir: %w", err)
	}
	return hostProjectsRoot, nil
}

func ProjectsRoot() (string, error) {
	return ensureAgentProjectsRoot()
}

func llmBridgeBaseURL(managerBaseURL, agentID string) string {
	managerBaseURL = strings.TrimRight(strings.TrimSpace(managerBaseURL), "/")
	return managerBaseURL + "/api/v1/agents/" + strings.TrimSpace(agentID) + "/llm"
}

func bridgeLLMEnvVars(llmBaseURL, accessToken, modelID string) map[string]string {
	return map[string]string{
		"CSGCLAW_LLM_BASE_URL": llmBaseURL,
		"CSGCLAW_LLM_API_KEY":  accessToken,
		"CSGCLAW_LLM_MODEL_ID": modelID,
		"OPENAI_BASE_URL":      llmBaseURL,
		"OPENAI_API_KEY":       accessToken,
		"OPENAI_MODEL":         modelID,
	}
}

func runtimeProfileFromAgent(profile AgentProfile) agentruntime.Profile {
	profile = normalizeProfile(profile, profile.Name, profile.Description)
	return (agentruntime.Profile{
		Provider:        strings.TrimSpace(profile.Provider),
		BaseURL:         strings.TrimSpace(profile.BaseURL),
		APIKey:          strings.TrimSpace(profile.APIKey),
		ModelID:         strings.TrimSpace(profile.ModelID),
		ReasoningEffort: strings.TrimSpace(profile.ReasoningEffort),
		Env:             normalizeStringMap(profile.Env),
	}).Normalized()
}

func (s *Service) gatewayRuntimeAgent(h agentruntime.Handle) (Agent, error) {
	runtimeID := normalizeRuntimeID(h.RuntimeID, "")
	if runtimeID == "" {
		return Agent{}, fmt.Errorf("runtime id is required")
	}
	runtimeAliases := runtimeIDLookupAliases(runtimeID)

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, alias := range runtimeAliases {
		if rt, ok := s.runtimeRecords[alias]; ok {
			for _, agentID := range rt.AgentIDs {
				if got, ok := s.agents[agentID]; ok {
					return *cloneAgent(&got), nil
				}
			}
		}
	}
	for _, got := range s.agents {
		if slices.Contains(runtimeAliases, normalizeRuntimeID(got.RuntimeID, got.ID)) || slices.Contains(runtimeAliases, strings.TrimSpace(got.RuntimeID)) {
			return *cloneAgent(&got), nil
		}
	}
	return Agent{}, fmt.Errorf("runtime %q not found", runtimeID)
}
