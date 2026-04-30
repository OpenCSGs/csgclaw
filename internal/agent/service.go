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
	"syscall"
	"time"

	"csgclaw/internal/config"
	"csgclaw/internal/sandbox"
)

const (
	ManagerName          = "manager"
	ManagerUserID        = "u-manager"
	managerHostPort      = 18790
	managerGuestPort     = 18790
	managerDebugMode     = true
	hostPicoClawDir      = ".picoclaw"
	hostWorkspaceDir     = "workspace"
	hostProjectsDir      = "projects"
	hostPicoClawConfig   = "config.json"
	hostPicoClawSecurity = ".security.yml"
	hostPicoClawStateDir = ".csgclaw/picoclaw"
	boxPicoClawDir       = "/home/picoclaw/.picoclaw"
	boxWorkspaceDir      = boxPicoClawDir + "/workspace"
	boxProjectsDir       = "/home/picoclaw/.picoclaw/workspace/projects"
	boxGatewayLogPath    = boxWorkspaceDir + "/gateway.log"
	gatewayLogPoll       = 200 * time.Millisecond
)

var localIPv4Resolver = localIPv4

var osRemoveAll = os.RemoveAll

var defaultSandboxProvider sandbox.Provider = unconfiguredSandboxProvider{}

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

type Service struct {
	model            config.ModelConfig
	llm              config.LLMConfig
	server           config.ServerConfig
	channels         config.ChannelsConfig
	managerImage     string
	state            string
	sandbox          sandbox.Provider
	sandboxHome      string
	mu               sync.RWMutex
	runtimes         map[string]sandbox.Runtime
	agents           map[string]Agent
	profileDefaults  AgentProfile
	detectionResults []ProfileDetectionResult
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

func WithSandboxHomeDirName(name string) ServiceOption {
	return func(s *Service) error {
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("sandbox home dir name is required")
		}
		s.sandboxHome = name
		return nil
	}
}

func NewService(model config.ModelConfig, server config.ServerConfig, managerImage, statePath string, opts ...ServiceOption) (*Service, error) {
	return NewServiceWithLLM(config.SingleProfileLLM(model), server, managerImage, statePath, opts...)
}

func NewServiceWithChannels(model config.ModelConfig, server config.ServerConfig, channels config.ChannelsConfig, managerImage, statePath string, opts ...ServiceOption) (*Service, error) {
	return NewServiceWithLLMAndChannels(config.SingleProfileLLM(model), server, channels, managerImage, statePath, opts...)
}

func NewServiceWithLLM(llmCfg config.LLMConfig, server config.ServerConfig, managerImage, statePath string, opts ...ServiceOption) (*Service, error) {
	return NewServiceWithLLMAndChannels(llmCfg, server, config.ChannelsConfig{}, managerImage, statePath, opts...)
}

func NewServiceWithLLMAndChannels(llmCfg config.LLMConfig, server config.ServerConfig, channels config.ChannelsConfig, managerImage, statePath string, opts ...ServiceOption) (*Service, error) {
	// agent.Service owns the persisted registry and the live sandbox lifecycle.
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
		channels:        cloneChannelsConfig(channels),
		managerImage:    managerImage,
		state:           statePath,
		sandbox:         defaultSandboxProvider,
		sandboxHome:     config.DefaultSandboxHomeDirName,
		runtimes:        make(map[string]sandbox.Runtime),
		agents:          make(map[string]Agent),
		profileDefaults: profileFromConfigModel("", "", model),
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

func cloneChannelsConfig(channels config.ChannelsConfig) config.ChannelsConfig {
	cloned := config.ChannelsConfig{
		FeishuAdminOpenID: channels.FeishuAdminOpenID,
	}
	if len(channels.Feishu) > 0 {
		cloned.Feishu = make(map[string]config.FeishuConfig, len(channels.Feishu))
		for name, feishu := range channels.Feishu {
			cloned.Feishu[name] = feishu
		}
	}
	return cloned
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
		if _, err := ensureAgentPicoClawConfig(ManagerName, ManagerUserID, s.server, config.ModelConfig{ModelID: startProfile.ModelID}); err != nil {
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
		s.mu.Lock()
		manager := s.agents[ManagerUserID]
		if manager.ID == "" || forceRecreate {
			manager = Agent{
				ID:        ManagerUserID,
				Name:      ManagerName,
				Image:     managerImage,
				Status:    "profile_incomplete",
				CreatedAt: now,
				Role:      RoleManager,
			}
		}
		manager.AgentProfile = startProfile
		manager.ProfileComplete = false
		manager.DetectionResults = detectionResults
		manager.Profile = profileSelector(startProfile)
		manager.Provider = startProfile.Provider
		manager.ModelID = startProfile.ModelID
		manager.ReasoningEffort = startProfile.ReasoningEffort
		s.agents[ManagerUserID] = manager
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
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-progressDone:
					return
				case <-ticker.C:
					log.Printf("still creating bootstrap manager box %q with image %q; image download may still be in progress", ManagerName, managerImage)
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
		if err := s.startBox(ctx, box); err != nil {
			return Agent{}, fmt.Errorf("start bootstrap manager box: %w", err)
		}
		info, err = s.boxInfo(ctx, box)
		if err != nil {
			return Agent{}, fmt.Errorf("read bootstrap manager box info: %w", err)
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
	if image == "" {
		image = s.managerImage
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
		Mounts: []sandbox.Mount{
			{HostPath: projectsRoot, GuestPath: boxProjectsDir},
		},
		Env: make(map[string]string),
	}
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
	if resolvedProfile.ProfileComplete {
		s.profileDefaults = cloneProfile(resolvedProfile)
	}
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
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

	rt, err := s.ensureRuntime(got.Name)
	if err != nil {
		return Agent{}, err
	}
	runtimeHome, err := s.sandboxRuntimeHome(got.Name)
	if err != nil {
		return Agent{}, err
	}
	defer func() {
		_ = s.closeRuntime(runtimeHome, rt)
	}()

	box, resolvedKey, err := s.resolveAgentBox(ctx, rt, got)
	if err != nil {
		if sandbox.IsNotFound(err) {
			_ = s.closeRuntime(runtimeHome, rt)
			rt = nil
			return s.Recreate(ctx, id)
		}
		return Agent{}, err
	}
	defer func() {
		_ = s.closeBox(box)
	}()

	if err := s.startBox(ctx, box); err != nil {
		return Agent{}, fmt.Errorf("start agent box: %w", err)
	}
	info, err := s.boxInfo(ctx, box)
	if err != nil {
		return Agent{}, fmt.Errorf("read agent box info: %w", err)
	}

	s.mu.Lock()
	current, ok := s.agents[id]
	if !ok {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	if strings.TrimSpace(info.ID) != "" {
		current.BoxID = info.ID
	}
	if info.State != "" {
		current.Status = string(info.State)
	}
	s.agents[id] = current
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return Agent{}, err
	}

	if err := s.refreshAgentBoxID(id, got, resolvedKey, box); err != nil {
		return Agent{}, err
	}

	started, ok := s.Agent(id)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	return started, nil
}

func (s *Service) ensureWorkerGatewayConfig(got Agent) error {
	if s == nil || !strings.EqualFold(strings.TrimSpace(got.Role), RoleWorker) || !isAgentProfileComplete(got) {
		return nil
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
	_, err := ensureAgentPicoClawConfig(name, botID, s.server, config.ModelConfig{ModelID: modelID})
	return err
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

	rt, err := s.ensureRuntime(got.Name)
	if err != nil {
		return Agent{}, err
	}
	runtimeHome, err := s.sandboxRuntimeHome(got.Name)
	if err != nil {
		return Agent{}, err
	}
	defer func() {
		_ = s.closeRuntime(runtimeHome, rt)
	}()

	box, resolvedKey, err := s.resolveAgentBox(ctx, rt, got)
	if err != nil {
		if sandbox.IsNotFound(err) {
			return Agent{}, fmt.Errorf("agent %q not found", id)
		}
		return Agent{}, err
	}
	defer func() {
		_ = s.closeBox(box)
	}()

	if err := s.stopBox(ctx, box, sandbox.StopOptions{}); err != nil {
		return Agent{}, fmt.Errorf("stop agent box: %w", err)
	}
	info, err := s.boxInfo(ctx, box)
	if err != nil {
		return Agent{}, fmt.Errorf("read agent box info: %w", err)
	}

	s.mu.Lock()
	current, ok := s.agents[id]
	if !ok {
		s.mu.Unlock()
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	if strings.TrimSpace(info.ID) != "" {
		current.BoxID = info.ID
	}
	if info.State != "" {
		current.Status = string(info.State)
	}
	s.agents[id] = current
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return Agent{}, err
	}

	if err := s.refreshAgentBoxID(id, got, resolvedKey, box); err != nil {
		return Agent{}, err
	}

	stopped, ok := s.Agent(id)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	return stopped, nil
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

	rt, err := s.ensureRuntime(existing.Name)
	if err != nil {
		return err
	}
	runtimeHome, err := s.sandboxRuntimeHome(existing.Name)
	if err != nil {
		return err
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

	agentHome, err := agentHomeDir(existing.Name)
	if err != nil {
		return err
	}
	if err := removeAllWithRetry(agentHome); err != nil {
		return fmt.Errorf("remove agent home: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.agents[id]
	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}
	delete(s.agents, id)
	runtimeHome, err = s.sandboxRuntimeHome(current.Name)
	if err != nil {
		return err
	}
	if rt := s.runtimes[runtimeHome]; rt != nil {
		delete(s.runtimes, runtimeHome)
	}
	return s.saveLocked()
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
		if isAgentRuntimeRunning(live) {
			// A running sandbox can still hold a stale PicoClaw event stream after
			// csgclaw serve restarts. Recreate the worker gateway so it subscribes
			// to the current server instance.
			if _, err := s.Recreate(ctx, live.ID); err != nil {
				startErr = errors.Join(startErr, fmt.Errorf("%s: %w", live.Name, err))
			}
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

func isAgentRuntimeRunning(a Agent) bool {
	return strings.EqualFold(strings.TrimSpace(a.Status), string(sandbox.StateRunning))
}

func (s *Service) CreateWorker(ctx context.Context, spec CreateAgentSpec) (Agent, error) {
	id := strings.TrimSpace(spec.ID)
	name := strings.TrimSpace(spec.Name)
	description := strings.TrimSpace(spec.Description)
	image := strings.TrimSpace(spec.Image)
	if image == "" {
		image = s.managerImage
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
	box, info, err := s.createGatewayBox(ctx, rt, image, name, id, resolvedProfile)
	if err != nil {
		return Agent{}, fmt.Errorf("create worker box: %w", err)
	}
	defer func() {
		_ = s.closeBox(box)
	}()

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.agents[id]; ok {
		return Agent{}, fmt.Errorf("agent id %q already exists", id)
	}
	if s.hasNameLocked(name) {
		return Agent{}, fmt.Errorf("agent name %q already exists", name)
	}

	worker := Agent{
		ID:              id,
		Name:            name,
		Image:           image,
		BoxID:           info.ID,
		Description:     description,
		Status:          string(info.State),
		CreatedAt:       info.CreatedAt.UTC(),
		Profile:         profileSelector(resolvedProfile),
		Provider:        resolvedProfile.Provider,
		ModelID:         resolvedProfile.ModelID,
		ReasoningEffort: resolvedProfile.ReasoningEffort,
		AgentProfile:    resolvedProfile,
		ProfileComplete: resolvedProfile.ProfileComplete,
		Role:            RoleWorker,
	}
	s.agents[worker.ID] = worker
	if resolvedProfile.ProfileComplete {
		s.profileDefaults = cloneProfile(resolvedProfile)
	}
	if err := s.saveLocked(); err != nil {
		delete(s.agents, worker.ID)
		return Agent{}, err
	}
	return *cloneAgent(&worker), nil
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
	if err := streamHostGatewayLog(ctx, got.Name, follow, lines, w); err == nil {
		return nil
	} else if follow || !errors.Is(err, os.ErrNotExist) {
		return err
	}

	rt, err := s.ensureRuntime(got.Name)
	if err != nil {
		return err
	}
	runtimeHome, err := s.sandboxRuntimeHome(got.Name)
	if err != nil {
		return err
	}
	defer func() {
		_ = s.closeRuntime(runtimeHome, rt)
	}()

	box, resolvedKey, err := s.resolveAgentBox(ctx, rt, got)
	if err != nil {
		if sandbox.IsNotFound(err) {
			boxIDOrName := strings.TrimSpace(got.BoxID)
			if boxIDOrName == "" {
				boxIDOrName = got.Name
			}
			return fmt.Errorf("agent box %q not found", boxIDOrName)
		}
		return err
	}
	defer func() {
		_ = s.closeBox(box)
	}()
	if err := s.refreshAgentBoxID(id, got, resolvedKey, box); err != nil {
		return err
	}

	args := []string{"-n", fmt.Sprintf("%d", lines)}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, boxGatewayLogPath)

	exitCode, err := s.runBoxCommand(ctx, box, "tail", args, w)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("tail exited with code %d", exitCode)
	}
	return nil
}

func streamHostGatewayLog(ctx context.Context, agentName string, follow bool, lines int, w io.Writer) error {
	logPaths, err := agentGatewayLogPaths(agentName)
	if err != nil {
		return err
	}
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

	rt, err := s.ensureRuntime(a.Name)
	if err != nil {
		return statusAfterHydrateFailure(a, "ensure_runtime", err)
	}
	runtimeHome, err := s.sandboxRuntimeHome(a.Name)
	if err != nil {
		return statusAfterHydrateFailure(a, "resolve_runtime_home", err)
	}
	defer func() {
		_ = s.closeRuntime(runtimeHome, rt)
	}()

	box, _, err := s.resolveAgentBox(ctx, rt, a)
	if err != nil {
		return statusAfterHydrateFailure(a, "resolve_agent_box", err)
	}
	defer func() {
		_ = s.closeBox(box)
	}()

	info, err := s.boxInfo(ctx, box)
	if err != nil {
		return statusAfterHydrateFailure(a, "read_box_info", err)
	}
	if strings.TrimSpace(info.ID) != "" {
		a.BoxID = info.ID
	}
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
