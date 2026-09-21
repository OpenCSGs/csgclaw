package dsh

import (
	"context"
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
	hostStateDirName = ".dsh"
	homeDirName      = "home"
	workspaceDirName = "workspace"
	runtimeFileName  = "runtime.json"
	stderrFileName   = "stderr.log"
	settingsFileName = "settings.yaml"
	patchFileName    = "csgclaw.patch.yml"
	llmAPIKeyEnvName = "CSGCLAW_DSH_LLM_API_KEY"
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

type BinaryResolver func(context.Context, string) (dshcli.Info, error)

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
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	client    *acpClient
	stderr    *os.File
	root      string
	workspace string
	profile   agentruntime.Profile
	mcp       []acpMCPServer
	meta      runtimeMetadata
	done      chan struct{}

	metadataMu sync.Mutex
	mu         sync.Mutex
	active     map[string]*activeTurn
	ready      map[string]bool
}

type activeTurn struct {
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
	RuntimeID  string             `json:"runtime_id"`
	AgentID    string             `json:"agent_id"`
	Executable string             `json:"executable"`
	Version    string             `json:"version"`
	PID        int                `json:"pid,omitempty"`
	State      agentruntime.State `json:"state"`
	CreatedAt  time.Time          `json:"created_at"`
	Sessions   map[string]string  `json:"sessions,omitempty"`
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
		deps.ResolveBinary = func(ctx context.Context, explicit string) (dshcli.Info, error) {
			return (dshcli.Provider{ExplicitPath: explicit}).Resolve(ctx)
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

func (r *Runtime) Conversation(runtimeID string) contract.RuntimeConversation {
	return &conversation{runtime: r, runtimeID: strings.TrimSpace(runtimeID)}
}

func (r *Runtime) Provision(ctx context.Context, req agentruntime.ProvisionRequest) error {
	agentID := identity.CanonicalAgentID(req.AgentID)
	if agentID == "" || r.deps.AgentHome == nil {
		return fmt.Errorf("DSH agent home resolver is required")
	}
	agentHome, err := r.deps.AgentHome(agentID)
	if err != nil {
		return err
	}
	layout := r.Layout(agentHome)
	root := filepath.Dir(layout.WorkspaceRoot)
	if err := os.MkdirAll(filepath.Join(root, homeDirName), 0o755); err != nil {
		return fmt.Errorf("create DSH home: %w", err)
	}
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
	block := runtimeinstructions.RenderRuntimeAgentsInstructionsBlock(agentID, req.Instructions)
	document := strings.TrimSpace(base)
	if document != "" {
		document += "\n\n"
	}
	document += strings.TrimSpace(block) + "\n"
	if err := os.WriteFile(layout.InstructionsPath, []byte(document), 0o644); err != nil {
		return fmt.Errorf("write DSH AGENTS.md: %w", err)
	}
	if err := writeSettings(filepath.Join(root, homeDirName, settingsFileName), req.Profile); err != nil {
		return err
	}
	if err := writeRuntimePatch(filepath.Join(root, patchFileName)); err != nil {
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
	settings := map[string]any{
		"llm-deepseek": map[string]any{
			"protocol":  "chat-completions",
			"baseURL":   profile.BaseURL,
			"apiKeyEnv": llmAPIKeyEnvName,
			"models":    []map[string]any{{"id": profile.ModelID}},
		},
	}
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

func writeRuntimePatch(path string) error {
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
	if spec != nil && ref.Profile.ModelID == "" {
		ref.Profile = spec.Profile
	}
	opts, err := DecodeRuntimeOptions(ref.RuntimeOptions)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	binary, err := r.deps.ResolveBinary(ctx, opts.ExecutablePath)
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	agentHome, err := r.deps.AgentHome(identity.CanonicalAgentID(ref.ID))
	if err != nil {
		return agentruntime.StateUnknown, err
	}
	layout := r.Layout(agentHome)
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
	meta := runtimeMetadata{RuntimeID: runtimeID, AgentID: ref.ID, Executable: binary.Path, Version: binary.Version, State: agentruntime.StateCreated, CreatedAt: time.Now().UTC(), Sessions: map[string]string{}}
	if persisted, readErr := readMetadata(root); readErr == nil {
		meta.CreatedAt = persisted.CreatedAt
		meta.Sessions = persisted.Sessions
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return agentruntime.StateUnknown, readErr
	}
	if meta.Sessions == nil {
		meta.Sessions = map[string]string{}
	}
	if err := writeSettings(filepath.Join(root, homeDirName, settingsFileName), ref.Profile); err != nil {
		return agentruntime.StateUnknown, err
	}
	if err := writeRuntimePatch(filepath.Join(root, patchFileName)); err != nil {
		return agentruntime.StateUnknown, err
	}
	proc, err := r.launch(ctx, root, layout.WorkspaceRoot, binary.Path, ref.Profile, mcpServers, meta, true)
	if err != nil {
		patchErr := err
		slog.Warn("DSH present tool overlay unavailable; retrying base ACP profile", "runtime_id", runtimeID, "version", binary.Version, "error", patchErr)
		proc, err = r.launch(ctx, root, layout.WorkspaceRoot, binary.Path, ref.Profile, mcpServers, meta, false)
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

func (r *Runtime) launch(ctx context.Context, root, workspace, binary string, profile agentruntime.Profile, mcp []acpMCPServer, meta runtimeMetadata, enablePresent bool) (*process, error) {
	stderr, err := os.OpenFile(filepath.Join(root, stderrFileName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open DSH stderr log: %w", err)
	}
	cmd := exec.Command(binary, dshLaunchArgs(root, enablePresent)...)
	configureProcessGroup(cmd)
	cmd.Dir = workspace
	cmd.Env = buildEnvironment(profile, filepath.Join(root, homeDirName))
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
	proc := &process{cmd: cmd, stdin: stdin, client: client, stderr: stderr, root: root, workspace: workspace, profile: profile.Normalized(), mcp: mcp, meta: meta, done: make(chan struct{}), active: map[string]*activeTurn{}, ready: map[string]bool{}}
	client.setHandlers(
		func(request serverRequest) { r.handleServerRequest(proc, request) },
		func(note notification) { r.handleNotification(proc, note) },
	)
	var initialized struct {
		ProtocolVersion   int `json:"protocolVersion"`
		AgentCapabilities struct {
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
	args := []string{"--profile", "acp"}
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

func buildEnvironment(profile agentruntime.Profile, home string) []string {
	blocked := map[string]bool{"DSH_HOME": true, "DSH_AGENTS_HOME": true, llmAPIKeyEnvName: true}
	values := make(map[string]string, len(os.Environ())+len(profile.Env)+4)
	for _, item := range os.Environ() {
		key, value, found := strings.Cut(item, "=")
		if !found {
			continue
		}
		if !blocked[key] {
			values[key] = value
		}
	}
	for key, value := range profile.Env {
		if !blocked[key] {
			values[key] = value
		}
	}
	values["DSH_HOME"] = home
	values["DSH_AGENTS_HOME"] = filepath.Join(home, "agents")
	values[llmAPIKeyEnvName] = profile.APIKey
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

func (r *Runtime) ValidateConfig(ctx context.Context, current agentruntime.RuntimeConfigSnapshot) error {
	profile := current.Profile
	if strings.TrimSpace(profile.APIKey) == "" || strings.TrimSpace(profile.BaseURL) == "" || strings.TrimSpace(profile.ModelID) == "" {
		return fmt.Errorf("DSH requires API key, base URL, and model ID")
	}
	opts, err := DecodeRuntimeOptions(current.Options)
	if err != nil {
		return err
	}
	_, err = r.deps.ResolveBinary(ctx, opts.ExecutablePath)
	return err
}

func (r *Runtime) RestartRequired(change agentruntime.RuntimeConfigChange) (bool, error) {
	if _, err := DecodeRuntimeOptions(change.Current.Options); err != nil {
		return false, err
	}
	return !reflect.DeepEqual(change.Previous, change.Current), nil
}

func (r *Runtime) ReconcileConfig(_ context.Context, h agentruntime.Handle, change agentruntime.RuntimeConfigChange) error {
	root, err := r.rootFor(h)
	if err != nil {
		return err
	}
	if err := writeSettings(filepath.Join(root, homeDirName, settingsFileName), agentruntime.Profile{
		Provider: change.Current.Profile.Provider,
		BaseURL:  change.Current.Profile.BaseURL,
		ModelID:  change.Current.Profile.ModelID,
	}); err != nil {
		return err
	}
	if r.deps.ResolveAgent == nil {
		return nil
	}
	ref, err := r.deps.ResolveAgent(h)
	if err != nil {
		return err
	}
	path := filepath.Join(root, workspaceDirName, "AGENTS.md")
	current, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read DSH AGENTS.md: %w", err)
	}
	base := stripManagedInstructions(string(current))
	block := runtimeinstructions.RenderRuntimeAgentsInstructionsBlock(ref.ID, ref.Instructions)
	document := strings.TrimSpace(base)
	if document != "" {
		document += "\n\n"
	}
	document += strings.TrimSpace(block) + "\n"
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		return fmt.Errorf("write DSH AGENTS.md: %w", err)
	}
	return nil
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
