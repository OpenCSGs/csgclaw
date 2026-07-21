package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"csgclaw/internal/config"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
)

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
		managerBaseURL := config.ResolveAdvertiseBaseURL(s.server)
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

func (s *Service) WorkspaceRoot(agentName string) (string, error) {
	got, ok := s.agentSnapshotByName(agentName)
	if !ok {
		return "", fmt.Errorf("agent %q not found", strings.TrimSpace(agentName))
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
