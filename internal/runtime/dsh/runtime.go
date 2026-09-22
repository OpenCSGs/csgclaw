package dsh

import (
	"context"
	"csgclaw/internal/modelcap"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/dshcli"
	"csgclaw/internal/identity"
	agentruntime "csgclaw/internal/runtime"
	runtimeinstructions "csgclaw/internal/runtime/instructions"
	"csgclaw/internal/runtime/sandboxgateway"
	templateembed "csgclaw/internal/template/embed"
)

const (
	hostStateDirName      = ".dsh"
	homeDirName           = "home"
	workspaceDirName      = "workspace"
	runtimeFileName       = "runtime.json"
	stderrFileName        = "stderr.log"
	settingsFileName      = "settings.yaml"
	sessionsDirName       = "sessions"
	patchFileName         = "csgclaw.patch.yml"
	contextPatchFileName  = "csgclaw-context.patch.yml"
	llmAPIKeyEnvName      = "CSGCLAW_DSH_LLM_API_KEY"
	permissionModeEnvName = "DSH_PERMISSION_MODE"
)

const runtimePatch = `- insert:
    - id: tool-present
      name: '@deepseek-ai/dsh-tool-present'
`

type AgentRef struct {
	ID             string
	Name           string
	RuntimeID      string
	Instructions   string
	RuntimeOptions map[string]any
	MCPServers     map[string]any
	Profile        agentruntime.Profile
}

type BinaryResolver func(context.Context) (dshcli.Info, error)

type Dependencies struct {
	ResolveBinary         BinaryResolver
	ResolveAgent          func(agentruntime.Handle) (AgentRef, error)
	MaterializeMCPServers func(context.Context, map[string]any) (map[string]any, error)
	AgentHome             func(string) (string, error)
}

type Runtime struct {
	deps Dependencies

	mu        sync.Mutex
	processes map[string]*process
	roots     map[string]string
	pending   map[string]*pendingPermission
	nextPerm  uint64
}

type process struct {
	contextUsage         map[string]modelcap.ContextUsage
	cmd                  *exec.Cmd
	stdin                io.WriteCloser
	client               *acpClient
	stderr               *os.File
	root                 string
	workspace            string
	profile              agentruntime.Profile
	imagePrompts         bool
	mcp                  []acpMCPServer
	meta                 runtimeMetadata
	environment          []string
	extensionDigests     map[string]string
	extensionExecutables map[string]string
	done                 chan struct{}

	metadataMu sync.Mutex
	mu         sync.Mutex
	active     map[string]*activeTurn
	ready      map[string]bool
}

type activeTurn struct {
	contextExceeded  bool
	compactionFailed bool
	request          contract.TurnRequest
	sink             contract.EventSink
	seq              uint64
	output           strings.Builder
	tools            map[string]contract.ToolActivity
	presentedFiles   []presentedFile
	presentedPaths   map[string]bool
	interactionError *contract.TurnError
}

type presentedFile struct {
	Path string
}

type pendingPermission struct {
	runtimeID      string
	conversation   contract.ConversationKey
	request        contract.InteractionRequest
	requestID      json.RawMessage
	client         *acpClient
	allowedOptions map[string]bool
}

type runtimeMetadata struct {
	RuntimeID      string             `json:"runtime_id"`
	AgentID        string             `json:"agent_id"`
	Executable     string             `json:"executable"`
	Version        string             `json:"version"`
	PermissionMode string             `json:"permission_mode,omitempty"`
	PID            int                `json:"pid,omitempty"`
	State          agentruntime.State `json:"state"`
	CreatedAt      time.Time          `json:"created_at"`
	Sessions       map[string]string  `json:"sessions,omitempty"`
}

var (
	_ agentruntime.Runtime                     = (*Runtime)(nil)
	_ agentruntime.Provisioner                 = (*Runtime)(nil)
	_ agentruntime.LogStreamer                 = (*Runtime)(nil)
	_ agentruntime.RuntimeOptionSchemaProvider = (*Runtime)(nil)
	_ agentruntime.RuntimeConfigController     = (*Runtime)(nil)
	_ agentruntime.MCPServersController        = (*Runtime)(nil)
	_ agentruntime.MCPServersReconciler        = (*Runtime)(nil)
	_ contract.ConversationProvider            = (*Runtime)(nil)
	_ io.Closer                                = (*Runtime)(nil)
)

func New(deps Dependencies) *Runtime {
	if deps.ResolveBinary == nil {
		deps.ResolveBinary = func(ctx context.Context) (dshcli.Info, error) {
			return (dshcli.Provider{}).Resolve(ctx)
		}
	}
	return &Runtime{
		deps:      deps,
		processes: map[string]*process{},
		roots:     map[string]string{},
		pending:   map[string]*pendingPermission{},
	}
}

func (r *Runtime) Kind() string { return agentruntime.KindDSH }

func (r *Runtime) Layout(agentHome string) agentruntime.Layout {
	root := filepath.Join(agentHome, hostStateDirName)
	return agentruntime.Layout{
		WorkspaceRoot:    filepath.Join(root, workspaceDirName),
		SkillsRoot:       filepath.Join(root, homeDirName, "skills"),
		InstructionsPath: filepath.Join(root, workspaceDirName, "AGENTS.md"),
		HostLogPaths:     []string{filepath.Join(root, stderrFileName)},
	}
}

func (r *Runtime) ensureRuntimeDirs(agentHome string) (agentruntime.Layout, error) {
	layout := r.Layout(agentHome)
	root := filepath.Dir(layout.WorkspaceRoot)
	for _, path := range []string{root, layout.WorkspaceRoot, filepath.Join(root, homeDirName)} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return agentruntime.Layout{}, fmt.Errorf("create DSH runtime dir %s: %w", path, err)
		}
	}
	return layout, nil
}

func (r *Runtime) Conversation(runtimeID string) contract.RuntimeConversation {
	return &conversation{runtime: r, runtimeID: strings.TrimSpace(runtimeID)}
}

func (r *Runtime) Provision(ctx context.Context, req agentruntime.ProvisionRequest) error {
	agentID := identity.CanonicalAgentID(req.AgentID)
	if agentID == "" || r.deps.AgentHome == nil {
		return fmt.Errorf("DSH agent home resolver is required")
	}
	if err := validateExecutionProfile(req.Profile); err != nil {
		return err
	}
	agentHome, err := r.deps.AgentHome(agentID)
	if err != nil {
		return err
	}
	layout, err := r.ensureRuntimeDirs(agentHome)
	if err != nil {
		return err
	}
	root := filepath.Dir(layout.WorkspaceRoot)
	if err := sandboxgateway.EnsureEmbeddedWorkspace(templateembed.DSHWorkerRoot, layout.WorkspaceRoot); err != nil {
		return fmt.Errorf("seed DSH worker workspace: %w", err)
	}
	if overlay := strings.TrimSpace(req.WorkspaceOverlay); overlay != "" {
		if err := sandboxgateway.OverlayWorkspaceTree(overlay, layout.WorkspaceRoot); err != nil {
			return fmt.Errorf("overlay DSH worker workspace: %w", err)
		}
	}
	workspaceSkills := filepath.Join(layout.WorkspaceRoot, "skills")
	if info, statErr := os.Stat(workspaceSkills); statErr == nil && info.IsDir() {
		if err := sandboxgateway.OverlayWorkspaceTree(workspaceSkills, layout.SkillsRoot); err != nil {
			return fmt.Errorf("install DSH template skills: %w", err)
		}
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect DSH template skills: %w", statErr)
	}
	base := stripManagedInstructions(req.TemplateInstructions)
	if base == "" {
		if data, readErr := os.ReadFile(layout.InstructionsPath); readErr == nil {
			base = stripManagedInstructions(string(data))
		}
	}
	fragments, err := managedExtensionInstructions(filepath.Join(root, homeDirName))
	if err != nil {
		return fmt.Errorf("read DSH Runtime extensions: %w", err)
	}
	block := runtimeinstructions.RenderRuntimeAgentsInstructionsBlockWithOptions(agentID, req.Instructions, runtimeinstructions.RuntimeManagedInstructionsOptions{Extensions: fragments})
	document := mergeDSHInstructionsDocument(base, block)
	if err := os.WriteFile(layout.InstructionsPath, []byte(document), 0o644); err != nil {
		return fmt.Errorf("write DSH AGENTS.md: %w", err)
	}
	if err := writeSettings(filepath.Join(root, homeDirName, settingsFileName), req.Profile); err != nil {
		return err
	}
	if err := writeRuntimePatch(filepath.Join(root, patchFileName), req.Profile); err != nil {
		return err
	}
	r.mu.Lock()
	r.roots[strings.TrimSpace(req.RuntimeID)] = root
	r.mu.Unlock()
	return nil
}

func stripManagedInstructions(current string) string {
	start, end := runtimeinstructions.AgentsInstructionsBlockMarkers()
	startAt := strings.Index(current, start)
	if startAt < 0 {
		return strings.TrimSpace(current)
	}
	endAt := strings.Index(current[startAt:], end)
	if endAt < 0 {
		return strings.TrimSpace(current[:startAt])
	}
	endAt = startAt + endAt + len(end)
	return strings.TrimSpace(current[:startAt] + current[endAt:])
}

func writeSettings(path string, profile agentruntime.Profile) error {
	profile = profile.Normalized()
	settings := map[string]any{"llm-deepseek": dshProviderSettings(profile)}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode DSH settings: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write DSH settings: %w", err)
	}
	return nil
}

func writeRuntimePatch(path string, profile agentruntime.Profile) error {
	enabled := profile.AutoCompact == nil || *profile.AutoCompact
	bridgePath := filepath.Join(filepath.Dir(path), "context-bridge.mjs")
	if err := os.WriteFile(bridgePath, []byte(contextBridgeModule), 0o600); err != nil {
		return err
	}
	providerConfig, err := json.Marshal(dshProviderSettings(profile))
	if err != nil {
		return err
	}
	patch := fmt.Sprintf("- id: llm-deepseek\n  config: %s\n- id: acp\n  config:\n    provider: deepseek-official\n    model: %q\n- id: compaction-basic\n  config:\n    thresholdRatio: 0.75\n    maxOverflowRetries: 1\n    auto: %t\n- insert:\n    - id: csgclaw-context\n      name: %q\n", providerConfig, profile.ModelID, enabled, bridgePath)
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), contextPatchFileName), []byte(patch), 0o600); err != nil {
		return err
	}

	if err := os.WriteFile(path, []byte(runtimePatch), 0o600); err != nil {
		return fmt.Errorf("write CSGClaw DSH patch: %w", err)
	}
	return nil
}

func (r *Runtime) New(ctx context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
	h := agentruntime.Handle{RuntimeID: strings.TrimSpace(spec.RuntimeID), HandleID: strings.TrimSpace(spec.RuntimeID)}
	if _, err := r.start(ctx, h, &spec); err != nil {
		return agentruntime.Handle{}, err
	}
	return h, nil
}

func (r *Runtime) Start(ctx context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	return r.start(ctx, h, nil)
}

func (r *Runtime) start(ctx context.Context, h agentruntime.Handle, spec *agentruntime.Spec) (agentruntime.State, error) {
	runtimeID := strings.TrimSpace(h.RuntimeID)
	if runtimeID == "" {
		return agentruntime.StateUnknown, fmt.Errorf("DSH runtime id is required")
	}
	r.mu.Lock()
	if existing := r.processes[runtimeID]; processRunning(existing) {
		r.mu.Unlock()
		return agentruntime.StateRunning, nil
	}
	r.mu.Unlock()
	if r.deps.ResolveAgent == nil || r.deps.AgentHome == nil {
		return agentruntime.StateUnknown, fmt.Errorf("DSH runtime dependencies are incomplete")
	}
	ref, err := r.deps.ResolveAgent(h)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	if spec != nil && !executionProfileComplete(ref.Profile) {
		ref.Profile = spec.Profile
	}
	ref.Profile = ref.Profile.Normalized()
	if err := validateExecutionProfile(ref.Profile); err != nil {
		return agentruntime.StateUnknown, err
	}
	opts, err := DecodeRuntimeOptions(ref.RuntimeOptions)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	binary, err := r.deps.ResolveBinary(ctx)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	agentHome, err := r.deps.AgentHome(identity.CanonicalAgentID(ref.ID))
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	layout, err := r.ensureRuntimeDirs(agentHome)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	root := filepath.Dir(layout.WorkspaceRoot)
	servers := ref.MCPServers
	if r.deps.MaterializeMCPServers != nil {
		servers, err = r.deps.MaterializeMCPServers(ctx, servers)
		if err != nil {
			return agentruntime.StateUnknown, err
		}
	}
	mcpServers, err := buildACPMCPServers(servers)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	meta := runtimeMetadata{RuntimeID: runtimeID, AgentID: ref.ID, Executable: binary.Path, Version: binary.Version, PermissionMode: opts.PermissionMode, State: agentruntime.StateCreated, CreatedAt: time.Now().UTC(), Sessions: map[string]string{}}
	if persisted, readErr := readMetadata(root); readErr == nil {
		meta.CreatedAt = persisted.CreatedAt
		var permissionChanged bool
		meta.Sessions, permissionChanged = sessionsForPermissionMode(persisted, opts.PermissionMode)
		if permissionChanged && len(persisted.Sessions) > 0 {
			slog.Info("DSH permission mode changed; starting fresh sessions", "runtime_id", runtimeID, "previous_mode", persistedPermissionMode(persisted), "permission_mode", opts.PermissionMode, "session_count", len(persisted.Sessions))
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return agentruntime.StateUnknown, readErr
	}
	if meta.Sessions == nil {
		meta.Sessions = map[string]string{}
	}
	if err := writeSettings(filepath.Join(root, homeDirName, settingsFileName), ref.Profile); err != nil {
		return agentruntime.StateUnknown, err
	}
	if err := writeRuntimePatch(filepath.Join(root, patchFileName), ref.Profile); err != nil {
		return agentruntime.StateUnknown, err
	}
	projections, err := r.ExtensionProjections(ref.ID)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	environment, extensionDigests, err := buildEnvironmentWithExtensions(ref.Profile, filepath.Join(root, homeDirName), opts.PermissionMode, projections)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	extensionExecutables := managedExtensionExecutables(projections)
	proc, err := r.launch(ctx, root, layout.WorkspaceRoot, binary.Path, ref.Profile, mcpServers, meta, environment, extensionDigests, extensionExecutables, true)
	if err != nil {
		patchErr := err
		slog.Warn("DSH present tool overlay unavailable; retrying base ACP profile", "runtime_id", runtimeID, "version", binary.Version, "error", patchErr)
		proc, err = r.launch(ctx, root, layout.WorkspaceRoot, binary.Path, ref.Profile, mcpServers, meta, environment, extensionDigests, extensionExecutables, false)
		if err != nil {
			return agentruntime.StateUnknown, errors.Join(patchErr, err)
		}
	}
	r.mu.Lock()
	r.processes[runtimeID] = proc
	r.roots[runtimeID] = root
	r.mu.Unlock()
	return agentruntime.StateRunning, nil
}

func (r *Runtime) launch(ctx context.Context, root, workspace, binary string, profile agentruntime.Profile, mcp []acpMCPServer, meta runtimeMetadata, environment []string, extensionDigests, extensionExecutables map[string]string, enablePresent bool) (*process, error) {
	stderr, err := os.OpenFile(filepath.Join(root, stderrFileName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open DSH stderr log: %w", err)
	}
	cmd := exec.Command(binary, dshLaunchArgs(root, enablePresent)...)
	configureProcessGroup(cmd)
	cmd.Dir = workspace
	cmd.Env = environment
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stderr.Close()
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		stderr.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		stderr.Close()
		return nil, fmt.Errorf("start DSH ACP process: %w", err)
	}
	client := newACPClient(stdout, stdin)
	proc := &process{cmd: cmd, stdin: stdin, client: client, stderr: stderr, root: root, workspace: workspace, profile: profile.Normalized(), mcp: mcp, meta: meta, environment: append([]string(nil), environment...), extensionDigests: extensionDigests, extensionExecutables: extensionExecutables, done: make(chan struct{}), active: map[string]*activeTurn{}, ready: map[string]bool{}}
	client.setHandlers(
		func(request serverRequest) { r.handleServerRequest(proc, request) },
		func(note notification) { r.handleNotification(proc, note) },
	)
	var initialized struct {
		ProtocolVersion   int `json:"protocolVersion"`
		AgentCapabilities struct {
			PromptCapabilities struct {
				Image bool `json:"image"`
			} `json:"promptCapabilities"`
			MCPCapabilities struct {
				HTTP bool `json:"http"`
			} `json:"mcpCapabilities"`
		} `json:"agentCapabilities"`
	}
	initCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := client.call(initCtx, "initialize", map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}}, &initialized, nil); err != nil {
		_ = terminateProcessTree(cmd.Process.Pid, true)
		_ = cmd.Wait()
		stderr.Close()
		return nil, fmt.Errorf("initialize DSH ACP: %w", err)
	}
	if initialized.ProtocolVersion != 1 {
		_ = terminateProcessTree(cmd.Process.Pid, true)
		_ = cmd.Wait()
		stderr.Close()
		return nil, fmt.Errorf("DSH ACP protocol version %d is unsupported", initialized.ProtocolVersion)
	}
	if requiresHTTPMCP(mcp) && !initialized.AgentCapabilities.MCPCapabilities.HTTP {
		_ = terminateProcessTree(cmd.Process.Pid, true)
		_ = cmd.Wait()
		stderr.Close()
		return nil, fmt.Errorf("DSH ACP does not advertise HTTP MCP support required by this Agent")
	}
	proc.imagePrompts = initialized.AgentCapabilities.PromptCapabilities.Image
	if err := proc.updateMetadata(func(meta *runtimeMetadata) {
		meta.PID = cmd.Process.Pid
		meta.State = agentruntime.StateRunning
	}); err != nil {
		_ = terminateProcessTree(cmd.Process.Pid, true)
		_ = cmd.Wait()
		stderr.Close()
		return nil, err
	}
	go r.waitProcess(proc)
	return proc, nil
}

func dshLaunchArgs(root string, enablePresent bool) []string {
	args := []string{"--profile", "acp", "--patch", filepath.Join(root, contextPatchFileName)}
	if enablePresent {
		args = append(args, "--patch", filepath.Join(root, patchFileName))
	}
	return args
}

func requiresHTTPMCP(servers []acpMCPServer) bool {
	for _, server := range servers {
		if server.Type == "http" {
			return true
		}
	}
	return false
}

func buildEnvironment(profile agentruntime.Profile, home, permissionMode string) []string {
	blocked := map[string]bool{
		"DSH_HOME": true, "DSH_AGENTS_HOME": true, llmAPIKeyEnvName: true, permissionModeEnvName: true,
		"LARKSUITE_CLI_CONFIG_DIR": true, "LARK_CHANNEL": true, "LARK_CHANNEL_HOME": true,
		"LARK_CHANNEL_PROFILE": true, "LARK_CHANNEL_CONFIG": true,
	}
	values := make(map[string]string, len(os.Environ())+len(profile.Env)+4)
	for _, item := range os.Environ() {
		key, value, found := strings.Cut(item, "=")
		if !found {
			continue
		}
		if !blocked[agentruntime.CanonicalEnvironmentKey(key)] {
			values[key] = value
		}
	}
	for key, value := range profile.Env {
		key = strings.TrimSpace(key)
		if key != "" && !blocked[agentruntime.CanonicalEnvironmentKey(key)] {
			values[key] = value
		}
	}
	values["DSH_HOME"] = home
	values["DSH_AGENTS_HOME"] = filepath.Join(home, "agents")
	values[llmAPIKeyEnvName] = profile.APIKey
	values[permissionModeEnvName] = permissionMode
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}

func buildEnvironmentWithExtensions(profile agentruntime.Profile, home, permissionMode string, projections []agentruntime.ExtensionProjection) ([]string, map[string]string, error) {
	return agentruntime.MergeExtensionEnvironment(buildEnvironment(profile, home, permissionMode), profile.Env, projections)
}

func (r *Runtime) waitProcess(proc *process) {
	_ = proc.cmd.Wait()
	_ = proc.stderr.Close()
	_ = proc.updateMetadata(func(meta *runtimeMetadata) {
		meta.PID = 0
		meta.State = agentruntime.StateExited
	})
	r.mu.Lock()
	if r.processes[proc.meta.RuntimeID] == proc {
		delete(r.processes, proc.meta.RuntimeID)
	}
	r.mu.Unlock()
	close(proc.done)
}

func (r *Runtime) Stop(ctx context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	runtimeID := strings.TrimSpace(h.RuntimeID)
	r.mu.Lock()
	proc := r.processes[runtimeID]
	r.mu.Unlock()
	if proc == nil {
		return agentruntime.StateStopped, nil
	}
	_ = proc.stdin.Close()
	exited := false
	select {
	case <-proc.done:
		exited = true
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
	}
	if !exited {
		_ = terminateProcessTree(proc.cmd.Process.Pid, false)
		select {
		case <-proc.done:
			exited = true
		case <-time.After(500 * time.Millisecond):
		}
	}
	if !exited {
		_ = terminateProcessTree(proc.cmd.Process.Pid, true)
		select {
		case <-proc.done:
			exited = true
		case <-time.After(time.Second):
		}
	}
	if !exited {
		return agentruntime.StateUnknown, fmt.Errorf("DSH process tree did not exit")
	}
	if err := proc.updateMetadata(func(meta *runtimeMetadata) {
		meta.PID = 0
		meta.State = agentruntime.StateStopped
	}); err != nil {
		return agentruntime.StateUnknown, err
	}
	r.mu.Lock()
	if r.processes[runtimeID] == proc {
		delete(r.processes, runtimeID)
	}
	r.mu.Unlock()
	return agentruntime.StateStopped, ctx.Err()
}

func (r *Runtime) Delete(ctx context.Context, h agentruntime.Handle) error {
	if _, err := r.Stop(ctx, h); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("stop DSH runtime before delete: %w", err)
	}
	root, err := r.rootFor(h)
	if err != nil {
		return err
	}
	preserved, err := preserveRecreateState(root)
	if err != nil {
		return err
	}
	if preserved != nil {
		defer preserved.Cleanup()
	}
	removeErr := os.RemoveAll(root)
	restoreErr := preserved.Restore()
	if removeErr != nil || restoreErr != nil {
		return errors.Join(removeErr, restoreErr)
	}
	r.mu.Lock()
	delete(r.roots, strings.TrimSpace(h.RuntimeID))
	r.mu.Unlock()
	return nil
}

func (r *Runtime) State(ctx context.Context, h agentruntime.Handle) (agentruntime.State, error) {
	info, err := r.Info(ctx, h)
	return info.State, err
}

func (r *Runtime) Info(_ context.Context, h agentruntime.Handle) (agentruntime.Info, error) {
	runtimeID := strings.TrimSpace(h.RuntimeID)
	r.mu.Lock()
	proc := r.processes[runtimeID]
	r.mu.Unlock()
	if processRunning(proc) {
		return agentruntime.Info{HandleID: runtimeID, State: agentruntime.StateRunning, CreatedAt: proc.meta.CreatedAt}, nil
	}
	root, err := r.rootFor(h)
	if err != nil {
		return agentruntime.Info{}, err
	}
	meta, err := readMetadata(root)
	if err != nil {
		return agentruntime.Info{}, err
	}
	state := meta.State
	if state == agentruntime.StateRunning {
		state = agentruntime.StateExited
	}
	return agentruntime.Info{HandleID: runtimeID, State: state, CreatedAt: meta.CreatedAt}, nil
}

func (r *Runtime) rootFor(h agentruntime.Handle) (string, error) {
	runtimeID := strings.TrimSpace(h.RuntimeID)
	r.mu.Lock()
	root := r.roots[runtimeID]
	r.mu.Unlock()
	if root != "" {
		return root, nil
	}
	if r.deps.ResolveAgent == nil || r.deps.AgentHome == nil {
		return "", fmt.Errorf("DSH runtime %q location is unavailable", runtimeID)
	}
	ref, err := r.deps.ResolveAgent(h)
	if err != nil {
		return "", err
	}
	home, err := r.deps.AgentHome(identity.CanonicalAgentID(ref.ID))
	if err != nil {
		return "", err
	}
	return filepath.Join(home, hostStateDirName), nil
}

func readMetadata(root string) (runtimeMetadata, error) {
	data, err := os.ReadFile(filepath.Join(root, runtimeFileName))
	if err != nil {
		return runtimeMetadata{}, err
	}
	var meta runtimeMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return runtimeMetadata{}, fmt.Errorf("decode DSH runtime metadata: %w", err)
	}
	return meta, nil
}

func writeMetadata(root string, meta runtimeMetadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(root, runtimeFileName)
	tmp, err := os.CreateTemp(root, ".runtime-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary DSH runtime metadata: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temporary DSH runtime metadata: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary DSH runtime metadata: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temporary DSH runtime metadata: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary DSH runtime metadata: %w", err)
	}
	if err := replaceMetadataFile(tmpPath, path); err != nil {
		return fmt.Errorf("replace DSH runtime metadata: %w", err)
	}
	cleanup = false
	return nil
}

func (p *process) updateMetadata(update func(*runtimeMetadata)) error {
	if p == nil {
		return fmt.Errorf("DSH process is unavailable")
	}
	p.metadataMu.Lock()
	defer p.metadataMu.Unlock()
	p.mu.Lock()
	if update != nil {
		update(&p.meta)
	}
	meta := cloneRuntimeMetadata(p.meta)
	p.mu.Unlock()
	return writeMetadata(p.root, meta)
}

func cloneRuntimeMetadata(meta runtimeMetadata) runtimeMetadata {
	clone := meta
	clone.Sessions = make(map[string]string, len(meta.Sessions))
	for key, sessionID := range meta.Sessions {
		clone.Sessions[key] = sessionID
	}
	return clone
}

func persistedPermissionMode(meta runtimeMetadata) string {
	if mode := strings.TrimSpace(meta.PermissionMode); mode != "" {
		return mode
	}
	return defaultPermissionMode
}

func sessionsForPermissionMode(meta runtimeMetadata, permissionMode string) (map[string]string, bool) {
	changed := persistedPermissionMode(meta) != strings.TrimSpace(permissionMode)
	sessions := make(map[string]string, len(meta.Sessions))
	if changed {
		return sessions, true
	}
	for key, sessionID := range meta.Sessions {
		sessions[key] = sessionID
	}
	return sessions, false
}

func (r *Runtime) ValidateConfig(ctx context.Context, current agentruntime.RuntimeConfigSnapshot) error {
	profile := current.Profile
	// Host runtimes receive an Agent-scoped LLM Bridge URL and token only after
	// the catalog profile has passed this validation. Built-in providers keep
	// their upstream credentials out of the persisted Agent profile, so requiring
	// BaseURL/APIKey here would reject valid OpenCSG, Codex, and Claude references
	// before the bridge profile can be materialized.
	if strings.TrimSpace(profile.ModelID) == "" {
		return fmt.Errorf("DSH requires a model ID")
	}
	provider := strings.ToLower(strings.TrimSpace(profile.Provider))
	switch provider {
	case "csghub", "opencsg", "codex", "claude_code", "claude-code":
		// These providers resolve credentials dynamically in the local LLM Bridge.
	default:
		if strings.TrimSpace(profile.BaseURL) == "" || strings.TrimSpace(profile.APIKey) == "" {
			return fmt.Errorf("DSH provider %q requires API key and base URL", provider)
		}
	}
	if _, err := DecodeRuntimeOptions(current.Options); err != nil {
		return err
	}
	_, err := r.deps.ResolveBinary(ctx)
	return err
}

func (r *Runtime) RestartRequired(change agentruntime.RuntimeConfigChange) (bool, error) {
	if _, err := DecodeRuntimeOptions(change.Current.Options); err != nil {
		return false, err
	}
	return !reflect.DeepEqual(change.Previous, change.Current), nil
}

func (r *Runtime) ReconcileConfig(ctx context.Context, h agentruntime.Handle, change agentruntime.RuntimeConfigChange) error {
	root, err := r.rootFor(h)
	if err != nil {
		return err
	}
	profile := agentruntime.Profile{
		Provider:        change.Current.Profile.Provider,
		BaseURL:         change.Current.Profile.BaseURL,
		APIKey:          change.Current.Profile.APIKey,
		ModelID:         change.Current.Profile.ModelID,
		ModelMetadata:   change.Current.Profile.ModelMetadata,
		AutoCompact:     change.Current.Profile.AutoCompact,
		ReasoningEffort: change.Current.Profile.ReasoningEffort,
	}
	var ref AgentRef
	if r.deps.ResolveAgent != nil {
		ref, err = r.deps.ResolveAgent(h)
		if err != nil {
			return err
		}
		profile = ref.Profile
	}
	if err := validateExecutionProfile(profile); err != nil {
		return err
	}
	if err := writeSettings(filepath.Join(root, homeDirName, settingsFileName), profile); err != nil {
		return err
	}
	if r.deps.ResolveAgent == nil {
		return nil
	}
	projections, err := r.ExtensionProjections(ref.ID)
	if err != nil {
		return err
	}
	path := filepath.Join(root, workspaceDirName, "AGENTS.md")
	return r.renderExtensionInstructions(ctx, path, ref.ID, &ref.Instructions, projections)
}

func validateExecutionProfile(profile agentruntime.Profile) error {
	if !executionProfileComplete(profile) {
		return fmt.Errorf("DSH runtime profile requires API key, base URL, and model ID")
	}
	return nil
}

func executionProfileComplete(profile agentruntime.Profile) bool {
	profile = profile.Normalized()
	return profile.APIKey != "" && profile.BaseURL != "" && profile.ModelID != ""
}

func (r *Runtime) ValidateMCPServers(_ context.Context, current agentruntime.MCPServersSnapshot) error {
	_, err := buildACPMCPServers(current.Servers)
	return err
}

func (r *Runtime) MCPServersRestartRequired(change agentruntime.MCPServersChange) (bool, error) {
	if _, err := buildACPMCPServers(change.Current.Servers); err != nil {
		return false, err
	}
	return agentruntime.MCPServersNeedsRestart(change.Previous.Servers, change.Current.Servers)
}

func (r *Runtime) ReconcileMCPServers(_ context.Context, _ agentruntime.Handle, change agentruntime.MCPServersChange) error {
	_, err := buildACPMCPServers(change.Current.Servers)
	return err
}

func (r *Runtime) Close() error {
	r.mu.Lock()
	handles := make([]agentruntime.Handle, 0, len(r.processes))
	for runtimeID := range r.processes {
		handles = append(handles, agentruntime.Handle{RuntimeID: runtimeID})
	}
	r.mu.Unlock()
	var result error
	for _, handle := range handles {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := r.Stop(ctx, handle)
		cancel()
		result = errors.Join(result, err)
	}
	return result
}

func (r *Runtime) process(runtimeID string) (*process, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	proc := r.processes[strings.TrimSpace(runtimeID)]
	if !processRunning(proc) {
		return nil, fmt.Errorf("DSH runtime %q is not running", runtimeID)
	}
	return proc, nil
}

func processRunning(proc *process) bool {
	if proc == nil || proc.done == nil {
		return false
	}
	select {
	case <-proc.done:
		return false
	default:
		return true
	}
}

func dshProviderSettings(profile agentruntime.Profile) map[string]any {
	profile = profile.Normalized()
	inputModalities := append([]string(nil), profile.InputModalities...)
	if len(inputModalities) == 0 {
		inputModalities = []string{"text"}
	}
	metadata := profile.ModelMetadata.Normalized()
	return map[string]any{"protocol": "chat-completions", "baseURL": profile.BaseURL, "apiKeyEnv": llmAPIKeyEnvName, "models": []map[string]any{{"id": profile.ModelID, "inputModalities": inputModalities, "contextWindow": metadata.ContextWindow, "maxTokens": max(int64(1), min(int64(8192), metadata.ContextWindow/4))}}}
}
