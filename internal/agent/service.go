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
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
)

const (
	ManagerName      = "manager"
	ManagerUserID    = "u-manager"
	managerHostPort  = 18790
	managerGuestPort = 18790
	managerDebugMode = true
	hostWorkspaceDir = "workspace"
	hostProjectsDir  = "projects"
	gatewayLogPoll   = 200 * time.Millisecond
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

type unconfiguredSandboxProvider struct{}

func (unconfiguredSandboxProvider) Name() string {
	return "unconfigured"
}

func (unconfiguredSandboxProvider) Open(context.Context, string) (sandbox.Runtime, error) {
	return nil, fmt.Errorf("sandbox provider is not configured")
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
	model            config.ModelConfig
	llm              config.LLMConfig
	server           config.ServerConfig
	managerImage     string
	gatewayRuntime   string
	state            string
	sandbox          sandbox.Provider
	mu               sync.RWMutex
	runtimes         map[string]sandbox.Runtime
	agents           map[string]Agent
	runtimeRecords   map[string]RuntimeRecord
	runtimeRegistry  map[string]agentruntime.Runtime
	lifecycle        LifecycleObserver
	profileDefaults  AgentProfile
	detectionResults []ProfileDetectionResult

	// gatewayWorkPhase is set by createGatewayBox for bootstrap progress logs (best-effort if concurrent).
	gatewayWorkPhase atomic.Uint32
}

type ServiceOption func(*Service) error

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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gatewayRuntime = kind
	if image := strings.TrimSpace(managerImage); image != "" {
		s.managerImage = image
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
	if managerImage == "" {
		managerImage = config.DefaultManagerImage
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
	if _, err := ensureAgentPicoClawConfig(ManagerName, ManagerUserID, svc.server, defaultModel); err != nil {
		return err
	}
	_, err = svc.EnsureManager(ctx, forceRecreate)
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

func (s *Service) ensureManager(ctx context.Context, forceRecreate bool, imageOverride string) (Agent, error) {
	if s == nil {
		return Agent{}, fmt.Errorf("agent service is required")
	}
	managerImage := strings.TrimSpace(imageOverride)
	if managerImage == "" {
		managerImage = s.managerImage
	}
	startProfile, detectionResults := s.managerStartupProfile(ctx)
	if startProfile.ProfileComplete {
		gatewayConfig, err := s.gatewayConfigurer()
		if err != nil {
			return Agent{}, err
		}
		if err := gatewayConfig.EnsureGatewayConfig(ManagerName, ManagerUserID, startProfile.ModelID); err != nil {
			return Agent{}, err
		}
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
		log.Printf("force recreating bootstrap manager box %q", ManagerName)
		removed := false
		for _, managerBoxIDOrName := range s.bootstrapManagerLookupKeys() {
			if err := s.forceRemoveBox(ctx, rt, managerBoxIDOrName); err != nil {
				if sandbox.IsNotFound(err) {
					log.Printf("bootstrap manager box %q (%q) does not exist yet; continuing", ManagerName, managerBoxIDOrName)
					continue
				}
				return Agent{}, fmt.Errorf("force remove bootstrap manager box %q (%q): %w", ManagerName, managerBoxIDOrName, err)
			}
			log.Printf("bootstrap manager box %q (%q) removed", ManagerName, managerBoxIDOrName)
			removed = true
			break
		}
		if !removed {
			log.Printf("bootstrap manager box %q not found under known identifiers; continuing", ManagerName)
		}
		if err := s.closeRuntime(runtimeHome, rt); err != nil {
			return Agent{}, fmt.Errorf("close bootstrap manager runtime before recreate: %w", err)
		}
		rt = nil
		managerHome, err := agentHomeDir(ManagerName)
		if err != nil {
			return Agent{}, err
		}
		if err := removeAllWithRetry(managerHome); err != nil {
			return Agent{}, fmt.Errorf("remove bootstrap manager home: %w", err)
		}
		rt, err = s.ensureRuntimeAtHome(runtimeHome)
		if err != nil {
			return Agent{}, err
		}
		box = nil
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
		manager.Provider = startProfile.Provider
		manager.ModelID = startProfile.ModelID
		manager.ReasoningEffort = startProfile.ReasoningEffort
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

	manager := Agent{
		ID:               ManagerUserID,
		Name:             ManagerName,
		RuntimeID:        runtimeIDForAgentID(ManagerUserID),
		RuntimeKind:      s.gatewayRuntimeKind(),
		Image:            managerImage,
		BoxID:            info.ID,
		Status:           string(info.State),
		CreatedAt:        info.CreatedAt.UTC(),
		Profile:          profileSelector(startProfile),
		Provider:         startProfile.Provider,
		ModelID:          startProfile.ModelID,
		ReasoningEffort:  startProfile.ReasoningEffort,
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
	agentRuntimeKind := normalizeRuntimeKind(a.RuntimeKind)
	rt := runtimeRecordForAgent(a)
	if rt.ID == "" {
		return
	}
	if strings.EqualFold(normalizeRole(a.Role), RoleManager) || (strings.EqualFold(normalizeRole(a.Role), RoleWorker) && (agentRuntimeKind == "" || isGatewayRuntimeKind(agentRuntimeKind))) {
		rt.Kind = s.runtimeKindForGatewayAgent(a)
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
	if req.Replace {
		return s.replace(ctx, req)
	}
	return s.createNew(ctx, req.Spec)
}

func (s *Service) createNew(ctx context.Context, spec CreateAgentSpec) (Agent, error) {
	if isManagerCreateSpec(spec) {
		return s.EnsureManager(ctx, false)
	}
	if shouldCreateWorkerSpec(spec) {
		spec.Role = RoleWorker
		return s.CreateWorker(ctx, spec)
	}

	id := strings.TrimSpace(spec.ID)
	name := strings.TrimSpace(spec.Name)
	description := strings.TrimSpace(spec.Description)
	image := strings.TrimSpace(spec.Image)
	runtimeExplicit := strings.TrimSpace(spec.RuntimeKind) != ""
	runtimeKind := normalizeRuntimeKind(spec.RuntimeKind)
	if runtimeKind == "" {
		runtimeKind = s.gatewayRuntimeKind()
	}
	if image == "" {
		if defaultImage := managerImageForRuntimeKind(runtimeKind); defaultImage != "" && runtimeExplicit {
			image = defaultImage
		}
		if image == "" && isGatewayRuntimeKind(runtimeKind) {
			image = s.managerImage
		}
	}
	role := normalizeRole(spec.Role)
	if name == "" {
		return Agent{}, fmt.Errorf("name is required")
	}
	if role == RoleManager {
		return Agent{}, fmt.Errorf("role %q is reserved", role)
	}
	if id == "" {
		id = fmt.Sprintf("%s-%d", role, time.Now().UnixNano())
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

	resolvedProfile, err := s.profileForCreateRequest(ctx, spec)
	if err != nil {
		return Agent{}, err
	}

	projectsRoot, err := ensureAgentProjectsRoot()
	if err != nil {
		return Agent{}, err
	}
	managerBaseURL := resolveManagerBaseURL(s.server)
	llmBaseURL := llmBridgeBaseURL(managerBaseURL, id)
	boxSpec := sandbox.CreateSpec{
		Image:      image,
		Name:       name,
		Detach:     true,
		AutoRemove: false,
		Mounts:     []sandbox.Mount{},
		Env:        make(map[string]string),
	}
	gatewayConfig, err := s.gatewayConfigurer()
	if err != nil {
		return Agent{}, err
	}
	boxSpec.Mounts = append(boxSpec.Mounts, sandbox.Mount{HostPath: projectsRoot, GuestPath: gatewayConfig.ProjectsGuestPath()})
	for key, value := range bridgeLLMEnvVars(llmBaseURL, s.server.AccessToken, resolvedProfile.ModelID) {
		boxSpec.Env[key] = value
	}
	addProfileEnvVars(boxSpec.Env, resolvedProfile.Env)
	box, err := s.createBox(ctx, rt, boxSpec)
	if err != nil {
		return Agent{}, fmt.Errorf("create sandbox agent: %w", err)
	}
	defer func() {
		_ = s.closeBox(box)
	}()

	createdAt := spec.CreatedAt.UTC()
	if spec.CreatedAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	status := strings.TrimSpace(spec.Status)
	if status == "" {
		status = "running"
	}
	agent := Agent{
		ID:              id,
		Name:            name,
		Description:     description,
		RuntimeID:       runtimeIDForAgentID(id),
		RuntimeKind:     runtimeKindForAgent(Agent{Role: role, RuntimeKind: runtimeKind}),
		Image:           image,
		Role:            role,
		Status:          status,
		CreatedAt:       createdAt,
		Profile:         profileSelector(resolvedProfile),
		Provider:        resolvedProfile.Provider,
		ModelID:         resolvedProfile.ModelID,
		ReasoningEffort: resolvedProfile.ReasoningEffort,
		AgentProfile:    resolvedProfile,
		ProfileComplete: resolvedProfile.ProfileComplete,
	}

	s.mu.Lock()
	s.agents[id] = agent
	s.syncRuntimeRecordLocked(agent)
	if resolvedProfile.ProfileComplete {
		s.profileDefaults = cloneProfile(resolvedProfile)
	}
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		s.mu.Lock()
		delete(s.agents, id)
		s.deleteRuntimeRecordLocked(agent.RuntimeID)
		s.mu.Unlock()
		return Agent{}, err
	}
	return agent, nil
}

func (s *Service) replace(ctx context.Context, req CreateRequest) (Agent, error) {
	spec := req.Spec
	id := normalizeCreateID(spec.ID)
	if id == "" {
		return Agent{}, fmt.Errorf("agent create --replace requires id")
	}
	managerImageOverride := replaceImageOverride(req)

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
		if strings.TrimSpace(spec.RuntimeKind) == "" {
			spec.RuntimeKind = existing.RuntimeKind
		}
		if strings.TrimSpace(spec.Role) == "" {
			spec.Role = existing.Role
		}
	}

	if isManagerAgent(existing) || isManagerCreateSpec(spec) {
		return s.ensureManager(ctx, true, managerImageOverride)
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

func mergeReplaceSpec(existing Agent, next CreateAgentSpec, fieldMask []string) (CreateAgentSpec, error) {
	merged := CreateAgentSpec{
		ID:           existing.ID,
		Name:         existing.Name,
		Description:  existing.Description,
		Image:        existing.Image,
		RuntimeKind:  existing.RuntimeKind,
		Role:         existing.Role,
		Status:       existing.Status,
		CreatedAt:    existing.CreatedAt,
		Profile:      existing.Profile,
		ModelID:      existing.ModelID,
		AgentProfile: cloneProfile(existing.AgentProfile),
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
			merged.ModelID = ""
		case "model_id":
			merged.ModelID = next.ModelID
			merged.Profile = ""
			merged.AgentProfile = AgentProfile{}
		case "agent_profile":
			merged.AgentProfile = cloneProfile(next.AgentProfile)
			merged.Profile = ""
			merged.ModelID = ""
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
	return s.hydrateAgentStatus(context.Background(), a), true
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
	if got.AgentProfile.EnvRestartRequired {
		return s.Recreate(ctx, id)
	}
	if err := s.ensureWorkerGatewayConfig(got); err != nil {
		return Agent{}, err
	}

	runtimeImpl, err := s.runtimeForAgent(got)
	if err != nil {
		return Agent{}, err
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

func (s *Service) ensureWorkerGatewayConfig(got Agent) error {
	if s == nil || !strings.EqualFold(normalizeRole(got.Role), RoleWorker) {
		return nil
	}
	return s.ensureGatewayConfigForAgent(got)
}

func (s *Service) ensureGatewayConfigForAgent(got Agent) error {
	if s == nil || !isAgentProfileComplete(got) {
		return nil
	}
	role := normalizeRole(got.Role)
	if role != RoleManager && role != RoleWorker {
		return nil
	}
	if role == RoleWorker {
		if kind := normalizeRuntimeKind(got.RuntimeKind); kind != "" && !isGatewayRuntimeKind(kind) {
			return nil
		}
	}
	name := strings.TrimSpace(got.Name)
	botID := strings.TrimSpace(got.ID)
	if name == "" || botID == "" {
		return fmt.Errorf("agent name and id are required")
	}
	profile := normalizeProfile(got.AgentProfile, name, got.Description)
	modelID := strings.TrimSpace(profile.ModelID)
	if modelID == "" {
		modelID = strings.TrimSpace(got.ModelID)
	}
	if modelID == "" {
		modelID = s.model.Resolved().ModelID
	}
	gatewayConfig, err := s.gatewayConfigurerForAgent(got)
	if err != nil {
		return err
	}
	return gatewayConfig.EnsureGatewayConfig(name, botID, modelID)
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

	runtimeImpl, err := s.runtimeForAgent(got)
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
	if info.State == "" {
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

	if runtimeImpl, err := s.runtimeForAgent(existing); err == nil && strings.TrimSpace(existing.BoxID) != "" {
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
			boxIDOrName := strings.TrimSpace(existing.BoxID)
			if boxIDOrName == "" {
				boxIDOrName = existing.Name
			}
			if _, resolvedKey, resolveErr := s.resolveAgentBox(ctx, rt, existing); resolveErr == nil && strings.TrimSpace(resolvedKey) != "" {
				boxIDOrName = resolvedKey
			}
			if err := s.forceRemoveBox(ctx, rt, boxIDOrName); err != nil && !sandbox.IsNotFound(err) {
				return fmt.Errorf("remove agent box: %w", err)
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
	s.stopLifecycleAgent(id)
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
	return errors.Is(err, syscall.ENOTEMPTY) || strings.Contains(strings.ToLower(err.Error()), "directory not empty")
}

func (s *Service) List() []Agent {
	s.mu.RLock()
	agents := sortedAgentsFromMap(s.agents)
	s.mu.RUnlock()
	for idx := range agents {
		agents[idx] = s.hydrateAgentStatus(context.Background(), agents[idx])
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
	id := strings.TrimSpace(spec.ID)
	name := strings.TrimSpace(spec.Name)
	description := strings.TrimSpace(spec.Description)
	image := strings.TrimSpace(spec.Image)
	runtimeKind := normalizeRuntimeKind(spec.RuntimeKind)
	if runtimeKind == "" {
		runtimeKind = s.gatewayRuntimeKind()
	}
	if image == "" {
		if defaultImage := managerImageForRuntimeKind(runtimeKind); defaultImage != "" && strings.TrimSpace(spec.RuntimeKind) != "" {
			image = defaultImage
		}
		if image == "" {
			image = s.managerImage
		}
	}
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

	if runtimeKind == "" {
		runtimeKind = s.gatewayRuntimeKind()
	}
	runtimeImpl, err := s.runtimeForKind(runtimeKind)
	if err != nil {
		return Agent{}, err
	}
	resolvedProfile, err := s.profileForCreateRequest(ctx, spec)
	if err != nil {
		return Agent{}, err
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
		return s.persistCreatedWorker(ctx, id, name, description, image, runtimeKind, resolvedProfile, agentruntime.Info{
			HandleID:  strings.TrimSpace(info.ID),
			State:     agentruntime.State(info.State),
			CreatedAt: info.CreatedAt.UTC(),
		})
	}
	handle, err := runtimeImpl.Create(ctx, agentruntime.Spec{
		RuntimeID: runtimeIDForAgentID(id),
		AgentID:   id,
		AgentName: name,
		Image:     image,
		Profile:   s.runtimeProfileForKind(runtimeKind, id, name, description, resolvedProfile),
	})
	if err != nil {
		return Agent{}, fmt.Errorf("create worker box: %w", err)
	}
	info := agentruntime.Info{
		HandleID:  strings.TrimSpace(handle.HandleID),
		State:     agentruntime.StateRunning,
		CreatedAt: time.Now().UTC(),
	}

	return s.persistCreatedWorker(ctx, id, name, description, image, runtimeKind, resolvedProfile, info)
}

func (s *Service) persistCreatedWorker(ctx context.Context, id, name, description, image, runtimeKind string, profile AgentProfile, info agentruntime.Info) (Agent, error) {
	s.mu.Lock()

	if _, ok := s.agents[id]; ok {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent id %q already exists", id)
	}
	if s.hasNameLocked(name) {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent name %q already exists", name)
	}

	createdAt := info.CreatedAt.UTC()
	if info.CreatedAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	state := info.State
	if state == "" {
		state = agentruntime.StateRunning
	}
	worker := Agent{
		ID:              id,
		Name:            name,
		RuntimeID:       runtimeIDForAgentID(id),
		RuntimeKind:     runtimeKind,
		Image:           image,
		BoxID:           strings.TrimSpace(info.HandleID),
		Description:     description,
		Status:          string(state),
		CreatedAt:       createdAt,
		Profile:         profileSelector(profile),
		Provider:        profile.Provider,
		ModelID:         profile.ModelID,
		ReasoningEffort: profile.ReasoningEffort,
		AgentProfile:    profile,
		ProfileComplete: profile.ProfileComplete,
		Role:            RoleWorker,
	}
	s.agents[worker.ID] = worker
	s.syncRuntimeRecordLocked(worker)
	if profile.ProfileComplete {
		s.profileDefaults = cloneProfile(profile)
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
	runtimeImpl, err := s.runtimeForAgent(got)
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
	logPaths, err := agentGatewayLogPaths(agentName)
	if err != nil {
		return err
	}
	return streamHostGatewayLogPaths(ctx, logPaths, follow, lines, w)
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
	root, err := agentWorkspaceRoot(agentName)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "gateway.log"), nil
}

func agentGatewayLogPaths(agentName string) ([]string, error) {
	primary, err := agentGatewayLogPath(agentName)
	if err != nil {
		return nil, err
	}
	legacy, err := legacyAgentGatewayLogPath(agentName)
	if err != nil {
		return nil, err
	}
	if legacy == primary {
		return []string{primary}, nil
	}
	return []string{primary, legacy}, nil
}

func legacyAgentGatewayLogPath(agentName string) (string, error) {
	root, err := agentPicoClawRoot(agentName)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "gateway.log"), nil
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

	runtimeImpl, err := s.runtimeForAgent(a)
	if err != nil {
		return statusAfterHydrateFailure(a, "select_runtime", err)
	}
	info, err := s.runtimeInfo(ctx, runtimeImpl, runtimeHandleForAgent(a))
	if err != nil {
		return statusAfterHydrateFailure(a, "read_runtime_info", err)
	}
	if strings.TrimSpace(info.HandleID) != "" {
		a.BoxID = info.HandleID
	}
	a.RuntimeID = normalizeRuntimeID(a.RuntimeID, a.ID)
	a.Status = string(info.State)
	return a
}

func statusAfterHydrateFailure(a Agent, stage string, err error) Agent {
	if strings.TrimSpace(a.Status) != "" && !sandbox.IsNotFound(err) {
		logHydrateStaleStatus(a, stage, err)
		return a
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
	for _, existing := range s.agents {
		if strings.EqualFold(existing.Name, name) {
			return true
		}
	}
	return false
}
