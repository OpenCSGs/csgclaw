package agent

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/config"
	"csgclaw/internal/sandbox"
)

const (
	ManagerName        = "manager"
	ManagerUserID      = "u-manager"
	managerHostPort    = 18790
	managerGuestPort   = 18790
	managerDebugMode   = true
	hostPicoClawDir    = ".picoclaw"
	hostWorkspaceDir   = "workspace"
	hostProjectsDir    = "projects"
	hostPicoClawConfig = "config.json"
	hostPicoClawLogs   = "logs"
	boxPicoClawDir     = "/home/picoclaw/.picoclaw"
	boxWorkspaceDir    = boxPicoClawDir + "/workspace"
	boxProjectsDir     = "/home/picoclaw/.picoclaw/workspace/projects"
)

var localIPv4Resolver = localIPv4

var defaultSandboxProvider sandbox.Provider = unconfiguredSandboxProvider{}

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
	testBoxInfoHook             func(*Service, context.Context, sandbox.Instance) (sandbox.Info, error)
	testCloseBoxHook            func(*Service, sandbox.Instance) error
	testCloseRuntimeHook        func(*Service, string, sandbox.Runtime) error
	testCreateBoxHook           func(*Service, context.Context, sandbox.Runtime, sandbox.CreateSpec) (sandbox.Instance, error)
	testCreateGatewayBoxHook    func(*Service, context.Context, sandbox.Runtime, string, string, string, config.ModelConfig) (sandbox.Instance, sandbox.Info, error)
	testForceRemoveBoxHook      func(*Service, context.Context, sandbox.Runtime, string) error
	testRunBoxCommandHook       func(*Service, context.Context, sandbox.Instance, string, []string, io.Writer) (int, error)
)

// SetTestHooks installs lightweight hooks for tests that need to bypass runtime/box creation.
func SetTestHooks(
	ensureRuntime func(*Service, string) (sandbox.Runtime, error),
	createGatewayBox func(*Service, context.Context, sandbox.Runtime, string, string, string, config.ModelConfig) (sandbox.Instance, sandbox.Info, error),
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

// TestOnlySetRunBoxCommandHook installs a test hook for command execution inside a box.
func TestOnlySetRunBoxCommandHook(hook func(*Service, context.Context, sandbox.Instance, string, []string, io.Writer) (int, error)) {
	testRunBoxCommandHook = hook
}

type Service struct {
	model             config.ModelConfig
	llm               config.LLMConfig
	server            config.ServerConfig
	channels          config.ChannelsConfig
	managerImage      string
	managerRootfsPath string
	state             string
	sandbox           sandbox.Provider
	sandboxHome       string
	mu                sync.RWMutex
	runtimes          map[string]sandbox.Runtime
	agents            map[string]Agent
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
	return NewServiceWithLLMAndChannelsAndBootstrap(llmCfg, server, channels, config.BootstrapConfig{ManagerImage: managerImage}, statePath, opts...)
}

func NewServiceWithLLMAndChannelsAndBootstrap(llmCfg config.LLMConfig, server config.ServerConfig, channels config.ChannelsConfig, bootstrap config.BootstrapConfig, statePath string, opts ...ServiceOption) (*Service, error) {
	// agent.Service owns the persisted registry and the live sandbox lifecycle.
	bootstrap = bootstrap.Resolved()
	if err := bootstrap.Validate(); err != nil {
		return nil, err
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
		model:             model,
		llm:               llmCfg.Normalized(),
		server:            server,
		channels:          cloneChannelsConfig(channels),
		managerImage:      bootstrap.ManagerImage,
		managerRootfsPath: bootstrap.ManagerRootfsPath,
		state:             statePath,
		sandbox:           defaultSandboxProvider,
		sandboxHome:       config.DefaultSandboxHomeDirName,
		runtimes:          make(map[string]sandbox.Runtime),
		agents:            make(map[string]Agent),
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
	return EnsureBootstrapStateWithLLMAndBootstrap(ctx, statePath, server, llmCfg, config.BootstrapConfig{ManagerImage: managerImage}, forceRecreate)
}

func EnsureBootstrapStateWithLLMAndBootstrap(ctx context.Context, statePath string, server config.ServerConfig, llmCfg config.LLMConfig, bootstrap config.BootstrapConfig, forceRecreate bool) error {
	svc, err := NewServiceWithLLMAndChannelsAndBootstrap(llmCfg, server, config.ChannelsConfig{}, bootstrap, statePath)
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

func (s *Service) EnsureManager(ctx context.Context, forceRecreate bool) (Agent, error) {
	if s == nil {
		return Agent{}, fmt.Errorf("agent service is required")
	}
	defaultProfile, defaultModel, err := s.llm.Resolve("")
	if err != nil {
		return Agent{}, err
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
		managerBoxIDOrName := s.bootstrapManagerBoxIDOrName()
		if err := s.forceRemoveBox(ctx, rt, managerBoxIDOrName); err != nil {
			if sandbox.IsNotFound(err) {
				log.Printf("bootstrap manager box %q (%q) does not exist yet; continuing", ManagerName, managerBoxIDOrName)
			} else {
				return Agent{}, fmt.Errorf("force remove bootstrap manager box %q (%q): %w", ManagerName, managerBoxIDOrName, err)
			}
		} else {
			log.Printf("bootstrap manager box %q (%q) removed", ManagerName, managerBoxIDOrName)
		}
		if err := s.closeRuntime(runtimeHome, rt); err != nil {
			return Agent{}, fmt.Errorf("close bootstrap manager runtime before recreate: %w", err)
		}
		rt = nil
		managerHome, err := agentHomeDir(ManagerName)
		if err != nil {
			return Agent{}, err
		}
		if err := os.RemoveAll(managerHome); err != nil {
			return Agent{}, fmt.Errorf("remove bootstrap manager home: %w", err)
		}
		rt, err = s.ensureRuntimeAtHome(runtimeHome)
		if err != nil {
			return Agent{}, err
		}
		box = nil
	}
	var info sandbox.Info
	if box == nil {
		log.Printf("bootstrap manager box %q not found, creating it with image %q", ManagerName, s.managerImage)
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
					log.Printf("still creating bootstrap manager box %q with image %q; image download may still be in progress", ManagerName, s.managerImage)
				}
			}
		}()
		box, info, err = s.createGatewayBox(ctx, rt, s.managerImage, ManagerName, ManagerUserID, defaultModel)
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
		ID:              ManagerUserID,
		Name:            ManagerName,
		Image:           s.managerImage,
		BoxID:           info.ID,
		Status:          string(info.State),
		CreatedAt:       info.CreatedAt.UTC(),
		Profile:         defaultProfile,
		Provider:        defaultModel.Resolved().Provider,
		ModelID:         defaultModel.Resolved().ModelID,
		ReasoningEffort: defaultModel.Resolved().ReasoningEffort,
		Role:            RoleManager,
	}
	for id, a := range s.agents {
		if isManagerAgent(a) && id != manager.ID {
			delete(s.agents, id)
		}
	}
	s.agents[manager.ID] = manager
	if err := s.saveLocked(); err != nil {
		return Agent{}, err
	}
	return *cloneAgent(&manager), nil
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

func (s *Service) Create(ctx context.Context, req CreateRequest) (Agent, error) {
	id := strings.TrimSpace(req.ID)
	name := strings.TrimSpace(req.Name)
	description := strings.TrimSpace(req.Description)
	image := strings.TrimSpace(req.Image)
	role := normalizeRole(req.Role)
	if name == "" {
		return Agent{}, fmt.Errorf("name is required")
	}
	if image == "" {
		return Agent{}, fmt.Errorf("image is required")
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

	requestedProfile := strings.TrimSpace(req.Profile)
	if requestedProfile == "" && strings.TrimSpace(req.ModelID) != "" {
		matchedProfile, _, ok := s.llm.MatchProfile(config.ModelConfig{ModelID: req.ModelID})
		if !ok {
			return Agent{}, fmt.Errorf("no llm profile matches model %q", strings.TrimSpace(req.ModelID))
		}
		requestedProfile = matchedProfile
	}

	profileName, resolvedModel, err := s.resolveModelProfile(requestedProfile)
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
	for key, value := range bridgeLLMEnvVars(llmBaseURL, s.server.AccessToken, resolvedModel.ModelID) {
		boxSpec.Env[key] = value
	}
	box, err := s.createBox(ctx, rt, boxSpec)
	if err != nil {
		return Agent{}, fmt.Errorf("create sandbox agent: %w", err)
	}
	defer func() {
		_ = s.closeBox(box)
	}()

	createdAt := req.CreatedAt.UTC()
	if req.CreatedAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	status := strings.TrimSpace(req.Status)
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
		Profile:         profileName,
		Provider:        resolvedModel.Provider,
		ModelID:         resolvedModel.ModelID,
		ReasoningEffort: resolvedModel.ReasoningEffort,
	}

	s.mu.Lock()
	s.agents[id] = agent
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return Agent{}, err
	}
	return agent, nil
}

func (s *Service) Agent(id string) (Agent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	a, ok := s.agents[strings.TrimSpace(id)]
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
	if isManagerAgent(existing) {
		return fmt.Errorf("agent %q is reserved", id)
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
	if err := os.RemoveAll(agentHome); err != nil {
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

func (s *Service) List() []Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sortedAgentsFromMap(s.agents)
}

func (s *Service) CreateWorker(ctx context.Context, req CreateRequest) (Agent, error) {
	id := strings.TrimSpace(req.ID)
	name := strings.TrimSpace(req.Name)
	description := strings.TrimSpace(req.Description)
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
	profileName, resolvedModel, err := s.resolveModelProfile(req.Profile)
	if err != nil {
		return Agent{}, err
	}
	box, info, err := s.createGatewayBox(ctx, rt, s.managerImage, name, id, resolvedModel)
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
		Image:           s.managerImage,
		BoxID:           info.ID,
		Description:     description,
		Status:          string(info.State),
		CreatedAt:       info.CreatedAt.UTC(),
		Profile:         profileName,
		Provider:        resolvedModel.Provider,
		ModelID:         resolvedModel.ModelID,
		ReasoningEffort: resolvedModel.ReasoningEffort,
		Role:            RoleWorker,
	}
	s.agents[worker.ID] = worker
	if err := s.saveLocked(); err != nil {
		delete(s.agents, worker.ID)
		return Agent{}, err
	}
	return *cloneAgent(&worker), nil
}

func (s *Service) ListWorkers() []Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workers := make(map[string]Agent)
	for id, a := range s.agents {
		if a.Role == RoleWorker {
			workers[id] = a
		}
	}
	return sortedAgentsFromMap(workers)
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

	got, ok := s.Agent(id)
	if !ok {
		return fmt.Errorf("agent %q not found", id)
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
	args = append(args, boxPicoClawDir+"/gateway.log")

	exitCode, err := s.runBoxCommand(ctx, box, "tail", args, w)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("tail exited with code %d", exitCode)
	}
	return nil
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
