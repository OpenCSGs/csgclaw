package agents

import (
	"bytes"
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/agentengine/interactionstate"
	"csgclaw/internal/agentengine/registry"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"csgclaw/internal/agentengine/lifecycle"
	"csgclaw/internal/assets"
	"csgclaw/internal/config"
	"csgclaw/internal/identity"
	"csgclaw/internal/knowledgebase"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
	hub "csgclaw/internal/template"
	"csgclaw/internal/utils"
)

const (
	ManagerName          = "manager"
	ManagerParticipantID = "pt-manager"
	ManagerUserID        = "agent-manager"
	managerHostPort      = 18790
	managerGuestPort     = 18790
	managerDebugMode     = true
	hostWorkspaceDir     = "workspace"
	hostProjectsDir      = "projects"
	gatewayLogPoll       = 200 * time.Millisecond
)

const (
	gatewayBoxPhaseIdle uint32 = iota
	gatewayBoxPhasePreparing
	gatewayBoxPhaseCreating
)

const agentListRuntimeProbeConcurrency = 4

const stopRunningSandboxAgentsConcurrency = 4

var (
	stopRunningSandboxAgentsTimeout    = 3 * time.Second
	sandboxShutdownAvailabilityTimeout = time.Second
)

var localIPv4Resolver = localIPv4

var osRemoveAll = os.RemoveAll

// ErrHomeCleanup is returned when an agent's local runtime files cannot be
// removed after its runtime has been stopped. Callers may expose a stable,
// path-free message while retaining the wrapped error for local diagnostics.
var ErrHomeCleanup = errors.New("agent home cleanup failed")

var defaultSandboxProvider sandbox.Provider = unconfiguredSandboxProvider{}
var testDefaultServiceOption ControllerOption

var errDefaultTemplateRuntimeMismatch = errors.New("default template runtime mismatch")

type unconfiguredSandboxProvider struct{}

func (unconfiguredSandboxProvider) Name() string {
	return "unconfigured"
}

func (unconfiguredSandboxProvider) Open(context.Context, string) (sandbox.Runtime, error) {
	return nil, fmt.Errorf("sandbox provider is not configured")
}

func (unconfiguredSandboxProvider) ListImages(context.Context, string) ([]string, error) {
	return []string{}, nil
}

var (
	testEnsureRuntimeHook       func(*Controller, string) (sandbox.Runtime, error)
	testEnsureRuntimeAtHomeHook func(*Controller, string) (sandbox.Runtime, error)
	testGetBoxHook              func(*Controller, context.Context, sandbox.Runtime, string) (sandbox.Instance, error)
	testStartBoxHook            func(*Controller, context.Context, sandbox.Instance) error
	testStopBoxHook             func(*Controller, context.Context, sandbox.Instance, sandbox.StopOptions) error
	testBoxInfoHook             func(*Controller, context.Context, sandbox.Instance) (sandbox.Info, error)
	testCloseBoxHook            func(*Controller, sandbox.Instance) error
	testCloseRuntimeHook        func(*Controller, string, sandbox.Runtime) error
	testCreateBoxHook           func(*Controller, context.Context, sandbox.Runtime, sandbox.CreateSpec) (sandbox.Instance, error)
	testCreateGatewayBoxHook    func(*Controller, context.Context, sandbox.Runtime, string, string, string, AgentProfile) (sandbox.Instance, sandbox.Info, error)
	testForceRemoveBoxHook      func(*Controller, context.Context, sandbox.Runtime, string) error
	testRunBoxCommandHook       func(*Controller, context.Context, sandbox.Instance, string, []string, io.Writer) (int, error)
)

// SetTestHooks installs lightweight hooks for tests that need to bypass runtime/box creation.
func SetTestHooks(
	ensureRuntime func(*Controller, string) (sandbox.Runtime, error),
	createGatewayBox func(*Controller, context.Context, sandbox.Runtime, string, string, string, AgentProfile) (sandbox.Instance, sandbox.Info, error),
) {
	testEnsureRuntimeHook = ensureRuntime
	if ensureRuntime != nil {
		testEnsureRuntimeAtHomeHook = func(s *Controller, _ string) (sandbox.Runtime, error) {
			return ensureRuntime(s, ManagerUserID)
		}
	} else {
		testEnsureRuntimeAtHomeHook = nil
	}
	testCreateGatewayBoxHook = createGatewayBox
}

// ResetTestHooks clears hooks installed via SetTestHooks.
func ResetTestHooks() {
	testEnsureRuntimeHook = nil
	testEnsureRuntimeAtHomeHook = nil
	testGetBoxHook = nil
	testStartBoxHook = nil
	testStopBoxHook = nil
	testBoxInfoHook = nil
	testCloseBoxHook = nil
	testCloseRuntimeHook = nil
	testCreateBoxHook = nil
	testCreateGatewayBoxHook = nil
	testForceRemoveBoxHook = nil
	testRunBoxCommandHook = nil
}

// TestOnlySetSandboxProvider replaces the default sandbox provider for newly
// created services. It returns a restore function for test cleanup.
func TestOnlySetSandboxProvider(provider sandbox.Provider) func() {
	previous := defaultSandboxProvider
	if provider == nil {
		defaultSandboxProvider = unconfiguredSandboxProvider{}
	} else {
		defaultSandboxProvider = provider
	}
	return func() {
		defaultSandboxProvider = previous
	}
}

// TestOnlySetGetBoxHook installs a test hook for box lookup.
func TestOnlySetGetBoxHook(hook func(*Controller, context.Context, sandbox.Runtime, string) (sandbox.Instance, error)) {
	testGetBoxHook = hook
}

// TestOnlySetStartBoxHook installs a test hook for starting a box.
func TestOnlySetStartBoxHook(hook func(*Controller, context.Context, sandbox.Instance) error) {
	testStartBoxHook = hook
}

// TestOnlySetStopBoxHook installs a test hook for stopping a box.
func TestOnlySetStopBoxHook(hook func(*Controller, context.Context, sandbox.Instance, sandbox.StopOptions) error) {
	testStopBoxHook = hook
}

// TestOnlySetBoxInfoHook installs a test hook for reading box info.
func TestOnlySetBoxInfoHook(hook func(*Controller, context.Context, sandbox.Instance) (sandbox.Info, error)) {
	testBoxInfoHook = hook
}

// TestOnlySetRunBoxCommandHook installs a test hook for command execution inside a box.
func TestOnlySetRunBoxCommandHook(hook func(*Controller, context.Context, sandbox.Instance, string, []string, io.Writer) (int, error)) {
	testRunBoxCommandHook = hook
}

func TestOnlySetDefaultControllerOption(opt ControllerOption) func() {
	previous := testDefaultServiceOption
	testDefaultServiceOption = opt
	return func() {
		testDefaultServiceOption = previous
	}
}

type Controller struct {
	resourcesOnce sync.Once
	WorkspaceService
	ModelConfiguration
	Repository
	files        *contract.FileStore
	interactions *interactionstate.Coordinator
	extensions   RuntimeExtensionsController

	server                 config.ServerConfig
	hub                    templateService
	defaultManagerTemplate string
	defaultWorkerTemplate  string
	managerImage           string
	gatewayRuntime         string

	sandbox sandbox.Provider
	// mcpServersMu serializes all MCP server mutations. A catalog batch add
	// first reads the current set before issuing its update, so it must share
	// the same lock as direct PUT/PATCH-style updates to avoid stale snapshots
	// overwriting a concurrent edit.
	mcpServersMu       sync.Mutex
	runtimes           map[string]sandbox.Runtime
	runtimeRegistry    *registry.Registry
	availability       map[string]RuntimeAvailability
	availabilityProbes map[string]struct{}
	// configuredAgentStartupPending is an ephemeral boot orchestration marker.
	// It is deliberately separate from both persisted lifecycle state and
	// runtime readiness: callers use it only to keep a roster fresh while the
	// configured-worker restore pass is still running.
	configuredAgentStartupPending bool
	agentLifecycleMu              sync.Mutex
	lifecycle                     *lifecycle.Coordinator
	startupProfileDetectOff       bool
	connectorCapabilityKey        []byte

	// gatewayWorkPhase is set by createGatewayBox for bootstrap progress logs (best-effort if concurrent).
	gatewayWorkPhase atomic.Uint32
}

type ControllerOption func(*Controller) error

// AttachEngine supplies the shared process-local execution resources at composition.
func (s *Controller) AttachEngine(files *contract.FileStore, interactions *interactionstate.Coordinator, extensions RuntimeExtensionsController) {
	s.bindResources()
	s.ModelConfiguration.Repository = &s.Repository
	s.files = files
	s.interactions = interactions
	s.extensions = extensions
}

type templateService interface {
	List(context.Context) ([]hub.Template, error)
	Get(context.Context, string) (hub.Template, error)
	FetchWorkspace(context.Context, string) (hub.WorkspaceRef, error)
}

func codexMemoryPath(layout agentruntime.Layout, runtimeKind string) string {
	if agentruntime.RuntimeConfigForKind(runtimeKind).LegacyKind() != RuntimeKindCodex || strings.TrimSpace(layout.SkillsRoot) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(layout.SkillsRoot), "memories", "memory_summary.md")
}

func WithSandboxProvider(provider sandbox.Provider) ControllerOption {
	return func(s *Controller) error {
		if provider == nil {
			return fmt.Errorf("sandbox provider is required")
		}
		s.sandbox = provider
		return nil
	}
}

func WithRuntime(rt agentruntime.Runtime) ControllerOption {
	return func(s *Controller) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		if rt == nil {
			return fmt.Errorf("runtime is required")
		}
		kind := strings.TrimSpace(rt.Kind())
		if kind == "" {
			return fmt.Errorf("runtime kind is required")
		}
		if s.runtimeRegistry == nil {
			s.runtimeRegistry = &registry.Registry{}
		}
		return s.runtimeRegistry.Register(rt)
	}
}

func WithHubService(svc *hub.Service) ControllerOption {
	return func(s *Controller) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		s.hub = svc
		return nil
	}
}

// SetHubService replaces the default template service used by operations that
// do not carry an HTTP request-scoped Hub.
func (s *Controller) SetHubService(hubSvc *hub.Service) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.hub = hubSvc
	s.mu.Unlock()
}

func WithBootstrapDefaultTemplates(cfg config.BootstrapConfig) ControllerOption {
	return func(s *Controller) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		s.defaultManagerTemplate = strings.TrimSpace(cfg.ResolvedDefaultManagerTemplate())
		s.defaultWorkerTemplate = strings.TrimSpace(cfg.ResolvedDefaultWorkerTemplate())
		return nil
	}
}

func WithStartupProfileDetectionDisabled() ControllerOption {
	return func(s *Controller) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		s.SetStartupProfileDetectionDisabled(true)
		return nil
	}
}

func (s *Controller) SetStartupProfileDetectionDisabled(disabled bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.startupProfileDetectOff = disabled
	s.mu.Unlock()
}

// WithGatewayRuntime sets picoclaw vs openclaw gateway behavior (from [bootstrap] or image inference).
func WithGatewayRuntime(runtime string) ControllerOption {
	return func(s *Controller) error {
		kind := runtimeKindForGatewayRuntime(runtime)
		if kind == "" {
			return fmt.Errorf("gateway runtime %q is not supported", runtime)
		}
		s.gatewayRuntime = kind
		return nil
	}
}

func (s *Controller) GatewayRuntime() string {
	if s == nil {
		return RuntimeKindPicoClawSandbox
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if kind := runtimeKindForGatewayRuntime(s.gatewayRuntime); kind != "" {
		return kind
	}
	return RuntimeKindPicoClawSandbox
}

func (s *Controller) SetGatewayRuntime(runtime, managerImage string) error {
	if s == nil {
		return fmt.Errorf("agent service is required")
	}
	kind := runtimeKindForGatewayRuntime(runtime)
	if kind == "" {
		return fmt.Errorf("gateway runtime %q is not supported", runtime)
	}
	managerImage = strings.TrimSpace(managerImage)
	if kind != s.gatewayRuntimeKind() && managerImage == "" {
		return fmt.Errorf("image is required when changing gateway runtime_kind to %q", kind)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gatewayRuntime = kind
	if managerImage != "" {
		s.managerImage = managerImage
	}
	return nil
}

func NewController(model config.ModelConfig, server config.ServerConfig, managerImage, statePath string, opts ...ControllerOption) (*Controller, error) {
	return NewControllerWithLLM(config.SingleProfileLLM(model), server, managerImage, statePath, opts...)
}

func NewControllerWithLLM(llmCfg config.LLMConfig, server config.ServerConfig, managerImage, statePath string, opts ...ControllerOption) (*Controller, error) {
	// agent.Service owns the persisted registry and runtime selection.
	defaultProfile, model, err := llmCfg.Resolve("")
	if err != nil {
		defaultProfile = strings.TrimSpace(llmCfg.Normalized().Default)
		if defaultProfile == "" {
			defaultProfile = strings.TrimSpace(llmCfg.Normalized().DefaultProfile)
		}
		model = config.ModelConfig{}.Resolved()
	}
	connectorCapabilityKey, err := newConnectorCapabilityKey()
	if err != nil {
		return nil, fmt.Errorf("generate connector capability key: %w", err)
	}
	svc := &Controller{

		server:       server,
		managerImage: managerImage,

		sandbox:  defaultSandboxProvider,
		runtimes: make(map[string]sandbox.Runtime),

		runtimeRegistry:    &registry.Registry{},
		availability:       make(map[string]RuntimeAvailability),
		availabilityProbes: make(map[string]struct{}),

		connectorCapabilityKey: connectorCapabilityKey, Repository: Repository{state: statePath,

			agents:         make(map[string]Agent),
			runtimeRecords: make(map[string]RuntimeRecord),

			profileDefaults: profileFromConfigModel("", "", model)}, ModelConfiguration: ModelConfiguration{model: model,
			llm: llmCfg.Normalized()}, WorkspaceService: WorkspaceService{agentsRoot: serviceAgentsRoot(statePath)},
	}
	svc.ModelConfiguration.Repository = &svc.Repository
	svc.bindResources()
	if testDefaultServiceOption != nil {
		if err := testDefaultServiceOption(svc); err != nil {
			return nil, err
		}
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(svc); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(svc.llm.DefaultProfile) == "" {
		svc.llm.DefaultProfile = defaultProfile
	}
	svc.normalizeProfileReference = profileCatalogNormalizer(llmCfg)
	if err := svc.Reload(); err != nil {
		return nil, err
	}
	return svc, nil
}

func EnsureBootstrapState(ctx context.Context, statePath string, server config.ServerConfig, model config.ModelConfig, managerImage string, forceRecreate bool) error {
	return EnsureBootstrapStateWithLLM(ctx, statePath, server, config.SingleProfileLLM(model), managerImage, forceRecreate)
}

func EnsureBootstrapStateWithLLM(ctx context.Context, statePath string, server config.ServerConfig, llmCfg config.LLMConfig, managerImage string, forceRecreate bool) error {
	svc, err := NewControllerWithLLM(llmCfg, server, managerImage, statePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = svc.Close()
	}()
	return svc.EnsureBootstrapManager(ctx, forceRecreate)
}

func (svc *Controller) EnsureBootstrapManager(ctx context.Context, forceRecreate bool) error {
	if svc == nil {
		return nil
	}
	_, err := svc.EnsureManager(ctx, forceRecreate)
	return err
}

func (s *Controller) logBootstrapManagerBoxProgress(elapsed time.Duration) {
	wait := elapsed.Round(time.Second).String()
	ph := s.gatewayWorkPhase.Load()
	switch ph {
	case gatewayBoxPhasePreparing:
		log.Printf(`still in stage "preparing" for bootstrap manager %q [%s elapsed]: host filesystem + gateway config/skills mounts (no registry pull yet)`, ManagerName, wait)
	case gatewayBoxPhaseCreating:
		log.Printf(`still in stage "creating" for manager %q [%s elapsed]: boxlite provisioning the sandbox (unpack layers if needed, disk/VM shim, boot, then CMD)`, ManagerName, wait)
	default:
		log.Printf(`still working on bootstrap manager %q [%s elapsed], image=%q`, ManagerName, wait, s.managerImage)
	}
}

func (s *Controller) EnsureManager(ctx context.Context, forceRecreate bool) (Agent, error) {
	return s.ensureManager(ctx, forceRecreate, "")
}

func (s *Controller) ensureManager(ctx context.Context, forceRecreate bool, runtimeOverride string) (Agent, error) {
	if s == nil {
		return Agent{}, fmt.Errorf("agent service is required")
	}
	ctx, release, err := s.acquireAgentLifecycle(ctx, ManagerUserID)
	if err != nil {
		return Agent{}, err
	}
	defer release()
	if err := validateCodexManagerRuntimeOverride(runtimeOverride); err != nil {
		return Agent{}, err
	}
	return s.ensureCodexManager(ctx, forceRecreate)
}

func validateCodexManagerRuntimeOverride(runtimeOverride string) error {
	runtimeOverride = strings.TrimSpace(runtimeOverride)
	if runtimeOverride == "" {
		return nil
	}
	cfg := agentruntime.RuntimeConfigForKind(runtimeOverride)
	if cfg.LegacyKind() == RuntimeKindCodex && cfg.Name == RuntimeNameCodex && !cfg.Sandboxed {
		return nil
	}
	return fmt.Errorf("manager runtime is fixed to codex")
}

func (s *Controller) ensureCodexManager(ctx context.Context, forceRecreate bool) (Agent, error) {
	managerDisplayName, managerDescription, managerInstructions, managerAvatar, managerCreatedAt := s.managerMetadata()
	managerMCPServers := s.managerMCPServers()
	startProfile, detectionResults := s.managerStartupProfile(ctx)
	startProfile = normalizeProfile(startProfile, managerDisplayName, managerDescription)

	if !startProfile.ProfileComplete {
		manager := s.newCodexManagerAgent(managerDisplayName, managerDescription, managerInstructions, managerAvatar, managerCreatedAt, "", agentruntime.StateUnknown, "profile_incomplete", startProfile, detectionResults)
		applyManagerMCPServers(&manager, managerMCPServers)
		manager.ProfileComplete = false
		return s.persistManagerAgent(ctx, manager, false)
	}

	if err := s.checkRuntimeAvailability(ctx, RuntimeKindCodex); err != nil {
		manager := s.newCodexManagerAgent(managerDisplayName, managerDescription, managerInstructions, managerAvatar, managerCreatedAt, "", agentruntime.StateUnknown, StatusRuntimeUnavailable, startProfile, detectionResults)
		applyManagerMCPServers(&manager, managerMCPServers)
		return s.persistManagerAgent(ctx, manager, false)
	}

	runtimeImpl, err := s.runtimeForKind(RuntimeKindCodex)
	if err != nil {
		manager := s.newCodexManagerAgent(managerDisplayName, managerDescription, managerInstructions, managerAvatar, managerCreatedAt, "", agentruntime.StateUnknown, StatusRuntimeUnavailable, startProfile, detectionResults)
		applyManagerMCPServers(&manager, managerMCPServers)
		return s.persistManagerAgent(ctx, manager, false)
	}

	existing, _ := s.Agent(ManagerUserID)
	managerCredentials, managerInitShell := existing.RuntimeProvision()
	managerRuntimeOptions := utils.CloneAnyMap(existing.RuntimeOptions)
	if err := s.validateMCPServers(ctx, RuntimeKindCodex, mcpServersSnapshotForAgent(managerMCPServers)); err != nil {
		return Agent{}, err
	}
	runtimeAgent := s.newCodexManagerAgent(managerDisplayName, managerDescription, managerInstructions, managerAvatar, managerCreatedAt, "", agentruntime.StateCreated, string(agentruntime.StateCreated), startProfile, detectionResults)
	runtimeAgent.RuntimeOptions = utils.CloneAnyMap(managerRuntimeOptions)
	applyManagerMCPServers(&runtimeAgent, managerMCPServers)
	runtimeAgent.SetRuntimeProvision(managerCredentials, managerInitShell)
	runtimeProfile := s.runtimeProfileForAgentWithProfile(runtimeAgent, s.hydrateProfileFromCatalog(startProfile))
	provisionReq := agentruntime.ProvisionRequest{
		RuntimeID:           runtimeIDForAgentID(ManagerUserID),
		AgentID:             ManagerUserID,
		ParticipantID:       ManagerParticipantID,
		AgentName:           managerDisplayName,
		Profile:             runtimeProfile,
		RuntimeOptions:      utils.CloneAnyMap(managerRuntimeOptions),
		MCPServers:          cloneMCPServers(runtimeAgent.MCPServers),
		Credentials:         managerCredentials,
		PreviousCredentials: sortedStringKeys(managerCredentials),
		InitShell:           managerInitShell,
	}
	if err := s.provisionRuntime(ctx, runtimeImpl, RuntimeKindCodex, provisionReq); err != nil {
		return Agent{}, fmt.Errorf("provision manager runtime: %w", err)
	}
	if forceRecreate {
		// The Agent mutation lease has drained active Engine Turns.
		if strings.TrimSpace(existing.RuntimeID) != "" {
			if err := runtimeImpl.Delete(ctx, runtimeHandleForAgent(existing)); err != nil && !sandbox.IsNotFound(err) {
				return Agent{}, fmt.Errorf("remove existing manager runtime: %w", err)
			}
		}
	}
	if _, err := s.persistManagerAgent(ctx, runtimeAgent, false); err != nil {
		return Agent{}, err
	}

	if err := s.prepareExtensions(ctx, ManagerUserID); err != nil {
		return Agent{}, err
	}
	handle, err := runtimeImpl.New(ctx, agentruntime.Spec{
		RuntimeID: runtimeIDForAgentID(ManagerUserID),
		AgentID:   ManagerUserID,
		AgentName: managerDisplayName,
		Image:     "",
		Profile:   runtimeProfile,
	})
	if err != nil {
		return Agent{}, fmt.Errorf("create manager runtime: %w", err)
	}
	if err := s.observeStartedExtensions(ctx, ManagerUserID); err != nil {
		return Agent{}, err
	}
	info, err := s.runtimeInfo(ctx, runtimeImpl, handle)
	if err != nil {
		return Agent{}, fmt.Errorf("read manager runtime info: %w", err)
	}
	if strings.TrimSpace(info.HandleID) == "" {
		info.HandleID = strings.TrimSpace(handle.HandleID)
	}
	if info.State == "" {
		info.State = agentruntime.StateRunning
	}
	manager := s.newCodexManagerAgent(managerDisplayName, managerDescription, managerInstructions, managerAvatar, managerCreatedAt, info.HandleID, info.State, string(info.State), startProfile, detectionResults)
	manager.RuntimeOptions = utils.CloneAnyMap(managerRuntimeOptions)
	applyManagerMCPServers(&manager, managerMCPServers)
	manager.SetRuntimeProvision(managerCredentials, managerInitShell)
	manager.AgentProfile.EnvRestartRequired = false
	manager.AgentProfile.ImageUpgradeRequired = false
	if !info.CreatedAt.IsZero() {
		manager.CreatedAt = info.CreatedAt.UTC()
	}
	persisted, err := s.persistManagerAgent(ctx, manager, true)
	if err != nil {
		return Agent{}, err
	}
	if !reflect.DeepEqual(managerMCPServers, persisted.MCPServers) {
		if err := s.reconcileMCPServers(ctx, manager, persisted); err != nil {
			return Agent{}, err
		}
	}
	return persisted, nil
}

func (s *Controller) managerMetadata() (name, description, instructions, avatar string, createdAt time.Time) {
	name = ManagerName
	avatar = assets.DefaultManagerAvatar
	if s == nil {
		return name, "", "", avatar, time.Time{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if existing, ok := s.agents[ManagerUserID]; ok {
		return managerMetadataFromAgent(existing)
	}
	for _, existing := range s.agents {
		if isManagerAgent(existing) {
			return managerMetadataFromAgent(existing)
		}
	}
	return name, "", "", avatar, time.Time{}
}

func managerMetadataFromAgent(existing Agent) (name, description, instructions, avatar string, createdAt time.Time) {
	name = strings.TrimSpace(existing.Name)
	if name == "" {
		name = ManagerName
	}
	avatar = strings.TrimSpace(existing.Avatar)
	avatar = assets.NormalizeManagerAvatar(avatar)
	return name, strings.TrimSpace(existing.Description), strings.TrimSpace(existing.Instructions), avatar, existing.CreatedAt.UTC()
}

func (s *Controller) managerMCPServers() map[string]any {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if existing, ok := s.agents[ManagerUserID]; ok {
		return cloneMCPServers(existing.MCPServers)
	}
	for _, existing := range s.agents {
		if isManagerAgent(existing) {
			return cloneMCPServers(existing.MCPServers)
		}
	}
	return nil
}

func applyManagerMCPServers(manager *Agent, mcpServers map[string]any) {
	if manager == nil {
		return
	}
	manager.MCPServers = cloneMCPServers(mcpServers)
}

func (s *Controller) newCodexManagerAgent(name, description, instructions, avatar string, createdAt time.Time, handleID string, state agentruntime.State, status string, profile AgentProfile, detectionResults []ProfileDetectionResult) Agent {
	now := time.Now().UTC()
	if createdAt.IsZero() {
		createdAt = now
	}
	if status == "" {
		status = string(state)
	}
	if status == "" {
		status = string(agentruntime.StateRunning)
	}
	prof := cloneProfile(profile)
	return Agent{
		ID:               ManagerUserID,
		Name:             strings.TrimSpace(name),
		Description:      strings.TrimSpace(description),
		Instructions:     strings.TrimSpace(instructions),
		RuntimeID:        runtimeIDForAgentID(ManagerUserID),
		RuntimeKind:      RuntimeKindCodex,
		RuntimeName:      RuntimeNameCodex,
		SandboxEnabled:   false,
		Image:            "",
		Avatar:           strings.TrimSpace(avatar),
		BoxID:            strings.TrimSpace(handleID),
		Role:             RoleManager,
		Status:           status,
		CreatedAt:        createdAt,
		UpdatedAt:        now,
		Profile:          profileSelector(prof),
		AgentProfile:     prof,
		ProfileComplete:  prof.ProfileComplete,
		DetectionResults: append([]ProfileDetectionResult(nil), detectionResults...),
	}
}

func (s *Controller) persistManagerAgent(ctx context.Context, manager Agent, runtimeApplied bool) (Agent, error) {
	if s == nil {
		return Agent{}, fmt.Errorf("agent service is required")
	}
	manager.ID = ManagerUserID
	manager.Role = RoleManager
	manager.RuntimeID = runtimeIDForAgentID(ManagerUserID)
	manager.RuntimeKind = RuntimeKindCodex
	manager.RuntimeName = RuntimeNameCodex
	manager.SandboxEnabled = false
	manager.Image = ""
	manager.RuntimeOptions = utils.CloneAnyMap(manager.RuntimeOptions)
	if strings.TrimSpace(manager.Name) == "" {
		manager.Name = ManagerName
	}
	now := time.Now().UTC()
	if manager.CreatedAt.IsZero() {
		manager.CreatedAt = now
	}
	manager.UpdatedAt = now
	manager.AgentProfile = cloneProfile(manager.AgentProfile)
	manager.Profile = profileSelector(manager.AgentProfile)
	manager.ProfileComplete = manager.AgentProfile.ProfileComplete
	manager.DetectionResults = append([]ProfileDetectionResult(nil), manager.DetectionResults...)

	s.mu.Lock()
	if existing, ok := s.agents[ManagerUserID]; ok {
		if manager.RuntimeOptions == nil {
			manager.RuntimeOptions = utils.CloneAnyMap(existing.RuntimeOptions)
		}
		// A successful runtime start clears requirements already applied to that
		// runtime, but a concurrent config change must keep its recreate signal.
		runtimeInputsChangedAfterApply := runtimeApplied && (!reflect.DeepEqual(manager.MCPServers, existing.MCPServers) ||
			!reflect.DeepEqual(
				runtimeConfigSnapshotForAgent(manager.AgentProfile, manager.RuntimeOptions),
				runtimeConfigSnapshotForAgent(existing.AgentProfile, existing.RuntimeOptions),
			) ||
			!profilesEqualEnv(manager.AgentProfile, existing.AgentProfile))
		manager.MCPServers = cloneMCPServers(existing.MCPServers)
		if existing.AgentProfile.EnvRestartRequired && (!runtimeApplied || runtimeInputsChangedAfterApply) {
			manager.AgentProfile.EnvRestartRequired = true
		}
		if existing.AgentProfile.ImageUpgradeRequired && !runtimeApplied {
			manager.AgentProfile.ImageUpgradeRequired = true
		}
	}
	for id, existing := range s.agents {
		if isManagerAgent(existing) && id != ManagerUserID {
			delete(s.agents, id)
		}
	}
	s.putAgentLocked(ManagerUserID, manager)
	s.syncRuntimeRecordLocked(manager)
	if manager.AgentProfile.ProfileComplete {
		s.profileDefaults = profileDefaultsSnapshot(manager.AgentProfile)
	}
	s.detectionResults = append([]ProfileDetectionResult(nil), manager.DetectionResults...)
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return Agent{}, err
	}
	created, ok := s.Agent(ManagerUserID)
	if !ok {
		return Agent{}, fmt.Errorf("manager agent not found after save")
	}
	_ = runtimeApplied
	return created, nil
}

func (s *Controller) cleanupBootstrapManagerForRecreate(ctx context.Context, rt sandbox.Runtime, runtimeHome, runtimeKind string) (sandbox.Runtime, error) {
	log.Printf("force recreating bootstrap manager box %q", ManagerName)
	removed := false
	for _, managerBoxIDOrName := range s.bootstrapManagerLookupKeys() {
		if err := s.forceRemoveBox(ctx, rt, managerBoxIDOrName); err != nil {
			if sandbox.IsNotFound(err) {
				log.Printf("bootstrap manager box %q (%q) does not exist yet; continuing", ManagerName, managerBoxIDOrName)
				continue
			}
			return rt, fmt.Errorf("force remove bootstrap manager box %q (%q): %w", ManagerName, managerBoxIDOrName, err)
		}
		log.Printf("bootstrap manager box %q (%q) removed", ManagerName, managerBoxIDOrName)
		removed = true
		break
	}
	if !removed {
		log.Printf("bootstrap manager box %q not found under known identifiers; continuing", ManagerName)
	}
	if err := s.closeRuntime(runtimeHome, rt); err != nil {
		return rt, fmt.Errorf("close bootstrap manager runtime before recreate: %w", err)
	}
	rt = nil
	managerHome, err := s.agentHomeDir(ManagerUserID)
	if err != nil {
		return nil, err
	}
	sourceRuntimeKind := s.managerSkillPreservationSourceRuntimeKind(runtimeKind)
	restoreSkills, cleanupSkills, err := s.prepareWorkspaceSkillsPreservation(ManagerUserID, sourceRuntimeKind, runtimeKind, RoleManager)
	if err != nil {
		return nil, fmt.Errorf("prepare bootstrap manager skills preservation: %w", err)
	}
	if cleanupSkills != nil {
		defer cleanupSkills()
	}
	if err := removeAll(managerHome); err != nil {
		return nil, fmt.Errorf("remove bootstrap manager home: %w", err)
	}
	if restoreSkills != nil {
		if err := restoreSkills(); err != nil {
			return nil, fmt.Errorf("restore bootstrap manager skills: %w", err)
		}
	}
	rt, err = s.ensureRuntimeAtHome(runtimeHome)
	if err != nil {
		return nil, err
	}
	return rt, nil
}

func (s *Controller) managerSkillPreservationSourceRuntimeKind(targetRuntimeKind string) string {
	targetRuntimeKind = strings.TrimSpace(targetRuntimeKind)
	if s == nil {
		return targetRuntimeKind
	}
	s.mu.RLock()
	existing := s.agents[ManagerUserID]
	s.mu.RUnlock()
	if source := strings.TrimSpace(existing.RuntimeKind); isGatewayRuntimeKind(source) {
		return source
	}
	return targetRuntimeKind
}

func (s *Controller) managerStartupProfile(ctx context.Context) (AgentProfile, []ProfileDetectionResult) {
	s.mu.RLock()
	if existing, ok := s.agents[ManagerUserID]; ok && existing.AgentProfile.ProfileComplete {
		name := strings.TrimSpace(existing.Name)
		if name == "" {
			name = ManagerName
		}
		profile := cloneProfile(existing.AgentProfile)
		results := append([]ProfileDetectionResult(nil), existing.DetectionResults...)
		s.mu.RUnlock()
		return normalizeProfile(profile, name, existing.Description), results
	}
	if s != nil && s.startupProfileDetectOff {
		if existing, ok := s.agents[ManagerUserID]; ok {
			name := strings.TrimSpace(existing.Name)
			if name == "" {
				name = ManagerName
			}
			profile := cloneProfile(existing.AgentProfile)
			results := append([]ProfileDetectionResult(nil), existing.DetectionResults...)
			s.mu.RUnlock()
			return normalizeProfile(profile, name, existing.Description), results
		}
		s.mu.RUnlock()
		return normalizeProfile(AgentProfile{Name: ManagerName, Provider: ProviderCSGHubLite}, ManagerName, ""), nil
	}
	s.mu.RUnlock()
	if s != nil {
		if profileName, model, err := s.llm.Resolve(""); err == nil {
			profile := profileFromConfigModel(profileName, "", model)
			profile.Name = ManagerName
			profile = normalizeProfile(profile, ManagerName, "")
			if profile.ProfileComplete {
				return profile, []ProfileDetectionResult{{
					Provider: profile.Provider,
					Status:   "ok",
					ModelID:  profile.ModelID,
				}}
			}
		}
	}
	return s.DetectDefaultProfile(ctx)
}

func (s *Controller) bootstrapManagerBoxIDOrName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, a := range s.agents {
		if !isManagerAgent(a) {
			continue
		}
		if boxID := strings.TrimSpace(a.BoxID); boxID != "" {
			return boxID
		}
	}
	return ManagerName
}

func (s *Repository) syncRuntimeRecordLocked(a Agent) {
	if s == nil {
		return
	}
	rt := runtimeRecordForAgent(a)
	if rt.ID == "" {
		return
	}
	s.runtimeRecords[rt.ID] = rt
}

func (s *Repository) deleteRuntimeRecordLocked(runtimeID string) {
	if s == nil {
		return
	}
	delete(s.runtimeRecords, normalizeRuntimeID(runtimeID, ""))
}

func (s *Controller) bootstrapManagerLookupKeys() []string {
	primary := s.bootstrapManagerBoxIDOrName()
	keys := make([]string, 0, 3)
	if primary != ManagerName {
		keys = appendLookupKey(keys, primary)
	}
	for _, key := range []string{sandboxNameForAgentID(ManagerUserID), ManagerName} {
		keys = appendLookupKey(keys, key)
	}
	return keys
}

func appendLookupKey(keys []string, key string) []string {
	key = strings.TrimSpace(key)
	if key == "" || slices.Contains(keys, key) {
		return keys
	}
	return append(keys, key)
}

func (s *Controller) CreateRecord(ctx context.Context, req CreateRequest) (Agent, error) {
	if req.Replace && strings.TrimSpace(req.Spec.FromTemplate) != "" {
		return Agent{}, fmt.Errorf("agent create --replace does not support from_template")
	}
	if isManagerCreateSpec(req.Spec) {
		if err := validateManagerRuntimeSpec(req.Spec); err != nil {
			return Agent{}, err
		}
		if err := validateRuntimeOptionsWithoutMCP(req.Spec.RuntimeOptions); err != nil {
			return Agent{}, err
		}
		if !req.Replace && createSpecSetsMCPServers(req.Spec) {
			return Agent{}, fmt.Errorf("manager mcpServers must be updated through the MCP servers endpoint")
		}
	}
	if req.Replace {
		return s.replace(ctx, req)
	}
	if shouldResolveTemplateCreateSpec(req.Spec) {
		var cleanup func()
		var err error
		var hubSvc templateService
		s.mu.RLock()
		defaultHubSvc := s.hub
		s.mu.RUnlock()
		if defaultHubSvc != nil {
			hubSvc = defaultHubSvc
		}
		if req.HubService != nil {
			hubSvc = req.HubService
		}
		req.Spec, cleanup, err = s.resolveTemplateCreateSpecWithService(ctx, req.Spec, hubSvc)
		if err != nil {
			return Agent{}, err
		}
		if cleanup != nil {
			defer cleanup()
		}
	}
	return s.createNew(ctx, req.Spec)
}

func (s *Controller) resolveTemplateCreateSpec(ctx context.Context, spec CreateAgentSpec) (CreateAgentSpec, func(), error) {
	s.mu.RLock()
	hubSvc := s.hub
	s.mu.RUnlock()
	return s.resolveTemplateCreateSpecWithService(ctx, spec, hubSvc)
}

func (s *Controller) resolveTemplateCreateSpecWithService(
	ctx context.Context,
	spec CreateAgentSpec,
	hubSvc templateService,
) (CreateAgentSpec, func(), error) {
	if s == nil {
		return CreateAgentSpec{}, nil, fmt.Errorf("agent service is required")
	}
	templateRef, expectedRole, usedDefault := s.templateRefForCreateSpec(spec)
	if templateRef == "" {
		return spec, nil, nil
	}
	if hubSvc == nil {
		if usedDefault {
			return CreateAgentSpec{}, nil, fmt.Errorf("default %s template %q requires hub service, but hub service is not configured", expectedRole, templateRef)
		}
		return CreateAgentSpec{}, nil, fmt.Errorf("hub service is not configured")
	}

	item, err := hubSvc.Get(ctx, templateRef)
	if err != nil {
		if usedDefault {
			return CreateAgentSpec{}, nil, fmt.Errorf("resolve default %s template %q: %w", expectedRole, templateRef, err)
		}
		return CreateAgentSpec{}, nil, err
	}
	if usedDefault {
		if err := validateDefaultTemplateCompatibility(expectedRole, spec, item, templateRef); err != nil {
			if errors.Is(err, errDefaultTemplateRuntimeMismatch) {
				return spec, nil, nil
			}
			return CreateAgentSpec{}, nil, err
		}
	}
	workspace, err := hubSvc.FetchWorkspace(ctx, templateRef)
	if err != nil {
		if usedDefault {
			return CreateAgentSpec{}, nil, fmt.Errorf("fetch default %s template workspace %q: %w", expectedRole, templateRef, err)
		}
		return CreateAgentSpec{}, nil, err
	}
	if strings.TrimSpace(workspace.Path) != "" && agentruntime.RuntimeConfigForKind(item.RuntimeKind).LegacyKind() == RuntimeKindPicoClawSandbox {
		agentsPath := filepath.Join(workspace.Path, "AGENTS.md")
		if _, statErr := os.Stat(agentsPath); statErr == nil {
			if err := os.Rename(agentsPath, filepath.Join(workspace.Path, "AGENT.md")); err != nil {
				return CreateAgentSpec{}, templateWorkspaceCleanup(item.Source.Kind, workspace), fmt.Errorf("adapt template instructions for picoclaw: %w", err)
			}
		}
		memoryPath := filepath.Join(workspace.Path, "MEMORY.md")
		if _, statErr := os.Stat(memoryPath); statErr == nil {
			if err := os.MkdirAll(filepath.Join(workspace.Path, "memory"), 0o755); err != nil {
				return CreateAgentSpec{}, templateWorkspaceCleanup(item.Source.Kind, workspace), err
			}
			if err := os.Rename(memoryPath, filepath.Join(workspace.Path, "memory", "MEMORY.md")); err != nil {
				return CreateAgentSpec{}, templateWorkspaceCleanup(item.Source.Kind, workspace), err
			}
		}
	}

	cleanup := templateWorkspaceCleanup(item.Source.Kind, workspace)
	spec = applyTemplateDefaults(spec, item)
	spec = applyTemplateEnvDefaults(spec, item)
	if strings.TrimSpace(workspace.Kind) == hub.WorkspaceKindDir {
		if agentruntime.RuntimeConfigForKind(item.RuntimeKind).LegacyKind() == RuntimeKindCodex {
			// Template creation seeds only the template/base document. Profile
			// instructions are introduced later through the managed block.
			spec.Instructions = ""
			instructionsPath := filepath.Join(workspace.Path, "AGENTS.md")
			if data, readErr := os.ReadFile(instructionsPath); readErr == nil {
				spec.TemplateInstructions = string(data)
				if removeErr := os.Remove(instructionsPath); removeErr != nil {
					return CreateAgentSpec{}, cleanup, fmt.Errorf("separate codex template instructions: %w", removeErr)
				}
			} else if !errors.Is(readErr, os.ErrNotExist) {
				return CreateAgentSpec{}, cleanup, fmt.Errorf("read codex template instructions: %w", readErr)
			}
			if memoryPath := strings.TrimSpace(workspace.MemoryPath); memoryPath != "" {
				memoryPath, pathErr := validatedCodexTemplateMemoryPath(workspace.Path, memoryPath)
				if pathErr != nil {
					return CreateAgentSpec{}, cleanup, pathErr
				}
				if spec.RuntimeOptions[templateMemoryModeKey] != templateMemoryModeDisabled {
					data, readErr := os.ReadFile(memoryPath)
					if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
						return CreateAgentSpec{}, cleanup, fmt.Errorf("read codex template memory: %w", readErr)
					}
					if readErr == nil {
						spec.TemplateMemory = string(data)
						spec.TemplateMemorySet = true
					}
				}
				if removeErr := os.RemoveAll(filepath.Dir(memoryPath)); removeErr != nil {
					return CreateAgentSpec{}, cleanup, fmt.Errorf("remove staged codex template memory: %w", removeErr)
				}
			}
		}
		spec.FromTemplate = strings.TrimSpace(workspace.Path)
		if !createSpecSetsMCPServers(spec) && strings.TrimSpace(workspace.MCPServersJSON) != "" {
			var templateMCPServers map[string]any
			if err := json.Unmarshal([]byte(workspace.MCPServersJSON), &templateMCPServers); err != nil {
				return CreateAgentSpec{}, cleanup, fmt.Errorf("decode template mcp servers: %w", err)
			}
			hydratedMCPServers, err := knowledgebase.HydrateTemplateServers(ctx, templateMCPServers)
			if err != nil {
				return CreateAgentSpec{}, cleanup, err
			}
			spec.MCPServers = hydratedMCPServers
			spec.MCPServersSet = true
		}
	}
	return spec, cleanup, nil
}

func validatedCodexTemplateMemoryPath(workspaceRoot, memoryPath string) (string, error) {
	workspaceRoot, err := filepath.Abs(strings.TrimSpace(workspaceRoot))
	if err != nil {
		return "", fmt.Errorf("resolve template workspace path: %w", err)
	}
	memoryPath, err = filepath.Abs(strings.TrimSpace(memoryPath))
	if err != nil {
		return "", fmt.Errorf("resolve codex template memory path: %w", err)
	}
	rel, err := filepath.Rel(workspaceRoot, memoryPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		filepath.Base(filepath.Dir(memoryPath)) != ".csgclaw-template-memory" || filepath.Base(memoryPath) != "memory_summary.md" {
		return "", fmt.Errorf("invalid staged codex template memory path %q", memoryPath)
	}
	return memoryPath, nil
}

func shouldResolveTemplateCreateSpec(spec CreateAgentSpec) bool {
	if strings.TrimSpace(spec.FromTemplate) != "" {
		return true
	}
	return shouldCreateWorkerSpec(spec)
}

func (s *Controller) templateRefForCreateSpec(spec CreateAgentSpec) (templateRef, role string, usedDefault bool) {
	if explicit := strings.TrimSpace(spec.FromTemplate); explicit != "" {
		return explicit, createTemplateRole(spec), false
	}
	role = createTemplateRole(spec)
	switch role {
	case RoleManager:
		return strings.TrimSpace(s.defaultManagerTemplate), role, true
	case RoleWorker:
		return strings.TrimSpace(s.defaultWorkerTemplate), role, true
	default:
		return "", role, false
	}
}

func createTemplateRole(spec CreateAgentSpec) string {
	if isManagerCreateSpec(spec) {
		return RoleManager
	}
	if shouldCreateWorkerSpec(spec) {
		return RoleWorker
	}
	return ""
}

func validateManagerRuntimeSpec(spec CreateAgentSpec) error {
	if !managerRuntimeRequested(spec) {
		return nil
	}
	cfg, err := agentruntime.RuntimeConfigFromSelection(spec.RuntimeKind, spec.RuntimeName, spec.SandboxEnabled)
	if err != nil {
		return err
	}
	if cfg.LegacyKind() == RuntimeKindCodex && cfg.Name == RuntimeNameCodex && !cfg.Sandboxed {
		return nil
	}
	return fmt.Errorf("manager runtime is fixed to codex")
}

func validateDefaultTemplateCompatibility(expectedRole string, spec CreateAgentSpec, item hub.Template, templateRef string) error {
	if actualRole := normalizeRole(item.Role); actualRole != expectedRole {
		if actualRole == "" {
			return fmt.Errorf("default %s template %q does not identify itself as a %s template", expectedRole, templateRef, expectedRole)
		}
		return fmt.Errorf("default %s template %q points to a %s template", expectedRole, templateRef, actualRole)
	}
	requestedRuntime := agentruntime.RuntimeConfigForKind(spec.RuntimeKind).LegacyKind()
	templateRuntime := agentruntime.RuntimeConfigForKind(item.RuntimeKind).LegacyKind()
	if requestedRuntime != "" && templateRuntime != "" && requestedRuntime != templateRuntime {
		return fmt.Errorf("%w: default %s template %q uses runtime_kind %q, incompatible with requested runtime_kind %q", errDefaultTemplateRuntimeMismatch, expectedRole, templateRef, item.RuntimeKind, spec.RuntimeKind)
	}
	return nil
}

func applyTemplateDefaults(spec CreateAgentSpec, item hub.Template) CreateAgentSpec {
	if strings.TrimSpace(spec.Name) == "" {
		spec.Name = item.Name
	}
	if strings.TrimSpace(spec.Description) == "" {
		spec.Description = item.Description
	}
	if strings.TrimSpace(spec.Image) == "" {
		spec.Image = item.Image
	}
	if strings.TrimSpace(spec.RuntimeKind) == "" {
		spec.RuntimeKind = item.RuntimeKind
	}
	if len(item.RuntimeOptions) > 0 {
		options := utils.CloneAnyMap(item.RuntimeOptions)
		for key, value := range spec.RuntimeOptions {
			options[key] = value
		}
		spec.RuntimeOptions = options
	}
	return spec
}

func applyTemplateEnvDefaults(spec CreateAgentSpec, item hub.Template) CreateAgentSpec {
	if len(item.ImageEnv) == 0 {
		return spec
	}
	env := spec.AgentProfile.Env
	if env == nil {
		env = make(map[string]string)
	}
	for _, contract := range item.ImageEnv {
		name := strings.TrimSpace(contract.Name)
		if name == "" {
			continue
		}
		if _, exists := env[name]; exists {
			continue
		}
		if defaultValue := strings.TrimSpace(contract.Default); defaultValue != "" {
			env[name] = defaultValue
		}
	}
	spec.AgentProfile.Env = env
	return spec
}

func templateWorkspaceCleanup(_ string, workspace hub.WorkspaceRef) func() {
	if strings.TrimSpace(workspace.Kind) != hub.WorkspaceKindDir {
		return nil
	}
	if !workspace.Temporary {
		return nil
	}
	path := strings.TrimSpace(workspace.Path)
	if path == "" {
		return nil
	}
	return func() {
		_ = os.RemoveAll(path)
	}
}

func (s *Controller) createNew(ctx context.Context, spec CreateAgentSpec) (Agent, error) {
	if isManagerCreateSpec(spec) {
		return s.EnsureManager(ctx, false)
	}
	if shouldCreateWorkerSpec(spec) {
		spec.Role = RoleWorker
		return s.CreateWorker(ctx, spec)
	}
	return Agent{}, fmt.Errorf("role must be one of %q or %q", RoleManager, RoleWorker)
}

func (s *Controller) replace(ctx context.Context, req CreateRequest) (Agent, error) {
	spec := req.Spec
	managerRuntimeRequested := managerRuntimeRequested(spec)
	id := normalizeCreateID(spec.ID)
	if id == "" {
		return Agent{}, fmt.Errorf("agent create --replace requires id")
	}

	s.mu.RLock()
	existing, _, ok := s.agentByIDLocked(id)
	s.mu.RUnlock()
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}

	if len(req.FieldMask) > 0 {
		var err error
		spec, err = mergeReplaceSpec(existing, spec, req.FieldMask)
		if err != nil {
			return Agent{}, err
		}
	} else {
		spec.ID = existing.ID
		if strings.TrimSpace(spec.Image) == "" {
			spec.Image = existing.Image
		}
		if strings.TrimSpace(spec.Avatar) == "" {
			spec.Avatar = existing.Avatar
		}
		if strings.TrimSpace(spec.RuntimeKind) == "" && strings.TrimSpace(spec.RuntimeName) == "" {
			spec.SetRuntimeConfig(existing.RuntimeConfig())
		}
		if strings.TrimSpace(spec.Role) == "" {
			spec.Role = existing.Role
		}
	}
	runtimeCfg, err := agentruntime.RuntimeConfigFromSelection(spec.RuntimeKind, spec.RuntimeName, spec.SandboxEnabled)
	if err != nil {
		return Agent{}, err
	}
	spec.SetRuntimeConfig(runtimeCfg)

	if isManagerAgent(existing) || isManagerCreateSpec(spec) {
		if err := validateRuntimeOptionsWithoutMCP(spec.RuntimeOptions); err != nil {
			return Agent{}, err
		}
		if managerReplaceSetsMCPServers(req) {
			return Agent{}, fmt.Errorf("manager mcpServers must be updated through the MCP servers endpoint")
		}
		if managerRuntimeRequested {
			if err := validateManagerRuntimeSpec(spec); err != nil {
				return Agent{}, err
			}
			return s.ensureManager(ctx, true, spec.RuntimeKind)
		}
		return s.ensureManager(ctx, true, "")
	}
	if shouldCreateWorkerSpec(spec) || strings.EqualFold(existing.Role, RoleWorker) {
		spec.Role = RoleWorker
		if err := s.validateReplaceWorkerSpecBeforeDelete(ctx, spec); err != nil {
			return Agent{}, err
		}
		return s.createWorker(ctx, spec, true)
	}

	if err := s.DeleteRecord(ctx, existing.ID); err != nil {
		return Agent{}, err
	}
	return s.createNew(ctx, spec)
}

func managerRuntimeRequested(spec CreateAgentSpec) bool {
	return strings.TrimSpace(spec.RuntimeKind) != "" || strings.TrimSpace(spec.RuntimeName) != "" || spec.SandboxEnabled
}

func managerReplaceSetsMCPServers(req CreateRequest) bool {
	if len(req.FieldMask) == 0 {
		return createSpecSetsMCPServers(req.Spec)
	}
	for _, field := range req.FieldMask {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "mcpservers":
			return true
		case "runtime", "runtime_options":
			if createSpecSetsMCPServers(req.Spec) {
				return true
			}
		}
	}
	return false
}

func createSpecSetsMCPServers(spec CreateAgentSpec) bool {
	return spec.MCPServersSet || spec.MCPServers != nil
}

func (s *Controller) validateReplaceWorkerSpecBeforeDelete(ctx context.Context, spec CreateAgentSpec) error {
	runtimeKind := strings.TrimSpace(spec.RuntimeKind)
	switch {
	case runtimeKind == "":
		return fmt.Errorf("runtime_kind is required")
	case isGatewayRuntimeKind(runtimeKind) && strings.TrimSpace(spec.Image) == "":
		return fmt.Errorf("image is required for runtime_kind %q", runtimeKind)
	}
	if _, err := s.runtimeForKind(runtimeKind); err != nil {
		return err
	}
	normalizedMCPServers, err := normalizeMCPServers(spec.MCPServers)
	if err != nil {
		return err
	}
	spec.MCPServers = normalizedMCPServers
	resolvedProfile, err := s.profileForCreateRequest(ctx, &spec)
	if err != nil {
		return err
	}
	if err := s.validateRuntimeConfig(ctx, runtimeKind, runtimeConfigSnapshotForAgent(s.hydrateProfileFromCatalog(resolvedProfile), spec.RuntimeOptions)); err != nil {
		return err
	}
	return s.validateMCPServers(ctx, runtimeKind, mcpServersSnapshotForAgent(spec.MCPServers))
}

func mergeReplaceSpec(existing Agent, next CreateAgentSpec, fieldMask []string) (CreateAgentSpec, error) {
	merged := CreateAgentSpec{
		ID:             existing.ID,
		Name:           existing.Name,
		Description:    existing.Description,
		Instructions:   existing.Instructions,
		Image:          existing.Image,
		Avatar:         existing.Avatar,
		RuntimeKind:    existing.RuntimeKind,
		RuntimeName:    existing.RuntimeName,
		SandboxEnabled: existing.SandboxEnabled,
		Role:           existing.Role,
		Status:         existing.Status,
		CreatedAt:      existing.CreatedAt,
		UpdatedAt:      existing.UpdatedAt,
		Profile:        existing.Profile,
		RuntimeOptions: utils.CloneAnyMap(existing.RuntimeOptions),
		MCPServers:     cloneMCPServers(existing.MCPServers),
		AgentProfile:   cloneProfile(existing.AgentProfile),
	}
	for _, field := range fieldMask {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "", "replace":
		case "id":
			if id := normalizeCreateID(next.ID); id != "" && id != existing.ID {
				return CreateAgentSpec{}, fmt.Errorf("replace id %q does not match existing agent %q", id, existing.ID)
			}
		case "name":
			merged.Name = next.Name
		case "description":
			merged.Description = next.Description
		case "instructions":
			merged.Instructions = next.Instructions
		case "image":
			merged.Image = next.Image
		case "avatar":
			merged.Avatar = next.Avatar
		case "runtime_kind":
			merged.RuntimeKind = next.RuntimeKind
			merged.RuntimeName = next.RuntimeName
			merged.SandboxEnabled = next.SandboxEnabled
		case "runtime_name":
			merged.RuntimeKind = next.RuntimeKind
			merged.RuntimeName = next.RuntimeName
		case "sandbox_enabled":
			merged.RuntimeKind = next.RuntimeKind
			merged.SandboxEnabled = next.SandboxEnabled
		case "runtime":
			merged.RuntimeKind = next.RuntimeKind
			merged.RuntimeName = next.RuntimeName
			merged.SandboxEnabled = next.SandboxEnabled
			merged.RuntimeOptions = utils.CloneAnyMap(next.RuntimeOptions)
		case "role":
			merged.Role = next.Role
		case "status":
			merged.Status = next.Status
		case "created_at":
			merged.CreatedAt = next.CreatedAt
		case "updated_at":
			merged.UpdatedAt = next.UpdatedAt
		case "profile":
			merged.Profile = next.Profile
			if strings.TrimSpace(next.Profile) != "" {
				merged.AgentProfile = AgentProfile{}
			}
		case "agent_profile", "model_config":
			merged.AgentProfile = cloneProfile(next.AgentProfile)
		case "runtime_options":
			merged.RuntimeOptions = utils.CloneAnyMap(next.RuntimeOptions)
		case "mcpservers":
			merged.MCPServers = cloneMCPServers(next.MCPServers)
		default:
			return CreateAgentSpec{}, fmt.Errorf("unsupported agent field mask path %q", field)
		}
	}
	runtimeCfg, err := agentruntime.RuntimeConfigFromSelection(merged.RuntimeKind, merged.RuntimeName, merged.SandboxEnabled)
	if err != nil {
		return CreateAgentSpec{}, err
	}
	merged.SetRuntimeConfig(runtimeCfg)
	return merged, nil
}

func isManagerCreateSpec(spec CreateAgentSpec) bool {
	id := normalizeCreateID(spec.ID)
	name := strings.TrimSpace(spec.Name)
	role := strings.TrimSpace(spec.Role)
	return strings.EqualFold(id, ManagerName) ||
		strings.EqualFold(id, ManagerUserID) ||
		strings.EqualFold(name, ManagerName) ||
		strings.EqualFold(role, RoleManager)
}

func shouldCreateWorkerSpec(spec CreateAgentSpec) bool {
	role := strings.ToLower(strings.TrimSpace(spec.Role))
	return role == "" || role == RoleWorker
}

func normalizeCreateID(id string) string {
	return canonicalAgentID(id)
}

func (s *Controller) Agent(id string) (Agent, bool) {
	a, ok := s.agentSnapshot(id)
	if !ok {
		return Agent{}, false
	}
	ctx := context.Background()
	return s.withConfiguredAgentStartupStatus(s.withRuntimeImageMigrationStatus(ctx, s.hydrateAgentStatus(ctx, a))), true
}

// AgentByName resolves the unique case-insensitive display name used by public
// agent selector APIs.
func (s *Controller) AgentByName(name string) (Agent, bool) {
	a, ok := s.agentSnapshotByName(name)
	if !ok {
		return Agent{}, false
	}
	ctx := context.Background()
	return s.withConfiguredAgentStartupStatus(s.withRuntimeImageMigrationStatus(ctx, s.hydrateAgentStatus(ctx, a))), true
}

// ResolveAgentID resolves an ID or unique display name without probing runtime
// state. It is intended for API routing before an operation performs its own
// runtime or profile work.
func (s *Repository) ResolveAgentID(selector string) (string, bool) {
	a, ok := s.agentSnapshot(selector)
	if !ok {
		a, ok = s.agentSnapshotByName(selector)
	}
	if !ok {
		return "", false
	}
	return strings.TrimSpace(a.ID), true
}

func (s *Repository) AgentMetadata(id string) (AgentMetadata, bool) {
	a, ok := s.agentSnapshot(id)
	if !ok {
		return AgentMetadata{}, false
	}
	return AgentMetadata{
		ID:          strings.TrimSpace(a.ID),
		Name:        strings.TrimSpace(a.Name),
		Description: strings.TrimSpace(a.Description),
		Role:        strings.TrimSpace(a.Role),
	}, true
}

func (s *Repository) AgentDisplayName(id string) (string, bool) {
	a, ok := s.AgentMetadata(id)
	if !ok {
		return "", false
	}
	name := strings.TrimSpace(a.Name)
	if name == "" {
		name = strings.TrimSpace(a.ID)
	}
	return name, name != ""
}

func (s *Repository) agentSnapshot(id string) (Agent, bool) {
	if s == nil {
		return Agent{}, false
	}
	s.mu.RLock()
	a, _, ok := s.agentByIDLocked(id)
	s.mu.RUnlock()
	if !ok {
		return Agent{}, false
	}
	return *cloneAgent(&a), true
}

func (s *Repository) agentByIDLocked(id string) (Agent, string, bool) {
	for _, key := range agentIDAliases(id) {
		if a, ok := s.agents[key]; ok {
			return a, key, true
		}
	}
	return Agent{}, "", false
}

func (s *Repository) agentSnapshotByName(name string) (Agent, bool) {
	if s == nil {
		return Agent{}, false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Agent{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.agents {
		if strings.EqualFold(strings.TrimSpace(a.Name), name) {
			return *cloneAgent(&a), true
		}
	}
	return Agent{}, false
}

func (s *Controller) resolveAgentBox(ctx context.Context, rt sandbox.Runtime, got Agent) (sandbox.Instance, string, error) {
	keys := make([]string, 0, 3)
	if boxID := strings.TrimSpace(got.BoxID); boxID != "" {
		keys = appendLookupKey(keys, boxID)
	}
	if name := sandboxNameForAgentID(got.ID); name != "" {
		keys = appendLookupKey(keys, name)
	}
	if name := strings.TrimSpace(got.Name); name != "" {
		keys = appendLookupKey(keys, name)
	}
	if len(keys) == 0 {
		return nil, "", fmt.Errorf("agent box identifier is required")
	}

	var lastNotFound error
	for _, key := range keys {
		box, err := s.getBox(ctx, rt, key)
		if err == nil {
			return box, key, nil
		}
		if sandbox.IsNotFound(err) {
			lastNotFound = err
			continue
		}
		return nil, "", fmt.Errorf("get agent box: %w", err)
	}
	if lastNotFound != nil {
		return nil, strings.TrimSpace(got.BoxID), lastNotFound
	}
	return nil, "", fmt.Errorf("agent box %q not found", got.Name)
}

func (s *Controller) refreshAgentBoxID(id string, got Agent, resolvedKey string, box sandbox.Instance) error {
	if box == nil {
		return nil
	}
	if strings.TrimSpace(got.BoxID) != "" && strings.TrimSpace(got.BoxID) == strings.TrimSpace(resolvedKey) {
		return nil
	}

	info, err := s.boxInfo(context.Background(), box)
	if err != nil {
		return fmt.Errorf("read agent box info: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, key, ok := s.agentByIDLocked(id)
	if !ok {
		return nil
	}
	if strings.TrimSpace(current.BoxID) == info.ID {
		return nil
	}
	current.BoxID = info.ID
	s.putAgentLocked(key, current)
	s.syncRuntimeRecordLocked(current)
	return s.saveLocked()
}

func (s *Controller) Start(ctx context.Context, id string) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, fmt.Errorf("agent id is required")
	}
	ctx, release, err := s.acquireAgentLifecycle(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	defer release()

	got, ok := s.Agent(id)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	if got.AgentProfile.EnvRestartRequired && !got.AgentProfile.ImageUpgradeRequired && strings.EqualFold(strings.TrimSpace(got.RuntimeKind), RuntimeKindCodex) {
		return s.restartRuntimeLocked(ctx, id)
	}
	if got.AgentProfile.EnvRestartRequired || got.AgentProfile.ImageUpgradeRequired {
		return s.RecreateRecord(ctx, id)
	}
	startProfile := s.hydrateProfileFromCatalog(normalizeProfileForAgentRuntime(got.AgentProfile, got.RuntimeOptions, got.Name, got.Description, got.RuntimeKind, nil))
	if err := s.validateRuntimeStartConfig(ctx, strings.TrimSpace(got.RuntimeKind), runtimeConfigSnapshotForAgent(startProfile, got.RuntimeOptions)); err != nil {
		return Agent{}, err
	}
	if err := s.validateMCPServers(ctx, strings.TrimSpace(got.RuntimeKind), mcpServersSnapshotForAgent(got.MCPServers)); err != nil {
		return Agent{}, err
	}

	runtimeImpl, err := s.runtimeForKind(strings.TrimSpace(got.RuntimeKind))
	if err != nil {
		return Agent{}, err
	}
	if err := s.provisionRuntimeForAgent(ctx, runtimeImpl, got, ""); err != nil {
		return Agent{}, fmt.Errorf("provision agent runtime: %w", err)
	}
	if err := s.prepareExtensions(ctx, id); err != nil {
		return Agent{}, err
	}
	handle := runtimeHandleForAgent(got)
	state, err := runtimeImpl.Start(ctx, handle)
	if err != nil {
		if sandbox.IsNotFound(err) {
			return s.RecreateRecord(ctx, id)
		}
		return Agent{}, err
	}
	info, err := s.runtimeInfo(ctx, runtimeImpl, handle)
	if err != nil {
		return Agent{}, fmt.Errorf("read agent runtime info: %w", err)
	}
	if info.State == "" {
		info.State = state
	}
	updated, err := s.updateRuntimeState(id, info)
	if err != nil {
		return Agent{}, err
	}
	if err := s.observeStartedExtensions(ctx, id); err != nil {
		return Agent{}, err
	}
	return s.refreshRuntimeAvailability(ctx, updated), nil
}

func (s *Controller) Stop(ctx context.Context, id string) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, fmt.Errorf("agent id is required")
	}
	ctx, release, err := s.acquireAgentLifecycle(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	defer release()

	got, ok := s.Agent(id)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}

	runtimeImpl, err := s.runtimeForKind(strings.TrimSpace(got.RuntimeKind))
	if err != nil {
		return Agent{}, err
	}
	handle := runtimeHandleForAgent(got)
	state, err := runtimeImpl.Stop(ctx, handle)
	if err != nil {
		if sandbox.IsNotFound(err) {
			return Agent{}, fmt.Errorf("agent %q not found", id)
		}
		return Agent{}, err
	}
	info, err := s.runtimeInfo(ctx, runtimeImpl, handle)
	if err != nil {
		return Agent{}, fmt.Errorf("read agent runtime info: %w", err)
	}
	// Prefer Stop()'s reported state over Info when Stop returns a concrete terminal state.
	if state != "" {
		info.State = state
	}
	updated, err := s.updateRuntimeState(id, info)
	if err != nil {
		return Agent{}, err
	}
	if err := s.observeStoppedExtensions(ctx, id); err != nil {
		return Agent{}, err
	}
	return updated, nil
}

func (s *Controller) DeleteRecord(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("agent id is required")
	}
	ctx, release, err := s.acquireAgentLifecycle(ctx, id)
	if err != nil {
		return err
	}
	defer release()

	s.mu.RLock()
	existing, _, ok := s.agentByIDLocked(id)
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}

	if err := s.removeAgentRuntime(ctx, existing); err != nil {
		return err
	}

	s.mu.Lock()

	current, key, ok := s.agentByIDLocked(id)
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("agent %q not found", id)
	}
	delete(s.agents, key)
	s.clearRuntimeAvailabilityLocked(current.ID)
	s.deleteRuntimeRecordLocked(current.RuntimeID)
	runtimeHome, err := s.sandboxRuntimeHome(current.ID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if rt := s.runtimes[runtimeHome]; rt != nil {
		delete(s.runtimes, runtimeHome)
	}
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	return nil
}

// removeAgentRuntime retires the old Adapter's files and process without
// deleting the Agent's desired state or independently managed child resources.
func (s *Controller) removeAgentRuntime(ctx context.Context, existing Agent) error {
	rt, err := s.runtimeForKind(existing.RuntimeKind)
	if err != nil {
		return err
	}
	if err := rt.Delete(ctx, runtimeHandleForAgent(existing)); err != nil && !sandbox.IsNotFound(err) {
		return fmt.Errorf("remove Agent Runtime: %w", err)
	}
	agentHome, err := s.agentHomeDir(existing.ID)
	if err != nil {
		return err
	}
	if err := removeAgentHome(agentHome); err != nil {
		return fmt.Errorf("%w: %v", ErrHomeCleanup, err)
	}
	runtimeHome, err := s.sandboxRuntimeHome(existing.ID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.runtimes, runtimeHome)
	s.mu.Unlock()
	return nil
}

func sandboxNameForAgentID(agentID string) string {
	return agentruntime.SandboxNameForAgentID(canonicalAgentID(agentID))
}

func removeAll(path string) error {
	return osRemoveAll(path)
}

func removeAgentHome(path string) error {
	const attempts = 3
	var err error
	for attempt := range attempts {
		err = removeAll(path)
		if err == nil || !errors.Is(err, syscall.ENOTEMPTY) {
			return err
		}
		if attempt+1 < attempts {
			time.Sleep(time.Duration(attempt+1) * 25 * time.Millisecond)
		}
	}
	return err
}

func (s *Controller) ListRecords() []Agent {
	return s.ListRecordsContext(context.Background())
}

// ListContext returns the persisted agent registry with best-effort live runtime status.
// Runtime status probes must honor ctx so API callers can remain responsive when a
// sandbox runtime is unavailable or contended.
func (s *Controller) ListRecordsContext(ctx context.Context) []Agent {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.RLock()
	agents := sortedAgentsFromMap(s.agents)
	s.mu.RUnlock()
	if len(agents) == 0 {
		return agents
	}

	workers := min(agentListRuntimeProbeConcurrency, len(agents))
	indices := make(chan int)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for idx := range indices {
				current := s.hydrateAgentStatus(ctx, agents[idx])
				// A readiness probe is intentionally not part of the normal
				// roster path. The only exception is a cached observation that
				// has reached its TTL: recheck it so the active workspace poll
				// can discover both failures and recoveries without probing every
				// running agent on every list request.
				agents[idx] = s.refreshExpiredRuntimeAvailability(ctx, current)
			}
		}()
	}
	for idx := range agents {
		indices <- idx
	}
	close(indices)
	group.Wait()

	for idx := range agents {
		agents[idx] = s.withConfiguredAgentStartupStatus(s.withRuntimeImageMigrationStatus(ctx, agents[idx]))
	}
	return agents
}

// PrepareConfiguredAgentsStartup marks the service as reconciling configured
// workers during process startup. The marker is transient and lets API clients
// keep polling while the server is already accepting HTTP requests.
//
// StartConfiguredAgents clears it when its restore pass finishes. It is public
// so a server can cover the small interval between opening its listener and
// scheduling that restore pass.
func (s *Controller) PrepareConfiguredAgentsStartup() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.configuredAgentStartupPending = true
	s.mu.Unlock()
}

func (s *Controller) finishConfiguredAgentsStartup() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.configuredAgentStartupPending = false
	s.mu.Unlock()
}

func (s *Controller) StartConfiguredAgents(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.PrepareConfiguredAgentsStartup()
	defer s.finishConfiguredAgentsStartup()
	agents := s.startupAgentCandidates()
	runningGateways := make([]Agent, 0, len(agents))
	var startErr error
	for _, a := range agents {
		if err := ctx.Err(); err != nil {
			return err
		}
		live := s.hydrateAgentStatus(ctx, a)
		if strings.EqualFold(strings.TrimSpace(live.RuntimeKind), RuntimeKindCodex) {
			if strings.EqualFold(strings.TrimSpace(live.Status), string(agentruntime.StateStopped)) {
				continue
			}
		} else if isRuntimeRunning(live) {
			runningGateways = append(runningGateways, live)
			continue
		}
		if _, err := s.Start(ctx, live.ID); err != nil {
			startErr = errors.Join(startErr, fmt.Errorf("%s: %w", live.Name, err))
		}
	}
	// Readiness observations are intentionally not persisted. Prime the running
	// gateway workers after a process restart so an already-dead gateway does
	// not look healthy until a user opens its detail endpoint. Each probe has a
	// deadline while the startup marker keeps the roster fresh.
	s.primeUnknownGatewayRuntimeAvailability(ctx, runningGateways)
	return startErr
}

// StopRunningSandboxAgents stops every registered sandbox agent whose runtime
// is currently running. Desktop uses this before stopping its sidecar so
// sandbox processes cannot retain a callback URL from the previous instance.
// In-process runtimes are closed normally and keep their persisted running
// state so they can be restored on the next launch.
func (s *Controller) StopRunningSandboxAgents(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.RLock()
	agents := sortedAgentsFromMap(s.agents)
	s.mu.RUnlock()

	candidates := agents[:0]
	for _, a := range agents {
		if isGatewayRuntimeKind(strings.TrimSpace(a.RuntimeKind)) {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, stopRunningSandboxAgentsTimeout)
	defer cancel()
	if err := s.checkSandboxProviderForShutdown(shutdownCtx); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		slog.Info("sandbox provider unavailable during shutdown; skipping sandbox agent stops",
			"provider", s.sandboxProviderName(),
			"agent_count", len(candidates),
			"error", err,
		)
		return nil
	}

	workers := min(stopRunningSandboxAgentsConcurrency, len(candidates))
	jobs := make(chan Agent)
	results := make(chan error, len(candidates))
	for range workers {
		go func() {
			for a := range jobs {
				live := s.hydrateAgentStatus(shutdownCtx, a)
				if !isRuntimeRunning(live) {
					results <- nil
					continue
				}
				_, err := s.Stop(shutdownCtx, live.ID)
				if err != nil {
					err = fmt.Errorf("%s: %w", live.Name, err)
				}
				results <- err
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, a := range candidates {
			select {
			case jobs <- a:
			case <-shutdownCtx.Done():
				return
			}
		}
	}()

	var stopErr error
	for range candidates {
		select {
		case err := <-results:
			stopErr = errors.Join(stopErr, err)
		case <-shutdownCtx.Done():
			return errors.Join(stopErr, fmt.Errorf("stop running sandbox agents: %w", shutdownCtx.Err()))
		}
	}
	return stopErr
}

func (s *Controller) checkSandboxProviderForShutdown(ctx context.Context) error {
	if s == nil || s.sandbox == nil {
		return nil
	}
	checker, ok := s.sandbox.(sandbox.AvailabilityChecker)
	if !ok {
		return nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, sandboxShutdownAvailabilityTimeout)
	defer cancel()
	return checker.CheckAvailability(probeCtx)
}

func (s *Controller) startupAgentCandidates() []Agent {
	s.mu.RLock()
	agents := sortedAgentsFromMap(s.agents)
	s.mu.RUnlock()

	candidates := agents[:0]
	for _, a := range agents {
		if !isConfiguredAgentStartupCandidate(a) {
			continue
		}
		candidates = append(candidates, a)
	}
	return candidates
}

func isConfiguredAgentStartupCandidate(a Agent) bool {
	if isManagerAgent(a) || !isAgentProfileComplete(a) {
		return false
	}
	runtimeKind := strings.TrimSpace(a.RuntimeKind)
	return !strings.EqualFold(normalizeRole(a.Role), RoleWorker) ||
		runtimeKind == "" ||
		isGatewayRuntimeKind(runtimeKind) ||
		strings.EqualFold(runtimeKind, RuntimeKindCodex)
}

func (s *Controller) withConfiguredAgentStartupStatus(a Agent) Agent {
	a.StartupPending = false
	if s == nil || !isConfiguredAgentStartupCandidate(a) {
		return a
	}
	s.mu.RLock()
	pending := s.configuredAgentStartupPending
	s.mu.RUnlock()
	a.StartupPending = pending
	return a
}

func isAgentProfileComplete(a Agent) bool {
	return a.ProfileComplete || a.AgentProfile.ProfileComplete
}

func isRuntimeRunning(a Agent) bool {
	return strings.EqualFold(strings.TrimSpace(a.Status), string(sandbox.StateRunning))
}

func (s *Controller) CreateWorker(ctx context.Context, spec CreateAgentSpec) (Agent, error) {
	return s.createWorker(ctx, spec, false)
}

func (s *Controller) createWorker(ctx context.Context, spec CreateAgentSpec, replacing bool) (_ Agent, err error) {
	if shouldResolveTemplateCreateSpec(spec) && !isResolvedWorkspacePath(spec.FromTemplate) {
		var cleanup func()
		var err error
		spec, cleanup, err = s.resolveTemplateCreateSpec(ctx, spec)
		if err != nil {
			return Agent{}, err
		}
		if cleanup != nil {
			defer cleanup()
		}
	}
	id := strings.TrimSpace(spec.ID)
	name := strings.TrimSpace(spec.Name)
	description := strings.TrimSpace(spec.Description)
	instructions := strings.TrimSpace(spec.Instructions)
	image := strings.TrimSpace(spec.Image)
	avatar := strings.TrimSpace(spec.Avatar)
	runtimeKindProvided := strings.TrimSpace(spec.RuntimeKind) != ""
	runtimeNameProvided := strings.TrimSpace(spec.RuntimeName) != ""
	runtimeCfg, err := agentruntime.RuntimeConfigFromSelection(spec.RuntimeKind, spec.RuntimeName, spec.SandboxEnabled)
	if err != nil {
		return Agent{}, err
	}
	spec.SetRuntimeConfig(runtimeCfg)
	normalizedMCPServers, err := normalizeMCPServers(spec.MCPServers)
	if err != nil {
		return Agent{}, err
	}
	spec.MCPServers = normalizedMCPServers
	runtimeKind := spec.RuntimeKind
	runtimeName := spec.RuntimeName
	sandboxed := spec.SandboxEnabled
	switch {
	case name == "":
		return Agent{}, fmt.Errorf("name is required")
	case strings.EqualFold(name, ManagerName):
		return Agent{}, fmt.Errorf("name %q is reserved", name)
	}
	if err := identity.ValidateMentionName(name); err != nil {
		return Agent{}, err
	}
	if id == "" {
		var err error
		id, err = newAgentID()
		if err != nil {
			return Agent{}, err
		}
	} else {
		var err error
		id, err = normalizeExplicitAgentID(id)
		if err != nil {
			return Agent{}, err
		}
	}
	ctx, release, err := s.acquireAgentLifecycle(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	defer release()

	s.mu.RLock()
	existing, _, idExists := s.agentByIDLocked(id)
	nameExists := s.hasNameLocked(name)
	if replacing {
		nameExists = s.hasNameLockedExcept(name, id)
	}
	s.mu.RUnlock()
	if idExists && !replacing {
		return Agent{}, fmt.Errorf("agent id %q already exists", id)
	}
	if replacing && !idExists {
		return Agent{}, fmt.Errorf("agent id %q not found", id)
	}
	if nameExists {
		return Agent{}, fmt.Errorf("agent name %q already exists", name)
	}
	switch {
	case runtimeName == "":
		return Agent{}, fmt.Errorf("runtime_kind is required")
	case !sandboxed && runtimeName != RuntimeNameCodex:
		return Agent{}, fmt.Errorf("runtime_name %q requires sandbox_enabled=true", runtimeName)
	case sandboxed && runtimeName != RuntimeNameOpenClaw && runtimeName != RuntimeNamePicoClaw:
		return Agent{}, fmt.Errorf("runtime_name %q is not supported with sandbox_enabled=true", runtimeName)
	}
	if !sandboxed {
		if err := s.checkRuntimeAvailability(ctx, runtimeKind); err != nil {
			return Agent{}, fmt.Errorf("codex cli not installed: %w", err)
		}
		image = ""
	} else if image == "" {
		if latest, ok := s.currentDefaultImageForAgent(ctx, Agent{Role: RoleWorker, RuntimeKind: runtimeKind}); ok {
			image = strings.TrimSpace(latest.image)
		}
		if image == "" {
			if runtimeNameProvided && !runtimeKindProvided {
				return Agent{}, fmt.Errorf("default image is not configured for sandbox runtime %q", runtimeName)
			}
			return Agent{}, fmt.Errorf("image is required for runtime_kind %q", runtimeKind)
		}
	}

	runtimeImpl, err := s.runtimeForKind(runtimeKind)
	if err != nil {
		return Agent{}, err
	}
	resolvedProfile, err := s.profileForCreateRequest(ctx, &spec)
	if err != nil {
		return Agent{}, err
	}
	runtimeResolvedProfile := s.hydrateProfileFromCatalog(resolvedProfile)
	if err := s.validateRuntimeConfig(ctx, runtimeKind, runtimeConfigSnapshotForAgent(runtimeResolvedProfile, spec.RuntimeOptions)); err != nil {
		return Agent{}, err
	}
	if err := s.validateMCPServers(ctx, runtimeKind, mcpServersSnapshotForAgent(spec.MCPServers)); err != nil {
		return Agent{}, err
	}
	runtimeProfile := s.runtimeProfileForKind(runtimeKind, id, name, description, runtimeResolvedProfile)
	if replacing {
		if err := s.removeAgentRuntime(ctx, existing); err != nil {
			return Agent{}, err
		}
		if err := s.persistStartingWorker(ctx, id, name, description, instructions, image, avatar, runtimeKind, runtimeName, sandboxed, resolvedProfile, spec.RuntimeOptions, spec.MCPServers, spec.RuntimeCredentials, spec.RuntimeInitShell, true); err != nil {
			return Agent{}, err
		}
		defer func() {
			if err != nil {
				// Keep the replacement's desired state and extensions available
				// for retry even if its Adapter cannot start or support a tool.
				_, saveErr := s.updateRuntimeState(id, agentruntime.Info{State: agentruntime.StateFailed})
				err = errors.Join(err, saveErr)
			}
		}()
	}
	if err := s.provisionRuntime(ctx, runtimeImpl, runtimeKind, agentruntime.ProvisionRequest{
		RuntimeID:            runtimeIDForAgentID(id),
		AgentID:              id,
		ParticipantID:        participantIDForAgent(name, id),
		AgentName:            name,
		Instructions:         instructions,
		TemplateInstructions: spec.TemplateInstructions,
		TemplateMemory:       spec.TemplateMemory,
		TemplateMemorySet:    spec.TemplateMemorySet,
		Profile:              runtimeProfile,
		RuntimeOptions:       utils.CloneAnyMap(spec.RuntimeOptions),
		MCPServers:           cloneMCPServers(spec.MCPServers),
		Credentials:          cloneStringMap(spec.RuntimeCredentials),
		InitShell:            spec.RuntimeInitShell,
		WorkspaceOverlay:     strings.TrimSpace(spec.FromTemplate),
	}); err != nil {
		return Agent{}, fmt.Errorf("provision worker runtime: %w", err)
	}
	if testCreateGatewayBoxHook != nil && isGatewayRuntimeKind(runtimeKind) {
		rt, err := s.ensureRuntime(id)
		if err != nil {
			return Agent{}, err
		}
		runtimeHome, err := s.sandboxRuntimeHome(id)
		if err != nil {
			return Agent{}, err
		}
		defer func() {
			_ = s.closeRuntime(runtimeHome, rt)
		}()
		box, info, err := s.createGatewayBox(ctx, rt, image, name, id, runtimeResolvedProfile)
		if err != nil {
			return Agent{}, fmt.Errorf("create worker box: %w", err)
		}
		defer func() {
			_ = s.closeBox(box)
		}()
		return s.persistCreatedWorker(ctx, id, name, description, instructions, image, avatar, runtimeKind, runtimeName, sandboxed, resolvedProfile, spec.RuntimeOptions, spec.MCPServers, spec.RuntimeCredentials, spec.RuntimeInitShell, agentruntime.Info{
			HandleID:  strings.TrimSpace(info.ID),
			State:     agentruntime.State(info.State),
			CreatedAt: info.CreatedAt.UTC(),
		})
	}
	if runtimeKind == RuntimeKindCodex && !replacing {
		if err := s.persistStartingWorker(ctx, id, name, description, instructions, image, avatar, runtimeKind, runtimeName, sandboxed, resolvedProfile, spec.RuntimeOptions, spec.MCPServers, spec.RuntimeCredentials, spec.RuntimeInitShell, false); err != nil {
			return Agent{}, err
		}
		defer func() {
			if err != nil {
				_ = s.removeStartingWorker(ctx, id)
			}
		}()
	}
	if err := s.prepareExtensions(ctx, id); err != nil {
		return Agent{}, err
	}
	handle, err := runtimeImpl.New(ctx, agentruntime.Spec{
		RuntimeID: runtimeIDForAgentID(id),
		AgentID:   id,
		AgentName: name,
		Image:     image,
		Profile:   runtimeProfile,
	})
	if err != nil {
		return Agent{}, fmt.Errorf("create worker box: %w", err)
	}
	if _, exists := s.agentSnapshot(id); exists {
		if err := s.observeStartedExtensions(ctx, id); err != nil {
			return Agent{}, err
		}
	}
	info := agentruntime.Info{
		HandleID:  strings.TrimSpace(handle.HandleID),
		State:     agentruntime.StateRunning,
		CreatedAt: time.Now().UTC(),
	}

	return s.persistCreatedWorker(ctx, id, name, description, instructions, image, avatar, runtimeKind, runtimeName, sandboxed, resolvedProfile, spec.RuntimeOptions, spec.MCPServers, spec.RuntimeCredentials, spec.RuntimeInitShell, info)
}

func (s *Controller) persistStartingWorker(ctx context.Context, id, name, description, instructions, image, avatar, runtimeKind, runtimeName string, sandboxEnabled bool, profile AgentProfile, runtimeOptions map[string]any, mcpServers map[string]any, runtimeCredentials map[string]string, runtimeInitShell string, replacing bool) error {
	s.mu.Lock()

	previous, previousKey, exists := s.agentByIDLocked(id)
	if exists && !replacing {
		s.mu.Unlock()
		return fmt.Errorf("agent id %q already exists", id)
	}
	if s.hasNameLockedExcept(name, id) {
		s.mu.Unlock()
		return fmt.Errorf("agent name %q already exists", name)
	}

	worker := newWorkerAgent(id, name, description, instructions, image, avatar, runtimeKind, runtimeName, sandboxEnabled, profile, runtimeOptions, mcpServers, runtimeCredentials, runtimeInitShell, agentruntime.Info{
		State:     agentruntime.StateCreated,
		CreatedAt: time.Now().UTC(),
	})
	if exists && previousKey != worker.ID {
		worker.RuntimeExtensions = cloneRawMessages(previous.RuntimeExtensions)
		delete(s.agents, previousKey)
	}
	s.putAgentLocked(worker.ID, worker)
	s.syncRuntimeRecordLocked(worker)
	err := s.saveLocked()
	if err != nil && exists {
		delete(s.agents, worker.ID)
		s.putAgentLocked(previousKey, previous)
		s.syncRuntimeRecordLocked(previous)
	}
	s.mu.Unlock()
	if err != nil {
		if !exists {
			_ = s.removeStartingWorker(ctx, id)
		}
		return err
	}
	return nil
}

func (s *Controller) removeStartingWorker(ctx context.Context, id string) error {
	s.mu.Lock()
	current, ok := s.agents[id]
	if ok && strings.TrimSpace(current.BoxID) == "" && strings.EqualFold(strings.TrimSpace(current.Status), string(agentruntime.StateCreated)) {
		delete(s.agents, id)
		s.clearRuntimeAvailabilityLocked(current.ID)
		s.deleteRuntimeRecordLocked(current.RuntimeID)
	}
	err := s.saveLocked()
	s.mu.Unlock()
	return err
}

func (s *Controller) persistCreatedWorker(ctx context.Context, id, name, description, instructions, image, avatar, runtimeKind, runtimeName string, sandboxEnabled bool, profile AgentProfile, createRuntimeExt map[string]any, mcpServers map[string]any, runtimeCredentials map[string]string, runtimeInitShell string, info agentruntime.Info) (Agent, error) {
	s.mu.Lock()

	previous, _, exists := s.agentByIDLocked(id)
	if exists && !isStartingWorker(previous) && !(previous.Status == string(agentruntime.StateCreated) && previous.BoxID == info.HandleID) {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent id %q already exists", id)
	}
	if s.hasNameLockedExcept(name, id) {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent name %q already exists", name)
	}

	worker := newWorkerAgent(id, name, description, instructions, image, avatar, runtimeKind, runtimeName, sandboxEnabled, profile, createRuntimeExt, mcpServers, runtimeCredentials, runtimeInitShell, info)
	s.clearRuntimeAvailabilityLocked(worker.ID)
	s.putAgentLocked(worker.ID, worker)
	s.syncRuntimeRecordLocked(worker)
	if worker.AgentProfile.ProfileComplete {
		s.profileDefaults = profileDefaultsSnapshot(worker.AgentProfile)
	}
	if err := s.saveLocked(); err != nil {
		if exists {
			s.putAgentLocked(worker.ID, previous)
			s.syncRuntimeRecordLocked(previous)
		} else {
			delete(s.agents, worker.ID)
			s.deleteRuntimeRecordLocked(worker.RuntimeID)
		}
		s.mu.Unlock()
		return Agent{}, err
	}
	created := *cloneAgent(&worker)
	s.mu.Unlock()
	return created, nil
}

func newWorkerAgent(id, name, description, instructions, image, avatar, runtimeKind, runtimeName string, sandboxEnabled bool, profile AgentProfile, runtimeOptions map[string]any, mcpServers map[string]any, runtimeCredentials map[string]string, runtimeInitShell string, info agentruntime.Info) Agent {
	createdAt := info.CreatedAt.UTC()
	if info.CreatedAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	state := info.State
	if state == "" {
		state = agentruntime.StateRunning
	}
	prof := cloneProfile(profile)
	var agentRX map[string]any
	if len(runtimeOptions) > 0 {
		agentRX = utils.CloneAnyMap(runtimeOptions)
	}
	runtimeCfg, _ := agentruntime.RuntimeConfigFromSelection(runtimeKind, runtimeName, sandboxEnabled)
	resolvedRuntimeKind := runtimeCfg.LegacyKind()
	resolvedRuntimeName := runtimeCfg.Name
	worker := Agent{
		ID:              id,
		Name:            name,
		RuntimeID:       runtimeIDForAgentID(id),
		RuntimeKind:     resolvedRuntimeKind,
		RuntimeName:     resolvedRuntimeName,
		SandboxEnabled:  runtimeCfg.Sandboxed,
		Image:           image,
		Avatar:          strings.TrimSpace(avatar),
		BoxID:           strings.TrimSpace(info.HandleID),
		Description:     description,
		Instructions:    strings.TrimSpace(instructions),
		Status:          string(state),
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
		RuntimeOptions:  agentRX,
		MCPServers:      cloneMCPServers(mcpServers),
		Profile:         profileSelector(prof),
		AgentProfile:    prof,
		ProfileComplete: prof.ProfileComplete,
		Role:            RoleWorker,
	}
	worker.SetRuntimeProvision(runtimeCredentials, runtimeInitShell)
	return worker
}

func isStartingWorker(a Agent) bool {
	return strings.TrimSpace(a.BoxID) == "" && strings.EqualFold(strings.TrimSpace(a.Status), string(agentruntime.StateCreated))
}

func isResolvedWorkspacePath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (s *Controller) provisionRuntimeRequest(ctx context.Context, rt agentruntime.Runtime, runtimeKind string, req agentruntime.ProvisionRequest) error {
	if rt == nil {
		return fmt.Errorf("runtime is required")
	}
	servers, err := s.materializeRuntimeMCPServers(ctx, runtimeKind, req.MCPServers)
	if err != nil {
		return err
	}
	req.MCPServers = servers
	if isGatewayRuntimeKind(runtimeKind) && req.Gateway == nil {
		gateway, err := s.gatewayProvisionRequest(runtimeKind, req.AgentName, req.AgentID)
		if err != nil {
			return err
		}
		req.Gateway = gateway
	}
	provisioner, ok := rt.(agentruntime.Provisioner)
	if !ok {
		return nil
	}
	return provisioner.Provision(ctx, req)
}

func (s *Controller) provisionRuntime(ctx context.Context, rt agentruntime.Runtime, runtimeKind string, req agentruntime.ProvisionRequest) error {
	if err := s.provisionRuntimeRequest(ctx, rt, runtimeKind, req); err != nil {
		return err
	}
	if err := s.installDefaultSystemSkills(req.AgentID, runtimeKind); err != nil {
		return fmt.Errorf("install default system skills: %w", err)
	}
	return nil
}

func (s *Controller) provisionRuntimeForAgent(ctx context.Context, rt agentruntime.Runtime, got Agent, workspaceOverlay string) error {
	credentials, _ := got.RuntimeProvision()
	return s.provisionRuntimeForAgentWithPrevious(ctx, rt, got, sortedStringKeys(credentials), workspaceOverlay)
}

func (s *Controller) provisionRuntimeForAgentWithPrevious(ctx context.Context, rt agentruntime.Runtime, got Agent, previousCredentials []string, workspaceOverlay string) error {
	if s == nil || rt == nil {
		return nil
	}
	credentials, initShell := got.RuntimeProvision()
	return s.provisionRuntime(ctx, rt, strings.TrimSpace(got.RuntimeKind), agentruntime.ProvisionRequest{
		RuntimeID:           normalizeRuntimeID(got.RuntimeID, got.ID),
		AgentID:             strings.TrimSpace(got.ID),
		ParticipantID:       participantIDForAgent(got.Name, got.ID),
		AgentName:           strings.TrimSpace(got.Name),
		Instructions:        strings.TrimSpace(got.Instructions),
		Profile:             s.runtimeProfileForAgent(got),
		RuntimeOptions:      utils.CloneAnyMap(got.RuntimeOptions),
		MCPServers:          cloneMCPServers(got.MCPServers),
		Credentials:         credentials,
		PreviousCredentials: append([]string(nil), previousCredentials...),
		InitShell:           initShell,
		WorkspaceOverlay:    strings.TrimSpace(workspaceOverlay),
	})
}

func participantIDForAgent(agentName, agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if managerGatewayMatch(agentName, agentID) {
		return ManagerParticipantID
	}
	return participantIDFromAgentID(agentID)
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func ParticipantIDForAgent(agentName, agentID string) string {
	return participantIDForAgent(agentName, agentID)
}

func participantIDFromAgentID(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if strings.HasPrefix(agentID, AgentIDPrefix) {
		suffix := strings.TrimPrefix(agentID, AgentIDPrefix)
		if suffix != "" {
			return "pt-" + suffix
		}
	}
	if strings.HasPrefix(agentID, "u-") {
		suffix := strings.TrimPrefix(agentID, "u-")
		suffix = strings.TrimPrefix(suffix, AgentIDPrefix)
		if suffix != "" {
			return "pt-" + suffix
		}
	}
	return agentID
}

func (s *Controller) gatewayProvisionRequest(runtimeKind, agentName, agentID string) (*agentruntime.GatewayProvision, error) {
	if s == nil {
		return nil, fmt.Errorf("agent service is required")
	}
	agentHome, err := s.agentHomeDir(agentID)
	if err != nil {
		return nil, err
	}
	projectsRoot, err := ensureAgentProjectsRoot()
	if err != nil {
		return nil, err
	}
	role := RoleWorker
	if managerGatewayMatch(agentName, agentID) {
		role = RoleManager
	}
	templateRoot, err := resolveRuntimeTemplateRoot(runtimeKind, role)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	modelFallback := s.model.Resolved().ModelID
	server := s.server
	s.mu.RUnlock()
	return &agentruntime.GatewayProvision{
		ModelFallback:     modelFallback,
		Server:            server,
		ManagerBaseURL:    s.resolveManagerBaseURL(server),
		AgentHome:         agentHome,
		ProjectsRoot:      projectsRoot,
		WorkspaceTemplate: templateRoot,
	}, nil
}

func (s *Controller) resolveManagerBaseURL(server config.ServerConfig) string {
	return resolveManagerBaseURLForSandboxProvider(server, s.sandboxProviderName())
}

func streamHostGatewayLogPaths(ctx context.Context, logPaths []string, follow bool, lines int, w io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := streamGatewayLogFile(ctx, logPaths, follow, lines, w); err != nil {
		if follow && errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	return nil
}

func streamGatewayLogFile(ctx context.Context, logPaths []string, follow bool, lines int, w io.Writer) error {
	file, err := openGatewayLogFile(ctx, logPaths, follow)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	offset, err := writeLastGatewayLogLines(file, lines, w)
	if err != nil || !follow {
		return err
	}
	return followGatewayLogFile(ctx, file, offset, w)
}

func openGatewayLogFile(ctx context.Context, logPaths []string, follow bool) (*os.File, error) {
	if len(logPaths) == 0 {
		return nil, os.ErrNotExist
	}
	for {
		var notFound error
		for _, logPath := range logPaths {
			file, err := os.Open(logPath)
			if err == nil {
				return file, nil
			}
			if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			if notFound == nil {
				notFound = err
			}
		}
		if !follow {
			if notFound != nil {
				return nil, notFound
			}
			return nil, os.ErrNotExist
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(gatewayLogPoll):
		}
	}
}

func writeLastGatewayLogLines(file *os.File, lines int, w io.Writer) (int64, error) {
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	size := info.Size()
	if size <= 0 {
		return 0, nil
	}
	data, err := readLastGatewayLogLines(file, size, lines)
	if err != nil {
		return 0, err
	}
	if len(data) > 0 {
		if _, err := w.Write(data); err != nil {
			return 0, err
		}
	}
	return size, nil
}

func readLastGatewayLogLines(file *os.File, size int64, lines int) ([]byte, error) {
	const chunkSize int64 = 4096

	var data []byte
	var newlineCount int
	pos := size
	for pos > 0 && (lines <= 0 || newlineCount <= lines) {
		readSize := chunkSize
		if pos < readSize {
			readSize = pos
		}
		pos -= readSize
		chunk := make([]byte, int(readSize))
		if _, err := file.ReadAt(chunk, pos); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		data = append(chunk, data...)
		newlineCount += bytes.Count(chunk, []byte{'\n'})
	}
	return trimLastGatewayLogLines(data, lines), nil
}

func trimLastGatewayLogLines(data []byte, lines int) []byte {
	if lines <= 0 || len(data) == 0 {
		return data
	}
	seen := 0
	for idx := len(data) - 1; idx >= 0; idx-- {
		if data[idx] != '\n' {
			continue
		}
		if idx == len(data)-1 {
			continue
		}
		seen++
		if seen == lines {
			return data[idx+1:]
		}
	}
	return data
}

func followGatewayLogFile(ctx context.Context, file *os.File, offset int64, w io.Writer) error {
	ticker := time.NewTicker(gatewayLogPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		info, err := file.Stat()
		if err != nil {
			return err
		}
		size := info.Size()
		if size < offset {
			offset = 0
		}
		if size == offset {
			continue
		}
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return err
		}
		n, err := io.CopyN(w, file, size-offset)
		offset += n
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
	}
}

func (s *Controller) hydrateAgentStatus(ctx context.Context, a Agent) (out Agent) {
	a = *cloneAgent(&a)
	defer func() {
		out = s.withRuntimeAvailability(out)
	}()
	if strings.TrimSpace(a.Name) == "" {
		logHydrateUnknownStatus(a, "validate_name", fmt.Errorf("agent name is required"))
		a.Status = string(sandbox.StateUnknown)
		return a
	}

	runtimeImpl, err := s.runtimeForKind(strings.TrimSpace(a.RuntimeKind))
	if err != nil {
		return statusAfterHydrateFailure(a, "select_runtime", err)
	}
	info, err := s.runtimeInfo(ctx, runtimeImpl, runtimeHandleForAgent(a))
	if err != nil {
		a = statusAfterHydrateFailure(a, "read_runtime_info", err)
		s.recordRuntimeAvailabilityFromLifecycleError(a, err)
		return a
	}
	if agentruntime.HydrateTrustPersistedStopped(runtimeImpl) && strings.EqualFold(strings.TrimSpace(a.Status), string(sandbox.StateStopped)) {
		if strings.TrimSpace(info.HandleID) != "" {
			a.BoxID = info.HandleID
		}
		a.RuntimeID = normalizeRuntimeID(a.RuntimeID, a.ID)
		return a
	}
	if strings.TrimSpace(info.HandleID) != "" {
		a.BoxID = info.HandleID
	}
	a.RuntimeID = normalizeRuntimeID(a.RuntimeID, a.ID)
	if info.State != "" {
		a.Status = string(info.State)
	}
	return a
}

func statusAfterHydrateFailure(a Agent, stage string, err error) Agent {
	if status := strings.TrimSpace(a.Status); status != "" {
		if !sandbox.IsNotFound(err) {
			logHydrateStaleStatus(a, stage, err)
			return a
		}
		if strings.EqualFold(status, "profile_incomplete") {
			return a
		}
	}
	logHydrateUnknownStatus(a, stage, err)
	a.Status = string(sandbox.StateUnknown)
	return a
}

func logHydrateStaleStatus(a Agent, stage string, err error) {
	if strings.TrimSpace(stage) == "" {
		stage = "unknown_stage"
	}
	attrs := []any{
		"agent_id", strings.TrimSpace(a.ID),
		"agent_name", strings.TrimSpace(a.Name),
		"agent_box_id", strings.TrimSpace(a.BoxID),
		"agent_status", strings.TrimSpace(a.Status),
		"stage", stage,
		"error", err,
	}
	if sandbox.IsBusy(err) {
		slog.Debug("agent status refresh skipped; sandbox runtime is busy", attrs...)
		return
	}
	slog.Warn("agent status refresh failed; keeping last known status",
		attrs...,
	)
}

func logHydrateUnknownStatus(a Agent, stage string, err error) {
	if strings.TrimSpace(stage) == "" {
		stage = "unknown_stage"
	}
	slog.Warn("agent status downgraded to unknown",
		"agent_id", strings.TrimSpace(a.ID),
		"agent_name", strings.TrimSpace(a.Name),
		"agent_box_id", strings.TrimSpace(a.BoxID),
		"stage", stage,
		"error", err,
	)
}

func (s *Controller) Close() error {
	s.mu.Lock()
	sandboxRuntimes := make(map[string]sandbox.Runtime, len(s.runtimes))
	for name, rt := range s.runtimes {
		sandboxRuntimes[name] = rt
		delete(s.runtimes, name)
	}
	s.mu.Unlock()

	var closeErr error
	for name, rt := range sandboxRuntimes {
		if err := rt.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close sandbox runtime %q: %w", name, err))
		}
	}
	closeErr = errors.Join(closeErr, s.runtimeRegistry.Close())
	return closeErr
}

// ResetSandboxRuntimes drops cached sandbox clients so the next operation
// reloads environment-sensitive credentials, such as the active CSGHub site
// and access token.
func (s *Controller) ResetSandboxRuntimes() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	sandboxRuntimes := make(map[string]sandbox.Runtime, len(s.runtimes))
	for name, rt := range s.runtimes {
		sandboxRuntimes[name] = rt
		delete(s.runtimes, name)
	}
	s.mu.Unlock()

	var closeErr error
	for name, rt := range sandboxRuntimes {
		if err := rt.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close sandbox runtime %q: %w", name, err))
		}
	}
	return closeErr
}

func (s *Controller) hasNameLocked(name string) bool {
	return s.hasNameLockedExcept(name, "")
}

func (s *Controller) hasNameLockedExcept(name, exceptID string) bool {
	for _, existing := range s.agents {
		if strings.TrimSpace(exceptID) != "" && strings.EqualFold(canonicalAgentID(existing.ID), canonicalAgentID(exceptID)) {
			continue
		}
		if strings.EqualFold(existing.Name, name) {
			return true
		}
	}
	return false
}

func (s *Controller) RuntimeRegistry() *registry.Registry {
	s.bindResources()
	return s.runtimeRegistry
}
