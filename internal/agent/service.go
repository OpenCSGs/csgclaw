package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"csgclaw/internal/codexcli"
	"csgclaw/internal/config"
	"csgclaw/internal/identity"
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

var localIPv4Resolver = localIPv4

var osRemoveAll = os.RemoveAll
var locateCodexCLI = func() (string, error) {
	return codexcli.Locator{}.Locate()
}

var defaultSandboxProvider sandbox.Provider = unconfiguredSandboxProvider{}
var testDefaultServiceOption ServiceOption

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
	testEnsureRuntimeHook       func(*Service, string) (sandbox.Runtime, error)
	testEnsureRuntimeAtHomeHook func(*Service, string) (sandbox.Runtime, error)
	testGetBoxHook              func(*Service, context.Context, sandbox.Runtime, string) (sandbox.Instance, error)
	testStartBoxHook            func(*Service, context.Context, sandbox.Instance) error
	testStopBoxHook             func(*Service, context.Context, sandbox.Instance, sandbox.StopOptions) error
	testBoxInfoHook             func(*Service, context.Context, sandbox.Instance) (sandbox.Info, error)
	testCloseBoxHook            func(*Service, sandbox.Instance) error
	testCloseRuntimeHook        func(*Service, string, sandbox.Runtime) error
	testCreateBoxHook           func(*Service, context.Context, sandbox.Runtime, sandbox.CreateSpec) (sandbox.Instance, error)
	testCreateGatewayBoxHook    func(*Service, context.Context, sandbox.Runtime, string, string, string, AgentProfile) (sandbox.Instance, sandbox.Info, error)
	testForceRemoveBoxHook      func(*Service, context.Context, sandbox.Runtime, string) error
	testRunBoxCommandHook       func(*Service, context.Context, sandbox.Instance, string, []string, io.Writer) (int, error)
)

// SetTestHooks installs lightweight hooks for tests that need to bypass runtime/box creation.
func SetTestHooks(
	ensureRuntime func(*Service, string) (sandbox.Runtime, error),
	createGatewayBox func(*Service, context.Context, sandbox.Runtime, string, string, string, AgentProfile) (sandbox.Instance, sandbox.Info, error),
) {
	testEnsureRuntimeHook = ensureRuntime
	if ensureRuntime != nil {
		testEnsureRuntimeAtHomeHook = func(s *Service, _ string) (sandbox.Runtime, error) {
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
func TestOnlySetGetBoxHook(hook func(*Service, context.Context, sandbox.Runtime, string) (sandbox.Instance, error)) {
	testGetBoxHook = hook
}

// TestOnlySetStartBoxHook installs a test hook for starting a box.
func TestOnlySetStartBoxHook(hook func(*Service, context.Context, sandbox.Instance) error) {
	testStartBoxHook = hook
}

// TestOnlySetStopBoxHook installs a test hook for stopping a box.
func TestOnlySetStopBoxHook(hook func(*Service, context.Context, sandbox.Instance, sandbox.StopOptions) error) {
	testStopBoxHook = hook
}

// TestOnlySetBoxInfoHook installs a test hook for reading box info.
func TestOnlySetBoxInfoHook(hook func(*Service, context.Context, sandbox.Instance) (sandbox.Info, error)) {
	testBoxInfoHook = hook
}

// TestOnlySetRunBoxCommandHook installs a test hook for command execution inside a box.
func TestOnlySetRunBoxCommandHook(hook func(*Service, context.Context, sandbox.Instance, string, []string, io.Writer) (int, error)) {
	testRunBoxCommandHook = hook
}

func TestOnlySetDefaultServiceOption(opt ServiceOption) func() {
	previous := testDefaultServiceOption
	testDefaultServiceOption = opt
	return func() {
		testDefaultServiceOption = previous
	}
}

type Service struct {
	model                  config.ModelConfig
	llm                    config.LLMConfig
	server                 config.ServerConfig
	hub                    templateService
	defaultManagerTemplate string
	defaultWorkerTemplate  string
	managerImage           string
	gatewayRuntime         string
	state                  string
	agentsRoot             string
	sandbox                sandbox.Provider
	mu                     sync.RWMutex
	// mcpServersMu serializes all MCP server mutations. A catalog batch add
	// first reads the current set before issuing its update, so it must share
	// the same lock as direct PUT/PATCH-style updates to avoid stale snapshots
	// overwriting a concurrent edit.
	mcpServersMu            sync.Mutex
	runtimes                map[string]sandbox.Runtime
	agents                  map[string]Agent
	runtimeRecords          map[string]RuntimeRecord
	runtimeRegistry         map[string]agentruntime.Runtime
	lifecycle               LifecycleObserver
	profileDefaults         AgentProfile
	detectionResults        []ProfileDetectionResult
	startupProfileDetectOff bool

	// gatewayWorkPhase is set by createGatewayBox for bootstrap progress logs (best-effort if concurrent).
	gatewayWorkPhase atomic.Uint32
}

type ServiceOption func(*Service) error

type templateService interface {
	List(context.Context) ([]hub.Template, error)
	Get(context.Context, string) (hub.Template, error)
	FetchWorkspace(context.Context, string) (hub.WorkspaceRef, error)
}

func (s *Service) HubPublishSpec(agentID string) (hub.PublishSpec, error) {
	if s == nil {
		return hub.PublishSpec{}, fmt.Errorf("agent service is required")
	}
	got, ok := s.agentSnapshot(agentID)
	if !ok {
		return hub.PublishSpec{}, fmt.Errorf("agent %q not found", strings.TrimSpace(agentID))
	}
	workspaceRoot, err := s.agentWorkspaceRoot(got.ID, got.RuntimeKind)
	if err != nil {
		return hub.PublishSpec{}, err
	}
	return hub.PublishSpec{
		ID:          got.Name,
		Name:        got.Name,
		Description: got.Description,
		Role:        got.Role,
		RuntimeKind: got.RuntimeConfig().Kind(),
		Image:       got.Image,
		WorkspaceRef: hub.WorkspaceRef{
			Kind: hub.WorkspaceKindDir,
			Path: workspaceRoot,
		},
	}, nil
}

func WithSandboxProvider(provider sandbox.Provider) ServiceOption {
	return func(s *Service) error {
		if provider == nil {
			return fmt.Errorf("sandbox provider is required")
		}
		s.sandbox = provider
		return nil
	}
}

func WithRuntime(rt agentruntime.Runtime) ServiceOption {
	return func(s *Service) error {
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
		s.runtimeRegistry[kind] = rt
		return nil
	}
}

func WithHubService(svc *hub.Service) ServiceOption {
	return func(s *Service) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		s.hub = svc
		return nil
	}
}

func WithBootstrapDefaultTemplates(cfg config.BootstrapConfig) ServiceOption {
	return func(s *Service) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		s.defaultManagerTemplate = strings.TrimSpace(cfg.ResolvedDefaultManagerTemplate())
		s.defaultWorkerTemplate = strings.TrimSpace(cfg.ResolvedDefaultWorkerTemplate())
		return nil
	}
}

func WithStartupProfileDetectionDisabled() ServiceOption {
	return func(s *Service) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		s.SetStartupProfileDetectionDisabled(true)
		return nil
	}
}

func (s *Service) SetStartupProfileDetectionDisabled(disabled bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.startupProfileDetectOff = disabled
	s.mu.Unlock()
}

func WithLifecycleObserver(observer LifecycleObserver) ServiceOption {
	return func(s *Service) error {
		if s == nil {
			return fmt.Errorf("agent service is required")
		}
		s.lifecycle = observer
		return nil
	}
}

// WithGatewayRuntime sets picoclaw vs openclaw gateway behavior (from [bootstrap] or image inference).
func WithGatewayRuntime(runtime string) ServiceOption {
	return func(s *Service) error {
		kind := runtimeKindForGatewayRuntime(runtime)
		if kind == "" {
			return fmt.Errorf("gateway runtime %q is not supported", runtime)
		}
		s.gatewayRuntime = kind
		return nil
	}
}

func (s *Service) GatewayRuntime() string {
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

func (s *Service) SetGatewayRuntime(runtime, managerImage string) error {
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

func NewService(model config.ModelConfig, server config.ServerConfig, managerImage, statePath string, opts ...ServiceOption) (*Service, error) {
	return NewServiceWithLLM(config.SingleProfileLLM(model), server, managerImage, statePath, opts...)
}

func NewServiceWithLLM(llmCfg config.LLMConfig, server config.ServerConfig, managerImage, statePath string, opts ...ServiceOption) (*Service, error) {
	// agent.Service owns the persisted registry and runtime selection.
	defaultProfile, model, err := llmCfg.Resolve("")
	if err != nil {
		defaultProfile = strings.TrimSpace(llmCfg.Normalized().Default)
		if defaultProfile == "" {
			defaultProfile = strings.TrimSpace(llmCfg.Normalized().DefaultProfile)
		}
		model = config.ModelConfig{}.Resolved()
	}
	svc := &Service{
		model:           model,
		llm:             llmCfg.Normalized(),
		server:          server,
		managerImage:    managerImage,
		state:           statePath,
		agentsRoot:      serviceAgentsRoot(statePath),
		sandbox:         defaultSandboxProvider,
		runtimes:        make(map[string]sandbox.Runtime),
		agents:          make(map[string]Agent),
		runtimeRecords:  make(map[string]RuntimeRecord),
		runtimeRegistry: make(map[string]agentruntime.Runtime),
		profileDefaults: profileFromConfigModel("", "", model),
	}
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
	if err := svc.load(); err != nil {
		return nil, err
	}
	return svc, nil
}

func (s *Service) SetLLMConfig(llmCfg config.LLMConfig) {
	if s == nil {
		return
	}
	llmCfg = llmCfg.Normalized()
	defaultSelector, defaultModel, err := llmCfg.Resolve("")
	s.mu.Lock()
	s.llm = llmCfg
	if err == nil {
		s.model = defaultModel
		if strings.TrimSpace(s.profileDefaults.ModelProviderID) != "" || strings.TrimSpace(s.profileDefaults.Provider) == "" {
			s.profileDefaults = profileFromConfigModel(defaultSelector, "", defaultModel)
		}
	}
	s.mu.Unlock()
}

func EnsureBootstrapState(ctx context.Context, statePath string, server config.ServerConfig, model config.ModelConfig, managerImage string, forceRecreate bool) error {
	return EnsureBootstrapStateWithLLM(ctx, statePath, server, config.SingleProfileLLM(model), managerImage, forceRecreate)
}

func EnsureBootstrapStateWithLLM(ctx context.Context, statePath string, server config.ServerConfig, llmCfg config.LLMConfig, managerImage string, forceRecreate bool) error {
	svc, err := NewServiceWithLLM(llmCfg, server, managerImage, statePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = svc.Close()
	}()
	return svc.EnsureBootstrapManager(ctx, forceRecreate)
}

func (svc *Service) EnsureBootstrapManager(ctx context.Context, forceRecreate bool) error {
	if svc == nil {
		return nil
	}
	_, err := svc.EnsureManager(ctx, forceRecreate)
	return err
}

func (s *Service) SetLifecycleObserver(observer LifecycleObserver) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.lifecycle = observer
	s.mu.Unlock()
}

func (s *Service) logBootstrapManagerBoxProgress(elapsed time.Duration) {
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

func (s *Service) EnsureManager(ctx context.Context, forceRecreate bool) (Agent, error) {
	return s.ensureManager(ctx, forceRecreate, "")
}

func (s *Service) ensureManager(ctx context.Context, forceRecreate bool, runtimeOverride string) (Agent, error) {
	if s == nil {
		return Agent{}, fmt.Errorf("agent service is required")
	}
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

func (s *Service) ensureCodexManager(ctx context.Context, forceRecreate bool) (Agent, error) {
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

	if _, err := locateCodexCLI(); err != nil {
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
	if err := s.validateMCPServers(ctx, RuntimeKindCodex, mcpServersSnapshotForAgent(managerMCPServers)); err != nil {
		return Agent{}, err
	}
	legacyCleanupKeys := s.legacyManagerSandboxCleanupKeys()
	if forceRecreate && strings.TrimSpace(existing.RuntimeID) != "" {
		if err := runtimeImpl.Delete(ctx, runtimeHandleForAgent(existing)); err != nil && !sandbox.IsNotFound(err) {
			return Agent{}, fmt.Errorf("remove existing manager runtime: %w", err)
		}
	}

	runtimeAgent := s.newCodexManagerAgent(managerDisplayName, managerDescription, managerInstructions, managerAvatar, managerCreatedAt, "", agentruntime.StateCreated, string(agentruntime.StateCreated), startProfile, detectionResults)
	applyManagerMCPServers(&runtimeAgent, managerMCPServers)
	runtimeProfile := s.runtimeProfileForAgentWithProfile(runtimeAgent, s.hydrateProfileFromCatalog(startProfile))
	provisionReq := agentruntime.ProvisionRequest{
		RuntimeID:     runtimeIDForAgentID(ManagerUserID),
		AgentID:       ManagerUserID,
		ParticipantID: ManagerParticipantID,
		AgentName:     managerDisplayName,
		Profile:       runtimeProfile,
		MCPServers:    cloneMCPServers(runtimeAgent.MCPServers),
	}
	if err := s.provisionRuntime(ctx, runtimeImpl, RuntimeKindCodex, provisionReq); err != nil {
		return Agent{}, fmt.Errorf("provision manager runtime: %w", err)
	}
	if _, err := s.persistManagerAgent(ctx, runtimeAgent, false); err != nil {
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
	applyManagerMCPServers(&manager, managerMCPServers)
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
	if err := s.cleanupLegacyManagerSandboxRuntime(ctx, legacyCleanupKeys); err != nil {
		log.Printf("skipping legacy manager sandbox cleanup after starting Codex manager: %v", err)
	}
	return persisted, nil
}

func (s *Service) cleanupLegacyManagerSandboxRuntime(ctx context.Context, keys []string) error {
	if s == nil {
		return nil
	}
	if len(keys) == 0 {
		return nil
	}
	if strings.EqualFold(s.sandboxProviderName(), unconfiguredSandboxProvider{}.Name()) {
		return nil
	}
	rt, err := s.ensureRuntime(ManagerUserID)
	if err != nil {
		return fmt.Errorf("open legacy manager sandbox runtime: %w", err)
	}
	runtimeHome, err := s.sandboxRuntimeHome(ManagerUserID)
	if err != nil {
		_ = s.closeRuntime("", rt)
		return err
	}
	defer func() {
		_ = s.closeRuntime(runtimeHome, rt)
	}()

	for _, key := range keys {
		if err := s.forceRemoveBox(ctx, rt, key); err != nil {
			if sandbox.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("remove legacy manager sandbox runtime %q: %w", key, err)
		}
		log.Printf("removed legacy manager sandbox runtime %q after starting Codex manager", key)
	}
	return nil
}

func (s *Service) legacyManagerSandboxCleanupKeys() []string {
	keys := make([]string, 0, 4)
	if s != nil {
		s.mu.RLock()
		for _, existing := range s.agents {
			if !isManagerAgent(existing) || !isGatewayRuntimeKind(strings.TrimSpace(existing.RuntimeKind)) {
				continue
			}
			keys = appendLookupKey(keys, existing.BoxID)
			keys = appendLookupKey(keys, existing.Name)
		}
		s.mu.RUnlock()
	}
	keys = appendLookupKey(keys, sandboxNameForAgentID(ManagerUserID))
	keys = appendLookupKey(keys, ManagerName)
	return keys
}

func (s *Service) managerMetadata() (name, description, instructions, avatar string, createdAt time.Time) {
	name = ManagerName
	if s == nil {
		return name, "", "", "", time.Time{}
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
	return name, "", "", "", time.Time{}
}

func managerMetadataFromAgent(existing Agent) (name, description, instructions, avatar string, createdAt time.Time) {
	name = strings.TrimSpace(existing.Name)
	if name == "" {
		name = ManagerName
	}
	return name, strings.TrimSpace(existing.Description), strings.TrimSpace(existing.Instructions), strings.TrimSpace(existing.Avatar), existing.CreatedAt.UTC()
}

func (s *Service) managerMCPServers() map[string]any {
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

func (s *Service) newCodexManagerAgent(name, description, instructions, avatar string, createdAt time.Time, handleID string, state agentruntime.State, status string, profile AgentProfile, detectionResults []ProfileDetectionResult) Agent {
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

func (s *Service) persistManagerAgent(ctx context.Context, manager Agent, syncLifecycle bool) (Agent, error) {
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
	manager.RuntimeOptions = nil
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
		manager.MCPServers = cloneMCPServers(existing.MCPServers)
		if existing.AgentProfile.EnvRestartRequired {
			manager.AgentProfile.EnvRestartRequired = true
		}
		if existing.AgentProfile.ImageUpgradeRequired {
			manager.AgentProfile.ImageUpgradeRequired = true
		}
	}
	for id, existing := range s.agents {
		if isManagerAgent(existing) && id != ManagerUserID {
			delete(s.agents, id)
		}
	}
	s.agents[ManagerUserID] = manager
	s.syncRuntimeRecordLocked(manager)
	if manager.AgentProfile.ProfileComplete {
		s.profileDefaults = cloneProfile(manager.AgentProfile)
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
	if syncLifecycle {
		if err := s.syncLifecycleForAgent(ctx, created); err != nil {
			return Agent{}, err
		}
	}
	return created, nil
}

func (s *Service) cleanupBootstrapManagerForRecreate(ctx context.Context, rt sandbox.Runtime, runtimeHome, runtimeKind string) (sandbox.Runtime, error) {
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

func (s *Service) managerSkillPreservationSourceRuntimeKind(targetRuntimeKind string) string {
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

func (s *Service) managerStartupProfile(ctx context.Context) (AgentProfile, []ProfileDetectionResult) {
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

func (s *Service) bootstrapManagerBoxIDOrName() string {
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

func (s *Service) syncRuntimeRecordLocked(a Agent) {
	if s == nil {
		return
	}
	rt := runtimeRecordForAgent(a)
	if rt.ID == "" {
		return
	}
	s.runtimeRecords[rt.ID] = rt
}

func (s *Service) deleteRuntimeRecordLocked(runtimeID string) {
	if s == nil {
		return
	}
	delete(s.runtimeRecords, normalizeRuntimeID(runtimeID, ""))
}

func (s *Service) bootstrapManagerLookupKeys() []string {
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

func (s *Service) Create(ctx context.Context, req CreateRequest) (Agent, error) {
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
		req.Spec, cleanup, err = s.resolveTemplateCreateSpec(ctx, req.Spec)
		if err != nil {
			return Agent{}, err
		}
		if cleanup != nil {
			defer cleanup()
		}
	}
	return s.createNew(ctx, req.Spec)
}

func (s *Service) resolveTemplateCreateSpec(ctx context.Context, spec CreateAgentSpec) (CreateAgentSpec, func(), error) {
	if s == nil {
		return CreateAgentSpec{}, nil, fmt.Errorf("agent service is required")
	}
	templateRef, expectedRole, usedDefault := s.templateRefForCreateSpec(spec)
	if templateRef == "" {
		return spec, nil, nil
	}
	if s.hub == nil {
		if usedDefault {
			return CreateAgentSpec{}, nil, fmt.Errorf("default %s template %q requires hub service, but hub service is not configured", expectedRole, templateRef)
		}
		return CreateAgentSpec{}, nil, fmt.Errorf("hub service is not configured")
	}

	item, err := s.hub.Get(ctx, templateRef)
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
	workspace, err := s.hub.FetchWorkspace(ctx, templateRef)
	if err != nil {
		if usedDefault {
			return CreateAgentSpec{}, nil, fmt.Errorf("fetch default %s template workspace %q: %w", expectedRole, templateRef, err)
		}
		return CreateAgentSpec{}, nil, err
	}

	cleanup := templateWorkspaceCleanup(item.Source.Kind, workspace)
	spec = applyTemplateDefaults(spec, item)
	spec = applyTemplateEnvDefaults(spec, item)
	if strings.TrimSpace(workspace.Kind) == hub.WorkspaceKindDir {
		spec.FromTemplate = strings.TrimSpace(workspace.Path)
	}
	return spec, cleanup, nil
}

func shouldResolveTemplateCreateSpec(spec CreateAgentSpec) bool {
	if strings.TrimSpace(spec.FromTemplate) != "" {
		return true
	}
	return shouldCreateWorkerSpec(spec)
}

func (s *Service) templateRefForCreateSpec(spec CreateAgentSpec) (templateRef, role string, usedDefault bool) {
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

func templateWorkspaceCleanup(kind string, workspace hub.WorkspaceRef) func() {
	if strings.TrimSpace(workspace.Kind) != hub.WorkspaceKindDir {
		return nil
	}
	switch strings.TrimSpace(kind) {
	case hub.RegistryKindBuiltin, hub.RegistryKindRemote:
	default:
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

func (s *Service) createNew(ctx context.Context, spec CreateAgentSpec) (Agent, error) {
	if isManagerCreateSpec(spec) {
		return s.EnsureManager(ctx, false)
	}
	if shouldCreateWorkerSpec(spec) {
		spec.Role = RoleWorker
		return s.CreateWorker(ctx, spec)
	}
	return Agent{}, fmt.Errorf("role must be one of %q or %q", RoleManager, RoleWorker)
}

func (s *Service) replace(ctx context.Context, req CreateRequest) (Agent, error) {
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
		if err := s.Delete(ctx, existing.ID); err != nil {
			return Agent{}, err
		}
		return s.CreateWorker(ctx, spec)
	}

	if err := s.Delete(ctx, existing.ID); err != nil {
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

func (s *Service) validateReplaceWorkerSpecBeforeDelete(ctx context.Context, spec CreateAgentSpec) error {
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

func (s *Service) Agent(id string) (Agent, bool) {
	a, ok := s.agentSnapshot(id)
	if !ok {
		return Agent{}, false
	}
	ctx := context.Background()
	return s.withRuntimeImageMigrationStatus(ctx, s.hydrateAgentStatus(ctx, a)), true
}

func (s *Service) AgentDisplayName(id string) (string, bool) {
	a, ok := s.agentSnapshot(id)
	if !ok {
		return "", false
	}
	name := strings.TrimSpace(a.Name)
	if name == "" {
		name = strings.TrimSpace(a.ID)
	}
	return name, name != ""
}

func (s *Service) agentSnapshot(id string) (Agent, bool) {
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

func (s *Service) agentByIDLocked(id string) (Agent, string, bool) {
	for _, key := range agentIDAliases(id) {
		if a, ok := s.agents[key]; ok {
			return a, key, true
		}
	}
	return Agent{}, "", false
}

func (s *Service) agentSnapshotByName(name string) (Agent, bool) {
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

func (s *Service) resolveAgentBox(ctx context.Context, rt sandbox.Runtime, got Agent) (sandbox.Instance, string, error) {
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

func (s *Service) refreshAgentBoxID(id string, got Agent, resolvedKey string, box sandbox.Instance) error {
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
	s.agents[key] = current
	s.syncRuntimeRecordLocked(current)
	return s.saveLocked()
}

func (s *Service) Start(ctx context.Context, id string) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, fmt.Errorf("agent id is required")
	}

	got, ok := s.Agent(id)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	if got.AgentProfile.EnvRestartRequired || got.AgentProfile.ImageUpgradeRequired {
		return s.Recreate(ctx, id)
	}
	startProfile := s.hydrateProfileFromCatalog(normalizeProfileForAgentRuntime(got.AgentProfile, got.RuntimeOptions, got.Name, got.Description, got.RuntimeKind, nil))
	if err := s.validateRuntimeConfig(ctx, strings.TrimSpace(got.RuntimeKind), runtimeConfigSnapshotForAgent(startProfile, got.RuntimeOptions)); err != nil {
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
	handle := runtimeHandleForAgent(got)
	state, err := runtimeImpl.Start(ctx, handle)
	if err != nil {
		if sandbox.IsNotFound(err) {
			return s.Recreate(ctx, id)
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
	if err := s.syncLifecycleForAgent(ctx, updated); err != nil {
		return Agent{}, err
	}
	return updated, nil
}

func (s *Service) Stop(ctx context.Context, id string) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, fmt.Errorf("agent id is required")
	}

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
	s.stopLifecycleAgent(updated.ID)
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("agent id is required")
	}

	s.mu.RLock()
	existing, _, ok := s.agentByIDLocked(id)
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}

	s.stopLifecycleAgent(existing.ID)

	if runtimeImpl, err := s.runtimeForKind(strings.TrimSpace(existing.RuntimeKind)); err == nil && strings.TrimSpace(existing.BoxID) != "" {
		if err := runtimeImpl.Delete(ctx, runtimeHandleForAgent(existing)); err != nil && !sandbox.IsNotFound(err) {
			return fmt.Errorf("remove agent box: %w", err)
		}
	} else {
		rt, ensureErr := s.ensureRuntime(existing.ID)
		if ensureErr != nil {
			return ensureErr
		}
		runtimeHome, homeErr := s.sandboxRuntimeHome(existing.ID)
		if homeErr != nil {
			return homeErr
		}
		if rt != nil {
			if err := s.stopAndForceRemoveBox(ctx, rt, existing); err != nil {
				return err
			}
			_ = s.closeRuntime(runtimeHome, rt)
		}
	}

	agentHome, err := s.agentHomeDir(existing.ID)
	if err != nil {
		return err
	}
	if err := removeAll(agentHome); err != nil {
		return fmt.Errorf("remove agent home: %w", err)
	}

	s.mu.Lock()

	current, key, ok := s.agentByIDLocked(id)
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("agent %q not found", id)
	}
	delete(s.agents, key)
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

func (s *Service) stopAndForceRemoveBox(ctx context.Context, rt sandbox.Runtime, got Agent) error {
	boxIDOrName := strings.TrimSpace(got.BoxID)
	if boxIDOrName == "" {
		boxIDOrName = sandboxNameForAgentID(got.ID)
	}
	if boxIDOrName == "" {
		boxIDOrName = strings.TrimSpace(got.Name)
	}
	box, resolvedKey, err := s.resolveAgentBox(ctx, rt, got)
	if err == nil && box != nil {
		if key := strings.TrimSpace(resolvedKey); key != "" {
			boxIDOrName = key
		}
		if stopErr := s.stopBox(ctx, box, sandbox.StopOptions{}); stopErr != nil && !sandbox.IsNotFound(stopErr) {
			_ = s.closeBox(box)
			return fmt.Errorf("stop agent box: %w", stopErr)
		}
		_ = s.closeBox(box)
	} else if err != nil && !sandbox.IsNotFound(err) {
		return fmt.Errorf("resolve agent box: %w", err)
	}
	if err := s.forceRemoveBox(ctx, rt, boxIDOrName); err != nil && !sandbox.IsNotFound(err) {
		return fmt.Errorf("remove agent box: %w", err)
	}
	return nil
}

func (s *Service) removeResolvedGatewayBox(ctx context.Context, rt sandbox.Runtime, box sandbox.Instance, info sandbox.Info, resolvedKey string) error {
	removeKey := gatewayBoxRemoveKey(info, resolvedKey)
	if removeKey == "" {
		return fmt.Errorf("agent box identifier is required")
	}
	if box != nil {
		if err := s.stopBox(ctx, box, sandbox.StopOptions{}); err != nil && !sandbox.IsNotFound(err) {
			_ = s.closeBox(box)
			return fmt.Errorf("stop agent box: %w", err)
		}
		_ = s.closeBox(box)
	}
	if err := s.forceRemoveBox(ctx, rt, removeKey); err != nil && !sandbox.IsNotFound(err) {
		return fmt.Errorf("remove agent box: %w", err)
	}
	return nil
}

func gatewayBoxRemoveKey(info sandbox.Info, resolvedKey string) string {
	for _, key := range []string{resolvedKey, info.ID, info.Name} {
		if key = strings.TrimSpace(key); key != "" {
			return key
		}
	}
	return ""
}

func sandboxInfoNeedsCanonicalAgentName(agentID, displayName string, info sandbox.Info, resolvedKey string) bool {
	canonicalName := strings.TrimSpace(sandboxNameForAgentID(agentID))
	if canonicalName == "" {
		return false
	}
	if name := strings.TrimSpace(info.Name); name != "" {
		return name != canonicalName
	}
	key := strings.TrimSpace(resolvedKey)
	if key == "" || key == canonicalName || key == strings.TrimSpace(info.ID) {
		return false
	}
	if display := strings.TrimSpace(displayName); display != "" && key == display {
		return true
	}
	if suffix := strings.TrimPrefix(canonicalAgentID(agentID), AgentIDPrefix); suffix != "" && key == suffix {
		return true
	}
	return false
}

func (s *Service) recreateLegacyNamedGatewayAgentBox(ctx context.Context, got Agent) (Agent, bool, error) {
	if !isGatewayRuntimeKind(strings.TrimSpace(got.RuntimeKind)) {
		return got, false, nil
	}
	rt, err := s.ensureRuntime(got.ID)
	if err != nil {
		return got, false, err
	}
	runtimeHome, err := s.sandboxRuntimeHome(got.ID)
	if err != nil {
		return got, false, err
	}
	defer func() {
		_ = s.closeRuntime(runtimeHome, rt)
	}()

	box, resolvedKey, err := s.resolveAgentBox(ctx, rt, got)
	if err != nil {
		if sandbox.IsNotFound(err) {
			return got, false, nil
		}
		return got, false, err
	}
	info, err := s.boxInfo(ctx, box)
	if err != nil {
		_ = s.closeBox(box)
		return got, false, fmt.Errorf("read agent box info: %w", err)
	}
	if !sandboxInfoNeedsCanonicalAgentName(got.ID, got.Name, info, resolvedKey) {
		_ = s.closeBox(box)
		return got, false, nil
	}

	startProfile := s.hydrateProfileFromCatalog(normalizeProfileForAgentRuntime(got.AgentProfile, got.RuntimeOptions, got.Name, got.Description, got.RuntimeKind, nil))
	if err := s.validateRuntimeConfig(ctx, strings.TrimSpace(got.RuntimeKind), runtimeConfigSnapshotForAgent(startProfile, got.RuntimeOptions)); err != nil {
		_ = s.closeBox(box)
		return got, false, err
	}
	if err := s.validateMCPServers(ctx, strings.TrimSpace(got.RuntimeKind), mcpServersSnapshotForAgent(got.MCPServers)); err != nil {
		_ = s.closeBox(box)
		return got, false, err
	}
	runtimeImpl, err := s.runtimeForKind(strings.TrimSpace(got.RuntimeKind))
	if err != nil {
		_ = s.closeBox(box)
		return got, false, err
	}
	if err := s.provisionRuntimeForAgent(ctx, runtimeImpl, got, ""); err != nil {
		_ = s.closeBox(box)
		return got, false, fmt.Errorf("provision agent runtime: %w", err)
	}

	log.Printf("agent %s sandbox %q uses legacy sandbox name %q; recreating as %q", got.ID, gatewayBoxRemoveKey(info, resolvedKey), strings.TrimSpace(info.Name), sandboxNameForAgentID(got.ID))
	if err := s.removeResolvedGatewayBox(ctx, rt, box, info, resolvedKey); err != nil {
		return got, false, err
	}
	box = nil

	newBox, newInfo, err := s.createGatewayBox(ctx, rt, got.Image, got.Name, got.ID, startProfile)
	if err != nil {
		return got, false, fmt.Errorf("create agent box with canonical name: %w", err)
	}
	defer func() {
		_ = s.closeBox(newBox)
	}()
	updated, err := s.updateRuntimeState(got.ID, agentruntime.Info{
		HandleID:  strings.TrimSpace(newInfo.ID),
		State:     agentruntime.State(newInfo.State),
		CreatedAt: newInfo.CreatedAt.UTC(),
	})
	if err != nil {
		return got, false, err
	}
	if err := s.syncLifecycleForAgent(ctx, updated); err != nil {
		return got, false, err
	}
	return updated, true, nil
}

func sandboxNameForAgentID(agentID string) string {
	return agentruntime.SandboxNameForAgentID(canonicalAgentID(agentID))
}

func removeAll(path string) error {
	return osRemoveAll(path)
}

func (s *Service) List() []Agent {
	return s.ListContext(context.Background())
}

// ListContext returns the persisted agent registry with best-effort live runtime status.
// Runtime status probes must honor ctx so API callers can remain responsive when a
// sandbox runtime is unavailable or contended.
func (s *Service) ListContext(ctx context.Context) []Agent {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.RLock()
	agents := sortedAgentsFromMap(s.agents)
	s.mu.RUnlock()
	for idx := range agents {
		agents[idx] = s.withRuntimeImageMigrationStatus(ctx, s.hydrateAgentStatus(ctx, agents[idx]))
	}
	return agents
}

func (s *Service) StartConfiguredAgents(ctx context.Context) error {
	if s == nil {
		return nil
	}
	agents := s.startupAgentCandidates()
	var startErr error
	for _, a := range agents {
		if err := ctx.Err(); err != nil {
			return err
		}
		live := s.hydrateAgentStatus(ctx, a)
		_, reconciled, err := s.recreateLegacyNamedGatewayAgentBox(ctx, live)
		if err != nil {
			startErr = errors.Join(startErr, fmt.Errorf("%s: %w", live.Name, err))
			continue
		}
		if reconciled {
			continue
		}
		if isRuntimeRunning(live) {
			continue
		}
		if _, err := s.Start(ctx, live.ID); err != nil {
			startErr = errors.Join(startErr, fmt.Errorf("%s: %w", live.Name, err))
		}
	}
	return startErr
}

func (s *Service) startupAgentCandidates() []Agent {
	s.mu.RLock()
	agents := sortedAgentsFromMap(s.agents)
	s.mu.RUnlock()

	candidates := agents[:0]
	for _, a := range agents {
		if isManagerAgent(a) || !isAgentProfileComplete(a) {
			continue
		}
		rk := a.RuntimeKind
		if strings.EqualFold(normalizeRole(a.Role), RoleWorker) && rk != "" && !isGatewayRuntimeKind(rk) {
			continue
		}
		candidates = append(candidates, a)
	}
	return candidates
}

func isAgentProfileComplete(a Agent) bool {
	return a.ProfileComplete || a.AgentProfile.ProfileComplete
}

func isRuntimeRunning(a Agent) bool {
	return strings.EqualFold(strings.TrimSpace(a.Status), string(sandbox.StateRunning))
}

func (s *Service) CreateWorker(ctx context.Context, spec CreateAgentSpec) (Agent, error) {
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

	s.mu.RLock()
	idExists := false
	if _, _, ok := s.agentByIDLocked(id); ok {
		idExists = true
	}
	nameExists := s.hasNameLocked(name)
	s.mu.RUnlock()
	if idExists {
		return Agent{}, fmt.Errorf("agent id %q already exists", id)
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
		if _, err := locateCodexCLI(); err != nil {
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
	if err := s.provisionRuntime(ctx, runtimeImpl, runtimeKind, agentruntime.ProvisionRequest{
		RuntimeID:        runtimeIDForAgentID(id),
		AgentID:          id,
		ParticipantID:    participantIDForAgent(name, id),
		AgentName:        name,
		Instructions:     instructions,
		Profile:          runtimeProfile,
		RuntimeOptions:   utils.CloneAnyMap(spec.RuntimeOptions),
		MCPServers:       cloneMCPServers(spec.MCPServers),
		WorkspaceOverlay: strings.TrimSpace(spec.FromTemplate),
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
		return s.persistCreatedWorker(ctx, id, name, description, instructions, image, avatar, runtimeKind, runtimeName, sandboxed, resolvedProfile, spec.RuntimeOptions, spec.MCPServers, agentruntime.Info{
			HandleID:  strings.TrimSpace(info.ID),
			State:     agentruntime.State(info.State),
			CreatedAt: info.CreatedAt.UTC(),
		})
	}
	if runtimeKind == RuntimeKindCodex {
		if err := s.persistStartingWorker(ctx, id, name, description, instructions, image, avatar, runtimeKind, runtimeName, sandboxed, resolvedProfile, spec.RuntimeOptions, spec.MCPServers); err != nil {
			return Agent{}, err
		}
		defer func() {
			if err != nil {
				_ = s.removeStartingWorker(ctx, id)
			}
		}()
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
	info := agentruntime.Info{
		HandleID:  strings.TrimSpace(handle.HandleID),
		State:     agentruntime.StateRunning,
		CreatedAt: time.Now().UTC(),
	}

	return s.persistCreatedWorker(ctx, id, name, description, instructions, image, avatar, runtimeKind, runtimeName, sandboxed, resolvedProfile, spec.RuntimeOptions, spec.MCPServers, info)
}

func (s *Service) persistStartingWorker(ctx context.Context, id, name, description, instructions, image, avatar, runtimeKind, runtimeName string, sandboxEnabled bool, profile AgentProfile, runtimeOptions map[string]any, mcpServers map[string]any) error {
	s.mu.Lock()

	if _, _, ok := s.agentByIDLocked(id); ok {
		s.mu.Unlock()
		return fmt.Errorf("agent id %q already exists", id)
	}
	if s.hasNameLocked(name) {
		s.mu.Unlock()
		return fmt.Errorf("agent name %q already exists", name)
	}

	worker := newWorkerAgent(id, name, description, instructions, image, avatar, runtimeKind, runtimeName, sandboxEnabled, profile, runtimeOptions, mcpServers, agentruntime.Info{
		State:     agentruntime.StateCreated,
		CreatedAt: time.Now().UTC(),
	})
	s.agents[worker.ID] = worker
	s.syncRuntimeRecordLocked(worker)
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		_ = s.removeStartingWorker(ctx, id)
		return err
	}
	return nil
}

func (s *Service) removeStartingWorker(ctx context.Context, id string) error {
	s.mu.Lock()
	current, ok := s.agents[id]
	if ok && strings.TrimSpace(current.BoxID) == "" && strings.EqualFold(strings.TrimSpace(current.Status), string(agentruntime.StateCreated)) {
		delete(s.agents, id)
		s.deleteRuntimeRecordLocked(current.RuntimeID)
	}
	err := s.saveLocked()
	s.mu.Unlock()
	return err
}

func (s *Service) persistCreatedWorker(ctx context.Context, id, name, description, instructions, image, avatar, runtimeKind, runtimeName string, sandboxEnabled bool, profile AgentProfile, createRuntimeExt map[string]any, mcpServers map[string]any, info agentruntime.Info) (Agent, error) {
	s.mu.Lock()

	if existing, _, ok := s.agentByIDLocked(id); ok && !isStartingWorker(existing) {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent id %q already exists", id)
	}
	if s.hasNameLockedExcept(name, id) {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent name %q already exists", name)
	}

	worker := newWorkerAgent(id, name, description, instructions, image, avatar, runtimeKind, runtimeName, sandboxEnabled, profile, createRuntimeExt, mcpServers, info)
	s.agents[worker.ID] = worker
	s.syncRuntimeRecordLocked(worker)
	if worker.AgentProfile.ProfileComplete {
		s.profileDefaults = cloneProfile(worker.AgentProfile)
	}
	if err := s.saveLocked(); err != nil {
		delete(s.agents, worker.ID)
		s.deleteRuntimeRecordLocked(worker.RuntimeID)
		s.mu.Unlock()
		return Agent{}, err
	}
	created := *cloneAgent(&worker)
	s.mu.Unlock()
	if err := s.syncLifecycleForAgent(ctx, created); err != nil {
		return Agent{}, err
	}
	return created, nil
}

func newWorkerAgent(id, name, description, instructions, image, avatar, runtimeKind, runtimeName string, sandboxEnabled bool, profile AgentProfile, runtimeOptions map[string]any, mcpServers map[string]any, info agentruntime.Info) Agent {
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
	return Agent{
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

func (s *Service) provisionRuntimeRequest(ctx context.Context, rt agentruntime.Runtime, runtimeKind string, req agentruntime.ProvisionRequest) error {
	if rt == nil {
		return fmt.Errorf("runtime is required")
	}
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

func (s *Service) provisionRuntime(ctx context.Context, rt agentruntime.Runtime, runtimeKind string, req agentruntime.ProvisionRequest) error {
	if err := s.provisionRuntimeRequest(ctx, rt, runtimeKind, req); err != nil {
		return err
	}
	if err := s.installDefaultSystemSkills(req.AgentID, runtimeKind); err != nil {
		return fmt.Errorf("install default system skills: %w", err)
	}
	return nil
}

func (s *Service) provisionRuntimeForAgent(ctx context.Context, rt agentruntime.Runtime, got Agent, workspaceOverlay string) error {
	if s == nil || rt == nil {
		return nil
	}
	return s.provisionRuntime(ctx, rt, strings.TrimSpace(got.RuntimeKind), agentruntime.ProvisionRequest{
		RuntimeID:        normalizeRuntimeID(got.RuntimeID, got.ID),
		AgentID:          strings.TrimSpace(got.ID),
		ParticipantID:    participantIDForAgent(got.Name, got.ID),
		AgentName:        strings.TrimSpace(got.Name),
		Instructions:     strings.TrimSpace(got.Instructions),
		Profile:          s.runtimeProfileForAgent(got),
		RuntimeOptions:   utils.CloneAnyMap(got.RuntimeOptions),
		MCPServers:       cloneMCPServers(got.MCPServers),
		WorkspaceOverlay: strings.TrimSpace(workspaceOverlay),
	})
}

func participantIDForAgent(agentName, agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if managerGatewayMatch(agentName, agentID) {
		return ManagerParticipantID
	}
	return participantIDFromAgentID(agentID)
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

func (s *Service) gatewayProvisionRequest(runtimeKind, agentName, agentID string) (*agentruntime.GatewayProvision, error) {
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

func (s *Service) resolveManagerBaseURL(server config.ServerConfig) string {
	return resolveManagerBaseURLForSandboxProvider(server, s.sandboxProviderName())
}

func (s *Service) StreamLogs(ctx context.Context, id string, follow bool, lines int, w io.Writer) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("agent id is required")
	}
	if w == nil {
		return fmt.Errorf("log writer is required")
	}
	if lines <= 0 {
		lines = 20
	}

	got, ok := s.agentSnapshot(id)
	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}
	runtimeImpl, err := s.runtimeForKind(strings.TrimSpace(got.RuntimeKind))
	if err != nil {
		return err
	}
	streamer, ok := runtimeImpl.(agentruntime.LogStreamer)
	if !ok {
		return fmt.Errorf("runtime %q does not support log streaming", runtimeImpl.Kind())
	}
	return streamer.StreamLogs(ctx, runtimeHandleForAgent(got), agentruntime.LogOptions{
		Follow: follow,
		Tail:   lines,
		Writer: w,
	})
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

func (s *Service) hydrateAgentStatus(ctx context.Context, a Agent) Agent {
	a = *cloneAgent(&a)
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
		return statusAfterHydrateFailure(a, "read_runtime_info", err)
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
	if isSandboxRuntimeContention(err) {
		slog.Debug("agent status refresh skipped; sandbox runtime is busy", attrs...)
		return
	}
	slog.Warn("agent status refresh failed; keeping last known status",
		attrs...,
	)
}

func isSandboxRuntimeContention(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "failed to acquire runtime lock") ||
		strings.Contains(msg, "another boxliteruntime is already using directory")
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

func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var closeErr error
	for name, rt := range s.runtimes {
		if err := rt.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		delete(s.runtimes, name)
	}
	return closeErr
}

func (s *Service) hasNameLocked(name string) bool {
	return s.hasNameLockedExcept(name, "")
}

func (s *Service) hasNameLockedExcept(name, exceptID string) bool {
	for _, existing := range s.agents {
		if strings.TrimSpace(exceptID) != "" && strings.EqualFold(existing.ID, exceptID) {
			continue
		}
		if strings.EqualFold(existing.Name, name) {
			return true
		}
	}
	return false
}
