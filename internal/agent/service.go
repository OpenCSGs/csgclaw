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
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"csgclaw/internal/config"
	"csgclaw/internal/hub"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/runtime/picoclawsandbox"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/utils"
)

const (
	ManagerName          = "manager"
	ManagerParticipantID = "manager"
	ManagerUserID        = "u-manager"
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

var defaultSandboxProvider sandbox.Provider = unconfiguredSandboxProvider{}
var testDefaultServiceOption ServiceOption

const removeAllRetryAttempts = 5

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
			return ensureRuntime(s, ManagerName)
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
	sandbox                sandbox.Provider
	mu                     sync.RWMutex
	runtimes               map[string]sandbox.Runtime
	agents                 map[string]Agent
	runtimeRecords         map[string]RuntimeRecord
	runtimeRegistry        map[string]agentruntime.Runtime
	lifecycle              LifecycleObserver
	profileDefaults        AgentProfile
	detectionResults       []ProfileDetectionResult

	// gatewayWorkPhase is set by createGatewayBox for bootstrap progress logs (best-effort if concurrent).
	gatewayWorkPhase atomic.Uint32
}

type ServiceOption func(*Service) error

type templateService interface {
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
	workspaceRoot, err := s.agentWorkspaceRoot(got.Name, got.RuntimeKind)
	if err != nil {
		return hub.PublishSpec{}, err
	}
	return hub.PublishSpec{
		ID:          got.Name,
		Name:        got.Name,
		Description: got.Description,
		Role:        got.Role,
		RuntimeKind: got.RuntimeKind,
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

func (s *Service) useOpenClawGateway() bool {
	return s != nil && s.gatewayRuntimeKind() == RuntimeKindOpenClawSandbox
}

func NewService(model config.ModelConfig, server config.ServerConfig, managerImage, statePath string, opts ...ServiceOption) (*Service, error) {
	return NewServiceWithLLM(config.SingleProfileLLM(model), server, managerImage, statePath, opts...)
}

func NewServiceWithLLM(llmCfg config.LLMConfig, server config.ServerConfig, managerImage, statePath string, opts ...ServiceOption) (*Service, error) {
	// agent.Service owns the persisted registry and runtime selection.
	if strings.TrimSpace(managerImage) == "" {
		return nil, fmt.Errorf("manager image is required")
	}
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
	_, defaultModel, err := svc.llm.Resolve("")
	if err != nil {
		return err
	}
	modelCfg := defaultModel
	svc.mu.RLock()
	if manager, ok := svc.agents[ManagerUserID]; ok {
		profile := normalizeProfileForAgentRuntime(manager.AgentProfile, manager.RuntimeOptions, manager.Name, manager.Description, manager.RuntimeKind, nil)
		if profile.ProfileComplete {
			modelCfg = modelConfigFromProfile(profile)
		}
	}
	svc.mu.RUnlock()
	recreateForParticipantBridgeConfig := !forceRecreate && agentPicoClawConfigNeedsParticipantRecreate(ManagerName, ManagerParticipantID)
	if _, err := ensureAgentPicoClawConfigForParticipant(ManagerName, ManagerParticipantID, ManagerUserID, svc.server, modelCfg); err != nil {
		return err
	}
	if recreateForParticipantBridgeConfig {
		log.Printf("bootstrap manager PicoClaw config uses legacy bot bridge fields; recreating manager to load participant bridge config")
	}
	_, err = svc.EnsureManager(ctx, forceRecreate || recreateForParticipantBridgeConfig)
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
	return s.ensureManager(ctx, forceRecreate, "", "")
}

func (s *Service) ensureManager(ctx context.Context, forceRecreate bool, imageOverride, runtimeOverride string) (_ Agent, retErr error) {
	if s == nil {
		return Agent{}, fmt.Errorf("agent service is required")
	}
	managerAvatar := ""
	s.mu.RLock()
	if existing, ok := s.agents[ManagerUserID]; ok {
		managerAvatar = strings.TrimSpace(existing.Avatar)
	} else {
		for _, existing := range s.agents {
			if isManagerAgent(existing) {
				managerAvatar = strings.TrimSpace(existing.Avatar)
				break
			}
		}
	}
	s.mu.RUnlock()
	runtimeKind := runtimeKindForGatewayRuntime(runtimeOverride)
	if strings.TrimSpace(runtimeOverride) != "" && runtimeKind == "" {
		return Agent{}, fmt.Errorf("gateway runtime %q is not supported", runtimeOverride)
	}
	if runtimeKind == "" {
		runtimeKind = s.gatewayRuntimeKind()
	}

	managerImage := strings.TrimSpace(imageOverride)
	if runtimeKind != s.gatewayRuntimeKind() && managerImage == "" {
		return Agent{}, fmt.Errorf("image is required when changing gateway runtime_kind to %q", runtimeKind)
	}
	previousGatewayRuntime := ""
	previousManagerImage := ""
	shouldUpdateGatewayDefaults := runtimeKind != s.gatewayRuntimeKind() || managerImage != ""
	if shouldUpdateGatewayDefaults {
		s.mu.Lock()
		previousGatewayRuntime = s.gatewayRuntime
		previousManagerImage = s.managerImage
		s.gatewayRuntime = runtimeKind
		if managerImage != "" {
			s.managerImage = managerImage
		}
		managerImage = s.managerImage
		s.mu.Unlock()
		defer func() {
			if retErr == nil {
				return
			}
			s.mu.Lock()
			s.gatewayRuntime = previousGatewayRuntime
			s.managerImage = previousManagerImage
			s.mu.Unlock()
		}()
	} else {
		s.mu.RLock()
		managerImage = s.managerImage
		s.mu.RUnlock()
	}
	startProfile, detectionResults := s.managerStartupProfile(ctx)
	provisionBootstrapManagerRuntime := func() error {
		if !startProfile.ProfileComplete {
			return nil
		}
		runtimeImpl, err := s.runtimeForKind(runtimeKind)
		if err != nil {
			return err
		}
		if err := s.provisionRuntime(ctx, runtimeImpl, runtimeKind, agentruntime.ProvisionRequest{
			RuntimeID:     runtimeIDForAgentID(ManagerUserID),
			AgentID:       ManagerUserID,
			ParticipantID: ManagerParticipantID,
			AgentName:     ManagerName,
			Profile:       s.runtimeProfileForKind(runtimeKind, ManagerUserID, ManagerName, "", startProfile),
		}); err != nil {
			return fmt.Errorf("provision bootstrap manager runtime: %w", err)
		}
		return nil
	}
	rt, box, err := s.lookupBootstrapManager(ctx)
	if err != nil {
		return Agent{}, err
	}
	runtimeHome, err := s.sandboxRuntimeHome(ManagerName)
	if err != nil {
		return Agent{}, err
	}
	defer func() {
		_ = s.closeRuntime(runtimeHome, rt)
	}()
	if forceRecreate {
		rt, err = s.cleanupBootstrapManagerForRecreate(ctx, rt, runtimeHome, runtimeKind)
		if err != nil {
			return Agent{}, err
		}
		box = nil
	}
	if err := provisionBootstrapManagerRuntime(); err != nil {
		return Agent{}, err
	}
	if !startProfile.ProfileComplete {
		now := time.Now().UTC()
		runtimeKind := s.gatewayRuntimeKind()
		s.mu.Lock()
		manager := s.agents[ManagerUserID]
		if manager.ID == "" || forceRecreate {
			manager = Agent{
				ID:          ManagerUserID,
				Name:        ManagerName,
				RuntimeID:   runtimeIDForAgentID(ManagerUserID),
				RuntimeKind: runtimeKind,
				Image:       managerImage,
				Avatar:      managerAvatar,
				Status:      "profile_incomplete",
				CreatedAt:   now,
				Role:        RoleManager,
			}
		}
		manager.RuntimeKind = runtimeKind
		manager.AgentProfile = startProfile
		manager.ProfileComplete = false
		manager.DetectionResults = detectionResults
		manager.Profile = profileSelector(startProfile)
		s.agents[ManagerUserID] = manager
		s.syncRuntimeRecordLocked(manager)
		s.detectionResults = detectionResults
		err := s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			return Agent{}, err
		}
		return *cloneAgent(&manager), nil
	}

	var info sandbox.Info
	createdBootstrapManagerBox := false
	if box == nil {
		log.Printf("bootstrap manager box %q not found, creating it with image %q", ManagerName, managerImage)
		log.Printf("if the image is not present locally, the first pull may take a while")
		progressDone := make(chan struct{})
		waitStarted := time.Now()
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-progressDone:
					return
				case <-ticker.C:
					s.logBootstrapManagerBoxProgress(time.Since(waitStarted))
				}
			}
		}()
		box, info, err = s.createGatewayBox(ctx, rt, managerImage, ManagerName, ManagerUserID, startProfile)
		close(progressDone)
		if err != nil {
			return Agent{}, fmt.Errorf("create bootstrap manager box: %w", err)
		}
		createdBootstrapManagerBox = true
		log.Printf("bootstrap manager box %q created", ManagerName)
	} else {
		info, err = s.boxInfo(ctx, box)
		if err != nil {
			return Agent{}, fmt.Errorf("read bootstrap manager box info: %w", err)
		}
		if info.State != sandbox.StateRunning {
			if err := s.startBox(ctx, box); err != nil {
				return Agent{}, fmt.Errorf("start bootstrap manager box: %w", err)
			}
			info, err = s.boxInfo(ctx, box)
			if err != nil {
				return Agent{}, fmt.Errorf("read bootstrap manager box info after start: %w", err)
			}
		}
	}
	defer func() {
		_ = s.closeBox(box)
	}()

	s.mu.Lock()
	defer s.mu.Unlock()

	if createdBootstrapManagerBox {
		startProfile.EnvRestartRequired = false
	}
	manager := Agent{
		ID:               ManagerUserID,
		Name:             ManagerName,
		RuntimeID:        runtimeIDForAgentID(ManagerUserID),
		RuntimeKind:      s.gatewayRuntimeKind(),
		Image:            managerImage,
		Avatar:           managerAvatar,
		BoxID:            info.ID,
		Status:           string(info.State),
		CreatedAt:        info.CreatedAt.UTC(),
		Profile:          profileSelector(startProfile),
		AgentProfile:     startProfile,
		ProfileComplete:  true,
		DetectionResults: detectionResults,
		Role:             RoleManager,
	}
	for id, a := range s.agents {
		if isManagerAgent(a) && id != manager.ID {
			delete(s.agents, id)
		}
	}
	s.agents[manager.ID] = manager
	s.syncRuntimeRecordLocked(manager)
	s.profileDefaults = cloneProfile(startProfile)
	s.detectionResults = detectionResults
	if err := s.saveLocked(); err != nil {
		return Agent{}, err
	}
	return *cloneAgent(&manager), nil
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
	managerHome, err := agentHomeDir(ManagerName)
	if err != nil {
		return nil, err
	}
	sourceRuntimeKind := s.managerSkillPreservationSourceRuntimeKind(runtimeKind)
	restoreSkills, cleanupSkills, err := s.prepareWorkspaceSkillsPreservation(ManagerName, sourceRuntimeKind, runtimeKind, RoleManager)
	if err != nil {
		return nil, fmt.Errorf("prepare bootstrap manager skills preservation: %w", err)
	}
	if cleanupSkills != nil {
		defer cleanupSkills()
	}
	if err := removeAllWithRetry(managerHome); err != nil {
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
		profile := cloneProfile(existing.AgentProfile)
		results := append([]ProfileDetectionResult(nil), existing.DetectionResults...)
		s.mu.RUnlock()
		return normalizeProfile(profile, ManagerName, existing.Description), results
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
	keys := []string{primary}
	if primary != ManagerName {
		keys = append(keys, ManagerName)
	}
	return keys
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Agent, error) {
	if req.Replace && strings.TrimSpace(req.Spec.FromTemplate) != "" {
		return Agent{}, fmt.Errorf("agent create --replace does not support from_template")
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
	return isManagerCreateSpec(spec) || shouldCreateWorkerSpec(spec)
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

func validateDefaultTemplateCompatibility(expectedRole string, spec CreateAgentSpec, item hub.Template, templateRef string) error {
	if actualRole := normalizeRole(item.Role); actualRole != expectedRole {
		if actualRole == "" {
			return fmt.Errorf("default %s template %q does not identify itself as a %s template", expectedRole, templateRef, expectedRole)
		}
		return fmt.Errorf("default %s template %q points to a %s template", expectedRole, templateRef, actualRole)
	}
	requestedRuntime := spec.RuntimeKind
	templateRuntime := item.RuntimeKind
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
	id := normalizeCreateID(spec.ID)
	if id == "" {
		return Agent{}, fmt.Errorf("agent create --replace requires id")
	}

	s.mu.RLock()
	existing, ok := s.agents[id]
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
		if strings.TrimSpace(spec.RuntimeKind) == "" {
			spec.RuntimeKind = existing.RuntimeKind
		}
		if strings.TrimSpace(spec.Role) == "" {
			spec.Role = existing.Role
		}
	}

	if isManagerAgent(existing) || isManagerCreateSpec(spec) {
		managerImageOverride := s.managerImageOverrideForReplace(ctx, existing, req, spec.RuntimeKind)
		return s.ensureManager(ctx, true, managerImageOverride, spec.RuntimeKind)
	}
	if shouldCreateWorkerSpec(spec) || strings.EqualFold(existing.Role, RoleWorker) {
		if err := s.Delete(ctx, existing.ID); err != nil {
			return Agent{}, err
		}
		spec.Role = RoleWorker
		return s.CreateWorker(ctx, spec)
	}

	if err := s.Delete(ctx, existing.ID); err != nil {
		return Agent{}, err
	}
	return s.createNew(ctx, spec)
}

func replaceImageOverride(req CreateRequest) string {
	if len(req.FieldMask) == 0 {
		return req.Spec.Image
	}
	for _, field := range req.FieldMask {
		if strings.EqualFold(strings.TrimSpace(field), "image") {
			return req.Spec.Image
		}
	}
	return ""
}

func (s *Service) managerImageOverrideForReplace(ctx context.Context, existing Agent, req CreateRequest, runtimeKind string) string {
	requested := strings.TrimSpace(replaceImageOverride(req))
	if requested == "" {
		return ""
	}
	if requested != strings.TrimSpace(existing.Image) {
		return requested
	}
	if runtimeKindForGatewayRuntime(runtimeKind) != runtimeKindForGatewayRuntime(existing.RuntimeKind) {
		return requested
	}
	if latest, ok := s.currentDefaultImageForAgent(ctx, existing); ok && imageNeedsDefaultRecreate(existing.Image, latest) {
		return latest
	}
	return requested
}

func mergeReplaceSpec(existing Agent, next CreateAgentSpec, fieldMask []string) (CreateAgentSpec, error) {
	merged := CreateAgentSpec{
		ID:             existing.ID,
		Name:           existing.Name,
		Description:    existing.Description,
		Image:          existing.Image,
		Avatar:         existing.Avatar,
		RuntimeKind:    existing.RuntimeKind,
		Role:           existing.Role,
		Status:         existing.Status,
		CreatedAt:      existing.CreatedAt,
		Profile:        existing.Profile,
		RuntimeOptions: utils.CloneAnyMap(existing.RuntimeOptions),
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
		case "image":
			merged.Image = next.Image
		case "avatar":
			merged.Avatar = next.Avatar
		case "runtime_kind":
			merged.RuntimeKind = next.RuntimeKind
		case "role":
			merged.Role = next.Role
		case "status":
			merged.Status = next.Status
		case "created_at":
			merged.CreatedAt = next.CreatedAt
		case "profile":
			merged.Profile = next.Profile
			if strings.TrimSpace(next.Profile) != "" {
				merged.AgentProfile = AgentProfile{}
			}
		case "agent_profile":
			merged.AgentProfile = cloneProfile(next.AgentProfile)
		case "runtime_options":
			merged.RuntimeOptions = utils.CloneAnyMap(next.RuntimeOptions)
		default:
			return CreateAgentSpec{}, fmt.Errorf("unsupported agent field mask path %q", field)
		}
	}
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
	if strings.EqualFold(strings.TrimSpace(id), ManagerName) {
		return ManagerUserID
	}
	return strings.TrimSpace(id)
}

func (s *Service) Agent(id string) (Agent, bool) {
	a, ok := s.agentSnapshot(id)
	if !ok {
		return Agent{}, false
	}
	ctx := context.Background()
	return s.withRuntimeImageMigrationStatus(ctx, s.hydrateAgentStatus(ctx, a)), true
}

func (s *Service) agentSnapshot(id string) (Agent, bool) {
	if s == nil {
		return Agent{}, false
	}
	s.mu.RLock()
	a, ok := s.agents[strings.TrimSpace(id)]
	s.mu.RUnlock()
	if !ok {
		return Agent{}, false
	}
	return *cloneAgent(&a), true
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
	keys := make([]string, 0, 2)
	if boxID := strings.TrimSpace(got.BoxID); boxID != "" {
		keys = append(keys, boxID)
	}
	if name := strings.TrimSpace(got.Name); name != "" {
		if len(keys) == 0 || keys[0] != name {
			keys = append(keys, name)
		}
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

	current, ok := s.agents[id]
	if !ok {
		return nil
	}
	if strings.TrimSpace(current.BoxID) == info.ID {
		return nil
	}
	current.BoxID = info.ID
	s.agents[id] = current
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
	if err := s.ensureCodexResponsesAPI(ctx, strings.TrimSpace(got.RuntimeKind), got.AgentProfile); err != nil {
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
	existing, ok := s.agents[id]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}

	s.stopLifecycleAgent(id)

	if runtimeImpl, err := s.runtimeForKind(strings.TrimSpace(existing.RuntimeKind)); err == nil && strings.TrimSpace(existing.BoxID) != "" {
		if err := runtimeImpl.Delete(ctx, runtimeHandleForAgent(existing)); err != nil && !sandbox.IsNotFound(err) {
			return fmt.Errorf("remove agent box: %w", err)
		}
	} else {
		rt, ensureErr := s.ensureRuntime(existing.Name)
		if ensureErr != nil {
			return ensureErr
		}
		runtimeHome, homeErr := s.sandboxRuntimeHome(existing.Name)
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

	agentHome, err := agentHomeDir(existing.Name)
	if err != nil {
		return err
	}
	if err := removeAllWithRetry(agentHome); err != nil {
		return fmt.Errorf("remove agent home: %w", err)
	}

	s.mu.Lock()

	current, ok := s.agents[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("agent %q not found", id)
	}
	delete(s.agents, id)
	s.deleteRuntimeRecordLocked(current.RuntimeID)
	runtimeHome, err := s.sandboxRuntimeHome(current.Name)
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

func removeAllWithRetry(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("path is required")
	}

	var lastErr error
	for attempt := 0; attempt < removeAllRetryAttempts; attempt++ {
		if err := osRemoveAll(path); err == nil || os.IsNotExist(err) {
			return nil
		} else {
			lastErr = err
			// Defensive retry: BoxLite runtime cleanup can briefly lag behind Close(),
			// so agent home removal may transiently fail with "directory not empty".
			// If runtime shutdown semantics improve later, prefer fixing that timing
			// instead of relying on retries here.
			if !isRetryableRemoveAllError(err) || attempt == removeAllRetryAttempts-1 {
				return err
			}
		}
		time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
	}
	return lastErr
}

func isRetryableRemoveAllError(err error) bool {
	if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EACCES) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "directory not empty") || strings.Contains(lower, "permission denied")
}

func (s *Service) List() []Agent {
	s.mu.RLock()
	agents := sortedAgentsFromMap(s.agents)
	s.mu.RUnlock()
	ctx := context.Background()
	localImages := s.localImageCandidates(ctx)
	for idx := range agents {
		agents[idx] = s.withRuntimeImageMigrationStatusFromCandidates(ctx, s.hydrateAgentStatus(ctx, agents[idx]), localImages)
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
	image := strings.TrimSpace(spec.Image)
	avatar := strings.TrimSpace(spec.Avatar)
	runtimeKind := strings.TrimSpace(spec.RuntimeKind)
	switch {
	case name == "":
		return Agent{}, fmt.Errorf("name is required")
	case strings.EqualFold(name, ManagerName):
		return Agent{}, fmt.Errorf("name %q is reserved", name)
	}
	if id == "" {
		// id = fmt.Sprintf("%s-%d", RoleWorker, time.Now().UnixNano())
		id = fmt.Sprintf("u-%s", name)
	}

	s.mu.RLock()
	idExists := false
	if _, ok := s.agents[id]; ok {
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
	case runtimeKind == "":
		return Agent{}, fmt.Errorf("runtime_kind is required")
	case isGatewayRuntimeKind(runtimeKind) && image == "":
		return Agent{}, fmt.Errorf("image is required for runtime_kind %q", runtimeKind)
	}

	runtimeImpl, err := s.runtimeForKind(runtimeKind)
	if err != nil {
		return Agent{}, err
	}
	resolvedProfile, err := s.profileForCreateRequest(ctx, &spec)
	if err != nil {
		return Agent{}, err
	}
	if err := s.ensureCodexResponsesAPI(ctx, runtimeKind, resolvedProfile); err != nil {
		return Agent{}, err
	}
	runtimeProfile := s.runtimeProfileForKind(runtimeKind, id, name, description, resolvedProfile)
	if err := s.provisionRuntime(ctx, runtimeImpl, runtimeKind, agentruntime.ProvisionRequest{
		RuntimeID:        runtimeIDForAgentID(id),
		AgentID:          id,
		ParticipantID:    participantIDForAgent(name, id),
		AgentName:        name,
		Profile:          runtimeProfile,
		WorkspaceOverlay: strings.TrimSpace(spec.FromTemplate),
	}); err != nil {
		return Agent{}, fmt.Errorf("provision worker runtime: %w", err)
	}
	if testCreateGatewayBoxHook != nil && isGatewayRuntimeKind(runtimeKind) {
		rt, err := s.ensureRuntime(name)
		if err != nil {
			return Agent{}, err
		}
		runtimeHome, err := s.sandboxRuntimeHome(name)
		if err != nil {
			return Agent{}, err
		}
		defer func() {
			_ = s.closeRuntime(runtimeHome, rt)
		}()
		box, info, err := s.createGatewayBox(ctx, rt, image, name, id, resolvedProfile)
		if err != nil {
			return Agent{}, fmt.Errorf("create worker box: %w", err)
		}
		defer func() {
			_ = s.closeBox(box)
		}()
		return s.persistCreatedWorker(ctx, id, name, description, image, avatar, runtimeKind, resolvedProfile, spec.RuntimeOptions, agentruntime.Info{
			HandleID:  strings.TrimSpace(info.ID),
			State:     agentruntime.State(info.State),
			CreatedAt: info.CreatedAt.UTC(),
		})
	}
	if runtimeKind == RuntimeKindCodex {
		if err := s.persistStartingWorker(ctx, id, name, description, image, avatar, runtimeKind, resolvedProfile, spec.RuntimeOptions); err != nil {
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

	return s.persistCreatedWorker(ctx, id, name, description, image, avatar, runtimeKind, resolvedProfile, spec.RuntimeOptions, info)
}

func (s *Service) persistStartingWorker(ctx context.Context, id, name, description, image, avatar, runtimeKind string, profile AgentProfile, runtimeOptions map[string]any) error {
	s.mu.Lock()

	if _, ok := s.agents[id]; ok {
		s.mu.Unlock()
		return fmt.Errorf("agent id %q already exists", id)
	}
	if s.hasNameLocked(name) {
		s.mu.Unlock()
		return fmt.Errorf("agent name %q already exists", name)
	}

	worker := newWorkerAgent(id, name, description, image, avatar, runtimeKind, profile, runtimeOptions, agentruntime.Info{
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

func (s *Service) persistCreatedWorker(ctx context.Context, id, name, description, image, avatar, runtimeKind string, profile AgentProfile, createRuntimeExt map[string]any, info agentruntime.Info) (Agent, error) {
	s.mu.Lock()

	if existing, ok := s.agents[id]; ok && !isStartingWorker(existing) {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent id %q already exists", id)
	}
	if s.hasNameLockedExcept(name, id) {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent name %q already exists", name)
	}

	worker := newWorkerAgent(id, name, description, image, avatar, runtimeKind, profile, createRuntimeExt, info)
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

func newWorkerAgent(id, name, description, image, avatar, runtimeKind string, profile AgentProfile, runtimeOptions map[string]any, info agentruntime.Info) Agent {
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
	return Agent{
		ID:              id,
		Name:            name,
		RuntimeID:       runtimeIDForAgentID(id),
		RuntimeKind:     runtimeKind,
		Image:           image,
		Avatar:          strings.TrimSpace(avatar),
		BoxID:           strings.TrimSpace(info.HandleID),
		Description:     description,
		Status:          string(state),
		CreatedAt:       createdAt,
		RuntimeOptions:  agentRX,
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

func (s *Service) provisionRuntime(ctx context.Context, rt agentruntime.Runtime, runtimeKind string, req agentruntime.ProvisionRequest) error {
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

func (s *Service) provisionRuntimeForAgent(ctx context.Context, rt agentruntime.Runtime, got Agent, workspaceOverlay string) error {
	if s == nil || rt == nil {
		return nil
	}
	return s.provisionRuntime(ctx, rt, strings.TrimSpace(got.RuntimeKind), agentruntime.ProvisionRequest{
		RuntimeID:        normalizeRuntimeID(got.RuntimeID, got.ID),
		AgentID:          strings.TrimSpace(got.ID),
		ParticipantID:    participantIDForAgent(got.Name, got.ID),
		AgentName:        strings.TrimSpace(got.Name),
		Profile:          s.runtimeProfileForAgent(got),
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
	withoutPrefix := strings.TrimPrefix(agentID, "u-")
	if withoutPrefix != "" && withoutPrefix != agentID {
		return withoutPrefix
	}
	return agentID
}

func (s *Service) gatewayProvisionRequest(runtimeKind, agentName, agentID string) (*agentruntime.GatewayProvision, error) {
	if s == nil {
		return nil, fmt.Errorf("agent service is required")
	}
	agentHome, err := agentHomeDir(agentName)
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
		ManagerBaseURL:    resolveManagerBaseURL(server),
		AgentHome:         agentHome,
		ProjectsRoot:      projectsRoot,
		WorkspaceTemplate: templateRoot,
	}, nil
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

func streamHostGatewayLog(ctx context.Context, agentName string, follow bool, lines int, w io.Writer) error {
	logPath, err := agentGatewayLogPath(agentName)
	if err != nil {
		return err
	}
	return streamHostGatewayLogPaths(ctx, []string{logPath}, follow, lines, w)
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

func agentGatewayLogPath(agentName string) (string, error) {
	agentHome, err := agentHomeDir(agentName)
	if err != nil {
		return "", err
	}
	return picoclawsandbox.HostGatewayLogPath(agentHome), nil
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
	a.Status = string(info.State)
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
